package kube

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/util/jsonpath"
)

// This file holds the column values that cannot be read straight out of a
// field: counts across a pod's containers, a workload's condition, a service's
// port list. Everything simpler is a JSONPath in columns.go.

// evalPath renders a kubectl-style JSONPath against an object. An expression
// that matches nothing yields "", which is what the tables want -- a missing
// optional field is not an error worth showing in a cell.
func evalPath(u *unstructured.Unstructured, path string) string {
	if path == "" {
		return ""
	}
	jp := jsonpath.New("column").AllowMissingKeys(true)
	if err := jp.Parse("{" + path + "}"); err != nil {
		return ""
	}
	var buf bytes.Buffer
	if err := jp.Execute(&buf, u.Object); err != nil {
		return ""
	}
	return buf.String()
}

// ratio renders "ready/desired" and tones it by how far short it falls.
func ratio(ready, desired int64) Cell {
	text := fmt.Sprintf("%d/%d", ready, desired)
	switch {
	case desired == 0 && ready == 0:
		return muted(text)
	case ready >= desired:
		return toned(text, "ok")
	case ready == 0:
		return toned(text, "error")
	default:
		return toned(text, "warn")
	}
}

// parseTime reads an RFC3339 timestamp, returning the zero time for anything
// missing or unparseable so the cell renders as "<none>" rather than as noise.
func parseTime(ts string) time.Time {
	if ts == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		return time.Time{}
	}
	return t
}

// since renders an RFC3339 timestamp as an age, for the report text that has
// nowhere to carry a sort key.
func since(ts string) string {
	t := parseTime(ts)
	if t.IsZero() {
		return "<none>"
	}
	return age(int(time.Since(t).Minutes()))
}

// joinMap renders a label or selector map deterministically.
func joinMap(m map[string]string) string {
	if len(m) == 0 {
		return "<none>"
	}
	parts := make([]string, 0, len(m))
	for k, v := range m {
		parts = append(parts, k+"="+v)
	}
	sort.Strings(parts)
	return strings.Join(parts, ", ")
}

// joinAny renders the first few entries of a string list, saying how many more
// there are rather than overflowing the cell.
func joinAny(items []any, limit int) string {
	if len(items) == 0 {
		return "<none>"
	}
	parts := make([]string, 0, len(items))
	for _, it := range items {
		if s, ok := it.(string); ok {
			parts = append(parts, s)
		}
	}
	if len(parts) > limit {
		return strings.Join(parts[:limit], ", ") + fmt.Sprintf(" +%d more", len(parts)-limit)
	}
	return strings.Join(parts, ", ")
}

// ---- pods ------------------------------------------------------------------

func podReady(u *unstructured.Unstructured) Cell {
	statuses := nestedSlice(u, "status", "containerStatuses")
	ready := 0
	for _, raw := range statuses {
		if isReady, _ := asMap(raw)["ready"].(bool); isReady {
			ready++
		}
	}
	total := len(statuses)
	if total == 0 {
		total = len(nestedSlice(u, "spec", "containers"))
	}
	return ratio(int64(ready), int64(total))
}

// podStatus reproduces the status kubectl shows, which is not simply the phase:
// a pod stuck pulling an image is Pending with a container waiting on
// ImagePullBackOff, and the waiting reason is the useful half.
func podStatus(u *unstructured.Unstructured) Cell {
	if u.GetDeletionTimestamp() != nil {
		return status("Terminating")
	}
	for _, raw := range nestedSlice(u, "status", "containerStatuses") {
		state := asMap(asMap(raw)["state"])
		if waiting := asMap(state["waiting"]); waiting != nil {
			if reason := mapString(waiting, "reason"); reason != "" {
				return status(reason)
			}
		}
		if term := asMap(state["terminated"]); term != nil {
			if reason := mapString(term, "reason"); reason != "" && reason != "Completed" {
				return status(reason)
			}
		}
	}
	if reason := nestedString(u, "status", "reason"); reason != "" {
		return status(reason)
	}
	return status(nestedString(u, "status", "phase"))
}

func podRestarts(u *unstructured.Unstructured) Cell {
	var total int64
	for _, raw := range nestedSlice(u, "status", "containerStatuses") {
		if n, ok := asMap(raw)["restartCount"].(int64); ok {
			total += n
		}
	}
	cell := number(int(total))
	switch {
	case total > 5:
		cell.Tone = "error"
	case total > 0:
		cell.Tone = "warn"
	}
	return cell
}

