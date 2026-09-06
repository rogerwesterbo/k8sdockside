package updates

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// github stands in for the releases endpoint, answering however the test says.
func github(t *testing.T, handler http.HandlerFunc) *Checker {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	c := New("v0.0.2")
	c.URL = srv.URL
	return c
}

// release answers the way GitHub does for one published release. The html_url
// deliberately points somewhere else, because the app must not follow it.
func release(tag string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w,
			`{"tag_name":%q,"name":"Release %s","html_url":"https://example.com/not-github","published_at":"2026-09-05T16:02:10Z","draft":false,"prerelease":false}`,
			tag, tag)
	}
}

func TestLatestReadsTheReleaseGitHubAnswersWith(t *testing.T) {
	c := github(t, release("v1.2.0"))

	got, err := c.Latest(context.Background())
	if err != nil {
		t.Fatalf("Latest: %v", err)
	}

	if got.Version != "v1.2.0" || got.Name != "Release v1.2.0" || got.PublishedAt != "2026-09-05T16:02:10Z" {
		t.Errorf("release = %+v", got)
	}
	// The address the browser is sent to is this repository's own page for
	// the tag, whatever the response said it was.
	if want := ReleasesPage + "/tag/v1.2.0"; got.URL != want {
		t.Errorf("url = %q, want %q -- built from the tag, not taken from html_url", got.URL, want)
	}
}

func TestLatestIdentifiesItselfToGitHub(t *testing.T) {
	var agent, accept string
	c := github(t, func(w http.ResponseWriter, r *http.Request) {
		agent = r.Header.Get("User-Agent")
		accept = r.Header.Get("Accept")
		release("v1.0.0")(w, r)
	})

	if _, err := c.Latest(context.Background()); err != nil {
		t.Fatalf("Latest: %v", err)
	}

	// GitHub refuses anonymous clients, and the version is the one thing worth
	// telling it: it lets a broken release be seen in the endpoint's traffic.
	if !strings.HasPrefix(agent, "k8sdockside/v0.0.2") {
		t.Errorf("User-Agent = %q, want the app and its version", agent)
	}
	if accept != "application/vnd.github+json" {
		t.Errorf("Accept = %q", accept)
	}
}

