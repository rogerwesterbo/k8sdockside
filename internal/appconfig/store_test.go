package appconfig

import (
	json "encoding/json/v2"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rogerwesterbo/k8sdockside/internal/themes"
)

// tempSettings is a throwaway settings file for one test. Tests must never
// touch the real one. Tests that need Open itself, rather than openAt, sandbox
// it by pointing HOME at a temporary directory -- see sandboxHome.
func tempSettings(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "settings.json")
}

// openIn points the store at a throwaway settings file.
func openIn(t *testing.T) *Store {
	t.Helper()

	store, err := openAt(tempSettings(t))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	return store
}

func TestOpenWithNoFileYieldsDefaults(t *testing.T) {
	store := openIn(t)

	got := store.Get()
	if got.Layout.DetailDock != "right" || got.Layout.DetailSize != 520 {
		t.Errorf("layout = %+v, want the defaults", got.Layout)
	}
	// These are serialised straight to JSON, where nil would become null and
	// break the frontend's array handling.
	if got.ManualFiles == nil || got.Contexts == nil || got.TabOrder == nil {
		t.Error("empty collections should be initialised, not nil")
	}
	if _, err := os.Stat(store.Path()); !os.IsNotExist(err) {
		t.Error("Open should not create the settings file")
	}
}

func TestSettingsSurviveAReopen(t *testing.T) {
	path := tempSettings(t)

	store, err := openAt(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.SetContextPrefs("cfg::prod", ContextPrefs{Alias: "Production", Color: "#b8384b"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.AddManualFile("/srv/kubeconfig"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SetPanes(Panes{
		Main: PaneState{Tabs: []PaneTabRef{{Type: ViewResource, ContextID: "cfg::prod", Kind: "pods"}}, Open: true},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SetLayout(Layout{DetailDock: "bottom", DetailSize: 340, SidebarWidth: 300}); err != nil {
		t.Fatal(err)
	}

	reopened, err := openAt(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	got := reopened.Get()

	if got.Contexts["cfg::prod"].Alias != "Production" {
		t.Errorf("alias = %q, want Production", got.Contexts["cfg::prod"].Alias)
	}
	if len(got.ManualFiles) != 1 || got.ManualFiles[0] != "/srv/kubeconfig" {
		t.Errorf("manual files = %v", got.ManualFiles)
	}
	if main := got.Panes.Main.Tabs; len(main) != 1 || main[0].Kind != "pods" {
		t.Errorf("main pane = %+v", main)
	}
	if got.Layout.DetailDock != "bottom" || got.Layout.DetailSize != 340 {
		t.Errorf("layout = %+v", got.Layout)
	}
}

func TestClearingBothPrefsForgetsTheContext(t *testing.T) {
	store := openIn(t)

	if _, err := store.SetContextPrefs("cfg::prod", ContextPrefs{Alias: "Production"}); err != nil {
		t.Fatal(err)
	}
	saved, err := store.SetContextPrefs("cfg::prod", ContextPrefs{})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := saved.Contexts["cfg::prod"]; ok {
		t.Error("a context with no alias and no colour should not be stored at all")
	}
}

func TestAddManualFileIsIdempotent(t *testing.T) {
	store := openIn(t)

	for range 3 {
		if _, err := store.AddManualFile("/srv/kubeconfig"); err != nil {
			t.Fatal(err)
		}
	}
	if got := store.ManualFiles(); len(got) != 1 {
		t.Errorf("manual files = %v, want one entry", got)
	}

	if _, err := store.RemoveManualFile("/srv/kubeconfig"); err != nil {
		t.Fatal(err)
	}
	if got := store.ManualFiles(); len(got) != 0 {
		t.Errorf("manual files = %v, want none", got)
	}
}

func TestGetReturnsACopy(t *testing.T) {
	store := openIn(t)
	if _, err := store.SetContextPrefs("cfg::prod", ContextPrefs{Color: "#fff"}); err != nil {
		t.Fatal(err)
	}

	// Callers must not be able to reach into the store's state by mutating what
	// they were handed.
	snapshot := store.Get()
	snapshot.Contexts["cfg::prod"] = ContextPrefs{Color: "#000"}
	snapshot.ManualFiles = append(snapshot.ManualFiles, "/tmp/x")

	if store.Get().Contexts["cfg::prod"].Color != "#fff" {
		t.Error("mutating the returned settings changed the store")
	}
}

func TestUnreadableSettingsAreAnErrorRatherThanSilentDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")

	if err := os.WriteFile(path, []byte("{ not json"), 0o600); err != nil {
		t.Fatal(err)
	}

	// Falling back to defaults here would quietly discard the user's colours
	// and aliases, and the next write would overwrite the file for good.
	if _, err := openAt(path); err == nil {
		t.Fatal("expected an error for an unparsable settings file")
	}
}

func TestOlderOrHandEditedFilesAreFilledIn(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")

	// No layout at all, and a dock value that is not one of the three edges.
	if err := os.WriteFile(path, []byte(`{"contexts":{},"layout":{"detailDock":"sideways"}}`), 0o600); err != nil {
		t.Fatal(err)
	}

	store, err := openAt(path)
	if err != nil {
		t.Fatal(err)
	}
	got := store.Get()
	if got.Layout.DetailDock != "right" {
		t.Errorf("dock = %q, want the default after an invalid value", got.Layout.DetailDock)
	}
	if got.Layout.SidebarWidth < 180 {
		t.Errorf("sidebar width = %d, want the default", got.Layout.SidebarWidth)
	}
	if got.ManualFiles == nil || got.TabOrder == nil {
		t.Error("missing collections should be initialised")
	}
}

func TestWritesLeaveNoTemporaryFilesBehind(t *testing.T) {
	store := openIn(t)
	if _, err := store.SetContextPrefs("cfg::prod", ContextPrefs{Color: "#fff"}); err != nil {
		t.Fatal(err)
	}

	entries, err := os.ReadDir(filepath.Dir(store.Path()))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".settings-") {
			t.Errorf("left a temporary file behind: %s", e.Name())
		}
	}

	raw, err := os.ReadFile(store.Path())
	if err != nil {
		t.Fatal(err)
	}
	var parsed Settings
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatalf("settings file is not valid JSON: %v", err)
	}

	info, err := os.Stat(store.Path())
	if err != nil {
		t.Fatal(err)
	}
	// The file records nothing secret, but it is per-user state and the config
	// directory is shared, so it should not be world-readable.
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("settings file mode = %o, want 600", perm)
	}
}

// Collapsed nav groups are a layout preference like the rest, so they persist
// and survive a settings file written before the field existed.
func TestCollapsedGroupsRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")

	store, err := openAt(path)
	if err != nil {
		t.Fatal(err)
	}

	layout := store.Get().Layout
	layout.CollapsedGroups = []string{"Gateway API", "Admission"}
	if _, err := store.SetLayout(layout); err != nil {
		t.Fatalf("SetLayout: %v", err)
	}

	reopened, err := openAt(path)
	if err != nil {
		t.Fatal(err)
	}
	got := reopened.Get().Layout.CollapsedGroups
	if len(got) != 2 || got[0] != "Gateway API" || got[1] != "Admission" {
		t.Errorf("collapsed groups = %v, want [Gateway API Admission]", got)
	}
}

// Nil and empty mean different things here and the distinction has to survive
// a write and a re-read: nil is "the user has never said", which lets the
// frontend apply its own default folding, while an empty list is "the user
// expanded everything" and must not be re-defaulted out from under them.
//
// The group names themselves stay in the frontend catalogue, which is where
// they are defined; the store only remembers the strings it is handed.
func TestCollapsedGroupsAreNilUntilTheUserChooses(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")

	// A settings file written before this field existed.
	if err := os.WriteFile(path, []byte(`{"contexts":{},"layout":{"detailDock":"right"}}`), 0o600); err != nil {
		t.Fatal(err)
	}

	store, err := openAt(path)
	if err != nil {
		t.Fatal(err)
	}
	if store.Get().Layout.CollapsedGroups != nil {
		t.Error("CollapsedGroups should stay nil until chosen, so defaults can be applied once")
	}
}

func TestAnEmptyCollapsedListIsRememberedAsAChoice(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")

	store, err := openAt(path)
	if err != nil {
		t.Fatal(err)
	}
	layout := store.Get().Layout
	layout.CollapsedGroups = []string{}
	if _, err := store.SetLayout(layout); err != nil {
		t.Fatalf("SetLayout: %v", err)
	}

	reopened, err := openAt(path)
	if err != nil {
		t.Fatal(err)
	}
	got := reopened.Get().Layout.CollapsedGroups
	if got == nil {
		t.Error("an explicitly empty list came back nil, so the defaults would be re-applied")
	}
	if len(got) != 0 {
		t.Errorf("collapsed groups = %v, want empty", got)
	}
}

// A context may override the global folding. Nil means it follows the baseline,
// so the field cannot be flattened to an empty slice on the way through.
func TestAContextCanOverrideTheGlobalFolding(t *testing.T) {
	path := tempSettings(t)

	store, err := openAt(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.SetContextPrefs("cfg::kubevirt", ContextPrefs{
		CollapsedGroups: []string{"Admission"},
	}); err != nil {
		t.Fatal(err)
	}

	reopened, err := openAt(path)
	if err != nil {
		t.Fatal(err)
	}
	got := reopened.Get().Contexts["cfg::kubevirt"].CollapsedGroups
	if len(got) != 1 || got[0] != "Admission" {
		t.Errorf("collapsed groups = %v, want [Admission]", got)
	}
}

// Folding is a real preference, so a context carrying only an override must not
// be discarded the way one with no alias and no colour is.
func TestAnOverrideAloneKeepsTheContextEntry(t *testing.T) {
	store, err := openAt(tempSettings(t))
	if err != nil {
		t.Fatal(err)
	}

	saved, err := store.SetContextPrefs("cfg::kubevirt", ContextPrefs{CollapsedGroups: []string{}})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := saved.Contexts["cfg::kubevirt"]; !ok {
		t.Error("a context overriding the folding with an empty list was dropped")
	}
}

func TestAContextWithNothingSetIsStillForgotten(t *testing.T) {
	store, err := openAt(tempSettings(t))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.SetContextPrefs("cfg::prod", ContextPrefs{Alias: "Production"}); err != nil {
		t.Fatal(err)
	}

	saved, err := store.SetContextPrefs("cfg::prod", ContextPrefs{})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := saved.Contexts["cfg::prod"]; ok {
		t.Error("a context with no alias, colour or override should be forgotten")
	}
}

// Zoom is a layout preference like the sidebar width: remembered, and repaired
// if the file carries something unusable.
func TestZoomRoundTripsAndIsRepaired(t *testing.T) {
	path := tempSettings(t)
	store, err := openAt(path)
	if err != nil {
		t.Fatal(err)
	}
	if store.Get().Layout.Zoom != 1 {
		t.Errorf("zoom = %v, want 1 by default", store.Get().Layout.Zoom)
	}

	layout := store.Get().Layout
	layout.Zoom = 1.25
	if _, err := store.SetLayout(layout); err != nil {
		t.Fatal(err)
	}

	reopened, err := openAt(path)
	if err != nil {
		t.Fatal(err)
	}
	if reopened.Get().Layout.Zoom != 1.25 {
		t.Errorf("zoom = %v, want 1.25 after a reopen", reopened.Get().Layout.Zoom)
	}
}

func TestAnUnusableZoomFallsBackToNormalSize(t *testing.T) {
	path := tempSettings(t)
	if err := os.WriteFile(path, []byte(`{"contexts":{},"layout":{"detailDock":"right","zoom":0}}`), 0o600); err != nil {
		t.Fatal(err)
	}

	store, err := openAt(path)
	if err != nil {
		t.Fatal(err)
	}
	// A zero here would render the window at nothing; older files have no zoom
	// at all and unmarshal to exactly that.
	if store.Get().Layout.Zoom != 1 {
		t.Errorf("zoom = %v, want 1", store.Get().Layout.Zoom)
	}
}

// The store writes with encoding/json/v2, which formats a nil slice as [] and
// iterates maps in a random order unless told otherwise. Both defaults would be
// quietly wrong here, and neither shows up in the round-trip tests above --
// hence these two. See settingsFormat.

// Saving anything at all rewrites the whole file, including the fields the user
// has not touched. Folding they have never chosen has to still be unchosen
// afterwards, or the frontend stops applying its one-time default the moment
// the user does something unrelated like adding a kubeconfig.
func TestNeverChosenFoldingSurvivesAnUnrelatedSave(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")

	store, err := openAt(path)
	if err != nil {
		t.Fatal(err)
	}
	if store.Get().Layout.CollapsedGroups != nil {
		t.Fatal("a fresh store should start with no folding choice")
	}
	// Something unrelated, which flushes the file.
	if _, err := store.AddManualFile("/tmp/some.config"); err != nil {
		t.Fatalf("AddManualFile: %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"collapsedGroups": null`) {
		t.Errorf("an unchosen folding was not written as null:\n%s", raw)
	}

	reopened, err := openAt(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := reopened.Get().Layout.CollapsedGroups; got != nil {
		t.Errorf("collapsed groups = %v, want nil -- the one-time default is now lost", got)
	}
}

// Contexts live in a map, and a map written in iteration order would reshuffle
// the file on every save: useless to diff, and noisy for anyone who keeps their
// settings in version control.
func TestTheFileKeepsAStableContextOrder(t *testing.T) {
	write := func(t *testing.T) string {
		t.Helper()
		store, err := openAt(filepath.Join(t.TempDir(), "settings.json"))
		if err != nil {
			t.Fatal(err)
		}
		for _, id := range []string{"z::admin", "a::admin", "m::admin", "b::admin", "y::admin"} {
			if _, err := store.SetContextPrefs(id, ContextPrefs{Alias: id}); err != nil {
				t.Fatalf("SetContextPrefs(%q): %v", id, err)
			}
		}
		raw, err := os.ReadFile(store.Path())
		if err != nil {
			t.Fatal(err)
		}
		return string(raw)
	}

	first := write(t)
	for i := range 8 {
		if got := write(t); got != first {
			t.Fatalf("run %d wrote a different file:\n--- first ---\n%s\n--- got ---\n%s", i+1, first, got)
		}
	}
}

func TestPreferencesRoundTrip(t *testing.T) {
	path := tempSettings(t)

	store, err := openAt(path)
	if err != nil {
		t.Fatal(err)
	}
	off := false
	if _, err := store.SetPreferences(Preferences{
		Theme:                "k8sdockside-light",
		Density:              DensityCompact,
		RestoreTabs:          &off,
		ConfirmSourceRemoval: true,
		ShowKubeconfigNames:  true,
	}); err != nil {
		t.Fatalf("SetPreferences: %v", err)
	}

	reopened, err := openAt(path)
	if err != nil {
		t.Fatal(err)
	}
	got := reopened.Get().Preferences
	if got.Theme != "k8sdockside-light" || got.Density != DensityCompact {
		t.Errorf("preferences = %+v, want light and compact", got)
	}
	if !got.ConfirmSourceRemoval {
		t.Error("ConfirmSourceRemoval did not survive the reopen")
	}
	if got.RestoreTabs == nil || *got.RestoreTabs {
		t.Error("an explicit 'do not restore tabs' came back as nil or true")
	}
	if !got.ShowKubeconfigNames {
		t.Error("an explicit 'show kubeconfig names' did not survive the reopen")
	}
}

// The default is true, so a file written before the field existed must not
// unmarshal to Go's zero value and silently stop restoring anyone's tabs.
func TestRestoreTabsIsNilUntilTheUserChooses(t *testing.T) {
	path := tempSettings(t)

	if err := os.WriteFile(path, []byte(`{"contexts":{},"layout":{"detailDock":"right"}}`), 0o600); err != nil {
		t.Fatal(err)
	}

	store, err := openAt(path)
	if err != nil {
		t.Fatal(err)
	}
	prefs := store.Get().Preferences
	if prefs.RestoreTabs != nil {
		t.Error("RestoreTabs should stay nil until chosen, so the frontend can default it to true")
	}
	// ShowKubeconfigNames needs no such treatment: its default is "hidden",
	// which is Go's zero value, so an older file reads correctly on its own.
	if prefs.ShowKubeconfigNames {
		t.Error("kubeconfig names should be hidden until the user asks for them")
	}
}

func TestUnknownPreferenceValuesFallBackToTheDefaults(t *testing.T) {
	path := tempSettings(t)

	// Hand-edited: a density that does not exist. fontSize is deliberately
	// still here -- it was a real field once, and a settings file written by
	// that version must not fail to parse now that it is gone.
	body := `{"contexts":{},"preferences":{"density":"roomy","fontSize":400}}`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	store, err := openAt(path)
	if err != nil {
		t.Fatal(err)
	}
	got := store.Get().Preferences
	if got.Theme != themes.DefaultID {
		t.Errorf("theme = %q, want %q", got.Theme, themes.DefaultID)
	}
	if got.Density != DensityComfortable {
		t.Errorf("density = %q, want %q", got.Density, DensityComfortable)
	}
}

// A theme id the store does not recognise is kept rather than reset. The store
// cannot know what themes exist -- one may live in a folder that has not been
// read, or on a machine this file has not reached -- so resetting would quietly
// throw away a choice that is about to become valid again.
func TestAnUnknownThemeIDIsKept(t *testing.T) {
	path := tempSettings(t)
	body := `{"contexts":{},"preferences":{"theme":"someone-elses-theme"}}`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	store, err := openAt(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := store.Get().Preferences.Theme; got != "someone-elses-theme" {
		t.Errorf("theme = %q, want it left alone", got)
	}
}

// The three values Theme held before the app had a gallery are rewritten to the
// themes that replaced them, so upgrading does not repaint anyone's app.
func TestLegacyThemeValuesAreMigrated(t *testing.T) {
	for old, want := range map[string]string{
		"dark":   themes.DefaultID,
		"light":  "k8sdockside-light",
		"system": themes.DefaultID,
	} {
		t.Run(old, func(t *testing.T) {
			path := tempSettings(t)
			body := `{"contexts":{},"preferences":{"theme":"` + old + `"}}`
			if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
				t.Fatal(err)
			}

			store, err := openAt(path)
			if err != nil {
				t.Fatal(err)
			}
			if got := store.Get().Preferences.Theme; got != want {
				t.Errorf("theme %q migrated to %q, want %q", old, got, want)
			}
		})
	}
}

func TestThemeFolders(t *testing.T) {
	store := openIn(t)

	if _, err := store.AddThemeFolder("/home/u/themes"); err != nil {
		t.Fatalf("AddThemeFolder: %v", err)
	}
	// Adding the same folder twice is a no-op, not a duplicate row in the UI.
	if _, err := store.AddThemeFolder("/home/u/themes"); err != nil {
		t.Fatalf("AddThemeFolder again: %v", err)
	}
	if got := store.ThemeFolders(); len(got) != 1 || got[0] != "/home/u/themes" {
		t.Fatalf("folders = %v, want the one", got)
	}
	if _, err := store.AddThemeFolder(""); err == nil {
		t.Error("an empty path was accepted")
	}

	if _, err := store.RemoveThemeFolder("/home/u/themes"); err != nil {
		t.Fatalf("RemoveThemeFolder: %v", err)
	}
	if got := store.ThemeFolders(); len(got) != 0 {
		t.Errorf("folders = %v, want none", got)
	}
}

// Get hands out copies. A pointer field would otherwise let a caller reach
// through the returned value and mutate the store's own state.
func TestPreferencesCopyThePointerField(t *testing.T) {
	store := openIn(t)

	on := true
	if _, err := store.SetPreferences(Preferences{
		Theme:       themes.DefaultID,
		Density:     DensityComfortable,
		RestoreTabs: &on,
	}); err != nil {
		t.Fatalf("SetPreferences: %v", err)
	}

	// The caller's own pointer must not be the one the store kept.
	on = false
	if got := store.Get().Preferences.RestoreTabs; got == nil || !*got {
		t.Error("mutating the caller's pointer changed what the store holds")
	}

	// Nor may a pointer handed back be written through.
	handed := store.Get().Preferences
	*handed.RestoreTabs = false
	if got := store.Get().Preferences.RestoreTabs; got == nil || !*got {
		t.Error("writing through a returned pointer changed what the store holds")
	}
}

func TestTheBottomPaneRoundTrips(t *testing.T) {
	path := tempSettings(t)

	store, err := openAt(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.SetPanes(Panes{
		Bottom: PaneState{
			Open: true,
			Size: 420,
			Tabs: []PaneTabRef{
				{Type: ViewEdit, ContextID: "cfg::prod", Kind: "pods", Namespace: "default", Name: "web"},
				{Type: ViewEdit, ContextID: "cfg::stage", Kind: "nodes", Name: "node-1"},
			},
		},
	}); err != nil {
		t.Fatalf("SetPanes: %v", err)
	}

	reopened, err := openAt(path)
	if err != nil {
		t.Fatal(err)
	}
	bottom := reopened.Get().Panes.Bottom
	if !bottom.Open || bottom.Size != 420 {
		t.Errorf("bottom pane = %+v, want it open at 420", bottom)
	}
	got := bottom.Tabs
	if len(got) != 2 {
		t.Fatalf("bottom pane tabs = %d, want 2", len(got))
	}
	if got[0].Namespace != "default" || got[0].Name != "web" || got[0].Type != ViewEdit {
		t.Errorf("first tab = %+v, want the namespaced pod it was given", got[0])
	}
	// A cluster-scoped object has no namespace, and that has to survive as the
	// empty string rather than becoming the tab's undoing on the way back.
	if got[1].Namespace != "" || got[1].Name != "node-1" {
		t.Errorf("second tab = %+v, want the cluster-scoped node", got[1])
	}
}

func TestTheDockHasAListOfTabsEvenWhenTheFileHasNone(t *testing.T) {
	path := tempSettings(t)

	if err := os.WriteFile(path, []byte(`{"contexts":{}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := openAt(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := store.Get().Dock.Tabs; got == nil {
		t.Error("the dock's tabs are nil, want an empty list so the frontend need not guard")
	}
}

// A pane's size has the same problem the zoom did: a file written before the
// field existed unmarshals to 0, which is a pane with no view in it. This one
// comes in through the legacy dock, so it checks the repair and the migration
// in the same breath.
func TestAnUnusablePaneSizeFallsBackToTheDefault(t *testing.T) {
	path := tempSettings(t)

	if err := os.WriteFile(path, []byte(`{"contexts":{},"dock":{"size":12,"open":true}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := openAt(path)
	if err != nil {
		t.Fatal(err)
	}
	bottom := store.Get().Panes.Bottom
	if bottom.Size != Defaults().Panes.Bottom.Size {
		t.Errorf("bottom pane size = %d, want the default %d", bottom.Size, Defaults().Panes.Bottom.Size)
	}
	// Repairing the height must not close a pane the user left open.
	if !bottom.Open {
		t.Error("the bottom pane was closed by the repair")
	}
}

// An upgrade must land the user's session where they left it: what was in the
// strip along the top belongs to the main pane, and what was in the dock at the
// foot belongs to the bottom one.
func TestASettingsFileFromBeforePanesKeepsItsSession(t *testing.T) {
	path := tempSettings(t)

	legacy := `{"contexts":{},` +
		`"tabOrder":[{"contextId":"cfg::prod","kind":"pods"},{"contextId":"cfg::prod","kind":"nodes"}],` +
		`"dock":{"open":true,"size":380,"tabs":[` +
		`{"type":"edit","contextId":"cfg::prod","kind":"pods","namespace":"default","name":"web"}]}}`
	if err := os.WriteFile(path, []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := openAt(path)
	if err != nil {
		t.Fatal(err)
	}
	got := store.Get()

	main := got.Panes.Main.Tabs
	if len(main) != 2 || main[0].Kind != "pods" || main[1].Kind != "nodes" {
		t.Fatalf("main pane = %+v, want the two tabs the strip had, in order", main)
	}
	// Everything the old strip held was a collection, so everything that comes
	// across from it is a resource view.
	if main[0].Type != ViewResource {
		t.Errorf("main tab type = %q, want %q", main[0].Type, ViewResource)
	}

	bottom := got.Panes.Bottom
	if len(bottom.Tabs) != 1 || bottom.Tabs[0].Name != "web" || bottom.Tabs[0].Type != ViewEdit {
		t.Fatalf("bottom pane = %+v, want the editor the dock had", bottom.Tabs)
	}
	// The dock's own two answers come across with its tabs.
	if !bottom.Open || bottom.Size != 380 {
		t.Errorf("bottom pane = open %v at %d, want open at 380", bottom.Open, bottom.Size)
	}
	// Read once and then cleared: two records of one session with nothing
	// keeping them honest is a bug waiting for the day they disagree.
	if len(got.TabOrder) != 0 || len(got.Dock.Tabs) != 0 {
		t.Errorf("the legacy fields survived the migration: tabOrder=%+v dock=%+v", got.TabOrder, got.Dock.Tabs)
	}
}

// The default is on, so a file written before the field existed must not
// unmarshal to Go's zero value and take the gutter away.
func TestShowLineNumbersIsNilUntilTheUserChooses(t *testing.T) {
	path := tempSettings(t)

	if err := os.WriteFile(path, []byte(`{"contexts":{},"preferences":{"theme":"dark"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := openAt(path)
	if err != nil {
		t.Fatal(err)
	}
	if store.Get().Preferences.ShowLineNumbers != nil {
		t.Error("ShowLineNumbers should stay nil until chosen, so the frontend can default it to true")
	}
}

func TestTurningLineNumbersOffSurvivesAReopen(t *testing.T) {
	path := tempSettings(t)

	store, err := openAt(path)
	if err != nil {
		t.Fatal(err)
	}
	off := false
	if _, err := store.SetPreferences(Preferences{
		Theme:           themes.DefaultID,
		Density:         DensityComfortable,
		ShowLineNumbers: &off,
	}); err != nil {
		t.Fatalf("SetPreferences: %v", err)
	}

	// The caller's own pointer must not be the one the store kept.
	off = true

	reopened, err := openAt(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := reopened.Get().Preferences.ShowLineNumbers; got == nil || *got {
		t.Error("an explicit 'no line numbers' came back as nil or true")
	}
	if got := store.Get().Preferences.ShowLineNumbers; got == nil || *got {
		t.Error("mutating the caller's pointer changed what the store holds")
	}
}

// The panes are written as one value precisely so that the frontend's concerns
// -- what is open where, and whether each pane is showing it -- cannot reach the
// file through separate calls and undo each other.
func TestSetPanesKeepsTheCallersSliceOutOfTheStore(t *testing.T) {
	store := openIn(t)

	tabs := []PaneTabRef{{Type: ViewEdit, ContextID: "cfg::prod", Kind: "pods", Name: "web"}}
	if _, err := store.SetPanes(Panes{Bottom: PaneState{Open: true, Size: 320, Tabs: tabs}}); err != nil {
		t.Fatalf("SetPanes: %v", err)
	}

	tabs[0].Name = "elsewhere"
	if got := store.Get().Panes.Bottom.Tabs[0].Name; got != "web" {
		t.Errorf("tab name = %q, want the store to hold its own copy", got)
	}
}

func TestSetPluginEnabledRecordsOnlyTheDisabledOnes(t *testing.T) {
	store := openIn(t)

	// Nothing is disabled until somebody says so: the list is an opt-out, so a
	// built-in added in a later release is on without a migration.
	if got := store.DisabledPlugins(); len(got) != 0 {
		t.Fatalf("DisabledPlugins() = %v, want none to start with", got)
	}

	if _, err := store.SetPluginEnabled("argocd", false); err != nil {
		t.Fatalf("SetPluginEnabled: %v", err)
	}
	if got := store.DisabledPlugins(); len(got) != 1 || got[0] != "argocd" {
		t.Errorf("DisabledPlugins() = %v, want [argocd]", got)
	}

	// Disabling twice must not record it twice.
	if _, err := store.SetPluginEnabled("argocd", false); err != nil {
		t.Fatalf("SetPluginEnabled: %v", err)
	}
	if got := store.DisabledPlugins(); len(got) != 1 {
		t.Errorf("DisabledPlugins() = %v, want argocd recorded once", got)
	}

	if _, err := store.SetPluginEnabled("argocd", true); err != nil {
		t.Fatalf("SetPluginEnabled: %v", err)
	}
	if got := store.DisabledPlugins(); len(got) != 0 {
		t.Errorf("DisabledPlugins() = %v, want re-enabling to clear it", got)
	}
}

func TestSetPluginEnabledSurvivesAReopen(t *testing.T) {
	path := tempSettings(t)
	store, err := openAt(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if _, err := store.SetPluginEnabled("flux", false); err != nil {
		t.Fatalf("SetPluginEnabled: %v", err)
	}

	reopened, err := openAt(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if got := reopened.DisabledPlugins(); len(got) != 1 || got[0] != "flux" {
		t.Errorf("after reopen DisabledPlugins() = %v, want [flux]", got)
	}
}

func TestSetPluginEnabledRefusesABlankID(t *testing.T) {
	store := openIn(t)

	if _, err := store.SetPluginEnabled("", false); err == nil {
		t.Error("expected an error for a plugin id that is empty")
	}
}

// The sidebar was a fixed strip with its width in Layout. It is a pane holding
// the cluster tree now, so an upgrade has to carry that width across and put the
// tree in it -- otherwise the window comes back with no way to navigate.
func TestASettingsFileFromBeforeTheSidebarWasAPane(t *testing.T) {
	path := tempSettings(t)

	legacy := `{"contexts":{},"layout":{"detailDock":"right","detailSize":520,"sidebarWidth":345,"zoom":1}}`
	if err := os.WriteFile(path, []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := openAt(path)
	if err != nil {
		t.Fatal(err)
	}
	left := store.Get().Panes.Left

	if left.Size != 345 {
		t.Errorf("left pane size = %d, want the sidebar's own width 345", left.Size)
	}
	if len(left.Tabs) != 1 || left.Tabs[0].Type != ViewClusters {
		t.Fatalf("left pane = %+v, want the cluster tree in it", left.Tabs)
	}
	if !left.Open {
		t.Error("the left pane came back hidden, so the tree would be nowhere")
	}
}

// The tree is the one view that cannot be closed, so a file that has lost it --
// hand-edited, or written by a build where it was not yet a tab -- is repaired
// rather than obeyed.
func TestAFileMissingTheClusterTreeGetsItBack(t *testing.T) {
	path := tempSettings(t)

	none := `{"contexts":{},"panes":{"left":{"tabs":[],"open":true,"size":260},` +
		`"main":{"tabs":[],"open":true},"right":{"tabs":[]},"bottom":{"tabs":[],"size":320}}}`
	if err := os.WriteFile(path, []byte(none), 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := openAt(path)
	if err != nil {
		t.Fatal(err)
	}

	left := store.Get().Panes.Left
	if len(left.Tabs) != 1 || left.Tabs[0].Type != ViewClusters {
		t.Errorf("left pane = %+v, want the tree put back", left.Tabs)
	}
}

// ...but a tree the user has moved is left where they put it.
func TestAClusterTreeMovedToAnotherPaneStaysThere(t *testing.T) {
	path := tempSettings(t)

	moved := `{"contexts":{},"panes":{"left":{"tabs":[],"open":true,"size":260},` +
		`"main":{"tabs":[],"open":true},` +
		`"right":{"tabs":[{"type":"clusters","kind":"clusters"}],"open":true,"size":420},` +
		`"bottom":{"tabs":[],"size":320}}}`
	if err := os.WriteFile(path, []byte(moved), 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := openAt(path)
	if err != nil {
		t.Fatal(err)
	}
	got := store.Get().Panes

	if len(got.Left.Tabs) != 0 {
		t.Errorf("left pane = %+v, want it left empty", got.Left.Tabs)
	}
	if len(got.Right.Tabs) != 1 || got.Right.Tabs[0].Type != ViewClusters {
		t.Errorf("right pane = %+v, want the tree the user moved there", got.Right.Tabs)
	}
}
