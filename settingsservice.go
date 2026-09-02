package main

import (
	"github.com/roger/k8sdockside/internal/appconfig"
)

// SettingsService exposes the persisted user preferences to the frontend: the
// per-context alias and colour, the tab order, and the window layout. Every
// mutator returns the full settings so the frontend can replace its state with
// what actually reached disk rather than assuming its optimistic update stuck.
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

// SetContextPrefs saves the display name and colour for one context. Clearing
// both fields resets the context to its defaults.
func (s *SettingsService) SetContextPrefs(id string, prefs appconfig.ContextPrefs) (appconfig.Settings, error) {
	return s.store.SetContextPrefs(id, prefs)
}

// SetLayout saves the sidebar width and the detail panel's dock and size.
func (s *SettingsService) SetLayout(layout appconfig.Layout) (appconfig.Settings, error) {
	return s.store.SetLayout(layout)
}

// SetTabOrder saves the order the user dragged their tabs into.
func (s *SettingsService) SetTabOrder(order []appconfig.TabRef) (appconfig.Settings, error) {
	return s.store.SetTabOrder(order)
}
