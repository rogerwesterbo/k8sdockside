// Package plugins is the app's second extension point: a solution plugin
// teaches k8sdockside about something installed *in a cluster* -- Argo CD,
// Flux, Prometheus -- and gives it a place of its own in the sidebar rather
// than leaving its custom resources scattered through the definitions tree
// under group names.
//
// Like a theme, a plugin is a JSON file and nothing else. It cannot ship code
// or queries; it names resource kinds the app already knows how to list, and
// says how to arrange and summarise them. What that buys is that installing
// someone else's plugin is as safe as installing their theme, and that a plugin
// written today keeps working as the app grows.
//
// A plugin is installed on *this machine*. Whether the thing it describes is
// installed in the cluster in front of you is a separate question, asked per
// context and answered plainly -- see Summary. A plugin whose CRDs are absent
// is an ordinary state, not a broken install.
package plugins

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/roger/k8sdockside/internal/addons"
	"github.com/roger/k8sdockside/internal/kube"
	"github.com/roger/k8sdockside/internal/metrics"
	"k8s.io/apimachinery/pkg/labels"
)

// Prefix marks a tab opened on one of a plugin's views:
// "plugin:<pluginID>/<viewID>", e.g. "plugin:argocd/applications".
//
// It is a prefixed string for exactly the reason "crd:" is one. A tab's kind is
// persisted, reordered, restored and titled by machinery that never looks
// inside it, so a plugin view becomes a tab without any of that learning a
// second shape. Both sides parse it in one place: here, and the frontend
// catalogue.
const Prefix = "plugin:"

// ViewKind builds the kind string a tab for one view is opened with.
func ViewKind(pluginID, viewID string) string {
	return Prefix + pluginID + "/" + viewID
}

// ParseViewKind splits a "plugin:" kind back into its plugin and view.
func ParseViewKind(kind string) (pluginID, viewID string, ok bool) {
	rest, found := strings.CutPrefix(kind, Prefix)
	if !found {
		return "", "", false
	}
	pluginID, viewID, found = strings.Cut(rest, "/")
	if !found || pluginID == "" || viewID == "" {
		return "", "", false
	}
	return pluginID, viewID, true
}

// The kinds of view a plugin may declare.
const (
	// ViewOverview is the plugin's landing page: what it is, whether this
	// cluster has it, and a live count of what it manages. Every plugin gets
	// one whether or not it asks, because "is this even installed here?" is the
	// first question and it needs somewhere to be answered.
	ViewOverview = "overview"
	// ViewTable is a resource listing, which is every other view.
	ViewTable = "table"
)

// OverviewID is the view id the generated overview takes. It is reserved: a
// plugin declaring a view of its own by this name is refused rather than
// silently shadowed.
const OverviewID = "overview"

// View is one entry under a plugin in the sidebar, and one tab when opened.
type View struct {
	// ID is stable and appears in the tab's kind, so it is what a restored tab
	// is found by. Renaming one loses its place in a saved session, which is
	// why it is required rather than derived from the label.
	ID    string `json:"id"`
	Label string `json:"label"`
	Icon  string `json:"icon,omitzero"`
	// Type is ViewOverview or ViewTable; empty means table, which is what
	// almost every view is.
	Type string `json:"type,omitzero"`
	// Kind is the resource this view lists: a built-in kind name, or a
	// "crd:<plural>.<group>" custom resource. Required for a table view and
	// meaningless on the overview.
	Kind string `json:"kind,omitzero"`
	// Namespace pins the view to one namespace. Empty means every namespace and
	// leaves the tab's own namespace filter free; set, it is where the view
	// opens and the filter is fixed there, because a view that says
	// "Argo CD's controllers" is not answering a question about kube-system.
	Namespace string `json:"namespace,omitzero"`
	// Selector narrows the view to objects carrying certain labels, in the
	// usual `a=b,c in (d,e)` syntax. It is what lets a plugin offer a view of a
	// built-in kind -- the Deployments that are Argo CD's -- rather than only
	// of custom resources nothing else owns.
	Selector string `json:"selector,omitzero"`
}

// Requirement is a kind the plugin needs the cluster to serve. It is what the
// overview's readiness check is made of, and what decides whether the sidebar
// shows the plugin as present in a given cluster.
type Requirement struct {
	Kind  string `json:"kind"`
	Label string `json:"label,omitzero"`
	// Optional requirements are reported but do not decide whether the plugin
	// counts as installed. Argo CD without ApplicationSets is still Argo CD.
	Optional bool `json:"optional,omitzero"`
}

