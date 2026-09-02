package main

import (
	"fmt"

	"github.com/roger/k8sdockside/internal/kube"
)

// ResourceService serves the cluster data behind each tab.
//
// The data is currently fabricated (see internal/kube/stub.go) but is shaped
// exactly like the real thing and is stable per context, so the UI can be built
// against it. Wiring in client-go means changing what these methods call, not
// their signatures or the frontend.
type ResourceService struct {
	configs *KubeconfigService
}

// NewResourceService wires the service to the kubeconfig cache it resolves
// context IDs against.
func NewResourceService(configs *KubeconfigService) *ResourceService {
	return &ResourceService{configs: configs}
}

// Overview is the dashboard payload for one context.
func (s *ResourceService) Overview(contextID string) (kube.Overview, error) {
	ctx, err := s.resolve(contextID)
	if err != nil {
		return kube.Overview{Error: err.Error()}, err
	}
	return kube.BuildOverview(ctx), nil
}

// Table lists one resource kind, optionally restricted to a namespace. Pass an
// empty namespace for all namespaces.
func (s *ResourceService) Table(contextID, kind, namespace string) (kube.Table, error) {
	ctx, err := s.resolve(contextID)
	if err != nil {
		return kube.Table{Kind: kind, Columns: []string{}, Rows: []kube.Row{}, Error: err.Error()}, err
	}
	return kube.BuildTable(ctx, kind, namespace), nil
}

// Describe renders the detail report shown in the slide-in panel.
func (s *ResourceService) Describe(contextID, kind, namespace, name string) (string, error) {
	ctx, err := s.resolve(contextID)
	if err != nil {
		return "", err
	}
	return kube.BuildDescribe(ctx, kind, namespace, name), nil
}

// Namespaces lists the namespaces available for the namespace filter.
func (s *ResourceService) Namespaces(contextID string) ([]string, error) {
	ctx, err := s.resolve(contextID)
	if err != nil {
		return []string{}, err
	}
	return kube.Namespaces(ctx), nil
}

func (s *ResourceService) resolve(contextID string) (kube.Context, error) {
	ctx, ok := s.configs.lookup(contextID)
	if !ok {
		return kube.Context{}, fmt.Errorf("unknown context %q -- it may have been removed from the kubeconfig", contextID)
	}
	return ctx, nil
}
