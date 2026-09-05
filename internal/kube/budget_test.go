package kube

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// podSpec builds a pod with one container's requests and limits, which is what
// most of these tests need and all of them start from.
func podSpec(namespace, name, node, phase string, requests, limits map[string]any) unstructured.Unstructured {
	container := map[string]any{"name": "app", "resources": map[string]any{}}
	resources := asMap(container["resources"])
	if requests != nil {
		resources["requests"] = requests
	}
	if limits != nil {
		resources["limits"] = limits
	}
	return *obj(map[string]any{
		"metadata": map[string]any{"name": name, "namespace": namespace},
		"spec":     map[string]any{"nodeName": node, "containers": []any{container}},
		"status":   map[string]any{"phase": phase},
	})
}

// amountFor picks one dimension out of a budget, failing if it is missing.
func amountFor(t *testing.T, b Budget, label string) Amount {
	t.Helper()
	for _, a := range b.Amounts {
		if a.Label == label {
			return a
		}
	}
	t.Fatalf("budget has no %q amount, got %+v", label, b.Amounts)
	return Amount{}
}

func TestClusterBudgetSumsRequestsAndLimitsAcrossPods(t *testing.T) {
	pods := []unstructured.Unstructured{
		podSpec("prod", "api-0", "worker-1", "Running",
			map[string]any{"cpu": "500m", "memory": "1Gi"},
			map[string]any{"cpu": "2", "memory": "2Gi"}),
		podSpec("prod", "api-1", "worker-2", "Running",
			map[string]any{"cpu": "250m", "memory": "512Mi"},
			map[string]any{"cpu": "1", "memory": "1Gi"}),
	}

	b := Rollup(Scope{Kind: ScopeCluster}, Inventory{Pods: pods}, Usage{})

	cpu := amountFor(t, b, "CPU")
	if cpu.Requested != 0.75 {
		t.Errorf("CPU requested = %v, want 0.75", cpu.Requested)
	}
	if cpu.Limits != 3 {
		t.Errorf("CPU limits = %v, want 3", cpu.Limits)
	}

	mem := amountFor(t, b, "Memory")
	if mem.Requested != 1.5 {
		t.Errorf("memory requested = %v, want 1.5", mem.Requested)
	}
	if mem.Limits != 3 {
		t.Errorf("memory limits = %v, want 3", mem.Limits)
	}
}

func TestBudgetIgnoresPodsThatHaveFinished(t *testing.T) {
	// A cluster that has run a lot of Jobs is mostly Succeeded pods. They hold
	// no reservation on any node, so counting them would report a cluster far
	// fuller than it is.
	pods := []unstructured.Unstructured{
		podSpec("prod", "api-0", "worker-1", "Running", map[string]any{"cpu": "1"}, nil),
		podSpec("prod", "backup-1", "worker-1", "Succeeded", map[string]any{"cpu": "4"}, nil),
		podSpec("prod", "backup-2", "worker-1", "Failed", map[string]any{"cpu": "4"}, nil),
		podSpec("prod", "queued", "", "Pending", map[string]any{"cpu": "2"}, nil),
	}

	b := Rollup(Scope{Kind: ScopeCluster}, Inventory{Pods: pods}, Usage{})

	// Pending counts: it is waiting for room and the scheduler is holding it
	// against the cluster. Succeeded and Failed do not.
	if got := amountFor(t, b, "CPU").Requested; got != 3 {
		t.Errorf("CPU requested = %v, want 3", got)
	}
}

// podWithInit builds a pod whose init containers each ask for one CPU quantity,
// optionally as a sidecar (restartPolicy Always).
func podWithInit(main string, sidecar bool, init ...string) unstructured.Unstructured {
	p := podSpec("prod", "job-0", "worker-1", "Running", map[string]any{"cpu": main}, nil)
	var containers []any
	for _, cpu := range init {
		c := map[string]any{
			"name":      "init",
			"resources": map[string]any{"requests": map[string]any{"cpu": cpu}},
		}
		if sidecar {
			c["restartPolicy"] = "Always"
		}
		containers = append(containers, c)
	}
	_ = unstructured.SetNestedSlice(p.Object, containers, "spec", "initContainers")
	return p
}

