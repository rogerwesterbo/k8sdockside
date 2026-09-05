// Package metrics reads time series out of a cluster's Prometheus.
//
// It deliberately knows nothing about client-go. Everything that touches a
// cluster arrives as a Fetch function -- see Client -- so the query building,
// the variable substitution and the response parsing, which is where all the
// judgement is, can be exercised without a Kubernetes API or a Prometheus.
//
// The queries themselves come from plugin files. That is not the same kind of
// thing as the field paths in internal/plugins: a field path is an expression
// *this app* evaluates, so it is kept to two shapes it can evaluate safely,
// while a PromQL string is passed through untouched to a server that already
// exists to answer it. What this package does police is the substitution --
// what a plugin is allowed to interpolate into a query, and what it is allowed
// to point one at.
package metrics

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"math"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
)

// sortStrings and sortSeries keep a chart's output stable between refreshes.
func sortStrings(values []string) { slices.Sort(values) }

func sortSeries(series []Series) {
	sort.SliceStable(series, func(i, j int) bool { return series[i].Name < series[j].Name })
}

// Point is one sample: a moment, and what the series was worth at it.
type Point struct {
	// T is Unix seconds. A float rather than a time so it crosses the binding
	// as a number the chart can scale directly.
	T float64 `json:"t"`
	V float64 `json:"v"`
}

// Series is one line on a chart.
type Series struct {
	// Name is what the legend calls it, taken from the label the chart named or
	// built from the whole label set.
	Name   string  `json:"name"`
	Points []Point `json:"points"`
}

// Result is one chart's worth of data.
type Result struct {
	Series []Series `json:"series"`
	// Error is why this chart has no data, if it has none. It is carried rather
	// than returned so one failing query does not empty a page of charts.
	Error string `json:"error"`
}

// Fetch performs one GET against the Prometheus API and returns the body. It is
// how this package reaches a cluster without knowing how the connection is made
// -- through the API server's service proxy, or straight at a URL.
type Fetch func(ctx context.Context, path string, params map[string]string) ([]byte, error)

// Range is the window a chart covers, and how finely.
type Range struct {
	// Minutes back from now. The chart's own control sets it.
	Minutes int
	// End is when the window closes, normally now. Settable so a test does not
	// depend on the clock.
	End time.Time
}

// steps is how many samples a range is drawn with. Enough to show shape,
// few enough that the query is cheap and the SVG stays small: a chart three
// hundred pixels wide cannot show more than this anyway.
const steps = 120

// step is the resolution Prometheus is asked for, never finer than 15 seconds
// (below Prometheus's usual scrape interval there is nothing new to say) and
// rounded to a whole second because the API takes it as a duration.
func (r Range) step() time.Duration {
	if r.Minutes <= 0 {
		return 15 * time.Second
	}
	s := time.Duration(r.Minutes) * time.Minute / steps
	if s < 15*time.Second {
		return 15 * time.Second
	}
	return s.Round(time.Second)
}

func (r Range) end() time.Time {
	if r.End.IsZero() {
		return time.Now()
	}
	return r.End
}

// QueryRange runs one PromQL query over a window and returns the series it
// produced.
//
// `legend` names the label to take each series' name from. Empty falls back to
// the whole label set, which is what Prometheus's own UI shows and is the only
// honest answer when a query returns several series and the plugin has not said
// how to tell them apart.
func QueryRange(ctx context.Context, fetch Fetch, query, legend string, r Range) (Result, error) {
	end := r.end()
	start := end.Add(-time.Duration(r.Minutes) * time.Minute)

	raw, err := fetch(ctx, "/api/v1/query_range", map[string]string{
		"query": query,
		"start": strconv.FormatInt(start.Unix(), 10),
		"end":   strconv.FormatInt(end.Unix(), 10),
		"step":  strconv.FormatInt(int64(r.step().Seconds()), 10) + "s",
	})
	if err != nil {
		return Result{}, err
	}
	return parseRange(raw, legend)
}

// Variables are the values a chart's query may refer to: the object it is being
// drawn for. Nothing else is interpolable, which is the point -- see Expand.
type Variables struct {
	Namespace string
	Name      string
	Node      string
}

// safeValue is what a variable's value has to look like before it may be put
// into a query. Kubernetes names are DNS-1123 and node names are hostnames, so
// this admits every real value; anything else -- a quote, a brace, a newline --
// would be a way to end the label matcher it lands in and write a different
// query, so it is refused rather than escaped.
var safeValue = regexp.MustCompile(`^[a-zA-Z0-9._:-]*$`)

// variable matches the `$name` references a query may contain.
var variable = regexp.MustCompile(`\$[a-zA-Z]+`)

