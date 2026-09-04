package kube

// Editing an object: read it back as YAML, tell the editor whether what is in
// it is still YAML, and write it to the cluster.
//
// The read and the write both go through the dynamic client rather than the
// informer cache, and that is deliberate on both counts. The cache is a
// projection -- managed fields are dropped and secret values are redacted on
// the way in -- so editing what it holds would offer the user a document that
// is not the object. An edit has to start from what the API server currently
// says and end at what it accepts.

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utiljson "k8s.io/apimachinery/pkg/util/json"
	"k8s.io/client-go/dynamic"
	sigsyaml "sigs.k8s.io/yaml"
)

// YAMLCheck is the answer to the only question the editor asks while you type:
// is this still YAML, and if not, where did it stop being YAML?
//
// Line is 1-based and 0 when the parser named none, so the gutter can mark the
// offending row without the frontend having to read error prose.
type YAMLCheck struct {
	Valid   bool   `json:"valid"`
	Message string `json:"message"`
	Line    int    `json:"line"`
}

// ValidateYAML reports whether text parses as a single YAML mapping.
//
// It is deliberately about syntax and shape rather than about Kubernetes: this
// runs on every keystroke, and half-written documents are the normal state of
// an editor. Whether the object is one this cluster will accept is the API
// server's answer to give, and it is asked at save time.
func ValidateYAML(text string) YAMLCheck {
	if strings.TrimSpace(text) == "" {
		return YAMLCheck{Message: "the document is empty"}
	}

	// Decoded into a map rather than a yaml.Node: a node accepts anything the
	// parser can read, including a list or a bare scalar, and duplicate keys --
	// none of which is an object. The map is what catches all four.
	var out map[string]any
	if err := yaml.Unmarshal([]byte(text), &out); err != nil {
		message, line := explainYAML(err)
		return YAMLCheck{Message: message, Line: line}
	}
	if len(out) == 0 {
		return YAMLCheck{Message: "the document is empty"}
	}
	return YAMLCheck{Valid: true}
}

// yamlLine pulls the line number out of a go-yaml message. Both the parser's
// own errors ("yaml: line 3: ...") and its type errors ("  line 3: ...") carry
// it there and nowhere else -- the package exposes no structured position.
var yamlLine = regexp.MustCompile(`line (\d+): `)

// explainYAML turns a go-yaml error into one line of prose and the line it
// happened on.
//
// The two shapes it has to flatten are the parser's single-line error and a
// yaml.TypeError, which is a multi-line list. Only the first entry of a list is
// kept: they are usually consequences of each other, and the editor shows one
// message beside one marked row.
func explainYAML(err error) (string, int) {
	message := err.Error()
	if typed, ok := err.(*yaml.TypeError); ok && len(typed.Errors) > 0 {
		message = typed.Errors[0]
	}
	message = strings.TrimSpace(strings.TrimPrefix(message, "yaml:"))

	line := 0
	if m := yamlLine.FindStringSubmatch(message); m != nil {
		// The parse cannot fail: the pattern matched a run of digits.
		line, _ = strconv.Atoi(m[1])
		message = strings.TrimSpace(message[len(m[0]):])
	}
	return strings.TrimSpace(message), line
}

// ResourceYAML returns one live object as the YAML the editor opens with.
func (w *Watcher) ResourceYAML(kc Context, kind, namespace, name string) (string, error) {
	var out string
	err := w.withClient(kc, func(c *clusterClient) error {
		ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
		defer cancel()

		got, _, err := c.get(ctx, kind, namespace, name)
		if err != nil {
			return err
		}
		out, err = toYAML(forEditing(got))
		return err
	})
	return out, err
}

// ApplyYAML writes the edited document back to the cluster and returns the
// object as the server left it.
//
// The result is what the editor then holds, rather than the text that was sent:
// a successful update comes back with a new resourceVersion and whatever
// defaulting or admission control did to the object on the way in. Keeping the
// sent text would leave the editor holding a stale version, and the next save
// would be rejected as a conflict against an object nobody else had touched.
func (w *Watcher) ApplyYAML(kc Context, kind, namespace, name, text string) (string, error) {
	var out string
	err := w.withClient(kc, func(c *clusterClient) error {
		ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
		defer cancel()

		mapping, err := c.mappingForKind(kind)
		if err != nil {
			return err
		}

		obj, err := parseObject(text)
		if err != nil {
			return err
		}
		if err := sameObject(obj, mapping.GroupVersionKind, namespace, name); err != nil {
			return err
		}

		saved, err := resourceFor(c.dynamic, mapping, namespace).Update(ctx, obj, metav1.UpdateOptions{})
		if err != nil {
			return err
		}
		out, err = toYAML(forEditing(saved))
		return err
	})
	return out, err
}

