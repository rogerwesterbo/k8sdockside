package appconfig

import (
	"os"
	"strings"
	"testing"
)

func TestHelmDefaultsToFindingHelmItself(t *testing.T) {
	got := openIn(t).Get().Preferences.Helm

	// Empty is the right default and not an omission: on Linux and Windows helm
	// is normally on PATH, and a path guessed here would be wrong more often
	// than the search is.
	if got.Path != "" {
		t.Errorf("path = %q, want empty -- the default is to look for it", got.Path)
	}
	// helm's own defaults, so a release changed from this app behaves the way
	// the same command would from a shell.
	if got.Wait || got.Atomic {
		t.Errorf("waiting = %v / atomic = %v, want both off", got.Wait, got.Atomic)
	}
	if got.TimeoutSeconds != DefaultHelmTimeout {
		t.Errorf("timeout = %d, want %d", got.TimeoutSeconds, DefaultHelmTimeout)
	}
}

func TestHelmSettingsSurviveAReopen(t *testing.T) {
	path := tempSettings(t)
	store, err := openAt(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	prefs := store.Get().Preferences
	prefs.Helm = Helm{Path: "/opt/homebrew/bin/helm", Wait: true, TimeoutSeconds: 900}
	if _, err := store.SetPreferences(prefs); err != nil {
		t.Fatalf("SetPreferences: %v", err)
	}

	reopened, err := openAt(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	got := reopened.Get().Preferences.Helm

	if got.Path != "/opt/homebrew/bin/helm" {
		t.Errorf("path = %q", got.Path)
	}
	if !got.Wait || got.TimeoutSeconds != 900 {
		t.Errorf("helm = %+v, want the saved wait and timeout", got)
	}
}

// The whole reason the path is a setting: an app launched from Finder does not
// see the PATH the user's helm is on, so they type it, and it has to survive.
func TestAPathWithSpaceAroundItIsTidiedRatherThanBroken(t *testing.T) {
	path := tempSettings(t)
	store, err := openAt(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	prefs := store.Get().Preferences
	prefs.Helm = Helm{Path: "  /usr/local/bin/helm\n", TimeoutSeconds: DefaultHelmTimeout}
	saved, err := store.SetPreferences(prefs)
	if err != nil {
		t.Fatalf("SetPreferences: %v", err)
	}

	if got := saved.Preferences.Helm.Path; got != "/usr/local/bin/helm" {
		t.Errorf("path = %q, want it trimmed", got)
	}
}

// A path that is not there is still the user's answer. It is reported where
// helm is looked for, which is somewhere there is room to say what to do about
// it -- not silently dropped here, on every read, including on a machine the
// settings file was only synced to.
func TestAPathThatIsNotThereIsKeptRatherThanCleared(t *testing.T) {
	path := tempSettings(t)
	store, err := openAt(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	prefs := store.Get().Preferences
	prefs.Helm = Helm{Path: "/nowhere/at/all/helm", TimeoutSeconds: DefaultHelmTimeout}
	saved, err := store.SetPreferences(prefs)
	if err != nil {
		t.Fatalf("SetPreferences: %v", err)
	}

	if got := saved.Preferences.Helm.Path; got != "/nowhere/at/all/helm" {
		t.Errorf("path = %q, want it kept as written", got)
	}
}

func TestAHandEditedHelmTimeoutIsRepaired(t *testing.T) {
	for _, written := range []int{0, -1, 5, 100000} {
		if got := normaliseHelm(Helm{TimeoutSeconds: written}).TimeoutSeconds; got != DefaultHelmTimeout {
			t.Errorf("timeout %d normalised to %d, want %d", written, got, DefaultHelmTimeout)
		}
	}
	// Anything inside the bounds is the user's choice and is left alone.
	if got := normaliseHelm(Helm{TimeoutSeconds: 900}).TimeoutSeconds; got != 900 {
		t.Errorf("timeout = %d, want the chosen 900", got)
	}
}

// --atomic waits whether or not waiting was asked for, so recording it as it
// will behave keeps the settings view honest: no box sitting off beside a flag
// that turns it on.
func TestAtomicImpliesWaiting(t *testing.T) {
	if got := normaliseHelm(Helm{Atomic: true, TimeoutSeconds: DefaultHelmTimeout}); !got.Wait {
		t.Error("atomic did not imply waiting")
	}
}

// A settings file written before helm existed in this app has none of these
// fields, and must come back with something that runs rather than a timeout of
// zero seconds.
func TestAnOlderFileGainsWorkingHelmSettings(t *testing.T) {
	path := tempSettings(t)
	if err := os.WriteFile(path, []byte(`{"preferences":{"theme":"dockside"}}`), 0o600); err != nil {
		t.Fatal(err)
	}

	store, err := openAt(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	got := store.Get().Preferences.Helm

	if got.TimeoutSeconds != DefaultHelmTimeout {
		t.Errorf("timeout = %d, want %d", got.TimeoutSeconds, DefaultHelmTimeout)
	}
	if got.Path != "" {
		t.Errorf("path = %q, want empty", got.Path)
	}
}

// The defaults are omitzero, so a settings file for someone who has never
// opened the Helm section does not gain a block of noise saying so.
func TestUntouchedHelmSettingsWriteNothingToTheFile(t *testing.T) {
	path := tempSettings(t)
	store, err := openAt(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if _, err := store.AddManualFile("/home/u/.kube/config"); err != nil {
		t.Fatalf("AddManualFile: %v", err)
	}

	written, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(written), "\"path\":\"\"") {
		t.Errorf("an untouched helm path was written to the file:\n%s", written)
	}
}
