package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rogerwesterbo/k8sdockside/internal/appconfig"
	"github.com/rogerwesterbo/k8sdockside/internal/updates"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// releasesEndpoint stands in for GitHub, answering with whatever tag the test
// has set and counting how often it was asked.
type releasesEndpoint struct {
	srv   *httptest.Server
	tag   atomic.Value
	fail  atomic.Bool
	asked atomic.Int32
}

func newReleasesEndpoint(t *testing.T, tag string) *releasesEndpoint {
	t.Helper()
	e := &releasesEndpoint{}
	e.tag.Store(tag)
	e.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		e.asked.Add(1)
		if e.fail.Load() {
			http.Error(w, "down", http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"tag_name":%q,"name":%q,"published_at":"2026-09-05T16:02:10Z"}`, e.tag.Load(), e.tag.Load())
	}))
	t.Cleanup(e.srv.Close)
	return e
}

// updateServiceFor builds the service against a throwaway settings file and
// the fake endpoint, reporting itself as the given version.
func updateServiceFor(t *testing.T, current string, endpoint *releasesEndpoint) (*UpdateService, *appconfig.Store) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	store, err := appconfig.Open()
	if err != nil {
		t.Fatal(err)
	}
	s := NewUpdateService(store)
	s.current = current
	s.checker = updates.New(current)
	s.checker.URL = endpoint.srv.URL
	return s, store
}

func TestNothingIsKnownBeforeTheFirstCheck(t *testing.T) {
	s, _ := updateServiceFor(t, "v0.0.2", newReleasesEndpoint(t, "v0.0.3"))

	got := s.Status()

	if got.Current != "v0.0.2" || got.Latest != nil || got.Newer || got.Unread || got.CheckedAt != "" || got.Error != "" {
		t.Errorf("status before any check = %+v", got)
	}
}

func TestCheckReportsANewerReleaseAsUnread(t *testing.T) {
	s, _ := updateServiceFor(t, "v0.0.2", newReleasesEndpoint(t, "v0.0.3"))

	got := s.Check()

	if got.Latest == nil || got.Latest.Version != "v0.0.3" {
		t.Fatalf("latest = %+v, want v0.0.3", got.Latest)
	}
	if !got.Newer || !got.Unread {
		t.Errorf("newer = %v, unread = %v, want both", got.Newer, got.Unread)
	}
	if got.CheckedAt == "" || got.Error != "" {
		t.Errorf("checkedAt = %q, error = %q", got.CheckedAt, got.Error)
	}
	if want := updates.ReleasesPage + "/tag/v0.0.3"; got.Latest.URL != want {
		t.Errorf("url = %q, want %q", got.Latest.URL, want)
	}
}

func TestTheReleaseYouAreOnIsNotNews(t *testing.T) {
	s, _ := updateServiceFor(t, "v0.0.3", newReleasesEndpoint(t, "v0.0.3"))

	got := s.Check()

	if got.Latest == nil || got.Newer || got.Unread {
		t.Errorf("status = %+v, want the release known but nothing to say about it", got)
	}
}

// A development build is what every contributor runs, and it postdates every
// release. Telling it to upgrade would be wrong every time.
func TestADevelopmentBuildIsToldWhatIsOutButNeverToUpgrade(t *testing.T) {
	s, _ := updateServiceFor(t, "development build", newReleasesEndpoint(t, "v9.9.9"))

	got := s.Check()

	if got.Latest == nil || got.Latest.Version != "v9.9.9" {
		t.Fatalf("latest = %+v, want the release known -- the About page shows it", got.Latest)
	}
	if got.Newer || got.Unread {
		t.Errorf("newer = %v, unread = %v, want neither", got.Newer, got.Unread)
	}
}

// The point of the bell: marking a notice as read puts it away for that
// release only, and it stays away after a restart.
func TestMarkReadQuietensThatReleaseAndOnlyThatRelease(t *testing.T) {
	endpoint := newReleasesEndpoint(t, "v0.0.3")
	s, store := updateServiceFor(t, "v0.0.2", endpoint)
	s.Check()

	got, err := s.MarkRead()
	if err != nil {
		t.Fatalf("MarkRead: %v", err)
	}
	if !got.Newer || got.Unread {
		t.Errorf("after marking read: newer = %v, unread = %v; want still newer, no longer unread", got.Newer, got.Unread)
	}
	if store.Get().Updates.ReadVersion != "v0.0.3" {
		t.Errorf("readVersion = %q, want v0.0.3 on disk", store.Get().Updates.ReadVersion)
	}

	// A fresh service over the same settings -- the app restarted -- agrees.
	again := NewUpdateService(store)
	again.current = "v0.0.2"
	again.checker.URL = endpoint.srv.URL
	if got := again.Check(); got.Unread {
		t.Error("a release marked as read rang the bell again after a restart")
	}

	// The next release is news again.
	endpoint.tag.Store("v0.0.4")
	if got := s.Check(); !got.Unread {
		t.Error("a newer release than the one marked read did not ring the bell")
	}
}

func TestMarkReadWithNothingKnownChangesNothing(t *testing.T) {
	s, store := updateServiceFor(t, "v0.0.2", newReleasesEndpoint(t, "v0.0.3"))

	got, err := s.MarkRead()
	if err != nil {
		t.Fatalf("MarkRead: %v", err)
	}
	if got.Latest != nil || store.Get().Updates.ReadVersion != "" {
		t.Errorf("marking nothing as read wrote %q", store.Get().Updates.ReadVersion)
	}
}

// What was known this morning is still true this afternoon, whatever GitHub
// is doing now. The failure is reported beside it, not instead of it.
func TestAFailedCheckKeepsTheLastKnownRelease(t *testing.T) {
	endpoint := newReleasesEndpoint(t, "v0.0.3")
	s, _ := updateServiceFor(t, "v0.0.2", endpoint)
	s.Check()

	endpoint.fail.Store(true)
	got := s.Check()

	if got.Error == "" {
		t.Error("a failed check reported no error")
	}
	if got.Latest == nil || got.Latest.Version != "v0.0.3" || !got.Newer {
		t.Errorf("status after a failure = %+v, want v0.0.3 still known and still newer", got)
	}

	// And the next success clears it.
	endpoint.fail.Store(false)
	if got := s.Check(); got.Error != "" {
		t.Errorf("error = %q after a check that succeeded", got.Error)
	}
}

func TestTheLoopAsksOnlyWhenThePreferenceAllows(t *testing.T) {
	endpoint := newReleasesEndpoint(t, "v0.0.3")
	s, store := updateServiceFor(t, "v0.0.2", endpoint)
	s.delay = 10 * time.Millisecond
	s.every = 10 * time.Millisecond

	off := false
	prefs := store.Get().Preferences
	prefs.CheckForUpdates = &off
	if _, err := store.SetPreferences(prefs); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.loop(ctx)

	time.Sleep(60 * time.Millisecond)
	if n := endpoint.asked.Load(); n != 0 {
		t.Fatalf("GitHub was asked %d times with the check switched off", n)
	}

	// Switching it on takes effect on the next tick, with no restart.
	on := true
	prefs.CheckForUpdates = &on
	if _, err := store.SetPreferences(prefs); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for endpoint.asked.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if endpoint.asked.Load() == 0 {
		t.Fatal("GitHub was never asked after the check was switched on")
	}
	if got := s.Status(); got.Latest == nil || !got.Unread {
		t.Errorf("status after the loop's check = %+v, want the release known and unread", got)
	}
}

func TestShutdownStopsTheLoop(t *testing.T) {
	endpoint := newReleasesEndpoint(t, "v0.0.3")
	s, _ := updateServiceFor(t, "v0.0.2", endpoint)
	s.delay = 5 * time.Millisecond
	s.every = 5 * time.Millisecond

	if err := s.ServiceStartup(context.Background(), applicationServiceOptions()); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for endpoint.asked.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if err := s.ServiceShutdown(); err != nil {
		t.Fatal(err)
	}

	// Let a tick that was already in flight land, then make sure no more do.
	time.Sleep(20 * time.Millisecond)
	before := endpoint.asked.Load()
	time.Sleep(40 * time.Millisecond)
	if after := endpoint.asked.Load(); after != before {
		t.Errorf("GitHub was asked %d more times after shutdown", after-before)
	}
}

// Outside a running app there is no browser to hand the page to, and the
// answer has to be an error rather than a crash: the settings page is one
// click away from this at any time.
func TestOpenReleaseWithoutAnAppFailsRatherThanPanics(t *testing.T) {
	s, _ := updateServiceFor(t, "v0.0.2", newReleasesEndpoint(t, "v0.0.3"))

	if err := s.OpenRelease(); err == nil {
		t.Error("OpenRelease succeeded with no application to open a browser from")
	}
}

// applicationServiceOptions is the empty options Wails would pass; the service
// ignores them.
func applicationServiceOptions() application.ServiceOptions {
	return application.ServiceOptions{}
}
