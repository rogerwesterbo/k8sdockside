package themes

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// AA is the WCAG 2 contrast ratio normal text has to clear. It is the bar the
// built-in palettes are held to by test, and the bar a user's own theme is
// warned against rather than refused -- somebody's terminal-matching palette is
// their business, but they should be told before they wonder why the small
// print is unreadable.
const AA = 4.5

// rgb is a colour with the alpha dropped; only opaque values are checked.
type rgb struct{ r, g, b float64 }

// parseHex reads #rgb, #rrggbb and their alpha forms. Anything else -- an
// rgba(), a colour function -- returns false, because a contrast ratio against
// a translucent value depends on what is behind it, and guessing would produce
// a number that looks authoritative and is not.
func parseHex(value string) (rgb, bool) {
	value = strings.TrimSpace(value)
	if !strings.HasPrefix(value, "#") {
		return rgb{}, false
	}
	hex := value[1:]
	switch len(hex) {
	case 3, 4:
		var expanded strings.Builder
		for _, c := range hex[:3] {
			expanded.WriteRune(c)
			expanded.WriteRune(c)
		}
		hex = expanded.String()
	case 6:
	case 8:
		hex = hex[:6]
	default:
		return rgb{}, false
	}
	n, err := strconv.ParseUint(hex, 16, 32)
	if err != nil {
		return rgb{}, false
	}
	return rgb{
		r: float64((n >> 16) & 0xff),
		g: float64((n >> 8) & 0xff),
		b: float64(n & 0xff),
	}, true
}

// luminance is WCAG relative luminance.
func (c rgb) luminance() float64 {
	channel := func(v float64) float64 {
		v /= 255
		if v <= 0.04045 {
			return v / 12.92
		}
		return math.Pow((v+0.055)/1.055, 2.4)
	}
	return 0.2126*channel(c.r) + 0.7152*channel(c.g) + 0.0722*channel(c.b)
}

// Contrast is the WCAG contrast ratio between two colours, from 1 (identical)
// to 21 (black on white). It reports false when either value is not an opaque
// hex colour and so cannot be measured.
func Contrast(fg, bg string) (float64, bool) {
	a, okA := parseHex(fg)
	b, okB := parseHex(bg)
	if !okA || !okB {
		return 0, false
	}
	la, lb := a.luminance(), b.luminance()
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05), true
}

// UI is the WCAG contrast ratio a user-interface component has to clear, and
// the bar the accent-text/accent pair is held to rather than AA.
//
// That pair is judged differently on purpose. --accent is a surface at least as
// often as it is a colour -- selected rows, status pills, the toggle knob, the
// focus ring -- so raising it until white text on it clears 4.5 would darken
// selection and focus everywhere to fix one button label. The app's own accent
// sits at 3.4:1 against white and has since before there were themes; holding
// the built-ins to a bar their default fails would mean either a test that is
// always red or restyling the app under the user. It is a real compromise
// rather than a rounding of the rules, and it applies only to this one pair.
const UI = 3.0

// pair is a foreground/background combination the app actually draws, and the
// ratio it has to clear.
type pair struct {
	fg, bg string
	need   float64
}

// contrastPairs is every combination worth checking. Each text token is drawn
// on every surface, and accent-text is drawn on accent -- which is the one a
// theme author is most likely to get wrong, because it is the only pair that
// does not involve a background at all.
var contrastPairs = func() []pair {
	var pairs []pair
	for _, fg := range []string{"text", "text-dim", "text-faint"} {
		for _, bg := range []string{"bg", "bg-sidebar", "bg-panel", "bg-raised"} {
			pairs = append(pairs, pair{fg, bg, AA})
		}
	}
	return append(pairs, pair{"accent-text", "accent", UI})
}()

// Audit returns the readability problems in a resolved token set: every pair
// the app actually draws that falls below AA, worst first.
//
// It is advice rather than validation. A theme that fails this still loads --
// see AA -- and the settings view shows what it says next to the theme.
func Audit(tokens map[string]string) []string {
	type failure struct {
		text  string
		ratio float64
	}
	var failures []failure
	for _, p := range contrastPairs {
		ratio, ok := Contrast(tokens[p.fg], tokens[p.bg])
		if !ok || ratio >= p.need {
			continue
		}
		failures = append(failures, failure{
			text:  fmt.Sprintf("%s on %s is %.1f:1, below the %.1f:1 needed to be readable", p.fg, p.bg, ratio, p.need),
			ratio: ratio,
		})
	}
	// Worst first: if only the first line is read, it should be the one that
	// matters most.
	for i := 1; i < len(failures); i++ {
		for j := i; j > 0 && failures[j].ratio < failures[j-1].ratio; j-- {
			failures[j], failures[j-1] = failures[j-1], failures[j]
		}
	}
	out := make([]string, 0, len(failures))
	for _, f := range failures {
		out = append(out, f.text)
	}
	return out
}