// get reads one object, whether or not its kind is namespaced. It reports the
// mapping alongside, because the caller that wants an object usually also wants
// to know what it turned out to be.
func (c *clusterClient) get(ctx context.Context, kind, namespace, name string) (*unstructured.Unstructured, *meta.RESTMapping, error) {
	mapping, err := c.mappingForKind(kind)
	if err != nil {
		return nil, nil, err
	}
	got, err := resourceFor(c.dynamic, mapping, namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, nil, err
	}
	return got, mapping, nil
}

// resourceFor narrows a client to the collection an object lives in: the
// namespace for a namespaced kind, the cluster for one that is not.
func resourceFor(client dynamic.Interface, mapping *meta.RESTMapping, namespace string) dynamic.ResourceInterface {
	ri := client.Resource(mapping.Resource)
	if mapping.Scope.Name() == meta.RESTScopeNameNamespace {
		return ri.Namespace(namespace)
	}
	return ri
}

// parseObject turns editor text into the object to send.
//
// It goes through JSON rather than decoding YAML straight into a map, because
// an unstructured object may only hold the types JSON has: go-yaml would give
// back plain ints, which client-go's deep copy panics on rather than sends.
func parseObject(text string) (*unstructured.Unstructured, error) {
	if check := ValidateYAML(text); !check.Valid {
		if check.Line > 0 {
			return nil, fmt.Errorf("line %d: %s", check.Line, check.Message)
		}
		return nil, fmt.Errorf("%s", check.Message)
	}
	if documents(text) > 1 {
		return nil, fmt.Errorf("this edits one object, so the document may not be split with ---")
	}

	raw, err := sigsyaml.YAMLToJSON([]byte(text))
	if err != nil {
		message, line := explainYAML(err)
		if line > 0 {
			return nil, fmt.Errorf("line %d: %s", line, message)
		}
		return nil, fmt.Errorf("%s", message)
	}

	// apimachinery's decoder, not encoding/json: it settles whole numbers as
	// int64 rather than float64, which is the difference between a replica
	// count that round-trips and one that is sent as 3e+00.
	fields := map[string]any{}
	if err := utiljson.Unmarshal(raw, &fields); err != nil {
		return nil, err
	}
	return &unstructured.Unstructured{Object: fields}, nil
}

// documents counts the YAML documents in text. go-yaml's Unmarshal silently
// decodes only the first, so a second one would otherwise be dropped without
// anything saying so.
func documents(text string) int {
	decoder := yaml.NewDecoder(strings.NewReader(text))
	count := 0
	for {
		var node yaml.Node
		if err := decoder.Decode(&node); err != nil {
			return count
		}
		count++
	}
}

// sameObject refuses an edit that has changed what the object *is* rather than
// what it says.
//
// Renaming or re-kinding in the editor cannot do what it looks like it does:
// the request goes to the URL of the object that was opened, so a changed name
// is either rejected by the API server or -- worse to read afterwards -- would
// be a write to the object you were looking at under a name you thought you
// were creating. It is refused here so the message says which field is the
// problem.
func sameObject(obj *unstructured.Unstructured, gvk schema.GroupVersionKind, namespace, name string) error {
	if got := obj.GroupVersionKind(); got != gvk {
		return fmt.Errorf(
			"apiVersion and kind may not be changed here: this is %s %s, not %s %s",
			gvk.GroupVersion(), gvk.Kind, got.GroupVersion(), got.Kind,
		)
	}
	if got := obj.GetName(); got != name {
		return fmt.Errorf("the name may not be changed here: this object is %q, not %q", name, got)
	}
	// An absent namespace is the object's own, which is what a cluster-scoped
	// kind always has and what a namespaced document is free to leave out.
	if got := obj.GetNamespace(); got != "" && got != namespace {
		return fmt.Errorf("the namespace may not be changed here: this object is in %q, not %q", namespace, got)
	}
	return nil
}

// forEditing drops the parts of an object that are the API server's
// bookkeeping rather than anything a person edits: the managed-field ledger,
// which is longer than most specs, and kubectl's copy of the whole object.
//
// What it deliberately keeps is resourceVersion. That is what makes a save
// fail rather than silently overwrite when someone -- or a controller -- has
// changed the object since it was opened.
func forEditing(u *unstructured.Unstructured) *unstructured.Unstructured {
	out := u.DeepCopy()
	out.SetManagedFields(nil)
	if annotations := stripLastApplied(out.GetAnnotations()); len(annotations) > 0 {
		out.SetAnnotations(annotations)
	} else {
		out.SetAnnotations(nil)
	}
	return out
}

// toYAML renders an object the way kubectl does: through JSON, so that the
// field names are the API's own and the keys come out in a stable order.
func toYAML(u *unstructured.Unstructured) (string, error) {
	raw, err := sigsyaml.Marshal(u.Object)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}
