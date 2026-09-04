package kube

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Ping is the reachability check behind the sidebar's connection indicator, so
// unlike the rest of the live layer it has to be testable without a cluster:
// these run against an httptest server standing in for an API server.

// kubeconfigFor writes a single-context kubeconfig pointing at server and
// returns the parsed context.
func kubeconfigFor(t *testing.T, server string) Context {
	t.Helper()

	path := filepath.Join(t.TempDir(), "config")
	body := fmt.Sprintf(`apiVersion: v1
kind: Config
current-context: test
clusters:
  - name: test
    cluster:
      server: %s
contexts:
  - name: test
    context:
      cluster: test
      user: test
users:
  - name: test
    user: {}
`, server)

	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("writing kubeconfig: %v", err)
	}

	file := ParseFile(path, SourceManual)
	if file.Error != "" {
		t.Fatalf("parsing the kubeconfig we just wrote: %s", file.Error)
	}
	if len(file.Contexts) != 1 {
		t.Fatalf("contexts = %d, want 1", len(file.Contexts))
	}
	return file.Contexts[0]
}

func TestPingReachesAServerThatAnswers(t *testing.T) {
	var asked string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"major":"1","minor":"31","gitVersion":"v1.31.0"}`)
	}))
	defer srv.Close()

	w := NewWatcher(func(Snapshot) {})
	defer w.Close()

	if err := w.Ping(kubeconfigFor(t, srv.URL)); err != nil {
		t.Fatalf("Ping: %v", err)
	}
	if asked != "/version" {
		t.Errorf("probed %q, want /version -- the cheapest call that proves TLS and credentials", asked)
	}
}

func TestPingFailsWhenNothingIsListening(t *testing.T) {
	// A server started and immediately stopped leaves a port nobody is on,
	// which is exactly the "connection refused" the sidebar has to show.
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := srv.URL
	srv.Close()

	w := NewWatcher(func(Snapshot) {})
	defer w.Close()

	err := w.Ping(kubeconfigFor(t, url))
	if err == nil {
		t.Fatal("Ping succeeded against a closed port, want an error")
	}
	if !strings.Contains(err.Error(), "connection refused") {
		t.Errorf("error = %q, want it to carry the underlying connection failure", err)
	}
}

func TestPingReportsAnUnauthorisedServer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"kind":"Status","code":401,"message":"Unauthorized"}`, http.StatusUnauthorized)
	}))
	defer srv.Close()

	w := NewWatcher(func(Snapshot) {})
	defer w.Close()

	err := w.Ping(kubeconfigFor(t, srv.URL))
	if err == nil {
		t.Fatal("Ping succeeded against a 401, want an error -- reachable is not the same as usable")
	}
}

// Ping must not leak the client it borrowed, or a context probed from the
// sidebar would keep a connection open for the life of the app.
func TestPingReleasesTheClusterItBorrowed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, `{"major":"1","minor":"31"}`)
	}))
	defer srv.Close()

	w := NewWatcher(func(Snapshot) {})
	defer w.Close()

	kc := kubeconfigFor(t, srv.URL)
	if err := w.Ping(kc); err != nil {
		t.Fatalf("Ping: %v", err)
	}

	w.mu.Lock()
	held := len(w.clusters)
	w.mu.Unlock()

	if held != 0 {
		t.Errorf("watcher still holds %d cluster(s) after a ping, want 0", held)
	}
}
