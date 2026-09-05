// Resource accounting: what a cluster, a node or a namespace has, what has been
// promised out of it, and what is actually being used.
//
// The five numbers are deliberately kept apart because they answer different
// questions. Capacity and allocatable come from the nodes and say what exists.
// Requests and limits come from the pods and say what the scheduler has booked
// and what those pods are allowed to grow into. Usage comes from a metrics
// source and says what is really happening. A cluster can be full by requests
// and idle by usage at the same time, and that gap is the whole point of
// showing them together.
//
// All of it is one pure function over objects somebody else listed, so the
// rules that are easy to get wrong -- init containers, pod overhead, pods that
// have finished -- live in exactly one place and are testable without a
// cluster.
package kube

import "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

// Scope kinds. A budget is always for one of these three.
const (
	ScopeCluster   = "cluster"
	ScopeNode      = "node"
	ScopeNamespace = "namespace"
)

// Scope says which slice of the cluster a budget covers.
type Scope struct {
	Kind string `json:"kind"`
	// Name is the node or namespace. Empty for a whole cluster.
	Name string `json:"name"`
}

// Amount is one resource dimension -- CPU, memory, pods -- with every number
// known about it.
type Amount struct {
	Label string `json:"label"`
	Unit  string `json:"unit"`

	Capacity    float64 `json:"capacity"`
	Allocatable float64 `json:"allocatable"`
	Requested   float64 `json:"requested"`
	Limits      float64 `json:"limits"`
	Used        float64 `json:"used"`

	// The three flags below separate "zero" from "not a number here". Without
	// them a namespace with no quota renders as a full bar, and a cluster with
	// no metrics source renders as one using nothing at all -- both of which
	// are worse than saying nothing.

	// HasCapacity is whether anything bounds this amount from above. False for
	// a namespace with no ResourceQuota, which owns no hardware.
	HasCapacity bool `json:"hasCapacity"`
	// HasDemand is whether requests and limits mean anything here. False for
	// pods, which are counted rather than asked for.
	HasDemand bool `json:"hasDemand"`
	// HasUsed is whether Used was actually measured.
	HasUsed bool `json:"hasUsed"`
}

// Metrics sources, in the order they are tried. Neither is guaranteed to be
// installed, and a cluster with neither is a normal cluster -- everything
// except the used column still works.
const (
	SourceMetricsServer = "metrics-server"
	SourcePrometheus    = "prometheus"
)

// Measured is one reading: cores and GiB, matching the units the amounts use.
type Measured struct {
	CPU    float64 `json:"cpu"`
	Memory float64 `json:"memory"`
}

// Usage is what a metrics source measured, and why there is nothing if there is
// nothing.
type Usage struct {
	// Source names who answered, so the UI can say where the number came from
	// and the two sources are never silently mixed. Empty when nobody did.
	Source string `json:"source"`
	// Error is why there is no reading: not installed, or installed and
	// unhappy. The difference matters to whoever has to fix it.
	Error string `json:"error"`

	// Nodes is keyed by node name, Pods by "namespace/name".
	Nodes map[string]Measured `json:"-"`
	Pods  map[string]Measured `json:"-"`
}

// Budget is one scope's worth of accounting.
type Budget struct {
	Scope   Scope    `json:"scope"`
	Amounts []Amount `json:"amounts"`
	Usage   Usage    `json:"usage"`
	// Error is why there is no budget at all, as opposed to Usage.Error, which
	// is why one column of it is missing.
	Error string `json:"error"`
}

// Inventory is the raw material a budget is built from: whatever the caller
// listed. Nodes and pods are the whole cluster's, not the scope's -- Rollup
// does the filtering, so every surface passes the same lists and cannot
// disagree about what "in scope" means.
type Inventory struct {
	Nodes []unstructured.Unstructured
	Pods  []unstructured.Unstructured
	// Quotas are the ResourceQuotas in the namespace being scoped. Ignored for
	// the other scopes, which are bounded by hardware instead.
	Quotas []unstructured.Unstructured
}

// Rollup totals an inventory into one scope's budget.
func Rollup(scope Scope, inv Inventory, usage Usage) Budget {
	nodes, pods := inv.Nodes, inv.Pods
	cpu := Amount{Label: "CPU", Unit: "cores", HasDemand: true}
	mem := Amount{Label: "Memory", Unit: "GiB", HasDemand: true}
	pod := Amount{Label: "Pods", HasUsed: true}

	// A namespace holds no hardware, so nodes contribute nothing to it. Its
	// ceiling, if it has one at all, is a ResourceQuota.
	if scope.Kind != ScopeNamespace {
		counted := 0
		for i := range nodes {
			n := &nodes[i]
			if scope.Kind == ScopeNode && n.GetName() != scope.Name {
				continue
			}
			counted++
			cpu.Capacity += parseCPU(nestedString(n, "status", "capacity", "cpu"))
			cpu.Allocatable += parseCPU(nestedString(n, "status", "allocatable", "cpu"))
			mem.Capacity += parseMemory(nestedString(n, "status", "capacity", "memory"))
			mem.Allocatable += parseMemory(nestedString(n, "status", "allocatable", "memory"))
			pod.Capacity += parseCount(nestedString(n, "status", "capacity", "pods"))
			pod.Allocatable += parseCount(nestedString(n, "status", "allocatable", "pods"))
		}
		if counted > 0 {
			cpu.HasCapacity, mem.HasCapacity, pod.HasCapacity = true, true, true
		}
	}

	// A namespace's ceiling, when it has one, is whatever ResourceQuota the
	// admins put on it.
	if scope.Kind == ScopeNamespace {
		applyQuota(&cpu, inv.Quotas, parseCPU, "requests.cpu", "cpu")
		applyQuota(&mem, inv.Quotas, parseMemory, "requests.memory", "memory")
		applyQuota(&pod, inv.Quotas, parseCount, "pods")
	}

	for i := range pods {
		p := &pods[i]
		if terminal(p) || !inScope(p, scope) {
			continue
		}
		pod.Used++
		cpu.Requested += podResource(p, "requests", "cpu", parseCPU)
		mem.Requested += podResource(p, "requests", "memory", parseMemory)
		cpu.Limits += podResource(p, "limits", "cpu", parseCPU)
		mem.Limits += podResource(p, "limits", "memory", parseMemory)
	}

	if measured, ok := usage.measure(scope, pods); ok {
		cpu.Used, cpu.HasUsed = measured.CPU, true
		mem.Used, mem.HasUsed = measured.Memory, true
	}

	return Budget{Scope: scope, Amounts: []Amount{cpu, mem, pod}, Usage: usage}
}

