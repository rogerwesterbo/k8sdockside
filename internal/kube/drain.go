package kube

// Draining a node: closing it to new work, then moving what is on it.
//
// This is the one action that is a process rather than a call. It cordons, then
// evicts every pod that should go, one at a time, honouring the disruption
// budgets that exist precisely to stop this operation taking a service down. It
// can take minutes, so it runs in the background and reports as it goes.
//
// What it will and will not touch follows `kubectl drain`. With no options it
// is kubectl with no flags: the two things kubectl refuses without being told
// twice -- pods holding data in an emptyDir, and pods nothing would recreate --
// are refused here too, and named. DrainOptions carries the flags that tell it
// twice, and the ones k9s's drain dialog asks for: a grace period, a timeout,
// and deleting rather than evicting.

import (
	"context"
	"errors"
	"fmt"
	"time"

	policyv1 "k8s.io/api/policy/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
)

// mirrorPod marks the API server's read-only copy of a pod the kubelet runs
// from a file on disk. Evicting the copy does nothing to what is running.
const mirrorPod = "kubernetes.io/config.mirror"

// The options a refusal can name as the one that would have moved its pod. The
// strings are the DrainOptions field names as JSON has them, so the panel can
// tick the right box without a table of its own.
const (
	optionForce              = "force"
	optionDeleteEmptyDirData = "deleteEmptyDirData"
)

// DrainOptions are the choices `kubectl drain` takes as flags. The zero value
// is kubectl with none of them.
type DrainOptions struct {
	// GracePeriodSeconds overrides each pod's own terminationGracePeriodSeconds.
	// Nil leaves every pod its own, which is kubectl's -1.
	GracePeriodSeconds *int64 `json:"gracePeriodSeconds"`
	// TimeoutSeconds gives up on the drain after this long. Zero waits for as
	// long as the disruption budgets take.
	TimeoutSeconds int64 `json:"timeoutSeconds"`
	// DeleteEmptyDirData moves pods holding data in an emptyDir volume, and
	// the data goes with them: kubectl's --delete-emptydir-data.
	DeleteEmptyDirData bool `json:"deleteEmptyDirData"`
	// Force moves pods nothing manages, which deletes them for good:
	// kubectl's --force.
	Force bool `json:"force"`
	// DisableEviction deletes pods rather than evicting them, which means no
	// disruption budget is asked: kubectl's --disable-eviction.
	DisableEviction bool `json:"disableEviction"`
	// PodSelector narrows the drain to pods carrying these labels, in the
	// usual selector syntax: kubectl's --pod-selector.
	PodSelector string `json:"podSelector"`
}

// Validate says what is wrong with the options before a drain starts on them,
// so a mistyped selector is an answer to the click rather than a drain that
// cordons the node and then fails.
func (o DrainOptions) Validate() error {
	if o.GracePeriodSeconds != nil && *o.GracePeriodSeconds < 0 {
		return fmt.Errorf("a grace period of %ds is not a grace period -- leave it empty for each pod's own", *o.GracePeriodSeconds)
	}
	if o.TimeoutSeconds < 0 {
		return fmt.Errorf("a timeout of %ds is not a timeout -- leave it empty to wait as long as it takes", o.TimeoutSeconds)
	}
	if _, err := labels.Parse(o.PodSelector); err != nil {
		return fmt.Errorf("pod selector %q: %w", o.PodSelector, err)
	}
	return nil
}

// PodRef names one pod. Namespace and name, because that is all an eviction
// needs and all the panel shows.
type PodRef struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
}

// Refusal is a pod the drain would not move, and why. It is reported rather
// than acted on: the reasons are the ones kubectl needs a second flag for.
type Refusal struct {
	Pod    PodRef `json:"pod"`
	Reason string `json:"reason"`
	// Option is the DrainOptions field that would have moved it: "force" or
	// "deleteEmptyDirData".
	Option string `json:"option"`
}

// disposition is what a drain will do with one pod.
type disposition int

const (
	evictIt disposition = iota
	skipIt
	refuseIt
)

func (d disposition) String() string {
	switch d {
	case evictIt:
		return "evict"
	case skipIt:
		return "skip"
	default:
		return "refuse"
	}
}

