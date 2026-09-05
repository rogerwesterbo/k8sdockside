// Package appconfig persists the user's own choices -- which kubeconfig files
// they added, what they renamed each context to, the colour they gave it, and
// where they docked the detail panel. None of this can be derived from the
// kubeconfig files themselves, so it lives in its own file under the user's
// config directory -- ~/.config/k8sdockside, alongside the other Kubernetes
// tooling, rather than wherever the platform would hide application state.
package appconfig

import (
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sync"

	"github.com/roger/k8sdockside/internal/themes"
)

// ContextPrefs is what the user decided about one kubeconfig context. Alias
// overrides the context's name in the UI; Color tints its sidebar entry and
// every tab opened against it. An empty field means "use the default".
type ContextPrefs struct {
	Alias string `json:"alias"`
	Color string `json:"color"`
	// Metrics overrides where this cluster's Prometheus is found, for the charts
	// a solution plugin draws. Empty means "look for it", which is what almost
	// every cluster wants: a Prometheus installed by the Operator or the
	// community chart is discoverable, and reaching it through the API server
	// needs no address at all.
	//
	// Two forms are accepted -- `namespace/service:port` and an http(s) URL --
	// because there are two genuinely different situations behind them; see
	// metrics.ParseEndpoint.
	Metrics string `json:"metrics,omitzero"`
	// CollapsedGroups overrides Layout.CollapsedGroups for this context alone.
	// Nil means "follow the global setting", which is the usual case -- a
	// cluster only carries its own list once the user folds a group for it
	// specifically, e.g. because this is the one cluster with the Gateway API
	// installed.
	CollapsedGroups []string `json:"collapsedGroups"`
}

// TabRef identifies one open tab: a kubeconfig context and the resource kind
// shown in it. It is stored as a pair rather than a joined string because
// context IDs contain a file path, so a composite key could not be split back
// apart reliably.
type TabRef struct {
	ContextID string `json:"contextId"`
	Kind      string `json:"kind"`
}

// DockTabRef identifies one tab in the bottom dock. Where a TabRef names a
// collection -- the pods of a cluster -- a dock tab is a view onto one object,
// so it carries that object's namespace and name as well.
//
// Type names which view it is. There is only one today, "edit", and it is
// stored rather than assumed so that a dock which grows a second view can still
// read a file written by this one.
type DockTabRef struct {
	Type      string `json:"type"`
	ContextID string `json:"contextId"`
	Kind      string `json:"kind"`
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
}

// Dock is the state of the strip at the foot of the window: what it has open,
// whether it is showing it, and how tall it stands when it is.
//
// It is one record with one writer rather than tabs here and a size in Layout,
// and that is load bearing. Everything in it changes together -- opening an
// editor adds a tab and unfolds the dock in the same gesture -- and every
// mutator on this store answers with the whole settings for the frontend to
// adopt. Two writers over one gesture would each answer with the whole of it,
// and the slower would carry the other's change back to what it was before it
// was made.
type Dock struct {
	Open bool `json:"open"`
	// Size is the dock's height in px when it is open. Zero from a file written
	// before the dock existed, which normalise repairs.
	Size int          `json:"size"`
	Tabs []DockTabRef `json:"tabs"`
}

// Layout is the arrangement the user last left the window in.
type Layout struct {
	DetailDock   string `json:"detailDock"`   // right | bottom | left
	DetailSize   int    `json:"detailSize"`   // px along the docked edge
	SidebarWidth int    `json:"sidebarWidth"` // px
	// Zoom is the webview scale, 1 being normal size. Persisted so the window
	// comes back the size the user left it readable at.
	Zoom float64 `json:"zoom"`
	// CollapsedGroups are the sidebar's resource-tree headings the user has
	// folded away. It is deliberately nullable: nil means they have never
	// chosen, which is what lets the frontend fold the specialist groups once
	// on a fresh install, while an empty list is the real choice "show me
	// everything" and must not be defaulted over. The names are the frontend
	// catalogue's group labels; the store only remembers what it is handed.
	//
	// No omitempty: it would drop an empty list on write, turning "I expanded
	// everything" back into "never chosen" on the next read.
	CollapsedGroups []string `json:"collapsedGroups"`
}

