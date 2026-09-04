package main

import (
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"

	"github.com/roger/k8sdockside/internal/appconfig"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// SettingsService exposes the persisted user preferences to the frontend: the
// per-context alias and colour, the tab order, the window layout, and the
// app-wide preferences the settings view edits. Every mutator returns the full
// settings so the frontend can replace its state with what actually reached
// disk rather than assuming its optimistic update stuck.
type SettingsService struct {
	store *appconfig.Store
}

// NewSettingsService wires the service to the settings store.
func NewSettingsService(store *appconfig.Store) *SettingsService {
	return &SettingsService{store: store}
}

// Get returns everything the user has customised.
func (s *SettingsService) Get() appconfig.Settings {
	return s.store.Get()
}

// ConfigPath is where the settings file lives, shown in the UI so the user can
// find it.
func (s *SettingsService) ConfigPath() string {
	return s.store.Path()
}

// RevealConfig opens the settings file in the platform's file manager with the
// file itself selected, which is the useful thing to offer next to a path the
// user cannot click. The path comes from the store rather than the frontend so
// that nothing the webview says can decide what gets opened.
//
// Nothing may have been saved yet -- Open deliberately creates no file -- so a
// missing one falls back to its directory rather than failing. That is still
// the answer to "where does this live?", which is what was asked.
func (s *SettingsService) RevealConfig() error {
	path := s.store.Path()
	if _, err := os.Stat(path); err != nil {
		return application.Get().Env.OpenFileManager(filepath.Dir(path), false)
	}
	return application.Get().Env.OpenFileManager(path, true)
}

// About is what the settings view's About section shows.
type About struct {
	Version  string `json:"version"`
	Wails    string `json:"wails"`
	Go       string `json:"go"`
	Platform string `json:"platform"`
}

// version is stamped at build time where the build does so, and otherwise
// reports what the module graph says.
var version = ""

// About reports what this build is. The values come from the binary's own
// build info rather than a constant, so a development build says so instead of
// claiming whatever number was last committed.
func (s *SettingsService) About() About {
	about := About{
		Version:  version,
		Go:       runtime.Version(),
		Platform: runtime.GOOS + "/" + runtime.GOARCH,
	}
	if info, ok := debug.ReadBuildInfo(); ok {
		if about.Version == "" {
			about.Version = info.Main.Version
		}
		for _, dep := range info.Deps {
			if dep.Path == "github.com/wailsapp/wails/v3" {
				about.Wails = dep.Version
				break
			}
		}
	}
	if about.Version == "" || about.Version == "(devel)" {
		about.Version = "development build"
	}
	return about
}

// SetContextPrefs saves the display name and colour for one context. Clearing
// both fields resets the context to its defaults.
func (s *SettingsService) SetContextPrefs(id string, prefs appconfig.ContextPrefs) (appconfig.Settings, error) {
	return s.store.SetContextPrefs(id, prefs)
}

// SetLayout saves the sidebar width and the detail panel's dock and size.
func (s *SettingsService) SetLayout(layout appconfig.Layout) (appconfig.Settings, error) {
	return s.store.SetLayout(layout)
}

// SetPreferences saves the app-wide preferences: theme, density, font size,
// whether tabs are restored at launch, and whether removing a kubeconfig
// source asks first.
func (s *SettingsService) SetPreferences(prefs appconfig.Preferences) (appconfig.Settings, error) {
	return s.store.SetPreferences(prefs)
}

// SetTabOrder saves the order the user dragged their tabs into.
func (s *SettingsService) SetTabOrder(order []appconfig.TabRef) (appconfig.Settings, error) {
	return s.store.SetTabOrder(order)
}

// SetDock saves the bottom dock: its tabs in order, whether it is open, and how
// tall it stands. It is a separate call from SetTabOrder because the two strips
// are dragged independently and each writes on its own.
func (s *SettingsService) SetDock(dock appconfig.Dock) (appconfig.Settings, error) {
	return s.store.SetDock(dock)
}
