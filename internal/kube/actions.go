package kube

// Acting on one object: deleting it, scaling it, restarting it, cordoning a
// node.
//
// These are the operations that change a cluster from a button rather than
// from the editor. Everything here goes through the same dynamic client and
// REST mapper the rest of the app uses, so a custom resource is deleted by
// exactly the path a Pod is, and none of it needs a kind compiled in.
//
// The wrappers are deliberately thin. What decides anything -- which patch to
// send, what an object's state means for the buttons -- is a plain function
// below, which is what the tests are pointed at.

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
)

// restartedAt is the annotation `kubectl rollout restart` stamps on a pod
// template. Writing the same one means a restart from this app and one from the
// command line are indistinguishable to everything watching -- and that the two
// do not fight over separate annotations that mean the same thing.
const restartedAt = "kubectl.kubernetes.io/restarted-at"

// ObjectState is what the action bar needs to know about an object beyond its
// name: which of its buttons apply, and what they should start from.
type ObjectState struct {
	// Scalable reports whether the object carries a replica count. The
	// workloads that have one are exactly the ones worth offering Scale for.
	Scalable bool  `json:"scalable"`
	Replicas int32 `json:"replicas"`
	// Cordoned is a node's spec.unschedulable, which decides whether its button
	// offers to cordon or to uncordon.
	Cordoned bool `json:"cordoned"`
}

// stateOf reads an object for the few facts the action bar needs.
//
// It reads spec rather than asking discovery what subresources exist: a replica
// count in the spec is the same question as "can this be scaled", one Get
// answers it, and the answer also pre-fills the field the user is about to type
// into.
func stateOf(u *unstructured.Unstructured) ObjectState {
	var out ObjectState
	if replicas, found, err := unstructured.NestedInt64(u.Object, "spec", "replicas"); err == nil && found {
		out.Scalable = true
		out.Replicas = replicaCount(replicas)
	}
	if cordoned, _, err := unstructured.NestedBool(u.Object, "spec", "unschedulable"); err == nil {
		out.Cordoned = cordoned
	}
	return out
}

// replicaCount narrows a replica count to the int32 the API uses.
//
// Unstructured objects carry numbers as int64, and a custom resource is free to
// put anything in the field. Narrowing it by conversion alone would wrap: a
// count too large to fit comes back negative, which reads as a real answer
// rather than as the nonsense it is.
func replicaCount(n int64) int32 {
	switch {
	case n < 0:
		return 0
	case n > math.MaxInt32:
		return math.MaxInt32
	default:
		return int32(n)
	}
}

// restartPatch is the merge patch that rolls a workload: a new value under the
// annotation kubectl uses, which changes the pod template and so starts a
// rollout without anything else about the object changing.
func restartPatch(at time.Time) []byte {
	// Marshalled rather than written as a string so the timestamp cannot be
	// pasted into the document unescaped.
	patch, _ := json.Marshal(map[string]any{
		"spec": map[string]any{
			"template": map[string]any{
				"metadata": map[string]any{
					"annotations": map[string]any{restartedAt: at.UTC().Format(time.RFC3339)},
				},
			},
		},
	})
	return patch
}

// cordonPatch is the merge patch that closes a node to new work, or reopens it.
func cordonPatch(on bool) []byte {
	patch, _ := json.Marshal(map[string]any{"spec": map[string]any{"unschedulable": on}})
	return patch
}

// ObjectState reads one object for what its action bar needs to know.
func (w *Watcher) ObjectState(kc Context, kind, namespace, name string) (ObjectState, error) {
	var out ObjectState
	err := w.withClient(kc, func(c *clusterClient) error {
		ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
		defer cancel()

		got, _, err := c.get(ctx, kind, namespace, name)
		if err != nil {
			return err
		}
		out = stateOf(got)
		return nil
	})
	return out, err
}

// Delete removes one object.
//
// No propagation policy is set, which leaves the API server's own default in
// place -- the same thing `kubectl delete` does, so a Deployment deleted here
// takes its ReplicaSets and Pods with it exactly as it would from the command
// line.
func (w *Watcher) Delete(kc Context, kind, namespace, name string) error {
	return w.withClient(kc, func(c *clusterClient) error {
		ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
		defer cancel()

		mapping, err := c.mappingForKind(kind)
		if err != nil {
			return err
		}
		return resourceFor(c.dynamic, mapping, namespace).Delete(ctx, name, metav1.DeleteOptions{})
	})
}

// Scale sets a workload's replica count through the scale subresource.
//
// The subresource rather than a patch of spec.replicas, because it is the one
// interface every scalable kind shares: a Deployment, a StatefulSet and a
// custom resource with a scale subresource are all set the same way, and a kind
// that has no scale is refused by the API server rather than silently patched.
func (w *Watcher) Scale(kc Context, kind, namespace, name string, replicas int32) error {
	if replicas < 0 {
		return fmt.Errorf("a replica count may not be negative")
	}
	return w.withClient(kc, func(c *clusterClient) error {
		ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
		defer cancel()

		mapping, err := c.mappingForKind(kind)
		if err != nil {
			return err
		}
		client := resourceFor(c.dynamic, mapping, namespace)

		scale, err := client.Get(ctx, name, metav1.GetOptions{}, "scale")
		if err != nil {
			return err
		}
		if err := unstructured.SetNestedField(scale.Object, int64(replicas), "spec", "replicas"); err != nil {
			return err
		}
		_, err = client.Update(ctx, scale, metav1.UpdateOptions{}, "scale")
		return err
	})
}

// Restart rolls a workload by stamping its pod template, the way
// `kubectl rollout restart` does.
func (w *Watcher) Restart(kc Context, kind, namespace, name string) error {
	return w.withClient(kc, func(c *clusterClient) error {
		ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
		defer cancel()

		mapping, err := c.mappingForKind(kind)
		if err != nil {
			return err
		}
		_, err = resourceFor(c.dynamic, mapping, namespace).Patch(
			ctx, name, types.MergePatchType, restartPatch(time.Now()), metav1.PatchOptions{},
		)
		return err
	})
}

// Cordon closes a node to new work, or reopens it.
func (w *Watcher) Cordon(kc Context, name string, on bool) error {
	return w.withClient(kc, func(c *clusterClient) error {
		ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
		defer cancel()
		return c.cordon(ctx, name, on)
	})
}

// cordon is the patch itself, split out because a drain starts with one.
func (c *clusterClient) cordon(ctx context.Context, name string, on bool) error {
	mapping, err := c.mappingForKind(KindNodes)
	if err != nil {
		return err
	}
	_, err = resourceFor(c.dynamic, mapping, "").Patch(
		ctx, name, types.MergePatchType, cordonPatch(on), metav1.PatchOptions{},
	)
	return err
}
