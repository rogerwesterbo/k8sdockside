// Reading a budget off a live cluster. The accounting itself is in budget.go
// and stays a pure function of what is listed here.
package kube

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// Budget reads one scope's resource accounting from a cluster.
//
// `fallback` is asked for usage when metrics-server is not installed; nil means
// there is nothing to fall back to. A cluster with no metrics source at all
// still returns a full budget -- capacity, allocatable, requests and limits all
// come from the API server -- with only the used column absent and the reason
// carried in Usage.
func (w *Watcher) Budget(kc Context, scope Scope, fallback UsageFallback) (Budget, error) {
	out := Budget{Scope: scope, Amounts: []Amount{}}

	err := w.withClient(kc, func(c *clusterClient) error {
		ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
		defer cancel()

		inv, err := c.inventory(ctx, scope)
		if err != nil {
			return err
		}
		out = Rollup(scope, inv, c.usage(ctx, scope, fallback))
		return nil
	})

	if err != nil {
		out.Scope, out.Error = scope, err.Error()
	}
	return out, err
}

// inventory lists what one scope's budget is built from.
func (c *clusterClient) inventory(ctx context.Context, scope Scope) (Inventory, error) {
	var inv Inventory

	// A namespace owns no hardware, so its nodes are not worth a list call.
	if scope.Kind != ScopeNamespace {
		nodes, _, err := c.list(ctx, KindNodes, metav1.ListOptions{})
		if err != nil {
			return inv, err
		}
		inv.Nodes = nodes
	}

	pods, err := c.podsInScope(ctx, scope)
	if err != nil {
		return inv, err
	}
	inv.Pods = pods

	if scope.Kind == ScopeNamespace {
		// Most namespaces have no quota, and a cluster that will not list them
		// should still show what its pods asked for -- so this failing costs
		// the ceiling and nothing else.
		if quotas, err := c.listIn(ctx, KindResourceQuotas, scope.Name, metav1.ListOptions{}); err == nil {
			inv.Quotas = quotas
		}
	}
	return inv, nil
}

// podsInScope lists the pods a budget covers, narrowed by the API server
// wherever it can be -- on a large cluster that is the difference between one
// node's pods and every pod there is.
func (c *clusterClient) podsInScope(ctx context.Context, scope Scope) ([]unstructured.Unstructured, error) {
	switch scope.Kind {
	case ScopeNode:
		return c.listIn(ctx, KindPods, "", metav1.ListOptions{
			FieldSelector: "spec.nodeName=" + scope.Name,
		})
	case ScopeNamespace:
		return c.listIn(ctx, KindPods, scope.Name, metav1.ListOptions{})
	default:
		return c.listIn(ctx, KindPods, "", metav1.ListOptions{})
	}
}

// listIn is list with a namespace, for the calls that can be narrowed to one.
func (c *clusterClient) listIn(ctx context.Context, kind, namespace string, opts metav1.ListOptions) ([]unstructured.Unstructured, error) {
	mapping, err := c.mappingForKind(kind)
	if err != nil {
		return nil, err
	}
	l, err := resourceFor(c.dynamic, mapping, namespace).List(ctx, opts)
	if err != nil {
		return nil, err
	}
	return l.Items, nil
}
