package kube

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// pod builds one pod as the API server would hand it over, with only the fields
// a drain looks at.
func testPod(name string, tweak func(map[string]any)) *unstructured.Unstructured {
	obj := map[string]any{
		"apiVersion": "v1",
		"kind":       "Pod",
		"metadata": map[string]any{
			"name":      name,
			"namespace": "default",
			"ownerReferences": []any{
				map[string]any{"kind": "ReplicaSet", "name": "web-7d4"},
			},
		},
		"spec":   map[string]any{"nodeName": "wrkr01"},
		"status": map[string]any{"phase": "Running"},
	}
	if tweak != nil {
		tweak(obj)
	}
	return &unstructured.Unstructured{Object: obj}
}

func owners(kind string) func(map[string]any) {
	return func(obj map[string]any) {
		meta := obj["metadata"].(map[string]any)
		meta["ownerReferences"] = []any{map[string]any{"kind": kind, "name": "somebody"}}
	}
}

// unmanaged takes a pod's owners away: a pod somebody ran by hand.
func unmanaged(obj map[string]any) {
	delete(obj["metadata"].(map[string]any), "ownerReferences")
}

// scratch gives a pod an emptyDir beside a volume that holds nothing local.
func scratch(obj map[string]any) {
	obj["spec"].(map[string]any)["volumes"] = []any{
		map[string]any{"name": "config", "configMap": map[string]any{"name": "c"}},
		map[string]any{"name": "scratch", "emptyDir": map[string]any{}},
	}
}

// both applies several tweaks to one pod.
func both(tweaks ...func(map[string]any)) func(map[string]any) {
	return func(obj map[string]any) {
		for _, t := range tweaks {
			t(obj)
		}
	}
}

func TestClassifyEvictsAnOrdinaryPod(t *testing.T) {
	got, reason, _ := classify(testPod("web", nil), DrainOptions{})

	if got != evictIt {
		t.Errorf("classify(a replicaset's pod) = %v (%s), want evict", got, reason)
	}
}

// A DaemonSet puts a pod back on the node the moment it goes, so evicting one
// is work that undoes itself. kubectl skips them; so do we.
func TestClassifySkipsDaemonSetPods(t *testing.T) {
	got, _, _ := classify(testPod("node-exporter", owners("DaemonSet")), DrainOptions{})

	if got != skipIt {
		t.Errorf("classify(a daemonset's pod) = %v, want skip", got)
	}
}

// A mirror pod is the API server's copy of something the kubelet runs from
// disk. Evicting the copy does nothing to the pod, and it has no owner -- so it
// has to be recognised before the unmanaged check refuses it.
func TestClassifySkipsMirrorPodsRatherThanRefusingThem(t *testing.T) {
	mirror := testPod("kube-apiserver-cp01", func(obj map[string]any) {
		meta := obj["metadata"].(map[string]any)
		delete(meta, "ownerReferences")
		meta["annotations"] = map[string]any{mirrorPod: "0123456789abcdef"}
	})

	got, reason, _ := classify(mirror, DrainOptions{})

	if got != skipIt {
		t.Errorf("classify(a mirror pod) = %v (%s), want skip", got, reason)
	}
}

func TestClassifySkipsPodsThatHaveAlreadyFinished(t *testing.T) {
	for _, phase := range []string{"Succeeded", "Failed"} {
		done := testPod("batch", func(obj map[string]any) {
			obj["status"] = map[string]any{"phase": phase}
		})
		if got, _, _ := classify(done, DrainOptions{}); got != skipIt {
			t.Errorf("classify(a %s pod) = %v, want skip", phase, got)
		}
	}
}

// kubectl refuses these without --delete-emptydir-data, because evicting the
// pod destroys the data. Without the option we refuse too, and say which pod.
func TestClassifyRefusesAPodHoldingLocalData(t *testing.T) {
	got, reason, _ := classify(testPod("cache", scratch), DrainOptions{})

	if got != refuseIt {
		t.Fatalf("classify(a pod with emptyDir) = %v, want refuse", got)
	}
	if reason == "" {
		t.Error("a refusal with no reason tells the user nothing")
	}
}

// kubectl refuses these without --force: nothing would recreate the pod, so
// evicting it is deleting it.
func TestClassifyRefusesAPodNothingManages(t *testing.T) {
	got, reason, _ := classify(testPod("debug", unmanaged), DrainOptions{})

	if got != refuseIt {
		t.Fatalf("classify(a bare pod) = %v, want refuse", got)
	}
	if reason == "" {
		t.Error("a refusal with no reason tells the user nothing")
	}
}

// Precedence: a DaemonSet's pod is skipped whatever else is true of it. Were
// the emptyDir check to win, every node running a DaemonSet that scratches to
// disk would refuse to drain.
func TestClassifySkipsADaemonSetPodEvenWithLocalData(t *testing.T) {
	pod := testPod("fluent-bit", both(owners("DaemonSet"), scratch))

	if got, _, _ := classify(pod, DrainOptions{}); got != skipIt {
		t.Errorf("classify(a daemonset pod with emptyDir) = %v, want skip", got)
	}
}

