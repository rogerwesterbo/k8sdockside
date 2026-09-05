package main

import (
	"context"
	"fmt"
	"time"

	"github.com/rogerwesterbo/k8sdockside/internal/kube"
	"github.com/rogerwesterbo/k8sdockside/internal/metrics"
	"github.com/rogerwesterbo/k8sdockside/internal/plugins"
)

// Budget reports what one slice of a cluster has, what has been promised out of
// it, and what is actually being used.
//
// `scope` is "cluster", "node" or "namespace"; `name` identifies the node or
// namespace and is ignored for a cluster.
//
// Usage is best-effort by design. metrics-server is asked first, the enabled
// plugins' Prometheus queries second, and a cluster with neither still gets a
// full answer for everything the API server knows -- capacity, allocatable,
// requests and limits -- with the reason for the missing column carried in
// Usage.Error rather than raised as a failure.
func (s *ResourceService) Budget(contextID, scope, name string) (kube.Budget, error) {
	want := kube.Scope{Kind: scope, Name: name}
	switch scope {
	case kube.ScopeCluster, kube.ScopeNode, kube.ScopeNamespace:
	default:
		return kube.Budget{Scope: want}, fmt.Errorf("unknown scope %q", scope)
	}

	kc, err := s.resolve(contextID)
	if err != nil {
		return kube.Budget{Scope: want, Error: err.Error()}, err
	}
	return s.watcher.Budget(kc, want, s.promUsage(contextID, kc, want))
}

// promUsage builds the fallback that reads usage out of the cluster's
// Prometheus, for when metrics-server is not installed.
//
// The queries come from the plugins rather than from here: a Prometheus set up
// differently from the kube-prometheus-stack uses different metric and label
// names, and the plugin file is where the app already lets someone say so.
// Returns nil when nothing can answer, which is the ordinary case on a cluster
// running neither.
func (s *ResourceService) promUsage(contextID string, kc kube.Context, scope kube.Scope) kube.UsageFallback {
	if s.graphs == nil || s.plugins == nil {
		return nil
	}

	return func(ctx context.Context) kube.Usage {
		queries, ok := plugins.UsageQueriesFor(s.plugins.List().Enabled())
		if !ok {
			return kube.Usage{Error: "prometheus: no enabled plugin says how to query it for usage"}
		}

		source := s.graphs.Source(contextID)
		if !source.Available {
			if source.Error != "" {
				return kube.Usage{Error: "prometheus: " + source.Error}
			}
			return kube.Usage{Error: "prometheus: none found in this cluster"}
		}

		fetch := s.watcher.PrometheusFetch(kc, source.Endpoint)

		// A namespace has no node reading, so it is the pods that have to be
		// added up; everything else is answered from the nodes, which is both
		// the smaller query and the more complete number.
		pair, keys := queries.Node, []string{"node"}
		if scope.Kind == kube.ScopeNamespace {
			pair, keys = queries.Pod, []string{"namespace", "pod"}
		}
		if !pair.Filled() {
			return kube.Usage{Error: "prometheus: the plugin gives no usage query for this view"}
		}

		readings, err := promReadings(ctx, fetch, pair, keys)
		if err != nil {
			return kube.Usage{Error: "prometheus: " + err.Error()}
		}

		usage := kube.Usage{Source: kube.SourcePrometheus}
		if scope.Kind == kube.ScopeNamespace {
			usage.Pods = readings
		} else {
			usage.Nodes = readings
		}
		return usage
	}
}

// promReadings runs the CPU and memory queries and pairs them up.
func promReadings(ctx context.Context, fetch metrics.Fetch, pair plugins.UsagePair, keys []string) (map[string]kube.Measured, error) {
	// Bounded a little above the per-request timeout, so a slow query fails
	// with Prometheus's own reason rather than this deadline.
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	cpu, err := metrics.QueryInstant(ctx, fetch, pair.CPU, keys...)
	if err != nil {
		return nil, err
	}
	memory, err := metrics.QueryInstant(ctx, fetch, pair.Memory, keys...)
	if err != nil {
		return nil, err
	}

	out := make(map[string]kube.Measured, len(cpu))
	for key, cores := range cpu {
		out[key] = kube.Measured{CPU: cores}
	}
	for key, bytes := range memory {
		reading := out[key]
		// The queries are declared to return bytes; the amounts are in GiB.
		reading.Memory = bytes / (1 << 30)
		out[key] = reading
	}
	return out, nil
}