func TestInitContainersCountAsTheLargestNotTheSum(t *testing.T) {
	// Init containers run one at a time and are gone before the main ones
	// start, so the pod's floor is the biggest of them -- not their total.
	pod := podWithInit("1", false, "2", "3")

	b := Rollup(Scope{Kind: ScopeCluster}, Inventory{Pods: []unstructured.Unstructured{pod}}, Usage{})

	if got := amountFor(t, b, "CPU").Requested; got != 3 {
		t.Errorf("CPU requested = %v, want 3 (max init, not 1+2+3 and not 1)", got)
	}
}

func TestSidecarInitContainersAddToTheRunningTotal(t *testing.T) {
	// An init container with restartPolicy Always is a sidecar: it keeps
	// running alongside the main containers, so its request is held for the
	// pod's whole life and adds on top rather than overlapping.
	pod := podWithInit("1", true, "2")

	b := Rollup(Scope{Kind: ScopeCluster}, Inventory{Pods: []unstructured.Unstructured{pod}}, Usage{})

	if got := amountFor(t, b, "CPU").Requested; got != 3 {
		t.Errorf("CPU requested = %v, want 3 (1 main + 2 sidecar)", got)
	}
}

func TestPodOverheadIsHeldOnTopOfTheContainers(t *testing.T) {
	// A RuntimeClass with overhead (kata, gVisor) charges the pod for its
	// sandbox as well as its containers, and the scheduler reserves it.
	pod := podSpec("prod", "api-0", "worker-1", "Running", map[string]any{"cpu": "1", "memory": "1Gi"}, nil)
	_ = unstructured.SetNestedStringMap(pod.Object,
		map[string]string{"cpu": "250m", "memory": "128Mi"}, "spec", "overhead")

	b := Rollup(Scope{Kind: ScopeCluster}, Inventory{Pods: []unstructured.Unstructured{pod}}, Usage{})

	if got := amountFor(t, b, "CPU").Requested; got != 1.25 {
		t.Errorf("CPU requested = %v, want 1.25", got)
	}
	if got := amountFor(t, b, "Memory").Requested; got != 1.125 {
		t.Errorf("memory requested = %v, want 1.125", got)
	}
}

// node builds a node with capacity and allocatable for the three dimensions.
func node(name string, capCPU, allocCPU, capMem, allocMem, capPods, allocPods string) unstructured.Unstructured {
	return *obj(map[string]any{
		"metadata": map[string]any{"name": name},
		"status": map[string]any{
			"capacity":    map[string]any{"cpu": capCPU, "memory": capMem, "pods": capPods},
			"allocatable": map[string]any{"cpu": allocCPU, "memory": allocMem, "pods": allocPods},
		},
	})
}

func TestClusterBudgetSumsNodeCapacityAndAllocatable(t *testing.T) {
	// Allocatable is what the scheduler may hand out; capacity is the hardware.
	// The gap is what the kubelet keeps for the system, and seeing it is half
	// the reason for showing both.
	nodes := []unstructured.Unstructured{
		node("worker-1", "8", "7800m", "32Gi", "30Gi", "110", "110"),
		node("worker-2", "4", "3800m", "16Gi", "15Gi", "110", "110"),
	}

	b := Rollup(Scope{Kind: ScopeCluster}, Inventory{Nodes: nodes}, Usage{})

	cpu := amountFor(t, b, "CPU")
	if cpu.Capacity != 12 {
		t.Errorf("CPU capacity = %v, want 12", cpu.Capacity)
	}
	if cpu.Allocatable != 11.6 {
		t.Errorf("CPU allocatable = %v, want 11.6", cpu.Allocatable)
	}

	mem := amountFor(t, b, "Memory")
	if mem.Capacity != 48 {
		t.Errorf("memory capacity = %v, want 48", mem.Capacity)
	}
	if mem.Allocatable != 45 {
		t.Errorf("memory allocatable = %v, want 45", mem.Allocatable)
	}

	pods := amountFor(t, b, "Pods")
	if pods.Allocatable != 220 {
		t.Errorf("pods allocatable = %v, want 220", pods.Allocatable)
	}
}

