package kube

import (
	"fmt"
	"hash/fnv"
	"math/rand"
	"sort"
	"strings"
)

// This file fabricates the cluster data the UI renders. It exists so the whole
// frontend -- tabs, tables, the describe panel -- can be built and used before
// a live API client is wired in.
//
// Everything is derived from one deterministic model per context: pods belong
// to workloads that exist, run on nodes that exist, and events reference real
// pods. That coherence is the point -- it keeps the UI honest about what the
// real data will look like. Replacing this file with client-go calls should not
// require touching the types in resources.go or anything in the frontend.

var (
	appNames = []string{
		"api-gateway", "auth-service", "billing-api", "cart-service", "checkout",
		"search-indexer", "notification-hub", "recommender", "media-worker",
		"metrics-collector", "report-builder", "session-store", "webhook-relay",
	}
	systemApps  = []string{"coredns", "kube-proxy", "metrics-server", "cilium", "node-exporter"}
	registries  = []string{"ghcr.io/acme", "registry.k8s.io", "docker.io/library", "quay.io/acme"}
	k8sVersions = []string{"v1.31.4", "v1.32.2", "v1.33.1", "v1.30.9"}
	distros     = []string{"Talos", "k3s", "kubeadm", "RKE2", "EKS"}
	optionalNS  = []string{"monitoring", "ingress-nginx", "cert-manager", "argocd", "production", "staging", "data", "observability"}
	podPhases   = []string{"Running", "Running", "Running", "Running", "Running", "Running", "Pending", "Succeeded", "CrashLoopBackOff", "ImagePullBackOff", "Terminating"}
)

// seeded returns a generator whose output depends only on the given parts, so
// the same context always produces the same cluster.
func seeded(parts ...string) *rand.Rand {
	h := fnv.New64a()
	for _, p := range parts {
		_, _ = h.Write([]byte(p))
		_, _ = h.Write([]byte{0})
	}
	return rand.New(rand.NewSource(int64(h.Sum64())))
}

type nodeInfo struct {
	Name     string
	Roles    string
	Version  string
	CPU      int
	MemGiB   int
	DiskGiB  int
	Ready    bool
	Taints   int
	IP       string
	AgeMins  int
	CPUUsed  float64
	MemUsed  float64
	DiskUsed float64
}

type workloadInfo struct {
	Kind      string // Deployment, StatefulSet, DaemonSet, Job, CronJob
	Name      string
	Namespace string
	Desired   int
	Ready     int
	Image     string
	Schedule  string // CronJob only
	AgeMins   int
}

type podInfo struct {
	Name       string
	Namespace  string
	Node       string
	Phase      string
	Restarts   int
	Containers []string
	ReadyCount int
	QoS        string
	IP         string
	AgeMins    int
	OwnerKind  string
	OwnerName  string
}

// model is a whole fabricated cluster. Every table and describe view is a
// projection of one of these.
type model struct {
	ctx          Context
	version      string
	distribution string
	namespaces   []string
	nodes        []nodeInfo
	workloads    []workloadInfo
	pods         []podInfo
}

// buildModel fabricates the cluster behind a context. It is deterministic: the
// same context yields the same cluster on every call, so tables stay stable as
// the user moves between tabs.
func buildModel(ctx Context) model {
	r := seeded(ctx.ID)
	m := model{
		ctx:          ctx,
		version:      k8sVersions[r.Intn(len(k8sVersions))],
		distribution: distros[r.Intn(len(distros))],
	}

	m.namespaces = buildNamespaces(r)
	m.nodes = buildNodes(r, m.version)
	m.workloads = buildWorkloads(r, m.namespaces)
	m.pods = buildPods(r, m.workloads, m.nodes)
	return m
}

func buildNamespaces(r *rand.Rand) []string {
	ns := []string{"default", "kube-system", "kube-public", "kube-node-lease"}
	extra := append([]string(nil), optionalNS...)
	r.Shuffle(len(extra), func(i, j int) { extra[i], extra[j] = extra[j], extra[i] })
	ns = append(ns, extra[:3+r.Intn(3)]...)
	sort.Strings(ns)
	return ns
}