// Card is one live tile on the plugin's overview: how many of a kind there are,
// divided up by one of their own fields.
type Card struct {
	Label string `json:"label"`
	Kind  string `json:"kind"`
	// GroupBy addresses the field the count is divided by -- see kube.FieldPath
	// for the two shapes it takes. Empty counts without dividing, which is
	// still useful for a kind with no status worth reading.
	GroupBy kube.FieldPath `json:"groupBy,omitzero"`
	// Tones maps a field value to how it should read: "ok", "warn", "error" or
	// "info". A value with no entry is drawn plainly, so a plugin only has to
	// name the ones that mean something.
	Tones map[string]string `json:"tones,omitzero"`
	// Namespace and Selector narrow what is counted, exactly as on a View, so a
	// card can agree with the view it sits above.
	Namespace string `json:"namespace,omitzero"`
	Selector  string `json:"selector,omitzero"`
}

// The surfaces a chart can be attached to, beyond a resource kind.
const (
	// AttachDashboard puts the chart on the cluster's own dashboard, where it
	// is about the cluster rather than about any one object.
	AttachDashboard = "dashboard"
	// AttachOverview puts it on the plugin's own landing page.
	AttachOverview = "overview"
)

// Chart is one time series drawn from the cluster's Prometheus.
//
// A chart is where a plugin stops describing Kubernetes objects and starts
// describing what is happening to them, which is the one thing the API server
// cannot answer. It is still declarative: a query, a label and a unit. The query
// is passed through to Prometheus untouched -- see internal/metrics for why that
// is a different kind of thing from the field paths in Card.
type Chart struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	// Attach names where the chart is drawn: AttachDashboard, AttachOverview,
	// or a kind, in which case it appears in the detail panel of any object of
	// that kind.
	Attach string `json:"attach"`
	// Query is PromQL. It may refer to the object being drawn for through the
	// variables in metrics.VariableNames -- $namespace, $name, $node -- which
	// are the only things interpolable into it.
	Query string `json:"query"`
	// Legend names the Prometheus label each series is titled by. Empty falls
	// back to the whole label set, which is the honest answer when a query
	// returns several series and the plugin has not said how to tell them
	// apart.
	Legend string `json:"legend,omitzero"`
	// Unit decides how values are written: see ChartUnits. Empty prints the
	// number as it comes.
	Unit string `json:"unit,omitzero"`
	// Description is a sentence under the chart's title, for saying what the
	// query actually measures -- which is rarely obvious from a label like
	// "CPU".
	Description string `json:"description,omitzero"`
}

// ChartUnits are the units a chart may declare. They decide only how a number
// is written -- 1.5 cores, 512 MiB, 87% -- so an unknown one would silently
// print raw numbers, which is why the set is closed and checked on load.
var ChartUnits = []string{"", "cores", "bytes", "bytes/s", "percent", "ops/s", "seconds", "count"}

// Plugin is one solution the app knows how to show.
type Plugin struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Tagline string `json:"tagline,omitzero"`
	Icon    string `json:"icon,omitzero"`
	Author  string `json:"author,omitzero"`
	// Docs is a link shown on the overview. Only http(s) is accepted; a plugin
	// file is not allowed to hand the app an arbitrary URL scheme to open.
	Docs        string        `json:"docs,omitzero"`
	Description string        `json:"description,omitzero"`
	Requires    []Requirement `json:"requires,omitzero"`
	Views       []View        `json:"views"`
	Cards       []Card        `json:"cards,omitzero"`
	Charts      []Chart       `json:"charts,omitzero"`

	// Origin is filled in by the loader and ignored on the way in: BuiltinOrigin
	// or the path of the file it was read from.
	Origin string `json:"origin"`
	// Pack is the collection it arrived in, empty for one that came on its own.
	Pack string `json:"pack"`
}

// BuiltinOrigin marks the plugins that ship with the app.
const BuiltinOrigin = "builtin"

// Builtin reports whether the plugin came with the app rather than off disk.
func (p Plugin) Builtin() bool { return p.Origin == BuiltinOrigin }

// Identity is the plugin's id -- see addons.Identified.
func (p Plugin) Identity() string { return p.ID }

// Source is the file it came from -- see addons.Sourced.
func (p Plugin) Source() string { return p.Origin }

// View returns the view with the given id.
func (p Plugin) View(id string) (View, bool) {
	for _, v := range p.Views {
		if v.ID == id {
			return v, true
		}
	}
	return View{}, false
}

