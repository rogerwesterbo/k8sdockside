// How much of the time a slice of the cluster spends stopped rather than
// running.
//
// Usage answers "how much CPU is this using". It cannot answer "is this slow
// because something is holding it back", and those are different questions with
// the same-looking chart: two containers burning 200 millicores look identical
// whether one of them has a limit it hits every hundred milliseconds and the
// other has all the room it wants. The kernel counts the difference per cgroup,
// and the kubelet already publishes the counters.
//
// They are read from the kubelet's own cAdvisor endpoint, through the API
// server proxy -- the same route the Prometheus charts take, minus the
// Prometheus. That is the whole point of this file: a cluster running no
// monitoring stack at all can still say whether it is being throttled, which is
// most clusters somebody is debugging on a laptop.
package kube

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SourceKubelet names the kubelet as where a reading came from, beside
// SourceMetricsServer and SourcePrometheus.
const SourceKubelet = "kubelet"

// maxDelayNodes bounds how many kubelets one reading scrapes.
//
// A node's cAdvisor page is a megabyte or two, and a cluster-wide reading would
// otherwise pull one per node every time the panel refreshes. Ten is enough to
// show that a cluster is throttling -- the reading says how many of how many it
// covered, so a partial answer is never passed off as a total.
const maxDelayNodes = 10

// delayScrapes is how many kubelets are read at once. Enough to keep a ten-node
// reading inside its deadline, few enough not to open ten megabytes of body at
// the same moment.
const delayScrapes = 4

// delayTimeout bounds a whole reading, every node in it included.
const delayTimeout = 15 * time.Second

// CPUDelay is one scope's answer to "is this being held back".
type CPUDelay struct {
	// Source is SourceKubelet, or empty when nothing answered.
	Source string `json:"source"`
	// Error is why there is no reading. The usual one is the nodes/proxy
	// permission, which plenty of read-only roles leave out.
	Error string `json:"error"`

	// Throttled is the share of CFS enforcement periods that ended with the
	// cgroup stopped early, 0-1. This is the number to read: a container over
	// its quota is stopped for the rest of the period whatever else the node
	// has spare.
	Throttled float64 `json:"throttled"`
	// Stalled is seconds of throttling per second -- the delay itself, rather
	// than how often it happened. Summed over the containers in scope, so it
	// can exceed one the way a load average can.
	Stalled float64 `json:"stalled"`
	// Limited is whether anything in scope has a CPU limit at all. Without one
	// the kernel never throttles, and a zero means "nothing here is capped"
	// rather than "nothing is being held back" -- different answers that would
	// otherwise look the same.
	Limited bool `json:"limited"`

	// Pressure is PSI: the share of time some task in scope was waiting on CPU
	// it could not get, 0-1. Present only where the kubelet's cAdvisor is new
	// enough to export it on a cgroup v2 node.
	Pressure    float64 `json:"pressure"`
	HasPressure bool    `json:"hasPressure"`

	// Nodes is how many nodes the scope covers and Sampled how many were
	// actually read, so a capped or partly failed reading can say so.
	Nodes   int `json:"nodes"`
	Sampled int `json:"sampled"`
	// Waiting is the first reading of a counter, before there is a second one
	// to make a rate out of. It resolves itself on the next refresh and is not
	// an error.
	Waiting bool `json:"waiting"`
}

// Measured reports whether there is a throttling reading to draw.
func (d CPUDelay) Measured() bool { return d.Source != "" && !d.Waiting }

// CPUDelay reads how much CPU time one scope is losing to throttling.
//
// A cluster where the kubelets cannot be reached is an ordinary cluster, so the
// reason comes back inside the reading rather than as an error that would take
// the panel down with it.
func (w *Watcher) CPUDelay(kc Context, scope Scope) CPUDelay {
	var out CPUDelay
	err := w.withClient(kc, func(c *clusterClient) error {
		ctx, cancel := context.WithTimeout(context.Background(), delayTimeout)
		defer cancel()

		nodes, err := c.nodesInScope(ctx, scope)
		if err != nil {
			return err
		}
		out = c.cpuDelay(ctx, scope, nodes)
		return nil
	})
	if err != nil {
		return CPUDelay{Error: err.Error()}
	}
	return out
}

