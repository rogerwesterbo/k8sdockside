package kube

import (
	"encoding/json"
	"math"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestRestartPatchStampsTheAnnotationKubectlUses(t *testing.T) {
	at := time.Date(2026, 9, 4, 15, 4, 5, 0, time.UTC)

	var got map[string]any
	if err := json.Unmarshal(restartPatch(at), &got); err != nil {
		t.Fatalf("restartPatch is not JSON: %v", err)
	}

	// The same key kubectl uses, so a restart from here and one from the
	// command line are the same event to everything watching.
	spec := got["spec"].(map[string]any)
	template := spec["template"].(map[string]any)
	metadata := template["metadata"].(map[string]any)
	annotations := metadata["annotations"].(map[string]any)
	if want := "2026-09-04T15:04:05Z"; annotations[restartedAt] != want {
		t.Errorf("restartPatch stamped %q, want %q", annotations[restartedAt], want)
	}
	if restartedAt != "kubectl.kubernetes.io/restartedAt" && restartedAt != "kubectl.kubernetes.io/restarted-at" {
		t.Errorf("restartedAt = %q, which is not the annotation kubectl writes", restartedAt)
	}
}

func TestCordonPatchSetsUnschedulable(t *testing.T) {
	for _, on := range []bool{true, false} {
		var got map[string]any
		if err := json.Unmarshal(cordonPatch(on), &got); err != nil {
			t.Fatalf("cordonPatch(%v) is not JSON: %v", on, err)
		}
		spec := got["spec"].(map[string]any)
		if spec["unschedulable"] != on {
			t.Errorf("cordonPatch(%v) set unschedulable=%v", on, spec["unschedulable"])
		}
	}
}

func TestStateOfReadsAReplicaCount(t *testing.T) {
	deployment := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "apps/v1",
		"kind":       "Deployment",
		"spec":       map[string]any{"replicas": int64(3)},
	}}

	got := stateOf(deployment)

	if !got.Scalable {
		t.Error("a Deployment with spec.replicas is not reported as scalable")
	}
	if got.Replicas != 3 {
		t.Errorf("Replicas = %d, want 3", got.Replicas)
	}
}

// A workload scaled to nothing is still scalable -- that is how it is scaled
// back up. Reading the field's presence rather than its value is the point.
func TestStateOfCallsAWorkloadAtZeroScalable(t *testing.T) {
	deployment := &unstructured.Unstructured{Object: map[string]any{
		"spec": map[string]any{"replicas": int64(0)},
	}}

	if got := stateOf(deployment); !got.Scalable || got.Replicas != 0 {
		t.Errorf("stateOf(scaled to zero) = %+v, want scalable at 0", got)
	}
}

func TestStateOfSaysAPodIsNotScalable(t *testing.T) {
	pod := &unstructured.Unstructured{Object: map[string]any{
		"kind": "Pod",
		"spec": map[string]any{"nodeName": "wrkr01"},
	}}

	if got := stateOf(pod); got.Scalable {
		t.Errorf("stateOf(pod) = %+v, want not scalable", got)
	}
}

func TestStateOfReadsACordonedNode(t *testing.T) {
	cordoned := &unstructured.Unstructured{Object: map[string]any{
		"kind": "Node",
		"spec": map[string]any{"unschedulable": true},
	}}
	open := &unstructured.Unstructured{Object: map[string]any{
		"kind": "Node",
		"spec": map[string]any{},
	}}

	if !stateOf(cordoned).Cordoned {
		t.Error("a node with spec.unschedulable is not reported as cordoned")
	}
	if stateOf(open).Cordoned {
		t.Error("a node without spec.unschedulable is reported as cordoned")
	}
}

// A replica count is an int32 in the API. A custom resource is free to put
// something else in the field, and a raw conversion would wrap it -- turning a
// nonsense number into a plausible negative one, or into zero.
func TestReplicaCountIsNarrowedRatherThanWrapped(t *testing.T) {
	cases := []struct {
		name string
		in   int64
		want int32
	}{
		{"an ordinary count", 3, 3},
		{"none", 0, 0},
		{"the largest that fits", math.MaxInt32, math.MaxInt32},
		{"larger than fits", math.MaxInt32 + 1, math.MaxInt32},
		{"very much larger", math.MaxInt64, math.MaxInt32},
		{"a negative count, which is not a thing", -5, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := replicaCount(c.in); got != c.want {
				t.Errorf("replicaCount(%d) = %d, want %d", c.in, got, c.want)
			}
		})
	}
}
