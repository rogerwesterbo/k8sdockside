package plugins

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

const customPlugin = `{
    "id": "acme",
    "name": "Acme",
    "requires": [{ "kind": "crd:meshes.acme.io" }],
    "ui": { "kinds": ["pods"], "write": true },
    "views": [
        { "id": "map", "label": "Map", "type": "custom" },
        { "id": "second", "label": "Second", "type": "custom", "entry": "pages/second.html" },
        { "id": "meshes", "label": "Meshes", "kind": "crd:meshes.acme.io" }
    ]
}`

// installCustom writes a plugin with its own views into a fresh plugins folder,
// with a ui folder beside it, and loads it.
func installCustom(t *testing.T) (Catalogue, string) {
	t.Helper()
	dir := t.TempDir()
	pluginDir := filepath.Join(dir, "acme")
	write(t, pluginDir, "plugin.json", customPlugin)
	write(t, filepath.Join(pluginDir, "ui"), "index.html", "<p>map</p>")
	write(t, filepath.Join(pluginDir, "ui", "pages"), "second.html", "<p>second</p>")
	write(t, pluginDir, "secret.txt", "not for the frame")

	cat := Load(dir, nil, nil)
	if len(cat.Problems) > 0 {
		t.Fatalf("problems loading: %v", cat.Problems)
	}
	return cat, dir
}

func TestACustomViewLoadsWithItsDefaults(t *testing.T) {
	cat, _ := installCustom(t)
	p, ok := cat.Find("acme")
	if !ok {
		t.Fatal("acme did not load")
	}
	v, _ := p.View("map")
	if v.Type != ViewCustom || v.Entry != DefaultEntry {
		t.Errorf("map view = %+v, want a custom view opening %s", v, DefaultEntry)
	}
	if p.UI == nil || p.UI.Dir != DefaultUIDir {
		t.Fatalf("ui = %+v, want the default folder", p.UI)
	}
	// Everything the plugin names, plus what its UI asks for -- once each.
	want := []string{"crd:meshes.acme.io", "pods"}
	if !slices.Equal(p.UI.Readable, want) {
		t.Errorf("readable = %v, want %v", p.UI.Readable, want)
	}
	if !p.CanWrite("pods") || p.CanRead("secrets") || p.CanRead("deployments") {
		t.Errorf("permissions wrong: write pods %v, read secrets %v, read deployments %v",
			p.CanWrite("pods"), p.CanRead("secrets"), p.CanRead("deployments"))
	}

	resolved, err := cat.ResolveKind(ViewKind("acme", "second"))
	if err != nil {
		t.Fatal(err)
	}
	if !resolved.Custom || resolved.Entry != "pages/second.html" || resolved.Kind != "" {
		t.Errorf("resolved = %+v", resolved)
	}
}

func TestACustomViewWithoutAUIBlockGetsOne(t *testing.T) {
	p, err := validate(Plugin{ID: "x", Views: []View{{ID: "v", Type: ViewCustom}}})
	if err != nil {
		t.Fatal(err)
	}
	if p.UI == nil || p.UI.Dir != DefaultUIDir || p.UI.Write {
		t.Errorf("ui = %+v, want the read-only default", p.UI)
	}
}

func TestValidateRefusesBadCustomViews(t *testing.T) {
	cases := map[string]Plugin{
		"entry escapes":       {ID: "x", Views: []View{{ID: "v", Type: ViewCustom, Entry: "../other/index.html"}}},
		"absolute entry":      {ID: "x", Views: []View{{ID: "v", Type: ViewCustom, Entry: "/etc/passwd"}}},
		"custom with a kind":  {ID: "x", Views: []View{{ID: "v", Type: ViewCustom, Kind: "pods"}}},
		"table with an entry": {ID: "x", Views: []View{{ID: "v", Kind: "pods", Entry: "index.html"}}},
		"ui dir escapes":      {ID: "x", UI: &UI{Dir: "../elsewhere"}, Views: []View{{ID: "v", Type: ViewCustom}}},
		"ui reads secrets":    {ID: "x", UI: &UI{Kinds: []string{"secrets"}}, Views: []View{{ID: "v", Type: ViewCustom}}},
		"ui reads nonsense":   {ID: "x", UI: &UI{Kinds: []string{"widgets"}}, Views: []View{{ID: "v", Type: ViewCustom}}},
	}
	for name, p := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := validate(p); err == nil {
				t.Error("accepted")
			}
		})
	}
}

