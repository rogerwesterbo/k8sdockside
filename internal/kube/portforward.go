package kube

// Forwarding a local port into the cluster.
//
// A forward is a tunnel that stays up: one long-lived request to a pod's
// `portforward` subresource, with a local listener in front of it. Everything
// here is about the two questions that have to be answered before that request
// can be made -- which pod, and which port on it -- because the thing the user
// picked is very often neither.
//
// A Service is the case in point. It has ports of its own that exist nowhere
// but in the cluster's own routing: forwarding to "the service on 80" means
// finding a pod behind it, working out which port on that pod 80 actually
// lands on, and forwarding to that. kubectl does the same thing, and its
// output saying `Forwarding from 127.0.0.1:8080 -> 8080` is quietly naming the
// pod's port rather than the service's.

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport/spdy"
	"k8s.io/streaming/pkg/httpstream"
)

// PortOption is one port a forward could be started on, as offered to the user
// before anything is opened.
type PortOption struct {
	// Name is the port's name in the object, empty for an unnamed one.
	Name string `json:"name"`
	// Port is what the user would ask for: a container port on a pod, a service
	// port on a Service.
	Port int32 `json:"port"`
	// Protocol is TCP or UDP. UDP is listed rather than hidden, and refused
	// when chosen: a port that exists should not silently vanish from the list,
	// and "port-forward cannot do UDP" is a better answer than no answer.
	Protocol string `json:"protocol"`
	// Target is where a service port lands on the pod, when that differs --
	// shown so that "8080 → http" is visible before it is opened rather than
	// afterwards in a log line.
	Target string `json:"target"`
}

// ForwardEndpoint is what a forward actually reached: the pod the tunnel goes
// to and the port on it. It is reported back because for a Service neither is
// what the user typed, and a forward that broke because the pod behind it went
// away can only be understood if the pod was named.
type ForwardEndpoint struct {
	Namespace string `json:"namespace"`
	Pod       string `json:"pod"`
	Port      int32  `json:"port"`
}

// portsOfContainers reads the ports declared by a set of containers.
func portsOfContainers(containers []any) []PortOption {
	out := []PortOption{}
	for _, raw := range containers {
		for _, rawPort := range asSlice(asMap(raw)["ports"]) {
			port := asMap(rawPort)
			number := portNumber(port["containerPort"])
			if number == 0 {
				continue
			}
			protocol := mapString(port, "protocol")
			if protocol == "" {
				protocol = "TCP"
			}
			out = append(out, PortOption{
				Name:     mapString(port, "name"),
				Port:     number,
				Protocol: protocol,
			})
		}
	}
	return out
}

// portsOfService reads a service's ports, each with where it lands on the pod.
func portsOfService(obj *unstructured.Unstructured) []PortOption {
	out := []PortOption{}
	for _, raw := range nestedSlice(obj, "spec", "ports") {
		port := asMap(raw)
		number := portNumber(port["port"])
		if number == 0 {
			continue
		}
		protocol := mapString(port, "protocol")
		if protocol == "" {
			protocol = "TCP"
		}
		out = append(out, PortOption{
			Name:     mapString(port, "name"),
			Port:     number,
			Protocol: protocol,
			Target:   targetPortText(port["targetPort"]),
		})
	}
	return out
}

// targetPortText renders a service's targetPort, which is a number or a name.
func targetPortText(value any) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return v
	default:
		if n := portNumber(value); n != 0 {
			return fmt.Sprintf("%d", n)
		}
		return ""
	}
}

// ForwardablePorts lists what could be forwarded from one object.
//
// Pods and Services answer for themselves; a workload answers from its pod
// template, which is the same list its pods will have and is available without
// finding one of them first.
func (w *Watcher) ForwardablePorts(kc Context, kind, namespace, name string) ([]PortOption, error) {
	out := []PortOption{}
	err := w.withClient(kc, func(c *clusterClient) error {
		ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
		defer cancel()

		obj, _, err := c.get(ctx, kind, namespace, name)
		if err != nil {
			return err
		}

		switch kind {
		case KindServices:
			out = portsOfService(obj)
		case KindPods:
			out = portsOfContainers(nestedSlice(obj, "spec", "containers"))
		default:
			out = portsOfContainers(nestedSlice(obj, "spec", "template", "spec", "containers"))
		}
		return nil
	})
	return out, err
}