func TestNodeScopeCountsOnlyThePodsPlacedOnThatNode(t *testing.T) {
	nodes := []unstructured.Unstructured{
		node("worker-1", "8", "7800m", "32Gi", "30Gi", "110", "110"),
		node("worker-2", "4", "3800m", "16Gi", "15Gi", "110", "110"),
	}
	pods := []unstructured.Unstructured{
		podSpec("prod", "api-0", "worker-1", "Running", map[string]any{"cpu": "1"}, nil),
		podSpec("prod", "api-1", "worker-1", "Running", map[string]any{"cpu": "2"}, nil),
		podSpec("prod", "api-2", "worker-2", "Running", map[string]any{"cpu": "4"}, nil),
	}

	b := Rollup(Scope{Kind: ScopeNode, Name: "worker-1"}, Inventory{Nodes: nodes, Pods: pods}, Usage{})

	cpu := amountFor(t, b, "CPU")
	if cpu.Requested != 3 {
		t.Errorf("CPU requested = %v, want 3 (worker-2's pod excluded)", cpu.Requested)
	}
	if cpu.Capacity != 8 {
		t.Errorf("CPU capacity = %v, want 8 (this node only)", cpu.Capacity)
	}
	if got := amountFor(t, b, "Pods").Used; got != 2 {
		t.Errorf("pods used = %v, want 2", got)
	}
}

func TestNamespaceScopeHasDemandButNoCapacity(t *testing.T) {
	// A namespace does not own hardware. Asking how full it is only means
	// anything against a ResourceQuota, and most namespaces have none -- so the
	// capacity numbers must read as absent rather than as zero, or every
	// namespace would render as 100% full.
	nodes := []unstructured.Unstructured{node("worker-1", "8", "7800m", "32Gi", "30Gi", "110", "110")}
	pods := []unstructured.Unstructured{
		podSpec("prod", "api-0", "worker-1", "Running", map[string]any{"cpu": "1"}, nil),
		podSpec("staging", "api-0", "worker-1", "Running", map[string]any{"cpu": "4"}, nil),
	}

	b := Rollup(Scope{Kind: ScopeNamespace, Name: "prod"}, Inventory{Nodes: nodes, Pods: pods}, Usage{})

	cpu := amountFor(t, b, "CPU")
	if cpu.Requested != 1 {
		t.Errorf("CPU requested = %v, want 1 (staging excluded)", cpu.Requested)
	}
	if cpu.HasCapacity {
		t.Error("namespace CPU reports a capacity; it has none without a quota")
	}
	if cpu.Capacity != 0 {
		t.Errorf("CPU capacity = %v, want 0", cpu.Capacity)
	}
}

func TestPodsDimensionCarriesNoRequestsOrLimits(t *testing.T) {
	// Pods are counted, not requested. A requests bar under "Pods" would be
	// drawing a number that does not exist.
	nodes := []unstructured.Unstructured{node("worker-1", "8", "7800m", "32Gi", "30Gi", "110", "110")}

	b := Rollup(Scope{Kind: ScopeCluster}, Inventory{Nodes: nodes}, Usage{})

	pods := amountFor(t, b, "Pods")
	if pods.HasDemand {
		t.Error("pods dimension claims requests and limits; it has neither")
	}
	if cpu := amountFor(t, b, "CPU"); !cpu.HasDemand {
		t.Error("CPU dimension should carry requests and limits")
	}
	if !pods.HasCapacity {
		t.Error("pods dimension should carry a capacity at cluster scope")
	}
}

func TestNodeUsageFillsTheUsedColumn(t *testing.T) {
	nodes := []unstructured.Unstructured{
		node("worker-1", "8", "7800m", "32Gi", "30Gi", "110", "110"),
		node("worker-2", "4", "3800m", "16Gi", "15Gi", "110", "110"),
	}
	usage := Usage{
		Source: SourceMetricsServer,
		Nodes: map[string]Measured{
			"worker-1": {CPU: 2.5, Memory: 8},
			"worker-2": {CPU: 0.5, Memory: 2},
		},
	}

	cluster := Rollup(Scope{Kind: ScopeCluster}, Inventory{Nodes: nodes}, usage)
	cpu := amountFor(t, cluster, "CPU")
	if !cpu.HasUsed {
		t.Fatal("cluster CPU has no usage though the nodes were measured")
	}
	if cpu.Used != 3 {
		t.Errorf("cluster CPU used = %v, want 3", cpu.Used)
	}

	one := Rollup(Scope{Kind: ScopeNode, Name: "worker-1"}, Inventory{Nodes: nodes}, usage)
	if got := amountFor(t, one, "Memory").Used; got != 8 {
		t.Errorf("worker-1 memory used = %v, want 8", got)
	}
}