// nodesInScope names the nodes whose kubelets hold a scope's counters.
//
// Narrowed by the API server wherever it can be: a pod's delay is on exactly
// one node, and asking every kubelet in the cluster for it would be both slower
// and less accurate.
func (c *clusterClient) nodesInScope(ctx context.Context, scope Scope) ([]string, error) {
	switch scope.Kind {
	case ScopeNode:
		return []string{scope.Name}, nil

	case ScopePod:
		pods, err := c.listIn(ctx, KindPods, scope.Namespace, metav1.ListOptions{
			FieldSelector: "metadata.name=" + scope.Name,
		})
		if err != nil {
			return nil, err
		}
		for i := range pods {
			if node := nestedString(&pods[i], "spec", "nodeName"); node != "" {
				return []string{node}, nil
			}
		}
		// A pod that has not been scheduled has no kubelet to ask, which is an
		// ordinary state rather than a failure.
		return nil, nil

	case ScopeNamespace:
		pods, err := c.listIn(ctx, KindPods, scope.Name, metav1.ListOptions{})
		if err != nil {
			return nil, err
		}
		seen := map[string]bool{}
		var out []string
		for i := range pods {
			node := nestedString(&pods[i], "spec", "nodeName")
			if node == "" || seen[node] {
				continue
			}
			seen[node] = true
			out = append(out, node)
		}
		sort.Strings(out)
		return out, nil

	default:
		nodes, _, err := c.list(ctx, KindNodes, metav1.ListOptions{})
		if err != nil {
			return nil, err
		}
		out := make([]string, 0, len(nodes))
		for i := range nodes {
			out = append(out, nodes[i].GetName())
		}
		sort.Strings(out)
		return out, nil
	}
}

// cpuDelay scrapes the kubelets and totals what they say for one scope.
func (c *clusterClient) cpuDelay(ctx context.Context, scope Scope, nodes []string) CPUDelay {
	out := CPUDelay{Nodes: len(nodes)}
	if len(nodes) == 0 {
		return out
	}

	// Sorted by the caller, so the same ten nodes are sampled every refresh --
	// a reading that walked a different subset each time would jump around for
	// reasons that have nothing to do with the cluster.
	wanted := nodes
	if len(wanted) > maxDelayNodes {
		wanted = wanted[:maxDelayNodes]
	}

	var (
		mu      sync.Mutex
		totals  delayTotals
		failure error
		waiting bool
	)

	var wg sync.WaitGroup
	slots := make(chan struct{}, delayScrapes)
	for _, node := range wanted {
		wg.Add(1)
		go func() {
			defer wg.Done()
			slots <- struct{}{}
			defer func() { <-slots }()

			sample, err := c.cadvisorSample(ctx, node)

			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				if failure == nil {
					failure = err
				}
				return
			}
			previous, had := c.rememberSample(node, sample)
			if !had {
				waiting = true
				return
			}
			totals.add(between(previous, sample, scope))
			out.Sampled++
		}()
	}
	wg.Wait()

	if out.Sampled == 0 {
		if failure != nil {
			out.Error = failure.Error()
			return out
		}
		// Every node answered but none of them twice yet: one more refresh and
		// there is a rate.
		out.Waiting = waiting
		out.Source = SourceKubelet
		return out
	}

	out.Source = SourceKubelet
	out.Stalled = totals.stalled
	out.Limited = totals.periods > 0
	if out.Limited {
		out.Throttled = totals.throttledPeriods / totals.periods
	}
	out.Pressure, out.HasPressure = totals.pressure, totals.hasPressure
	return out
}

// rememberSample stores this scrape and hands back the one before it.
func (c *clusterClient) rememberSample(node string, sample nodeSample) (nodeSample, bool) {
	c.delayMu.Lock()
	defer c.delayMu.Unlock()

	if c.delaySamples == nil {
		c.delaySamples = map[string]nodeSample{}
	}
	previous, had := c.delaySamples[node]
	c.delaySamples[node] = sample
	return previous, had
}

// maxCadvisorBody bounds one kubelet's answer. A busy node's page is a few
// megabytes; this is well above that and still a ceiling, so a kubelet
// answering with something unbounded cannot spend all the memory it likes.
const maxCadvisorBody = 32 << 20 // 32 MiB

// cadvisorSample reads one kubelet's counters through the API server proxy.
func (c *clusterClient) cadvisorSample(ctx context.Context, node string) (nodeSample, error) {
	body, err := c.typed.CoreV1().RESTClient().Get().
		Resource("nodes").
		Name(node).
		SubResource("proxy").
		Suffix("metrics", "cadvisor").
		Stream(ctx)
	if err != nil {
		return nodeSample{}, kubeletError(node, err)
	}
	// Closed for its side effect of releasing the connection; a failure to
	// close a body we have finished reading is nothing the caller can act on.
	defer func() { _ = body.Close() }()

	raw, err := io.ReadAll(io.LimitReader(body, maxCadvisorBody))
	if err != nil {
		return nodeSample{}, kubeletError(node, err)
	}
	return parseCadvisor(raw, time.Now()), nil
}

// kubeletError explains a failed scrape in terms of the thing to fix. The wire
// error names a path inside the API server, which tells nobody anything.
func kubeletError(node string, err error) error {
	text := err.Error()
	switch {
	case strings.Contains(text, "forbidden"), strings.Contains(text, "Forbidden"):
		return fmt.Errorf("not allowed to read node %s's kubelet metrics -- that needs the nodes/proxy permission", node)
	case strings.Contains(text, "not found"):
		return fmt.Errorf("node %s has no metrics endpoint", node)
	default:
		return fmt.Errorf("reading node %s's kubelet metrics: %w", node, err)
	}
}

