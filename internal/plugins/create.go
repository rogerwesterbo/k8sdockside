package plugins

import (
	json "encoding/json/v2"
	"fmt"
	"strings"
)

// NewObject checks an object a plugin's own view asked to create and returns
// it ready to send: parsed, complete enough to be created, and in the
// namespace the request names.
//
// It is the create counterpart of CanWrite and applies the same rule -- a kind
// the plugin declares, with "ui": {"write": true} -- plus the one actions obey:
// nothing that is a door to more than itself (Secrets, RBAC, webhooks, CRDs)
// may be written, whatever the manifest says. The user has seen the object and
// said yes before this runs; that is the frame's job, not this.
func (p Plugin) NewObject(kind, namespace, text string) (map[string]any, error) {
	if !p.CanRead(kind) {
		return nil, fmt.Errorf("the %s plugin does not declare %q, so its views cannot create one", p.Name, kind)
	}
	if !p.CanWrite(kind) {
		return nil, fmt.Errorf("the %s plugin does not declare \"ui\": {\"write\": true}, so its views cannot change anything", p.Name)
	}
	if !writable(kind) {
		return nil, fmt.Errorf("a plugin's views may not create %s", kind)
	}

	var object map[string]any
	if err := json.Unmarshal([]byte(text), &object); err != nil {
		return nil, fmt.Errorf("the object to create is not a JSON object: %w", err)
	}
	if object == nil {
		return nil, fmt.Errorf("the object to create is empty")
	}
	for _, field := range []string{"apiVersion", "kind"} {
		if s, _ := object[field].(string); strings.TrimSpace(s) == "" {
			return nil, fmt.Errorf("the object to create has no %s", field)
		}
	}
	meta, _ := object["metadata"].(map[string]any)
	if meta == nil {
		return nil, fmt.Errorf("the object to create has no metadata")
	}
	name, _ := meta["name"].(string)
	generate, _ := meta["generateName"].(string)
	if name == "" && generate == "" {
		return nil, fmt.Errorf("the object to create has neither metadata.name nor metadata.generateName")
	}
	// The namespace is the request's, whatever the object says, so what the
	// user was asked about is where it lands.
	if namespace != "" {
		meta["namespace"] = namespace
	} else {
		delete(meta, "namespace")
	}
	// Fields only the server sets; a page copying an existing object would
	// otherwise have the create refused for them.
	for _, field := range []string{"resourceVersion", "uid", "creationTimestamp", "managedFields", "generation"} {
		delete(meta, field)
	}
	delete(object, "status")
	return object, nil
}
