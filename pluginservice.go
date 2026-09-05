package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/roger/k8sdockside/internal/appconfig"
	"github.com/roger/k8sdockside/internal/kube"
	"github.com/roger/k8sdockside/internal/plugins"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// PluginService is how the frontend gets the solution plugins: the ones that
// ship with the app, the ones the user has installed, and -- per cluster --
// whether the thing each one describes is actually in front of you.
//
// It mirrors ThemeService deliberately. The two extension points are installed
// the same way, fail the same way, and are managed from the same shape of
// settings section, because they are the same promise made twice: drop a JSON
// file in a folder and the app knows about your thing.
//
// Unlike themes, the catalogue is cached. A theme is read when the settings
// view asks; a plugin is read every time a tab is opened, because a tab's kind
// has to be resolved back to the view it names -- see Resolve.
type PluginService struct {
	store   *appconfig.Store
	watcher *kube.Watcher
	configs *KubeconfigService

	mu     sync.RWMutex
	cached *plugins.Catalogue
}

// NewPluginService wires the service to the settings store, and to the cluster
// connections the overview's counts come from.
func NewPluginService(store *appconfig.Store, configs *KubeconfigService, watcher *kube.Watcher) *PluginService {
	return &PluginService{store: store, configs: configs, watcher: watcher}
}

// List returns every plugin available right now, with anything that failed to
// load and why.
func (s *PluginService) List() plugins.Catalogue {
	return s.catalogue()
}

// catalogue reads the plugin folders, or hands back what was read last time.
//
// The cache exists because Resolve is on the path of every tab open and every
// snapshot subscription, and re-reading a directory to answer "what does
// plugin:argocd/applications mean" would put a filesystem walk behind an
// interaction that should be instant. Everything that could change the answer
// -- reloading, adding a folder, dropping one -- clears it explicitly, so it is
// never stale in a way the user did not ask for.
func (s *PluginService) catalogue() plugins.Catalogue {
	s.mu.RLock()
	cached := s.cached
	s.mu.RUnlock()
	if cached != nil {
		return *cached
	}

	loaded := plugins.Load(s.store.PluginsDir(), s.store.PluginFolders())
	s.mu.Lock()
	s.cached = &loaded
	s.mu.Unlock()
	return loaded
}

// forget drops the cache, so the next read goes back to disk.
func (s *PluginService) forget() {
	s.mu.Lock()
	s.cached = nil
	s.mu.Unlock()
}

// Reload rereads the plugin folders, picking up a file edited since launch.
func (s *PluginService) Reload() plugins.Catalogue {
	s.forget()
	return s.catalogue()
}

// Resolve turns a "plugin:" tab kind into the underlying kind to list and the
// filters the view pins, so the rest of the app can go on treating a tab as a
// kind and a namespace.
//
// It is exported for the frontend as well as used by ResourceService: the tab
// needs to know whether its namespace is fixed before it draws the filter.
func (s *PluginService) Resolve(kind string) (plugins.Resolved, error) {
	return s.catalogue().ResolveKind(kind)
}

// Summary is the plugin's overview for one context: whether this cluster serves
// what the plugin needs, and a live count of what it manages.
func (s *PluginService) Summary(contextID, pluginID string) (plugins.Summary, error) {
	plugin, ok := s.catalogue().Find(pluginID)
	if !ok {
		return plugins.Summary{PluginID: pluginID}, fmt.Errorf("no plugin called %q is installed", pluginID)
	}
	ctx, ok := s.configs.lookup(contextID)
	if !ok {
		return plugins.Summary{PluginID: pluginID}, fmt.Errorf("unknown context %q -- it may have been removed from the kubeconfig", contextID)
	}
	return plugins.Summarise(plugin, &clusterFor{watcher: s.watcher, ctx: ctx}), nil
}

// clusterFor adapts the watcher to the narrow interface the summary builder
// wants, so that the wording-and-ordering half of the overview can be tested
// without a cluster.
type clusterFor struct {
	watcher *kube.Watcher
	ctx     kube.Context
}

func (c *clusterFor) KindServed(kind string) (bool, error) {
	return c.watcher.KindServed(c.ctx, kind)
}

func (c *clusterFor) CountBy(kind, namespace, selector string, path kube.FieldPath) (kube.Tally, error) {
	return c.watcher.CountBy(c.ctx, kind, namespace, selector, path)
}

// Dir is the folder user plugins are read from by default.
func (s *PluginService) Dir() string {
	return s.store.PluginsDir()
}

// RevealDir opens the plugins folder in the platform's file manager, creating
// it first if it has never been used. The path comes from the store rather than
// the frontend, so nothing the webview says can decide what gets opened.
func (s *PluginService) RevealDir() error {
	dir := s.store.PluginsDir()
	if err := plugins.EnsureDir(dir); err != nil {
		return err
	}
	return application.Get().Env.OpenFileManager(dir, false)
}

// CreateExample writes a starter plugin into the plugins folder and returns the
// path it wrote.
func (s *PluginService) CreateExample() (string, error) {
	path, err := plugins.WriteExample(s.store.PluginsDir())
	if err != nil {
		return "", err
	}
	s.forget()
	return path, nil
}

// AddFolder starts reading plugins from another directory.
func (s *PluginService) AddFolder(path string) (plugins.Catalogue, error) {
	path = filepath.Clean(path)
	info, err := os.Stat(path)
	if err != nil {
		return s.catalogue(), err
	}
	if !info.IsDir() {
		return s.catalogue(), errors.New(path + " is a file, not a folder")
	}
	if _, err := s.store.AddPluginFolder(path); err != nil {
		return s.catalogue(), err
	}
	s.forget()
	return s.catalogue(), nil
}

// RemoveFolder stops reading plugins from a directory. Nothing is deleted; the
// plugins in it stop being offered, and a tab open on one of their views says
// so rather than emptying.
func (s *PluginService) RemoveFolder(path string) (plugins.Catalogue, error) {
	if _, err := s.store.RemovePluginFolder(path); err != nil {
		return s.catalogue(), err
	}
	s.forget()
	return s.catalogue(), nil
}

// BrowseForFolder opens the native picker in directory mode and adds the folder
// chosen. Cancelling leaves everything as it was and is not an error.
func (s *PluginService) BrowseForFolder() (plugins.Catalogue, error) {
	dialog := application.Get().Dialog.OpenFile().
		SetTitle("Add a folder of plugins").
		CanChooseFiles(false).
		CanChooseDirectories(true).
		ShowHiddenFiles(true)

	if home, err := os.UserHomeDir(); err == nil {
		dialog.SetDirectory(home)
	}

	path, err := dialog.PromptForSingleSelection()
	if err != nil {
		return s.catalogue(), err
	}
	if path == "" {
		return s.catalogue(), nil // cancelled
	}
	return s.AddFolder(path)
}
