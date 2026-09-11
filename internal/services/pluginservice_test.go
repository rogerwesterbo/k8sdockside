package services

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/rogerwesterbo/k8sdockside/internal/plugins"
)

// After a clone, "installed" followed by nothing appearing is the outcome that
// leaves the user with nowhere to look, so what became of the folder is said.
func TestInstalledSaysWhatBecameOfAClone(t *testing.T) {
	dest := filepath.Join("/plugins", "k8sdockside-metallb")

	loaded := plugins.Catalogue{Plugins: []plugins.Plugin{
		{ID: "argocd", Origin: plugins.BuiltinOrigin},
		{ID: "metallb", Origin: filepath.Join(dest, "plugin.json")},
	}}
	if err := installed(loaded, dest); err != nil {
		t.Errorf("a clone whose plugin loaded reported %v", err)
	}

	refused := plugins.Catalogue{
		Plugins: []plugins.Plugin{{ID: "argocd", Origin: plugins.BuiltinOrigin}},
		Problems: []plugins.Problem{
			{Path: filepath.Join(dest, "plugin.json"), Message: `plugin "metallb" needs K8s Dockside 0.0.15 or newer`},
			// Another folder's trouble is not this clone's.
			{Path: filepath.Join("/plugins", "k8sdockside-metallb-old", "plugin.json"), Message: "unrelated"},
		},
	}
	err := installed(refused, dest)
	if err == nil || !strings.Contains(err.Error(), "would not load") || !strings.Contains(err.Error(), "0.0.15") {
		t.Errorf("a clone that would not load reported %v", err)
	}
	if err != nil && strings.Contains(err.Error(), "unrelated") {
		t.Errorf("a problem in a neighbouring folder was blamed on this clone: %v", err)
	}

	if err := installed(plugins.Catalogue{}, dest); err == nil || !strings.Contains(err.Error(), "no plugin.json") {
		t.Errorf("a clone with nothing in it reported %v", err)
	}
}
