package kube

import (
	"strings"
	"testing"
	"time"
)

// page is a small stand-in for a kubelet's cAdvisor output: the four counters
// that matter, on a node running one pod of two containers, plus the noise
// every real page is mostly made of.
const page = `
# HELP container_cpu_cfs_periods_total Number of elapsed enforcement period intervals.
# TYPE container_cpu_cfs_periods_total counter
container_cpu_cfs_periods_total{container="app",id="/kubepods/pod-1/app",image="nginx",name="k8s_app_web-1",namespace="prod",pod="web-1"} 1000
container_cpu_cfs_throttled_periods_total{container="app",id="/kubepods/pod-1/app",namespace="prod",pod="web-1"} 100
container_cpu_cfs_throttled_seconds_total{container="app",id="/kubepods/pod-1/app",namespace="prod",pod="web-1"} 20
container_cpu_cfs_periods_total{container="sidecar",id="/kubepods/pod-1/sidecar",namespace="prod",pod="web-1"} 500
container_cpu_cfs_throttled_periods_total{container="sidecar",id="/kubepods/pod-1/sidecar",namespace="prod",pod="web-1"} 0
container_cpu_cfs_throttled_seconds_total{container="sidecar",id="/kubepods/pod-1/sidecar",namespace="prod",pod="web-1"} 0
container_cpu_cfs_periods_total{container="app",id="/kubepods/pod-2/app",namespace="staging",pod="db-0"} 200
container_cpu_cfs_throttled_periods_total{container="app",id="/kubepods/pod-2/app",namespace="staging",pod="db-0"} 20
container_cpu_cfs_throttled_seconds_total{container="app",id="/kubepods/pod-2/app",namespace="staging",pod="db-0"} 4
container_pressure_cpu_waiting_seconds_total{id="/"} 60
container_pressure_cpu_waiting_seconds_total{container="app",id="/kubepods/pod-1/app",namespace="prod",pod="web-1"} 30
container_memory_working_set_bytes{container="app",id="/kubepods/pod-1/app",namespace="prod",pod="web-1"} 12345
container_cpu_usage_seconds_total{container="app",id="/kubepods/pod-1/app",namespace="prod",pod="web-1"} 900
`

// later is the same node a minute on: the app container has spent a further 100
// periods, 50 of them throttled, and 10 seconds stopped.
const laterPage = `
container_cpu_cfs_periods_total{container="app",id="/kubepods/pod-1/app",namespace="prod",pod="web-1"} 1100
container_cpu_cfs_throttled_periods_total{container="app",id="/kubepods/pod-1/app",namespace="prod",pod="web-1"} 150
container_cpu_cfs_throttled_seconds_total{container="app",id="/kubepods/pod-1/app",namespace="prod",pod="web-1"} 30
container_cpu_cfs_periods_total{container="sidecar",id="/kubepods/pod-1/sidecar",namespace="prod",pod="web-1"} 600
container_cpu_cfs_throttled_periods_total{container="sidecar",id="/kubepods/pod-1/sidecar",namespace="prod",pod="web-1"} 0
container_cpu_cfs_throttled_seconds_total{container="sidecar",id="/kubepods/pod-1/sidecar",namespace="prod",pod="web-1"} 0
container_cpu_cfs_periods_total{container="app",id="/kubepods/pod-2/app",namespace="staging",pod="db-0"} 300
container_cpu_cfs_throttled_periods_total{container="app",id="/kubepods/pod-2/app",namespace="staging",pod="db-0"} 30
container_cpu_cfs_throttled_seconds_total{container="app",id="/kubepods/pod-2/app",namespace="staging",pod="db-0"} 5
container_pressure_cpu_waiting_seconds_total{id="/"} 66
container_pressure_cpu_waiting_seconds_total{container="app",id="/kubepods/pod-1/app",namespace="prod",pod="web-1"} 33
`

var (
	base  = time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC)
	after = base.Add(10 * time.Second)
)

func samples(t *testing.T) (nodeSample, nodeSample) {
	t.Helper()
	return parseCadvisor([]byte(page), base), parseCadvisor([]byte(laterPage), after)
}

func TestParseCadvisorReadsTheCountersItNeedsAndIgnoresTheRest(t *testing.T) {
	sample := parseCadvisor([]byte(page), base)

	app := cgroupKey{namespace: "prod", pod: "web-1", container: "app"}
	got, ok := sample.cgroups[app]
	if !ok {
		t.Fatalf("no counters for %+v, got %+v", app, sample.cgroups)
	}
	if got.periods != 1000 || got.throttledPeriods != 100 || got.throttledSeconds != 20 {
		t.Errorf("app counters = %+v, want 1000/100/20", got)
	}
	if !got.hasPressure || got.pressureSeconds != 30 {
		t.Errorf("app pressure = %v/%v, want 30 and present", got.pressureSeconds, got.hasPressure)
	}

	// The node's own cgroup is where a node-wide PSI reading comes from, and it
	// is told apart from every other unlabelled roll-up by its id.
	if !sample.hasRoot || sample.root.pressureSeconds != 60 {
		t.Errorf("root pressure = %v/%v, want 60 and present", sample.root.pressureSeconds, sample.hasRoot)
	}

	// Three cgroups, not five: the memory and usage lines name cgroups this
	// reading has no counter for and must not invent an entry for.
	if len(sample.cgroups) != 3 {
		t.Errorf("cgroups = %d, want 3: %+v", len(sample.cgroups), sample.cgroups)
	}
}