// ----- the options ------------------------------------------------------

// --delete-emptydir-data: the user has agreed to lose the data, so the pod goes.
func TestClassifyEvictsAPodHoldingLocalDataWhenToldTo(t *testing.T) {
	got, reason, _ := classify(testPod("cache", scratch), DrainOptions{DeleteEmptyDirData: true})

	if got != evictIt {
		t.Errorf("classify(a pod with emptyDir, delete-emptydir-data) = %v (%s), want evict", got, reason)
	}
}

// --force: the user has agreed the pod is gone for good, so it goes.
func TestClassifyEvictsAPodNothingManagesWhenForced(t *testing.T) {
	got, reason, _ := classify(testPod("debug", unmanaged), DrainOptions{Force: true})

	if got != evictIt {
		t.Errorf("classify(a bare pod, force) = %v (%s), want evict", got, reason)
	}
}

// Force is agreement to lose the pod, not its data. A bare pod with an emptyDir
// still needs the second option, exactly as it does in kubectl.
func TestForceDoesNotAlsoAgreeToLosingLocalData(t *testing.T) {
	got, _, option := classify(testPod("debug", both(unmanaged, scratch)), DrainOptions{Force: true})

	if got != refuseIt {
		t.Fatalf("classify(a bare pod with emptyDir, force) = %v, want refuse", got)
	}
	if option != optionDeleteEmptyDirData {
		t.Errorf("option = %q, want %q", option, optionDeleteEmptyDirData)
	}
}

// The options widen what may be moved, never what is skipped: a DaemonSet's pod
// evicted is a pod put straight back.
func TestTheOptionsNeverMoveAPodADrainSkips(t *testing.T) {
	all := DrainOptions{Force: true, DeleteEmptyDirData: true, DisableEviction: true}
	pod := testPod("fluent-bit", both(owners("DaemonSet"), scratch))

	if got, _, _ := classify(pod, all); got != skipIt {
		t.Errorf("classify(a daemonset pod, every option) = %v, want skip", got)
	}
}

// A refusal names the option that would have moved its pod, which is how the
// panel ticks the right box when asked to drain what was left.
func TestARefusalNamesTheOptionThatWouldHaveMovedIt(t *testing.T) {
	cases := []struct {
		name string
		pod  *unstructured.Unstructured
		want string
	}{
		{"a bare pod", testPod("debug", unmanaged), optionForce},
		{"a pod with emptyDir", testPod("cache", scratch), optionDeleteEmptyDirData},
	}
	for _, c := range cases {
		if _, _, option := classify(c.pod, DrainOptions{}); option != c.want {
			t.Errorf("%s: option = %q, want %q", c.name, option, c.want)
		}
	}
}

func TestValidateDrainOptions(t *testing.T) {
	negative, zero := int64(-1), int64(0)
	cases := []struct {
		name string
		opts DrainOptions
		ok   bool
	}{
		{"the defaults", DrainOptions{}, true},
		{"a grace period of nothing, which is kill now", DrainOptions{GracePeriodSeconds: &zero}, true},
		{"a negative grace period", DrainOptions{GracePeriodSeconds: &negative}, false},
		{"a negative timeout", DrainOptions{TimeoutSeconds: -5}, false},
		{"a selector", DrainOptions{PodSelector: "app=web,tier!=db"}, true},
		{"a selector that does not parse", DrainOptions{PodSelector: "app in (web"}, false},
	}
	for _, c := range cases {
		err := c.opts.Validate()
		if c.ok && err != nil {
			t.Errorf("%s: Validate = %v, want nil", c.name, err)
		}
		if !c.ok && err == nil {
			t.Errorf("%s: Validate = nil, want an error", c.name)
		}
	}
}

// The selector is the API server's to apply, beside the node, so the listing is
// only ever the pods the drain is about.
func TestListOnNodeCarriesTheSelector(t *testing.T) {
	opts := listOnNode("wrkr01", "app=web")

	if opts.FieldSelector != "spec.nodeName=wrkr01" {
		t.Errorf("FieldSelector = %q", opts.FieldSelector)
	}
	if opts.LabelSelector != "app=web" {
		t.Errorf("LabelSelector = %q, want app=web", opts.LabelSelector)
	}
}

// ----- the eviction loop ------------------------------------------------

/** A recorder standing in for the cluster, so the loop can be driven exactly. */
type evictions struct {
	calls  []string
	fail   map[string]error
	waited int
}

func (e *evictions) run(_ context.Context, p PodRef) error {
	e.calls = append(e.calls, p.Name)
	if err, ok := e.fail[p.Name]; ok {
		delete(e.fail, p.Name)
		return err
	}
	return nil
}

func (e *evictions) hold(_ context.Context, _ int) error {
	e.waited++
	return nil
}

func (e *evictions) eviction() eviction {
	return eviction{evict: e.run, wait: e.hold}
}

func refs(names ...string) []PodRef {
	out := make([]PodRef, 0, len(names))
	for _, n := range names {
		out = append(out, PodRef{Namespace: "default", Name: n})
	}
	return out
}

