package plugins

import (
	"errors"
	"testing"

	"github.com/roger/k8sdockside/internal/kube"
)

// fakeCluster answers the two questions Summarise asks, so the wording and
// ordering can be tested without a Kubernetes API.
type fakeCluster struct {
	served  map[string]bool
	tallies map[string]kube.Tally
	fail    map[string]error
	// asked counts the discovery calls, to prove a kind named several times is
	// only looked up once.
	asked map[string]int
}

func (f *fakeCluster) KindServed(kind string) (bool, error) {
	if f.asked == nil {
		f.asked = map[string]int{}
	}
	f.asked[kind]++
	if err, bad := f.fail[kind]; bad {
		return false, err
	}
	return f.served[kind], nil
}

func (f *fakeCluster) CountBy(kind, _, _ string, _ kube.FieldPath) (kube.Tally, error) {
	if err, bad := f.fail[kind]; bad {
		return kube.Tally{}, err
	}
	return f.tallies[kind], nil
}

var argo = Plugin{
	ID:   "argocd",
	Name: "Argo CD",
	Requires: []Requirement{
		{Kind: "crd:applications.argoproj.io", Label: "Applications"},
		{Kind: "crd:applicationsets.argoproj.io", Label: "Application Sets", Optional: true},
	},
	Cards: []Card{{
		Label:   "Applications by health",
		Kind:    "crd:applications.argoproj.io",
		GroupBy: "status.health.status",
		Tones: map[string]string{
			"Healthy": "ok", "Progressing": "warn", "Degraded": "error", "Unknown": "info",
		},
	}},
}

func TestSummariseAHealthyInstall(t *testing.T) {
	cl := &fakeCluster{
		served: map[string]bool{"crd:applications.argoproj.io": true, "crd:applicationsets.argoproj.io": true},
		tallies: map[string]kube.Tally{
			"crd:applications.argoproj.io": {Total: 42, Counts: map[string]int{"Healthy": 38, "Progressing": 3, "Degraded": 1}},
		},
	}

	got := Summarise(argo, cl)

	if !got.Installed || !got.Checked {
		t.Fatalf("summary = %+v, want installed and checked", got)
	}
	if len(got.Cards) != 1 || got.Cards[0].Total != 42 {
		t.Fatalf("cards = %+v", got.Cards)
	}
	// The kind is named by a requirement and a card; asking twice would be two
	// discovery round trips for one answer.
	if cl.asked["crd:applications.argoproj.io"] != 1 {
		t.Errorf("looked the kind up %d times, want 1", cl.asked["crd:applications.argoproj.io"])
	}
}

// A missing optional requirement is reported but must not make the plugin read
// as absent: Argo CD without ApplicationSets is still Argo CD.
func TestOptionalRequirementsDoNotDecideInstalled(t *testing.T) {
	cl := &fakeCluster{served: map[string]bool{"crd:applications.argoproj.io": true}}

	got := Summarise(argo, cl)

	if !got.Installed {
		t.Error("a missing optional requirement made the plugin read as not installed")
	}
	var sets Presence
	for _, req := range got.Requirements {
		if req.Kind == "crd:applicationsets.argoproj.io" {
			sets = req
		}
	}
	if sets.Served || !sets.Optional {
		t.Errorf("the optional requirement is reported as %+v", sets)
	}
}

// The state the user asked about: the plugin is installed on this machine and
// the CRDs are not in this cluster.
func TestSummariseWhenTheSolutionIsNotInstalled(t *testing.T) {
	got := Summarise(argo, &fakeCluster{served: map[string]bool{}})

	if got.Installed {
		t.Error("a cluster serving none of the CRDs read as installed")
	}
	if !got.Checked {
		t.Error("we did ask and got an answer, so the answer should count")
	}
	if len(got.Cards) != 1 || got.Cards[0].Error == "" {
		t.Fatalf("the card should say why it has no number: %+v", got.Cards)
	}
	if got.Cards[0].Total != 0 {
		t.Errorf("total = %d, want nothing counted", got.Cards[0].Total)
	}
}

// An unreachable cluster must not read as "Argo CD is not installed here" --
// they call for opposite reactions.
func TestAnUnreachableClusterIsNotTheSameAsNotInstalled(t *testing.T) {
	cl := &fakeCluster{fail: map[string]error{
		"crd:applications.argoproj.io":    errors.New("connection refused"),
		"crd:applicationsets.argoproj.io": errors.New("connection refused"),
	}}

	got := Summarise(argo, cl)

	if got.Checked {
		t.Error("a cluster we could not ask was reported as checked")
	}
	if got.Installed {
		t.Error("a cluster we could not ask was reported as installed")
	}
	if got.Error == "" {
		t.Error("nothing says why the check did not happen")
	}
	if got.Requirements[0].Error == "" {
		t.Error("the requirement does not carry its reason")
	}
}

// One Degraded application among forty Healthy ones is the reason the tile is
// on screen; ordering by count would bury it under the good news.
func TestBucketsLeadWithWhatIsWrong(t *testing.T) {
	cl := &fakeCluster{
		served: map[string]bool{"crd:applications.argoproj.io": true, "crd:applicationsets.argoproj.io": true},
		tallies: map[string]kube.Tally{
			"crd:applications.argoproj.io": {Total: 45, Counts: map[string]int{
				"Healthy": 38, "Progressing": 3, "Degraded": 1, "Unknown": 2, "": 1,
			}},
		},
	}

	buckets := Summarise(argo, cl).Cards[0].Buckets

	var order []string
	for _, b := range buckets {
		order = append(order, b.Value)
	}
	want := []string{"Degraded", "Progressing", "Healthy", "Unknown", ""}
	if len(order) != len(want) {
		t.Fatalf("buckets = %v, want %v", order, want)
	}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("buckets = %v, want %v (worst first, the absent-field bucket last)", order, want)
		}
	}
	if buckets[0].Tone != "error" {
		t.Errorf("the Degraded bucket has tone %q", buckets[0].Tone)
	}
}

// A card with no groupBy is a plain count, and should not pretend to a
// breakdown it was never given.
func TestAnUngroupedCardIsJustACount(t *testing.T) {
	plugin := Plugin{ID: "x", Cards: []Card{{Label: "Pods", Kind: "pods"}}}
	cl := &fakeCluster{
		served:  map[string]bool{"pods": true},
		tallies: map[string]kube.Tally{"pods": {Total: 7, Counts: map[string]int{"": 7}}},
	}

	card := Summarise(plugin, cl).Cards[0]

	if card.Total != 7 {
		t.Errorf("total = %d, want 7", card.Total)
	}
	if card.Grouped || len(card.Buckets) != 0 {
		t.Errorf("card = %+v, want no breakdown", card)
	}
}

// A plugin that declares no requirements is for something with no CRDs to look
// for, and is taken at its word rather than reported as missing.
func TestAPluginWithNoRequirementsCountsAsInstalled(t *testing.T) {
	plugin := Plugin{ID: "x", Views: []View{{ID: "pods", Kind: "pods"}}}

	if got := Summarise(plugin, &fakeCluster{}); !got.Installed {
		t.Errorf("summary = %+v, want installed", got)
	}
}