// ---- the counters ----------------------------------------------------------

// cgroupKey identifies one cgroup by the labels cAdvisor puts on it. The node's
// own root cgroup carries none of them, so it is the zero value.
type cgroupKey struct {
	namespace string
	pod       string
	container string
}

// workload reports whether a key names a container somebody deployed, as
// opposed to a pod's own cgroup, its sandbox, or the node's roll-ups. Only
// those carry a CPU limit worth counting, and counting the parents as well
// would add every container in twice.
func (k cgroupKey) workload() bool {
	return k.container != "" && k.container != "POD"
}

// cgroupCounters is what one cgroup contributes to a reading.
type cgroupCounters struct {
	periods          float64
	throttledPeriods float64
	throttledSeconds float64
	pressureSeconds  float64
	hasPressure      bool
}

// nodeSample is one node's counters at one moment. Counters mean nothing on
// their own -- a rate needs two of these, which is why they are kept.
type nodeSample struct {
	at      time.Time
	cgroups map[cgroupKey]cgroupCounters
	// root is the node's own cgroup, which is where its PSI reading lives.
	root    cgroupCounters
	hasRoot bool
}

// delayTotals accumulates one reading across the nodes it covers.
type delayTotals struct {
	periods          float64
	throttledPeriods float64
	stalled          float64
	pressure         float64
	hasPressure      bool
}

func (t *delayTotals) add(other delayTotals) {
	t.periods += other.periods
	t.throttledPeriods += other.throttledPeriods
	t.stalled += other.stalled
	// The worst node rather than their average: a cluster with one stalled node
	// and nine idle ones has a problem, and averaging it away hides exactly the
	// case the reading exists for.
	if other.hasPressure {
		t.pressure = max(t.pressure, other.pressure)
		t.hasPressure = true
	}
}

// between turns two samples of one node into what that node contributes.
//
// Counters that went backwards are dropped rather than counted as a huge jump:
// that is a container that restarted, and its cgroup started again from zero.
func between(previous, current nodeSample, scope Scope) delayTotals {
	var out delayTotals

	elapsed := current.at.Sub(previous.at).Seconds()
	if elapsed <= 0 {
		return out
	}

	for key, now := range current.cgroups {
		if !key.workload() || !key.inScope(scope) {
			continue
		}
		before, had := previous.cgroups[key]
		if !had {
			continue
		}
		out.periods += forward(now.periods, before.periods)
		out.throttledPeriods += forward(now.throttledPeriods, before.throttledPeriods)
		out.stalled += forward(now.throttledSeconds, before.throttledSeconds) / elapsed
	}

	// Node-wide pressure comes off the node's own cgroup, which is the whole
	// machine including the kubelet and the runtime. A namespace or a pod has
	// no such cgroup, so there it is the worst of the cgroups in scope.
	if scope.Kind == ScopeCluster || scope.Kind == ScopeNode {
		if current.hasRoot && previous.hasRoot && current.root.hasPressure && previous.root.hasPressure {
			out.pressure = forward(current.root.pressureSeconds, previous.root.pressureSeconds) / elapsed
			out.hasPressure = true
		}
		return out
	}

	for key, now := range current.cgroups {
		if !now.hasPressure || !key.inScope(scope) {
			continue
		}
		before, had := previous.cgroups[key]
		if !had || !before.hasPressure {
			continue
		}
		out.pressure = max(out.pressure, forward(now.pressureSeconds, before.pressureSeconds)/elapsed)
		out.hasPressure = true
	}
	return out
}

// inScope reports whether one cgroup belongs to the slice being read.
func (k cgroupKey) inScope(scope Scope) bool {
	switch scope.Kind {
	case ScopeNamespace:
		return k.namespace == scope.Name
	case ScopePod:
		return k.namespace == scope.Namespace && k.pod == scope.Name
	default:
		// A node's reading is everything its kubelet reports, and a cluster's
		// is that again for every node sampled.
		return true
	}
}

// forward is a counter's increase, or zero if it went backwards.
func forward(now, before float64) float64 {
	if now < before {
		return 0
	}
	return now - before
}

// ---- reading the kubelet's page --------------------------------------------

// The counters a reading is made of. cAdvisor publishes hundreds of series per
// node; these four are the only ones read, and every other line is skipped on
// the metric name alone.
const (
	metricPeriods          = "container_cpu_cfs_periods_total"
	metricThrottledPeriods = "container_cpu_cfs_throttled_periods_total"
	metricThrottledSeconds = "container_cpu_cfs_throttled_seconds_total"
	metricPressureWaiting  = "container_pressure_cpu_waiting_seconds_total"
)

