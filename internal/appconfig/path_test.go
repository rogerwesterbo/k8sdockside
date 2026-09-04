package appconfig

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// sandboxHome points HOME and XDG_CONFIG_HOME at a throwaway directory so that
// path resolution and migration run entirely inside it. Both the new location
// and the legacy one hang off HOME, so this is enough to keep a test away from
// the developer's own settings file.
func sandboxHome(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("path resolution is %AppData% on Windows and not env-sandboxable here")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	return home
}

func TestDefaultPathHonoursXDGConfigHome(t *testing.T) {
	sandboxHome(t)
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)

	got, err := defaultPath()
	if err != nil {
		t.Fatalf("defaultPath: %v", err)
	}
	want := filepath.Join(xdg, "k8sdockside", "settings.json")
	if got != want {
		t.Errorf("defaultPath = %q, want %q", got, want)
	}
}

func TestDefaultPathFallsBackToDotConfig(t *testing.T) {
	home := sandboxHome(t)

	got, err := defaultPath()
	if err != nil {
		t.Fatalf("defaultPath: %v", err)
	}
	// The point of the exercise: on macOS this must not be Library/Application
	// Support, which is what os.UserConfigDir would hand back.
	want := filepath.Join(home, ".config", "k8sdockside", "settings.json")
	if got != want {
		t.Errorf("defaultPath = %q, want %q", got, want)
	}
}

func TestDefaultPathRejectsARelativeXDGConfigHome(t *testing.T) {
	sandboxHome(t)
	t.Setenv("XDG_CONFIG_HOME", "relative/config")

	got, err := defaultPath()
	if err == nil {
		t.Fatalf("defaultPath = %q, want an error for a relative XDG_CONFIG_HOME", got)
	}
}

func TestLegacyPathIsTheStdlibLocation(t *testing.T) {
	home := sandboxHome(t)

	got, err := legacyPath()
	if err != nil {
		t.Fatalf("legacyPath: %v", err)
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(dir, "k8sdockside", "settings.json")
	if got != want {
		t.Errorf("legacyPath = %q, want %q", got, want)
	}
	if runtime.GOOS == "darwin" && got != filepath.Join(home, "Library", "Application Support", "k8sdockside", "settings.json") {
		t.Errorf("legacyPath = %q, want the Application Support location on macOS", got)
	}
}

// writeSettings drops a settings file with a recognisable alias in it, so a
// migration test can tell which of two files it ended up reading.
func writeSettings(t *testing.T, path, alias string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	body := `{"contexts":{"cfg::prod":{"alias":"` + alias + `","color":"#b8384b"}}}`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestMigrateMovesTheLegacyFile(t *testing.T) {
	dir := t.TempDir()
	legacy := filepath.Join(dir, "legacy", "k8sdockside", "settings.json")
	path := filepath.Join(dir, "new", "k8sdockside", "settings.json")
	writeSettings(t, legacy, "Production")

	if err := migrate(legacy, path); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	moved, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading the migrated file: %v", err)
	}
	if !strings.Contains(string(moved), "Production") {
		t.Errorf("migrated file = %s, want the legacy contents", moved)
	}
	if _, err := os.Stat(legacy); !os.IsNotExist(err) {
		t.Error("the legacy file should be gone once it has been moved")
	}
	if info, err := os.Stat(path); err != nil {
		t.Fatal(err)
	} else if info.Mode().Perm() != 0o600 {
		t.Errorf("migrated file mode = %v, want 0600", info.Mode().Perm())
	}
}

func TestMigrateRemovesTheEmptiedLegacyDirectory(t *testing.T) {
	dir := t.TempDir()
	legacy := filepath.Join(dir, "legacy", "k8sdockside", "settings.json")
	path := filepath.Join(dir, "new", "k8sdockside", "settings.json")
	writeSettings(t, legacy, "Production")

	if err := migrate(legacy, path); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	if _, err := os.Stat(filepath.Dir(legacy)); !os.IsNotExist(err) {
		t.Error("the emptied legacy directory should be cleaned up")
	}
}

func TestMigrateNeverClobbersAnExistingFile(t *testing.T) {
	dir := t.TempDir()
	legacy := filepath.Join(dir, "legacy", "k8sdockside", "settings.json")
	path := filepath.Join(dir, "new", "k8sdockside", "settings.json")
	writeSettings(t, legacy, "Stale")
	writeSettings(t, path, "Current")

	if err := migrate(legacy, path); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	kept, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(kept), "Current") {
		t.Errorf("file at the new path = %s, want it left untouched", kept)
	}
	// The legacy file was not moved, so it is not ours to delete either.
	if _, err := os.Stat(legacy); err != nil {
		t.Error("a legacy file that was not migrated should be left alone")
	}
}

func TestMigrateIsANoOpWhenThePathsAreTheSame(t *testing.T) {
	path := filepath.Join(t.TempDir(), "k8sdockside", "settings.json")
	writeSettings(t, path, "Production")

	if err := migrate(path, path); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	// On Linux with no XDG_CONFIG_HOME the legacy and new paths are identical;
	// treating that as a move would delete the only copy.
	kept, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("the file must survive: %v", err)
	}
	if !strings.Contains(string(kept), "Production") {
		t.Errorf("file = %s, want it untouched", kept)
	}
}

func TestMigrateIsANoOpWithNothingToMigrate(t *testing.T) {
	dir := t.TempDir()
	legacy := filepath.Join(dir, "legacy", "settings.json")
	path := filepath.Join(dir, "new", "settings.json")

	if err := migrate(legacy, path); err != nil {
		t.Errorf("migrate with no legacy file = %v, want no error", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("migrate should not create a file out of nothing")
	}
}

func TestOpenMigratesSettingsFromTheLegacyLocation(t *testing.T) {
	sandboxHome(t)

	legacy, err := legacyPath()
	if err != nil {
		t.Fatal(err)
	}
	writeSettings(t, legacy, "Production")

	store, err := Open()
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	if got := store.Get().Contexts["cfg::prod"].Alias; got != "Production" {
		t.Errorf("alias = %q, want the alias carried over from the legacy file", got)
	}
	want, err := defaultPath()
	if err != nil {
		t.Fatal(err)
	}
	if store.Path() != want {
		t.Errorf("Path = %q, want %q", store.Path(), want)
	}
	if _, err := os.Stat(legacy); !os.IsNotExist(err) {
		t.Error("the legacy file should have been moved, not copied")
	}
}