// Preferences are the app-wide choices that belong to neither one context nor
// the window's arrangement: how the app looks, and what it does on its own
// without being asked. They are kept apart from Layout deliberately -- Layout
// is "where the user left the furniture", which is written on every splitter
// drag, while these are settings someone sat down and chose.
type Preferences struct {
	// Theme is the id of the theme the user chose -- a built-in one, or one
	// they installed. Empty means the default.
	//
	// It is stored as a free string and deliberately not validated against a
	// list, because the store has no way to know what themes exist: a theme
	// may live in a folder that has not been read yet, or on a machine this
	// settings file has not reached. An id nothing answers to is kept as
	// written and falls back to the default at the point it is applied, so
	// that a theme temporarily missing does not silently unset the choice.
	//
	// Nothing here knows what a colour is; see internal/themes.
	Theme string `json:"theme"`
	// Density is comfortable or compact, and drives the table row height.
	// Empty means comfortable.
	Density string `json:"density"`
	// RestoreTabs reopens last session's tabs at launch.
	//
	// Nullable for the same reason Layout.CollapsedGroups is, and it matters
	// more here: the default is *true*, so a plain bool read from a settings
	// file written before this field existed would unmarshal to false and
	// silently stop restoring tabs for every existing user. Nil means "never
	// chosen" and resolves to true; only an explicit false turns it off.
	RestoreTabs *bool `json:"restoreTabs"`
	// ConfirmSourceRemoval asks before hiding a kubeconfig or dropping a
	// watched folder. No pointer needed: false is what the app does today, so
	// an older file reading as false is already correct.
	ConfirmSourceRemoval bool `json:"confirmSourceRemoval"`
	// ShowKubeconfigNames groups the sidebar's contexts under the file they
	// came from, headed by its name. Off by default -- most people keep every
	// context in one ~/.kube/config, where the heading only repeats itself --
	// leaving the contexts as one flat list. A file that could not be parsed is
	// named either way, because it has no contexts to show in its place and
	// would otherwise vanish from the sidebar silently.
	//
	// A plain bool, unlike RestoreTabs: here the default and Go's zero value
	// are the same, so a settings file written before this field existed reads
	// as "hidden", which is exactly right.
	ShowKubeconfigNames bool `json:"showKubeconfigNames"`
	// MetricsRange is how far back a chart looks, in minutes. Persisted because
	// it is a way of working rather than a moment: somebody watching a rollout
	// wants fifteen minutes every time they open a pod, and somebody reviewing
	// capacity wants a day.
	//
	// Zero means never chosen and resolves to an hour.
	MetricsRange int `json:"metricsRange,omitzero"`
	// ShowLineNumbers draws a line-number gutter down the side of the YAML
	// editor in the bottom dock.
	//
	// Nullable for the same reason RestoreTabs is, and for the same reason it
	// matters: the default is *on*, so a plain bool read from a file written
	// before this field existed would unmarshal to false and quietly take the
	// gutter away from everyone who already had it.
	ShowLineNumbers *bool `json:"showLineNumbers"`
}

