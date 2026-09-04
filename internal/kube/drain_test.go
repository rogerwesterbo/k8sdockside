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

func TestClassifyEvictsAnOrdinaryPod(t *testing.T) {
	got, reason := classify(testPod("web", nil))

	if got != evictIt {
		t.Errorf("classify(a replicaset's pod) = %v (%s), want evict", got, reason)
	}
}

// A DaemonSet puts a pod back on the node the moment it goes, so evicting one
// is work that undoes itself. kubectl skips them; so do we.
func TestClassifySkipsDaemonSetPods(t *testing.T) {
	got, _ := classify(testPod("node-exporter", owners("DaemonSet")))

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

	got, reason := classify(mirror)

	if got != skipIt {
		t.Errorf("classify(a mirror pod) = %v (%s), want skip", got, reason)
	}
}

func TestClassifySkipsPodsThatHaveAlreadyFinished(t *testing.T) {
	for _, phase := range []string{"Succeeded", "Failed"} {
		done := testPod("batch", func(obj map[string]any) {
			obj["status"] = map[string]any{"phase": phase}
		})
		if got, _ := classify(done); got != skipIt {
			t.Errorf("classify(a %s pod) = %v, want skip", phase, got)
		}
	}
}

// kubectl refuses these without --delete-emptydir-data, because evicting the
// pod destroys the data. We have no such flag, so we refuse and say which pod.
func TestClassifyRefusesAPodHoldingLocalData(t *testing.T) {
	withData := testPod("cache", func(obj map[string]any) {
		spec := obj["spec"].(map[string]any)
		spec["volumes"] = []any{
			map[string]any{"name": "config", "configMap": map[string]any{"name": "c"}},
			map[string]any{"name": "scratch", "emptyDir": map[string]any{}},
		}
	})

	got, reason := classify(withData)

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
	bare := testPod("debug", func(obj map[string]any) {
		delete(obj["metadata"].(map[string]any), "ownerReferences")
	})

	got, reason := classify(bare)

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
	both := testPod("fluent-bit", func(obj map[string]any) {
		meta := obj["metadata"].(map[string]any)
		meta["ownerReferences"] = []any{map[string]any{"kind": "DaemonSet", "name": "fluent-bit"}}
		obj["spec"].(map[string]any)["volumes"] = []any{
			map[string]any{"name": "buf", "emptyDir": map[string]any{}},
		}
	})

	if got, _ := classify(both); got != skipIt {
		t.Errorf("classify(a daemonset pod with emptyDir) = %v, want skip", got)
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
		*testPod("debug", func(obj map[string]any) { // refused
			delete(obj["metadata"].(map[string]any), "ownerReferences")
		}),
	}

	plan := planDrain(pods)

	if want := []PodRef{{Namespace: "default", Name: "web"}}; !slices.Equal(plan.Evict, want) {
		t.Errorf("Evict = %v, want %v", plan.Evict, want)
	}
	if len(plan.Refused) != 1 || plan.Refused[0].Pod.Name != "debug" {
		t.Errorf("Refused = %v, want the bare pod alone", plan.Refused)
	}
	// A skipped pod belongs in neither list: it is not work, and it is not a
	// refusal the user has to do anything about.
	for _, r := range plan.Refused {
		if r.Pod.Name == "node-exporter" {
			t.Error("a DaemonSet's pod was reported as a refusal")
		}
	}
}
