package main

import (
	"fmt"

	"github.com/roger/k8sdockside/internal/kube"
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
}

// NewResourceService wires the service to the kubeconfig cache it resolves
// context IDs against.
func NewResourceService(configs *KubeconfigService) *ResourceService {
	s := &ResourceService{configs: configs}
	s.watcher = kube.NewWatcher(s.push)
	return s
}

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
	return s.watcher.Subscribe(ctx, kind, namespace)
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

// HelmReleases lists the Helm releases installed in a cluster.
//
// It is a plain call rather than a subscription because Helm releases are not a
// watchable resource: they are Secrets whose payload has to be decoded, and
// that payload is exactly what must not be kept in the informer cache. The
// table is built and the payload dropped in the same breath.
func (s *ResourceService) HelmReleases(contextID, namespace string) (kube.Table, error) {
	ctx, err := s.resolve(contextID)
	if err != nil {
		return kube.Table{}, err
	}
	return s.watcher.HelmReleases(ctx, namespace)
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
