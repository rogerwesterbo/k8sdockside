package appconfig

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// openIn points the store at a throwaway config directory.
func openIn(t *testing.T) *Store {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	store, err := Open()
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
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	store, err := Open()
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

	reopened, err := Open()
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
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	path := filepath.Join(dir, "k8sdockside", "settings.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{ not json"), 0o600); err != nil {
		t.Fatal(err)
	}

	// Falling back to defaults here would quietly discard the user's colours
	// and aliases, and the next write would overwrite the file for good.
	if _, err := Open(); err == nil {
		t.Fatal("expected an error for an unparsable settings file")
	}
}

func TestOlderOrHandEditedFilesAreFilledIn(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	path := filepath.Join(dir, "k8sdockside", "settings.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	// No layout at all, and a dock value that is not one of the three edges.
	if err := os.WriteFile(path, []byte(`{"contexts":{},"layout":{"detailDock":"sideways"}}`), 0o600); err != nil {
		t.Fatal(err)
	}

	store, err := Open()
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
