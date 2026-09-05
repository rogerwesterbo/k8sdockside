package plugins

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/roger/k8sdockside/internal/kube"
)

func write(t *testing.T, dir, name, body string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("creating %s: %v", dir, err)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("writing %s: %v", path, err)
	}
	return path
}

func TestBuiltinPluginsLoad(t *testing.T) {
	builtin := Builtin()
	if len(builtin) == 0 {
		t.Fatal("no builtin plugins")
	}

	seen := map[string]bool{}
	for _, p := range builtin {
		if seen[p.ID] {
			t.Errorf("builtin id %q appears twice", p.ID)
		}
		seen[p.ID] = true
		if p.Name == "" || p.Tagline == "" {
			t.Errorf("builtin %q has no name or tagline; both are shown in the sidebar", p.ID)
		}
		if !p.Builtin() {
			t.Errorf("builtin %q has origin %q", p.ID, p.Origin)
		}
		if len(p.Views) == 0 {
			t.Errorf("builtin %q offers no views", p.ID)
		}
		if len(p.Requires) == 0 {
			t.Errorf("builtin %q declares no requirements, so its overview cannot say whether it is installed", p.ID)
		}
	}
	for _, id := range []string{"argocd", "flux", "prometheus"} {
		if !seen[id] {
			t.Errorf("the %s plugin is missing", id)
		}
	}
}

// Every kind a built-in names has to be one the app can actually open, or the
// view is a row that opens onto an error.
func TestBuiltinPluginKindsAreOpenable(t *testing.T) {
	for _, p := range Builtin() {
		for _, v := range p.Views {
			if !kube.IsKnownKind(v.Kind) {
				t.Errorf("%s/%s lists %q, which is not an openable kind", p.ID, v.ID, v.Kind)
			}
		}
		for _, c := range p.Cards {
			if !kube.IsKnownKind(c.Kind) {
				t.Errorf("%s card %q counts %q, which is not an openable kind", p.ID, c.Label, c.Kind)
			}
			if c.GroupBy != "" && !c.GroupBy.Valid() {
				t.Errorf("%s card %q groups by %q, which is not a field path", p.ID, c.Label, c.GroupBy)
			}
		}
	}
}

func TestViewKindRoundTrips(t *testing.T) {
	kind := ViewKind("argocd", "applications")
	if kind != "plugin:argocd/applications" {
		t.Fatalf("ViewKind = %q", kind)
	}
	plugin, view, ok := ParseViewKind(kind)
	if !ok || plugin != "argocd" || view != "applications" {
		t.Fatalf("ParseViewKind(%q) = %q, %q, %v", kind, plugin, view, ok)
	}

	for _, bad := range []string{"pods", "crd:applications.argoproj.io", "plugin:", "plugin:argocd", "plugin:/applications", "plugin:argocd/"} {
		if _, _, ok := ParseViewKind(bad); ok {
			t.Errorf("ParseViewKind(%q) accepted it", bad)
		}
	}
}

func TestResolveKind(t *testing.T) {
	cat := Catalogue{Plugins: Builtin()}

	got, err := cat.ResolveKind("plugin:argocd/components")
	if err != nil {
		t.Fatalf("ResolveKind: %v", err)
	}
	if got.Kind != "deployments" {
		t.Errorf("kind = %q, want the real kind to subscribe to", got.Kind)
	}
	if got.Selector == "" {
		t.Error("the view's selector was dropped, so the tab would list every Deployment in the cluster")
	}
	if got.Overview {
		t.Error("a table view came back marked as the overview")
	}

	// Every plugin has an overview whether or not it declares one.
	overview, err := cat.ResolveKind("plugin:argocd/overview")
	if err != nil {
		t.Fatalf("resolving the overview: %v", err)
	}
	if !overview.Overview || overview.Kind != "" {
		t.Errorf("overview resolved to %+v, want no kind and Overview set", overview)
	}
}

// A tab restored after its plugin's folder was dropped has to say so. An empty
// table would read as "Argo CD has no applications".
func TestResolveKindExplainsAMissingPlugin(t *testing.T) {
	cat := Catalogue{Plugins: Builtin()}

	_, err := cat.ResolveKind("plugin:nothing-here/applications")
	if err == nil {
		t.Fatal("resolving a view of an uninstalled plugin succeeded")
	}
	if !strings.Contains(err.Error(), "nothing-here") {
		t.Errorf("error %q does not name the missing plugin", err)
	}

	if _, err := cat.ResolveKind("plugin:argocd/not-a-view"); err == nil {
		t.Error("resolving a view the plugin does not have succeeded")
	}
}

