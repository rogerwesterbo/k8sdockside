package kube

import (
	"context"
	"fmt"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
)

// The reads a plugin's own views make. Unlike a table, a custom view wants the
// objects themselves -- an Application's status.resources, a VM's
// printableStatus -- so these hand back whole objects rather than projected
// rows. Which kinds a view may ask for is decided by the plugin layer, not
// here.

// Objects lists one kind as plain objects. namespace and selector narrow it
// exactly as they do a plugin's table view; either may be empty.
func (w *Watcher) Objects(kc Context, kind, namespace, selector string) ([]map[string]any, error) {
	chosen, err := labels.Parse(selector)
	if err != nil {
		return nil, fmt.Errorf("label selector %q: %w", selector, err)
	}

	out := []map[string]any{}
	err = w.withClient(kc, func(c *clusterClient) error {
		ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
		defer cancel()

		items, _, err := c.list(ctx, kind, metav1.ListOptions{LabelSelector: chosen.String()})
		if err != nil {
			return err
		}
		for i := range items {
			if namespace != AllNamespaces && items[i].GetNamespace() != namespace {
				continue
			}
			out = append(out, trimmed(&items[i]))
		}
		return nil
	})
	return out, err
}

// Object reads one object, live.
func (w *Watcher) Object(kc Context, kind, namespace, name string) (map[string]any, error) {
	var out map[string]any
	err := w.withClient(kc, func(c *clusterClient) error {
		ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
		defer cancel()

		u, _, err := c.get(ctx, kind, namespace, name)
		if err != nil {
			return err
		}
		out = trimmed(u)
		return nil
	})
	return out, err
}

// ----- what a plugin's actions write ----------------------------------------
//
// Which requests a plugin may make, and on what, is decided by the plugin
// layer from its manifest. These only make them.

// CallSubresource makes one request to a verb the API serves beside an object
// rather than as a write to it: KubeVirt's pause, restart and migrate. path is
// absolute, and method is PUT or POST.
func (w *Watcher) CallSubresource(kc Context, method, path string, body []byte) error {
	return w.withClient(kc, func(c *clusterClient) error {
		ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
		defer cancel()

		req := c.typed.CoreV1().RESTClient().Verb(method).AbsPath(path)
		if len(body) > 0 {
			req = req.SetHeader("Content-Type", "application/json").Body(body)
		}
		err := req.Do(ctx).Error()
		if err == nil {
			return nil
		}
		text := err.Error()
		switch {
		case strings.Contains(text, "the server could not find the requested resource"):
			return fmt.Errorf("this cluster does not serve %s -- the API that answers it is not installed or not running", path)
		case strings.Contains(text, "orbidden"):
			return fmt.Errorf("not allowed to call %s -- a subresource needs its own permission, separate from write access to the object: %w", path, err)
		}
		return err
	})
}

// Create makes one object of a kind in a namespace and returns the name it was
// given, which for an object asking for a generateName is the only way to know
// it.
func (w *Watcher) Create(kc Context, kind, namespace string, object map[string]any) (string, error) {
	var name string
	err := w.withClient(kc, func(c *clusterClient) error {
		ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
		defer cancel()

		mapping, err := c.mappingForKind(kind)
		if err != nil {
			return err
		}
		created, err := resourceFor(c.dynamic, mapping, namespace).Create(ctx, &unstructured.Unstructured{Object: object}, metav1.CreateOptions{})
		if err != nil {
			return err
		}
		name = created.GetName()
		return nil
	})
	return name, err
}

// trimmed drops what no view wants and every object carries: the server's
// field-ownership bookkeeping, and kubectl's copy of the whole object in an
// annotation. Together they are often most of the bytes.
func trimmed(u *unstructured.Unstructured) map[string]any {
	u.SetManagedFields(nil)
	if annotations := u.GetAnnotations(); len(annotations) > 0 {
		u.SetAnnotations(stripLastApplied(annotations))
	}
	return u.Object
}
