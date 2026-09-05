package kube

import (
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestNodeReadingsParseTheUnitsMetricsServerActuallyUses(t *testing.T) {
	// metrics-server reports CPU in nanocores and memory in KiB, which look
	// nothing like the quantities on a pod spec and are the easiest thing in
	// this whole feature to get wrong by three orders of magnitude.
	items := []unstructured.Unstructured{
		*obj(map[string]any{
			"metadata": map[string]any{"name": "worker-1"},
			"usage":    map[string]any{"cpu": "2500000000n", "memory": "8388608Ki"},
		}),
	}

	readings := nodeReadings(items)

	got, ok := readings["worker-1"]
	if !ok {
		t.Fatalf("no reading for worker-1, got %v", readings)
	}
	if got.CPU != 2.5 {
		t.Errorf("CPU = %v, want 2.5 cores", got.CPU)
	}
	if got.Memory != 8 {
		t.Errorf("memory = %v, want 8 GiB", got.Memory)
	}
}

func TestPodReadingsAddUpEveryContainer(t *testing.T) {
	items := []unstructured.Unstructured{
		*obj(map[string]any{
			"metadata": map[string]any{"name": "api-0", "namespace": "prod"},
			"containers": []any{
				map[string]any{"name": "app", "usage": map[string]any{"cpu": "500m", "memory": "1Gi"}},
				map[string]any{"name": "sidecar", "usage": map[string]any{"cpu": "250m", "memory": "512Mi"}},
			},
		}),
	}

	readings := podReadings(items)

	got, ok := readings["prod/api-0"]
	if !ok {
		t.Fatalf("no reading for prod/api-0, got %v", readings)
	}
	if got.CPU != 0.75 {
		t.Errorf("CPU = %v, want 0.75", got.CPU)
	}
	if got.Memory != 1.5 {
		t.Errorf("memory = %v, want 1.5", got.Memory)
	}
}

func TestMetricsServerIsPreferredWhenItAnswers(t *testing.T) {
	primary := Usage{Source: SourceMetricsServer, Nodes: map[string]Measured{"worker-1": {CPU: 1}}}
	fallback := Usage{Source: SourcePrometheus, Nodes: map[string]Measured{"worker-1": {CPU: 99}}}

	got := chooseUsage(primary, fallback)

	if got.Source != SourceMetricsServer {
		t.Errorf("source = %q, want %q", got.Source, SourceMetricsServer)
	}
	if got.Nodes["worker-1"].CPU != 1 {
		t.Error("the fallback's numbers were used even though metrics-server answered")
	}
}

func TestPrometheusIsUsedWhenMetricsServerIsNotInstalled(t *testing.T) {
	primary := Usage{Error: `metrics-server: the server could not find the requested resource`}
	fallback := Usage{Source: SourcePrometheus, Nodes: map[string]Measured{"worker-1": {CPU: 2}}}

	got := chooseUsage(primary, fallback)

	if got.Source != SourcePrometheus {
		t.Errorf("source = %q, want %q", got.Source, SourcePrometheus)
	}
	if got.Nodes["worker-1"].CPU != 2 {
		t.Errorf("CPU = %v, want 2", got.Nodes["worker-1"].CPU)
	}
}

func TestWithNeitherSourceTheReasonNamesBoth(t *testing.T) {
	// Whoever has to fix this needs to know that both were tried, and what each
	// one said -- "no metrics" alone sends them looking in one place.
	primary := Usage{Error: "metrics-server: the server could not find the requested resource"}
	fallback := Usage{Error: "prometheus: no Prometheus service found"}

	got := chooseUsage(primary, fallback)

	if got.Source != "" {
		t.Errorf("source = %q, want empty -- nobody answered", got.Source)
	}
	if !strings.Contains(got.Error, "metrics-server") || !strings.Contains(got.Error, "prometheus") {
		t.Errorf("error = %q, want it to name both sources", got.Error)
	}
}