func TestEvictAllEvictsEveryPodAndCountsThemOff(t *testing.T) {
	cluster := &evictions{}
	var progress []int

	err := evictAll(context.Background(), refs("a", "b", "c"), cluster.eviction(), func(done int) {
		progress = append(progress, done)
	})

	if err != nil {
		t.Fatalf("evictAll: %v", err)
	}
	if got := len(cluster.calls); got != 3 {
		t.Errorf("evicted %d pods, want 3", got)
	}
	// Counted off one at a time, which is what the panel draws a bar from.
	if want := []int{1, 2, 3}; !slices.Equal(progress, want) {
		t.Errorf("progress = %v, want %v", progress, want)
	}
}

// A 429 is a PodDisruptionBudget saying "not yet", not a failure. Treating it
// as one would abandon the drain the first time a budget did its job.
func TestEvictAllRetriesAPodABudgetIsHoldingBack(t *testing.T) {
	cluster := &evictions{fail: map[string]error{"b": apierrors.NewTooManyRequests("budget", 1)}}

	err := evictAll(context.Background(), refs("a", "b"), cluster.eviction(), func(int) {})

	if err != nil {
		t.Fatalf("evictAll gave up on a budget: %v", err)
	}
	if want := []string{"a", "b", "b"}; !slices.Equal(cluster.calls, want) {
		t.Errorf("calls = %v, want %v", cluster.calls, want)
	}
	if cluster.waited == 0 {
		t.Error("retried without waiting, which is a hot loop against the API server")
	}
}

// A pod that has already gone is the outcome we wanted, not an error to stop on.
func TestEvictAllTreatsAPodAlreadyGoneAsEvicted(t *testing.T) {
	gone := apierrors.NewNotFound(schema.GroupResource{Resource: "pods"}, "b")
	cluster := &evictions{fail: map[string]error{"b": gone}}
	done := 0

	err := evictAll(context.Background(), refs("a", "b"), cluster.eviction(), func(n int) { done = n })

	if err != nil {
		t.Fatalf("evictAll: %v", err)
	}
	if done != 2 {
		t.Errorf("counted %d evicted, want 2", done)
	}
}

func TestEvictAllStopsAndSaysWhichPodRefused(t *testing.T) {
	cluster := &evictions{fail: map[string]error{"b": errors.New("forbidden")}}

	err := evictAll(context.Background(), refs("a", "b", "c"), cluster.eviction(), func(int) {})

	if err == nil {
		t.Fatal("evictAll carried on past a refusal")
	}
	if !strings.Contains(err.Error(), "b") {
		t.Errorf("error %q does not name the pod that refused", err)
	}
	// And it stopped there rather than working through the rest.
	if want := []string{"a", "b"}; !slices.Equal(cluster.calls, want) {
		t.Errorf("calls = %v, want %v", cluster.calls, want)
	}
}

func TestEvictAllStopsWhenCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cluster := &evictions{}
	cancel()

	err := evictAll(ctx, refs("a", "b"), cluster.eviction(), func(int) {})

	if !errors.Is(err, context.Canceled) {
		t.Errorf("evictAll after cancel = %v, want context.Canceled", err)
	}
	if len(cluster.calls) != 0 {
		t.Errorf("evicted %v after being cancelled", cluster.calls)
	}
}

func TestPlanDrainSortsPodsIntoWhatMovesAndWhatDoesNot(t *testing.T) {
	pods := []unstructured.Unstructured{
		*testPod("web", nil),                           // moves
		*testPod("node-exporter", owners("DaemonSet")), // skipped, and named nowhere
		*testPod("debug", unmanaged),                   // refused
	}

	plan := planDrain(pods, DrainOptions{})

	if want := []PodRef{{Namespace: "default", Name: "web"}}; !slices.Equal(plan.Evict, want) {
		t.Errorf("Evict = %v, want %v", plan.Evict, want)
	}
	if len(plan.Refused) != 1 || plan.Refused[0].Pod.Name != "debug" {
		t.Fatalf("Refused = %v, want the bare pod alone", plan.Refused)
	}
	if plan.Refused[0].Option != optionForce {
		t.Errorf("Refused[0].Option = %q, want %q", plan.Refused[0].Option, optionForce)
	}
	// A skipped pod belongs in neither list: it is not work, and it is not a
	// refusal the user has to do anything about.
	for _, r := range plan.Refused {
		if r.Pod.Name == "node-exporter" {
			t.Error("a DaemonSet's pod was reported as a refusal")
		}
	}
}

// The options move what the defaults leave: with both, nothing is left behind.
func TestPlanDrainWithTheOptionsLeavesNothingBehind(t *testing.T) {
	pods := []unstructured.Unstructured{
		*testPod("web", nil),
		*testPod("debug", unmanaged),
		*testPod("cache", scratch),
	}

	plan := planDrain(pods, DrainOptions{Force: true, DeleteEmptyDirData: true})

	if len(plan.Evict) != 3 {
		t.Errorf("Evict = %v, want all three", plan.Evict)
	}
	if len(plan.Refused) != 0 {
		t.Errorf("Refused = %v, want none", plan.Refused)
	}
}