// Expand substitutes the object's identity into a query.
//
// This is the one place a plugin's string is modified, and the only injection
// surface the charts have: the query itself goes to Prometheus as written --
// which is fine, it is a read-only expression language on a server that exists
// to evaluate it -- but a *value* interpolated into a label matcher could close
// the matcher and add a different one. So values are checked rather than
// trusted, and an unknown variable is an error instead of being left in the
// query to be read as a Prometheus operator.
func Expand(query string, vars Variables) (string, error) {
	known := map[string]string{
		"$namespace": vars.Namespace,
		"$name":      vars.Name,
		"$node":      vars.Node,
	}

	var bad []string
	out := variable.ReplaceAllStringFunc(query, func(ref string) string {
		value, ok := known[ref]
		if !ok {
			bad = append(bad, ref)
			return ref
		}
		if !safeValue.MatchString(value) {
			bad = append(bad, ref+" (the value is not a Kubernetes name)")
			return ref
		}
		return value
	})

	if len(bad) > 0 {
		return "", fmt.Errorf("query refers to %s, which is not a value this chart is given", strings.Join(bad, ", "))
	}
	return out, nil
}

// Variables returns the names a query may use, for documentation and for the
// plugin loader's validation.
func VariableNames() []string { return []string{"$namespace", "$name", "$node"} }

// CheckQuery reports whether a query only refers to variables that exist. It is
// called when a plugin file is read, so a typo is reported against the plugin
// rather than surfacing later as an empty chart.
func CheckQuery(query string) error {
	if strings.TrimSpace(query) == "" {
		return errors.New("the query is empty")
	}
	// Expanded against placeholder values that satisfy safeValue: what is being
	// checked here is the set of variable names, not any particular object.
	_, err := Expand(query, Variables{Namespace: "x", Name: "x", Node: "x"})
	return err
}

// ---- response parsing ------------------------------------------------------

// promResponse is the envelope every Prometheus API answer comes in.
type promResponse struct {
	Status    string   `json:"status"`
	Data      promData `json:"data"`
	ErrorType string   `json:"errorType"`
	Error     string   `json:"error"`
	Warnings  []string `json:"warnings"`
}

type promData struct {
	ResultType string       `json:"resultType"`
	Result     []promSeries `json:"result"`
}

type promSeries struct {
	Metric map[string]string `json:"metric"`
	// Values are [unixSeconds, "value"] pairs -- Prometheus sends the sample as
	// a string so that NaN and very large numbers survive JSON.
	Values [][2]any `json:"values"`
}

// seriesName is what the legend calls a series.
func seriesName(labels map[string]string, legend string) string {
	if legend != "" {
		if value, ok := labels[legend]; ok && value != "" {
			return value
		}
	}
	// A query that aggregates everything away returns one series with no labels
	// at all, which is the common case for a single-line chart.
	if len(labels) == 0 {
		return ""
	}
	if name, ok := labels["__name__"]; ok && len(labels) == 1 {
		return name
	}

	parts := make([]string, 0, len(labels))
	for key, value := range labels {
		if key == "__name__" {
			continue
		}
		parts = append(parts, key+"="+value)
	}
	sortStrings(parts)
	return strings.Join(parts, ", ")
}

// sample reads one [time, "value"] pair. A sample Prometheus could not compute
// arrives as "NaN", which is a real answer -- the series has a gap there -- and
// is dropped rather than drawn as zero, because a gap and a zero mean opposite
// things on a chart.
func sample(pair [2]any) (Point, bool) {
	at, ok := pair[0].(float64)
	if !ok {
		return Point{}, false
	}
	text, ok := pair[1].(string)
	if !ok {
		return Point{}, false
	}
	value, err := strconv.ParseFloat(text, 64)
	if err != nil || math.IsNaN(value) || math.IsInf(value, 0) {
		return Point{}, false
	}
	return Point{T: at, V: value}, true
}

// parseRange turns a query_range answer into series.
func parseRange(raw []byte, legend string) (Result, error) {
	var answer promResponse
	if err := json.Unmarshal(raw, &answer); err != nil {
		// Anything but JSON here almost always means the proxy reached
		// something that is not Prometheus, which is worth saying plainly
		// rather than reporting as a parse error at byte 4.
		return Result{}, fmt.Errorf("the reply was not a Prometheus response: %w", err)
	}
	if answer.Status != "success" {
		if answer.Error != "" {
			return Result{}, errors.New(answer.Error)
		}
		return Result{}, fmt.Errorf("prometheus answered %q", answer.Status)
	}

	out := Result{Series: []Series{}}
	for _, raw := range answer.Data.Result {
		points := make([]Point, 0, len(raw.Values))
		for _, pair := range raw.Values {
			if point, ok := sample(pair); ok {
				points = append(points, point)
			}
		}
		if len(points) == 0 {
			continue
		}
		out.Series = append(out.Series, Series{Name: seriesName(raw.Metric, legend), Points: points})
	}

	// Stable ordering, so a series keeps its colour between refreshes. Without
	// it Prometheus's own order can move a line from blue to orange on a reload,
	// which reads as the data having changed.
	sortSeries(out.Series)
	return out, nil
}