// A secret named anywhere else in the plugin still does not become readable.
func TestSecretsAreNeverReadableByAView(t *testing.T) {
	p, err := validate(Plugin{
		ID:    "x",
		Views: []View{{ID: "s", Kind: "secrets"}, {ID: "v", Type: ViewCustom}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if p.CanRead("secrets") {
		t.Error("a custom view may read secrets")
	}
}

// The example in the repository is what a plugin author copies, so it has to
// keep loading as the format moves.
func TestTheExamplePluginsLoad(t *testing.T) {
	cat := Load(filepath.Join("..", "..", "examples", "plugins"), nil, nil)
	if len(cat.Problems) > 0 {
		t.Fatalf("problems: %v", cat.Problems)
	}
	p, ok := cat.Find("argocd-ui")
	if !ok {
		t.Fatal("argocd-ui did not load")
	}
	if v, _ := p.View("map"); v.Type != ViewCustom {
		t.Errorf("map view = %+v", v)
	}
	if !p.CanRead("crd:applications.argoproj.io") || !p.CanWrite("crd:applications.argoproj.io") {
		t.Errorf("the example cannot read or write its own Applications: %+v", p.UI)
	}

	vms, ok := cat.Find("kubevirt-ui")
	if !ok {
		t.Fatal("kubevirt-ui did not load")
	}
	if len(vms.Actions) != 7 || len(vms.Sections) != 1 {
		t.Errorf("kubevirt-ui has %d actions and %d sections, want 7 and 1", len(vms.Actions), len(vms.Sections))
	}
	// Everything its panel reads has to be declared, or the panel is refused.
	for _, kind := range []string{"crd:virtualmachines.kubevirt.io", "crd:virtualmachineinstances.kubevirt.io", "crd:virtualmachineinstancemigrations.kubevirt.io", "pods", "nodes"} {
		if !vms.CanRead(kind) {
			t.Errorf("kubevirt-ui's panel cannot read %s", kind)
		}
	}
}

func TestAPluginWhoseUIFolderIsMissingIsRefused(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "acme"), "plugin.json", customPlugin)
	cat := Load(dir, nil, nil)
	if _, ok := cat.Find("acme"); ok {
		t.Error("loaded without its ui folder")
	}
	if len(cat.Problems) != 1 || !strings.Contains(cat.Problems[0].Message, "ui folder") {
		t.Errorf("problems = %v", cat.Problems)
	}
}

func serve(t *testing.T, cat Catalogue, req *http.Request) *httptest.ResponseRecorder {
	t.Helper()
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})
	rec := httptest.NewRecorder()
	Middleware(func() Catalogue { return cat })(next).ServeHTTP(rec, req)
	return rec
}

func TestMiddlewareServesAViewWithItsSandbox(t *testing.T) {
	cat, _ := installCustom(t)

	req := httptest.NewRequest(http.MethodGet, "/plugin-ui/acme/pages/second.html", nil)
	req.Host = "localhost"
	rec := serve(t, cat, req)

	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "second") {
		t.Fatalf("got %d %q", rec.Code, rec.Body.String())
	}
	csp := rec.Header().Get("Content-Security-Policy")
	for _, want := range []string{
		"sandbox allow-scripts",
		"connect-src 'none'",
		"wails://localhost/plugin-ui/acme/",
		"wails://localhost/plugin-ui/_sdk/",
	} {
		if !strings.Contains(csp, want) {
			t.Errorf("policy %q lacks %q", csp, want)
		}
	}
	if strings.Contains(csp, "allow-same-origin") {
		t.Errorf("policy %q gives the frame the app's origin", csp)
	}
}

func TestMiddlewareServesTheEntryForTheBareFolder(t *testing.T) {
	cat, _ := installCustom(t)
	rec := serve(t, cat, httptest.NewRequest(http.MethodGet, "/plugin-ui/acme/", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "map") {
		t.Errorf("got %d %q", rec.Code, rec.Body.String())
	}
}

func TestMiddlewareServesNothingOutsideTheUIFolder(t *testing.T) {
	cat, _ := installCustom(t)
	for _, path := range []string{
		"/plugin-ui/acme/../secret.txt",
		"/plugin-ui/acme/%2e%2e/secret.txt",
		"/plugin-ui/acme/../plugin.json",
		"/plugin-ui/acme/missing.html",
		"/plugin-ui/argocd/index.html", // a built-in has no folder
		"/plugin-ui/nobody/index.html",
		"/plugin-ui/_sdk/other.js",
	} {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.URL.Path = strings.ReplaceAll(path, "%2e", ".")
		rec := serve(t, cat, req)
		if rec.Code == http.StatusOK {
			t.Errorf("%s was served: %q", path, rec.Body.String())
		}
	}
}

func TestMiddlewareServesNothingForADisabledPlugin(t *testing.T) {
	_, dir := installCustom(t)
	cat := Load(dir, nil, []string{"acme"})
	rec := serve(t, cat, httptest.NewRequest(http.MethodGet, "/plugin-ui/acme/index.html", nil))
	if rec.Code != http.StatusNotFound {
		t.Errorf("got %d", rec.Code)
	}
}

func TestMiddlewareServesTheSDK(t *testing.T) {
	rec := serve(t, Catalogue{}, httptest.NewRequest(http.MethodGet, UIPath+sdkDir+"/"+SDKFile, nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "window.k8sdockside") {
		t.Errorf("got %d", rec.Code)
	}
	if !strings.HasPrefix(rec.Header().Get("Content-Type"), "text/javascript") {
		t.Errorf("content type %q", rec.Header().Get("Content-Type"))
	}
}

func TestMiddlewareRefusesTheRuntimeToASandboxedFrame(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/wails/runtime", strings.NewReader(`{"object":0,"method":0}`))
	req.Header.Set("Origin", "null")
	if rec := serve(t, Catalogue{}, req); rec.Code != http.StatusForbidden {
		t.Errorf("sandboxed call got %d", rec.Code)
	}

	// The app's own calls pass through untouched.
	req = httptest.NewRequest(http.MethodPost, "/wails/runtime", strings.NewReader(`{}`))
	req.Header.Set("Origin", "wails://localhost")
	if rec := serve(t, Catalogue{}, req); rec.Code != http.StatusTeapot {
		t.Errorf("the app's own call got %d", rec.Code)
	}
	if rec := serve(t, Catalogue{}, httptest.NewRequest(http.MethodGet, "/index.html", nil)); rec.Code != http.StatusTeapot {
		t.Errorf("an ordinary asset got %d", rec.Code)
	}
}
