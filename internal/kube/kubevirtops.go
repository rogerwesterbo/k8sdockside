// Doing things to a virtual machine: the lifecycle half of virtctl.
//
// Three different mechanisms, chosen per operation rather than uniformly, and
// the choice is the interesting part of this file.
//
// Start and stop are a patch to the VirtualMachine's own run strategy. virtctl
// uses a subresource for them, but the run strategy is the thing the subresource
// ultimately sets, and patching it needs only ordinary write access to
// virtualmachines -- where the subresources live in a separate API group that
// plenty of read-mostly roles are never granted. Same outcome, fewer clusters
// where the button is refused.
//
// Migrate creates a VirtualMachineInstanceMigration, which is what that object
// is for. It is an ordinary create, it leaves a record somebody can look at
// afterwards, and it is the documented way to move a guest.
//
// Restart, pause, unpause and soft reboot have no equivalent in the object
// model -- there is nothing to write that means "reboot" -- so they go through
// the subresources.kubevirt.io endpoints, exactly as virtctl does.
package kube

import (
	"context"
	json "encoding/json/v2"
	"fmt"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
)

// The lifecycle operations a virtual machine offers, as the frontend names them.
const (
	VMStart      = "start"
	VMStop       = "stop"
	VMRestart    = "restart"
	VMPause      = "pause"
	VMUnpause    = "unpause"
	VMSoftReboot = "softreboot"
	VMMigrate    = "migrate"
)

// The run strategies KubeVirt understands. Halted is stopped; Always is the
// ordinary "keep it running".
const (
	runAlways = "Always"
	runHalted = "Halted"
)

// subresourceGroup is where KubeVirt puts the verbs that are not writes to an
// object. Pinned to v1 rather than discovered: every KubeVirt that has these
// serves them at v1, and a cluster that does not have them at all fails the
// call with a message this app passes straight through.
const subresourceGroup = "/apis/subresources.kubevirt.io/v1"

// VMOperation runs one lifecycle operation against a virtual machine.
//
// `name` is the VirtualMachine's, which is also its instance's -- KubeVirt names
// the instance after the machine that owns it, so the operations that act on the
// running guest need no second lookup to find it.
func (w *Watcher) VMOperation(kc Context, op, namespace, name string) error {
	if namespace == "" || name == "" {
		return fmt.Errorf("a virtual machine is named by a namespace and a name")
	}
	// Checked before a cluster is reached for. An operation this app does not
	// have is a mistake in our own code, and finding it out after a connection
	// and a round trip would report it as if the cluster had refused.
	if !isVMOperation(op) {
		return fmt.Errorf("%q is not a virtual machine operation", op)
	}

	return w.withClient(kc, func(c *clusterClient) error {
		ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
		defer cancel()

		switch op {
		case VMStart:
			return c.setRunStrategy(ctx, namespace, name, runAlways)
		case VMStop:
			return c.setRunStrategy(ctx, namespace, name, runHalted)
		case VMMigrate:
			return c.startMigration(ctx, namespace, name)
		case VMRestart:
			return c.vmSubresource(ctx, "virtualmachines", namespace, name, "restart")
		case VMPause:
			return c.vmSubresource(ctx, "virtualmachineinstances", namespace, name, "pause")
		case VMUnpause:
			return c.vmSubresource(ctx, "virtualmachineinstances", namespace, name, "unpause")
		case VMSoftReboot:
			return c.vmSubresource(ctx, "virtualmachineinstances", namespace, name, "softreboot")
		}
		// Unreachable: isVMOperation above admits exactly the cases named here,
		// and a new one added to that list without a case here would land in a
		// silent success otherwise.
		return fmt.Errorf("%q is not a virtual machine operation", op)
	})
}

// isVMOperation reports whether this app knows the operation.
func isVMOperation(op string) bool {
	switch op {
	case VMStart, VMStop, VMRestart, VMPause, VMUnpause, VMSoftReboot, VMMigrate:
		return true
	}
	return false
}

// setRunStrategy starts or stops a machine by writing what it should be doing.
//
// Which field is written follows what the object already uses. `spec.running`
// is the older boolean and is still what a great many manifests carry; writing
// runStrategy onto one that has it is rejected by KubeVirt, because the two are
// mutually exclusive. So the object is read first and answered in its own terms.
func (c *clusterClient) setRunStrategy(ctx context.Context, namespace, name, strategy string) error {
	mapping, err := c.mappingForKind(KindVirtualMachines)
	if err != nil {
		return err
	}
	client := resourceFor(c.dynamic, mapping, namespace)

	vm, err := client.Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return err
	}

	var patch []byte
	if _, found, _ := unstructured.NestedBool(vm.Object, "spec", "running"); found {
		patch, _ = json.Marshal(map[string]any{"spec": map[string]any{"running": strategy == runAlways}})
	} else {
		patch, _ = json.Marshal(map[string]any{"spec": map[string]any{"runStrategy": strategy}})
	}

	_, err = client.Patch(ctx, name, types.MergePatchType, patch, metav1.PatchOptions{})
	return err
}