// endpointFor works out which pod a forward goes to, and which port on it.
func (c *clusterClient) endpointFor(ctx context.Context, kind, namespace, name string, port int32) (ForwardEndpoint, error) {
	if kind != KindServices {
		pod, err := c.runningPodFor(ctx, kind, namespace, name)
		if err != nil {
			return ForwardEndpoint{}, err
		}
		return ForwardEndpoint{Namespace: pod.GetNamespace(), Pod: pod.GetName(), Port: port}, nil
	}

	service, _, err := c.get(ctx, kind, namespace, name)
	if err != nil {
		return ForwardEndpoint{}, err
	}

	selector, err := serviceSelector(service)
	if err != nil {
		return ForwardEndpoint{}, err
	}
	mapping, err := c.mappingForKind(KindPods)
	if err != nil {
		return ForwardEndpoint{}, err
	}
	pods, err := resourceFor(c.dynamic, mapping, namespace).
		List(ctx, metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		return ForwardEndpoint{}, err
	}

	for i := range pods.Items {
		pod := &pods.Items[i]
		phase, _, _ := unstructured.NestedString(pod.Object, "status", "phase")
		if phase != "Running" {
			continue
		}
		target, err := servicePortOnPod(service, pod, port)
		if err != nil {
			return ForwardEndpoint{}, err
		}
		return ForwardEndpoint{Namespace: pod.GetNamespace(), Pod: pod.GetName(), Port: target}, nil
	}
	return ForwardEndpoint{}, fmt.Errorf("service %s has no running pod behind it to forward to", name)
}

// serviceSelector reads the labels a service routes to. A service without one
// is routed by hand-written endpoints -- often to something outside the cluster
// entirely -- and there is no pod for a forward to reach.
func serviceSelector(service *unstructured.Unstructured) (string, error) {
	raw, found, err := unstructured.NestedStringMap(service.Object, "spec", "selector")
	if err != nil || !found || len(raw) == 0 {
		return "", fmt.Errorf(
			"service %s selects no pods -- its endpoints are set by hand, so there is nothing to forward to",
			service.GetName(),
		)
	}
	parts := make([]string, 0, len(raw))
	for key, value := range raw {
		parts = append(parts, key+"="+value)
	}
	// Sorted, so the same service asks the same question every time.
	sort.Strings(parts)
	return strings.Join(parts, ","), nil
}

// servicePortOnPod translates a service port into the port on the pod behind
// it, which is what the forward's request actually names.
//
// A targetPort may be a number, a name declared by one of the pod's containers,
// or absent -- in which case it is the service port itself.
func servicePortOnPod(service, pod *unstructured.Unstructured, port int32) (int32, error) {
	for _, raw := range nestedSlice(service, "spec", "ports") {
		entry := asMap(raw)
		if portNumber(entry["port"]) != port {
			continue
		}
		if protocol := mapString(entry, "protocol"); protocol == "UDP" {
			return 0, fmt.Errorf("port %d is UDP, and port-forward can only carry TCP", port)
		}

		switch target := entry["targetPort"].(type) {
		case nil:
			return port, nil
		case string:
			if target == "" {
				return port, nil
			}
			for _, container := range nestedSlice(pod, "spec", "containers") {
				for _, rawPort := range asSlice(asMap(container)["ports"]) {
					declared := asMap(rawPort)
					if mapString(declared, "name") == target {
						return portNumber(declared["containerPort"]), nil
					}
				}
			}
			return 0, fmt.Errorf(
				"port %d points at %q, which pod %s does not declare",
				port, target, pod.GetName(),
			)
		default:
			if number := portNumber(target); number != 0 {
				return number, nil
			}
			return port, nil
		}
	}

	// The service does not serve that port. Rather than refuse, take it as a
	// pod port: somebody forwarding to a service's debug port that was never
	// added to the service means exactly that, and it is what kubectl does.
	return port, nil
}

// lines turns the forwarder's own writes into whole lines for the caller.
//
// client-go writes progress to one writer and trouble to another, in prose
// meant for a terminal. The wording is worth keeping -- "an error occurred
// forwarding 8080 -> 80" names both ends -- but a partial write is not a
// message, so they are buffered to the newline.
type lines struct {
	buf  bytes.Buffer
	emit func(string)
}

