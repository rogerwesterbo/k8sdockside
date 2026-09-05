package plugins

import (
	"embed"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/roger/k8sdockside/internal/addons"
)

//go:embed builtin/*.json
var builtinFS embed.FS

// jsonIndent matches the indentation the built-in plugin files are written in,
// so a starter written by the app and one copied out of the repository look the
// same in an editor.
var jsonIndent = jsontext.WithIndent("    ")

// builtinOrder is the order the built-in plugins are offered in: alphabetical
// would be an accident, and this is a judgement about what a Kubernetes user is
// most likely to be looking for.
var builtinOrder = []string{
	"argocd",
	"flux",
	"prometheus",
}

// Builtin returns the plugins that ship with the app, in the order they are
// offered. A failure is a mistake in our own data, which a test catches, so it
// is fatal rather than something every caller has to handle.
var Builtin = sync.OnceValue(func() []Plugin {
	entries, err := builtinFS.ReadDir("builtin")
	if err != nil {
		panic(fmt.Sprintf("plugins: reading embedded builtins: %v", err))
	}

	byID := make(map[string]Plugin, len(entries))
	for _, entry := range entries {
		name := "builtin/" + entry.Name()
		raw, err := builtinFS.ReadFile(name)
		if err != nil {
			panic(fmt.Sprintf("plugins: reading %s: %v", name, err))
		}
		var plugin Plugin
		if err := json.Unmarshal(raw, &plugin); err != nil {
			panic(fmt.Sprintf("plugins: parsing %s: %v", name, err))
		}
		plugin, err = validate(plugin)
		if err != nil {
			panic(fmt.Sprintf("plugins: %s: %v", name, err))
		}
		plugin.Origin = BuiltinOrigin
		byID[plugin.ID] = plugin
	}

	out := make([]Plugin, 0, len(byID))
	for _, id := range builtinOrder {
		plugin, ok := byID[id]
		if !ok {
			panic(fmt.Sprintf("plugins: builtinOrder names %q, which has no file", id))
		}
		out = append(out, plugin)
		delete(byID, id)
	}
	// A file added without being listed would otherwise vanish silently.
	if len(byID) > 0 {
		missing := make([]string, 0, len(byID))
		for id := range byID {
			missing = append(missing, id)
		}
		addons.Sort(missing)
		panic(fmt.Sprintf("plugins: builtin plugins missing from builtinOrder: %s", strings.Join(missing, ", ")))
	}
	return out
})

// Load builds the catalogue: the built-in plugins, plus everything readable in
// dir and in each of the extra folders the user has added.
//
// The finding, the size limits, the one-level-deep rule and what happens to a
// file that will not parse are all in internal/addons, shared with the theme
// loader. What is left here is only what a *plugin* file is.
func Load(dir string, extra []string) Catalogue {
	// The default folder first, so a plugin in one the user added later cannot
	// quietly take an id from the folder we told them to use.
	folders := append([]string{dir}, extra...)
	loaded, problems := addons.Load(Builtin(), folders, parseFile)
	return Catalogue{
		Plugins:  loaded,
		Dir:      dir,
		Folders:  append([]string{}, extra...),
		Problems: problems,
	}
}

// parseFile reads one plugin file, which may hold a single plugin or a pack of
// them. The two are told apart by whether `plugins` is present, so the simplest
// contribution is still one object.
func parseFile(path string, raw []byte) (loaded []Plugin, refused []string, err error) {
	var pack Pack
	if err := json.Unmarshal(raw, &pack); err != nil {
		return nil, nil, fmt.Errorf("not valid JSON: %w", err)
	}

	// A single plugin unmarshals into Pack with no Plugins, so the same bytes
	// are read again into the shape they actually are rather than being sniffed.
	if len(pack.Plugins) == 0 {
		var plugin Plugin
		if err := json.Unmarshal(raw, &plugin); err != nil {
			return nil, nil, fmt.Errorf("not valid JSON: %w", err)
		}
		pack = Pack{Plugins: []Plugin{plugin}}
	}

	for _, plugin := range pack.Plugins {
		plugin, err := validate(plugin)
		if err != nil {
			refused = append(refused, err.Error())
			continue
		}
		plugin.Origin = path
		plugin.Pack = pack.Name
		loaded = append(loaded, plugin)
	}
	if len(loaded) == 0 && len(refused) > 0 {
		return nil, nil, errors.New(strings.Join(refused, "; "))
	}
	return loaded, refused, nil
}

// Example is a starter plugin, written into the plugins folder on request. It
// is a real, working plugin for something almost every cluster has -- rather
// than a skeleton of placeholders -- so the first edit is changing something
// that already appears in the sidebar.
func Example() []byte {
	example := Plugin{
		ID:          "my-solution",
		Name:        "My Solution",
		Tagline:     "a starting point",
		Icon:        "puzzle",
		Docs:        "https://example.com",
		Description: "Replace the kinds below with the ones your solution installs. Every view lists a kind this app already knows how to open: a built-in name like \"deployments\", or \"crd:<plural>.<group>\" for a custom resource.",
		Requires: []Requirement{
			{Kind: "deployments", Label: "Deployments"},
		},
		Views: []View{
			{
				ID:       "workloads",
				Label:    "Workloads",
				Icon:     "rocket",
				Kind:     "deployments",
				Selector: "app.kubernetes.io/part-of=my-solution",
			},
		},
		Cards: []Card{
			{
				Label:    "Workloads",
				Kind:     "deployments",
				Selector: "app.kubernetes.io/part-of=my-solution",
			},
		},
	}

	raw, err := json.Marshal(example, json.Deterministic(true), jsonIndent)
	if err != nil {
		// Marshalling a literal we wrote cannot fail; a panic here would be a
		// bug in this function rather than anything the user did.
		panic(fmt.Sprintf("plugins: rendering the example: %v", err))
	}
	return append(raw, '\n')
}

// WriteExample writes the starter plugin into dir without overwriting one that
// is already there, and returns the path it chose.
func WriteExample(dir string) (string, error) {
	return addons.WriteExample(dir, "my-solution", Example())
}

// EnsureDir creates the plugins folder, so that "open the folder" has something
// to open on a machine where nothing has ever been installed.
func EnsureDir(dir string) error { return addons.EnsureDir(dir) }
