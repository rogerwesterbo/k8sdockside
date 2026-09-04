package kube

import (
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// running builds a containerStatus for a container that is up.
func running(name string, ready bool) map[string]any {
	return map[string]any{
		"name":  name,
		"ready": ready,
		"state": map[string]any{"running": map[string]any{"startedAt": "2026-09-04T10:00:00Z"}},
	}
}

func terminated(name string, exitCode int64, reason string) map[string]any {
	return map[string]any{
		"name":  name,
		"ready": false,
		"state": map[string]any{
			"terminated": map[string]any{"exitCode": exitCode, "reason": reason},
		},
	}
}

func waiting(name, reason string) map[string]any {
	return map[string]any{
		"name":  name,
		"ready": false,
		"state": map[string]any{"waiting": map[string]any{"reason": reason}},
	}
}

func TestPillForAContainerThatIsUpAndServing(t *testing.T) {
	got := pillFor("app", running("app", true), false)

	if got.Tone != "ok" {
		t.Errorf("tone = %q, want ok", got.Tone)
	}
	if got.Label != "app" {
		t.Errorf("label = %q, want app", got.Label)
	}
	if got.Detail == "" {
		t.Error("a rectangle with no detail says nothing when you hover it")
	}
}

// The state your three colours leave out. A container that is up but failing
// its readiness probe is not fine, has not exited, and has not crashed --
// colouring it green would hide the commonest half-broken state there is.
func TestPillForAContainerUpButNotReadyIsWarned(t *testing.T) {
	got := pillFor("app", running("app", false), false)

	if got.Tone != "warn" {
		t.Errorf("tone = %q, want warn", got.Tone)
	}
}

func TestPillForAContainerThatFinishedCleanly(t *testing.T) {
	got := pillFor("migrate", terminated("migrate", 0, "Completed"), false)

	if got.Tone != "" {
		t.Errorf("tone = %q, want the plain one", got.Tone)
	}
	if got.Detail != "Exited (0)" {
		t.Errorf("detail = %q, want Exited (0)", got.Detail)
	}
}

func TestPillForAContainerThatFailed(t *testing.T) {
	got := pillFor("app", terminated("app", 137, "OOMKilled"), false)

	if got.Tone != "error" {
		t.Errorf("tone = %q, want error", got.Tone)
	}
	// The reason is the useful half: "OOMKilled" says what to do next in a way
	// that "exit 137" does not.
	if got.Detail != "OOMKilled (137)" {
		t.Errorf("detail = %q, want OOMKilled (137)", got.Detail)
	}
}

func TestPillForAContainerThatIsStillStarting(t *testing.T) {
	for _, reason := range []string{"ContainerCreating", "PodInitializing"} {
		got := pillFor("app", waiting("app", reason), false)
		if got.Tone != "" {
			t.Errorf("waiting on %s = tone %q, want the plain one", reason, got.Tone)
		}
	}
}

func TestPillForAContainerThatCannotStart(t *testing.T) {
	for _, reason := range []string{
		"CrashLoopBackOff", "ImagePullBackOff", "ErrImagePull",
		"CreateContainerError", "InvalidImageName", "CreateContainerConfigError",
	} {
		got := pillFor("app", waiting("app", reason), false)
		if got.Tone != "error" {
			t.Errorf("waiting on %s = tone %q, want error", reason, got.Tone)
		}
		if got.Detail != reason {
			t.Errorf("detail = %q, want %q", got.Detail, reason)
		}
	}
}

// A container the kubelet has not reported on yet still gets a rectangle, or
// the count would change as a pod starts.
func TestPillForAContainerWithNoStatusYet(t *testing.T) {
	got := pillFor("app", nil, false)

	if got.Tone != "" {
		t.Errorf("tone = %q, want the plain one", got.Tone)
	}
	if got.Label != "app" {
		t.Errorf("label = %q, want app", got.Label)
	}
}

func TestPillForAnInitContainerSaysSo(t *testing.T) {
	got := pillFor("wait-for-db", terminated("wait-for-db", 0, "Completed"), true)

	if !strings.Contains(got.Detail, "init") {
		t.Errorf("detail = %q, which does not say it is an init container", got.Detail)
	}
}

// ---- the whole cell --------------------------------------------------------

func podWith(spec, status map[string]any) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "Pod",
		"metadata":   map[string]any{"name": "web", "namespace": "default"},
		"spec":       spec,
		"status":     status,
	}}
}