func TestNamespaceUsageComesFromItsOwnPods(t *testing.T) {
	// There is no such thing as a node reading for a namespace, so the pods
	// have to be added up instead.
	pods := []unstructured.Unstructured{
		podSpec("prod", "api-0", "worker-1", "Running", map[string]any{"cpu": "1"}, nil),
		podSpec("staging", "api-0", "worker-1", "Running", map[string]any{"cpu": "1"}, nil),
	}
	usage := Usage{
		Source: SourceMetricsServer,
		Pods: map[string]Measured{
			"prod/api-0":    {CPU: 0.4, Memory: 1},
			"staging/api-0": {CPU: 2, Memory: 9},
		},
	}

	b := Rollup(Scope{Kind: ScopeNamespace, Name: "prod"}, Inventory{Pods: pods}, usage)

	cpu := amountFor(t, b, "CPU")
	if !cpu.HasUsed {
		t.Fatal("namespace CPU has no usage though its pod was measured")
	}
	if cpu.Used != 0.4 {
		t.Errorf("prod CPU used = %v, want 0.4 (staging excluded)", cpu.Used)
	}
}

func TestWithoutAMetricsSourceUsageIsAbsentNotZero(t *testing.T) {
	// Neither metrics-server nor Prometheus is installed. Everything else still
	// works; the used column just has nothing to say, and must say so rather
	// than draw a cluster that is using none of itself.
	nodes := []unstructured.Unstructured{node("worker-1", "8", "7800m", "32Gi", "30Gi", "110", "110")}
	pods := []unstructured.Unstructured{
		podSpec("prod", "api-0", "worker-1", "Running", map[string]any{"cpu": "1"}, nil),
	}

	b := Rollup(Scope{Kind: ScopeCluster}, Inventory{Nodes: nodes, Pods: pods},
		Usage{Error: "no metrics-server and no Prometheus"})

	cpu := amountFor(t, b, "CPU")
	if cpu.HasUsed {
		t.Error("CPU claims a usage reading with no metrics source")
	}
	if cpu.Requested != 1 {
		t.Errorf("CPU requested = %v, want 1 (requests work without metrics)", cpu.Requested)
	}
	if b.Usage.Error == "" {
		t.Error("budget does not carry why there is no usage")
	}
	// The pod count is the API server's own, so it survives.
	if got := amountFor(t, b, "Pods"); !got.HasUsed || got.Used != 1 {
		t.Errorf("pods used = %v (has=%v), want 1/true", got.Used, got.HasUsed)
	}
}

// quota builds a ResourceQuota with the hard limits given.
func quota(name string, hard map[string]any) unstructured.Unstructured {
	return *obj(map[string]any{
		"metadata": map[string]any{"name": name, "namespace": "prod"},
		"status":   map[string]any{"hard": hard},
	})
}

func TestAResourceQuotaGivesANamespaceItsCeiling(t *testing.T) {
	pods := []unstructured.Unstructured{
		podSpec("prod", "api-0", "worker-1", "Running", map[string]any{"cpu": "1"}, nil),
	}
	quotas := []unstructured.Unstructured{quota("team", map[string]any{
		"requests.cpu":    "10",
		"limits.cpu":      "20",
		"requests.memory": "20Gi",
		"pods":            "50",
	})}

	b := Rollup(Scope{Kind: ScopeNamespace, Name: "prod"},
		Inventory{Pods: pods, Quotas: quotas}, Usage{})

	cpu := amountFor(t, b, "CPU")
	if !cpu.HasCapacity {
		t.Fatal("quota'd namespace reports no ceiling")
	}
	// Allocatable is the number requests are measured against, whether it comes
	// from hardware or from a quota, so one bar renders both.
	if cpu.Allocatable != 10 {
		t.Errorf("CPU ceiling = %v, want 10", cpu.Allocatable)
	}
	if cpu.Capacity != 0 {
		t.Errorf("CPU capacity = %v, want 0 -- a namespace owns no hardware", cpu.Capacity)
	}
	if got := amountFor(t, b, "Memory").Allocatable; got != 20 {
		t.Errorf("memory ceiling = %v, want 20", got)
	}
	if got := amountFor(t, b, "Pods").Allocatable; got != 50 {
		t.Errorf("pods ceiling = %v, want 50", got)
	}
}