// classify decides what a drain does with one pod, and for a refusal, why and
// which option would have moved it.
//
// The order of the checks is load-bearing. A mirror pod has no owner, so the
// unmanaged check would refuse it if it ran first; a DaemonSet's pod often
// scratches to an emptyDir, so the local-data check would refuse every node
// running one. Both are settled before either refusal can fire. The two
// refusals are independent of each other: forcing a bare pod off the node does
// not also agree to losing its emptyDir, any more than it does in kubectl.
func classify(p *unstructured.Unstructured, opts DrainOptions) (what disposition, why, option string) {
	if _, found := p.GetAnnotations()[mirrorPod]; found {
		return skipIt, "the kubelet runs it from a file, so evicting the API server's copy would do nothing", ""
	}

	if phase, _, _ := unstructured.NestedString(p.Object, "status", "phase"); phase == "Succeeded" || phase == "Failed" {
		return skipIt, "it has already finished", ""
	}

	owners := p.GetOwnerReferences()
	for i := range owners {
		if owners[i].Kind == "DaemonSet" {
			return skipIt, "a DaemonSet would put it straight back", ""
		}
	}
	if len(owners) == 0 && !opts.Force {
		return refuseIt, "nothing manages it, so evicting it would delete it for good", optionForce
	}

	if !opts.DeleteEmptyDirData {
		volumes, _, _ := unstructured.NestedSlice(p.Object, "spec", "volumes")
		for _, v := range volumes {
			volume, ok := v.(map[string]any)
			if !ok {
				continue
			}
			if _, found := volume["emptyDir"]; found {
				return refuseIt, "it holds data in an emptyDir volume, which evicting it would destroy", optionDeleteEmptyDirData
			}
		}
	}

	return evictIt, "", ""
}

// DrainPlan is what a drain will do, settled before it starts anything so the
// panel can say what is about to happen and what will be left behind.
type DrainPlan struct {
	Evict   []PodRef  `json:"evict"`
	Refused []Refusal `json:"refused"`
}

// planDrain sorts a node's pods into the ones that will be moved and the ones
// that will not.
func planDrain(pods []unstructured.Unstructured, opts DrainOptions) DrainPlan {
	plan := DrainPlan{Evict: []PodRef{}, Refused: []Refusal{}}
	for i := range pods {
		ref := PodRef{Namespace: pods[i].GetNamespace(), Name: pods[i].GetName()}
		switch what, why, option := classify(&pods[i], opts); what {
		case evictIt:
			plan.Evict = append(plan.Evict, ref)
		case refuseIt:
			plan.Refused = append(plan.Refused, Refusal{Pod: ref, Reason: why, Option: option})
		}
	}
	return plan
}

// eviction is a drain's whole dependency on a cluster, taken as functions so
// that the loop around them -- ordering, budget retries, cancellation, progress
// -- is testable without one.
type eviction struct {
	evict func(context.Context, PodRef) error
	wait  func(context.Context, int) error
}

// evictAll moves every pod off the node, reporting the running count.
//
// Pods a disruption budget is holding back come round again rather than
// failing: a 429 is the budget doing its job, and giving up on it would make
// this app's drain less careful than kubectl's. Anything else stops the drain,
// because a drain that carries on past a real refusal leaves a node half moved
// with nothing saying so.
func evictAll(ctx context.Context, targets []PodRef, ev eviction, report func(evicted int)) error {
	remaining := targets
	evicted := 0

	for attempt := 0; len(remaining) > 0; attempt++ {
		var held []PodRef
		for _, pod := range remaining {
			if err := ctx.Err(); err != nil {
				return err
			}
			err := ev.evict(ctx, pod)
			switch {
			// Already gone is the outcome we were after.
			case err == nil, apierrors.IsNotFound(err):
				evicted++
				report(evicted)
			case apierrors.IsTooManyRequests(err):
				held = append(held, pod)
			default:
				return fmt.Errorf("evicting %s/%s: %w", pod.Namespace, pod.Name, err)
			}
		}
		if len(held) == 0 {
			return nil
		}
		remaining = held
		if err := ev.wait(ctx, attempt); err != nil {
			return err
		}
	}
	return nil
}

// backoff waits before asking a disruption budget again, lengthening the pause
// so a budget that is going to hold for a while is not asked every second.
func backoff(ctx context.Context, attempt int) error {
	pause := time.Duration(1<<min(attempt, 4)) * time.Second
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(pause):
		return nil
	}
}