// Settings is the whole persisted file.
type Settings struct {
	ManualFiles []string `json:"manualFiles"`
	// ManualFolders are directories the user asked us to watch. They are kept
	// as folders rather than expanded into their files at the time they were
	// added, so that a config dropped into one later is picked up by the next
	// sync instead of needing to be added by hand.
	ManualFolders []string `json:"manualFolders"`
	// ExcludedFiles are kubeconfigs found by discovery that the user has said
	// they do not want. A discovered file cannot simply be forgotten -- the
	// next scan would find it again -- so refusing it has to be recorded.
	ExcludedFiles []string `json:"excludedFiles"`
	// ThemeFolders are extra directories to read themes from, on top of the
	// themes folder beside this file. They sit here rather than in Preferences
	// for the same reason ManualFolders does: this is where something comes
	// from, not a choice about how the app behaves.
	ThemeFolders []string `json:"themeFolders"`
	// PluginFolders is the same for solution plugins. Kept separate from
	// ThemeFolders rather than merged into one list of "add-on folders":
	// somebody who syncs a folder of themes has not asked for plugins out of
	// it, and one list would mean every folder was scanned for both.
	PluginFolders []string                `json:"pluginFolders"`
	Contexts      map[string]ContextPrefs `json:"contexts"`
	TabOrder      []TabRef                `json:"tabOrder"`
	// Dock is the bottom strip: its tabs in the order the user left them, and
	// whether it is showing them. Kept apart from TabOrder rather than merged
	// into it: the two strips are reordered independently and hold different
	// things, and one list would have to carry a discriminator to be split
	// back apart on read.
	Dock        Dock        `json:"dock"`
	Layout      Layout      `json:"layout"`
	Preferences Preferences `json:"preferences"`
}

// Defaults returns a settings value that is safe to use before anything has
// been saved, and that also fills in any field missing from an older file.
func Defaults() Settings {
	return Settings{
		ManualFiles:   []string{},
		ManualFolders: []string{},
		ExcludedFiles: []string{},
		ThemeFolders:  []string{},
		PluginFolders: []string{},
		Contexts:      map[string]ContextPrefs{},
		TabOrder:      []TabRef{},
		Dock:          Dock{Size: 320, Tabs: []DockTabRef{}},
		Layout:        Layout{DetailDock: "right", DetailSize: 520, SidebarWidth: 260, Zoom: 1},
		Preferences:   Preferences{Theme: themes.DefaultID, Density: DensityComfortable},
	}
}

// The range the webview may be scaled to. Below MinZoom the native macOS
// traffic lights no longer fit the window's own title bar, which is drawn in
// CSS pixels and so shrinks with the zoom; above MaxZoom the sidebar can no
// longer show a context name.
const (
	MinZoom = 0.5
	MaxZoom = 2.0
)

// The values Preferences.Density may take. They are strings rather than an enum
// so the settings file stays readable and so an unknown value from a
// hand-edited file normalises back to the default instead of failing to parse.
const (
	DensityComfortable = "comfortable"
	DensityCompact     = "compact"
)

// MaxMetricsRange bounds how far back a chart may look, in minutes. A week of
// samples is already more than a line four hundred pixels wide can say.
const MaxMetricsRange = 7 * 24 * 60

// legacyThemes maps what Preferences.Theme held before the app had themes onto
// the theme ids that replaced them.
//
// "system" is here because following the OS appearance is no longer a thing the
// app does: with a gallery of themes there is no pair for the OS to choose
// between, and the setting became "which theme", full stop. Anyone who was on
// it lands on the dark palette they were most likely already looking at, which
// is also what the app defaults to.
var legacyThemes = map[string]string{
	"system": themes.DefaultID,
	"dark":   themes.DefaultID,
	"light":  "k8sdockside-light",
}

// Store is the settings file plus the lock guarding it. Wails calls service
// methods from multiple goroutines, so every read and write goes through the
// mutex and hands back a copy.
type Store struct {
	mu   sync.Mutex
	path string
	data Settings
}

// Open loads the settings file, creating nothing on disk. A missing file is not
// an error -- it just means the user has not customised anything yet. A file
// that exists but cannot be parsed *is* an error, because silently replacing it
// with defaults would throw away the user's colours and aliases.
func Open() (*Store, error) {
	path, err := defaultPath()
	if err != nil {
		return nil, err
	}
	legacy, err := legacyPath()
	if err != nil {
		return nil, err
	}
	// A failed migration is fatal rather than ignored: carrying on would open
	// an empty file at the new path and present the user with a store that has
	// lost every alias, colour and tab they had.
	if err := migrate(legacy, path); err != nil {
		return nil, fmt.Errorf("migrating settings from %s: %w", legacy, err)
	}
	return openAt(path)
}