func TestLatestSaysWhenThereIsNoReleaseYet(t *testing.T) {
	c := github(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"message":"Not Found"}`, http.StatusNotFound)
	})

	_, err := c.Latest(context.Background())
	if err == nil || !strings.Contains(err.Error(), "no published release") {
		t.Errorf("err = %v, want it to say there is no release", err)
	}
}

// A 403 with the remaining count at zero is GitHub's way of saying "you have
// asked sixty times this hour", and the sentence has to say that rather than
// "forbidden", which reads as though the app were not allowed to ask at all.
func TestLatestReportsTheRateLimitInWords(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		header string
	}{
		{"a classic 403", http.StatusForbidden, "0"},
		{"the newer 429", http.StatusTooManyRequests, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := github(t, func(w http.ResponseWriter, r *http.Request) {
				if tc.header != "" {
					w.Header().Set("X-RateLimit-Remaining", tc.header)
				}
				http.Error(w, `{"message":"API rate limit exceeded"}`, tc.status)
			})

			_, err := c.Latest(context.Background())
			if err == nil || !strings.Contains(err.Error(), "rate limit") {
				t.Errorf("err = %v, want it to name the rate limit", err)
			}
		})
	}
}

// A 403 that is not the rate limit is reported as the status it is.
func TestLatestReportsAnyOtherFailureByStatus(t *testing.T) {
	c := github(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "down", http.StatusBadGateway)
	})

	_, err := c.Latest(context.Background())
	if err == nil || !strings.Contains(err.Error(), "502") {
		t.Errorf("err = %v, want the status", err)
	}
}

func TestLatestRefusesATagThatIsNotAVersion(t *testing.T) {
	c := github(t, release("nightly"))

	_, err := c.Latest(context.Background())
	if err == nil || !strings.Contains(err.Error(), `"nightly"`) {
		t.Errorf("err = %v, want it to name the tag", err)
	}
}

func TestLatestRefusesAnAnswerThatIsNotJSON(t *testing.T) {
	c := github(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, "<html>maintenance</html>")
	})

	if _, err := c.Latest(context.Background()); err == nil {
		t.Error("Latest accepted an HTML page as a release")
	}
}

func TestLatestGivesUpWhenNothingIsListening(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := srv.URL
	srv.Close()
	c := New("v0.0.2")
	c.URL = url

	_, err := c.Latest(context.Background())
	if err == nil || !strings.Contains(err.Error(), "reaching GitHub") {
		t.Errorf("err = %v, want it to say GitHub could not be reached", err)
	}
}

func TestCompareOrdersVersionsAsSemverDoes(t *testing.T) {
	for _, tc := range []struct {
		a, b string
		want int
	}{
		{"v1.0.0", "v1.0.0", 0},
		{"1.0.0", "v1.0.0", 0},
		{"v1.0.0+build.7", "v1.0.0", 0},
		{"v1.0.1", "v1.0.0", 1},
		{"v1.1.0", "v1.0.9", 1},
		{"v2.0.0", "v1.99.99", 1},
		{"v0.0.2", "v0.0.10", -1},
		// A pre-release comes before its release.
		{"v1.2.0-rc.1", "v1.2.0", -1},
		{"v1.2.0", "v1.2.0-rc.1", 1},
		// ...and after the release before it.
		{"v1.2.0-rc.1", "v1.1.9", 1},
		// Pre-releases of one number: numbers numerically, words as text,
		// numbers before words, and the longer list is the later.
		{"v1.0.0-rc.2", "v1.0.0-rc.10", -1},
		{"v1.0.0-alpha", "v1.0.0-beta", -1},
		{"v1.0.0-1", "v1.0.0-alpha", -1},
		{"v1.0.0-rc.1", "v1.0.0-rc.1.1", -1},
		// Not a version at all: older than anything that is.
		{"development build", "v0.0.1", -1},
		{"development build", "(devel)", 0},
	} {
		if got := Compare(tc.a, tc.b); sign(got) != tc.want {
			t.Errorf("Compare(%q, %q) = %d, want sign %d", tc.a, tc.b, got, tc.want)
		}
		// Antisymmetric, or a sort would not be a sort.
		if got := Compare(tc.b, tc.a); sign(got) != -tc.want {
			t.Errorf("Compare(%q, %q) = %d, want sign %d", tc.b, tc.a, got, -tc.want)
		}
	}
}

func sign(n int) int {
	switch {
	case n < 0:
		return -1
	case n > 0:
		return 1
	}
	return 0
}

func TestIsVersionAcceptsWhatTheReleaseWorkflowWouldTag(t *testing.T) {
	for _, tag := range []string{"v1.2.3", "1.2.3", "v1.2.3-rc.1", "v0.0.1-beta", "v1.2.3+sha.abcdef"} {
		if !IsVersion(tag) {
			t.Errorf("IsVersion(%q) = false", tag)
		}
	}
	for _, tag := range []string{"", "development build", "(devel)", "v1.2", "v1.2.3.4", "v1.2.x", "v1.2.3-", "v1..3", "latest"} {
		if IsVersion(tag) {
			t.Errorf("IsVersion(%q) = true", tag)
		}
	}
}

func TestNewerIsAboutTheBuildYouAreRunning(t *testing.T) {
	for _, tc := range []struct {
		current, latest string
		want            bool
	}{
		{"v0.0.2", "v0.0.3", true},
		{"v0.0.2", "v0.0.2", false},
		{"v0.0.3", "v0.0.2", false},
		// Somebody on a release candidate hears about the release it became.
		{"v1.0.0-rc.1", "v1.0.0", true},
		// A development build has no version to be behind, and must not be
		// nagged about every release it postdates.
		{"development build", "v9.9.9", false},
		// And a tag that is not a version is not news either.
		{"v0.0.2", "nightly", false},
	} {
		if got := Newer(tc.current, tc.latest); got != tc.want {
			t.Errorf("Newer(%q, %q) = %v, want %v", tc.current, tc.latest, got, tc.want)
		}
	}
}