func TestTheTightestQuotaWins(t *testing.T) {
	// Several quotas in one namespace all apply, so the binding one is the
	// smallest -- that is the wall a deployment actually hits.
	quotas := []unstructured.Unstructured{
		quota("team", map[string]any{"requests.cpu": "10"}),
		// The bare "cpu" key is the older spelling of requests.cpu.
		quota("platform", map[string]any{"cpu": "4"}),
	}

	b := Rollup(Scope{Kind: ScopeNamespace, Name: "prod"},
		Inventory{Quotas: quotas}, Usage{})

	if got := amountFor(t, b, "CPU").Allocatable; got != 4 {
		t.Errorf("CPU ceiling = %v, want 4 (the tighter of 10 and 4)", got)
	}
}

func TestANamespaceQuotaThatSaysNothingAboutCPULeavesItUnbounded(t *testing.T) {
	// A quota that only caps pods must not make CPU look capped at zero.
	quotas := []unstructured.Unstructured{quota("team", map[string]any{"pods": "50"})}

	b := Rollup(Scope{Kind: ScopeNamespace, Name: "prod"},
		Inventory{Quotas: quotas}, Usage{})

	if cpu := amountFor(t, b, "CPU"); cpu.HasCapacity {
		t.Error("CPU claims a ceiling from a quota that never mentions it")
	}
	if pods := amountFor(t, b, "Pods"); !pods.HasCapacity || pods.Allocatable != 50 {
		t.Errorf("pods ceiling = %v (has=%v), want 50/true", pods.Allocatable, pods.HasCapacity)
	}
}

func TestAFreshQuotaIsReadFromItsSpecUntilTheControllerFillsInStatus(t *testing.T) {
	// status.hard is written by the quota controller. Between creating a quota
	// and that happening -- or if the controller is wedged -- the ceiling is
	// still knowable from the spec, and showing it beats showing none.
	q := *obj(map[string]any{
		"metadata": map[string]any{"name": "team", "namespace": "prod"},
		"spec":     map[string]any{"hard": map[string]any{"requests.cpu": "6"}},
	})

	b := Rollup(Scope{Kind: ScopeNamespace, Name: "prod"},
		Inventory{Quotas: []unstructured.Unstructured{q}}, Usage{})

	if got := amountFor(t, b, "CPU"); !got.HasCapacity || got.Allocatable != 6 {
		t.Errorf("CPU ceiling = %v (has=%v), want 6/true", got.Allocatable, got.HasCapacity)
	}
}

func TestTheNodesTableShowsAllocatableBesideCapacity(t *testing.T) {
	// Capacity alone reads as room the scheduler does not actually have: the
	// kubelet's reservation is already gone from allocatable, and the gap is
	// invisible until both are on screen.
	n := node("worker-1", "8", "7800m", "32Gi", "30Gi", "110", "110")

	table := buildLiveTable(KindNodes, builtinColumns[KindNodes], false, []*unstructured.Unstructured{&n})
	cells := cellsByHeader(t, table, 0)

	if cells["CPU"] != "8" {
		t.Errorf("CPU = %q, want the capacity 8", cells["CPU"])
	}
	if cells["CPU alloc"] != "7800m" {
		t.Errorf("CPU alloc = %q, want 7800m", cells["CPU alloc"])
	}
	if cells["Memory"] != "32Gi" {
		t.Errorf("Memory = %q, want 32Gi", cells["Memory"])
	}
	if cells["Mem alloc"] != "30Gi" {
		t.Errorf("Mem alloc = %q, want 30Gi", cells["Mem alloc"])
	}
}
