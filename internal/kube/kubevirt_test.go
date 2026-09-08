package kube

import (
	"errors"
	"os"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// sectionNamed picks one block out of a layout, failing if it is missing.
func sectionNamed(t *testing.T, sections []Section, title string) Section {
	t.Helper()
	for _, s := range sections {
		if s.Title == title {
			return s
		}
	}
	t.Fatalf("no %q section; got %v", title, titlesOf(sections))
	return Section{}
}

func titlesOf(sections []Section) []string {
	out := make([]string, 0, len(sections))
	for _, s := range sections {
		out = append(out, s.Title)
	}
	return out
}

// factNamed picks one labelled value out of a block.
func factNamed(t *testing.T, s Section, label string) Fact {
	t.Helper()
	for _, f := range s.Facts {
		if f.Label == label {
			return f
		}
	}
	t.Fatalf("no %q fact in %q; got %+v", label, s.Title, s.Facts)
	return Fact{}
}

func vmObj(name string, spec, status map[string]any) *unstructured.Unstructured {
	return obj(map[string]any{
		"metadata": map[string]any{"name": name, "namespace": "vms"},
		"spec":     spec,
		"status":   status,
	})
}

// The node is the fact the whole panel is worth opening for. A VM does not
// reschedule the way a pod does, so which machine it is on is a standing
// property of it -- and it is on the instance, not on the VirtualMachine.
func TestAVirtualMachineNamesItsNodeAndOffersIt(t *testing.T) {
	vmi := vmObj("win11", nil, map[string]any{"nodeName": "kvw2", "phase": "Running"})

	section := sectionNamed(t, machineSectionsFrom(KindVirtualMachineInstances, nil, vmi, nil), "Virtual machine")
	node := factNamed(t, section, "Node")

	if node.Value != "kvw2" {
		t.Errorf("node = %q, want kvw2", node.Value)
	}
	if node.Ref == nil || node.Ref.Kind != KindNodes || node.Ref.Name != "kvw2" {
		t.Errorf("node ref = %+v, want a link to the node itself", node.Ref)
	}
	// A node is cluster-scoped: a namespace on the link would open nothing.
	if node.Ref != nil && node.Ref.Namespace != "" {
		t.Errorf("node ref carries namespace %q; nodes are cluster-scoped", node.Ref.Namespace)
	}
}

// A stopped VirtualMachine has no instance at all. That is an ordinary state,
// not a failure, and the panel says what the definition can still tell it.
func TestAStoppedVirtualMachineStillLaysOut(t *testing.T) {
	vm := vmObj("win11", map[string]any{
		"template": map[string]any{"spec": map[string]any{
			"domain": map[string]any{"cpu": map[string]any{"cores": int64(4)}},
		}},
	}, map[string]any{"printableStatus": "Stopped"})

	sections := machineSectionsFrom(KindVirtualMachines, vm, nil, nil)
	section := sectionNamed(t, sections, "Virtual machine")

	if got := factNamed(t, section, "Status"); got.Value != "Stopped" {
		t.Errorf("status = %q, want Stopped", got.Value)
	}
	if got := factNamed(t, section, "CPU"); got.Value != "4 cores" {
		t.Errorf("cpu = %q, want 4 cores", got.Value)
	}
	for _, f := range section.Facts {
		if f.Label == "Node" {
			t.Error("a stopped machine is on no node, so it should not claim one")
		}
	}
}

// The guest's addresses are assigned inside it and reported back by the agent,
// so they appear nowhere in the spec: this block is the only place to read them.
func TestTheInterfacesBlockCarriesTheAddressesTheGuestReports(t *testing.T) {
	vmi := vmObj("win11", nil, map[string]any{
		"interfaces": []any{map[string]any{
			"name":          "vlan0",
			"interfaceName": "eth0",
			"mac":           "02:12:d9:39:d7:19",
			"ipAddresses":   []any{"192.168.3.234", "192.168.2.106"},
		}},
	})

	section := sectionNamed(t, machineSectionsFrom(KindVirtualMachineInstances, nil, vmi, nil), "Network interfaces")

	if len(section.Rows) != 1 {
		t.Fatalf("rows = %d, want one interface", len(section.Rows))
	}
	if got := section.Rows[0][3].Value; got != "192.168.3.234, 192.168.2.106" {
		t.Errorf("addresses = %q, want both of them", got)
	}
}

// A disk on its own says "vda, virtio" and a volume on its own says "this PVC".
// Neither answers "what is vda", which is the join this block exists to make.
func TestDisksArePairedWithTheVolumesBehindThem(t *testing.T) {
	vm := vmObj("win11", map[string]any{
		"template": map[string]any{"spec": map[string]any{
			"domain": map[string]any{"devices": map[string]any{"disks": []any{
				map[string]any{"name": "disk-0", "disk": map[string]any{"bus": "virtio"}},
				map[string]any{"name": "cloudinitdisk", "disk": map[string]any{"bus": "virtio"}},
			}}},
			"volumes": []any{
				map[string]any{"name": "disk-0", "persistentVolumeClaim": map[string]any{"claimName": "win11-root"}},
				map[string]any{"name": "cloudinitdisk", "cloudInitNoCloud": map[string]any{}},
			},
		}},
	}, nil)

	section := sectionNamed(t, machineSectionsFrom(KindVirtualMachines, vm, nil, nil), "Disks and volumes")

	if len(section.Rows) != 2 {
		t.Fatalf("rows = %d, want both disks", len(section.Rows))
	}
	backing := section.Rows[0][2]
	if backing.Value != "win11-root" {
		t.Errorf("disk-0 is backed by %q, want the claim name", backing.Value)
	}
	if backing.Ref == nil || backing.Ref.Kind != KindPVCs || backing.Ref.Namespace != "vms" {
		t.Errorf("claim ref = %+v, want a link to the PVC in the VM's namespace", backing.Ref)
	}
	// A cloud-init disk names no object, so there is nothing to click -- and
	// saying what it is beats leaving the cell blank.
	if got := section.Rows[1][2]; got.Ref != nil || got.Value == "" {
		t.Errorf("cloud-init backing = %+v, want a name and no link", got)
	}
}

// A migration is entirely about moving a guest between two machines, and
// neither appears in the object's own printer columns.
func TestAMigrationNamesBothNodesAndOffersThem(t *testing.T) {
	migration := obj(map[string]any{
		"metadata": map[string]any{"name": "kubevirt-migrate-vm-94stk", "namespace": "vms", "uid": "abc-123"},
		"spec":     map[string]any{"vmiName": "win11"},
		"status":   map[string]any{"phase": "Succeeded"},
	})
	vmi := vmObj("win11", nil, map[string]any{"migrationState": map[string]any{
		"migrationUid":      "abc-123",
		"sourceNode":        "wrk13",
		"targetNode":        "wrk16",
		"sourcePod":         "virt-launcher-win11-8xpjj",
		"targetPod":         "virt-launcher-win11-kptzz",
		"targetNodeAddress": "10.244.4.44",
		"mode":              "PreCopy",
	}})

	sections := migrationSectionsFrom(migration, vmi)
	nodes := sectionNamed(t, sections, "Nodes")

	for _, want := range []struct{ label, name string }{
		{"Source node", "wrk13"},
		{"Target node", "wrk16"},
	} {
		got := factNamed(t, nodes, want.label)
		if got.Value != want.name {
			t.Errorf("%s = %q, want %q", want.label, got.Value, want.name)
		}
		if got.Ref == nil || got.Ref.Kind != KindNodes {
			t.Errorf("%s ref = %+v, want a link to the node", want.label, got.Ref)
		}
	}
	if got := factNamed(t, nodes, "Target pod"); got.Ref == nil || got.Ref.Kind != KindPods {
		t.Errorf("target pod ref = %+v, want a link to the pod", got.Ref)
	}
	if got := factNamed(t, sectionNamed(t, sections, "Migration"), "Status"); got.Tone != "ok" {
		t.Errorf("a succeeded migration reads %q, want the good tone", got.Tone)
	}
}

// The instance keeps only its latest migration's state. Naming the nodes of a
// different migration would be worse than naming none.
func TestAMigrationTheGuestHasMovedPastNamesNoNodes(t *testing.T) {
	migration := obj(map[string]any{
		"metadata": map[string]any{"name": "old", "namespace": "vms", "uid": "older"},
		"spec":     map[string]any{"vmiName": "win11"},
		"status":   map[string]any{"phase": "Succeeded"},
	})
	vmi := vmObj("win11", nil, map[string]any{"migrationState": map[string]any{
		"migrationUid": "newer",
		"sourceNode":   "wrk13",
		"targetNode":   "wrk16",
	}})

	for _, s := range migrationSectionsFrom(migration, vmi) {
		if s.Title == "Nodes" {
			t.Errorf("named the nodes of a later migration: %+v", s.Facts)
		}
	}
}

// IsKubeVirtDetailKind is what the frontend checks before it renders a panel,
// and the two lists have to agree.
func TestOnlyTheThreeKubeVirtKindsHaveADetailView(t *testing.T) {
	for _, kind := range []string{KindVirtualMachines, KindVirtualMachineInstances, KindVirtualMachineMigrations} {
		if !IsKubeVirtDetailKind(kind) {
			t.Errorf("%s should have a detail view", kind)
		}
	}
	for _, kind := range []string{KindPods, KindNodes, CustomKind("datavolumes", "cdi.kubevirt.io")} {
		if IsKubeVirtDetailKind(kind) {
			t.Errorf("%s should describe as YAML like everything else", kind)
		}
	}
}

// The frontend decides whether to render this panel before it fetches anything,
// so it carries the same list -- DetailPanel.svelte's KUBEVIRT_DETAIL. A kind
// gaining a panel here without gaining one there draws nothing and reports no
// error, which is the failure nobody notices.
func TestTheDetailKindsAreTheOnesTheFrontendAsksFor(t *testing.T) {
	panel, err := os.ReadFile("../../frontend/src/lib/components/DetailPanel.svelte")
	if err != nil {
		t.Skipf("frontend not present: %v", err)
	}
	for _, kind := range []string{KindVirtualMachines, KindVirtualMachineInstances, KindVirtualMachineMigrations} {
		if !strings.Contains(string(panel), "'"+kind+"'") {
			t.Errorf("DetailPanel.svelte does not list %s, so its panel would never be drawn", kind)
		}
	}
}

// ----- lifecycle operations -------------------------------------------------

// Which of the buttons the bar draws is decided from the cluster's own answer
// rather than from the last one pressed: a machine started from kubectl a
// moment ago should offer Stop here without anything being clicked first.
func TestAStoppedMachineIsNotRunningPausedOrMigratable(t *testing.T) {
	vm := vmObj("win11", nil, map[string]any{"printableStatus": "Stopped"})

	state := vmStateOf(vm, nil)

	if !state.IsMachine || state.Running || state.Paused || state.Migratable {
		t.Errorf("state = %+v, want a machine that is doing none of those", state)
	}
	if state.Status != "Stopped" {
		t.Errorf("status = %q, want Stopped", state.Status)
	}
}

func TestARunningMachineReadsItsConditionsForWhatItCanDo(t *testing.T) {
	vm := vmObj("win11", nil, map[string]any{"printableStatus": "Running"})
	vmi := vmObj("win11", nil, map[string]any{
		"phase": "Running",
		"conditions": []any{
			map[string]any{"type": "LiveMigratable", "status": "True"},
			map[string]any{"type": "Paused", "status": "False"},
		},
	})

	state := vmStateOf(vm, vmi)

	if !state.Running || !state.Migratable || state.Paused {
		t.Errorf("state = %+v, want running and migratable but not paused", state)
	}
}

// A machine with passed-through hardware cannot be moved, and offering the
// button anyway produces a migration that fails a moment later.
func TestAMachineThatCannotMoveIsNotMigratable(t *testing.T) {
	vmi := vmObj("gpu-box", nil, map[string]any{
		"phase":      "Running",
		"conditions": []any{map[string]any{"type": "LiveMigratable", "status": "False"}},
	})

	if state := vmStateOf(nil, vmi); state.Migratable {
		t.Error("offered a migration for a guest KubeVirt says cannot be moved")
	}
}

func TestAPausedGuestIsReportedAsPaused(t *testing.T) {
	vmi := vmObj("win11", nil, map[string]any{
		"phase":      "Running",
		"conditions": []any{map[string]any{"type": "Paused", "status": "True"}},
	})

	if state := vmStateOf(nil, vmi); !state.Paused {
		t.Error("a paused guest should offer Resume rather than Pause")
	}
}

// Only the two machine kinds. A DataVolume is not something to start and stop.
func TestVMStateIsNotAskedOfEveryKind(t *testing.T) {
	w := &Watcher{}
	for _, kind := range []string{KindPods, KindNodes, CustomKind("datavolumes", "cdi.kubevirt.io")} {
		state, err := w.VMState(Context{}, kind, "ns", "name")
		if err != nil {
			t.Errorf("%s: unexpected error %v", kind, err)
		}
		if state.IsMachine {
			t.Errorf("%s should not be treated as a virtual machine", kind)
		}
	}
}

func TestAnUnknownOperationIsRefusedRatherThanGuessed(t *testing.T) {
	w := &Watcher{}
	if err := w.VMOperation(Context{}, "destroy", "vms", "win11"); err == nil {
		t.Error("an operation this app does not have should be refused")
	}
}

func TestAnOperationNeedsBothANamespaceAndAName(t *testing.T) {
	w := &Watcher{}
	for _, args := range [][2]string{{"", "win11"}, {"vms", ""}} {
		if err := w.VMOperation(Context{}, VMStart, args[0], args[1]); err == nil {
			t.Errorf("VMOperation(%q, %q) should be refused", args[0], args[1])
		}
	}
}

// The wire error names a path inside an API group most people have never heard
// of. What the reader needs is which permission to ask for.
func TestARefusedSubresourceSaysWhichPermissionItNeeds(t *testing.T) {
	err := kubevirtError("pause", "win11", errors.New(`virtualmachineinstances.subresources.kubevirt.io "win11" is forbidden`))

	if err == nil || !strings.Contains(err.Error(), "subresources.kubevirt.io permission") {
		t.Errorf("error = %v, want it to name the permission to ask for", err)
	}
}

func TestAClusterWithoutTheSubresourceEndpointsSaysSo(t *testing.T) {
	err := kubevirtError("softreboot", "win11", errors.New("the server could not find the requested resource"))

	if err == nil || !strings.Contains(err.Error(), "virt-api") {
		t.Errorf("error = %v, want it to name what is missing", err)
	}
}

func TestASuccessfulCallReportsNoError(t *testing.T) {
	if err := kubevirtError("start", "win11", nil); err != nil {
		t.Errorf("error = %v, want none", err)
	}
}
