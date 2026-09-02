package kube

import (
	"fmt"
	"strings"
)

// BuildOverview is the dashboard payload for a context: how much of the cluster
// is healthy, how loaded it is, and what has gone wrong recently.
func BuildOverview(ctx Context) Overview {
	m := buildModel(ctx)

	nodesReady := 0
	var cpuCap, cpuUsed, memCap, memUsed float64
	for _, n := range m.nodes {
		if n.Ready {
			nodesReady++
		}
		cpuCap += float64(n.CPU)
		cpuUsed += float64(n.CPU) * n.CPUUsed
		memCap += float64(n.MemGiB)
		memUsed += float64(n.MemGiB) * n.MemUsed
	}

	podsRunning := 0
	for _, p := range m.pods {
		if p.Phase == "Running" {
			podsRunning++
		}
	}

	deployTotal, deployReady, services := 0, 0, 0
	for _, w := range m.workloads {
		switch w.Kind {
		case "Deployment":
			deployTotal++
			if w.Ready == w.Desired {
				deployReady++
			}
			services++
		case "StatefulSet":
			services++
		}
	}

	events := m.events()
	if len(events) > 8 {
		events = events[:8]
	}

	return Overview{
		ContextID:    ctx.ID,
		Context:      ctx.Name,
		Cluster:      ctx.Cluster,
		Server:       ctx.Server,
		Version:      m.version,
		Distribution: m.distribution,
		Namespaces:   m.namespaces,
		Stats: []Stat{
			{Label: "Nodes", Ready: nodesReady, Total: len(m.nodes)},
			{Label: "Pods", Ready: podsRunning, Total: len(m.pods)},
			{Label: "Deployments", Ready: deployReady, Total: deployTotal},
			{Label: "Namespaces", Ready: len(m.namespaces), Total: len(m.namespaces)},
		},
		Gauges: []Gauge{
			{Label: "CPU", Used: round1(cpuUsed), Capacity: cpuCap, Unit: "cores"},
			{Label: "Memory", Used: round1(memUsed), Capacity: memCap, Unit: "GiB"},
			{Label: "Pods", Used: float64(len(m.pods)), Capacity: float64(len(m.nodes) * 110), Unit: ""},
		},
		Events: events,
	}
}

func round1(v float64) float64 { return float64(int(v*10+0.5)) / 10 }

// BuildDescribe renders a `kubectl describe`-style report for one object. The
// shape is deliberately close to the real command's output, so swapping in live
// data later does not change how the slide-in panel looks.
func BuildDescribe(ctx Context, kind, namespace, name string) string {
	m := buildModel(ctx)

	switch kind {
	case KindPods:
		for _, p := range m.pods {
			if p.Name == name && p.Namespace == namespace {
				return describePod(m, p)
			}
		}
	case KindNodes:
		for _, n := range m.nodes {
			if n.Name == name {
				return describeNode(m, n)
			}
		}
	case KindDeployments, KindStatefulSet, KindDaemonSets, KindJobs, KindCronJobs:
		for _, w := range m.workloads {
			if w.Name == name && w.Namespace == namespace {
				return describeWorkload(m, w)
			}
		}
	}
	return describeGeneric(m, kind, namespace, name)
}

type describer struct{ b strings.Builder }

func (d *describer) field(k, v string) {
	d.b.WriteString(fmt.Sprintf("%-22s%s\n", k+":", v))
}

func (d *describer) section(title string) {
	d.b.WriteString(title + ":\n")
}

func (d *describer) line(indent int, format string, args ...any) {
	d.b.WriteString(strings.Repeat(" ", indent) + fmt.Sprintf(format, args...) + "\n")
}

func (d *describer) blank() { d.b.WriteString("\n") }

func describePod(m model, p podInfo) string {
	d := &describer{}
	d.field("Name", p.Name)
	d.field("Namespace", p.Namespace)
	d.field("Priority", "0")
	d.field("Node", p.Node+"/"+nodeIP(m, p.Node))
	d.field("Start Time", fmt.Sprintf("%s ago", age(p.AgeMins)))
	d.field("Labels", "app="+p.OwnerName+"\n"+strings.Repeat(" ", 22)+"pod-template-hash="+token(seeded(p.Name), 9))
	d.field("Status", p.Phase)
	d.field("IP", p.IP)
	d.field("Controlled By", p.OwnerKind+"/"+p.OwnerName)
	d.field("QoS Class", p.QoS)
	d.blank()

	d.section("Containers")
	for i, c := range p.Containers {
		ready := i < p.ReadyCount
		d.line(2, "%s:", c)
		d.line(4, "Container ID:  containerd://%s", token(seeded(p.Name, c), 32))
		d.line(4, "Image:         %s", imageFor(m, p, c))
		d.line(4, "State:         %s", stateFor(p.Phase))
		d.line(4, "Ready:         %t", ready)
		d.line(4, "Restart Count: %d", p.Restarts)
		d.line(4, "Limits:        cpu: 500m, memory: 512Mi")
		d.line(4, "Requests:      cpu: 100m, memory: 128Mi")
	}
	d.blank()

	d.section("Conditions")
	for _, c := range []string{"Initialized", "Ready", "ContainersReady", "PodScheduled"} {
		value := "True"
		if p.Phase != "Running" && (c == "Ready" || c == "ContainersReady") {
			value = "False"
		}
		d.line(2, "%-20s%s", c, value)
	}
	d.blank()

	d.section("Events")
	found := false
	for _, e := range m.events() {
		if e.Object == p.Namespace+"/"+p.Name {
			d.line(2, "%-9s %-18s %-6s %s", e.Type, e.Reason, e.Age, e.Message)
			found = true
		}
	}
	if !found {
		d.line(2, "<none>")
	}
	return d.b.String()
}

