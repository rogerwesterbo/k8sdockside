package kube

import "testing"

// The namespace filter behind a tab: nothing chosen means everything, and a
// choice is a set.
func TestNamespaceFilterTreatsNoChoiceAsEveryNamespace(t *testing.T) {
	for _, names := range [][]string{nil, {}, {""}, {"  "}} {
		if keep := namespaceFilter(names); keep != nil {
			t.Errorf("namespaceFilter(%q) = %v, want nil for every namespace", names, keep)
		}
	}
}

func TestNamespaceFilterKeepsEveryNamespaceNamed(t *testing.T) {
	keep := namespaceFilter([]string{"default", " kube-system ", "", "default"})

	if len(keep) != 2 || !keep["default"] || !keep["kube-system"] {
		t.Errorf("namespaceFilter = %v, want default and kube-system, trimmed and once", keep)
	}
	if keep["monitoring"] {
		t.Error("a namespace not named was kept")
	}
}