// startMigration asks for a live migration by creating the object that
// represents one.
//
// A generated name rather than a chosen one: a guest may be migrated many times
// and each is its own record, so a fixed name would collide with the history
// rather than adding to it. The prefix matches what virtctl uses, so a
// migration started here reads the same in `kubectl get` as one started there.
func (c *clusterClient) startMigration(ctx context.Context, namespace, name string) error {
	mapping, err := c.mappingForKind(KindVirtualMachineMigrations)
	if err != nil {
		return err
	}
	migration := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "kubevirt.io/v1",
		"kind":       "VirtualMachineInstanceMigration",
		"metadata": map[string]any{
			"generateName": "kubevirt-migrate-vm-",
			"namespace":    namespace,
		},
		"spec": map[string]any{"vmiName": name},
	}}
	_, err = resourceFor(c.dynamic, mapping, namespace).Create(ctx, migration, metav1.CreateOptions{})
	return err
}

// vmSubresource calls one of the verbs KubeVirt serves outside the object model.
func (c *clusterClient) vmSubresource(ctx context.Context, resource, namespace, name, verb string) error {
	err := c.typed.CoreV1().RESTClient().Put().
		AbsPath(subresourceGroup, "namespaces", namespace, resource, name, verb).
		Do(ctx).
		Error()
	return kubevirtError(verb, name, err)
}

// kubevirtError explains a refused operation in terms of the thing to fix. The
// wire error names a path in an API group most people have never heard of.
func kubevirtError(verb, name string, err error) error {
	if err == nil {
		return nil
	}
	text := err.Error()
	switch {
	case strings.Contains(text, "the server could not find the requested resource"),
		strings.Contains(text, "not found"):
		return fmt.Errorf("this cluster does not serve %s -- it needs the virt-api subresource endpoints, and %s must be running", verb, name)
	case strings.Contains(text, "orbidden"):
		return fmt.Errorf("not allowed to %s %s -- that needs the subresources.kubevirt.io permission, which is separate from write access to the machine itself", verb, name)
	}
	return fmt.Errorf("%s %s: %w", verb, name, err)
}

// VMState is what a virtual machine's action bar needs to know: which of the
// operations make sense right now.
//
// Read from the objects rather than guessed from the last button pressed. A VM
// started from kubectl a moment ago should offer Stop here without anything
// being clicked first.
type VMState struct {
	// IsMachine reports whether this object has VM operations at all.
	IsMachine bool `json:"isMachine"`
	// Running is whether a guest exists for it right now.
	Running bool `json:"running"`
	// Paused is whether that guest is paused, which is a state of the instance
	// rather than of the machine.
	Paused bool `json:"paused"`
	// Migratable is whether KubeVirt says the guest can be moved. A machine
	// with passed-through hardware cannot, and offering the button anyway
	// produces a migration that fails a moment later.
	Migratable bool `json:"migratable"`
	// Status is the machine's own printable status, for the label.
	Status string `json:"status"`
}

// vmStateOf reads the two objects for what the bar needs.
func vmStateOf(vm, vmi *unstructured.Unstructured) VMState {
	out := VMState{IsMachine: true, Status: nestedString(vm, "status", "printableStatus")}
	if vmi == nil {
		return out
	}
	out.Running = nestedString(vmi, "status", "phase") == "Running"
	if out.Status == "" {
		out.Status = nestedString(vmi, "status", "phase")
	}
	for _, raw := range nestedSlice(vmi, "status", "conditions") {
		cond := asMap(raw)
		switch mapString(cond, "type") {
		case "Paused":
			out.Paused = mapString(cond, "status") == "True"
		case "LiveMigratable":
			out.Migratable = mapString(cond, "status") == "True"
		}
	}
	return out
}

// VMState reads one machine's state for its action bar.
func (w *Watcher) VMState(kc Context, kind, namespace, name string) (VMState, error) {
	var out VMState
	if kind != KindVirtualMachines && kind != KindVirtualMachineInstances {
		return out, nil
	}
	err := w.withClient(kc, func(c *clusterClient) error {
		ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
		defer cancel()

		vm := fetch(ctx, c, KindVirtualMachines, namespace, name)
		vmi := fetch(ctx, c, KindVirtualMachineInstances, namespace, name)
		if vm == nil && vmi == nil {
			return fmt.Errorf("no virtual machine named %q in %s", name, namespace)
		}
		out = vmStateOf(vm, vmi)
		return nil
	})
	return out, err
}
