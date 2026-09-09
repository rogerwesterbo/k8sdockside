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
	"encoding/base64"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"gopkg.in/yaml.v3"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
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
//
// opened is the document the editor started from, and is what makes a rejected
// save recoverable: with it, a conflict can be told apart from a collision --
// see replay. Passing it empty is allowed and simply gives up the retry, which
// is the behaviour this had before.
func (w *Watcher) ApplyYAML(kc Context, kind, namespace, name, text, opened string) (string, error) {
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

		client := resourceFor(c.dynamic, mapping, namespace)
		saved, err := client.Update(ctx, obj, metav1.UpdateOptions{})
		if apierrors.IsConflict(err) && opened != "" {
			saved, err = w.replay(ctx, client, kind, namespace, name, c, obj, opened)
		}
		if err != nil {
			return err
		}
		out, err = toYAML(forEditing(saved))
		return err
	})
	return out, err
}

// replay sends a rejected edit again against the object as the cluster now has
// it, when nothing the edit touched has moved underneath it.
//
// This is what stops a controller from making its own resources uneditable. An
// object with something reconciling it gets a new resourceVersion every few
// seconds -- a status condition, an observedGeneration -- so by the time anyone
// has finished typing, the version the editor opened with is old and the save
// is refused. Nothing about that refusal concerns the person editing: they
// changed spec, the controller changed status.
//
// So the object is read again and the edit is compared against what actually
// changed. Where the two are disjoint the edit is replayed onto the current
// object, which is the save the user asked for. Where they overlap it is
// refused, and the message names the fields rather than talking about versions:
// a genuine second editor is exactly what the resourceVersion guard is for, and
// this keeps it.
func (w *Watcher) replay(
	ctx context.Context,
	client dynamic.ResourceInterface,
	kind, namespace, name string,
	c *clusterClient,
	edited *unstructured.Unstructured,
	opened string,
) (*unstructured.Unstructured, error) {
	before, err := parseObject(opened)
	if err != nil {
		// The editor sent something that is not the document it opened with.
		// Nothing can be worked out from it, so the conflict stands.
		return nil, conflictError(name)
	}

	current, _, err := c.get(ctx, kind, namespace, name)
	if err != nil {
		return nil, err
	}
	fresh := forEditing(current)

	mine := diffFields(before.Object, edited.Object)
	// The cluster's own bookkeeping is not a change anyone has to defend
	// against, and on a live object it is usually all that moved.
	theirs := withoutFields(diffFields(before.Object, fresh.Object), serverFields)

	if clashes := conflictingPaths(mine, theirs); len(clashes) > 0 {
		return nil, fmt.Errorf(
			"%s was changed in the cluster while you were editing it, in the same place you changed: %s. "+
				"Reload it to see what it says now",
			name, listPaths(clashes),
		)
	}

	// Onto the current object rather than the edited one, so the save carries
	// the cluster's resourceVersion and whatever else moved while typing.
	//
	// Onto `fresh` specifically, not the raw object it came from: the edit was
	// made against a document that had been through forEditing, so that is the
	// only shape it can be replayed onto. Against the raw object a Secret would
	// go quietly wrong -- the edit speaks of stringData, the raw object holds
	// data, and removing a key would land on a field that is not there while
	// the base64 it was meant to remove stayed put.
	//
	// Copied because `theirs` still holds references into fresh.
	merged := fresh.DeepCopy()
	applyFields(merged.Object, mine)
	return client.Update(ctx, merged, metav1.UpdateOptions{})
}

// conflictError is the plain version, for when a retry could not be attempted.
func conflictError(name string) error {
	return fmt.Errorf("%s was changed in the cluster while you were editing it. Reload it to see what it says now", name)
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
	readableSecret(out)
	return out
}

// readableSecret rewrites a Secret's base64 payload into the plaintext field
// Kubernetes already has for writing one.
//
// A Secret read back from the API server carries `data`, whose values are
// base64. That is what the object is, and it is also unreadable and worse than
// unreadable to edit: changing one character of a password means decoding it by
// hand, editing, re-encoding, and pasting it back, with no way to see whether
// you got it right until something fails to start.
//
// So the decodable entries are moved to `stringData` in plain text. This is not
// a display trick invented here -- `stringData` is a real field, write-only by
// design, and the API server base64s it back into `data` on the way in. That
// matters more than legibility: the document stays a valid Secret that can be
// copied out and applied with kubectl, and a save round-trips through the
// server's own conversion rather than through an encoder of ours.
//
// Entries that are not text are left in `data` exactly as they were. A TLS
// keystore or a binary token has no plaintext form to offer, and turning one
// into a string would corrupt it on the way back.
func readableSecret(u *unstructured.Unstructured) {
	if u.GetKind() != "Secret" || u.GetAPIVersion() != "v1" {
		return
	}
	data, found, err := unstructured.NestedMap(u.Object, "data")
	if err != nil || !found || len(data) == 0 {
		return
	}

	text := map[string]any{}
	binary := map[string]any{}
	for key, value := range data {
		encoded, ok := value.(string)
		if !ok {
			binary[key] = value
			continue
		}
		decoded, err := base64.StdEncoding.DecodeString(encoded)
		// Not text is not a failure: it is a keystore, a DER certificate, a
		// gzipped blob. It stays base64 because that is the only honest way to
		// show it.
		if err != nil || !utf8.Valid(decoded) {
			binary[key] = value
			continue
		}
		text[key] = string(decoded)
	}

	if len(text) == 0 {
		return
	}
	if len(binary) > 0 {
		_ = unstructured.SetNestedMap(u.Object, binary, "data")
	} else {
		unstructured.RemoveNestedField(u.Object, "data")
	}
	// Written after data, so a Secret that is entirely text reads as one field
	// rather than as an empty map beside a full one.
	_ = unstructured.SetNestedMap(u.Object, text, "stringData")
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