func describeNode(m model, n nodeInfo) string {
	d := &describer{}
	d.field("Name", n.Name)
	d.field("Roles", n.Roles)
	d.field("Labels", "kubernetes.io/arch=amd64\n"+strings.Repeat(" ", 22)+"kubernetes.io/hostname="+n.Name)
	d.field("CreationTimestamp", fmt.Sprintf("%s ago", age(n.AgeMins)))
	if n.Taints > 0 {
		d.field("Taints", "node-role.kubernetes.io/control-plane:NoSchedule")
	} else {
		d.field("Taints", "<none>")
	}
	d.field("Unschedulable", "false")
	d.blank()

	d.section("Conditions")
	ready := "True"
	if !n.Ready {
		ready = "False"
	}
	d.line(2, "%-20s%-8s%s", "MemoryPressure", "False", "kubelet has sufficient memory available")
	d.line(2, "%-20s%-8s%s", "DiskPressure", "False", "kubelet has no disk pressure")
	d.line(2, "%-20s%-8s%s", "PIDPressure", "False", "kubelet has sufficient PID available")
	d.line(2, "%-20s%-8s%s", "Ready", ready, "kubelet is posting ready status")
	d.blank()

	d.section("Addresses")
	d.line(2, "InternalIP:  %s", n.IP)
	d.line(2, "Hostname:    %s", n.Name)
	d.blank()

	d.section("Capacity")
	d.line(2, "cpu:     %d", n.CPU)
	d.line(2, "memory:  %dGi", n.MemGiB)
	d.line(2, "pods:    110")
	d.blank()

	d.section("Allocated resources")
	d.line(2, "cpu     %.1f (%.0f%%)", float64(n.CPU)*n.CPUUsed, n.CPUUsed*100)
	d.line(2, "memory  %.1fGi (%.0f%%)", float64(n.MemGiB)*n.MemUsed, n.MemUsed*100)
	d.blank()

	d.section("Non-terminated Pods")
	for _, p := range m.pods {
		if p.Node == n.Name {
			d.line(2, "%-20s %-42s %s", p.Namespace, p.Name, p.Phase)
		}
	}
	return d.b.String()
}

func describeWorkload(m model, w workloadInfo) string {
	d := &describer{}
	d.field("Name", w.Name)
	d.field("Namespace", w.Namespace)
	d.field("CreationTimestamp", fmt.Sprintf("%s ago", age(w.AgeMins)))
	d.field("Labels", "app="+w.Name)
	d.field("Selector", "app="+w.Name)
	if w.Kind == "CronJob" {
		d.field("Schedule", w.Schedule)
		d.field("Concurrency Policy", "Allow")
	} else {
		d.field("Replicas", fmt.Sprintf("%d desired | %d updated | %d available", w.Desired, w.Desired, w.Ready))
		d.field("StrategyType", "RollingUpdate")
	}
	d.blank()

	d.section("Pod Template")
	d.line(2, "Labels:  app=%s", w.Name)
	d.line(2, "Containers:")
	d.line(4, "%s:", w.Name)
	d.line(6, "Image:      %s", w.Image)
	d.line(6, "Port:       8080/TCP")
	d.line(6, "Limits:     cpu: 500m, memory: 512Mi")
	d.line(6, "Requests:   cpu: 100m, memory: 128Mi")
	d.blank()

	d.section("Pods")
	found := false
	for _, p := range m.pods {
		if p.OwnerName == w.Name && p.Namespace == w.Namespace {
			d.line(2, "%-46s %-18s restarts: %d", p.Name, p.Phase, p.Restarts)
			found = true
		}
	}
	if !found {
		d.line(2, "<none>")
	}
	return d.b.String()
}

// describeGeneric covers the kinds without a bespoke renderer. Those objects
// are fabricated inside their table builder rather than in the model, so there
// is nothing richer to report than their identity.
func describeGeneric(m model, kind, namespace, name string) string {
	d := &describer{}
	d.field("Name", name)
	if namespace != "" {
		d.field("Namespace", namespace)
	}
	d.field("Kind", kind)
	d.field("Context", m.ctx.Name)
	d.field("Server", m.ctx.Server)
	d.blank()

	table := BuildTable(m.ctx, kind, AllNamespaces)
	for _, row := range table.Rows {
		if row.Name != name || row.Namespace != namespace {
			continue
		}
		d.section("Details")
		for i, cell := range row.Cells {
			if i < len(table.Columns) {
				d.line(2, "%-20s%s", table.Columns[i]+":", cell.Text)
			}
		}
		return d.b.String()
	}

	d.line(0, "This object is no longer present in the cluster listing.")
	return d.b.String()
}

func nodeIP(m model, name string) string {
	for _, n := range m.nodes {
		if n.Name == name {
			return n.IP
		}
	}
	return "<unknown>"
}

func imageFor(m model, p podInfo, container string) string {
	if container == "istio-proxy" {
		return "docker.io/istio/proxyv2:1.24.1"
	}
	for _, w := range m.workloads {
		if w.Name == p.OwnerName && w.Namespace == p.Namespace {
			return w.Image
		}
	}
	return "unknown"
}

func stateFor(phase string) string {
	switch phase {
	case "Running":
		return "Running"
	case "Succeeded":
		return "Terminated (Completed, exit code 0)"
	case "CrashLoopBackOff":
		return "Waiting (CrashLoopBackOff)"
	case "ImagePullBackOff":
		return "Waiting (ImagePullBackOff)"
	case "Pending":
		return "Waiting (ContainerCreating)"
	default:
		return phase
	}
}
