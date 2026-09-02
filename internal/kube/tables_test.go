package kube

import (
	"strings"
	"testing"
)

func testContext(name string) Context {
	return Context{
		ID:      "/home/u/.kube/config::" + name,
		Name:    name,
		Cluster: name,
		Server:  "https://10.0.0.1:6443",
		File:    "/home/u/.kube/config",
	}
}

// allKinds is every kind the sidebar can open a tab for.
var allKinds = []string{
	KindNodes, KindNamespaces, KindPods, KindDeployments, KindStatefulSet,
	KindDaemonSets, KindJobs, KindCronJobs, KindServices, KindIngresses,
	KindConfigMaps, KindSecrets, KindPVCs, KindEvents,
}

func TestEveryKindReturnsAWellFormedTable(t *testing.T) {
	ctx := testContext("prod")

	for _, kind := range allKinds {
		t.Run(kind, func(t *testing.T) {
			table := BuildTable(ctx, kind, AllNamespaces)
			if table.Error != "" {
				t.Fatalf("unexpected error: %s", table.Error)
			}
			if len(table.Columns) == 0 {
				t.Fatal("table has no columns")
			}
			// The frontend renders cells positionally against the header row,
			// so a mismatch would silently shift every value one column over.
			for _, row := range table.Rows {
				if len(row.Cells) != len(table.Columns) {
					t.Fatalf("row %q has %d cells, want %d", row.Name, len(row.Cells), len(table.Columns))
				}
				if row.ID == "" || row.Name == "" {
					t.Fatalf("row is missing an identity: %+v", row)
				}
			}
		})
	}
}

func TestUnknownKindIsReportedInTheTable(t *testing.T) {
	table := BuildTable(testContext("prod"), "widgets", AllNamespaces)
	if table.Error == "" {
		t.Fatal("expected an error for an unknown kind")
	}
	// The UI renders the message in place of the rows, so the slices must not
	// be nil -- they are serialised straight to JSON.
	if table.Rows == nil || table.Columns == nil {
		t.Error("rows and columns should be empty, not nil")
	}
}

func TestTablesAreStableForTheSameContext(t *testing.T) {
	ctx := testContext("prod")

	for _, kind := range allKinds {
		first := BuildTable(ctx, kind, AllNamespaces)
		second := BuildTable(ctx, kind, AllNamespaces)

		if len(first.Rows) != len(second.Rows) {
			t.Fatalf("%s: row count changed between calls (%d then %d)", kind, len(first.Rows), len(second.Rows))
		}
		for i := range first.Rows {
			if first.Rows[i].ID != second.Rows[i].ID {
				t.Fatalf("%s: row %d changed between calls: %q then %q", kind, i, first.Rows[i].ID, second.Rows[i].ID)
			}
		}
	}
}

func TestDifferentContextsGetDifferentClusters(t *testing.T) {
	a := BuildOverview(testContext("prod"))
	b := BuildOverview(testContext("staging"))

	if a.Namespaces == nil || b.Namespaces == nil {
		t.Fatal("namespaces should never be nil")
	}
	same := a.Version == b.Version && a.Distribution == b.Distribution &&
		len(a.Namespaces) == len(b.Namespaces) && a.Stats[1].Total == b.Stats[1].Total
	if same {
		t.Error("two different contexts produced an identical cluster")
	}
}

func TestNamespaceFilterOnlyAppliesToNamespacedKinds(t *testing.T) {
	ctx := testContext("prod")
	ns := Namespaces(ctx)[0]

	pods := BuildTable(ctx, KindPods, ns)
	if !pods.Namespaced {
		t.Fatal("pods should be namespaced")
	}
	if len(pods.Rows) == 0 {
		t.Skip("this fabricated cluster has no pods in the first namespace")
	}
	for _, row := range pods.Rows {
		if row.Namespace != ns {
			t.Fatalf("row %q is in %q, want %q", row.Name, row.Namespace, ns)
		}
	}

	// Nodes are cluster-scoped, so a namespace filter must not empty the table.
	nodes := BuildTable(ctx, KindNodes, ns)
	if nodes.Namespaced {
		t.Error("nodes should not be namespaced")
	}
	if len(nodes.Rows) == 0 {
		t.Error("filtering by namespace removed the cluster-scoped nodes")
	}
}