// ---- workloads -------------------------------------------------------------

func firstImage(u *unstructured.Unstructured) Cell {
	containers := nestedSlice(u, "spec", "template", "spec", "containers")
	if len(containers) == 0 {
		return muted("")
	}
	image := mapString(asMap(containers[0]), "image")
	if len(containers) > 1 {
		image += fmt.Sprintf(" +%d more", len(containers)-1)
	}
	return muted(image)
}

func workloadCondition(u *unstructured.Unstructured) Cell {
	ready := nestedInt(u, "status", "readyReplicas")
	desired := nestedInt(u, "spec", "replicas")
	switch {
	case desired == 0:
		return muted("Scaled to zero")
	case ready >= desired:
		return toned("Available", "ok")
	case ready == 0:
		return toned("Unavailable", "error")
	default:
		return toned("Progressing", "warn")
	}
}

func jobDuration(u *unstructured.Unstructured) Cell {
	start := nestedString(u, "status", "startTime")
	if start == "" {
		return muted("")
	}
	from, err := time.Parse(time.RFC3339, start)
	if err != nil {
		return muted("")
	}
	until := time.Now()
	if done := parseTime(nestedString(u, "status", "completionTime")); !done.IsZero() {
		until = done
	}
	return durationCell(until.Sub(from))
}

func jobCondition(u *unstructured.Unstructured) Cell {
	switch {
	case conditionStatus(u, "Complete", "status", "conditions") == "True":
		return status("Complete")
	case conditionStatus(u, "Failed", "status", "conditions") == "True":
		return status("Failed")
	case nestedInt(u, "status", "active") > 0:
		return status("Running")
	default:
		return muted("Pending")
	}
}

// ---- services and ingresses ------------------------------------------------

func servicePorts(u *unstructured.Unstructured) Cell {
	var parts []string
	for _, raw := range nestedSlice(u, "spec", "ports") {
		p := asMap(raw)
		port, _ := p["port"].(int64)
		proto := mapString(p, "protocol")
		if proto == "" {
			proto = "TCP"
		}
		if node, ok := p["nodePort"].(int64); ok && node > 0 {
			parts = append(parts, fmt.Sprintf("%d:%d/%s", port, node, proto))
			continue
		}
		parts = append(parts, fmt.Sprintf("%d/%s", port, proto))
	}
	if len(parts) == 0 {
		return muted("<none>")
	}
	return muted(strings.Join(parts, ", "))
}

func serviceExternalIP(u *unstructured.Unstructured) Cell {
	if external := nestedSlice(u, "spec", "externalIPs"); len(external) > 0 {
		return muted(joinAny(external, 2))
	}
	return loadBalancerAddress(u)
}

// loadBalancerAddress reads the address a load balancer or ingress controller
// has assigned, which may be an IP or a hostname depending on the provider.
func loadBalancerAddress(u *unstructured.Unstructured) Cell {
	var parts []string
	for _, raw := range nestedSlice(u, "status", "loadBalancer", "ingress") {
		e := asMap(raw)
		if ip := mapString(e, "ip"); ip != "" {
			parts = append(parts, ip)
			continue
		}
		if host := mapString(e, "hostname"); host != "" {
			parts = append(parts, host)
		}
	}
	if len(parts) == 0 {
		return muted("<pending>")
	}
	return muted(strings.Join(parts, ", "))
}

func ingressHosts(u *unstructured.Unstructured) Cell {
	var hosts []string
	for _, raw := range nestedSlice(u, "spec", "rules") {
		if host := mapString(asMap(raw), "host"); host != "" {
			hosts = append(hosts, host)
		}
	}
	if len(hosts) == 0 {
		return muted("*")
	}
	if len(hosts) > 3 {
		return plain(strings.Join(hosts[:3], ", ") + fmt.Sprintf(" +%d more", len(hosts)-3))
	}
	return plain(strings.Join(hosts, ", "))
}

// ---- nodes -----------------------------------------------------------------

// nodeRoles reads the roles off the well-known label prefix, the same place
// kubectl looks -- a node's role is a label convention, not a field.
func nodeRoles(u *unstructured.Unstructured) Cell {
	var roles []string
	for k, v := range u.GetLabels() {
		if role, ok := strings.CutPrefix(k, "node-role.kubernetes.io/"); ok && role != "" {
			roles = append(roles, role)
			continue
		}
		if k == "kubernetes.io/role" && v != "" {
			roles = append(roles, v)
		}
	}
	if len(roles) == 0 {
		return muted("<none>")
	}
	sort.Strings(roles)
	return plain(strings.Join(roles, ", "))
}

