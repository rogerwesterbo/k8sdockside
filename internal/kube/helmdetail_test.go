package kube

import (
	"encoding/json/jsontext"
	"strconv"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// revisionSecret is helmSecret with the labels Helm writes alongside the
// payload, which is what the history reads to order revisions without
// decompressing them.
func revisionSecret(t *testing.T, name string, revision int, payload string) *unstructured.Unstructured {
	t.Helper()
	u := helmSecret(t, "sh.helm.release.v1."+name+".v"+strconv.Itoa(revision), payload)
	u.SetLabels(map[string]string{
		helmOwnerLabel:   helmOwner,
		helmNameLabel:    name,
		helmVersionLabel: strconv.Itoa(revision),
	})
	return u
}

const detailedRelease = `{
  "name": "rook-ceph",
  "version": 3,
  "namespace": "rook-ceph",
  "info": {
    "status": "deployed",
    "first_deployed": "2026-06-01T09:00:00Z",
    "last_deployed": "2026-08-05T11:27:04Z",
    "description": "Upgrade complete",
    "notes": "The Rook Operator has been installed.\n"
  },
  "chart": {
    "metadata": {"name": "rook-ceph", "version": "v1.19.8", "appVersion": "v1.19.8"},
    "values": {"allowLoopDevices": false, "csi": {"enableRbdDriver": true, "provisionerReplicas": 2}}
  },
  "config": {"csi": {"provisionerReplicas": 3}},
  "manifest": "---\n# Source: rook-ceph/templates/sa.yaml\napiVersion: v1\nkind: ServiceAccount\nmetadata:\n  name: rook-ceph-system\n---\napiVersion: rbac.authorization.k8s.io/v1\nkind: ClusterRole\nmetadata:\n  name: rook-ceph-global\n"
}`

func TestReleaseDetailCarriesWhatTheDrawerShows(t *testing.T) {
	detail, err := decodeHelmDetail(helmSecret(t, "sh.helm.release.v1.rook-ceph.v3", detailedRelease))
	if err != nil {
		t.Fatalf("decodeHelmDetail: %v", err)
	}

	if detail.Chart != "rook-ceph-v1.19.8" {
		t.Errorf("chart = %q, want rook-ceph-v1.19.8", detail.Chart)
	}
	// Carried apart as well, because an upgrade asks the repo for the chart by
	// name and offers its versions.
	if detail.ChartName != "rook-ceph" || detail.ChartVersion != "v1.19.8" {
		t.Errorf("chart split = %q / %q, want rook-ceph / v1.19.8", detail.ChartName, detail.ChartVersion)
	}
	if detail.Status != "deployed" {
		t.Errorf("status = %q, want deployed", detail.Status)
	}
	if detail.Description != "Upgrade complete" {
		t.Errorf("description = %q, want Upgrade complete", detail.Description)
	}
	if detail.FirstDeployed != "2026-06-01T09:00:00Z" {
		t.Errorf("first deployed = %q", detail.FirstDeployed)
	}
	if !strings.Contains(detail.Notes, "Rook Operator has been installed") {
		t.Errorf("notes = %q, want the rendered NOTES.txt", detail.Notes)
	}
}

// The whole point of the "user-supplied values only" toggle: one document is
// what the release is doing, the other is what somebody typed.
func TestUserValuesAreTheOverridesAloneAndValuesAreTheMerge(t *testing.T) {
	detail, err := decodeHelmDetail(helmSecret(t, "sh.helm.release.v1.rook-ceph.v3", detailedRelease))
	if err != nil {
		t.Fatalf("decodeHelmDetail: %v", err)
	}

	if strings.Contains(detail.UserValues, "allowLoopDevices") {
		t.Errorf("user values carried a chart default:\n%s", detail.UserValues)
	}
	if !strings.Contains(detail.UserValues, "provisionerReplicas: 3") {
		t.Errorf("user values lost the override:\n%s", detail.UserValues)
	}

	// The merged document keeps the defaults the override said nothing about,
	// and shows the override where it did.
	if !strings.Contains(detail.Values, "allowLoopDevices: false") {
		t.Errorf("merged values lost a chart default:\n%s", detail.Values)
	}
	if !strings.Contains(detail.Values, "enableRbdDriver: true") {
		t.Errorf("merged values lost a nested chart default:\n%s", detail.Values)
	}
	if !strings.Contains(detail.Values, "provisionerReplicas: 3") {
		t.Errorf("merged values did not take the override:\n%s", detail.Values)
	}
	if strings.Contains(detail.Values, "provisionerReplicas: 2") {
		t.Errorf("merged values kept the overridden default:\n%s", detail.Values)
	}
}

// Why the merge works on raw JSON rather than on decoded maps: a values file is
// free to carry an integer larger than a float64 can hold exactly, and a round
// trip through `any` would hand it back as a different number.
func TestMergingValuesDoesNotRoundTripNumbersThroughFloats(t *testing.T) {
	const big = "9007199254740993" // 2^53 + 1, the first integer a float64 cannot hold
	defaults := jsontext.Value(`{"snowflake": ` + big + `, "keep": 1}`)
	override := jsontext.Value(`{"other": ` + big + `}`)

	got := valuesYAML(mergeValues(defaults, override))

	if !strings.Contains(got, "snowflake: "+big) {
		t.Errorf("a default integer was not preserved exactly:\n%s", got)
	}
	if !strings.Contains(got, "other: "+big) {
		t.Errorf("an override integer was not preserved exactly:\n%s", got)
	}
}

// Helm's own rule, and the one people are surprised by: a list in a values file
// is not appended to, it is swapped out.
func TestAListInTheOverrideReplacesTheDefaultRatherThanMerging(t *testing.T) {
	got := valuesYAML(mergeValues(
		jsontext.Value(`{"tolerations": [{"key": "a"}, {"key": "b"}]}`),
		jsontext.Value(`{"tolerations": [{"key": "c"}]}`),
	))

	if strings.Contains(got, "key: a") || strings.Contains(got, "key: b") {
		t.Errorf("the default list survived an override:\n%s", got)
	}
	if !strings.Contains(got, "key: c") {
		t.Errorf("the override list was lost:\n%s", got)
	}
}

func TestAReleaseWithNoOverridesHasNoUserValuesToShow(t *testing.T) {
	const noConfig = `{
	  "name": "app", "version": 1,
	  "info": {"status": "deployed"},
	  "chart": {"metadata": {"name": "app", "version": "1.0.0"}, "values": {"replicas": 1}},
	  "config": {}
	}`

	detail, err := decodeHelmDetail(helmSecret(t, "sh.helm.release.v1.app.v1", noConfig))
	if err != nil {
		t.Fatalf("decodeHelmDetail: %v", err)
	}

	// Empty rather than "{}", which would read as a value where what is meant
	// is the absence of one.
	if detail.UserValues != "" {
		t.Errorf("user values = %q, want empty", detail.UserValues)
	}
	if !strings.Contains(detail.Values, "replicas: 1") {
		t.Errorf("merged values lost the chart's defaults:\n%s", detail.Values)
	}
}

func TestTheManifestIsListedAsTheObjectsItRenders(t *testing.T) {
	detail, err := decodeHelmDetail(helmSecret(t, "sh.helm.release.v1.rook-ceph.v3", detailedRelease))
	if err != nil {
		t.Fatalf("decodeHelmDetail: %v", err)
	}

	if len(detail.Resources) != 2 {
		t.Fatalf("resources = %d, want 2: %+v", len(detail.Resources), detail.Resources)
	}
	// Sorted by kind, so ClusterRole leads ServiceAccount.
	if detail.Resources[0].Kind != "ClusterRole" || detail.Resources[0].Name != "rook-ceph-global" {
		t.Errorf("first resource = %+v", detail.Resources[0])
	}
	if detail.Resources[1].Kind != "ServiceAccount" || detail.Resources[1].Name != "rook-ceph-system" {
		t.Errorf("second resource = %+v", detail.Resources[1])
	}
}

// A template guarded by a false `if` renders its separator and nothing else,
// which every chart of any size does somewhere.
func TestEmptyManifestDocumentsAreNotListedAsResources(t *testing.T) {
	manifest := "---\n# Source: app/templates/ingress.yaml\n---\napiVersion: v1\nkind: Service\nmetadata:\n  name: app\n---\n"

	got := manifestResources(manifest)

	if len(got) != 1 {
		t.Fatalf("resources = %d, want 1: %+v", len(got), got)
	}
	if got[0].Name != "app" {
		t.Errorf("resource = %+v, want the Service", got[0])
	}
}

func TestAnObjectRenderedTwiceIsListedOnce(t *testing.T) {
	doc := "apiVersion: v1\nkind: ServiceAccount\nmetadata:\n  name: shared\n"

	got := manifestResources(doc + "---\n" + doc)

	if len(got) != 1 {
		t.Fatalf("resources = %d, want 1: %+v", len(got), got)
	}
}

func TestAManifestThatIsNotYAMLYieldsNoResourcesRatherThanFailing(t *testing.T) {
	if got := manifestResources("\tthis is not: [yaml"); len(got) != 0 {
		t.Errorf("resources = %+v, want none", got)
	}
	if got := manifestResources(""); got == nil {
		t.Error("an empty manifest gave a nil list, which marshals as null rather than []")
	}
}

func TestHistoryIsNewestFirstWithTheCurrentRevisionMarked(t *testing.T) {
	release := func(version int, status, chartVersion, description string) string {
		return `{"name":"app","version":` + strconv.Itoa(version) +
			`,"info":{"status":"` + status + `","description":"` + description + `"},` +
			`"chart":{"metadata":{"name":"app","version":"` + chartVersion + `"}}}`
	}

	// Out of order on the wire, which is how the API server returns them.
	items := []unstructured.Unstructured{
		*revisionSecret(t, "app", 2, release(2, "superseded", "1.1.0", "Upgrade complete")),
		*revisionSecret(t, "app", 5, release(5, "deployed", "1.4.0", "Rollback to 3")),
		*revisionSecret(t, "app", 3, release(3, "superseded", "1.2.0", "Upgrade complete")),
	}

	current, revisions := readRevisions(items)

	if current == nil {
		t.Fatal("no current revision was picked")
	}
	if got := revisionNumber(current); got != 5 {
		t.Errorf("current revision = %d, want 5", got)
	}

	if len(revisions) != 3 {
		t.Fatalf("revisions = %d, want 3", len(revisions))
	}
	if revisions[0].Revision != 5 || revisions[1].Revision != 3 || revisions[2].Revision != 2 {
		t.Errorf("revisions are not newest first: %+v", revisions)
	}
	if !revisions[0].Current {
		t.Error("the newest revision was not marked current")
	}
	if revisions[1].Current || revisions[2].Current {
		t.Error("an older revision was marked current")
	}
	// The column that makes a history readable, since the rest of the row
	// repeats itself.
	if revisions[0].Description != "Rollback to 3" {
		t.Errorf("description = %q, want Rollback to 3", revisions[0].Description)
	}
}

// A long-lived release on a cluster that raised --history-max carries however
// many revisions it was told to, and each one has to be decompressed to be
// read. The newest are the ones anyone is looking at.
func TestHistoryIsCutToTheNewestRevisionsBeforeAnythingIsDecoded(t *testing.T) {
	items := make([]unstructured.Unstructured, 0, maxRevisions+10)
	for version := 1; version <= maxRevisions+10; version++ {
		payload := `{"name":"app","version":` + strconv.Itoa(version) +
			`,"info":{"status":"superseded"},"chart":{"metadata":{"name":"app","version":"1.0.0"}}}`
		items = append(items, *revisionSecret(t, "app", version, payload))
	}

	current, revisions := readRevisions(items)

	if len(revisions) != maxRevisions {
		t.Fatalf("revisions = %d, want %d", len(revisions), maxRevisions)
	}
	if got := revisionNumber(current); got != int64(maxRevisions+10) {
		t.Errorf("current revision = %d, want %d", got, maxRevisions+10)
	}
	if revisions[0].Revision != int64(maxRevisions+10) {
		t.Errorf("the newest revision was cut: first = %d", revisions[0].Revision)
	}
}

func TestARevisionThatCannotBeDecodedIsSkippedRatherThanFailingTheHistory(t *testing.T) {
	good := revisionSecret(t, "app", 2,
		`{"name":"app","version":2,"info":{"status":"deployed"},"chart":{"metadata":{"name":"app","version":"1.0.0"}}}`)
	broken := obj(map[string]any{
		"metadata": map[string]any{
			"name":      "sh.helm.release.v1.app.v1",
			"namespace": "prod",
			"labels": map[string]any{
				helmOwnerLabel: helmOwner, helmNameLabel: "app", helmVersionLabel: "1",
			},
		},
		"type": HelmReleaseSecretType,
		"data": map[string]any{"release": "not base64 at all !!"},
	})

	_, revisions := readRevisions([]unstructured.Unstructured{*good, *broken})

	if len(revisions) != 1 {
		t.Fatalf("revisions = %d, want 1 -- the readable one", len(revisions))
	}
	if revisions[0].Revision != 2 {
		t.Errorf("revision = %d, want 2", revisions[0].Revision)
	}
}

func TestNoRevisionsMeansNoCurrentRelease(t *testing.T) {
	current, revisions := readRevisions(nil)

	if current != nil {
		t.Error("a current revision was picked from an empty listing")
	}
	if revisions == nil {
		t.Error("an empty history was nil, which marshals as null rather than []")
	}
}

func TestOneReleaseIsSelectedByTheLabelsHelmWrites(t *testing.T) {
	if got := helmReleaseSelector("ingress-nginx"); got != "owner=helm,name=ingress-nginx" {
		t.Errorf("selector = %q, want owner=helm,name=ingress-nginx", got)
	}
}
