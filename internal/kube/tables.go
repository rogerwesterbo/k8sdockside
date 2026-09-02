package kube

import (
	"fmt"
	"sort"
	"strings"
)

// AllNamespaces is the namespace filter meaning "do not filter".
const AllNamespaces = ""

// Namespaces lists the namespaces of the cluster behind a context.
func Namespaces(ctx Context) []string {
	return buildModel(ctx).namespaces
}

// BuildTable returns the listing for one resource kind, optionally restricted
// to a single namespace. An unknown kind comes back as an empty table with
// Error set rather than as a Go error, so the UI can render the message in
// place of the rows.
func BuildTable(ctx Context, kind, namespace string) Table {
	m := buildModel(ctx)

	var t Table
	switch kind {
	case KindPods:
		t = m.podTable()
	case KindDeployments:
		t = m.workloadTable(KindDeployments, "Deployment")
	case KindStatefulSet:
		t = m.workloadTable(KindStatefulSet, "StatefulSet")
	case KindDaemonSets:
		t = m.daemonSetTable()
	case KindJobs:
		t = m.jobTable()
	case KindCronJobs:
		t = m.cronJobTable()
	case KindServices:
		t = m.serviceTable()
	case KindIngresses:
		t = m.ingressTable()
	case KindConfigMaps:
		t = m.configMapTable()
	case KindSecrets:
		t = m.secretTable()
	case KindPVCs:
		t = m.pvcTable()
	case KindNodes:
		t = m.nodeTable()
	case KindNamespaces:
		t = m.namespaceTable()
	case KindEvents:
		t = m.eventTable()
	default:
		return Table{Kind: kind, Columns: []string{}, Rows: []Row{}, Error: "unknown resource kind: " + kind}
	}

	if t.Namespaced && namespace != AllNamespaces {
		kept := make([]Row, 0, len(t.Rows))
		for _, row := range t.Rows {
			if row.Namespace == namespace {
				kept = append(kept, row)
			}
		}
		t.Rows = kept
	}
	sort.SliceStable(t.Rows, func(i, j int) bool {
		if t.Rows[i].Namespace != t.Rows[j].Namespace {
			return t.Rows[i].Namespace < t.Rows[j].Namespace
		}
		return t.Rows[i].Name < t.Rows[j].Name
	})
	return t
}

// rowID is the key the frontend uses to identify a row across refreshes.
func rowID(kind, namespace, name string) string {
	return kind + "/" + namespace + "/" + name
}

func plain(s string) Cell       { return Cell{Text: s} }
func toned(s, tone string) Cell { return Cell{Text: s, Tone: tone} }
func status(s string) Cell      { return Cell{Text: s, Tone: toneFor(s)} }
func number(n int) Cell         { return Cell{Text: fmt.Sprintf("%d", n)} }
func muted(s string) Cell       { return Cell{Text: s, Tone: "info"} }

func (m model) podTable() Table {
	rows := make([]Row, 0, len(m.pods))
	for _, p := range m.pods {
		restarts := number(p.Restarts)
		switch {
		case p.Restarts > 5:
			restarts.Tone = "error"
		case p.Restarts > 0:
			restarts.Tone = "warn"
		}
		containers := fmt.Sprintf("%d/%d", p.ReadyCount, len(p.Containers))
		containerTone := "ok"
		if p.ReadyCount < len(p.Containers) {
			containerTone = "warn"
		}
		rows = append(rows, Row{
			ID:        rowID(KindPods, p.Namespace, p.Name),
			Name:      p.Name,
			Namespace: p.Namespace,
			Cells: []Cell{
				plain(p.Name),
				muted(p.Namespace),
				toned(containers, containerTone),
				restarts,
				muted(p.Node),
				muted(p.QoS),
				muted(age(p.AgeMins)),
				status(p.Phase),
			},
		})
	}
	return Table{
		Kind:       KindPods,
		Columns:    []string{"Name", "Namespace", "Containers", "Restarts", "Node", "QoS", "Age", "Status"},
		Rows:       rows,
		Namespaced: true,
	}
}

// workloadTable covers Deployments and StatefulSets, which share a shape.
func (m model) workloadTable(kind, ownerKind string) Table {
	rows := []Row{}
	for _, w := range m.workloads {
		if w.Kind != ownerKind {
			continue
		}
		condition, tone := "Available", "ok"
		if w.Ready < w.Desired {
			condition, tone = "Progressing", "warn"
		}
		if w.Ready == 0 {
			condition, tone = "Unavailable", "error"
		}
		rows = append(rows, Row{
			ID:        rowID(kind, w.Namespace, w.Name),
			Name:      w.Name,
			Namespace: w.Namespace,
			Cells: []Cell{
				plain(w.Name),
				muted(w.Namespace),
				toned(fmt.Sprintf("%d/%d", w.Ready, w.Desired), tone),
				muted(w.Image),
				muted(age(w.AgeMins)),
				toned(condition, tone),
			},
		})
	}
	return Table{
		Kind:       kind,
		Columns:    []string{"Name", "Namespace", "Pods", "Image", "Age", "Condition"},
		Rows:       rows,
		Namespaced: true,
	}
}