func nodeCondition(u *unstructured.Unstructured) Cell {
	if unschedulable, _, _ := unstructured.NestedBool(u.Object, "spec", "unschedulable"); unschedulable {
		return toned("SchedulingDisabled", "warn")
	}
	switch conditionStatus(u, "Ready", "status", "conditions") {
	case "True":
		return status("Ready")
	case "False":
		return status("NotReady")
	default:
		return status("Unknown")
	}
}

// ---- Gateway API -----------------------------------------------------------

func gatewayAddress(u *unstructured.Unstructured) Cell {
	var parts []string
	for _, raw := range nestedSlice(u, "status", "addresses") {
		if v := mapString(asMap(raw), "value"); v != "" {
			parts = append(parts, v)
		}
	}
	if len(parts) == 0 {
		return muted("<pending>")
	}
	return muted(strings.Join(parts, ", "))
}

// routeParents lists the Gateways a route has attached itself to, which is the
// question anyone opening a route list is actually asking.
func routeParents(u *unstructured.Unstructured) Cell {
	var parts []string
	for _, raw := range nestedSlice(u, "spec", "parentRefs") {
		p := asMap(raw)
		name := mapString(p, "name")
		if name == "" {
			continue
		}
		if ns := mapString(p, "namespace"); ns != "" {
			name = ns + "/" + name
		}
		if section := mapString(p, "sectionName"); section != "" {
			name += "#" + section
		}
		parts = append(parts, name)
	}
	if len(parts) == 0 {
		return muted("<none>")
	}
	return muted(strings.Join(parts, ", "))
}

func referenceGrantFrom(u *unstructured.Unstructured) Cell {
	var parts []string
	for _, raw := range nestedSlice(u, "spec", "from") {
		f := asMap(raw)
		parts = append(parts, mapString(f, "kind")+" in "+mapString(f, "namespace"))
	}
	if len(parts) == 0 {
		return muted("<none>")
	}
	return muted(strings.Join(parts, ", "))
}

// crdVersions lists the versions a CRD serves, marking the storage version,
// since that is the one whose schema actually persists.
func crdVersions(u *unstructured.Unstructured) Cell {
	var parts []string
	for _, raw := range nestedSlice(u, "spec", "versions") {
		v := asMap(raw)
		name := mapString(v, "name")
		if name == "" {
			continue
		}
		if stored, _ := v["storage"].(bool); stored {
			name += "*"
		}
		parts = append(parts, name)
	}
	if len(parts) == 0 {
		return muted("")
	}
	return plain(strings.Join(parts, ", "))
}

// ---- resource quantities ---------------------------------------------------

// parseCPU reads a CPU quantity ("500m", "2") as a number of cores.
func parseCPU(s string) float64 {
	if s == "" {
		return 0
	}
	q, err := resource.ParseQuantity(s)
	if err != nil {
		return 0
	}
	return q.AsApproximateFloat64()
}

// parseMemory reads a memory quantity ("512Mi", "8Gi") as GiB, the unit the
// dashboard gauges are labelled in.
func parseMemory(s string) float64 {
	if s == "" {
		return 0
	}
	q, err := resource.ParseQuantity(s)
	if err != nil {
		return 0
	}
	return q.AsApproximateFloat64() / (1 << 30)
}

// requestsOf totals what a pod asks the scheduler for, in cores and GiB.
//
// Init containers run before the others and are counted as the largest single
// one rather than added on top, because that is how the scheduler reserves for
// them -- summing would overstate every pod that has any.
func requestsOf(pod *unstructured.Unstructured) (cpu, mem float64) {
	for _, raw := range nestedSlice(pod, "spec", "containers") {
		requests := asMap(asMap(asMap(raw)["resources"])["requests"])
		cpu += parseCPU(mapString(requests, "cpu"))
		mem += parseMemory(mapString(requests, "memory"))
	}

	var initCPU, initMem float64
	for _, raw := range nestedSlice(pod, "spec", "initContainers") {
		requests := asMap(asMap(asMap(raw)["resources"])["requests"])
		initCPU = max(initCPU, parseCPU(mapString(requests, "cpu")))
		initMem = max(initMem, parseMemory(mapString(requests, "memory")))
	}
	return max(cpu, initCPU), max(mem, initMem)
}
