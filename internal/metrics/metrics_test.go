package metrics

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// fetchReturning answers every call with the same body.
func fetchReturning(body string) Fetch {
	return func(context.Context, string, map[string]string) ([]byte, error) {
		return []byte(body), nil
	}
}

func TestExpandSubstitutesTheObject(t *testing.T) {
	got, err := Expand(`rate(x{namespace="$namespace",pod="$name"}[5m])`, Variables{
		Namespace: "argocd",
		Name:      "argocd-server-7d9f",
	})
	if err != nil {
		t.Fatalf("Expand: %v", err)
	}
	want := `rate(x{namespace="argocd",pod="argocd-server-7d9f"}[5m])`
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// The one injection surface the charts have. A value lands inside a label
// matcher, so anything that could close the matcher and open another one has to
// be refused rather than escaped -- and no real Kubernetes name contains any of
// this.
func TestExpandRefusesValuesThatAreNotNames(t *testing.T) {
	nasty := []string{
		`x" or on() vector(1) or x="`,
		"a'b",
		"a}b",
		"a\nb",
		"a b",
		`a\b`,
	}
	for _, value := range nasty {
		if _, err := Expand(`x{pod="$name"}`, Variables{Name: value}); err == nil {
			t.Errorf("Expand accepted the name %q", value)
		}
	}

	// Every shape a real Kubernetes object name or node name takes.
	for _, value := range []string{"argocd-server-7d9f", "node1.internal.example.com", "kube_system", "ip-10-0-1-5", ""} {
		if _, err := Expand(`x{pod="$name"}`, Variables{Name: value}); err != nil {
			t.Errorf("Expand refused the legitimate name %q: %v", value, err)
		}
	}
}

// An unknown variable is an error rather than being left in the query, where
// Prometheus would read the `$` as something else entirely.
func TestExpandRefusesUnknownVariables(t *testing.T) {
	_, err := Expand(`x{cluster="$cluster"}`, Variables{})
	if err == nil {
		t.Fatal("Expand accepted an unknown variable")
	}
	if !strings.Contains(err.Error(), "$cluster") {
		t.Errorf("error %q does not name the variable", err)
	}
}

func TestCheckQuery(t *testing.T) {
	if err := CheckQuery(`sum(rate(x{pod="$name"}[5m]))`); err != nil {
		t.Errorf("CheckQuery refused a good query: %v", err)
	}
	if err := CheckQuery(""); err == nil {
		t.Error("CheckQuery accepted an empty query")
	}
	if err := CheckQuery(`x{a="$nope"}`); err == nil {
		t.Error("CheckQuery accepted an unknown variable")
	}
}

const matrix = `{
  "status": "success",
  "data": {
    "resultType": "matrix",
    "result": [
      {"metric": {"container": "server"}, "values": [[1700000000, "0.5"], [1700000060, "0.75"]]},
      {"metric": {"container": "redis"}, "values": [[1700000000, "0.1"]]}
    ]
  }
}`

func TestQueryRangeReadsAMatrix(t *testing.T) {
	got, err := QueryRange(context.Background(), fetchReturning(matrix), "up", "container", Range{Minutes: 60})
	if err != nil {
		t.Fatalf("QueryRange: %v", err)
	}
	if len(got.Series) != 2 {
		t.Fatalf("got %d series, want 2", len(got.Series))
	}
	// Sorted by name, so a series keeps its colour between refreshes rather than
	// moving from blue to orange on a reload.
	if got.Series[0].Name != "redis" || got.Series[1].Name != "server" {
		t.Errorf("series are %q and %q, want them sorted", got.Series[0].Name, got.Series[1].Name)
	}
	if len(got.Series[1].Points) != 2 || got.Series[1].Points[1].V != 0.75 {
		t.Errorf("points = %+v", got.Series[1].Points)
	}
}

// A gap and a zero mean opposite things on a chart, so a sample Prometheus
// could not compute is dropped rather than drawn as nothing.
func TestQueryRangeDropsUncomputableSamples(t *testing.T) {
	body := `{"status":"success","data":{"resultType":"matrix","result":[
		{"metric":{},"values":[[1,"NaN"],[2,"1.5"],[3,"+Inf"]]}]}}`

	got, err := QueryRange(context.Background(), fetchReturning(body), "up", "", Range{Minutes: 60})
	if err != nil {
		t.Fatalf("QueryRange: %v", err)
	}
	if len(got.Series) != 1 || len(got.Series[0].Points) != 1 {
		t.Fatalf("series = %+v, want one point", got.Series)
	}
	if got.Series[0].Points[0].V != 1.5 {
		t.Errorf("point = %+v", got.Series[0].Points[0])
	}
}

// A series with nothing left after that is dropped entirely rather than drawn
// as an empty line with a legend entry.
func TestQueryRangeDropsEmptySeries(t *testing.T) {
	body := `{"status":"success","data":{"resultType":"matrix","result":[
		{"metric":{"a":"b"},"values":[[1,"NaN"]]}]}}`

	got, _ := QueryRange(context.Background(), fetchReturning(body), "up", "", Range{Minutes: 60})
	if len(got.Series) != 0 {
		t.Errorf("series = %+v, want none", got.Series)
	}
}

func TestQueryRangeReportsPrometheusErrors(t *testing.T) {
	body := `{"status":"error","errorType":"bad_data","error":"parse error at char 4"}`

	_, err := QueryRange(context.Background(), fetchReturning(body), "up", "", Range{Minutes: 60})
	if err == nil {
		t.Fatal("a Prometheus error was not reported")
	}
	if !strings.Contains(err.Error(), "parse error") {
		t.Errorf("error %q does not carry Prometheus's own reason", err)
	}
}

// Reaching something that is not a Prometheus is the commonest misconfiguration,
// and "unexpected character at byte 4" helps nobody.
func TestQueryRangeExplainsANonPrometheusReply(t *testing.T) {
	_, err := QueryRange(context.Background(), fetchReturning("<html>404</html>"), "up", "", Range{Minutes: 60})
	if err == nil {
		t.Fatal("an HTML reply was accepted")
	}
	if !strings.Contains(err.Error(), "not a Prometheus response") {
		t.Errorf("error %q does not say what went wrong", err)
	}
}

func TestQueryRangePassesTheWindow(t *testing.T) {
	var seen map[string]string
	fetch := func(_ context.Context, path string, params map[string]string) ([]byte, error) {
		if path != "/api/v1/query_range" {
			t.Errorf("path = %q", path)
		}
		seen = params
		return []byte(matrix), nil
	}

	end := time.Unix(1700000000, 0)
	if _, err := QueryRange(context.Background(), fetch, "up", "", Range{Minutes: 60, End: end}); err != nil {
		t.Fatal(err)
	}

	if seen["query"] != "up" {
		t.Errorf("query = %q", seen["query"])
	}
	if seen["end"] != "1700000000" || seen["start"] != "1699996400" {
		t.Errorf("window = %s..%s, want an hour ending at the given time", seen["start"], seen["end"])
	}
	// 60 minutes over 120 steps is 30s, which is coarser than the floor.
	if seen["step"] != "30s" {
		t.Errorf("step = %q, want 30s", seen["step"])
	}
}

// Below Prometheus's usual scrape interval there is nothing new to say, and a
// finer step only makes the query more expensive.
func TestStepNeverGoesBelowTheScrapeInterval(t *testing.T) {
	if got := (Range{Minutes: 5}).step(); got != 15*time.Second {
		t.Errorf("step for 5 minutes = %v, want the 15s floor", got)
	}
	if got := (Range{Minutes: 1440}).step(); got != 12*time.Minute {
		t.Errorf("step for a day = %v, want 12m", got)
	}
}

func TestSeriesNaming(t *testing.T) {
	// The label the chart asked for.
	if got := seriesName(map[string]string{"container": "server", "pod": "x"}, "container"); got != "server" {
		t.Errorf("got %q, want the named label", got)
	}
	// A query that aggregates everything away: one line, and the chart's own
	// title already says what it is.
	if got := seriesName(map[string]string{}, "container"); got != "" {
		t.Errorf("got %q, want empty", got)
	}
	// No label to go on: the whole set, sorted, which is what Prometheus's own
	// UI shows and the only honest answer.
	if got := seriesName(map[string]string{"b": "2", "a": "1"}, ""); got != "a=1, b=2" {
		t.Errorf("got %q", got)
	}
	// The label the chart asked for is not on these series.
	if got := seriesName(map[string]string{"pod": "x"}, "container"); got != "pod=x" {
		t.Errorf("got %q, want the fallback", got)
	}
}

func TestQueryRangeCarriesFetchErrors(t *testing.T) {
	fetch := func(context.Context, string, map[string]string) ([]byte, error) {
		return nil, errors.New("connection refused")
	}
	if _, err := QueryRange(context.Background(), fetch, "up", "", Range{Minutes: 60}); err == nil {
		t.Fatal("a transport failure was swallowed")
	}
}
