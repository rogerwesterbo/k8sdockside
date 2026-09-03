package appconfig

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
	if _, err := store.SetTabOrder([]TabRef{{ContextID: "cfg::prod", Kind: "pods"}}); err != nil {
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
	if len(got.TabOrder) != 1 || got.TabOrder[0].Kind != "pods" {
		t.Errorf("tab order = %+v", got.TabOrder)
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
