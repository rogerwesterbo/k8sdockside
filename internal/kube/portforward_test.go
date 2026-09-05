package kube

import (
	"strings"
	"sync"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func serviceWith(spec map[string]any) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "Service",
		"metadata":   map[string]any{"name": "web", "namespace": "default"},
		"spec":       spec,
	}}
}

func TestPortsOfServiceNamesWhereEachPortLands(t *testing.T) {
	service := serviceWith(map[string]any{"ports": []any{
		map[string]any{"name": "http", "port": int64(80), "targetPort": "web"},
		map[string]any{"port": int64(443), "targetPort": int64(8443), "protocol": "TCP"},
		// A port with no targetPort lands on itself, which the picker shows by
		// saying nothing rather than by repeating the number.
		map[string]any{"name": "metrics", "port": int64(9090)},
	}})

	got := portsOfService(service)

	if len(got) != 3 {
		t.Fatalf("found %d ports, want 3: %+v", len(got), got)
	}
	if got[0].Name != "http" || got[0].Port != 80 || got[0].Target != "web" {
		t.Errorf("the named port read as %+v", got[0])
	}
	if got[1].Port != 443 || got[1].Target != "8443" {
		t.Errorf("the numbered target read as %+v", got[1])
	}
	if got[2].Target != "" {
		t.Errorf("a port with no target claimed one: %+v", got[2])
	}
	// Protocol is filled in rather than left empty: TCP is the API's default
	// and the picker should not have to know that.
	for _, port := range got {
		if port.Protocol != "TCP" {
			t.Errorf("port %d has protocol %q, want TCP", port.Port, port.Protocol)
		}
	}
}

func TestPortsOfContainersReadsEveryDeclaredPort(t *testing.T) {
	containers := []any{
		map[string]any{"name": "app", "ports": []any{
			map[string]any{"name": "http", "containerPort": int64(8080)},
			map[string]any{"containerPort": int64(9090), "protocol": "UDP"},
		}},
		// A container declaring nothing contributes nothing rather than a blank
		// row: a port of zero is not a port.
		map[string]any{"name": "sidecar"},
	}

	got := portsOfContainers(containers)

	if len(got) != 2 {
		t.Fatalf("found %d ports, want 2: %+v", len(got), got)
	}
	if got[0].Name != "http" || got[0].Port != 8080 {
		t.Errorf("the named port read as %+v", got[0])
	}
	// UDP is listed rather than hidden: a port that exists should not vanish
	// from the picker, and it is refused when it is chosen.
	if got[1].Protocol != "UDP" {
		t.Errorf("the UDP port read as %+v", got[1])
	}
}

func TestServicePortOnPodResolvesEveryShapeOfTargetPort(t *testing.T) {
	pod := podWith(map[string]any{"containers": []any{
		map[string]any{"name": "app", "ports": []any{
			map[string]any{"name": "web", "containerPort": int64(8080)},
		}},
	}}, map[string]any{})

	service := serviceWith(map[string]any{"ports": []any{
		map[string]any{"port": int64(80), "targetPort": "web"},
		map[string]any{"port": int64(443), "targetPort": int64(8443)},
		map[string]any{"port": int64(9090)},
	}})

	cases := []struct {
		name string
		port int32
		want int32
	}{
		{"a named target resolves against the pod", 80, 8080},
		{"a numbered target is taken as it is", 443, 8443},
		{"no target means the service's own port", 9090, 9090},
		// A port the service does not serve is taken as a pod port. Somebody
		// forwarding a debug port that was never added to the service means
		// exactly that, and it is what kubectl does.
		{"a port the service does not have falls through to the pod", 6060, 6060},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := servicePortOnPod(service, pod, tc.port)
			if err != nil {
				t.Fatalf("resolving %d: %v", tc.port, err)
			}
			if got != tc.want {
				t.Fatalf("port %d landed on %d, want %d", tc.port, got, tc.want)
			}
		})
	}
}

func TestServicePortOnPodRefusesWhatCannotBeForwarded(t *testing.T) {
	pod := podWith(map[string]any{"containers": []any{
		map[string]any{"name": "app", "ports": []any{
			map[string]any{"name": "web", "containerPort": int64(8080)},
		}},
	}}, map[string]any{})

	udp := serviceWith(map[string]any{"ports": []any{
		map[string]any{"port": int64(53), "protocol": "UDP"},
	}})
	if _, err := servicePortOnPod(udp, pod, 53); err == nil || !strings.Contains(err.Error(), "UDP") {
		t.Fatalf("a UDP port was accepted, or refused without saying why: %v", err)
	}

	missing := serviceWith(map[string]any{"ports": []any{
		map[string]any{"port": int64(80), "targetPort": "grpc"},
	}})
	if _, err := servicePortOnPod(missing, pod, 80); err == nil || !strings.Contains(err.Error(), "grpc") {
		t.Fatalf("a target the pod does not declare was accepted, or refused without naming it: %v", err)
	}
}

func TestServiceSelectorIsStableAndRefusesAServiceWithoutOne(t *testing.T) {
	service := serviceWith(map[string]any{"selector": map[string]any{
		"app":  "web",
		"tier": "front",
	}})

	got, err := serviceSelector(service)
	if err != nil {
		t.Fatalf("reading the selector: %v", err)
	}
	// Sorted, so the same service asks the same question every time -- map
	// iteration order would otherwise make this a different query each call.
	if got != "app=web,tier=front" {
		t.Fatalf("selector is %q, want app=web,tier=front", got)
	}

	// A service with hand-written endpoints often points outside the cluster
	// entirely, and there is no pod for a forward to reach.
	if _, err := serviceSelector(serviceWith(map[string]any{})); err == nil {
		t.Fatal("a service selecting nothing was accepted")
	}
}

func TestLinesHandsOverWholeLinesOnly(t *testing.T) {
	var mu sync.Mutex
	var got []string
	out := &lines{emit: func(text string) {
		mu.Lock()
		defer mu.Unlock()
		got = append(got, text)
	}}

	// client-go writes prose meant for a terminal, in whatever pieces the
	// buffer gave it. A partial write is not a message.
	if _, err := out.Write([]byte("Forwarding from 127.0.0.1:8080")); err != nil {
		t.Fatalf("writing: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("a half-written line was reported: %v", got)
	}

	if _, err := out.Write([]byte(" -> 80\nHandling connection\n\n")); err != nil {
		t.Fatalf("writing: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("reported %d lines, want 2: %v", len(got), got)
	}
	if got[0] != "Forwarding from 127.0.0.1:8080 -> 80" {
		t.Errorf("the first line came out as %q", got[0])
	}
	// A blank line is a wake-up for no reason.
	if got[1] != "Handling connection" {
		t.Errorf("the second line came out as %q", got[1])
	}
}

func TestPortNumberReadsTheShapesJSONArrivesIn(t *testing.T) {
	for _, tc := range []struct {
		name  string
		value any
		want  int32
	}{
		{"the dynamic client's int64", int64(8080), 8080},
		{"a JSON round trip's float64", float64(8080), 8080},
		{"a plain int", 8080, 8080},
		{"a name, which is not a number", "http", 0},
		{"nothing at all", nil, 0},
		// Not a port, whatever it is: a port is two bytes on the wire, and
		// taking the low half of a bigger number would forward to something
		// nobody asked for.
		{"a number too big to be a port", int64(99999), 0},
		{"a negative one", int64(-1), 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := portNumber(tc.value); got != tc.want {
				t.Fatalf("portNumber(%v) = %d, want %d", tc.value, got, tc.want)
			}
		})
	}
}