// Pack is a file carrying several plugins, which is how a collection is
// distributed as one file. Mirrors themes.Pack; the two extension points are
// installed the same way on purpose.
type Pack struct {
	Name    string   `json:"name,omitzero"`
	Author  string   `json:"author,omitzero"`
	Version string   `json:"version,omitzero"`
	Plugins []Plugin `json:"plugins"`
}

// Problem is a plugin file that could not be used, and why -- addons.Problem
// under a local name, so the settings view renders one shape whatever failed.
type Problem = addons.Problem

// Catalogue is everything the sidebar and the settings view need: what is
// installed on this machine, where it was read from, and what would not load.
type Catalogue struct {
	Plugins []Plugin `json:"plugins"`
	// Dir is the folder plugins are read from by default.
	Dir      string    `json:"dir"`
	Folders  []string  `json:"folders"`
	Problems []Problem `json:"problems"`
}

// Find returns the plugin with the given id.
func (c Catalogue) Find(id string) (Plugin, bool) {
	for _, p := range c.Plugins {
		if p.ID == id {
			return p, true
		}
	}
	return Plugin{}, false
}

// Resolve turns a "plugin:" tab kind into the plugin and view it names.
func (c Catalogue) Resolve(kind string) (Plugin, View, bool) {
	pluginID, viewID, ok := ParseViewKind(kind)
	if !ok {
		return Plugin{}, View{}, false
	}
	plugin, ok := c.Find(pluginID)
	if !ok {
		return Plugin{}, View{}, false
	}
	view, ok := plugin.View(viewID)
	if !ok {
		return Plugin{}, View{}, false
	}
	return plugin, view, true
}

// validate checks a plugin is usable and returns it normalised. As with a
// theme, it is forgiving about what is left out and strict about what is put
// in: a missing label has a defensible answer, a kind that does not exist does
// not.
func validate(p Plugin) (Plugin, error) {
	p.ID = strings.TrimSpace(p.ID)
	p.Name = strings.TrimSpace(p.Name)

	if p.ID == "" {
		return p, fmt.Errorf("plugin has no id")
	}
	if !addons.ValidID(p.ID) {
		return p, fmt.Errorf("plugin id %q must be lowercase letters, digits and dashes", p.ID)
	}
	if p.Name == "" {
		p.Name = p.ID
	}
	if p.Docs != "" && !webLink(p.Docs) {
		return p, fmt.Errorf("plugin %q has a docs link that is not http(s): %q", p.ID, p.Docs)
	}
	if p.Icon == "" {
		p.Icon = "puzzle"
	}

	for i, req := range p.Requires {
		req.Kind = strings.TrimSpace(req.Kind)
		if !kube.IsKnownKind(req.Kind) {
			return p, fmt.Errorf("plugin %q requires %q, which is not a kind this app can open", p.ID, req.Kind)
		}
		if req.Label == "" {
			req.Label = req.Kind
		}
		p.Requires[i] = req
	}

	seen := map[string]bool{}
	views := make([]View, 0, len(p.Views))
	for _, view := range p.Views {
		view, err := validateView(p.ID, view)
		if err != nil {
			return p, err
		}
		if seen[view.ID] {
			return p, fmt.Errorf("plugin %q has two views with id %q", p.ID, view.ID)
		}
		seen[view.ID] = true
		views = append(views, view)
	}
	p.Views = views

	for i, card := range p.Cards {
		card, err := validateCard(p.ID, card)
		if err != nil {
			return p, err
		}
		p.Cards[i] = card
	}

	seenCharts := map[string]bool{}
	for i, chart := range p.Charts {
		chart, err := validateChart(p.ID, chart)
		if err != nil {
			return p, err
		}
		if seenCharts[chart.ID] {
			return p, fmt.Errorf("plugin %q has two charts with id %q", p.ID, chart.ID)
		}
		seenCharts[chart.ID] = true
		p.Charts[i] = chart
	}

	if len(p.Views) == 0 && len(p.Cards) == 0 && len(p.Charts) == 0 {
		return p, fmt.Errorf("plugin %q has no views, nothing to summarise and nothing to chart, so there would be nothing to show", p.ID)
	}
	return p, nil
}