func (l *lines) Write(p []byte) (int, error) {
	l.buf.Write(p)
	for {
		text := l.buf.String()
		at := strings.IndexByte(text, '\n')
		if at < 0 {
			return len(p), nil
		}
		line := strings.TrimSpace(text[:at])
		l.buf.Next(at + 1)
		if line != "" && l.emit != nil {
			l.emit(line)
		}
	}
}

// Forward opens a tunnel from a local port to a port in the cluster and blocks
// until it is closed or breaks.
//
// A local port of 0 means "any free one", which is what the app asks for unless
// the user named a port: a forward is usually reached by clicking the link the
// app offers, and picking a port by hand only matters when something else --
// a config file, a bookmark, another process -- already expects one.
//
// `ready` is called once the listener is up, with the port it actually got and
// the pod it reached.
func (w *Watcher) Forward(
	ctx context.Context,
	kc Context,
	kind, namespace, name string,
	remotePort, localPort int32,
	ready func(local int32, at ForwardEndpoint),
	note func(string),
) error {
	return w.withClient(kc, func(c *clusterClient) error {
		resolve, cancel := context.WithTimeout(ctx, callTimeout)
		defer cancel()
		endpoint, err := c.endpointFor(resolve, kind, namespace, name, remotePort)
		if err != nil {
			return err
		}

		url := c.typed.CoreV1().RESTClient().Post().
			Resource("pods").
			Namespace(endpoint.Namespace).
			Name(endpoint.Pod).
			SubResource("portforward").
			URL()

		// The same two-protocol arrangement an exec uses, for the same reason:
		// websockets where the API server speaks them, SPDY where it or
		// something between here and it does not.
		transport, upgrader, err := spdy.RoundTripperFor(c.cfg)
		if err != nil {
			return err
		}
		legacy := spdy.NewDialer(upgrader, &http.Client{Transport: transport}, "POST", url)
		tunnelled, err := portforward.NewSPDYOverWebsocketDialer(url, c.cfg)
		if err != nil {
			return err
		}
		dialer := portforward.NewFallbackDialer(tunnelled, legacy, func(err error) bool {
			return httpstream.IsUpgradeFailure(err) || httpstream.IsHTTPSProxyError(err)
		})

		listening := make(chan struct{})
		forwarder, err := portforward.NewOnAddressesWithContext(
			ctx,
			dialer,
			// Both loopback addresses, so a browser sent to localhost reaches
			// it whichever of the two that name resolves to on this machine.
			[]string{"localhost"},
			[]string{fmt.Sprintf("%d:%d", localPort, endpoint.Port)},
			listening,
			&lines{emit: note},
			&lines{emit: note},
		)
		if err != nil {
			return err
		}

		go func() {
			select {
			case <-ctx.Done():
			case <-listening:
				ports, err := forwarder.GetPorts()
				if err != nil || len(ports) == 0 {
					return
				}
				if ready != nil {
					ready(int32(ports[0].Local), endpoint)
				}
			}
		}()

		return forwarder.ForwardPorts()
	})
}

// asSlice is nestedSlice's counterpart for a value already in hand.
func asSlice(v any) []any {
	out, _ := v.([]any)
	return out
}

// portNumber reads a port out of an unstructured value.
//
// A targetPort is whichever of a number and a name it was written as, so the
// type is not known until it is looked at -- and the dynamic client decodes
// JSON numbers to int64 while an object that has been through a round trip can
// hold float64, so both are read.
//
// Anything outside 1..65535 is not a port and comes back as zero, which every
// caller already treats as "there isn't one". That is the same check that makes
// the narrowing safe: a port is two bytes on the wire, and a number that could
// not fit in them was never one.
func portNumber(v any) int32 {
	var n int64
	switch value := v.(type) {
	case int64:
		n = value
	case int32:
		n = int64(value)
	case int:
		n = int64(value)
	case float64:
		n = int64(value)
	default:
		return 0
	}
	if n <= 0 || n > 65535 {
		return 0
	}
	return int32(n)
}

var _ io.Writer = (*lines)(nil)
