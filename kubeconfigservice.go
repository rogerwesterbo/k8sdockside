package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/rogerwesterbo/k8sdockside/internal/appconfig"
	"github.com/rogerwesterbo/k8sdockside/internal/kube"
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
	files := kube.Discover(kube.Sources{
		Files:            s.store.ManualFiles(),
		Folders:          s.store.ManualFolders(),
		Excluded:         s.store.ExcludedFiles(),
		ExcludedContexts: s.store.ExcludedContexts(),
	})

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

// RemoveFile takes a kubeconfig out of the sidebar, whatever put it there.
//
// From the user's side this is one action -- "I do not want this file" -- but
// it has two implementations. A file they added by hand is simply forgotten. A
// file discovery found cannot be forgotten, because the next sync would find it
// again, so refusing it is recorded as an exclusion instead.
func (s *KubeconfigService) RemoveFile(path string) ([]kube.File, error) {
	if path == "" {
		return s.Files(), errors.New("no file given")
	}

	manual := false
	for _, p := range s.store.ManualFiles() {
		if p == path {
			manual = true
			break
		}
	}

	var err error
	if manual {
		_, err = s.store.RemoveManualFile(path)
	} else {
		_, err = s.store.ExcludeFile(path)
	}
	if err != nil {
		return s.Files(), err
	}
	return s.Sync(), nil
}

// RestoreFile un-hides a file that was excluded, letting discovery find it
// again on this sync.
func (s *KubeconfigService) RestoreFile(path string) ([]kube.File, error) {
	if _, err := s.store.UnexcludeFile(path); err != nil {
		return s.Files(), err
	}
	return s.Sync(), nil
}

// HideContext takes one context out of the sidebar and leaves the rest of its
// file there. The kubeconfig itself is not touched -- this app never writes
// one -- so the hiding is remembered in the app's own settings, and a rescan
// leaves the context out again.
func (s *KubeconfigService) HideContext(id string) ([]kube.File, error) {
	if id == "" {
		return s.Files(), errors.New("no context given")
	}
	if _, err := s.store.ExcludeContext(id); err != nil {
		return s.Files(), err
	}
	return s.Sync(), nil
}

// RestoreContext shows a hidden context again.
func (s *KubeconfigService) RestoreContext(id string) ([]kube.File, error) {
	if _, err := s.store.UnexcludeContext(id); err != nil {
		return s.Files(), err
	}
	return s.Sync(), nil
}

// HiddenContexts returns the ids of the contexts hidden one by one, so the
// sidebar can list them and offer to show them again.
func (s *KubeconfigService) HiddenContexts() []string {
	return s.store.ExcludedContexts()
}

// Excluded returns the files the user has hidden, so the sidebar can say how
// many a folder is holding back and offer to show them again. Hidden state that
// cannot be seen anywhere is hidden state nobody can undo.
func (s *KubeconfigService) Excluded() []string {
	return s.store.ExcludedFiles()
}

// Folders returns the directories being watched, so the sidebar can list them
// and offer to stop watching one.
func (s *KubeconfigService) Folders() []string {
	return s.store.ManualFolders()
}

// AddFolder starts watching a directory for kubeconfigs and rescans.
//
// The folder is scanned before being stored so that choosing one with nothing
// in it fails with a message, rather than being accepted and then appearing to
// have done nothing.
func (s *KubeconfigService) AddFolder(path string) ([]kube.File, error) {
	if path == "" {
		return s.Files(), errors.New("no folder selected")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return s.Files(), err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return s.Files(), err
	}
	if !info.IsDir() {
		return s.Files(), errors.New("not a folder")
	}
	if found := kube.ScanFolder(abs); len(found) == 0 {
		return s.Files(), fmt.Errorf("no kubeconfig files in %s", filepath.Base(abs))
	}
	if _, err := s.store.AddManualFolder(abs); err != nil {
		return s.Files(), err
	}
	return s.Sync(), nil
}

// RemoveFolder stops watching a directory. The configs found through it leave
// the sidebar with the rescan this triggers.
//
// Anything hidden inside that folder is forgotten with it: keeping those
// exclusions would mean re-adding the folder silently produced fewer files than
// it contains, with nothing on screen explaining why.
func (s *KubeconfigService) RemoveFolder(path string) ([]kube.File, error) {
	if _, err := s.store.RemoveManualFolder(path); err != nil {
		return s.Files(), err
	}
	if _, err := s.store.ClearExclusionsIn(path); err != nil {
		return s.Files(), err
	}
	return s.Sync(), nil
}

// BrowseForFile opens the native file picker and adds everything the user
// chose. It returns the new file list; cancelling the dialog leaves everything
// as it was and is not an error.
//
// Several files can be picked at once, and one bad choice does not discard the
// good ones: every file is tried, and the failures are reported together.
func (s *KubeconfigService) BrowseForFile() ([]kube.File, error) {
	dialog := application.Get().Dialog.OpenFile().
		SetTitle("Add kubeconfigs").
		CanChooseFiles(true).
		CanChooseDirectories(false).
		ShowHiddenFiles(true)

	if home, err := os.UserHomeDir(); err == nil {
		dialog.SetDirectory(filepath.Join(home, ".kube"))
	}

	paths, err := dialog.PromptForMultipleSelection()
	if err != nil {
		return s.Files(), err
	}
	if len(paths) == 0 {
		return s.Files(), nil // cancelled
	}
	return s.AddFiles(paths)
}

// AddFiles remembers several kubeconfig paths at once. Files that parse are
// kept even when others alongside them do not, because discarding a good
// selection over one bad file would mean picking them all again.
func (s *KubeconfigService) AddFiles(paths []string) ([]kube.File, error) {
	var failures []string
	added := 0

	for _, path := range paths {
		if _, err := s.AddFile(path); err != nil {
			failures = append(failures, filepath.Base(path)+": "+err.Error())
			continue
		}
		added++
	}

	files := s.Files()
	if len(failures) == 0 {
		return files, nil
	}
	if added == 0 {
		return files, errors.New(strings.Join(failures, "; "))
	}
	return files, fmt.Errorf("added %d of %d -- %s", added, len(paths), strings.Join(failures, "; "))
}

// BrowseForFolder opens the native picker in directory mode and watches the
// folder the user chose.
func (s *KubeconfigService) BrowseForFolder() ([]kube.File, error) {
	dialog := application.Get().Dialog.OpenFile().
		SetTitle("Add a folder of kubeconfigs").
		CanChooseFiles(false).
		CanChooseDirectories(true).
		ShowHiddenFiles(true)

	if home, err := os.UserHomeDir(); err == nil {
		dialog.SetDirectory(home)
	}

	path, err := dialog.PromptForSingleSelection()
	if err != nil {
		return s.Files(), err
	}
	if path == "" {
		return s.Files(), nil // cancelled
	}
	return s.AddFolder(path)
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