func TestOverviewAgreesWithTheTables(t *testing.T) {
	ctx := testContext("prod")
	overview := BuildOverview(ctx)

	byLabel := map[string]Stat{}
	for _, s := range overview.Stats {
		byLabel[s.Label] = s
	}

	if got, want := byLabel["Nodes"].Total, len(BuildTable(ctx, KindNodes, AllNamespaces).Rows); got != want {
		t.Errorf("overview reports %d nodes, the nodes table has %d", got, want)
	}
	if got, want := byLabel["Pods"].Total, len(BuildTable(ctx, KindPods, AllNamespaces).Rows); got != want {
		t.Errorf("overview reports %d pods, the pods table has %d", got, want)
	}
	if got, want := byLabel["Namespaces"].Total, len(Namespaces(ctx)); got != want {
		t.Errorf("overview reports %d namespaces, Namespaces returned %d", got, want)
	}
	if byLabel["Nodes"].Ready > byLabel["Nodes"].Total {
		t.Error("more nodes ready than exist")
	}
}

func TestDescribeReportsTheObjectItWasAskedFor(t *testing.T) {
	ctx := testContext("prod")

	pods := BuildTable(ctx, KindPods, AllNamespaces)
	if len(pods.Rows) == 0 {
		t.Fatal("no pods to describe")
	}
	pod := pods.Rows[0]

	out := BuildDescribe(ctx, KindPods, pod.Namespace, pod.Name)
	for _, want := range []string{pod.Name, pod.Namespace, "Containers:", "Conditions:"} {
		if !strings.Contains(out, want) {
			t.Errorf("describe output is missing %q:\n%s", want, out)
		}
	}
}

func TestDescribeFallsBackForKindsWithoutAReport(t *testing.T) {
	ctx := testContext("prod")

	secrets := BuildTable(ctx, KindSecrets, AllNamespaces)
	if len(secrets.Rows) == 0 {
		t.Fatal("no secrets to describe")
	}
	secret := secrets.Rows[0]

	out := BuildDescribe(ctx, KindSecrets, secret.Namespace, secret.Name)
	if !strings.Contains(out, secret.Name) {
		t.Errorf("generic describe is missing the object name:\n%s", out)
	}
	if !strings.Contains(out, "Details") {
		t.Errorf("generic describe should list the row's columns:\n%s", out)
	}
}

func TestDescribeOfAMissingObjectSaysSo(t *testing.T) {
	out := BuildDescribe(testContext("prod"), KindPods, "default", "no-such-pod")
	if !strings.Contains(out, "no longer present") {
		t.Errorf("expected a not-found report, got:\n%s", out)
	}
}

func TestAgeFormatsLikeKubectl(t *testing.T) {
	cases := map[int]string{
		0:            "0s",
		45:           "45m",
		60:           "1h",
		90:           "1h30m",
		60 * 24 * 3:  "3d",
		60*24*3 + 60: "3d1h",
		60 * 24 * 40: "40d",
	}
	for minutes, want := range cases {
		if got := age(minutes); got != want {
			t.Errorf("age(%d) = %q, want %q", minutes, got, want)
		}
	}
}

func TestUnhealthyPodsAreNotShownAsFullyReady(t *testing.T) {
	table := BuildTable(testContext("prod"), KindPods, AllNamespaces)

	const (
		containersColumn = 2
		statusColumn     = 7
	)
	checked := 0
	for _, row := range table.Rows {
		status := row.Cells[statusColumn].Text
		if status == "Running" {
			continue
		}
		checked++

		// "0/2" style. A pod that is not running cannot have every container
		// ready, or the table would contradict its own status column.
		containers := row.Cells[containersColumn].Text
		ready, total, ok := strings.Cut(containers, "/")
		if !ok {
			t.Fatalf("%s: containers cell %q is not ready/total", row.Name, containers)
		}
		if ready == total {
			t.Errorf("%s is %s but reports %s containers ready", row.Name, status, containers)
		}
	}
	if checked == 0 {
		t.Skip("this fabricated cluster happens to be entirely healthy")
	}
}
