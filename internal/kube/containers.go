package kube

// A pod's containers, as the row of rectangles the table draws.
//
// The column says two things at once that would otherwise take a sentence: how
// many containers a pod has, and which of them is in trouble. Colour carries
// the state, so a screen of pods can be scanned rather than read, and the
// container's name and its state in words ride along for the tooltip -- which
// is also what a screen reader gets.

import (
	"fmt"
	"sort"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// badWaits are the reasons a container is waiting that mean it is not coming
// up on its own. Every other reason -- ContainerCreating, PodInitializing -- is
// a container on its way, which is not news.
var badWaits = map[string]bool{
	"CrashLoopBackOff":           true,
	"ImagePullBackOff":           true,
	"ErrImagePull":               true,
	"ErrImageNeverPull":          true,
	"InvalidImageName":           true,
	"CreateContainerError":       true,
	"CreateContainerConfigError": true,
	"RunContainerError":          true,
}

// pillFor reads one container's status into its rectangle.
//
// A nil status is a container the kubelet has not reported on yet, which still
// gets a rectangle: the count must not change as a pod starts.
func pillFor(name string, status map[string]any, init bool) Pill {
	pill := stateOfContainer(name, status)
	if init {
		// Said in the tooltip rather than drawn differently: an init container
		// is one of the pod's containers, and the count is the point.
		pill.Detail += " · init container"
	}
	return pill
}

// stateOfContainer is pillFor without the init note, which is the part worth
// reading on its own.
func stateOfContainer(name string, status map[string]any) Pill {
	pill := Pill{Label: name, Detail: "Waiting"}
	if status == nil {
		return pill
	}

	state := asMap(status["state"])

	if term := asMap(state["terminated"]); term != nil {
		code := mapInt(term, "exitCode")
		reason := mapString(term, "reason")
		if code == 0 {
			pill.Detail = "Exited (0)"
			return pill
		}
		if reason == "" {
			reason = "Error"
		}
		pill.Tone, pill.Detail = "error", fmt.Sprintf("%s (%d)", reason, code)
		return pill
	}

	if wait := asMap(state["waiting"]); wait != nil {
		reason := mapString(wait, "reason")
		if reason == "" {
			reason = "Waiting"
		}
		pill.Detail = reason
		if badWaits[reason] {
			pill.Tone = "error"
		}
		return pill
	}

	if asMap(state["running"]) != nil {
		if ready, _ := status["ready"].(bool); ready {
			pill.Tone, pill.Detail = "ok", "Running"
		} else {
			// Up, but its readiness probe is not passing. Neither fine nor
			// broken, and green would hide it.
			pill.Tone, pill.Detail = "warn", "Running, not ready"
		}
		return pill
	}

	return pill
}

// podContainers builds the row of rectangles for one pod.
func podContainers(u *unstructured.Unstructured) Cell {
	pills := containerPills(u, "initContainers", "initContainerStatuses", true)
	pills = append(pills, containerPills(u, "containers", "containerStatuses", false)...)

	// Running containers to the left, so the working part of a pod reads first
	// and the eye lands on the gap where something is missing. Stable, so that
	// among the rest the declared order stands: re-ordering two stopped
	// containers says nothing, and would make the row jump about as a pod
	// starts.
	sort.SliceStable(pills, func(i, j int) bool {
		return isRunning(pills[i]) && !isRunning(pills[j])
	})

	names := make([]string, 0, len(pills))
	troubled := 0
	for _, p := range pills {
		names = append(names, p.Label)
		if p.Tone == "error" || p.Tone == "warn" {
			troubled++
		}
	}

	return Cell{
		// The names rather than a count, so filtering the table by a container
		// name finds the pods running it.
		Text:  strings.Join(names, " "),
		Pills: pills,
		// Sorted by how much is wrong, descending, so one click brings the pods
		// worth looking at to the top. Sorting the names would be no use to
		// anyone.
		Sort: fmt.Sprintf("%04d", len(pills)-troubled),
	}
}

// containerPills pairs the containers a pod declares with the statuses it has.
//
// The spec is the source of how many there are and the status of how they are
// doing: a pod that has just been scheduled has the first and not the second,
// and it still has its containers.
func containerPills(u *unstructured.Unstructured, specField, statusField string, init bool) []Pill {
	statuses := map[string]map[string]any{}
	for _, raw := range nestedSlice(u, "status", statusField) {
		s := asMap(raw)
		statuses[mapString(s, "name")] = s
	}

	declared := nestedSlice(u, "spec", specField)
	pills := make([]Pill, 0, len(declared))
	for _, raw := range declared {
		name := mapString(asMap(raw), "name")
		pills = append(pills, pillFor(name, statuses[name], init))
	}

	// A container reported on but no longer declared is rare -- an ephemeral
	// debug container, or a spec read a moment behind the status -- but it is
	// running on the node and belongs in the count.
	if len(pills) < len(statuses) {
		var extra []string
		for name := range statuses {
			if !declares(declared, name) {
				extra = append(extra, name)
			}
		}
		sort.Strings(extra)
		for _, name := range extra {
			pills = append(pills, pillFor(name, statuses[name], init))
		}
	}
	return pills
}

// isRunning is what puts a rectangle on the left: a container that is up,
// whether or not its readiness probe has caught up yet.
func isRunning(p Pill) bool {
	return p.Tone == "ok" || p.Tone == "warn"
}

func declares(containers []any, name string) bool {
	for _, raw := range containers {
		if mapString(asMap(raw), "name") == name {
			return true
		}
	}
	return false
}
