// Reading one KubeVirt object in full, for a detail panel that means something.
//
// Every other kind in this app describes as YAML, and that is the right default:
// `kubectl describe` has a bespoke printer per built-in kind and nothing at all
// for a CRD, while YAML is complete for every kind alike. A virtual machine is
// where that default is worst. What somebody wants to know about a VM -- which
// node it landed on, how much of its memory the hypervisor charges it, what its
// disks are backed by, which addresses its interfaces got -- is spread across
// three objects (the VirtualMachine, its VirtualMachineInstance, and the
// virt-launcher pod running it) and buried under a hundred lines of spec.
//
// So this file does for KubeVirt what helmdetail.go does for a Helm release:
// reads the objects, pulls out the handful of facts a person is actually
// looking for, and hands back something a panel can lay out. It is the app's
// knowledge of KubeVirt, not the plugin's -- a plugin file is data and cannot
// carry this -- and it draws nothing at all on a cluster without KubeVirt.
package kube

import (
	"context"
	"fmt"
	"sort"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// The KubeVirt kinds this file knows how to read, as the app's kind strings.
var (
	KindVirtualMachines          = CustomKind("virtualmachines", "kubevirt.io")
	KindVirtualMachineInstances  = CustomKind("virtualmachineinstances", "kubevirt.io")
	KindVirtualMachineMigrations = CustomKind("virtualmachineinstancemigrations", "kubevirt.io")
)

// IsKubeVirtDetailKind reports whether this kind has a detail view of its own.
// The frontend asks before it renders one, so a kind that gains one here needs
// no second list over there.
func IsKubeVirtDetailKind(kind string) bool {
	switch kind {
	case KindVirtualMachines, KindVirtualMachineInstances, KindVirtualMachineMigrations:
		return true
	}
	return false
}

// Ref names another object this one leads to, so the panel can offer it as a
// link rather than as text somebody has to go and find.
//
// Kind is the app's own kind string, which is what opening a tab or a describe
// panel takes. An empty Kind means "show the name, do not link it" -- a node
// this app cannot open, or a pod that has since gone.
type Ref struct {
	Kind      string `json:"kind"`
	Namespace string `json:"namespace,omitzero"`
	Name      string `json:"name"`
}

// Fact is one labelled value in the panel's summary block. Ref is set where the
// value names another object.
type Fact struct {
	Label string `json:"label"`
	Value string `json:"value"`
	Ref   *Ref   `json:"ref,omitzero"`
	// Tone colours the value: "ok", "warn", "error", "info". Empty is plain.
	Tone string `json:"tone,omitzero"`
	// Note is the sentence behind a value that needs one -- what memory
	// overhead is, why a guest OS is unknown.
	Note string `json:"note,omitzero"`
}

// Section is one titled block: either a list of facts or a small table.
//
// One shape for both because the panel lays them out the same way and the
// difference is only how many columns a row has. A section with no rows is
// dropped rather than drawn empty -- see keep.
type Section struct {
	Title string `json:"title"`
	Facts []Fact `json:"facts,omitzero"`
	// Columns and Rows make a table. A cell carrying a Ref is a link, exactly
	// as a Fact's is.
	Columns []string `json:"columns,omitzero"`
	Rows    [][]Fact `json:"rows,omitzero"`
	// Empty is what to say instead when there are no rows and the absence is
	// worth stating -- "No migrations for this VM" rather than no section.
	Empty string `json:"empty,omitzero"`
}

// KubeVirtDetail is one KubeVirt object laid out for the panel.
type KubeVirtDetail struct {
	// Kind is the app kind that was read, so the frontend can tell a VM's
	// layout from a migration's without parsing the sections.
	Kind     string    `json:"kind"`
	Sections []Section `json:"sections"`
	// Error is why there is nothing, carried rather than returned so a panel
	// that also draws charts and a report does not lose them to this.
	Error string `json:"error"`
}

// KubeVirtDetail reads one VirtualMachine, VirtualMachineInstance or migration.
func (w *Watcher) KubeVirtDetail(kc Context, kind, namespace, name string) (KubeVirtDetail, error) {
	out := KubeVirtDetail{Kind: kind, Sections: []Section{}}
	if !IsKubeVirtDetailKind(kind) {
		return out, fmt.Errorf("%s has no KubeVirt detail view", kind)
	}

	err := w.withClient(kc, func(c *clusterClient) error {
		ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
		defer cancel()

		obj, _, err := c.get(ctx, kind, namespace, name)
		if err != nil {
			return err
		}
		switch kind {
		case KindVirtualMachineMigrations:
			out.Sections = migrationSections(ctx, c, obj)
		default:
			out.Sections = machineSections(ctx, c, kind, obj)
		}
		return nil
	})
	if err != nil {
		out.Error = err.Error()
	}
	return out, err
}

// ---- the virtual machine ---------------------------------------------------

// machineSections lays out a VirtualMachine or a VirtualMachineInstance.
//
// The two are read together on purpose. A VirtualMachine is the definition and
// carries the disks and the requested hardware; the VirtualMachineInstance is
// the guest actually running and is the only one that knows which node it is
// on, what addresses it got and what the hypervisor is charging it. Somebody
// opening either wants both, and being told to go and find the other object
// first is the thing this panel exists to stop.
func machineSections(ctx context.Context, c *clusterClient, kind string, obj *unstructured.Unstructured) []Section {
	var vm, vmi *unstructured.Unstructured
	if kind == KindVirtualMachines {
		vm = obj
		// The instance carries the VM's own name: KubeVirt names it after the
		// VirtualMachine that owns it. A stopped VM simply has none, which is
		// not an error -- the panel says what it can from the definition.
		vmi = fetch(ctx, c, KindVirtualMachineInstances, obj.GetNamespace(), obj.GetName())
	} else {
		vmi = obj
		vm = fetch(ctx, c, KindVirtualMachines, obj.GetNamespace(), obj.GetName())
	}

	sections := machineSectionsFrom(kind, vm, vmi, launcherPod(ctx, c, vmi))
	// The one block that needs a list rather than a get, so it is added here
	// rather than in the pure layout below.
	return keep(append(sections, migrationsForSection(ctx, c, obj)))
}

// machineSectionsFrom is the layout itself: a pure function of the objects
// somebody else read, so the rules that are easy to get wrong -- which of the
// two carries the node, what a volume is actually backed by -- are testable
// without a cluster. The same split budget.go makes, for the same reason.
func machineSectionsFrom(kind string, vm, vmi, pod *unstructured.Unstructured) []Section {
	return keep([]Section{
		overviewSection(kind, vm, vmi, pod),
		conditionsSection(vmi, vm),
		interfacesSection(vmi),
		disksSection(vm, vmi),
		devicesSection(vm, vmi),
	})
}

// overviewSection is the block at the top: what the machine is, and where it is.
func overviewSection(kind string, vm, vmi *unstructured.Unstructured, pod *unstructured.Unstructured) Section {
	facts := []Fact{}

	if state := printableStatus(vm, vmi); state.Label != "" {
		facts = append(facts, state)
	}
	if cpu := guestCPU(vm, vmi); cpu != "" {
		facts = append(facts, Fact{Label: "CPU", Value: cpu})
	}
	if mem := guestMemory(vm, vmi); mem != "" {
		facts = append(facts, Fact{Label: "Memory", Value: mem})
	}
	// What the hypervisor charges on top of the guest's own memory, for the
	// emulator and its page tables. It is why a VM's virt-launcher pod always
	// asks for more than the VM was given, and it is invisible anywhere else.
	if over := nestedString(vmi, "status", "memory", "guestRequested"); over != "" {
		facts = append(facts, Fact{Label: "Guest requested", Value: over})
	}
	if over := nestedString(vmi, "status", "memory", "guestAtBoot"); over != "" {
		facts = append(facts, Fact{Label: "Memory at boot", Value: over})
	}

	// The node. This is the fact the whole panel is worth opening for: a VM
	// does not reschedule the way a pod does, so which machine it is on is a
	// standing property of it rather than an implementation detail.
	if node := nestedString(vmi, "status", "nodeName"); node != "" {
		facts = append(facts, Fact{
			Label: "Node",
			Value: node,
			Ref:   &Ref{Kind: KindNodes, Name: node},
		})
	}

	if os := guestOS(vmi); os != "" {
		facts = append(facts, Fact{Label: "Guest OS", Value: os})
	} else if vmi != nil {
		facts = append(facts, Fact{
			Label: "Guest OS",
			Value: "Unknown",
			Tone:  "info",
			Note:  "Reported by the QEMU guest agent. Unknown means the agent is not installed in the guest, or has not connected yet.",
		})
	}

	// The two objects underneath: the running instance, and the pod it runs in.
	// Both are ordinary objects this app can open, and neither is findable from
	// the YAML without knowing KubeVirt's naming.
	if vmi != nil && vmi != vm {
		facts = append(facts, Fact{
			Label: "Instance",
			Value: vmi.GetName(),
			Ref:   &Ref{Kind: KindVirtualMachineInstances, Namespace: vmi.GetNamespace(), Name: vmi.GetName()},
		})
	}
	if vm != nil && vm != vmi && kind != KindVirtualMachines {
		facts = append(facts, Fact{
			Label: "Virtual Machine",
			Value: vm.GetName(),
			Ref:   &Ref{Kind: KindVirtualMachines, Namespace: vm.GetNamespace(), Name: vm.GetName()},
		})
	}
	if pod != nil {
		facts = append(facts, Fact{
			Label: "Pod",
			Value: pod.GetName(),
			Ref:   &Ref{Kind: KindPods, Namespace: pod.GetNamespace(), Name: pod.GetName()},
		})
	}
	return Section{Title: "Virtual machine", Facts: facts}
}

// conditionsSection is the standard Kubernetes conditions block, which for a VM
// carries readings nothing else does: LiveMigratable says whether the guest can
// be moved at all, and AgentConnected whether anything inside it is answering.
func conditionsSection(objs ...*unstructured.Unstructured) Section {
	rows := [][]Fact{}
	for _, obj := range objs {
		for _, raw := range nestedSlice(obj, "status", "conditions") {
			cond := asMap(raw)
			kind := mapString(cond, "type")
			if kind == "" {
				continue
			}
			state := mapString(cond, "status")
			rows = append(rows, []Fact{
				{Value: kind},
				{Value: state, Tone: conditionTone(kind, state)},
				{Value: mapString(cond, "reason")},
				{Value: mapString(cond, "message")},
			})
		}
		// One object's conditions are enough. A stopped VM has the
		// VirtualMachine's; a running one has the instance's, which are the
		// more specific of the two.
		if len(rows) > 0 {
			break
		}
	}
	return Section{Title: "Conditions", Columns: []string{"Condition", "Status", "Reason", "Message"}, Rows: rows}
}

// conditionTone colours a condition by what True means for it, which is not the
// same for all of them: Ready True is good and Paused True is not.
func conditionTone(kind, state string) string {
	bad := kind == "Paused" || strings.HasPrefix(kind, "Unschedulable") || strings.Contains(kind, "Failure")
	switch state {
	case "True":
		if bad {
			return "warn"
		}
		return "ok"
	case "False":
		if bad {
			return "ok"
		}
		return "warn"
	}
	return "info"
}

// interfacesSection is what the guest's network actually came out as: the
// addresses it holds, on which of the cluster's networks, with the MACs.
//
// The addresses are the point. A VM's IP is assigned inside the guest and
// reported back by the agent, so it appears nowhere in the spec -- the only
// place to read it is here.
func interfacesSection(vmi *unstructured.Unstructured) Section {
	rows := [][]Fact{}
	for _, raw := range nestedSlice(vmi, "status", "interfaces") {
		iface := asMap(raw)
		name := mapString(iface, "name")
		addresses := mapString(iface, "ipAddress")
		if list := stringsIn(iface["ipAddresses"]); len(list) > 0 {
			addresses = strings.Join(list, ", ")
		}
		net := Fact{Value: mapString(iface, "interfaceName")}
		rows = append(rows, []Fact{
			{Value: name},
			net,
			{Value: mapString(iface, "mac")},
			{Value: addresses},
		})
	}
	return Section{
		Title:   "Network interfaces",
		Columns: []string{"Name", "Interface", "MAC", "Addresses"},
		Rows:    rows,
		Empty:   "This guest reports no interfaces. A running VM with none usually means the guest agent is not installed.",
	}
}

// disksSection pairs each disk the VM declares with the volume behind it, which
// is the join somebody actually wants: a disk on its own says "vda, virtio" and
// a volume on its own says "this PVC", and neither answers "what is vda".
func disksSection(vm, vmi *unstructured.Unstructured) Section {
	volumes := map[string]map[string]any{}
	for _, obj := range []*unstructured.Unstructured{vm, vmi} {
		for _, raw := range nestedSlice(obj, "spec", "template", "spec", "volumes") {
			volumes[mapString(asMap(raw), "name")] = asMap(raw)
		}
		for _, raw := range nestedSlice(obj, "spec", "volumes") {
			volumes[mapString(asMap(raw), "name")] = asMap(raw)
		}
	}

	rows := [][]Fact{}
	for _, obj := range []*unstructured.Unstructured{vm, vmi} {
		disks := nestedSlice(obj, "spec", "template", "spec", "domain", "devices", "disks")
		if len(disks) == 0 {
			disks = nestedSlice(obj, "spec", "domain", "devices", "disks")
		}
		for _, raw := range disks {
			disk := asMap(raw)
			name := mapString(disk, "name")
			backing, ref := volumeBacking(volumes[name], namespaceOf(vm, vmi))
			rows = append(rows, []Fact{
				{Value: name},
				{Value: diskTarget(disk)},
				{Value: backing, Ref: ref},
			})
		}
		if len(rows) > 0 {
			break
		}
	}
	return Section{
		Title:   "Disks and volumes",
		Columns: []string{"Disk", "Target", "Backed by"},
		Rows:    rows,
		Empty:   "This machine declares no disks.",
	}
}

// devicesSection is the hardware passed through from the node: GPUs and host
// devices. Almost always empty, and worth its own block when it is not --
// passed-through hardware is why a VM cannot be live migrated.
func devicesSection(vm, vmi *unstructured.Unstructured) Section {
	rows := [][]Fact{}
	for _, obj := range []*unstructured.Unstructured{vmi, vm} {
		for _, path := range [][]string{
			{"spec", "template", "spec", "domain", "devices"},
			{"spec", "domain", "devices"},
		} {
			for _, group := range []string{"gpus", "hostDevices"} {
				for _, raw := range nestedSlice(obj, append(append([]string{}, path...), group)...) {
					dev := asMap(raw)
					rows = append(rows, []Fact{
						{Value: mapString(dev, "name")},
						{Value: strings.TrimSuffix(group, "s")},
						{Value: mapString(dev, "deviceName")},
					})
				}
			}
		}
		if len(rows) > 0 {
			break
		}
	}
	if len(rows) == 0 {
		// Not worth a block saying "none" on the overwhelming majority of VMs.
		return Section{}
	}
	return Section{Title: "GPUs and host devices", Columns: []string{"Name", "Kind", "Device"}, Rows: rows}
}

// ---- migrations ------------------------------------------------------------

// migrationSections lays out one VirtualMachineInstanceMigration.
//
// The nodes are the reason this exists. A migration is entirely about moving a
// guest from one machine to another, and neither machine appears in the
// object's printer columns -- they are in status.migrationState, which is on
// the *instance* rather than on the migration. So this reads both.
func migrationSections(ctx context.Context, c *clusterClient, obj *unstructured.Unstructured) []Section {
	vmi := fetch(ctx, c, KindVirtualMachineInstances, obj.GetNamespace(), nestedString(obj, "spec", "vmiName"))
	return migrationSectionsFrom(obj, vmi)
}

// migrationSectionsFrom is the layout, pure over the two objects it needs.
func migrationSectionsFrom(obj, vmi *unstructured.Unstructured) []Section {
	namespace := obj.GetNamespace()
	vmiName := nestedString(obj, "spec", "vmiName")

	// The migration's own status carries the phase; the guest's carries where
	// it went. A finished migration whose guest has since been deleted keeps
	// the phase and loses the nodes, which is honest rather than blank.
	state := asMap(nestedMap(vmi, "status", "migrationState"))
	if mapString(state, "migrationUid") != string(obj.GetUID()) {
		// The instance has moved on to a later migration, so its state is not
		// about this one. Better to say nothing than to name the wrong nodes.
		state = nil
	}

	phase := nestedString(obj, "status", "phase")
	info := Section{Title: "Migration", Facts: []Fact{
		{Label: "Status", Value: phase, Tone: toneFor(phase)},
	}}
	if vmiName != "" {
		info.Facts = append(info.Facts, Fact{
			Label: "Virtual machine",
			Value: vmiName,
			Ref:   &Ref{Kind: KindVirtualMachineInstances, Namespace: namespace, Name: vmiName},
		})
	}
	if mode := mapString(state, "mode"); mode != "" {
		info.Facts = append(info.Facts, Fact{
			Label: "Mode",
			Value: mode,
			Note:  "PreCopy copies memory while the guest runs and pauses it at the end; PostCopy pauses first and pages memory in on demand. PostCopy finishes a migration PreCopy cannot, at the cost of a guest that is running on a remote memory map until it does.",
		})
	}
	if started := mapString(state, "startTimestamp"); started != "" {
		info.Facts = append(info.Facts, Fact{Label: "Started", Value: started})
	}
	if ended := mapString(state, "endTimestamp"); ended != "" {
		info.Facts = append(info.Facts, Fact{Label: "Completed", Value: ended})
	}
	if failed, ok := state["failed"].(bool); ok && failed {
		info.Facts = append(info.Facts, Fact{
			Label: "Failure",
			Value: mapString(state, "failureReason"),
			Tone:  "error",
		})
	}

	nodes := Section{Title: "Nodes", Facts: []Fact{}}
	for _, pair := range []struct{ label, key string }{
		{"Source node", "sourceNode"},
		{"Target node", "targetNode"},
	} {
		if name := mapString(state, pair.key); name != "" {
			nodes.Facts = append(nodes.Facts, Fact{
				Label: pair.label,
				Value: name,
				Ref:   &Ref{Kind: KindNodes, Name: name},
			})
		}
	}
	for _, pair := range []struct{ label, key string }{
		{"Source pod", "sourcePod"},
		{"Target pod", "targetPod"},
	} {
		if name := mapString(state, pair.key); name != "" {
			nodes.Facts = append(nodes.Facts, Fact{
				Label: pair.label,
				Value: name,
				Ref:   &Ref{Kind: KindPods, Namespace: namespace, Name: name},
			})
		}
	}
	if addr := mapString(state, "targetNodeAddress"); addr != "" {
		nodes.Facts = append(nodes.Facts, Fact{Label: "Target address", Value: addr})
	}

	return keep([]Section{info, nodes})
}

// migrationsForSection lists the migrations of one machine, newest first, so a
// VM that has been moved says where it has been.
func migrationsForSection(ctx context.Context, c *clusterClient, obj *unstructured.Unstructured) Section {
	items, err := c.listIn(ctx, KindVirtualMachineMigrations, obj.GetNamespace(), metav1.ListOptions{})
	if err != nil {
		// A cluster without the migration CRD, or without permission on it, is
		// an ordinary cluster. It costs this block and nothing else.
		return Section{}
	}

	mine := make([]unstructured.Unstructured, 0, len(items))
	for i := range items {
		if nestedString(&items[i], "spec", "vmiName") == obj.GetName() {
			mine = append(mine, items[i])
		}
	}
	sort.SliceStable(mine, func(i, j int) bool {
		older, newer := mine[j].GetCreationTimestamp(), mine[i].GetCreationTimestamp()
		return older.Before(&newer)
	})

	rows := [][]Fact{}
	for i := range mine {
		m := &mine[i]
		phase := nestedString(m, "status", "phase")
		rows = append(rows, []Fact{
			{
				Value: m.GetName(),
				Ref:   &Ref{Kind: KindVirtualMachineMigrations, Namespace: m.GetNamespace(), Name: m.GetName()},
			},
			{Value: phase, Tone: toneFor(phase)},
			{Value: ageOf(m) + " ago"},
		})
	}
	return Section{
		Title:   "Migrations",
		Columns: []string{"Migration", "Status", "Started"},
		Rows:    rows,
		Empty:   "This machine has not been migrated.",
	}
}

// ---- helpers ---------------------------------------------------------------

// fetch reads one object, returning nil rather than an error for anything that
// is not there. Every caller here is filling in a panel: a stopped VM has no
// instance, a cluster may deny a kind, and neither is worth losing the rest of
// the panel over.
func fetch(ctx context.Context, c *clusterClient, kind, namespace, name string) *unstructured.Unstructured {
	if name == "" {
		return nil
	}
	obj, _, err := c.get(ctx, kind, namespace, name)
	if err != nil {
		return nil
	}
	return obj
}

// launcherPod finds the virt-launcher pod running a guest.
//
// By KubeVirt's own label rather than by the name, which carries a random
// suffix. A guest that is migrating has two for a moment; the one on the node
// the instance says it is on is the one running it.
func launcherPod(ctx context.Context, c *clusterClient, vmi *unstructured.Unstructured) *unstructured.Unstructured {
	if vmi == nil {
		return nil
	}
	pods, err := c.listIn(ctx, KindPods, vmi.GetNamespace(), metav1.ListOptions{
		LabelSelector: "kubevirt.io/domain=" + vmi.GetName(),
	})
	if err != nil || len(pods) == 0 {
		// Older KubeVirt labels the launcher with the VMI's name instead.
		pods, err = c.listIn(ctx, KindPods, vmi.GetNamespace(), metav1.ListOptions{
			LabelSelector: "vm.kubevirt.io/name=" + vmi.GetName(),
		})
		if err != nil || len(pods) == 0 {
			return nil
		}
	}
	node := nestedString(vmi, "status", "nodeName")
	for i := range pods {
		if node == "" || nestedString(&pods[i], "spec", "nodeName") == node {
			return &pods[i]
		}
	}
	return &pods[0]
}

// printableStatus is the state KubeVirt itself writes on a VirtualMachine --
// Running, Stopped, Migrating, ErrorUnschedulable and the rest -- falling back
// to the instance's phase for a VMI opened on its own.
func printableStatus(vm, vmi *unstructured.Unstructured) Fact {
	if state := nestedString(vm, "status", "printableStatus"); state != "" {
		return Fact{Label: "Status", Value: state, Tone: toneFor(state)}
	}
	if phase := nestedString(vmi, "status", "phase"); phase != "" {
		return Fact{Label: "Status", Value: phase, Tone: toneFor(phase)}
	}
	return Fact{}
}

// guestCPU writes what the guest was given, from whichever of the three ways
// KubeVirt lets it be said. An instance type puts it somewhere else again, in
// which case the instance's own resolved spec is the one that knows.
func guestCPU(vm, vmi *unstructured.Unstructured) string {
	for _, obj := range []*unstructured.Unstructured{vmi, vm} {
		for _, path := range [][]string{
			{"spec", "domain", "cpu"},
			{"spec", "template", "spec", "domain", "cpu"},
		} {
			cpu := asMap(nestedMap(obj, path...))
			cores, sockets, threads := mapInt(cpu, "cores"), mapInt(cpu, "sockets"), mapInt(cpu, "threads")
			if cores+sockets+threads == 0 {
				continue
			}
			parts := []string{}
			for _, p := range []struct {
				n     int64
				label string
			}{{cores, "core"}, {sockets, "socket"}, {threads, "thread"}} {
				if p.n > 0 {
					parts = append(parts, plural(p.n, p.label))
				}
			}
			return strings.Join(parts, ", ")
		}
		// No cpu block at all: a VM sized by resource requests like any pod.
		for _, path := range [][]string{
			{"spec", "domain", "resources", "requests"},
			{"spec", "template", "spec", "domain", "resources", "requests"},
		} {
			if v := mapString(asMap(nestedMap(obj, path...)), "cpu"); v != "" {
				return v
			}
		}
	}
	return ""
}

// guestMemory writes the guest's memory, preferring what it was actually given
// over what was asked for.
func guestMemory(vm, vmi *unstructured.Unstructured) string {
	for _, obj := range []*unstructured.Unstructured{vmi, vm} {
		for _, path := range [][]string{
			{"spec", "domain", "memory"},
			{"spec", "template", "spec", "domain", "memory"},
		} {
			mem := asMap(nestedMap(obj, path...))
			if v := mapString(mem, "guest"); v != "" {
				return v
			}
		}
		for _, path := range [][]string{
			{"spec", "domain", "resources", "requests"},
			{"spec", "template", "spec", "domain", "resources", "requests"},
		} {
			if v := mapString(asMap(nestedMap(obj, path...)), "memory"); v != "" {
				return v
			}
		}
	}
	return ""
}

// guestOS is what the agent inside the guest reports, which is the only place
// the operating system is knowable from: the spec says what disk it booted, not
// what is on it.
func guestOS(vmi *unstructured.Unstructured) string {
	os := asMap(nestedMap(vmi, "status", "guestOSInfo"))
	if name := mapString(os, "prettyName"); name != "" {
		return name
	}
	name, version := mapString(os, "name"), mapString(os, "version")
	return strings.TrimSpace(name + " " + version)
}

// diskTarget writes a disk's bus as the guest sees it -- "vda (virtio)".
func diskTarget(disk map[string]any) string {
	for _, kind := range []string{"disk", "cdrom", "lun", "floppy"} {
		if bus := mapString(asMap(disk[kind]), "bus"); bus != "" {
			return bus
		}
	}
	return ""
}

// volumeBacking says what a volume actually is, and links it where the thing it
// names is an object in the cluster.
func volumeBacking(volume map[string]any, namespace string) (string, *Ref) {
	if volume == nil {
		return "", nil
	}
	// The three that name another object are worth linking; the rest are worth
	// naming so a reader knows why there is nothing to click.
	for _, source := range []struct{ key, kind, field string }{
		{"persistentVolumeClaim", KindPVCs, "claimName"},
		{"dataVolume", CustomKind("datavolumes", "cdi.kubevirt.io"), "name"},
		{"configMap", KindConfigMaps, "name"},
		{"secret", KindSecrets, "secretName"},
	} {
		if inner := asMap(volume[source.key]); inner != nil {
			name := mapString(inner, source.field)
			if name == "" {
				name = mapString(inner, "name")
			}
			return name, &Ref{Kind: source.kind, Namespace: namespace, Name: name}
		}
	}
	for _, key := range []string{"containerDisk", "cloudInitNoCloud", "cloudInitConfigDrive", "emptyDisk", "hostDisk", "ephemeral", "memoryDump", "downwardAPI", "serviceAccount", "sysprep"} {
		if inner := asMap(volume[key]); inner != nil {
			if image := mapString(inner, "image"); image != "" {
				return image, nil
			}
			return key, nil
		}
	}
	return "", nil
}

func namespaceOf(objs ...*unstructured.Unstructured) string {
	for _, obj := range objs {
		if obj != nil && obj.GetNamespace() != "" {
			return obj.GetNamespace()
		}
	}
	return ""
}

// stringsIn reads a list of strings out of an unstructured field.
func stringsIn(value any) []string {
	items, ok := value.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(items))
	for _, raw := range items {
		if s, ok := raw.(string); ok && s != "" {
			out = append(out, s)
		}
	}
	return out
}

func plural(n int64, label string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, label)
	}
	return fmt.Sprintf("%d %ss", n, label)
}

// keep drops the sections with nothing in them. A block that says only that
// there is nothing is worth drawing where the absence is a fact about the
// machine -- no migrations, no interfaces -- and not otherwise, which is what
// Empty distinguishes.
func keep(sections []Section) []Section {
	out := make([]Section, 0, len(sections))
	for _, s := range sections {
		if s.Title == "" {
			continue
		}
		if len(s.Facts) == 0 && len(s.Rows) == 0 && s.Empty == "" {
			continue
		}
		out = append(out, s)
	}
	return out
}