// DrainProgress is one report from a drain in flight.
type DrainProgress struct {
	DrainID string `json:"drainId"`
	Node    string `json:"node"`
	// Phase is "cordoning", "planning", "evicting", "done" or "failed".
	Phase   string    `json:"phase"`
	Evicted int       `json:"evicted"`
	Total   int       `json:"total"`
	Refused []Refusal `json:"refused"`
	Error   string    `json:"error"`
	Done    bool      `json:"done"`
}

// Drain closes a node and moves everything off it, reporting as it goes.
//
// It blocks until the node is clear, the context is cancelled, the timeout in
// the options runs out, or something refuses; the caller runs it in a
// goroutine. A drain that stops early leaves the node cordoned on purpose -- a
// node half emptied must not quietly start taking work again.
func (w *Watcher) Drain(ctx context.Context, kc Context, id, node string, opts DrainOptions, report func(DrainProgress)) error {
	if err := opts.Validate(); err != nil {
		return err
	}

	var timeout time.Duration
	if opts.TimeoutSeconds > 0 {
		timeout = time.Duration(opts.TimeoutSeconds) * time.Second
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	at := DrainProgress{DrainID: id, Node: node, Refused: []Refusal{}}
	fail := func(err error) error {
		// A request cut off by the deadline fails in its own words, which name
		// a context rather than the timeout the user set.
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			err = fmt.Errorf("gave up after %s with %d of %d pods moved; the node stays cordoned", timeout, at.Evicted, at.Total)
		}
		at.Phase, at.Error, at.Done = "failed", err.Error(), true
		report(at)
		return err
	}

	return w.withClient(kc, func(c *clusterClient) error {
		at.Phase = "cordoning"
		report(at)
		if err := c.cordon(ctx, node, true); err != nil {
			return fail(err)
		}

		at.Phase = "planning"
		report(at)
		pods, _, err := c.list(ctx, KindPods, listOnNode(node, opts.PodSelector))
		if err != nil {
			return fail(err)
		}

		plan := planDrain(pods, opts)
		at.Phase, at.Total, at.Refused = "evicting", len(plan.Evict), plan.Refused
		report(at)

		ev := eviction{evict: c.remover(opts), wait: backoff}
		if err := evictAll(ctx, plan.Evict, ev, func(evicted int) {
			at.Evicted = evicted
			report(at)
		}); err != nil {
			return fail(err)
		}

		at.Phase, at.Done = "done", true
		report(at)
		return nil
	})
}

// listOnNode narrows a pod listing to one node, and to a label selector when
// there is one. Both are served by the API server, so a large cluster does not
// send every pod it has across the wire for us to filter.
func listOnNode(node, selector string) metav1.ListOptions {
	return metav1.ListOptions{FieldSelector: "spec.nodeName=" + node, LabelSelector: selector}
}

// remover is how this drain takes one pod off the node: an eviction, unless the
// options say to delete.
func (c *clusterClient) remover(opts DrainOptions) func(context.Context, PodRef) error {
	if opts.DisableEviction {
		return func(ctx context.Context, p PodRef) error { return c.deletePod(ctx, p, opts.GracePeriodSeconds) }
	}
	return func(ctx context.Context, p PodRef) error { return c.evictPod(ctx, p, opts.GracePeriodSeconds) }
}

// evictPod asks the API server to evict one pod.
//
// An eviction rather than a delete: it is the request PodDisruptionBudgets are
// evaluated against, so a budget can refuse it. Deleting the pod would go
// through regardless and take the service down that the budget exists to keep
// up.
func (c *clusterClient) evictPod(ctx context.Context, p PodRef, grace *int64) error {
	ev := &policyv1.Eviction{
		ObjectMeta: metav1.ObjectMeta{Namespace: p.Namespace, Name: p.Name},
	}
	if grace != nil {
		ev.DeleteOptions = &metav1.DeleteOptions{GracePeriodSeconds: grace}
	}
	return c.typed.PolicyV1().Evictions(p.Namespace).Evict(ctx, ev)
}

// deletePod removes one pod without asking any disruption budget. Only reached
// when the drain was told to, because it is exactly what evictPod exists to
// avoid.
func (c *clusterClient) deletePod(ctx context.Context, p PodRef, grace *int64) error {
	return c.typed.CoreV1().Pods(p.Namespace).Delete(ctx, p.Name, metav1.DeleteOptions{GracePeriodSeconds: grace})
}
