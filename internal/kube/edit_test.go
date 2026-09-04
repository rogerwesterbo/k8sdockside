package kube

import (
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const pod = `apiVersion: v1
kind: Pod
metadata:
  name: web
  namespace: default
spec:
  replicas: 3
`

func TestValidateYAMLAcceptsAnObject(t *testing.T) {
	if check := ValidateYAML(pod); !check.Valid {
		t.Errorf("ValidateYAML(pod) = %+v, want valid", check)
	}
}

func TestValidateYAMLSaysWhereItStopped(t *testing.T) {
	cases := []struct {
		name    string
		text    string
		line    int
		message string
	}{
		{"a stray colon", "a: b\nc: d: e\n", 2, "mapping values are not allowed in this context"},
		{"a tab", "a:\n\t- 1\n", 2, "found character that cannot start any token"},
		{"a repeated key", "a: 1\nb: 2\na: 3\n", 3, `mapping key "a" already defined at line 1`},
		{"a list", "- a\n- b\n", 1, "cannot unmarshal !!seq into map[string]interface {}"},
		{"nothing at all", "   \n", 0, "the document is empty"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			check := ValidateYAML(c.text)
			if check.Valid {
				t.Fatalf("ValidateYAML(%q) = valid, want an error", c.text)
			}
			if check.Line != c.line {
				t.Errorf("line = %d, want %d (message %q)", check.Line, c.line, check.Message)
			}
			if check.Message != c.message {
				t.Errorf("message = %q, want %q", check.Message, c.message)
			}
		})
	}
}

// A whole number that arrived as a float would be sent as 3e+00, which the API
// server rejects for a field typed as an integer.
func TestParseObjectKeepsWholeNumbersWhole(t *testing.T) {
	got, err := parseObject(pod)
	if err != nil {
		t.Fatalf("parseObject: %v", err)
	}
	replicas, found, err := unstructured.NestedFieldNoCopy(got.Object, "spec", "replicas")
	if err != nil || !found {
		t.Fatalf("spec.replicas not found: found=%v err=%v", found, err)
	}
	if _, ok := replicas.(int64); !ok {
		t.Errorf("spec.replicas is %T, want int64", replicas)
	}
}

func TestParseObjectRefusesWhatIsNotOneObject(t *testing.T) {
	cases := map[string]string{
		"a split document": pod + "---\napiVersion: v1\nkind: Pod\nmetadata:\n  name: other\n",
		"a broken one":     "a: b\nc: d: e\n",
		"an empty one":     "\n",
	}
	for name, text := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := parseObject(text); err == nil {
				t.Errorf("parseObject(%q) succeeded, want an error", text)
			}
		})
	}
}

func TestParseObjectPointsAtTheBrokenLine(t *testing.T) {
	_, err := parseObject("a: b\nc: d: e\n")
	if err == nil {
		t.Fatal("parseObject succeeded, want an error")
	}
	if !strings.HasPrefix(err.Error(), "line 2: ") {
		t.Errorf("error = %q, want it to start with the line", err)
	}
}

func TestSameObjectRefusesAChangeOfIdentity(t *testing.T) {
	gvk := schema.GroupVersionKind{Version: "v1", Kind: "Pod"}
	object := func(apiVersion, kind, namespace, name string) *unstructured.Unstructured {
		return &unstructured.Unstructured{Object: map[string]any{
			"apiVersion": apiVersion,
			"kind":       kind,
			"metadata":   map[string]any{"name": name, "namespace": namespace},
		}}
	}

	cases := map[string]*unstructured.Unstructured{
		"a rename":     object("v1", "Pod", "default", "other"),
		"a re-kind":    object("v1", "Service", "default", "web"),
		"a re-version": object("v2", "Pod", "default", "web"),
		"a move":       object("v1", "Pod", "kube-system", "web"),
	}
	for name, edited := range cases {
		t.Run(name, func(t *testing.T) {
			if err := sameObject(edited, gvk, "default", "web"); err == nil {
				t.Error("sameObject accepted it, want an error")
			}
		})
	}

	t.Run("the object as it was", func(t *testing.T) {
		if err := sameObject(object("v1", "Pod", "default", "web"), gvk, "default", "web"); err != nil {
			t.Errorf("sameObject: %v", err)
		}
	})

	// A namespaced document is free to leave the namespace out; it is the one
	// it was read from, which is the one it is written back to.
	t.Run("an omitted namespace", func(t *testing.T) {
		edited := object("v1", "Pod", "", "web")
		unstructured.RemoveNestedField(edited.Object, "metadata", "namespace")
		if err := sameObject(edited, gvk, "default", "web"); err != nil {
			t.Errorf("sameObject: %v", err)
		}
	})
}

func TestForEditingDropsTheBookkeepingButKeepsTheVersion(t *testing.T) {
	live := obj(map[string]any{
		"apiVersion": "v1",
		"kind":       "ConfigMap",
		"metadata": map[string]any{
			"name":            "settings",
			"resourceVersion": "4821",
			"managedFields":   []any{map[string]any{"manager": "kubectl"}},
			"annotations": map[string]any{
				"kubectl.kubernetes.io/last-applied-configuration": `{"kind":"ConfigMap"}`,
				"team": "platform",
			},
		},
	})

	edited := forEditing(live)

	if edited.GetManagedFields() != nil {
		t.Error("managedFields survived")
	}
	if _, found := edited.GetAnnotations()["kubectl.kubernetes.io/last-applied-configuration"]; found {
		t.Error("the last-applied annotation survived")
	}
	if got := edited.GetAnnotations()["team"]; got != "platform" {
		t.Errorf("annotations = %v, want the real ones kept", edited.GetAnnotations())
	}
	// Without it a save cannot be told apart from an overwrite.
	if got := edited.GetResourceVersion(); got != "4821" {
		t.Errorf("resourceVersion = %q, want it kept", got)
	}
	// The copy is what is edited; the live object must not have been touched.
	if live.GetManagedFields() == nil {
		t.Error("forEditing modified the object it was given")
	}
}

// The editor opens on this, so it has to read like `kubectl get -o yaml`,
// which means going through JSON: the field names are the API's own and the
// keys come out sorted rather than in whatever order the map iterated.
func TestToYAMLReadsLikeKubectl(t *testing.T) {
	text, err := toYAML(obj(map[string]any{
		"apiVersion": "v1",
		"kind":       "ConfigMap",
		"metadata":   map[string]any{"name": "settings"},
		"data":       map[string]any{"key": "value"},
	}))
	if err != nil {
		t.Fatalf("toYAML: %v", err)
	}
	want := "apiVersion: v1\ndata:\n  key: value\nkind: ConfigMap\n"
	if !strings.HasPrefix(text, want) {
		t.Errorf("toYAML =\n%s\nwant it to start with\n%s", text, want)
	}

	// And it has to come back as what it was.
	back, err := parseObject(text)
	if err != nil {
		t.Fatalf("parseObject: %v", err)
	}
	if back.GetName() != "settings" {
		t.Errorf("name = %q, want settings", back.GetName())
	}
}
