package kube

import (
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
)

// KindsServed reports which of a set of kinds this cluster serves, in one
// sweep of the discovery API -- the same listing `kubectl api-resources`
// prints.
//
// It is plural because that is the shape of the underlying question. Discovery
// is answered by a GET of /apis followed by one per group-version, so a *single*
// kind costs the whole fan-out; asking about three Argo CD CRDs one at a time
// cost three of them, and on a cluster without Argo CD each miss also invalidated
// the cache the last lookup had just filled, so the plugin overviews re-swept
// for every kind they named. One sweep answers for all of them.
//
// A cluster that has had a CRD installed since we last looked would answer from
// the cache and be wrong, so a sweep that fails to account for every kind asked
// about is retried once against a fresh listing. That is the same "look again
// before reporting it missing" the per-kind path does in mappingFor, except
// that it happens at most once for the whole set instead of once per miss.
func (w *Watcher) KindsServed(kc Context, kinds []string) (map[string]bool, error) {
	var out map[string]bool
	err := w.withClient(kc, func(c *clusterClient) error {
		served, err := sweepFor(c.disco, kinds)
		out = served
		return err
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// cachedResources is the part of the cached discovery client this needs: the
// listing, whether it came from a cache that could be stale, and a way to drop
// it. Narrow enough to fake, which is what lets the retry policy below be held
// to exactly the case that needs it.
type cachedResources interface {
	ServerGroupsAndResources() ([]*metav1.APIGroup, []*metav1.APIResourceList, error)
	Fresh() bool
	Invalidate()
}

// sweepFor answers for every kind against one listing, looking a second time
// only when the first answer came up short *and* came out of a cache that could
// be stale.
//
// Both halves of that condition matter. Without the retry a CRD installed since
// we last looked would read as absent for as long as the cache lived. Without
// the freshness check the retry would fire on every cluster that simply does
// not have the solution -- the ordinary case -- throwing away a listing fetched
// microseconds earlier to fetch the identical one again. Since a one-shot call
// on a context with no open tabs builds its client and drops it, that cache is
// usually cold and the first sweep is already the expensive one.
func sweepFor(d cachedResources, kinds []string) (map[string]bool, error) {
	lists, err := serverResources(d)
	if err != nil {
		return nil, err
	}
	served := servedIn(lists, kinds)
	if allServed(served) || d.Fresh() {
		return served, nil
	}

	d.Invalidate()
	lists, err = serverResources(d)
	if err != nil {
		// The cached answer is still the best one we have, and an overview
		// would rather say "not installed" than have nothing to show.
		return served, nil
	}
	return servedIn(lists, kinds), nil
}

// serverResources lists every resource the cluster serves.
//
// A partial listing is used rather than discarded. One unreachable aggregated
// APIService -- a metrics server that is down, most often -- fails its own group
// and returns ErrGroupDiscoveryFailed alongside every group that did answer, and
// treating that as a total failure would make Argo CD read as uninstalled
// because something unrelated was broken.
func serverResources(d cachedResources) ([]*metav1.APIResourceList, error) {
	_, lists, err := d.ServerGroupsAndResources()
	if err != nil {
		if !discovery.IsGroupDiscoveryFailedError(err) || len(lists) == 0 {
			return nil, err
		}
	}
	return lists, nil
}

func allServed(served map[string]bool) bool {
	for _, ok := range served {
		if !ok {
			return false
		}
	}
	return true
}

// servedIn answers which of the wanted kinds appear in a discovery listing.
//
// Custom resources are matched by plural and group, which is how a "crd:" kind
// names itself and what a CRD's own identity is. Built-in kinds are matched by
// group and Kind, because that is what the app maps its own names onto and it
// leaves the version to the cluster -- one entry covering v1 and v1beta1 alike.
func servedIn(lists []*metav1.APIResourceList, kinds []string) map[string]bool {
	plurals := map[schema.GroupResource]bool{}
	groupKinds := map[schema.GroupKind]bool{}
	for _, list := range lists {
		if list == nil {
			continue
		}
		gv, err := schema.ParseGroupVersion(list.GroupVersion)
		if err != nil {
			continue
		}
		for _, r := range list.APIResources {
			// A subresource is listed as "applications/status". It is not a
			// collection anything can be listed from, so matching on it would
			// report a kind as served on the strength of the wrong thing.
			if strings.Contains(r.Name, "/") {
				continue
			}
			plurals[schema.GroupResource{Group: gv.Group, Resource: r.Name}] = true
			groupKinds[schema.GroupKind{Group: gv.Group, Kind: r.Kind}] = true
		}
	}

	out := make(map[string]bool, len(kinds))
	for _, kind := range kinds {
		out[kind] = servedKind(kind, plurals, groupKinds)
	}
	return out
}

func servedKind(kind string, plurals map[schema.GroupResource]bool, groupKinds map[schema.GroupKind]bool) bool {
	if plural, group, ok := ParseCustomKind(kind); ok {
		return plurals[schema.GroupResource{Group: group, Resource: plural}]
	}
	// Helm releases are read out of Secrets rather than served as an API of
	// their own, so the cluster in front of us can always answer for them.
	if kind == KindHelmReleases {
		return true
	}
	gk, known := builtinKinds[kind]
	if !known {
		return false
	}
	return groupKinds[gk]
}
