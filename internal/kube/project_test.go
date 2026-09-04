package kube

import (
	"slices"
	"strconv"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// obj builds an unstructured object from a literal, so the tests read like the
// YAML they stand in for.
func obj(fields map[string]any) *unstructured.Unstructured {
	u := &unstructured.Unstructured{Object: fields}
	created := u.GetCreationTimestamp()
	if created.IsZero() {
		u.SetCreationTimestamp(metav1.NewTime(time.Now().Add(-90 * time.Minute)))
	}
	return u
}

func cellsByHeader(t *testing.T, table Table, row int) map[string]string {
	t.Helper()
	if row >= len(table.Rows) {
		t.Fatalf("row %d out of range, table has %d rows", row, len(table.Rows))
	}
	out := map[string]string{}
	for i, header := range table.Columns {
		if i < len(table.Rows[row].Cells) {
			out[header] = table.Rows[row].Cells[i].Text
		}
	}
	return out
}

func TestPodStatusPrefersTheWaitingReasonOverThePhase(t *testing.T) {
	pod := obj(map[string]any{
		"metadata": map[string]any{"name": "api-0", "namespace": "prod"},
		"spec":     map[string]any{"nodeName": "worker-1", "containers": []any{map[string]any{"name": "api"}}},
		"status": map[string]any{
			"phase":    "Pending",
			"qosClass": "Burstable",
			"containerStatuses": []any{map[string]any{
				"ready":        false,
				"restartCount": int64(7),
				"state":        map[string]any{"waiting": map[string]any{"reason": "ImagePullBackOff"}},
			}},
		},
	})

	table := buildLiveTable(KindPods, withNamespace(builtinColumns[KindPods], true), true, []*unstructured.Unstructured{pod})
	cells := cellsByHeader(t, table, 0)

	// The phase alone would say "Pending", which hides why.
	if cells["Status"] != "ImagePullBackOff" {
		t.Errorf("Status = %q, want ImagePullBackOff", cells["Status"])
	}
	if cells["Ready"] != "0/1" {
		t.Errorf("Ready = %q, want 0/1", cells["Ready"])
	}
	if cells["Restarts"] != "7" {
		t.Errorf("Restarts = %q, want 7", cells["Restarts"])
	}
	if cells["Node"] != "worker-1" {
		t.Errorf("Node = %q, want worker-1", cells["Node"])
	}
	if cells["Namespace"] != "prod" {
		t.Errorf("Namespace = %q, want prod", cells["Namespace"])
	}
}

func TestPodRestartsAreSummedAcrossContainers(t *testing.T) {
	pod := obj(map[string]any{
		"metadata": map[string]any{"name": "api-0"},
		"status": map[string]any{
			"phase": "Running",
			"containerStatuses": []any{
				map[string]any{"ready": true, "restartCount": int64(2), "state": map[string]any{"running": map[string]any{}}},
				map[string]any{"ready": true, "restartCount": int64(3), "state": map[string]any{"running": map[string]any{}}},
			},
		},
	})

	table := buildLiveTable(KindPods, builtinColumns[KindPods], false, []*unstructured.Unstructured{pod})
	cells := cellsByHeader(t, table, 0)

	if cells["Restarts"] != "5" {
		t.Errorf("Restarts = %q, want 5", cells["Restarts"])
	}
	if cells["Ready"] != "2/2" {
		t.Errorf("Ready = %q, want 2/2", cells["Ready"])
	}
	if cells["Status"] != "Running" {
		t.Errorf("Status = %q, want Running", cells["Status"])
	}
}

func TestGatewayColumnsReadAddressAndProgrammedCondition(t *testing.T) {
	gw := obj(map[string]any{
		"metadata": map[string]any{"name": "external", "namespace": "ingress"},
		"spec":     map[string]any{"gatewayClassName": "cilium"},
		"status": map[string]any{
			"addresses":  []any{map[string]any{"type": "IPAddress", "value": "192.168.3.200"}},
			"conditions": []any{map[string]any{"type": "Programmed", "status": "True"}},
		},
	})

	table := buildLiveTable(KindGateways, withNamespace(builtinColumns[KindGateways], true), true, []*unstructured.Unstructured{gw})
	cells := cellsByHeader(t, table, 0)

	if cells["Class"] != "cilium" {
		t.Errorf("Class = %q, want cilium", cells["Class"])
	}
	if cells["Address"] != "192.168.3.200" {
		t.Errorf("Address = %q, want 192.168.3.200", cells["Address"])
	}
	if cells["Programmed"] != "True" {
		t.Errorf("Programmed = %q, want True", cells["Programmed"])
	}
}

func TestHTTPRouteListsItsParentGateways(t *testing.T) {
	route := obj(map[string]any{
		"metadata": map[string]any{"name": "api", "namespace": "prod"},
		"spec": map[string]any{
			"hostnames":  []any{"api.example.com", "www.example.com"},
			"parentRefs": []any{map[string]any{"name": "external", "namespace": "ingress", "sectionName": "https"}},
		},
	})

	table := buildLiveTable(KindHTTPRoutes, withNamespace(routeColumns, true), true, []*unstructured.Unstructured{route})
	cells := cellsByHeader(t, table, 0)

	if want := "api.example.com, www.example.com"; cells["Hostnames"] != want {
		t.Errorf("Hostnames = %q, want %q", cells["Hostnames"], want)
	}
	if want := "ingress/external#https"; cells["Parents"] != want {
		t.Errorf("Parents = %q, want %q", cells["Parents"], want)
	}
}

func TestJSONPathColumnsSurviveAMissingField(t *testing.T) {
	// An optional field that is simply absent must render blank, not break the
	// row -- most CRD printer columns point at optional status fields.
	svc := obj(map[string]any{"metadata": map[string]any{"name": "headless", "namespace": "data"}})

	table := buildLiveTable(KindServices, withNamespace(builtinColumns[KindServices], true), true, []*unstructured.Unstructured{svc})
	cells := cellsByHeader(t, table, 0)

	if cells["Type"] != "" {
		t.Errorf("Type = %q, want empty", cells["Type"])
	}
	if cells["Ports"] != "<none>" {
		t.Errorf("Ports = %q, want <none>", cells["Ports"])
	}
	if cells["External IP"] != "<pending>" {
		t.Errorf("External IP = %q, want <pending>", cells["External IP"])
	}
}

func TestCRDVersionsMarkTheStorageVersion(t *testing.T) {
	crd := obj(map[string]any{
		"metadata": map[string]any{"name": "gateways.gateway.networking.k8s.io"},
		"spec": map[string]any{
			"group": "gateway.networking.k8s.io",
			"scope": "Namespaced",
			"names": map[string]any{"kind": "Gateway"},
			"versions": []any{
				map[string]any{"name": "v1beta1", "served": true, "storage": false},
				map[string]any{"name": "v1", "served": true, "storage": true},
			},
		},
	})

	table := buildLiveTable(KindCRDs, builtinColumns[KindCRDs], false, []*unstructured.Unstructured{crd})
	cells := cellsByHeader(t, table, 0)

	if want := "v1beta1, v1*"; cells["Version"] != want {
		t.Errorf("Version = %q, want %q", cells["Version"], want)
	}
	if cells["Kind"] != "Gateway" {
		t.Errorf("Kind = %q, want Gateway", cells["Kind"])
	}
}

func TestPickCRDVersionPrefersStorageOverServed(t *testing.T) {
	crd := obj(map[string]any{
		"metadata": map[string]any{"name": "certificates.cert-manager.io"},
		"spec": map[string]any{"versions": []any{
			map[string]any{"name": "v1alpha2", "served": true, "storage": false,
				"additionalPrinterColumns": []any{map[string]any{"name": "Old", "jsonPath": ".status.old"}}},
			map[string]any{"name": "v1", "served": true, "storage": true,
				"additionalPrinterColumns": []any{map[string]any{"name": "Ready", "jsonPath": `.status.conditions[?(@.type=="Ready")].status`}}},
		}},
	})

	cols := pickCRDVersion(crd)
	if len(cols) != 1 {
		t.Fatalf("got %d columns, want 1: %v", len(cols), cols)
	}
	if name := mapString(asMap(cols[0]), "name"); name != "Ready" {
		t.Errorf("column = %q, want the storage version's Ready", name)
	}
}

// TestCRDColumnsKeepTheStandardTableAndDropTheWideOnes pins the priority rule:
// a tab is a plain `kubectl get`, so priority-0 columns are the table and the
// higher ones -- the `-o wide` extras -- are left out.
func TestCRDColumnsKeepTheStandardTableAndDropTheWideOnes(t *testing.T) {
	crd := obj(map[string]any{
		"metadata": map[string]any{"name": "certificates.cert-manager.io"},
		"spec": map[string]any{"versions": []any{
			map[string]any{"name": "v1", "served": true, "storage": true,
				"additionalPrinterColumns": []any{
					map[string]any{"name": "Ready", "jsonPath": ".status.ready"},
					// Wide-only, spelled both ways a decoded manifest can hold
					// a number.
					map[string]any{"name": "Issuer", "jsonPath": ".spec.issuerRef.name", "priority": int64(1)},
					map[string]any{"name": "Secret", "jsonPath": ".spec.secretName", "priority": float64(1)},
					// Absent priority means 0, which is the standard table.
					map[string]any{"name": "Expires", "jsonPath": ".status.notAfter"},
					// A CRD's own Age would double the one we append.
					map[string]any{"name": "Age", "jsonPath": ".metadata.creationTimestamp"},
					// Neither half of a column is optional.
					map[string]any{"name": "Nameless", "jsonPath": ""},
					map[string]any{"name": "", "jsonPath": ".spec.pathless"},
				}},
		}},
	})

	var got []string
	for _, c := range crdColumns(crd) {
		got = append(got, c.Name)
	}

	want := []string{"Name", "Ready", "Expires", "Age"}
	if !slices.Equal(got, want) {
		t.Errorf("columns = %v, want %v", got, want)
	}
}

func TestStripBulkRedactsSecretValuesBeforeTheyAreCached(t *testing.T) {
	secret := obj(map[string]any{
		"kind":     "Secret",
		"metadata": map[string]any{"name": "api-tls", "namespace": "prod", "managedFields": []any{map[string]any{"manager": "kubectl"}}},
		"type":     "kubernetes.io/tls",
		"data":     map[string]any{"tls.crt": "REALCERT", "tls.key": "REALKEY"},
	})

	out, err := stripBulk(secret)
	if err != nil {
		t.Fatal(err)
	}
	u := out.(*unstructured.Unstructured)

	data, _, _ := unstructured.NestedMap(u.Object, "data")
	if len(data) != 2 {
		t.Errorf("got %d keys, want the key names kept for the count", len(data))
	}
	for k, v := range data {
		if v != "" {
			t.Errorf("data[%q] = %v, want the value dropped before caching", k, v)
		}
	}
	if u.GetManagedFields() != nil {
		t.Error("managed fields were cached")
	}
}

func TestParseCustomKindRoundTrips(t *testing.T) {
	kind := CustomKind("certificates", "cert-manager.io")
	if kind != "crd:certificates.cert-manager.io" {
		t.Fatalf("kind = %q", kind)
	}

	plural, group, ok := ParseCustomKind(kind)
	if !ok || plural != "certificates" || group != "cert-manager.io" {
		t.Errorf("ParseCustomKind = (%q, %q, %t)", plural, group, ok)
	}

	for _, bad := range []string{"pods", "crd:", "crd:nogroup", ""} {
		if _, _, ok := ParseCustomKind(bad); ok {
			t.Errorf("ParseCustomKind(%q) accepted a non-custom kind", bad)
		}
	}
}

// event builds an Event object whose last-seen moment is `ago` in the past.
func event(name, reason string, ago time.Duration) *unstructured.Unstructured {
	return obj(map[string]any{
		"metadata":       map[string]any{"name": name, "namespace": "default"},
		"type":           "Warning",
		"reason":         reason,
		"message":        reason + " happened",
		"involvedObject": map[string]any{"kind": "Pod", "name": name},
		"lastTimestamp":  time.Now().Add(-ago).Format(time.RFC3339),
	})
}

func TestEventsComeBackMostRecentFirst(t *testing.T) {
	// Deliberately built in an order that both alphabetical-by-type and
	// alphabetical-by-name would leave alone, so only a real time sort passes.
	items := []*unstructured.Unstructured{
		event("alpha", "Older", 6*time.Hour),
		event("bravo", "Newest", 2*time.Minute),
		event("charlie", "Oldest", 40*time.Hour),
		event("delta", "Middle", 90*time.Minute),
	}

	table := buildLiveTable(KindEvents, builtinColumns[KindEvents], false, items)

	var order []string
	for _, row := range table.Rows {
		order = append(order, row.Name)
	}
	if want := []string{"bravo", "delta", "alpha", "charlie"}; !slices.Equal(order, want) {
		t.Errorf("order = %v, want %v (most recent first)", order, want)
	}
}

func TestEventsStayOrderedAsTheyArrive(t *testing.T) {
	// A watch delivers changes in no particular order; the table has to be
	// sorted on every projection, not merely seeded that way.
	first := buildLiveTable(KindEvents, builtinColumns[KindEvents], false, []*unstructured.Unstructured{
		event("old", "Old", 5*time.Hour),
	})
	if first.Rows[0].Name != "old" {
		t.Fatalf("first row = %q", first.Rows[0].Name)
	}

	second := buildLiveTable(KindEvents, builtinColumns[KindEvents], false, []*unstructured.Unstructured{
		event("old", "Old", 5*time.Hour),
		event("new", "New", 1*time.Minute),
	})
	if second.Rows[0].Name != "new" {
		t.Errorf("a newly arrived event landed at row %d, want the top", indexOf(second, "new"))
	}
}

func TestAgeCellsSortByTimeNotByText(t *testing.T) {
	// "5m", "2h" and "3d" in text order read 2h, 3d, 5m -- which is why the
	// cell carries seconds alongside what it displays.
	items := []*unstructured.Unstructured{
		event("a", "A", 5*time.Minute),
		event("b", "B", 2*time.Hour),
		event("c", "C", 3*24*time.Hour),
	}

	table := buildLiveTable(KindEvents, builtinColumns[KindEvents], false, items)
	at := slices.Index(table.Columns, "Last Seen")
	if at == -1 {
		t.Fatal("no Last Seen column")
	}

	var texts []string
	var keys []int64
	for _, row := range table.Rows {
		texts = append(texts, row.Cells[at].Text)
		keys = append(keys, seconds(t, row.Cells[at]))
	}
	if want := []string{"5m", "2h", "3d"}; !slices.Equal(texts, want) {
		t.Errorf("displayed = %v, want %v", texts, want)
	}
	// The keys must rise as the rows go from most to least recent; sorting the
	// text alone would have produced 2h, 3d, 5m.
	for i := 1; i < len(keys); i++ {
		if keys[i-1] >= keys[i] {
			t.Errorf("sort keys %v are not increasing at %d", keys, i)
		}
	}
}

// seconds reads a cell's sort key as the number it is meant to be.
func seconds(t *testing.T, cell Cell) int64 {
	t.Helper()
	if cell.Sort == "" {
		t.Fatalf("cell %q carries no sort key", cell.Text)
	}
	n, err := strconv.ParseInt(cell.Sort, 10, 64)
	if err != nil {
		t.Fatalf("sort key %q of cell %q is not a number: %v", cell.Sort, cell.Text, err)
	}
	return n
}

func TestOtherKindsStaySortedByNamespaceAndName(t *testing.T) {
	pods := []*unstructured.Unstructured{
		obj(map[string]any{"metadata": map[string]any{"name": "zulu", "namespace": "alpha"}}),
		obj(map[string]any{"metadata": map[string]any{"name": "alpha", "namespace": "zulu"}}),
		obj(map[string]any{"metadata": map[string]any{"name": "mike", "namespace": "alpha"}}),
	}

	table := buildLiveTable(KindPods, withNamespace(builtinColumns[KindPods], true), true, pods)

	var order []string
	for _, row := range table.Rows {
		order = append(order, row.Namespace+"/"+row.Name)
	}
	if want := []string{"alpha/mike", "alpha/zulu", "zulu/alpha"}; !slices.Equal(order, want) {
		t.Errorf("order = %v, want %v", order, want)
	}
}

func TestVolumeSizesSortByValueNotByText(t *testing.T) {
	claim := obj(map[string]any{
		"metadata": map[string]any{"name": "data", "namespace": "prod"},
		"status":   map[string]any{"capacity": map[string]any{"storage": "500Mi"}, "phase": "Bound"},
	})
	bigger := obj(map[string]any{
		"metadata": map[string]any{"name": "bulk", "namespace": "prod"},
		"status":   map[string]any{"capacity": map[string]any{"storage": "5Gi"}, "phase": "Bound"},
	})

	table := buildLiveTable(KindPVCs, withNamespace(builtinColumns[KindPVCs], true), true,
		[]*unstructured.Unstructured{claim, bigger})
	at := slices.Index(table.Columns, "Size")

	byName := map[string]Cell{}
	for _, row := range table.Rows {
		byName[row.Name] = row.Cells[at]
	}
	if byName["data"].Text != "500Mi" || byName["bulk"].Text != "5Gi" {
		t.Fatalf("sizes rendered as %q and %q", byName["data"].Text, byName["bulk"].Text)
	}
	// 500Mi is 524288000 bytes, 5Gi is 5368709120. As text, "500Mi" sorts
	// after "5Gi", which is exactly the order the sort key exists to correct.
	small, big := seconds(t, byName["data"]), seconds(t, byName["bulk"])
	if small >= big {
		t.Errorf("500Mi sorts as %d and 5Gi as %d -- want the smaller first", small, big)
	}
}

func indexOf(t Table, name string) int {
	for i, row := range t.Rows {
		if row.Name == name {
			return i
		}
	}
	return -1
}
