package kube

import (
	"context"
	"encoding/base64"
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

// Secrets are read back base64-encoded, which is unreadable and close to
// uneditable: changing one character of a password means decoding by hand,
// editing, re-encoding and pasting back. These pin the plaintext form the
// editor is given instead, and the line it does not cross.

func secret(data map[string]any) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "Secret",
		"metadata":   map[string]any{"name": "creds", "namespace": "default"},
		"type":       "Opaque",
		"data":       data,
	}}
}

func TestASecretIsOpenedInPlainText(t *testing.T) {
	out := forEditing(secret(map[string]any{
		"username": base64.StdEncoding.EncodeToString([]byte("admin")),
		"password": base64.StdEncoding.EncodeToString([]byte("hunter2")),
	}))

	text, found, _ := unstructured.NestedStringMap(out.Object, "stringData")
	if !found {
		t.Fatal("no stringData: the values are still base64 and still uneditable")
	}
	if text["username"] != "admin" || text["password"] != "hunter2" {
		t.Errorf("stringData = %v, want the decoded values", text)
	}
	// data and stringData naming the same key would leave which one wins to the
	// API server's merge rules rather than to the document.
	if _, still, _ := unstructured.NestedMap(out.Object, "data"); still {
		t.Error("data survived beside stringData; the same key is now written twice")
	}
}

// stringData is not a display trick: it is a real write-only field the API
// server base64s back into data. That is what keeps the document applicable
// with kubectl and the save a round trip through the server's own conversion.
func TestTheDecodedFormIsStillAValidSecret(t *testing.T) {
	out := forEditing(secret(map[string]any{
		"token": base64.StdEncoding.EncodeToString([]byte("s3cr3t")),
	}))
	rendered, err := toYAML(out)
	if err != nil {
		t.Fatalf("rendering: %v", err)
	}
	back, err := parseObject(rendered)
	if err != nil {
		t.Fatalf("the document the editor shows does not parse back: %v", err)
	}
	if got := back.GetKind(); got != "Secret" {
		t.Errorf("kind = %q", got)
	}
	text, _, _ := unstructured.NestedStringMap(back.Object, "stringData")
	if text["token"] != "s3cr3t" {
		t.Errorf("stringData = %v, want the plaintext to survive the round trip", text)
	}
}

// A keystore or a DER certificate has no plaintext form. Turning one into a
// string would corrupt it on the way back, so it stays base64.
func TestBinaryValuesStayEncoded(t *testing.T) {
	binary := base64.StdEncoding.EncodeToString([]byte{0xff, 0xfe, 0x00, 0x01})
	out := forEditing(secret(map[string]any{
		"keystore.p12": binary,
		"username":     base64.StdEncoding.EncodeToString([]byte("admin")),
	}))

	data, found, _ := unstructured.NestedStringMap(out.Object, "data")
	if !found || data["keystore.p12"] != binary {
		t.Errorf("data = %v, want the binary entry left exactly as it was", data)
	}
	if _, leaked := data["username"]; leaked {
		t.Error("the text entry was left in data as well as decoded")
	}
	text, _, _ := unstructured.NestedStringMap(out.Object, "stringData")
	if text["username"] != "admin" {
		t.Errorf("stringData = %v, want the text entry decoded beside the binary one", text)
	}
}

// A value that is not base64 at all is somebody's hand-edited object. It is
// left alone rather than guessed at.
func TestAValueThatIsNotBase64IsLeftAlone(t *testing.T) {
	out := forEditing(secret(map[string]any{"broken": "not base64!!"}))

	data, found, _ := unstructured.NestedStringMap(out.Object, "data")
	if !found || data["broken"] != "not base64!!" {
		t.Errorf("data = %v, want the value untouched", data)
	}
	if _, invented := out.Object["stringData"]; invented {
		t.Error("stringData was invented for a value that could not be decoded")
	}
}

// Only Secrets. Anything else with a data map -- a ConfigMap, a custom resource
// -- means something entirely different by it.
func TestOnlySecretsAreDecoded(t *testing.T) {
	cm := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "ConfigMap",
		"metadata":   map[string]any{"name": "settings"},
		"data":       map[string]any{"greeting": "aGVsbG8="},
	}}

	out := forEditing(cm)
	if _, decoded := out.Object["stringData"]; decoded {
		t.Error("a ConfigMap's data was treated as base64")
	}
	data, _, _ := unstructured.NestedStringMap(out.Object, "data")
	if data["greeting"] != "aGVsbG8=" {
		t.Errorf("data = %v, want it untouched", data)
	}
}

// An empty value is a real thing to store, and decodes to an empty string
// rather than being mistaken for an absent one.
func TestAnEmptyValueDecodesRatherThanVanishing(t *testing.T) {
	out := forEditing(secret(map[string]any{"optional": ""}))

	text, found, _ := unstructured.NestedStringMap(out.Object, "stringData")
	if !found {
		t.Fatal("no stringData for an empty value")
	}
	if got, present := text["optional"]; !present || got != "" {
		t.Errorf("stringData[optional] = %q (present %v), want an empty string", got, present)
	}
}

// The report the detail panel shows is built from a fixed list of top-level
// fields. A revealed Secret's values move to stringData, so a list naming only
// `data` would answer the reveal button with a report showing nothing -- which
// is the one way this feature can be wired up and still look broken.
func TestARevealedSecretRendersItsDecodedValues(t *testing.T) {
	// Serving no kinds at all means objectEvents takes its early return before
	// it reaches the dynamic client, which this test does not have.
	c := &clusterClient{mapper: &countingMapper{}}
	gvr := schema.GroupVersionResource{Version: "v1", Resource: "secrets"}

	object := secret(map[string]any{
		"password": base64.StdEncoding.EncodeToString([]byte("hunter2")),
	})

	hidden := describeLive(context.Background(), c, object, gvr)
	if !strings.Contains(hidden, "aHVudGVyMg==") {
		t.Errorf("the unrevealed report does not show the encoded value:\n%s", hidden)
	}
	if strings.Contains(hidden, "hunter2") {
		t.Errorf("the unrevealed report leaked the plaintext:\n%s", hidden)
	}

	revealed := object.DeepCopy()
	readableSecret(revealed)
	shown := describeLive(context.Background(), c, revealed, gvr)
	if !strings.Contains(shown, "hunter2") {
		t.Errorf("the revealed report does not show the decoded value:\n%s", shown)
	}
}