func buildNodes(r *rand.Rand, version string) []nodeInfo {
	controlPlanes := 1
	if r.Intn(3) == 0 {
		controlPlanes = 3
	}
	workers := 2 + r.Intn(4)

	nodes := make([]nodeInfo, 0, controlPlanes+workers)
	for i := range controlPlanes {
		nodes = append(nodes, makeNode(r, fmt.Sprintf("cp-%d", i+1), "control-plane", version, true))
	}
	for i := range workers {
		nodes = append(nodes, makeNode(r, fmt.Sprintf("worker-%d", i+1), "worker", version, false))
	}
	// One node down occasionally, so the UI's unhealthy states get exercised.
	if r.Intn(6) == 0 {
		nodes[len(nodes)-1].Ready = false
	}
	return nodes
}

func makeNode(r *rand.Rand, name, role, version string, tainted bool) nodeInfo {
	cpu := []int{4, 8, 16, 32}[r.Intn(4)]
	mem := cpu * 4
	taints := 0
	if tainted {
		taints = 1
	}
	return nodeInfo{
		Name:     name,
		Roles:    role,
		Version:  version,
		CPU:      cpu,
		MemGiB:   mem,
		DiskGiB:  []int{100, 250, 500}[r.Intn(3)],
		Ready:    true,
		Taints:   taints,
		IP:       fmt.Sprintf("192.168.%d.%d", 3+r.Intn(3), 10+r.Intn(200)),
		AgeMins:  60 * (24*7 + r.Intn(24*300)),
		CPUUsed:  0.08 + r.Float64()*0.62,
		MemUsed:  0.20 + r.Float64()*0.55,
		DiskUsed: 0.15 + r.Float64()*0.50,
	}
}

func buildWorkloads(r *rand.Rand, namespaces []string) []workloadInfo {
	var out []workloadInfo

	for _, ns := range namespaces {
		pool, kinds := appNames, []string{"Deployment", "Deployment", "Deployment", "StatefulSet"}
		count := 1 + r.Intn(4)
		if strings.HasPrefix(ns, "kube-") {
			pool, kinds = systemApps, []string{"DaemonSet", "Deployment"}
			count = 2 + r.Intn(2)
		}

		names := append([]string(nil), pool...)
		r.Shuffle(len(names), func(i, j int) { names[i], names[j] = names[j], names[i] })
		if count > len(names) {
			count = len(names)
		}

		for _, name := range names[:count] {
			kind := kinds[r.Intn(len(kinds))]
			desired := 1 + r.Intn(4)
			if kind == "DaemonSet" {
				desired = 0 // filled in from the node count by the caller of Table
			}
			ready := desired
			if r.Intn(7) == 0 && desired > 0 {
				ready = desired - 1
			}
			out = append(out, workloadInfo{
				Kind:      kind,
				Name:      name,
				Namespace: ns,
				Desired:   desired,
				Ready:     ready,
				Image:     fmt.Sprintf("%s/%s:%s", registries[r.Intn(len(registries))], name, semver(r)),
				AgeMins:   60 * (1 + r.Intn(24*90)),
			})
		}
	}

	// A handful of batch workloads, so those tabs are not empty.
	for i := range 2 + r.Intn(3) {
		ns := namespaces[r.Intn(len(namespaces))]
		out = append(out, workloadInfo{
			Kind:      "CronJob",
			Name:      fmt.Sprintf("%s-cron-%d", appNames[r.Intn(len(appNames))], i+1),
			Namespace: ns,
			Desired:   1,
			Ready:     1,
			Image:     fmt.Sprintf("%s/batch:%s", registries[r.Intn(len(registries))], semver(r)),
			Schedule:  []string{"*/5 * * * *", "0 * * * *", "0 2 * * *", "*/15 * * * *", "0 0 * * 0"}[r.Intn(5)],
			AgeMins:   60 * (1 + r.Intn(24*60)),
		})
	}
	for i := range 1 + r.Intn(4) {
		ns := namespaces[r.Intn(len(namespaces))]
		out = append(out, workloadInfo{
			Kind:      "Job",
			Name:      fmt.Sprintf("migrate-db-%d", i+1),
			Namespace: ns,
			Desired:   1,
			Ready:     1,
			Image:     fmt.Sprintf("%s/migrator:%s", registries[r.Intn(len(registries))], semver(r)),
			AgeMins:   5 + r.Intn(60*24),
		})
	}
	return out
}

