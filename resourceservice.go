package main

import (
	"fmt"
	"strings"

	"github.com/rogerwesterbo/k8sdockside/internal/kube"
	"github.com/rogerwesterbo/k8sdockside/internal/plugins"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// SnapshotEvent is the event a tab's rows arrive on. One event carries the
// whole current contents of one subscription; the payload names which.
const SnapshotEvent = "resource:snapshot"

// Registering the event gives the binding generator the payload's type, so the
// frontend receives a typed Snapshot rather than an any. The generator only
// discovers direct calls with constant arguments from an init function.
func init() {
	application.RegisterEvent[kube.Snapshot](SnapshotEvent)
}

// ResourceService serves the cluster data behind each tab, live.
//
// Tables are not fetched, they are subscribed to: Subscribe opens a watch and
// returns immediately, and every change to the cluster arrives at the frontend
// as a SnapshotEvent. The one-shot methods below (the dashboard, the describe
// panel, the namespace filter) share the same connections.
type ResourceService struct {
	configs *KubeconfigService
	watcher *kube.Watcher
	// plugins resolves a "plugin:" tab kind back to the kind and filters the
	// view names. Set after construction because the plugin service needs this
	// service's watcher, and one of the two has to be built first.
	plugins *PluginService
	// graphs resolves where a context's Prometheus is, for the budget views'
	// usage fallback. Set after construction for the same reason: it is built
	// from this service's watcher.
	graphs *MetricsService
}

// NewResourceService wires the service to the kubeconfig cache it resolves
// context IDs against.
func NewResourceService(configs *KubeconfigService) *ResourceService {
	s := &ResourceService{configs: configs}
	s.watcher = kube.NewWatcher(s.push)
	return s
}

// usePlugins gives the service the plugin catalogue it resolves plugin views
// against. Unexported so it stays out of the generated bindings: it is wiring
// between two services in the same package, not something the frontend calls.
func (s *ResourceService) usePlugins(p *PluginService) { s.plugins = p }

// useMetrics gives the service the Prometheus endpoint resolver its budget
// views fall back to. Unexported for the same reason as usePlugins: wiring
// between two services, not something the frontend calls.
func (s *ResourceService) useMetrics(m *MetricsService) { s.graphs = m }

// push forwards one snapshot to the frontend. It is called from the watcher's
// background goroutines, which is why it tolerates being called before the app
// exists and after it has gone.
func (s *ResourceService) push(snap kube.Snapshot) {
	if app := application.Get(); app != nil {
		app.Event.Emit(SnapshotEvent, snap)
	}
}

// ServiceShutdown closes every watch when the app quits, so credentials and
// connections are not left open behind a closed window.
func (s *ResourceService) ServiceShutdown() error {
	s.watcher.Close()
	return nil
}

// Subscribe opens a live view of one resource kind and returns the subscription
// ID that its snapshots will carry. Pass an empty namespace for all namespaces.
//
// It returns as soon as the watch is started. The first rows arrive as an event
// once the cluster has answered, so a slow or unreachable cluster leaves the
// tab in its loading state rather than blocking the UI.
func (s *ResourceService) Subscribe(contextID, kind, namespace string) (string, error) {
	ctx, err := s.resolve(contextID)
	if err != nil {
		return "", err
	}

	// A plugin view is a kind like any other as far as the tab machinery is
	// concerned; it is turned back into a real kind and the filters it pins
	// here, at the last moment, so nothing between the sidebar and this line
	// has to know that plugins exist.
	kind, namespace, selector, err := s.view(kind, namespace)
	if err != nil {
		return "", err
	}
	return s.watcher.Subscribe(ctx, kind, namespace, selector)
}

// view resolves a tab's kind, which may name a plugin's view, into the kind to
// watch and the filters to apply. A kind that is not a plugin view passes
// through untouched, which is every kind but one.
//
// A view that pins a namespace overrides whatever the tab asked for, rather
// than intersecting with it: the pin is the whole point of the view, and a
// tab's namespace filter that silently did nothing would be worse than one that
// is not offered.
func (s *ResourceService) view(kind, namespace string) (string, string, string, error) {
	if !strings.HasPrefix(kind, plugins.Prefix) {
		return kind, namespace, kube.NoSelector, nil
	}
	if s.plugins == nil {
		return "", "", "", fmt.Errorf("plugins are not available")
	}

	resolved, err := s.plugins.Resolve(kind)
	if err != nil {
		return "", "", "", err
	}
	if resolved.Overview {
		return "", "", "", fmt.Errorf("%s is a plugin overview, which is not a resource listing", kind)
	}
	if resolved.Namespace != "" {
		namespace = resolved.Namespace
	}
	return resolved.Kind, namespace, resolved.Selector, nil
}

// Unsubscribe closes a tab's view. The underlying watch stays open if another
// tab is still using it.
func (s *ResourceService) Unsubscribe(subscriptionID string) {
	s.watcher.Unsubscribe(subscriptionID)
}

// SetNamespace re-points an open subscription at another namespace. The watch
// is cluster-wide, so this is a filter change: the new rows arrive as an event
// without anything being re-fetched.
func (s *ResourceService) SetNamespace(subscriptionID, namespace string) {
	s.watcher.SetNamespace(subscriptionID, namespace)
}

// Overview is the dashboard payload for one context.
func (s *ResourceService) Overview(contextID string) (kube.Overview, error) {
	ctx, err := s.resolve(contextID)
	if err != nil {
		return kube.Overview{Error: err.Error()}, err
	}
	return s.watcher.Overview(ctx)
}

// Ping reports whether a context's cluster can be reached, for the sidebar's
// connection indicator. It returns nil when the cluster answered and an error
// carrying the reason when it did not; there is no payload because the only
// question being asked is whether this works.
func (s *ResourceService) Ping(contextID string) error {
	ctx, err := s.resolve(contextID)
	if err != nil {
		return err
	}
	return s.watcher.Ping(ctx)
}

// CustomResourceKinds lists what a cluster defines, grouped by API group, for
// the definitions section of the sidebar.
func (s *ResourceService) CustomResourceKinds(contextID string) ([]kube.CustomResourceGroup, error) {
	ctx, err := s.resolve(contextID)
	if err != nil {
		return []kube.CustomResourceGroup{}, err
	}
	return s.watcher.CustomResourceKinds(ctx)
}

// Describe renders the detail report shown in the slide-in panel.
func (s *ResourceService) Describe(contextID, kind, namespace, name string) (string, error) {
	ctx, err := s.resolve(contextID)
	if err != nil {
		return "", err
	}
	return s.watcher.Describe(ctx, kind, namespace, name)
}

// ResourceYAML returns one object as the YAML the editor opens with. It is a
// live read rather than the informer's copy: the cache drops managed fields and
// redacts secret values, and an editor must open on the object rather than on
// the table's view of it.
func (s *ResourceService) ResourceYAML(contextID, kind, namespace, name string) (string, error) {
	ctx, err := s.resolve(contextID)
	if err != nil {
		return "", err
	}
	return s.watcher.ResourceYAML(ctx, kind, namespace, name)
}

// ApplyYAML writes an edited object back to the cluster and returns it as the
// server left it -- with the resourceVersion the next save will be checked
// against, and whatever defaulting and admission control did to it on the way
// in. The editor replaces its contents with the result, which is what makes a
// second save work rather than fail as a conflict.
func (s *ResourceService) ApplyYAML(contextID, kind, namespace, name, yaml string) (string, error) {
	ctx, err := s.resolve(contextID)
	if err != nil {
		return "", err
	}
	return s.watcher.ApplyYAML(ctx, kind, namespace, name, yaml)
}

// CheckYAML reports whether what is in the editor is still YAML. It touches no
// cluster: it is called as the user types, and the only question it answers is
// whether the document parses.
func (s *ResourceService) CheckYAML(yaml string) kube.YAMLCheck {
	return kube.ValidateYAML(yaml)
}

// Namespaces lists the namespaces available for the namespace filter.
func (s *ResourceService) Namespaces(contextID string) ([]string, error) {
	ctx, err := s.resolve(contextID)
	if err != nil {
		return []string{}, err
	}
	return s.watcher.Namespaces(ctx)
}

func (s *ResourceService) resolve(contextID string) (kube.Context, error) {
	ctx, ok := s.configs.lookup(contextID)
	if !ok {
		return kube.Context{}, fmt.Errorf("unknown context %q -- it may have been removed from the kubeconfig", contextID)
	}
	return ctx, nil
}