func TestParseCadvisorSurvivesABraceInsideALabelValue(t *testing.T) {
	// cAdvisor's own `name` and `image` labels carry container names and shell
	// commands, which is where a brace turns up. Splitting on the first one
	// would drop the line and quietly under-count the node.
	line := `container_cpu_cfs_periods_total{container="app",name="k8s_a{b}c",namespace="prod",pod="web-1",id="/x"} 42`

	sample := parseCadvisor([]byte(line), base)

	got, ok := sample.cgroups[cgroupKey{namespace: "prod", pod: "web-1", container: "app"}]
	if !ok || got.periods != 42 {
		t.Errorf("periods = %+v (found %v), want 42", got, ok)
	}
}

func TestDelayIsTheRateBetweenTwoSamples(t *testing.T) {
	previous, current := samples(t)

	got := between(previous, current, Scope{Kind: ScopeNode, Name: "worker-1"})

	// Every container on the node: 100 + 100 + 100 further periods.
	if got.periods != 300 {
		t.Errorf("periods = %v, want 300", got.periods)
	}
	if got.throttledPeriods != 60 {
		t.Errorf("throttled periods = %v, want 60", got.throttledPeriods)
	}
	// 11 seconds of throttling over the ten between the samples.
	if got.stalled != 1.1 {
		t.Errorf("stalled = %v, want 1.1", got.stalled)
	}
}

func TestANodeReadsItsPressureOffItsOwnCgroup(t *testing.T) {
	previous, current := samples(t)

	got := between(previous, current, Scope{Kind: ScopeNode, Name: "worker-1"})

	// The root cgroup moved 6 seconds in 10: the machine, not one container.
	if !got.hasPressure || got.pressure != 0.6 {
		t.Errorf("pressure = %v (present %v), want 0.6 from the root cgroup", got.pressure, got.hasPressure)
	}
}

func TestAPodReadsItsPressureOffItsOwnCgroupsInstead(t *testing.T) {
	previous, current := samples(t)

	got := between(previous, current, Scope{Kind: ScopePod, Namespace: "prod", Name: "web-1"})

	// The pod's own container moved 3 seconds in 10. The node's 0.6 is about
	// the whole machine and would be wrong to show against one pod.
	if !got.hasPressure || got.pressure != 0.3 {
		t.Errorf("pressure = %v (present %v), want 0.3 from the pod's own cgroup", got.pressure, got.hasPressure)
	}
}

func TestAScopeCountsOnlyItsOwnContainers(t *testing.T) {
	previous, current := samples(t)

	namespace := between(previous, current, Scope{Kind: ScopeNamespace, Name: "staging"})
	if namespace.periods != 100 || namespace.throttledPeriods != 10 {
		t.Errorf("staging = %v/%v periods, want 100/10 -- prod's containers are not its own",
			namespace.periods, namespace.throttledPeriods)
	}

	pod := between(previous, current, Scope{Kind: ScopePod, Namespace: "prod", Name: "web-1"})
	if pod.periods != 200 {
		t.Errorf("web-1 periods = %v, want 200 across its two containers", pod.periods)
	}
	if pod.stalled != 1 {
		t.Errorf("web-1 stalled = %v, want 1 -- ten seconds of throttling in ten", pod.stalled)
	}
}

func TestACounterThatWentBackwardsIsDroppedRatherThanCounted(t *testing.T) {
	// A restarted container's cgroup starts again from zero. Subtracting would
	// make an enormous negative, and taking the absolute value would make an
	// enormous positive; neither happened, so it contributes nothing.
	restarted := `container_cpu_cfs_periods_total{container="app",id="/x",namespace="prod",pod="web-1"} 5
container_cpu_cfs_throttled_periods_total{container="app",id="/x",namespace="prod",pod="web-1"} 1
container_cpu_cfs_throttled_seconds_total{container="app",id="/x",namespace="prod",pod="web-1"} 1`

	previous := parseCadvisor([]byte(page), base)
	current := parseCadvisor([]byte(restarted), after)

	got := between(previous, current, Scope{Kind: ScopeCluster})

	if got.periods != 0 || got.throttledPeriods != 0 || got.stalled != 0 {
		t.Errorf("totals = %+v, want zeroes: the counters restarted", got)
	}
}

func TestTwoSamplesTakenAtTheSameMomentMakeNoRate(t *testing.T) {
	previous := parseCadvisor([]byte(page), base)
	current := parseCadvisor([]byte(laterPage), base)

	if got := between(previous, current, Scope{Kind: ScopeCluster}); got != (delayTotals{}) {
		t.Errorf("totals = %+v, want nothing: no time passed to divide by", got)
	}
}

func TestPodCgroupsAndSandboxesAreNotCountedAsContainers(t *testing.T) {
	// cAdvisor reports the pod's own cgroup and its sandbox alongside the real
	// containers, with the same counters on them. Counting those would add
	// every container in twice over.
	if (cgroupKey{namespace: "prod", pod: "web-1"}).workload() {
		t.Error("a pod's own cgroup counted as a container")
	}
	if (cgroupKey{namespace: "prod", pod: "web-1", container: "POD"}).workload() {
		t.Error("a pod sandbox counted as a container")
	}
	if !(cgroupKey{namespace: "prod", pod: "web-1", container: "app"}).workload() {
		t.Error("a real container did not count as one")
	}
}

func TestKubeletErrorNamesThePermissionRatherThanTheURL(t *testing.T) {
	err := kubeletError("worker-1", errorString(`nodes "worker-1" is forbidden: User cannot get resource "nodes/proxy"`))

	if !strings.Contains(err.Error(), "nodes/proxy") {
		t.Errorf("error = %q, want it to name the permission to add", err)
	}
	if !strings.Contains(err.Error(), "worker-1") {
		t.Errorf("error = %q, want it to name the node", err)
	}
}

// errorString is an error with a fixed message, for the cases above.
type errorString string

func (e errorString) Error() string { return string(e) }
