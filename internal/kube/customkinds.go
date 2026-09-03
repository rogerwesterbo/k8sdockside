package kube

import (
	"context"
	"sort"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// The custom resources a cluster serves, arranged for the sidebar.
//
// The definitions themselves are already listable as a table; this is the same
// data seen the other way round -- as a tree of API groups, each holding the
// kinds it defines -- because that is how someone looks for a custom resource
// they know the name of.

// CustomResourceGroup is one API group and the kinds defined under it.
type CustomResourceGroup struct {
	Group string               `json:"group"`
	Kinds []CustomResourceKind `json:"kinds"`
}

// groupCustomKinds arranges definitions by API group, both levels sorted so a
// long list can be scanned.
//
// A definition with no group is skipped: it would be filed under a blank
// heading, and a CRD without a group is not something the API server serves.
func groupCustomKinds(crds []*unstructured.Unstructured) []CustomResourceGroup {
	byGroup := map[string][]CustomResourceKind{}
	for _, crd := range crds {
		kind := CustomResourceKindFor(crd)
		if kind.Group == "" || kind.Plural == "" {
			continue
		}
		byGroup[kind.Group] = append(byGroup[kind.Group], kind)
	}

	groups := make([]CustomResourceGroup, 0, len(byGroup))
	for group, kinds := range byGroup {
		sort.Slice(kinds, func(i, j int) bool { return kinds[i].Label < kinds[j].Label })
		groups = append(groups, CustomResourceGroup{Group: group, Kinds: kinds})
	}
	sort.Slice(groups, func(i, j int) bool { return groups[i].Group < groups[j].Group })
	return groups
}

// definitionsFrom turns the outcome of listing definitions into what the
// sidebar should show.
//
// A cluster that does not serve the definitions API is not a failure: it has no
// custom resources, which is an answer. Reporting it as an error would leave
// the reader unable to tell "none here" from "I could not look" -- and those
// call for opposite reactions. Every other failure stays one.
func definitionsFrom(items []unstructured.Unstructured, err error) ([]CustomResourceGroup, error) {
	if ErrNotServed(err) {
		return []CustomResourceGroup{}, nil
	}
	if err != nil {
		return []CustomResourceGroup{}, err
	}

	pointers := make([]*unstructured.Unstructured, 0, len(items))
	for i := range items {
		pointers = append(pointers, &items[i])
	}
	return groupCustomKinds(pointers), nil
}

// CustomResourceKinds lists what a cluster defines, grouped by API group.
//
// One-shot rather than watched: the sidebar asks for it when the definitions
// section is opened, and a cluster's set of CRDs changes when someone installs
// an operator, not continuously.
func (w *Watcher) CustomResourceKinds(kc Context) ([]CustomResourceGroup, error) {
	groups := []CustomResourceGroup{}
	err := w.withClient(kc, func(c *clusterClient) error {
		ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
		defer cancel()

		items, _, err := c.list(ctx, KindCRDs, metav1.ListOptions{})
		found, err := definitionsFrom(items, err)
		if err != nil {
			return err
		}
		groups = found
		return nil
	})
	return groups, err
}
