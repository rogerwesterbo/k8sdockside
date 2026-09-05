// Live usage readings, from whichever metrics source the cluster happens to
// have.
//
// Neither source is guaranteed. metrics-server is tried first: it is what
// `kubectl top` reads, it ships by default on most managed and local distros,
// and it answers in one call per kind. Prometheus is the fallback, and a
// cluster with neither is a perfectly normal cluster -- everything except the
// used column still works, and the reason is carried back so the UI can say
// which of the two is missing rather than drawing an empty bar.
package kube

import (
	"context"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// The metrics API is an ordinary aggregated API, so the dynamic client already
// in hand can read it and no extra module is needed to do so.
var (
	kindNodeMetrics = CustomKind("nodes", "metrics.k8s.io")
	kindPodMetrics  = CustomKind("pods", "metrics.k8s.io")
)

// MetricsServerUsage reads live usage from metrics-server.
//
// A cluster without it is the ordinary case rather than a failure, so the error
// comes back inside the Usage for the UI to explain, not as a Go error that
// would take the rest of the dashboard down with it.
func (c *clusterClient) metricsServerUsage(ctx context.Context, scope Scope) Usage {
	nodes, _, err := c.list(ctx, kindNodeMetrics, metav1.ListOptions{})
	if err != nil {
		return Usage{Error: "metrics-server: " + err.Error()}
	}

	usage := Usage{Source: SourceMetricsServer, Nodes: nodeReadings(nodes)}

	// Pod readings are a much longer list than the node readings and only a
	// namespace needs them -- there is no node reading for a namespace, so its
	// pods have to be added up instead. Failing to get them is not worth losing
	// the node readings over.
	if scope.Kind == ScopeNamespace {
		if pods, _, err := c.list(ctx, kindPodMetrics, metav1.ListOptions{}); err == nil {
			usage.Pods = podReadings(pods)
		}
	}
	return usage
}

// nodeReadings turns a NodeMetrics list into readings keyed by node name.
//
// metrics-server reports CPU in nanocores and memory in KiB; parseCPU and
// parseMemory read the suffixes, so the conversion is theirs rather than
// something spelled out again here.
func nodeReadings(items []unstructured.Unstructured) map[string]Measured {
	out := make(map[string]Measured, len(items))
	for i := range items {
		n := &items[i]
		out[n.GetName()] = Measured{
			CPU:    parseCPU(nestedString(n, "usage", "cpu")),
			Memory: parseMemory(nestedString(n, "usage", "memory")),
		}
	}
	return out
}

// podReadings turns a PodMetrics list into readings keyed by "namespace/name".
//
// A PodMetrics carries no pod total, only a figure per container, so the sum is
// made here.
func podReadings(items []unstructured.Unstructured) map[string]Measured {
	out := make(map[string]Measured, len(items))
	for i := range items {
		p := &items[i]
		var m Measured
		for _, raw := range nestedSlice(p, "containers") {
			usage := asMap(asMap(raw)["usage"])
			m.CPU += parseCPU(mapString(usage, "cpu"))
			m.Memory += parseMemory(mapString(usage, "memory"))
		}
		out[p.GetNamespace()+"/"+p.GetName()] = m
	}
	return out
}

// UsageFallback is asked for readings when metrics-server has not answered. It
// is a function rather than a Prometheus client because this package knows
// nothing about where a cluster's Prometheus is -- that lives a layer up, with
// the discovery and the user's override.
type UsageFallback func(ctx context.Context) Usage

// usage asks the sources in order and keeps whichever answered.
func (c *clusterClient) usage(ctx context.Context, scope Scope, fallback UsageFallback) Usage {
	primary := c.metricsServerUsage(ctx, scope)
	if primary.Source != "" || fallback == nil {
		return primary
	}
	return chooseUsage(primary, fallback(ctx))
}

// chooseUsage keeps the first source that answered, and when neither did,
// carries both reasons.
//
// Both are reported because they fail for unrelated causes -- metrics-server is
// usually simply not installed, while Prometheus is usually installed but
// somewhere this app did not look -- and whoever has to fix it needs to know
// which of the two to go after.
func chooseUsage(primary, fallback Usage) Usage {
	if primary.Source != "" {
		return primary
	}
	if fallback.Source != "" {
		return fallback
	}

	reasons := make([]string, 0, 2)
	for _, reason := range []string{primary.Error, fallback.Error} {
		if reason != "" {
			reasons = append(reasons, reason)
		}
	}
	if len(reasons) == 0 {
		reasons = append(reasons, "no metrics source was reachable")
	}
	return Usage{Error: strings.Join(reasons, "; ")}
}