func TestPodContainersDrawsOneRectanglePerContainer(t *testing.T) {
	pod := podWith(
		map[string]any{"containers": []any{
			map[string]any{"name": "app"},
			map[string]any{"name": "sidecar"},
		}},
		map[string]any{"containerStatuses": []any{
			running("app", true),
			running("sidecar", true),
		}},
	)

	got := podContainers(pod)

	if len(got.Pills) != 2 {
		t.Fatalf("drew %d rectangles, want 2", len(got.Pills))
	}
	for _, p := range got.Pills {
		if p.Tone != "ok" {
			t.Errorf("%s = tone %q, want ok", p.Label, p.Tone)
		}
	}
}

// Init containers come first, because a pod stuck in init is exactly what you
// are scanning the column for.
func TestPodContainersPutsInitContainersFirst(t *testing.T) {
	pod := podWith(
		map[string]any{
			"initContainers": []any{map[string]any{"name": "wait-for-db"}},
			"containers":     []any{map[string]any{"name": "app"}},
		},
		map[string]any{
			"initContainerStatuses": []any{waiting("wait-for-db", "PodInitializing")},
			"containerStatuses":     []any{waiting("app", "PodInitializing")},
		},
	)

	got := podContainers(pod)

	if len(got.Pills) != 2 {
		t.Fatalf("drew %d rectangles, want 2", len(got.Pills))
	}
	if got.Pills[0].Label != "wait-for-db" {
		t.Errorf("first rectangle is %q, want the init container", got.Pills[0].Label)
	}
}

// A pod whose containers have not been reported on yet still shows its count.
func TestPodContainersDrawsContainersWithNoStatusYet(t *testing.T) {
	pod := podWith(
		map[string]any{"containers": []any{
			map[string]any{"name": "app"},
			map[string]any{"name": "sidecar"},
		}},
		map[string]any{},
	)

	got := podContainers(pod)

	if len(got.Pills) != 2 {
		t.Errorf("drew %d rectangles for a pod with no statuses, want 2", len(got.Pills))
	}
}

// The text is what the table filters on, so typing a container's name should
// find the pods running it.
func TestPodContainersCanBeFoundByContainerName(t *testing.T) {
	pod := podWith(
		map[string]any{"containers": []any{
			map[string]any{"name": "nginx"},
			map[string]any{"name": "exporter"},
		}},
		map[string]any{"containerStatuses": []any{running("nginx", true), running("exporter", true)}},
	)

	got := podContainers(pod)

	if !strings.Contains(got.Text, "nginx") {
		t.Errorf("text = %q, which the filter cannot match on a container name", got.Text)
	}
}

// Sorting the column should bring the pods in trouble together, which sorting
// container names would not.
func TestPodContainersSortsTroubledPodsFirst(t *testing.T) {
	healthy := podContainers(podWith(
		map[string]any{"containers": []any{map[string]any{"name": "app"}}},
		map[string]any{"containerStatuses": []any{running("app", true)}},
	))
	broken := podContainers(podWith(
		map[string]any{"containers": []any{map[string]any{"name": "app"}}},
		map[string]any{"containerStatuses": []any{waiting("app", "CrashLoopBackOff")}},
	))

	if broken.Sort >= healthy.Sort {
		t.Errorf("a broken pod sorts to %q and a healthy one to %q; the broken one should come first",
			broken.Sort, healthy.Sort)
	}
}

// Running containers come first, so the left of the row is the part that is
// working and the eye lands on the gaps.
func TestPodContainersPutsRunningContainersFirst(t *testing.T) {
	pod := podWith(
		map[string]any{"containers": []any{
			map[string]any{"name": "crashed"},
			map[string]any{"name": "done"},
			map[string]any{"name": "app"},
		}},
		map[string]any{"containerStatuses": []any{
			waiting("crashed", "CrashLoopBackOff"),
			terminated("done", 0, "Completed"),
			running("app", true),
		}},
	)

	got := podContainers(pod)

	if got.Pills[0].Label != "app" {
		t.Errorf("first rectangle is %q, want the running container", got.Pills[0].Label)
	}
}

// Among the ones that are not running, the order they were declared in is kept:
// re-ordering two stopped containers tells the reader nothing and makes the row
// jump about as states change.
func TestPodContainersKeepsTheDeclaredOrderOtherwise(t *testing.T) {
	pod := podWith(
		map[string]any{"containers": []any{
			map[string]any{"name": "first"},
			map[string]any{"name": "second"},
		}},
		map[string]any{"containerStatuses": []any{
			terminated("first", 1, "Error"),
			terminated("second", 0, "Completed"),
		}},
	)

	got := podContainers(pod)

	if got.Pills[0].Label != "first" || got.Pills[1].Label != "second" {
		t.Errorf("order = %q, %q; want the order they were declared in",
			got.Pills[0].Label, got.Pills[1].Label)
	}
}