// openAt is Open against a named file. It exists so the tests can run against a
// temporary directory without going near the real settings file, and so that
// the loading rules can be exercised without also exercising the migration.
func openAt(path string) (*Store, error) {
	s := &Store{path: path, data: Defaults()}

	raw, err := os.ReadFile(path) // #nosec G304 -- the app's own settings file
	if errors.Is(err, fs.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}

	loaded := Defaults()
	if err := json.Unmarshal(raw, &loaded); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	s.data = normalise(loaded)
	return s, nil
}

// Path is where the settings are stored, surfaced in the UI so the user can
// find (or delete) the file.
func (s *Store) Path() string { return s.path }

// Get returns a copy of the current settings.
func (s *Store) Get() Settings {
	s.mu.Lock()
	defer s.mu.Unlock()
	return clone(s.data)
}

// ManualFiles returns the kubeconfig paths the user added by hand.
func (s *Store) ManualFiles() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.data.ManualFiles)
}

// ManualFolders returns the directories the user asked us to scan.
func (s *Store) ManualFolders() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.data.ManualFolders)
}

// update applies a change under the lock and flushes to disk. If the write
// fails the in-memory state is rolled back, so what the UI shows after an error
// still matches what is on disk.
func (s *Store) update(fn func(*Settings)) (Settings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	before := clone(s.data)
	fn(&s.data)
	s.data = normalise(s.data)

	if err := s.flush(); err != nil {
		s.data = before
		return clone(s.data), err
	}
	return clone(s.data), nil
}

// SetContextPrefs records the alias and colour for one context.
func (s *Store) SetContextPrefs(id string, prefs ContextPrefs) (Settings, error) {
	if id == "" {
		return s.Get(), errors.New("context id is required")
	}
	return s.update(func(d *Settings) {
		// A folding override is a preference in its own right, so a context
		// carrying only that one is kept. An empty override is meaningful: it
		// says "show every group here", which is not the same as no override.
		if prefs.Alias == "" && prefs.Color == "" && prefs.Metrics == "" && prefs.CollapsedGroups == nil {
			delete(d.Contexts, id)
			return
		}
		d.Contexts[id] = prefs
	})
}

// MetricsEndpoint returns where a context's Prometheus was configured to be,
// empty when the user has not said and discovery should look.
func (s *Store) MetricsEndpoint(contextID string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.data.Contexts[contextID].Metrics
}

// SetMetricsEndpoint records where a context's Prometheus is. An empty value
// clears the override rather than recording an empty one, which is what puts
// discovery back in charge.
func (s *Store) SetMetricsEndpoint(contextID, value string) (Settings, error) {
	if contextID == "" {
		return s.Get(), errors.New("context id is required")
	}
	return s.update(func(d *Settings) {
		prefs := d.Contexts[contextID]
		prefs.Metrics = value
		// The same emptiness rule SetContextPrefs applies: a context with
		// nothing left to say about it is forgotten rather than kept as a blank
		// entry cluttering the settings file.
		if prefs.Alias == "" && prefs.Color == "" && prefs.Metrics == "" && prefs.CollapsedGroups == nil {
			delete(d.Contexts, contextID)
			return
		}
		d.Contexts[contextID] = prefs
	})
}

// AddManualFile remembers a kubeconfig path the user chose. Adding a path that
// is already known is a no-op rather than an error.
func (s *Store) AddManualFile(path string) (Settings, error) {
	if path == "" {
		return s.Get(), errors.New("path is required")
	}
	return s.update(func(d *Settings) {
		if !slices.Contains(d.ManualFiles, path) {
			d.ManualFiles = append(d.ManualFiles, path)
		}
	})
}