func (m model) daemonSetTable() Table {
	rows := []Row{}
	desired := len(m.nodes)
	for _, w := range m.workloads {
		if w.Kind != "DaemonSet" {
			continue
		}
		ready := 0
		for _, p := range m.pods {
			if p.OwnerKind == "DaemonSet" && p.OwnerName == w.Name && p.Namespace == w.Namespace && p.Phase == "Running" {
				ready++
			}
		}
		tone := "ok"
		if ready < desired {
			tone = "warn"
		}
		rows = append(rows, Row{
			ID:        rowID(KindDaemonSets, w.Namespace, w.Name),
			Name:      w.Name,
			Namespace: w.Namespace,
			Cells: []Cell{
				plain(w.Name),
				muted(w.Namespace),
				number(desired),
				number(desired),
				toned(fmt.Sprintf("%d", ready), tone),
				muted("<none>"),
				muted(age(w.AgeMins)),
			},
		})
	}
	return Table{
		Kind:       KindDaemonSets,
		Columns:    []string{"Name", "Namespace", "Desired", "Current", "Ready", "Node Selector", "Age"},
		Rows:       rows,
		Namespaced: true,
	}
}

func (m model) jobTable() Table {
	rows := []Row{}
	for _, w := range m.workloads {
		if w.Kind != "Job" {
			continue
		}
		r := seeded(m.ctx.ID, "job", w.Namespace, w.Name)
		condition := "Complete"
		if r.Intn(5) == 0 {
			condition = "Failed"
		}
		rows = append(rows, Row{
			ID:        rowID(KindJobs, w.Namespace, w.Name),
			Name:      w.Name,
			Namespace: w.Namespace,
			Cells: []Cell{
				plain(w.Name),
				muted(w.Namespace),
				plain("1/1"),
				muted(fmt.Sprintf("%ds", 20+r.Intn(600))),
				muted(age(w.AgeMins)),
				status(condition),
			},
		})
	}
	return Table{
		Kind:       KindJobs,
		Columns:    []string{"Name", "Namespace", "Completions", "Duration", "Age", "Condition"},
		Rows:       rows,
		Namespaced: true,
	}
}

func (m model) cronJobTable() Table {
	rows := []Row{}
	for _, w := range m.workloads {
		if w.Kind != "CronJob" {
			continue
		}
		r := seeded(m.ctx.ID, "cronjob", w.Namespace, w.Name)
		suspended := r.Intn(6) == 0
		active := 0
		for _, p := range m.pods {
			if p.OwnerKind == "CronJob" && p.OwnerName == w.Name {
				active++
			}
		}
		suspendCell := toned("False", "ok")
		if suspended {
			suspendCell = toned("True", "warn")
		}
		rows = append(rows, Row{
			ID:        rowID(KindCronJobs, w.Namespace, w.Name),
			Name:      w.Name,
			Namespace: w.Namespace,
			Cells: []Cell{
				plain(w.Name),
				muted(w.Namespace),
				plain(w.Schedule),
				suspendCell,
				number(active),
				muted(age(r.Intn(120))),
				muted(age(w.AgeMins)),
			},
		})
	}
	return Table{
		Kind:       KindCronJobs,
		Columns:    []string{"Name", "Namespace", "Schedule", "Suspend", "Active", "Last Schedule", "Age"},
		Rows:       rows,
		Namespaced: true,
	}
}

