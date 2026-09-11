package plugins

import (
	_ "embed"
	"fmt"
	"strings"
	"sync"

	"github.com/rogerwesterbo/k8sdockside/internal/addons"
	"github.com/rogerwesterbo/k8sdockside/internal/kube"
)

// Plugins that are not built in are found somewhere. Most people will not go
// looking for a repository address, so the app carries a short list of the
// plugins it knows are out there -- each with the repository it is installed
// from and the kinds that give its product away in a cluster -- and the
// settings view offers them with one button each. The sidebar can then say
// "cert-manager is running here, and there is a plugin for it" about a cluster
// that has it.
//
// The list is data compiled into the app rather than fetched: it changes when
// a plugin is written, not every day, and asking a server what exists would be
// the app phoning home on every launch. A plugin that is not on the list is
// installed exactly as before, from its address.

//go:embed known.json
var knownRaw []byte

// Known is a plugin kept in a repository of its own that this app knows of.
type Known struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Tagline     string `json:"tagline,omitzero"`
	Icon        string `json:"icon,omitzero"`
	Description string `json:"description,omitzero"`
	// Repo is what installing it clones. https, so it needs no key.
	Repo string `json:"repo"`
	// Detect are kinds whose presence in a cluster means the product the
	// plugin is about is running there. Empty for one that works anywhere,
	// which is then never suggested for a cluster in particular.
	Detect []string `json:"detect,omitzero"`
	// Links point at what the plugin is about, as on an installed plugin.
	Links []Link `json:"links,omitzero"`
	// Official is one kept alongside the app, by its author.
	Official bool `json:"official,omitzero"`
}

// KnownPlugins is the list, in the order it is offered. A mistake in it is a
// mistake in our own data, which a test catches, so it is fatal.
var KnownPlugins = sync.OnceValue(func() []Known {
	var list []Known
	if err := decodeStrict(knownRaw, &list); err != nil {
		panic(fmt.Sprintf("plugins: known.json: %v", describe(knownRaw, err, "", true)))
	}
	seen := map[string]bool{}
	for i, k := range list {
		k, err := validateKnown(k)
		if err != nil {
			panic(fmt.Sprintf("plugins: known.json: %v", err))
		}
		if seen[k.ID] {
			panic(fmt.Sprintf("plugins: known.json lists %q twice", k.ID))
		}
		seen[k.ID] = true
		list[i] = k
	}
	return list
})

func validateKnown(k Known) (Known, error) {
	if !addons.ValidID(k.ID) {
		return k, fmt.Errorf("%q is not a plugin id", k.ID)
	}
	if strings.TrimSpace(k.Name) == "" {
		k.Name = k.ID
	}
	if err := checkIcon(k.ID, "itself", k.Icon); err != nil {
		return k, err
	}
	if k.Icon == "" {
		k.Icon = "puzzle"
	}
	if !strings.HasPrefix(k.Repo, "https://") || !ValidGitURL(k.Repo) {
		return k, fmt.Errorf("%s: repo %q must be an https repository address", k.ID, k.Repo)
	}
	for _, kind := range k.Detect {
		if !kube.IsKnownKind(kind) {
			return k, fmt.Errorf("%s: detects %q, which is not a kind this app can open", k.ID, kind)
		}
	}
	// Links are held to the plugin rule, which needs a plugin to report
	// against.
	p := Plugin{ID: k.ID, Links: k.Links}
	if err := validateLinks(&p); err != nil {
		return k, err
	}
	k.Links = p.Links
	return k, nil
}

// KnownOffer is a known plugin as the settings view lists it: whether it is
// already installed here, and from where.
type KnownOffer struct {
	Known
	// Installed is true when a plugin with this id is in the catalogue,
	// whoever installed it and however.
	Installed bool `json:"installed"`
}

// Offer lists the known plugins against what the catalogue already has.
func (c Catalogue) Offer() []KnownOffer {
	out := make([]KnownOffer, 0, len(KnownPlugins()))
	for _, k := range KnownPlugins() {
		_, installed := c.Find(k.ID)
		out = append(out, KnownOffer{Known: k, Installed: installed})
	}
	return out
}

// FindKnown returns the known plugin with the given id.
func FindKnown(id string) (Known, bool) {
	for _, k := range KnownPlugins() {
		if k.ID == id {
			return k, true
		}
	}
	return Known{}, false
}