func validateChart(pluginID string, c Chart) (Chart, error) {
	c.ID = strings.TrimSpace(c.ID)
	c.Label = strings.TrimSpace(c.Label)
	c.Attach = strings.TrimSpace(c.Attach)
	c.Query = strings.TrimSpace(c.Query)

	if !addons.ValidID(c.ID) {
		return c, fmt.Errorf("plugin %q has a chart with id %q, which must be lowercase letters, digits and dashes", pluginID, c.ID)
	}
	if c.Label == "" {
		c.Label = c.ID
	}
	switch c.Attach {
	case AttachDashboard, AttachOverview:
	case "":
		return c, fmt.Errorf("plugin %q has a chart %q that does not say where it is drawn", pluginID, c.ID)
	default:
		if !kube.IsKnownKind(c.Attach) {
			return c, fmt.Errorf("plugin %q attaches chart %q to %q, which is neither %q, %q, nor a kind this app can open",
				pluginID, c.ID, c.Attach, AttachDashboard, AttachOverview)
		}
	}
	// Checked when the file is read rather than when the chart is drawn, so a
	// typo is reported against the plugin instead of appearing as an empty box.
	if err := metrics.CheckQuery(c.Query); err != nil {
		return c, fmt.Errorf("plugin %q, chart %q: %w", pluginID, c.ID, err)
	}
	// A chart drawn for the cluster has no object to name, so a query wanting
	// one would always come out empty.
	if c.Attach == AttachDashboard || c.Attach == AttachOverview {
		if strings.Contains(c.Query, "$name") || strings.Contains(c.Query, "$namespace") || strings.Contains(c.Query, "$node") {
			return c, fmt.Errorf("plugin %q, chart %q is drawn for the whole cluster but its query asks about one object", pluginID, c.ID)
		}
	}
	if !slices.Contains(ChartUnits, c.Unit) {
		return c, fmt.Errorf("plugin %q, chart %q has unit %q; it must be one of %s",
			pluginID, c.ID, c.Unit, strings.Join(quoted(ChartUnits[1:]), ", "))
	}
	return c, nil
}

// quoted renders a list for an error message.
func quoted(values []string) []string {
	out := make([]string, 0, len(values))
	for _, v := range values {
		out = append(out, strconv.Quote(v))
	}
	return out
}

// ChartsFor returns the plugin's charts drawn on one surface.
func (p Plugin) ChartsFor(attach string) []Chart {
	var out []Chart
	for _, chart := range p.Charts {
		if chart.Attach == attach {
			out = append(out, chart)
		}
	}
	return out
}

func validateView(pluginID string, v View) (View, error) {
	v.ID = strings.TrimSpace(v.ID)
	v.Label = strings.TrimSpace(v.Label)
	v.Kind = strings.TrimSpace(v.Kind)

	if !addons.ValidID(v.ID) {
		return v, fmt.Errorf("plugin %q has a view with id %q, which must be lowercase letters, digits and dashes", pluginID, v.ID)
	}
	if v.ID == OverviewID {
		return v, fmt.Errorf("plugin %q declares a view called %q, which is the name of the overview every plugin already has", pluginID, OverviewID)
	}
	if v.Label == "" {
		v.Label = v.ID
	}
	if v.Type == "" {
		v.Type = ViewTable
	}
	if v.Type != ViewTable {
		return v, fmt.Errorf("plugin %q has a view of type %q; only %q may be declared", pluginID, v.Type, ViewTable)
	}
	if v.Kind == "" {
		return v, fmt.Errorf("plugin %q has a view %q with no kind to list", pluginID, v.ID)
	}
	if strings.HasPrefix(v.Kind, Prefix) {
		return v, fmt.Errorf("plugin %q has a view %q pointing at another plugin's view", pluginID, v.ID)
	}
	if !kube.IsKnownKind(v.Kind) {
		return v, fmt.Errorf("plugin %q has a view %q on %q, which is not a kind this app can open", pluginID, v.ID, v.Kind)
	}
	if err := checkFilter(v.Namespace, v.Selector); err != nil {
		return v, fmt.Errorf("plugin %q, view %q: %w", pluginID, v.ID, err)
	}
	if v.Icon == "" {
		v.Icon = "puzzle"
	}
	return v, nil
}

