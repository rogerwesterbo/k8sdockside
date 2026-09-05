package themes

import (
	"regexp"
	"slices"
	"strings"
)

// Token describes one of the colours a theme may set. The list is the contract
// between the app and a theme author: it is what the settings view documents,
// what the loader validates against, and the only thing a theme can change.
//
// It is a list rather than a set so the documentation and the settings view can
// show the tokens in an order that makes sense to read -- surfaces, then edges,
// then text, then the accent, then status -- instead of alphabetically.
type Token struct {
	Name string `json:"name"`
	Help string `json:"help"`
}

// Tokens is every token a theme may set, in the order they are worth reading.
var Tokens = []Token{
	{"bg", "The window behind everything."},
	{"bg-sidebar", "The context sidebar, the settings rail and the status bar."},
	{"bg-panel", "Panels that sit on the window: the detail panel, the dock, the editor."},
	{"bg-raised", "Controls that stand off their surface: buttons, pills, input wells."},
	{"bg-hover", "Laid over a surface on hover. Usually translucent, so it works on all of them."},
	{"bg-active", "Laid over a surface when it is selected or held. Usually translucent."},
	{"border", "The structural edge around a panel, where being seen is the point."},
	{"border-soft", "The line between rows in a list, where a line per item adds up. Much fainter than border."},
	{"text", "Body text and anything that has to be read first."},
	{"text-dim", "Secondary text: hints, values, the second line of a row."},
	{"text-faint", "Small print: counts, section headings, file paths. Still has to clear AA at 10px."},
	{"accent", "Selection, focus rings, links and primary buttons."},
	{"accent-text", "Text drawn on top of accent. The one token that must contrast with accent rather than a surface."},
	{"ok", "Healthy: Running, Ready, Bound."},
	{"warn", "Not yet, or not quite: Pending, Progressing, a nearly full disk."},
	{"error", "Failed: CrashLoopBackOff, Evicted, an unreachable cluster."},
	{"scrollbar", "The scrollbar thumb. Translucent, so dense tables do not gain a heavy frame."},
	{"scrollbar-hover", "The scrollbar thumb under the pointer."},

	// The chart series palette. Eight slots in a fixed order, assigned in
	// sequence and never cycled: the order is what keeps neighbouring series
	// distinguishable to a colourblind reader, so it is a property of the
	// palette rather than of any one chart.
	//
	// They are separate from ok/warn/error on purpose. Those are *status*
	// colours with reserved meaning, and a series that merely happens to be
	// fourth must never borrow the colour that means "failing".
	{"chart-1", "First series in a chart. The eight chart colours are assigned in order and never cycled."},
	{"chart-2", "Second series."},
	{"chart-3", "Third series."},
	{"chart-4", "Fourth series."},
	{"chart-5", "Fifth series."},
	{"chart-6", "Sixth series."},
	{"chart-7", "Seventh series."},
	{"chart-8", "Eighth series. A ninth is folded into this one rather than given a colour of its own."},
	{"chart-grid", "Gridlines and axes in a chart. Recessive: one step off the surface."},
}

// SeriesTokens is how many chart series colours a theme defines. A chart with
// more series than this folds the rest together rather than inventing a colour,
// because a generated ninth hue is exactly the thing the fixed order prevents.
const SeriesTokens = 8

// tokenNames is Tokens as a lookup, built once.
var tokenNames = func() map[string]bool {
	names := make(map[string]bool, len(Tokens))
	for _, t := range Tokens {
		names[t.Name] = true
	}
	return names
}()

// IsToken reports whether a theme may set this token.
func IsToken(name string) bool { return tokenNames[name] }

// TokenNames is every token name, for callers that only want the keys.
func TokenNames() []string {
	names := make([]string, 0, len(Tokens))
	for _, t := range Tokens {
		names = append(names, t.Name)
	}
	return names
}

// colorFuncs are the CSS colour functions a theme may use. Anything else --
// url(), var(), calc(), attr() -- is refused, because a token's value is
// written straight into a style declaration and the only thing that should be
// able to appear there is a colour.
var colorFuncs = []string{"rgb", "rgba", "hsl", "hsla", "hwb", "lab", "lch", "oklab", "oklch", "color-mix"}

var (
	hexColor = regexp.MustCompile(`^#([0-9a-fA-F]{3,4}|[0-9a-fA-F]{6}|[0-9a-fA-F]{8})$`)
	// The charset inside a colour function: numbers, units, separators and the
	// keywords a colour space takes. Notably absent are `;`, `}`, `"` and `\`,
	// which is what stops a value from ending the declaration it is written
	// into and starting something else.
	funcArgs = regexp.MustCompile(`^[0-9a-zA-Z.,%\s/+\-]*$`)
)

// IsColor reports whether a value is something this app is willing to write
// into a style declaration.
//
// This is a whitelist rather than a blacklist, and deliberately narrower than
// what CSS accepts: a theme file comes from outside the app, and the answer to
// "what else could go here?" should be "nothing" rather than a list of things
// we remembered to exclude. A theme that wanted `rebeccapurple` can write the
// hex.
func IsColor(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	if strings.EqualFold(value, "transparent") {
		return true
	}
	if hexColor.MatchString(value) {
		return true
	}

	open := strings.IndexByte(value, '(')
	if open < 0 || !strings.HasSuffix(value, ")") {
		return false
	}
	fn := strings.ToLower(strings.TrimSpace(value[:open]))
	if !slices.Contains(colorFuncs, fn) {
		return false
	}
	args := value[open+1 : len(value)-1]
	// color-mix names its colour spaces and nests colours, so it is the one
	// function whose arguments can contain another call. Allowing one level is
	// enough for `color-mix(in oklab, #fff 20%, transparent)` without opening
	// the door to arbitrary nesting.
	if fn == "color-mix" {
		args = strings.NewReplacer("(", " ", ")", " ", "#", " ", ":", " ").Replace(args)
	}
	return funcArgs.MatchString(args)
}