func TestValidateRefusesBadPlugins(t *testing.T) {
	view := View{ID: "things", Label: "Things", Kind: "pods"}
	cases := map[string]Plugin{
		"no id":              {Name: "x", Views: []View{view}},
		"id in caps":         {ID: "MyPlugin", Views: []View{view}},
		"nothing to show":    {ID: "empty", Name: "Empty"},
		"unknown kind":       {ID: "x", Views: []View{{ID: "v", Kind: "widgets"}}},
		"view with no kind":  {ID: "x", Views: []View{{ID: "v", Label: "V"}}},
		"view id in caps":    {ID: "x", Views: []View{{ID: "V", Kind: "pods"}}},
		"duplicate view ids": {ID: "x", Views: []View{{ID: "v", Kind: "pods"}, {ID: "v", Kind: "nodes"}}},
		"reserved view id":   {ID: "x", Views: []View{{ID: OverviewID, Kind: "pods"}}},
		"view type":          {ID: "x", Views: []View{{ID: "v", Kind: "pods", Type: "chart"}}},
		"nested plugin view": {ID: "x", Views: []View{{ID: "v", Kind: "plugin:other/thing"}}},
		"bad selector":       {ID: "x", Views: []View{{ID: "v", Kind: "pods", Selector: "!!! nope"}}},
		"bad namespace":      {ID: "x", Views: []View{{ID: "v", Kind: "pods", Namespace: "Not A Namespace"}}},
		"bad requirement":    {ID: "x", Views: []View{view}, Requires: []Requirement{{Kind: "widgets"}}},
		"bad field path":     {ID: "x", Views: []View{view}, Cards: []Card{{Kind: "pods", GroupBy: "status.$$$"}}},
		"bad tone":           {ID: "x", Views: []View{view}, Cards: []Card{{Kind: "pods", Tones: map[string]string{"a": "purple"}}}},
		// A plugin file is not allowed to hand the platform an arbitrary scheme
		// to open.
		"docs link scheme": {ID: "x", Views: []View{view}, Docs: "file:///etc/passwd"},
	}
	for name, plugin := range cases {
		if _, err := validate(plugin); err == nil {
			t.Errorf("%s: validate accepted it", name)
		}
	}
}

func TestValidateFillsInWhatItCan(t *testing.T) {
	p, err := validate(Plugin{ID: "minimal", Views: []View{{ID: "pods", Kind: "pods"}}})
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if p.Name != "minimal" {
		t.Errorf("name = %q, want the id", p.Name)
	}
	if p.Icon == "" || p.Views[0].Icon == "" {
		t.Error("an icon was left blank, which renders as nothing")
	}
	if p.Views[0].Label != "pods" {
		t.Errorf("label = %q, want the view id", p.Views[0].Label)
	}
	if p.Views[0].Type != ViewTable {
		t.Errorf("type = %q, want %q", p.Views[0].Type, ViewTable)
	}
}

func TestLoadReadsPluginsAndPacks(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "solo.json", `{"id": "solo", "name": "Solo", "views": [{"id": "pods", "kind": "pods"}]}`)
	write(t, dir, "pack.json", `{
		"name": "Ops Pack",
		"plugins": [
			{"id": "one", "name": "One", "views": [{"id": "pods", "kind": "pods"}]},
			{"id": "two", "name": "Two", "views": [{"id": "nodes", "kind": "nodes"}]}
		]
	}`)
	write(t, filepath.Join(dir, "cloned"), "plugin.json", `{"id": "three", "name": "Three", "views": [{"id": "pods", "kind": "pods"}]}`)

	cat := Load(dir, nil)
	if len(cat.Problems) > 0 {
		t.Fatalf("problems: %v", cat.Problems)
	}
	for _, id := range []string{"solo", "one", "two", "three"} {
		if _, ok := cat.Find(id); !ok {
			t.Errorf("plugin %q did not load", id)
		}
	}
	if p, _ := cat.Find("one"); p.Pack != "Ops Pack" {
		t.Errorf("pack name is %q, want it carried onto the plugin", p.Pack)
	}
	if _, ok := cat.Find("argocd"); !ok {
		t.Error("loading a folder dropped the builtin plugins")
	}
}

func TestLoadReportsBadFilesWithoutLosingGoodOnes(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "broken.json", `{ not json`)
	write(t, dir, "wrong.json", `{"id": "wrong", "views": [{"id": "v", "kind": "widgets"}]}`)
	write(t, dir, "fine.json", `{"id": "fine", "name": "Fine", "views": [{"id": "pods", "kind": "pods"}]}`)

	cat := Load(dir, nil)
	if _, ok := cat.Find("fine"); !ok {
		t.Error("a broken file alongside a good one cost us the good one")
	}
	if len(cat.Problems) != 2 {
		t.Fatalf("got %d problems, want one per bad file: %v", len(cat.Problems), cat.Problems)
	}
}

func TestLoadLetsAUserPluginReplaceABuiltin(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "mine.json", `{"id": "argocd", "name": "My Argo", "views": [{"id": "apps", "kind": "pods"}]}`)

	cat := Load(dir, nil)
	p, ok := cat.Find("argocd")
	if !ok {
		t.Fatal("argocd is missing entirely")
	}
	if p.Name != "My Argo" {
		t.Errorf("name = %q, want the user's own -- overriding a builtin by id is how you retune it", p.Name)
	}
	count := 0
	for _, each := range cat.Plugins {
		if each.ID == "argocd" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("argocd appears %d times, want 1", count)
	}
}

func TestExampleIsAPluginThatLoads(t *testing.T) {
	dir := t.TempDir()
	path, err := WriteExample(dir)
	if err != nil {
		t.Fatalf("WriteExample: %v", err)
	}

	cat := Load(dir, nil)
	if len(cat.Problems) > 0 {
		t.Fatalf("the starter plugin does not load: %v", cat.Problems)
	}
	if _, ok := cat.Find("my-solution"); !ok {
		t.Fatalf("the starter plugin written to %s is not in the catalogue", path)
	}

	again, err := WriteExample(dir)
	if err != nil {
		t.Fatalf("second WriteExample: %v", err)
	}
	if again == path {
		t.Errorf("both starters went to %s", path)
	}
}