// RemoveManualFile forgets a path the user added. Auto-discovered files cannot
// be removed this way -- they will simply reappear on the next sync.
func (s *Store) RemoveManualFile(path string) (Settings, error) {
	return s.update(func(d *Settings) {
		d.ManualFiles = slices.DeleteFunc(d.ManualFiles, func(p string) bool { return p == path })
	})
}

// ExcludedFiles returns the discovered kubeconfigs the user has hidden.
func (s *Store) ExcludedFiles() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.data.ExcludedFiles)
}

// ExcludeFile hides a discovered kubeconfig from the sidebar.
func (s *Store) ExcludeFile(path string) (Settings, error) {
	if path == "" {
		return s.Get(), errors.New("path is required")
	}
	return s.update(func(d *Settings) {
		if !slices.Contains(d.ExcludedFiles, path) {
			d.ExcludedFiles = append(d.ExcludedFiles, path)
		}
	})
}

// UnexcludeFile lets a hidden kubeconfig be discovered again.
func (s *Store) UnexcludeFile(path string) (Settings, error) {
	return s.update(func(d *Settings) {
		d.ExcludedFiles = slices.DeleteFunc(d.ExcludedFiles, func(p string) bool { return p == path })
	})
}

// ClearExclusionsIn forgets what was hidden inside one directory, so that
// re-adding a folder starts from a clean slate rather than quietly continuing
// to hide files the user can no longer see listed anywhere.
func (s *Store) ClearExclusionsIn(dir string) (Settings, error) {
	return s.update(func(d *Settings) {
		d.ExcludedFiles = slices.DeleteFunc(d.ExcludedFiles, func(p string) bool {
			return filepath.Dir(p) == dir
		})
	})
}

// AddManualFolder remembers a directory to scan for kubeconfigs. Adding one
// that is already known is a no-op rather than an error.
func (s *Store) AddManualFolder(path string) (Settings, error) {
	if path == "" {
		return s.Get(), errors.New("path is required")
	}
	return s.update(func(d *Settings) {
		if !slices.Contains(d.ManualFolders, path) {
			d.ManualFolders = append(d.ManualFolders, path)
		}
	})
}

// RemoveManualFolder stops scanning a directory. The configs found through it
// disappear from the sidebar on the next sync.
func (s *Store) RemoveManualFolder(path string) (Settings, error) {
	return s.update(func(d *Settings) {
		d.ManualFolders = slices.DeleteFunc(d.ManualFolders, func(p string) bool { return p == path })
	})
}

// ThemeFolders returns the extra directories themes are read from.
func (s *Store) ThemeFolders() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.data.ThemeFolders)
}

// AddThemeFolder remembers a directory to read themes from. Adding one that is
// already known is a no-op rather than an error.
func (s *Store) AddThemeFolder(path string) (Settings, error) {
	if path == "" {
		return s.Get(), errors.New("path is required")
	}
	return s.update(func(d *Settings) {
		if !slices.Contains(d.ThemeFolders, path) {
			d.ThemeFolders = append(d.ThemeFolders, path)
		}
	})
}

// RemoveThemeFolder stops reading themes from a directory. Nothing is deleted
// from disk; the themes simply stop being offered.
func (s *Store) RemoveThemeFolder(path string) (Settings, error) {
	return s.update(func(d *Settings) {
		d.ThemeFolders = slices.DeleteFunc(d.ThemeFolders, func(p string) bool { return p == path })
	})
}

// PluginFolders returns the extra directories plugins are read from.
func (s *Store) PluginFolders() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.data.PluginFolders)
}

// AddPluginFolder remembers a directory to read plugins from. Adding one that
// is already known is a no-op rather than an error.
func (s *Store) AddPluginFolder(path string) (Settings, error) {
	if path == "" {
		return s.Get(), errors.New("path is required")
	}
	return s.update(func(d *Settings) {
		if !slices.Contains(d.PluginFolders, path) {
			d.PluginFolders = append(d.PluginFolders, path)
		}
	})
}

