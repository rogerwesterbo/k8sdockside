package kube

import (
	"errors"
	"testing"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// crd builds a CustomResourceDefinition literal.
func crd(group, kind, plural, scope string) *unstructured.Unstructured {
	return obj(map[string]any{
		"metadata": map[string]any{"name": plural + "." + group},
		"spec": map[string]any{
			"group": group,
			"scope": scope,
			"names": map[string]any{"kind": kind, "plural": plural},
		},
	})
}

func TestCustomKindsAreGroupedByTheirAPIGroup(t *testing.T) {
	groups := groupCustomKinds([]*unstructured.Unstructured{
		crd("cert-manager.io", "Certificate", "certificates", "Namespaced"),
		crd("vitistack.io", "Machine", "machines", "Namespaced"),
		crd("cert-manager.io", "Issuer", "issuers", "Namespaced"),
	})

	if len(groups) != 2 {
		t.Fatalf("groups = %d, want 2", len(groups))
	}
	// Alphabetical, so a long list is scannable.
	if groups[0].Group != "cert-manager.io" || groups[1].Group != "vitistack.io" {
		t.Errorf("groups = %q, %q, want cert-manager.io then vitistack.io", groups[0].Group, groups[1].Group)
	}
	if len(groups[0].Kinds) != 2 {
		t.Fatalf("cert-manager kinds = %d, want 2", len(groups[0].Kinds))
	}
	if groups[0].Kinds[0].Label != "Certificate" || groups[0].Kinds[1].Label != "Issuer" {
		t.Errorf("kinds = %q, %q, want Certificate then Issuer", groups[0].Kinds[0].Label, groups[0].Kinds[1].Label)
	}
}

func TestEachCustomKindCarriesWhatOpensItsTab(t *testing.T) {
	groups := groupCustomKinds([]*unstructured.Unstructured{
		crd("vitistack.io", "Machine", "machines", "Namespaced"),
	})

	kind := groups[0].Kinds[0]
	if kind.Kind != CustomKind("machines", "vitistack.io") {
		t.Errorf("kind = %q, want the crd: form", kind.Kind)
	}
	if !kind.Scoped {
		t.Error("a Namespaced definition should be reported as scoped")
	}
}

// A definition with no group belongs to the core API and would otherwise be
// filed under an empty heading.
func TestADefinitionWithoutAGroupIsSkipped(t *testing.T) {
	groups := groupCustomKinds([]*unstructured.Unstructured{
		crd("", "Odd", "odds", "Namespaced"),
		crd("vitistack.io", "Machine", "machines", "Namespaced"),
	})

	if len(groups) != 1 || groups[0].Group != "vitistack.io" {
		t.Errorf("groups = %+v, want only vitistack.io", groups)
	}
}

func TestNoDefinitionsGivesAnEmptyListRatherThanNil(t *testing.T) {
	groups := groupCustomKinds(nil)
	if groups == nil {
		t.Error("groups is nil, want an empty list the frontend can render")
	}
}

// A cluster that does not serve the definitions API has no custom resources.
// That is an answer, not a failure, and reporting it as an error leaves the
// user unable to tell "none here" from "I could not look".
func TestAClusterWithoutTheDefinitionsAPIReportsNoneRatherThanFailing(t *testing.T) {
	groups, err := definitionsFrom(nil, notServed(KindCRDs, "apiextensions.k8s.io", &meta.NoKindMatchError{}))

	if err != nil {
		t.Fatalf("err = %v, want nil -- a cluster without the API simply has none", err)
	}
	if len(groups) != 0 {
		t.Errorf("groups = %v, want empty", groups)
	}
	if groups == nil {
		t.Error("groups is nil, want an empty list")
	}
}

// Anything else is a real failure and has to stay one: being unable to read the
// definitions is not the same as there being none.
func TestARefusedListingIsStillAFailure(t *testing.T) {
	refused := errors.New(`customresourcedefinitions.apiextensions.k8s.io is forbidden: User "x" cannot list resource`)

	if _, err := definitionsFrom(nil, refused); err == nil {
		t.Error("a forbidden listing came back as success")
	}
}

func TestASuccessfulListingIsGrouped(t *testing.T) {
	items := []unstructured.Unstructured{*crd("vitistack.io", "Machine", "machines", "Namespaced")}

	groups, err := definitionsFrom(items, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 1 || groups[0].Group != "vitistack.io" {
		t.Errorf("groups = %+v, want vitistack.io", groups)
	}
}
