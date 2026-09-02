package main

import (
	"errors"
	"os"
	"path/filepath"
	"sync"

	"github.com/roger/k8sdockside/internal/appconfig"
	"github.com/roger/k8sdockside/internal/kube"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// KubeconfigService is the frontend's view of the kubeconfig files on disk. It
// caches the last scan so that opening tabs and rendering the sidebar does not
// re-read every file, and rescans only when the user asks (Sync) or changes the
// set of files.
type KubeconfigService struct {
	store *appconfig.Store

	mu    sync.RWMutex
	files []kube.File
	index map[string]kube.Context
}

// NewKubeconfigService wires the service to the settings store, which owns the
// list of kubeconfig paths the user added by hand.
func NewKubeconfigService(store *appconfig.Store) *KubeconfigService {
	return &KubeconfigService{store: store, files: []kube.File{}, index: map[string]kube.Context{}}
}

// Sync rescans every source -- ~/.kube/config, $KUBECONFIG, the user's own
// paths, and the rest of ~/.kube -- and returns what it found.
func (s *KubeconfigService) Sync() []kube.File {
	files := kube.Discover(s.store.ManualFiles())

	index := make(map[string]kube.Context)
	for _, f := range files {
		for _, c := range f.Contexts {
			index[c.ID] = c
		}
	}

	s.mu.Lock()
	s.files, s.index = files, index
	s.mu.Unlock()
	return files
}

// Files returns the last scan, running one first if the app has just started.
func (s *KubeconfigService) Files() []kube.File {
	s.mu.RLock()
	cached := s.files
	s.mu.RUnlock()

	if cached == nil {
		return s.Sync()
	}
	return cached
}

// Contexts flattens the last scan into a single list, which is what the sidebar
// actually renders.
func (s *KubeconfigService) Contexts() []kube.Context {
	out := []kube.Context{}
	for _, f := range s.Files() {
		out = append(out, f.Contexts...)
	}
	return out
}

// AddFile remembers a kubeconfig path chosen by the user and rescans. The file
// is parsed before being stored so that picking something that is not a
// kubeconfig fails immediately, with a message, instead of quietly adding a
// broken entry to the sidebar.
func (s *KubeconfigService) AddFile(path string) ([]kube.File, error) {
	if path == "" {
		return s.Files(), errors.New("no file selected")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return s.Files(), err
	}
	if parsed := kube.ParseFile(abs, kube.SourceManual); parsed.Error != "" {
		return s.Files(), errors.New(parsed.Error)
	}
	if _, err := s.store.AddManualFile(abs); err != nil {
		return s.Files(), err
	}
	return s.Sync(), nil
}

// RemoveFile forgets a user-added kubeconfig. Auto-discovered files cannot be
// removed -- the next sync would find them again -- so that is reported as an
// error rather than appearing to work.
func (s *KubeconfigService) RemoveFile(path string) ([]kube.File, error) {
	for _, f := range s.Files() {
		if f.Path == path && f.Source != kube.SourceManual {
			return s.Files(), errors.New("this file was discovered automatically and cannot be removed")
		}
	}
	if _, err := s.store.RemoveManualFile(path); err != nil {
		return s.Files(), err
	}
	return s.Sync(), nil
}

// BrowseForFile opens the native file picker and adds whatever the user chose.
// It returns the new file list; cancelling the dialog leaves everything as it
// was and is not an error.
func (s *KubeconfigService) BrowseForFile() ([]kube.File, error) {
	dialog := application.Get().Dialog.OpenFile().
		SetTitle("Add kubeconfig").
		CanChooseFiles(true).
		CanChooseDirectories(false).
		ShowHiddenFiles(true)

	if home, err := os.UserHomeDir(); err == nil {
		dialog.SetDirectory(filepath.Join(home, ".kube"))
	}

	path, err := dialog.PromptForSingleSelection()
	if err != nil {
		return s.Files(), err
	}
	if path == "" {
		return s.Files(), nil // cancelled
	}
	return s.AddFile(path)
}

// lookup resolves a context ID against the last scan. It is unexported so it
// stays out of the generated bindings; only the other services use it.
func (s *KubeconfigService) lookup(id string) (kube.Context, bool) {
	s.mu.RLock()
	ctx, ok := s.index[id]
	s.mu.RUnlock()
	if ok {
		return ctx, true
	}

	// The context may come from a file added since the last scan (or the app
	// may have just started), so it is worth one rescan before giving up.
	s.Sync()
	s.mu.RLock()
	defer s.mu.RUnlock()
	ctx, ok = s.index[id]
	return ctx, ok
}
