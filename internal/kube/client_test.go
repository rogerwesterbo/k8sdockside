package kube

import (
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// countingMapper is a REST mapper that serves what it is given and counts how
// often it is told to look again. Only the two lookups mappingFor and
// mappingForKind make are implemented; the embedded nil interface turns any
// other call into a panic, which is the right outcome for a test that strayed.
type countingMapper struct {
	meta.ResettableRESTMapper
	resets int
	served map[schema.GroupKind]*meta.RESTMapping
	// installed is what a reset reveals, standing in for a CRD applied since
	// the cache was last filled.
	installed map[schema.GroupKind]*meta.RESTMapping
}

func (m *countingMapper) Reset() {
	m.resets++
	if m.installed != nil {
		m.served = m.installed
	}
}

func (m *countingMapper) RESTMapping(gk schema.GroupKind, _ ...string) (*meta.RESTMapping, error) {
	if mapping, ok := m.served[gk]; ok {
		return mapping, nil
	}
	return nil, &meta.NoKindMatchError{GroupKind: gk}
}

func (m *countingMapper) KindFor(gvr schema.GroupVersionResource) (schema.GroupVersionKind, error) {
	for _, mapping := range m.served {
		if mapping.Resource.Group == gvr.Group && mapping.Resource.Resource == gvr.Resource {
			return mapping.GroupVersionKind, nil
		}
	}
	return schema.GroupVersionKind{}, &meta.NoResourceMatchError{PartialResource: gvr}
}

var nodeMetricsGK = schema.GroupKind{Group: "metrics.k8s.io", Kind: "NodeMetrics"}

func nodeMetricsMapping() *meta.RESTMapping {
	return &meta.RESTMapping{
		Resource:         schema.GroupVersionResource{Group: "metrics.k8s.io", Version: "v1beta1", Resource: "nodes"},
		GroupVersionKind: schema.GroupVersionKind{Group: "metrics.k8s.io", Version: "v1beta1", Kind: "NodeMetrics"},
		Scope:            meta.RESTScopeRoot,
	}
}

// A miss looks again once, in case the kind was installed since the cache was
// filled -- and then takes the miss at face value for a while, because a kind
// that is simply not there must not cost a rediscovery every time it is asked
// for.
func TestAMissResetsDiscoveryOnceAndNotAgainRightAway(t *testing.T) {
	mapper := &countingMapper{}
	c := &clusterClient{mapper: mapper}

	if _, err := c.mappingFor(nodeMetricsGK); err == nil {
		t.Fatal("want a miss for a kind nobody serves")
	}
	if mapper.resets != 1 {
		t.Fatalf("resets = %d after the first miss, want 1", mapper.resets)
	}

	if _, err := c.mappingFor(nodeMetricsGK); err == nil {
		t.Fatal("want a miss for a kind nobody serves")
	}
	if mapper.resets != 1 {
		t.Errorf("resets = %d after a second miss within resetEvery, want still 1", mapper.resets)
	}

	c.resetAt = time.Now().Add(-2 * resetEvery)
	_, _ = c.mappingFor(nodeMetricsGK)
	if mapper.resets != 2 {
		t.Errorf("resets = %d once resetEvery has passed, want 2", mapper.resets)
	}
}

// The bound is per client, not per kind: the budget asks for node metrics and
// then pod metrics, and the second miss must not pay for a second look.
func TestACustomKindMissSharesTheBound(t *testing.T) {
	mapper := &countingMapper{}
	c := &clusterClient{mapper: mapper}

	if _, err := c.mappingForKind(CustomKind("nodes", "metrics.k8s.io")); err == nil {
		t.Fatal("want a miss for a kind nobody serves")
	}
	if _, err := c.mappingForKind(CustomKind("pods", "metrics.k8s.io")); err == nil {
		t.Fatal("want a miss for a kind nobody serves")
	}
	if mapper.resets != 1 {
		t.Errorf("resets = %d after two misses back to back, want 1", mapper.resets)
	}
}

// The reason the reset exists at all: a kind applied since the cache was
// filled is found on the second look rather than reported missing.
func TestARefreshFindsAKindInstalledSinceTheLastLook(t *testing.T) {
	mapper := &countingMapper{installed: map[schema.GroupKind]*meta.RESTMapping{nodeMetricsGK: nodeMetricsMapping()}}
	c := &clusterClient{mapper: mapper}

	m, err := c.mappingFor(nodeMetricsGK)
	if err != nil {
		t.Fatalf("mappingFor after the kind was installed: %v", err)
	}
	if m.Resource.Resource != "nodes" || mapper.resets != 1 {
		t.Errorf("got %+v after %d reset(s), want the nodes mapping after exactly one", m.Resource, mapper.resets)
	}

	// And by the custom-kind route, which the metrics API is read through.
	if _, err := c.mappingForKind(CustomKind("nodes", "metrics.k8s.io")); err != nil {
		t.Errorf("mappingForKind after the kind was installed: %v", err)
	}
}

// A client just built has just filled its cache, so its first miss is against
// a listing fetched moments ago and is not worth fetching again.
func TestAFreshClientCountsItsBuildAsAReset(t *testing.T) {
	c, err := newClusterClient(kubeconfigFor(t, "https://127.0.0.1:1"))
	if err != nil {
		t.Fatalf("newClusterClient: %v", err)
	}
	if time.Since(c.resetAt) > time.Minute {
		t.Errorf("resetAt = %v, want the moment the client was built", c.resetAt)
	}
	if c.refreshDiscovery() {
		t.Error("a miss right after the build refreshed discovery, want it to wait out resetEvery")
	}
}
