package kube

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// The listing `kubectl api-resources` prints, cut down to what these tests need:
// the core group, apps, and an Argo CD install.
func argoCluster() []*metav1.APIResourceList {
	return []*metav1.APIResourceList{
		{
			GroupVersion: "v1",
			APIResources: []metav1.APIResource{
				{Name: "pods", Kind: "Pod", Namespaced: true},
				{Name: "secrets", Kind: "Secret", Namespaced: true},
			},
		},
		{
			GroupVersion: "apps/v1",
			APIResources: []metav1.APIResource{
				{Name: "deployments", Kind: "Deployment", Namespaced: true},
			},
		},
		{
			GroupVersion: "argoproj.io/v1alpha1",
			APIResources: []metav1.APIResource{
				{Name: "applications", Kind: "Application", Namespaced: true},
				{Name: "applicationsets", Kind: "ApplicationSet", Namespaced: true},
				{Name: "appprojects", Kind: "AppProject", Namespaced: true},
			},
		},
	}
}

func TestServedInFindsCustomResourcesByPluralAndGroup(t *testing.T) {
	got := servedIn(argoCluster(), []string{
		"crd:applications.argoproj.io",
		"crd:appprojects.argoproj.io",
		"crd:kustomizations.kustomize.toolkit.fluxcd.io",
	})

	if !got["crd:applications.argoproj.io"] || !got["crd:appprojects.argoproj.io"] {
		t.Errorf("Argo CD's own CRDs read as absent from a cluster that serves them: %v", got)
	}
	if got["crd:kustomizations.kustomize.toolkit.fluxcd.io"] {
		t.Error("a Flux CRD read as served by a cluster that only has Argo CD")
	}
}

// A subresource is listed alongside the resource it hangs off. Matching on it
// would make "crd:applications/status.argoproj.io" resolve, and would let a
// kind read as served on the strength of something that is not a collection.
func TestServedInIgnoresSubresources(t *testing.T) {
	lists := []*metav1.APIResourceList{{
		GroupVersion: "argoproj.io/v1alpha1",
		APIResources: []metav1.APIResource{
			{Name: "applications/status", Kind: "Application"},
		},
	}}

	if servedIn(lists, []string{"crd:applications.argoproj.io"})["crd:applications.argoproj.io"] {
		t.Error("a subresource listing was taken for the resource itself")
	}
}

// Built-in kinds are named by the app's own word for them, and have to be
// matched through the group and Kind the app maps them onto.
func TestServedInFindsBuiltinKinds(t *testing.T) {
	got := servedIn(argoCluster(), []string{"pods", "deployments", "gateways"})

	if !got["pods"] {
		t.Error("pods read as absent; a core-group kind has an empty group")
	}
	if !got["deployments"] {
		t.Error("deployments read as absent")
	}
	if got["gateways"] {
		t.Error("the Gateway API read as served by a cluster without it")
	}
}

// Helm releases are read out of Secrets rather than served by the API, so a
// plugin naming them is asking about something every cluster can answer.
func TestServedInTreatsHelmReleasesAsAlwaysServed(t *testing.T) {
	if !servedIn(argoCluster(), []string{KindHelmReleases})[KindHelmReleases] {
		t.Error("helmreleases read as absent; they are synthesised from Secrets")
	}
}

func TestServedInSaysNoToAKindItDoesNotKnow(t *testing.T) {
	if servedIn(argoCluster(), []string{"nonsense"})["nonsense"] {
		t.Error("an unknown kind read as served")
	}
}

// One group failing discovery -- a stale aggregated APIService is the usual
// cause -- must not take the kinds every other group answered for with it.
func TestServedInUsesWhatCameBackWhenAGroupIsBroken(t *testing.T) {
	got := servedIn(argoCluster(), []string{"crd:applications.argoproj.io", "pods"})

	if !got["crd:applications.argoproj.io"] || !got["pods"] {
		t.Errorf("a partial listing lost kinds it did contain: %v", got)
	}
}

// fakeDisco is a discovery cache that counts its sweeps, so the retry can be
// held to the one case that needs it.
type fakeDisco struct {
	lists  []*metav1.APIResourceList
	fresh  bool
	sweeps int
	// gains is served by the cluster only after an Invalidate, standing in for
	// a CRD installed since we last looked.
	gains *metav1.APIResourceList
}

func (f *fakeDisco) ServerGroupsAndResources() ([]*metav1.APIGroup, []*metav1.APIResourceList, error) {
	f.sweeps++
	return nil, f.lists, nil
}

func (f *fakeDisco) Fresh() bool { return f.fresh }

func (f *fakeDisco) Invalidate() {
	f.fresh = true
	if f.gains != nil {
		f.lists = append(f.lists, f.gains)
		f.gains = nil
	}
}

func TestSweepDoesNotLookTwiceWhenEverythingWasFound(t *testing.T) {
	d := &fakeDisco{lists: argoCluster()}

	got, err := sweepFor(d, []string{"crd:applications.argoproj.io", "pods"})
	if err != nil {
		t.Fatalf("sweepFor: %v", err)
	}

	if !got["crd:applications.argoproj.io"] {
		t.Error("a kind the cluster serves read as absent")
	}
	if d.sweeps != 1 {
		t.Errorf("swept %d times, want 1: nothing was missing, so there is nothing to look again for", d.sweeps)
	}
}

// A CRD installed since we last looked would otherwise read as absent forever.
func TestSweepLooksAgainWhenAStaleCacheCameUpShort(t *testing.T) {
	d := &fakeDisco{
		lists: []*metav1.APIResourceList{{GroupVersion: "v1", APIResources: []metav1.APIResource{{Name: "pods", Kind: "Pod"}}}},
		fresh: false,
		gains: &metav1.APIResourceList{
			GroupVersion: "argoproj.io/v1alpha1",
			APIResources: []metav1.APIResource{{Name: "applications", Kind: "Application"}},
		},
	}

	got, err := sweepFor(d, []string{"crd:applications.argoproj.io"})
	if err != nil {
		t.Fatalf("sweepFor: %v", err)
	}

	if !got["crd:applications.argoproj.io"] {
		t.Error("a CRD installed since the cache was filled still reads as absent")
	}
	if d.sweeps != 2 {
		t.Errorf("swept %d times, want 2: a stale miss is worth looking again for", d.sweeps)
	}
}

// The expensive case, and the one that made the overview slow. A cluster with
// no Argo CD answers "absent" from a listing that was *just* fetched, and
// throwing that away to fetch the identical listing again doubles the cost of
// every overview on every cluster that does not have the plugin's solution.
func TestSweepTrustsAFreshMiss(t *testing.T) {
	d := &fakeDisco{lists: argoCluster(), fresh: true}

	got, err := sweepFor(d, []string{"crd:kustomizations.kustomize.toolkit.fluxcd.io"})
	if err != nil {
		t.Fatalf("sweepFor: %v", err)
	}

	if got["crd:kustomizations.kustomize.toolkit.fluxcd.io"] {
		t.Error("a kind the cluster does not serve read as served")
	}
	if d.sweeps != 1 {
		t.Errorf("swept %d times, want 1: the listing was already current, so looking again buys nothing", d.sweeps)
	}
}
