package kube

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func object(content map[string]any) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: content}
}

func TestFieldPathReadsADottedPath(t *testing.T) {
	app := object(map[string]any{
		"status": map[string]any{"health": map[string]any{"status": "Degraded"}},
	})

	if got := FieldPath("status.health.status").Value(app); got != "Degraded" {
		t.Errorf("value = %q, want Degraded", got)
	}
	// An object that has not been reconciled yet has no status at all, which is
	// an ordinary answer rather than a failure.
	if got := FieldPath("status.health.status").Value(object(map[string]any{})); got != "" {
		t.Errorf("value = %q, want empty for a missing field", got)
	}
}

// Conditions are the near-universal Kubernetes idiom and a dotted path cannot
// reach into a list, which is the whole reason for the second form.
func TestFieldPathReadsACondition(t *testing.T) {
	release := object(map[string]any{
		"status": map[string]any{"conditions": []any{
			map[string]any{"type": "Reconciling", "status": "False"},
			map[string]any{"type": "Ready", "status": "True"},
		}},
	})

	if got := FieldPath("status.conditions[Ready]").Value(release); got != "True" {
		t.Errorf("value = %q, want True", got)
	}
	if got := FieldPath("status.conditions[Stalled]").Value(release); got != "" {
		t.Errorf("value = %q, want empty for a condition that is not there", got)
	}
}

func TestFieldPathValid(t *testing.T) {
	good := []string{"status.health.status", "status.phase", "status.conditions[Ready]", "spec.suspend"}
	for _, path := range good {
		if !FieldPath(path).Valid() {
			t.Errorf("Valid(%q) = false, want true", path)
		}
	}

	bad := []string{"", "status..phase", "status.$(whoami)", "status.conditions[]", "status.conditions[Ready", "a b"}
	for _, path := range bad {
		if FieldPath(path).Valid() {
			t.Errorf("Valid(%q) = true, want false", path)
		}
	}
}

func TestIsKnownKind(t *testing.T) {
	good := []string{"pods", "deployments", "helmreleases", "crd:applications.argoproj.io"}
	for _, kind := range good {
		if !IsKnownKind(kind) {
			t.Errorf("IsKnownKind(%q) = false, want true", kind)
		}
	}

	bad := []string{"", "widgets", "crd:", "crd:applications", "plugin:argocd/applications"}
	for _, kind := range bad {
		if IsKnownKind(kind) {
			t.Errorf("IsKnownKind(%q) = true, want false", kind)
		}
	}
}
