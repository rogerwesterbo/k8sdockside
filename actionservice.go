package main

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/rogerwesterbo/k8sdockside/internal/kube"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// DrainEvent is the event a drain's progress arrives on. One event is one
// report about one drain; the payload names which.
const DrainEvent = "node:drain"

// Registering the event gives the binding generator the payload's type, the
// same way SnapshotEvent does in resourceservice.go.
func init() {
	application.RegisterEvent[kube.DrainProgress](DrainEvent)
}

// ActionService performs the operations that change a cluster from a button:
// deleting an object, scaling or restarting a workload, cordoning or draining a
// node.
//
// It is a separate service from ResourceService because the two answer
// different questions -- one serves what a cluster contains, this one changes
// it -- but they share a watcher, and so a client pool. A context already open
// in a tab costs no second connection and no second credential exec when you
// act on something in it.
type ActionService struct {
	configs *KubeconfigService
	watcher *kube.Watcher

	// Drains in flight, by ID, so one can be called off. A drain is the only
	// action that outlives its call: everything else is one request.
	mu     sync.Mutex
	drains map[string]context.CancelFunc
	nextID atomic.Uint64
}

// NewActionService wires the service to the kubeconfig cache it resolves
// context IDs against, and to the watcher whose clients it borrows.
func NewActionService(configs *KubeconfigService, watcher *kube.Watcher) *ActionService {
	return &ActionService{
		configs: configs,
		watcher: watcher,
		drains:  map[string]context.CancelFunc{},
	}
}

// ServiceShutdown calls off every drain still running when the app quits. A
// drain left going would go on evicting against a window that has gone.
func (s *ActionService) ServiceShutdown() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, cancel := range s.drains {
		cancel()
		delete(s.drains, id)
	}
	return nil
}

// ObjectState reads the few facts the action bar needs about one object: what
// its buttons should say, and what they should start from.
func (s *ActionService) ObjectState(contextID, kind, namespace, name string) (kube.ObjectState, error) {
	kc, err := s.resolve(contextID)
	if err != nil {
		return kube.ObjectState{}, err
	}
	return s.watcher.ObjectState(kc, kind, namespace, name)
}

// Delete removes one object from its cluster.
func (s *ActionService) Delete(contextID, kind, namespace, name string) error {
	kc, err := s.resolve(contextID)
	if err != nil {
		return err
	}
	return s.watcher.Delete(kc, kind, namespace, name)
}

// Scale sets a workload's replica count.
func (s *ActionService) Scale(contextID, kind, namespace, name string, replicas int32) error {
	kc, err := s.resolve(contextID)
	if err != nil {
		return err
	}
	return s.watcher.Scale(kc, kind, namespace, name, replicas)
}

// Restart rolls a workload, the way `kubectl rollout restart` does.
func (s *ActionService) Restart(contextID, kind, namespace, name string) error {
	kc, err := s.resolve(contextID)
	if err != nil {
		return err
	}
	return s.watcher.Restart(kc, kind, namespace, name)
}

// Cordon closes a node to new work, or reopens it.
func (s *ActionService) Cordon(contextID, name string, on bool) error {
	kc, err := s.resolve(contextID)
	if err != nil {
		return err
	}
	return s.watcher.Cordon(kc, name, on)
}

// Drain starts moving everything off a node and returns the ID its progress
// will arrive under.
//
// It returns as soon as the drain is under way. A drain takes minutes -- it
// waits on disruption budgets, which is the point of using the eviction API --
// so it reports through events rather than making the window wait.
func (s *ActionService) Drain(contextID, node string) (string, error) {
	kc, err := s.resolve(contextID)
	if err != nil {
		return "", err
	}

	id := fmt.Sprintf("drain-%d", s.nextID.Add(1))
	ctx, cancel := context.WithCancel(context.Background())

	s.mu.Lock()
	s.drains[id] = cancel
	s.mu.Unlock()

	go func() {
		defer s.finished(id)
		// The error is reported through the progress events, which carry it to
		// the panel that asked; there is nobody here to return it to.
		_ = s.watcher.Drain(ctx, kc, id, node, s.push)
	}()

	return id, nil
}

// CancelDrain calls off a drain in flight. The node stays cordoned: it is half
// emptied, and quietly letting work back onto it is not what stopping meant.
func (s *ActionService) CancelDrain(drainID string) {
	s.mu.Lock()
	cancel, found := s.drains[drainID]
	s.mu.Unlock()
	if found {
		cancel()
	}
}

// finished drops a drain's cancel once it has ended, however it ended.
func (s *ActionService) finished(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if cancel, found := s.drains[id]; found {
		cancel()
		delete(s.drains, id)
	}
}

// push forwards one drain report to the frontend. Called from the drain's own
// goroutine, so it tolerates the app being gone.
func (s *ActionService) push(progress kube.DrainProgress) {
	if app := application.Get(); app != nil {
		app.Event.Emit(DrainEvent, progress)
	}
}

func (s *ActionService) resolve(contextID string) (kube.Context, error) {
	ctx, ok := s.configs.lookup(contextID)
	if !ok {
		return kube.Context{}, fmt.Errorf("unknown context %q -- it may have been removed from the kubeconfig", contextID)
	}
	return ctx, nil
}