func buildPods(r *rand.Rand, workloads []workloadInfo, nodes []nodeInfo) []podInfo {
	var pods []podInfo
	for _, w := range workloads {
		replicas := w.Desired
		if w.Kind == "DaemonSet" {
			replicas = len(nodes)
		}
		if w.Kind == "CronJob" {
			replicas = r.Intn(2) // usually nothing running
		}
		for i := range replicas {
			node := nodes[r.Intn(len(nodes))]
			phase := podPhases[r.Intn(len(podPhases))]
			containers := []string{w.Name}
			if r.Intn(4) == 0 {
				containers = append(containers, "istio-proxy")
			}
			// A pod that is not running cannot have all its containers ready;
			// showing 1/1 next to ImagePullBackOff would misrepresent the state.
			ready := len(containers)
			if phase != "Running" {
				ready = r.Intn(len(containers))
			}
			pods = append(pods, podInfo{
				Name:       podName(r, w, i),
				Namespace:  w.Namespace,
				Node:       node.Name,
				Phase:      phase,
				Restarts:   restartsFor(r, phase),
				Containers: containers,
				ReadyCount: ready,
				QoS:        []string{"Guaranteed", "Burstable", "BestEffort"}[r.Intn(3)],
				IP:         fmt.Sprintf("10.244.%d.%d", r.Intn(8), 2+r.Intn(250)),
				AgeMins:    1 + r.Intn(w.AgeMins+2),
				OwnerKind:  w.Kind,
				OwnerName:  w.Name,
			})
		}
	}
	return pods
}

func podName(r *rand.Rand, w workloadInfo, index int) string {
	switch w.Kind {
	case "StatefulSet":
		return fmt.Sprintf("%s-%d", w.Name, index)
	case "DaemonSet":
		return fmt.Sprintf("%s-%s", w.Name, token(r, 5))
	case "Job", "CronJob":
		return fmt.Sprintf("%s-%s", w.Name, token(r, 5))
	default:
		return fmt.Sprintf("%s-%s-%s", w.Name, token(r, 9), token(r, 5))
	}
}

func restartsFor(r *rand.Rand, phase string) int {
	if phase == "CrashLoopBackOff" {
		return 3 + r.Intn(40)
	}
	if r.Intn(5) == 0 {
		return r.Intn(4)
	}
	return 0
}

func token(r *rand.Rand, n int) string {
	const alphabet = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, n)
	for i := range b {
		b[i] = alphabet[r.Intn(len(alphabet))]
	}
	return string(b)
}

func semver(r *rand.Rand) string {
	return fmt.Sprintf("%d.%d.%d", 1+r.Intn(4), r.Intn(20), r.Intn(15))
}

// age renders minutes the way kubectl does: the two most significant units,
// dropping to one once the value is large.
func age(minutes int) string {
	switch {
	case minutes < 1:
		return "0s"
	case minutes < 60:
		return fmt.Sprintf("%dm", minutes)
	case minutes < 60*48:
		h, m := minutes/60, minutes%60
		if h < 10 && m > 0 {
			return fmt.Sprintf("%dh%dm", h, m)
		}
		return fmt.Sprintf("%dh", h)
	default:
		d := minutes / (60 * 24)
		if d < 10 {
			if h := (minutes % (60 * 24)) / 60; h > 0 {
				return fmt.Sprintf("%dd%dh", d, h)
			}
		}
		return fmt.Sprintf("%dd", d)
	}
}

// toneFor maps a status string onto the four tones the frontend knows how to
// colour, so status colouring lives in one place rather than in every view.
func toneFor(status string) string {
	switch status {
	case "Running", "Ready", "Active", "Available", "Bound", "Complete", "Succeeded", "True", "Normal":
		return "ok"
	case "Pending", "Terminating", "Progressing", "ContainerCreating", "Suspended", "Warning":
		return "warn"
	case "CrashLoopBackOff", "ImagePullBackOff", "Failed", "Error", "NotReady", "Evicted", "Lost":
		return "error"
	default:
		return ""
	}
}