// RemovePluginFolder stops reading plugins from a directory. Nothing is deleted
// from disk; the plugins in it just stop being offered.
func (s *Store) RemovePluginFolder(path string) (Settings, error) {
	return s.update(func(d *Settings) {
		d.PluginFolders = slices.DeleteFunc(d.PluginFolders, func(p string) bool { return p == path })
	})
}

// SetLayout records the sidebar width and detail-panel dock and size.
func (s *Store) SetLayout(l Layout) (Settings, error) {
	return s.update(func(d *Settings) { d.Layout = l })
}

// SetPreferences records the app-wide preferences. It replaces the block
// wholesale rather than patching one field, matching SetLayout: the settings
// view edits them together and hands back the whole thing, so a partial
// mutator would only add a way for the two to disagree.
func (s *Store) SetPreferences(p Preferences) (Settings, error) {
	return s.update(func(d *Settings) {
		// Copied on the way in for the same reason clone copies it on the way
		// out: the caller's pointer must not become the store's.
		if p.RestoreTabs != nil {
			restore := *p.RestoreTabs
			p.RestoreTabs = &restore
		}
		if p.ShowLineNumbers != nil {
			numbers := *p.ShowLineNumbers
			p.ShowLineNumbers = &numbers
		}
		d.Preferences = p
	})
}

// SetTabOrder records the order the user dragged their tabs into.
func (s *Store) SetTabOrder(order []TabRef) (Settings, error) {
	return s.update(func(d *Settings) { d.TabOrder = slices.Clone(order) })
}

// SetDock records the whole state of the bottom dock. It is written as one
// value for the reason Dock is one -- see there.
func (s *Store) SetDock(dock Dock) (Settings, error) {
	return s.update(func(d *Settings) {
		// Cloned on the way in for the same reason clone copies it on the way
		// out: the caller's slice must not become the store's.
		dock.Tabs = slices.Clone(dock.Tabs)
		d.Dock = dock
	})
}

// settingsFormat pins the file format to what encoding/json v1 wrote, so moving
// to v2 does not rewrite every existing settings file. Two of these are load
// bearing rather than cosmetic:
//
//   - Deterministic keeps the contexts map in a stable key order. v2 otherwise
//     follows Go's randomised map iteration, which would reshuffle the file on
//     every save and make it useless to diff or eyeball.
//   - FormatNilSliceAsNull keeps nil distinct from empty, which Layout and
//     ContextPrefs both depend on -- see CollapsedGroups. v2 would write [] for
//     a nil slice, turning "never chosen" into the explicit choice "show me
//     everything" the first time a fresh install saved anything.
//
// EscapeForHTML and FormatNilMapAsNull only keep the bytes identical to what
// earlier releases produced; nothing reads the file that would care either way.
var settingsFormat = json.JoinOptions(
	jsontext.WithIndent("  "),
	jsontext.EscapeForHTML(true),
	json.Deterministic(true),
	json.FormatNilSliceAsNull(true),
	json.FormatNilMapAsNull(true),
)

// flush writes the settings via a temp file and a rename, so an interrupted
// write cannot leave a half-written config behind. The caller holds the lock.
func (s *Store) flush() error {
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("creating %s: %w", dir, err)
	}

	raw, err := json.Marshal(s.data, settingsFormat)
	if err != nil {
		return err
	}
	raw = append(raw, '\n')

	tmp, err := os.CreateTemp(dir, ".settings-*.json")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	// no-op once the rename below has succeeded
	defer func() { _ = os.Remove(tmpName) }()

	if _, err := tmp.Write(raw); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, s.path)
}

