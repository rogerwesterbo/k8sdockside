package kube

import (
	"os"
	"path/filepath"
	"testing"
)

const twoContexts = `apiVersion: v1
kind: Config
current-context: staging
clusters:
- name: staging
  cluster:
    server: https://10.0.0.1:6443
- name: prod
  cluster:
    server: https://10.0.0.2:6443
contexts:
- name: staging
  context:
    cluster: staging
    user: staging-admin
    namespace: apps
- name: prod
  context:
    cluster: prod
    user: prod-admin
users:
- name: staging-admin
- name: prod-admin
`

func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestParseFileFlattensContexts(t *testing.T) {
	path := writeFile(t, t.TempDir(), "config", twoContexts)

	f := ParseFile(path, SourceDefault)
	if f.Error != "" {
		t.Fatalf("unexpected error: %s", f.Error)
	}
	if len(f.Contexts) != 2 {
		t.Fatalf("got %d contexts, want 2", len(f.Contexts))
	}

	staging := f.Contexts[0]
	if staging.Name != "staging" {
		t.Errorf("Name = %q, want staging", staging.Name)
	}
	// The server lives on the cluster, not the context: flattening it is the
	// whole point of ParseFile.
	if staging.Server != "https://10.0.0.1:6443" {
		t.Errorf("Server = %q, want the staging cluster's server", staging.Server)
	}
	if staging.Namespace != "apps" {
		t.Errorf("Namespace = %q, want apps", staging.Namespace)
	}
	if !staging.Current {
		t.Error("staging should be marked as the current context")
	}
	if f.Contexts[1].Current {
		t.Error("prod is not the current context")
	}
	if staging.ID != ContextID(path, "staging") {
		t.Errorf("ID = %q, want it derived from path and name", staging.ID)
	}
}

func TestParseFileRejectsNonKubeconfigs(t *testing.T) {
	dir := t.TempDir()

	cases := []struct {
		name    string
		content string
	}{
		{"not yaml", "{{{ this is not yaml"},
		{"wrong kind", "apiVersion: v1\nkind: Secret\ndata: {}\n"},
		{"no contexts", "apiVersion: v1\nkind: Config\nclusters: []\n"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := writeFile(t, dir, tc.name+".yaml", tc.content)
			if f := ParseFile(path, SourceManual); f.Error == "" {
				t.Fatalf("expected an error, got %d contexts", len(f.Contexts))
			}
		})
	}
}

func TestParseFileReportsMissingFile(t *testing.T) {
	f := ParseFile(filepath.Join(t.TempDir(), "absent"), SourceManual)
	if f.Error == "" {
		t.Fatal("expected an error for a file that does not exist")
	}
	// Callers render the file even when it is broken, so the path must survive.
	if f.Path == "" {
		t.Error("Path should be preserved on failure")
	}
}

func TestDiscoverFindsEverySource(t *testing.T) {
	home := t.TempDir()
	elsewhere := t.TempDir()
	t.Setenv("HOME", home)

	def := writeFile(t, filepath.Join(home, ".kube"), "config", twoContexts)
	scanned := writeFile(t, filepath.Join(home, ".kube"), "extra.yaml", twoContexts)
	fromEnv := writeFile(t, elsewhere, "env-config", twoContexts)
	manual := writeFile(t, elsewhere, "manual.yaml", twoContexts)
	t.Setenv("KUBECONFIG", fromEnv)

	// Directories under ~/.kube must not be opened as config files.
	if err := os.MkdirAll(filepath.Join(home, ".kube", "cache"), 0o755); err != nil {
		t.Fatal(err)
	}

	sources := map[string]string{}
	for _, f := range Discover([]string{manual}) {
		sources[f.Path] = f.Source
	}

	want := map[string]string{
		def:     SourceDefault,
		fromEnv: SourceEnv,
		manual:  SourceManual,
		scanned: SourceScan,
	}
	for path, source := range want {
		if got := sources[path]; got != source {
			t.Errorf("%s: source = %q, want %q", filepath.Base(path), got, source)
		}
	}
	if len(sources) != len(want) {
		t.Errorf("found %d files, want %d: %v", len(sources), len(want), sources)
	}
}

func TestDiscoverDeduplicatesAndPrefersTheFirstSource(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	def := writeFile(t, filepath.Join(home, ".kube"), "config", twoContexts)
	// The same file named three ways: directly, through $KUBECONFIG, and as a
	// symlink. All three must collapse into one entry, labelled "default".
	link := filepath.Join(home, ".kube", "linked.yaml")
	if err := os.Symlink(def, link); err != nil {
		t.Fatal(err)
	}
	t.Setenv("KUBECONFIG", def)

	files := Discover([]string{link})
	if len(files) != 1 {
		t.Fatalf("got %d files, want 1: %v", len(files), files)
	}
	if files[0].Source != SourceDefault {
		t.Errorf("source = %q, want %q", files[0].Source, SourceDefault)
	}
}

func TestDiscoverSkipsJunkInKubeDirButReportsNamedFiles(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("KUBECONFIG", "")

	writeFile(t, filepath.Join(home, ".kube"), "config", twoContexts)
	// A stray YAML file that is not a kubeconfig: the user never asked for it,
	// so it should be dropped rather than shown as an error.
	writeFile(t, filepath.Join(home, ".kube"), "notes.yaml", "hello: world\n")
	broken := writeFile(t, t.TempDir(), "broken.yaml", "hello: world\n")

	files := Discover([]string{broken})

	var scanned, manual int
	for _, f := range files {
		switch f.Source {
		case SourceScan:
			scanned++
		case SourceManual:
			manual++
			if f.Error == "" {
				t.Error("a file the user added should report why it failed")
			}
		}
	}
	if scanned != 0 {
		t.Errorf("got %d scanned files, want 0", scanned)
	}
	if manual != 1 {
		t.Errorf("got %d manual files, want 1", manual)
	}
}

func TestResolveExpandsHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	if got, want := resolve("~/.kube/config"), filepath.Join(home, ".kube", "config"); got != want {
		t.Errorf("resolve(~) = %q, want %q", got, want)
	}
	if resolve("   ") != "" {
		t.Error("a blank path should resolve to nothing")
	}
}