func (m model) serviceTable() Table {
	rows := []Row{}
	for _, w := range m.workloads {
		if w.Kind != "Deployment" && w.Kind != "StatefulSet" {
			continue
		}
		r := seeded(m.ctx.ID, "svc", w.Namespace, w.Name)
		svcType := []string{"ClusterIP", "ClusterIP", "ClusterIP", "NodePort", "LoadBalancer"}[r.Intn(5)]
		port := 8080 + r.Intn(900)
		ports := fmt.Sprintf("%d/TCP", port)
		external := "-"
		switch svcType {
		case "NodePort":
			ports = fmt.Sprintf("%d:%d/TCP", port, 30000+r.Intn(2767))
		case "LoadBalancer":
			external = fmt.Sprintf("192.168.%d.%d", 3+r.Intn(3), 200+r.Intn(50))
		}
		rows = append(rows, Row{
			ID:        rowID(KindServices, w.Namespace, w.Name),
			Name:      w.Name,
			Namespace: w.Namespace,
			Cells: []Cell{
				plain(w.Name),
				muted(w.Namespace),
				plain(svcType),
				muted(fmt.Sprintf("10.96.%d.%d", r.Intn(255), 1+r.Intn(254))),
				muted(ports),
				muted(external),
				muted(age(w.AgeMins)),
				status("Active"),
			},
		})
	}
	return Table{
		Kind:       KindServices,
		Columns:    []string{"Name", "Namespace", "Type", "Cluster IP", "Ports", "External IP", "Age", "Status"},
		Rows:       rows,
		Namespaced: true,
	}
}

func (m model) ingressTable() Table {
	rows := []Row{}
	for _, w := range m.workloads {
		if w.Kind != "Deployment" || strings.HasPrefix(w.Namespace, "kube-") {
			continue
		}
		r := seeded(m.ctx.ID, "ing", w.Namespace, w.Name)
		if r.Intn(3) != 0 {
			continue // not every deployment is exposed
		}
		rows = append(rows, Row{
			ID:        rowID(KindIngresses, w.Namespace, w.Name),
			Name:      w.Name,
			Namespace: w.Namespace,
			Cells: []Cell{
				plain(w.Name),
				muted(w.Namespace),
				muted("nginx"),
				plain(fmt.Sprintf("%s.%s.example.com", w.Name, w.Namespace)),
				muted("/"),
				muted(age(w.AgeMins)),
			},
		})
	}
	return Table{
		Kind:       KindIngresses,
		Columns:    []string{"Name", "Namespace", "Class", "Hosts", "Path", "Age"},
		Rows:       rows,
		Namespaced: true,
	}
}

func (m model) configMapTable() Table {
	rows := []Row{}
	for _, ns := range m.namespaces {
		r := seeded(m.ctx.ID, "cm", ns)
		for i := range 2 + r.Intn(4) {
			name := fmt.Sprintf("%s-config-%d", ns, i+1)
			if i == 0 {
				name = "kube-root-ca.crt"
			}
			rows = append(rows, Row{
				ID:        rowID(KindConfigMaps, ns, name),
				Name:      name,
				Namespace: ns,
				Cells: []Cell{
					plain(name),
					muted(ns),
					number(1 + r.Intn(8)),
					muted(age(60 * (1 + r.Intn(24*120)))),
				},
			})
		}
	}
	return Table{
		Kind:       KindConfigMaps,
		Columns:    []string{"Name", "Namespace", "Keys", "Age"},
		Rows:       rows,
		Namespaced: true,
	}
}

func (m model) secretTable() Table {
	types := []string{"Opaque", "kubernetes.io/tls", "kubernetes.io/dockerconfigjson", "kubernetes.io/service-account-token"}
	rows := []Row{}
	for _, ns := range m.namespaces {
		r := seeded(m.ctx.ID, "secret", ns)
		for i := range 1 + r.Intn(4) {
			name := fmt.Sprintf("%s-secret-%d", ns, i+1)
			rows = append(rows, Row{
				ID:        rowID(KindSecrets, ns, name),
				Name:      name,
				Namespace: ns,
				Cells: []Cell{
					plain(name),
					muted(ns),
					muted(types[r.Intn(len(types))]),
					number(1 + r.Intn(4)),
					muted(age(60 * (1 + r.Intn(24*120)))),
				},
			})
		}
	}
	return Table{
		Kind:       KindSecrets,
		Columns:    []string{"Name", "Namespace", "Type", "Keys", "Age"},
		Rows:       rows,
		Namespaced: true,
	}
}

func (m model) pvcTable() Table {
	classes := []string{"local-path", "longhorn", "nfs-client", "ceph-rbd"}
	rows := []Row{}
	for _, w := range m.workloads {
		if w.Kind != "StatefulSet" {
			continue
		}
		r := seeded(m.ctx.ID, "pvc", w.Namespace, w.Name)
		for i := range w.Desired {
			name := fmt.Sprintf("data-%s-%d", w.Name, i)
			phase := "Bound"
			if r.Intn(12) == 0 {
				phase = "Pending"
			}
			rows = append(rows, Row{
				ID:        rowID(KindPVCs, w.Namespace, name),
				Name:      name,
				Namespace: w.Namespace,
				Cells: []Cell{
					plain(name),
					muted(w.Namespace),
					muted(classes[r.Intn(len(classes))]),
					plain(fmt.Sprintf("%dGi", []int{1, 5, 10, 20, 50, 100}[r.Intn(6)])),
					muted(fmt.Sprintf("%s-%d", w.Name, i)),
					muted(age(w.AgeMins)),
					status(phase),
				},
			})
		}
	}
	return Table{
		Kind:       KindPVCs,
		Columns:    []string{"Name", "Namespace", "Storage Class", "Size", "Pod", "Age", "Status"},
		Rows:       rows,
		Namespaced: true,
	}
}