// normalise fills in anything a hand-edited or older settings file left out, so
// the rest of the app never has to check for zero values.
func normalise(s Settings) Settings {
	if s.ManualFiles == nil {
		s.ManualFiles = []string{}
	}
	if s.ManualFolders == nil {
		s.ManualFolders = []string{}
	}
	if s.ExcludedFiles == nil {
		s.ExcludedFiles = []string{}
	}
	if s.ThemeFolders == nil {
		s.ThemeFolders = []string{}
	}
	if s.PluginFolders == nil {
		s.PluginFolders = []string{}
	}
	if s.Contexts == nil {
		s.Contexts = map[string]ContextPrefs{}
	}
	if s.TabOrder == nil {
		s.TabOrder = []TabRef{}
	}
	if s.Dock.Tabs == nil {
		s.Dock.Tabs = []DockTabRef{}
	}
	d := Defaults().Layout
	switch s.Layout.DetailDock {
	case "right", "bottom", "left":
	default:
		s.Layout.DetailDock = d.DetailDock
	}
	if s.Layout.DetailSize < 200 {
		s.Layout.DetailSize = d.DetailSize
	}
	if s.Layout.SidebarWidth < 180 {
		s.Layout.SidebarWidth = d.SidebarWidth
	}
	// A file written before zoom existed unmarshals to 0, which would render
	// the window at nothing.
	if s.Layout.Zoom < MinZoom || s.Layout.Zoom > MaxZoom {
		s.Layout.Zoom = d.Zoom
	}
	// Below this the dock would show its tab strip and a couple of lines of
	// YAML, which is not an editor. A file written before the dock existed
	// reads as 0 and lands here.
	if s.Dock.Size < 160 {
		s.Dock.Size = Defaults().Dock.Size
	}

	p := Defaults().Preferences
	// Only the three legacy values are rewritten. Anything else is a theme id
	// and is kept as written even if nothing currently answers to it -- see
	// Preferences.Theme for why.
	if replacement, legacy := legacyThemes[s.Preferences.Theme]; legacy {
		s.Preferences.Theme = replacement
	} else if s.Preferences.Theme == "" {
		s.Preferences.Theme = p.Theme
	}
	switch s.Preferences.Density {
	case DensityComfortable, DensityCompact:
	default:
		s.Preferences.Density = p.Density
	}
	// A hand-edited file could ask for a year of samples at fifteen-second
	// resolution, which is a query no cluster should be asked to answer.
	if s.Preferences.MetricsRange < 0 || s.Preferences.MetricsRange > MaxMetricsRange {
		s.Preferences.MetricsRange = 0
	}
	// RestoreTabs and ShowLineNumbers are deliberately not defaulted: nil is a
	// value in its own right for both, meaning "never chosen", and the frontend
	// resolves each to true.
	return s
}

func clone(s Settings) Settings {
	out := s
	out.ManualFiles = slices.Clone(s.ManualFiles)
	out.ManualFolders = slices.Clone(s.ManualFolders)
	out.ExcludedFiles = slices.Clone(s.ExcludedFiles)
	out.ThemeFolders = slices.Clone(s.ThemeFolders)
	out.PluginFolders = slices.Clone(s.PluginFolders)
	out.TabOrder = slices.Clone(s.TabOrder)
	out.Dock.Tabs = slices.Clone(s.Dock.Tabs)
	// slices.Clone keeps nil as nil, which is what preserves "never chosen".
	out.Layout.CollapsedGroups = slices.Clone(s.Layout.CollapsedGroups)
	// A copy of the struct shares the pointer; the whole point of clone is that
	// a caller holding the result cannot reach back into the store.
	if s.Preferences.RestoreTabs != nil {
		restore := *s.Preferences.RestoreTabs
		out.Preferences.RestoreTabs = &restore
	}
	if s.Preferences.ShowLineNumbers != nil {
		numbers := *s.Preferences.ShowLineNumbers
		out.Preferences.ShowLineNumbers = &numbers
	}
	out.Contexts = make(map[string]ContextPrefs, len(s.Contexts))
	for k, v := range s.Contexts {
		v.CollapsedGroups = slices.Clone(v.CollapsedGroups)
		out.Contexts[k] = v
	}
	return out
}