// measure totals the readings that belong to one scope.
//
// Node readings are preferred for a whole cluster because they include what the
// kubelet and the container runtime themselves burn, which no sum of pods will
// ever show. A namespace has no node reading by definition, so there it is the
// pods or nothing.
func (u Usage) measure(scope Scope, pods []unstructured.Unstructured) (Measured, bool) {
	var out Measured
	found := false

	switch scope.Kind {
	case ScopeNode:
		m, ok := u.Nodes[scope.Name]
		return m, ok

	case ScopeCluster:
		for _, m := range u.Nodes {
			out.CPU += m.CPU
			out.Memory += m.Memory
			found = true
		}
		if found {
			return out, true
		}
	}

	// Cluster with no node readings, or a namespace: add up the pods that are
	// in scope.
	for i := range pods {
		p := &pods[i]
		if terminal(p) || !inScope(p, scope) {
			continue
		}
		m, ok := u.Pods[p.GetNamespace()+"/"+p.GetName()]
		if !ok {
			continue
		}
		out.CPU += m.CPU
		out.Memory += m.Memory
		found = true
	}
	return out, found
}

// applyQuota narrows an amount to the tightest ceiling any of the namespace's
// ResourceQuotas puts on it.
//
// Every quota in a namespace applies at once, so the binding one is the
// smallest -- that is the wall a deployment actually hits. `keys` are the
// spellings of the same limit in preference order: `requests.cpu` and the older
// bare `cpu` mean the same thing, and a quota naming neither leaves the amount
// unbounded rather than capped at zero.
func applyQuota(a *Amount, quotas []unstructured.Unstructured, parse func(string) float64, keys ...string) {
	for i := range quotas {
		hard, found, _ := unstructured.NestedStringMap(quotas[i].Object, "status", "hard")
		if !found {
			// The quota controller writes status.hard. Until it has, the
			// ceiling is still knowable from what was asked for.
			hard, _, _ = unstructured.NestedStringMap(quotas[i].Object, "spec", "hard")
		}
		for _, key := range keys {
			raw, ok := hard[key]
			if !ok {
				continue
			}
			if value := parse(raw); !a.HasCapacity || value < a.Allocatable {
				a.Allocatable, a.HasCapacity = value, true
			}
			break
		}
	}
}

// inScope reports whether one pod belongs to the slice being totalled.
func inScope(pod *unstructured.Unstructured, scope Scope) bool {
	switch scope.Kind {
	case ScopeNode:
		return nestedString(pod, "spec", "nodeName") == scope.Name
	case ScopeNamespace:
		return pod.GetNamespace() == scope.Name
	default:
		return true
	}
}

// parseCount reads a plain count -- a node's pod capacity -- which is a
// quantity like any other, just not one of anything with a unit.
func parseCount(s string) float64 { return parseCPU(s) }

// terminal reports whether a pod has finished and so holds nothing on a node.
//
// The scheduler stops accounting for a pod once it reaches Succeeded or Failed,
// and so must we: a cluster that has run a lot of Jobs is mostly finished pods,
// and counting their requests would report it far fuller than it is.
func terminal(pod *unstructured.Unstructured) bool {
	switch nestedString(pod, "status", "phase") {
	case "Succeeded", "Failed":
		return true
	}
	return false
}

// podResource is what one pod holds of a single resource, by the rule the
// scheduler itself uses:
//
//	max( largest init container, main containers + sidecars )
//
// Init containers run one at a time and finish before the main containers
// start, so they overlap rather than add -- a pod whose init asks for 3 cores
// and whose app asks for 1 needs 3, not 4. A sidecar is the exception: an init
// container with restartPolicy Always keeps running alongside the app, so its
// request is held for the pod's whole life and belongs in the running total.
func podResource(pod *unstructured.Unstructured, kind, name string, parse func(string) float64) float64 {
	var running, initPeak float64

	for _, raw := range nestedSlice(pod, "spec", "containers") {
		running += containerValue(raw, kind, name, parse)
	}
	for _, raw := range nestedSlice(pod, "spec", "initContainers") {
		value := containerValue(raw, kind, name, parse)
		if mapString(asMap(raw), "restartPolicy") == "Always" {
			running += value
			continue
		}
		initPeak = max(initPeak, value)
	}

	// Overhead is what the pod's RuntimeClass charges for the sandbox itself,
	// on top of anything the containers asked for. Empty for the ordinary
	// runtimes; real for kata and gVisor, where it is large enough to matter.
	overheads, _, _ := unstructured.NestedStringMap(pod.Object, "spec", "overhead")
	overhead := parse(overheads[name])

	return max(running, initPeak) + overhead
}

// containerValue reads one resource quantity off one container.
func containerValue(raw any, kind, name string, parse func(string) float64) float64 {
	values := asMap(asMap(asMap(raw)["resources"])[kind])
	return parse(mapString(values, name))
}