func (m model) nodeTable() Table {
	rows := make([]Row, 0, len(m.nodes))
	for _, n := range m.nodes {
		condition := "Ready"
		if !n.Ready {
			condition = "NotReady"
		}
		rows = append(rows, Row{
			ID:   rowID(KindNodes, "", n.Name),
			Name: n.Name,
			Cells: []Cell{
				plain(n.Name),
				muted(fmt.Sprintf("%.0f%% of %d", n.CPUUsed*100, n.CPU)),
				muted(fmt.Sprintf("%.0f%% of %dGi", n.MemUsed*100, n.MemGiB)),
				muted(fmt.Sprintf("%.0f%% of %dGi", n.DiskUsed*100, n.DiskGiB)),
				number(n.Taints),
				plain(n.Roles),
				muted(n.Version),
				muted(age(n.AgeMins)),
				status(condition),
			},
		})
	}
	return Table{
		Kind:    KindNodes,
		Columns: []string{"Name", "CPU", "Memory", "Disk", "Taints", "Roles", "Version", "Age", "Conditions"},
		Rows:    rows,
	}
}

func (m model) namespaceTable() Table {
	rows := make([]Row, 0, len(m.namespaces))
	for _, ns := range m.namespaces {
		r := seeded(m.ctx.ID, "ns", ns)
		rows = append(rows, Row{
			ID:   rowID(KindNamespaces, "", ns),
			Name: ns,
			Cells: []Cell{
				plain(ns),
				muted(fmt.Sprintf("kubernetes.io/metadata.name=%s", ns)),
				muted(age(60 * (24 + r.Intn(24*400)))),
				status("Active"),
			},
		})
	}
	return Table{
		Kind:    KindNamespaces,
		Columns: []string{"Name", "Labels", "Age", "Status"},
		Rows:    rows,
	}
}

func (m model) eventTable() Table {
	rows := []Row{}
	for _, e := range m.events() {
		name := fmt.Sprintf("%s.%s", e.Object, e.Reason)
		rows = append(rows, Row{
			ID:        rowID(KindEvents, "", name),
			Name:      name,
			Namespace: "",
			Cells: []Cell{
				status(e.Type),
				plain(e.Message),
				muted(e.Reason),
				muted(e.Object),
				muted(e.Age),
			},
		})
	}
	return Table{
		Kind:    KindEvents,
		Columns: []string{"Type", "Message", "Reason", "Involved Object", "Last Seen"},
		Rows:    rows,
	}
}

// events derives cluster events from the pods that are not healthy, plus a
// little routine background chatter.
func (m model) events() []Event {
	r := seeded(m.ctx.ID, "events")
	var out []Event

	for _, p := range m.pods {
		switch p.Phase {
		case "CrashLoopBackOff":
			out = append(out, Event{"Warning", "BackOff", p.Namespace + "/" + p.Name,
				fmt.Sprintf("Back-off restarting failed container %s", p.Containers[0]), age(r.Intn(90))})
		case "ImagePullBackOff":
			out = append(out, Event{"Warning", "Failed", p.Namespace + "/" + p.Name,
				fmt.Sprintf("Failed to pull image for container %s: not found", p.Containers[0]), age(r.Intn(90))})
		case "Pending":
			out = append(out, Event{"Warning", "FailedScheduling", p.Namespace + "/" + p.Name,
				"0/" + fmt.Sprint(len(m.nodes)) + " nodes are available: insufficient cpu", age(r.Intn(60))})
		}
	}
	for _, n := range m.nodes {
		if !n.Ready {
			out = append(out, Event{"Warning", "NodeNotReady", "node/" + n.Name,
				"Node " + n.Name + " status is now: NodeNotReady", age(r.Intn(120))})
		}
	}
	// Some healthy noise so the list is not purely alarming.
	for i := 0; i < 4 && i < len(m.pods); i++ {
		p := m.pods[r.Intn(len(m.pods))]
		out = append(out, Event{"Normal", "Scheduled", p.Namespace + "/" + p.Name,
			"Successfully assigned " + p.Namespace + "/" + p.Name + " to " + p.Node, age(r.Intn(240))})
	}
	if out == nil {
		out = []Event{}
	}
	return out
}