func validateCard(pluginID string, c Card) (Card, error) {
	c.Kind = strings.TrimSpace(c.Kind)
	c.Label = strings.TrimSpace(c.Label)

	if !kube.IsKnownKind(c.Kind) {
		return c, fmt.Errorf("plugin %q has a card on %q, which is not a kind this app can open", pluginID, c.Kind)
	}
	if c.Label == "" {
		c.Label = c.Kind
	}
	if c.GroupBy != "" && !c.GroupBy.Valid() {
		return c, fmt.Errorf("plugin %q has a card grouped by %q, which is not a field path", pluginID, c.GroupBy)
	}
	for value, tone := range c.Tones {
		switch tone {
		case "ok", "warn", "error", "info":
		default:
			return c, fmt.Errorf("plugin %q gives %q the tone %q; it must be ok, warn, error or info", pluginID, value, tone)
		}
	}
	if err := checkFilter(c.Namespace, c.Selector); err != nil {
		return c, fmt.Errorf("plugin %q, card %q: %w", pluginID, c.Label, err)
	}
	return c, nil
}

// checkFilter validates the namespace and selector a view or card narrows with.
// The selector is parsed here, when the file is read, so a malformed one is
// reported against the plugin rather than surfacing later as a tab that will
// not open.
func checkFilter(namespace, selector string) error {
	if namespace != "" && !dnsLabel(namespace) {
		return fmt.Errorf("%q is not a namespace name", namespace)
	}
	if selector != "" {
		if _, err := labels.Parse(selector); err != nil {
			return fmt.Errorf("label selector %q: %w", selector, err)
		}
	}
	return nil
}

// webLink reports whether a URL is one the app is willing to open. A plugin
// file comes from outside the app, and handing the platform an arbitrary scheme
// to open is not something a list of colours and kind names has any business
// doing.
func webLink(url string) bool {
	return strings.HasPrefix(url, "https://") || strings.HasPrefix(url, "http://")
}

// dnsLabel is the Kubernetes name format a namespace has to be in.
func dnsLabel(s string) bool {
	if len(s) == 0 || len(s) > 63 {
		return false
	}
	for i, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
		case r == '-' && i > 0 && i < len(s)-1:
		default:
			return false
		}
	}
	return true
}

// Resolved is what a "plugin:" tab kind actually means: the kind to list, and
// the filters the view pins around it.
//
// It is one value rather than three returns because the frontend needs it too:
// a tab whose view fixes a namespace must not draw a namespace picker that
// looks like it would work.
type Resolved struct {
	// Kind is the real kind to subscribe to -- a built-in name or a "crd:" one.
	Kind string `json:"kind"`
	// Namespace is fixed for this view, or empty to leave the tab's own filter
	// free.
	Namespace string `json:"namespace"`
	Selector  string `json:"selector"`
	// PluginID and ViewID are carried back so the frontend can title the tab
	// and find its way to the plugin without parsing the kind a second time.
	PluginID   string `json:"pluginId"`
	PluginName string `json:"pluginName"`
	ViewID     string `json:"viewId"`
	Label      string `json:"label"`
	Icon       string `json:"icon"`
	// Overview is true for the plugin's landing page, which is not a listing at
	// all and has no Kind.
	Overview bool `json:"overview"`
}

// ResolveKind turns a "plugin:" tab kind into what it names.
//
// A kind naming a plugin that is not installed is an error with a sentence in
// it rather than a silent empty tab: it is what a restored session looks like
// after the plugin's folder was dropped, and the reader needs telling that the
// tab is fine and the plugin is missing.
func (c Catalogue) ResolveKind(kind string) (Resolved, error) {
	pluginID, viewID, ok := ParseViewKind(kind)
	if !ok {
		return Resolved{}, fmt.Errorf("%q does not name a plugin view", kind)
	}

	plugin, ok := c.Find(pluginID)
	if !ok {
		return Resolved{}, fmt.Errorf("no plugin called %q is installed -- the folder it came from may have been removed", pluginID)
	}

	// Every plugin has an overview whether or not it declares one, so it is
	// answered here rather than looked up.
	if viewID == OverviewID {
		return Resolved{
			PluginID:   plugin.ID,
			PluginName: plugin.Name,
			ViewID:     OverviewID,
			Label:      plugin.Name,
			Icon:       plugin.Icon,
			Overview:   true,
		}, nil
	}

	view, ok := plugin.View(viewID)
	if !ok {
		return Resolved{}, fmt.Errorf("the %s plugin has no view called %q", plugin.Name, viewID)
	}
	return Resolved{
		Kind:       view.Kind,
		Namespace:  view.Namespace,
		Selector:   view.Selector,
		PluginID:   plugin.ID,
		PluginName: plugin.Name,
		ViewID:     view.ID,
		Label:      view.Label,
		Icon:       view.Icon,
	}, nil
}