// parseCadvisor reads a kubelet's Prometheus-format page into one sample.
//
// A hand-written reader rather than a parsing library: four metric names out of
// a page of hundreds, and the alternative is a dependency that would have to
// build a full model of every line to give back four numbers.
func parseCadvisor(raw []byte, at time.Time) nodeSample {
	sample := nodeSample{at: at, cgroups: map[cgroupKey]cgroupCounters{}}

	for line := range strings.Lines(string(raw)) {
		line = strings.TrimSpace(line)
		if line == "" || line[0] == '#' {
			continue
		}
		brace := strings.IndexByte(line, '{')
		if brace < 0 {
			continue
		}
		name := line[:brace]
		switch name {
		case metricPeriods, metricThrottledPeriods, metricThrottledSeconds, metricPressureWaiting:
		default:
			continue
		}

		labels, rest, ok := splitLabels(line[brace+1:])
		if !ok {
			continue
		}
		value, ok := sampleValue(rest)
		if !ok {
			continue
		}

		key, id := cgroupOf(labels)
		// The node's own cgroup is the one with no pod on it and the root id.
		// Every other unlabelled cgroup -- /kubepods, a systemd slice -- is a
		// roll-up of things already counted, so it is left out.
		if key == (cgroupKey{}) {
			if id != "/" {
				continue
			}
			sample.hasRoot = true
			apply(&sample.root, name, value)
			continue
		}

		counters := sample.cgroups[key]
		apply(&counters, name, value)
		sample.cgroups[key] = counters
	}
	return sample
}

// apply files one sample under the counter it belongs to.
func apply(into *cgroupCounters, metric string, value float64) {
	switch metric {
	case metricPeriods:
		into.periods = value
	case metricThrottledPeriods:
		into.throttledPeriods = value
	case metricThrottledSeconds:
		into.throttledSeconds = value
	case metricPressureWaiting:
		into.pressureSeconds = value
		into.hasPressure = true
	}
}

// splitLabels splits `a="b",c="d"} 1.5` at the brace that closes the label set,
// which is not simply the next one: a label value may contain braces, and
// cAdvisor's `name` and `image` labels regularly do.
func splitLabels(s string) (labels, rest string, ok bool) {
	inQuotes, escaped := false, false
	for i := 0; i < len(s); i++ {
		switch {
		case escaped:
			escaped = false
		case s[i] == '\\' && inQuotes:
			escaped = true
		case s[i] == '"':
			inQuotes = !inQuotes
		case s[i] == '}' && !inQuotes:
			return s[:i], s[i+1:], true
		}
	}
	return "", "", false
}

// cgroupOf pulls the four labels a reading needs out of one line's label set.
func cgroupOf(labels string) (cgroupKey, string) {
	var key cgroupKey
	var id string
	eachLabel(labels, func(name, value string) {
		switch name {
		case "namespace":
			key.namespace = value
		case "pod":
			key.pod = value
		case "container":
			key.container = value
		case "id":
			id = value
		}
	})
	return key, id
}

// eachLabel calls fn for every `name="value"` pair in a label set.
func eachLabel(set string, fn func(name, value string)) {
	for i := 0; i < len(set); {
		eq := strings.IndexByte(set[i:], '=')
		if eq < 0 {
			return
		}
		name := strings.TrimSpace(set[i : i+eq])
		j := i + eq + 1
		if j >= len(set) || set[j] != '"' {
			return
		}
		j++

		start, escaped, end := j, false, -1
		for ; j < len(set); j++ {
			if escaped {
				escaped = false
				continue
			}
			if set[j] == '\\' {
				escaped = true
				continue
			}
			if set[j] == '"' {
				end = j
				break
			}
		}
		if end < 0 {
			return
		}
		value := set[start:end]
		// Unquoting only where it is needed: the labels this reads -- a
		// namespace, a pod, a container, a cgroup path -- never carry an
		// escape, and doing it unconditionally would allocate for every line
		// of a page with tens of thousands of them.
		if strings.IndexByte(value, '\\') >= 0 {
			if unquoted, err := strconv.Unquote(`"` + value + `"`); err == nil {
				value = unquoted
			}
		}
		fn(name, value)

		j = end + 1
		if j < len(set) && set[j] == ',' {
			j++
		}
		i = j
	}
}

// sampleValue reads the number after a label set. A line may carry a timestamp
// after it, which is not something any of these counters need.
func sampleValue(rest string) (float64, bool) {
	rest = strings.TrimSpace(rest)
	if space := strings.IndexByte(rest, ' '); space >= 0 {
		rest = rest[:space]
	}
	value, err := strconv.ParseFloat(rest, 64)
	if err != nil {
		return 0, false
	}
	return value, true
}
