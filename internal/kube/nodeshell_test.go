package kube

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
)

func TestNodeShellSpecFallsBackToSomethingThatWorks(t *testing.T) {
	got := NodeShellSpec{}.withDefaults()
	if got.Image != DefaultNodeShellImage || got.Namespace != DefaultNodeShellNamespace {
		t.Fatalf("an empty spec resolved to %+v", got)
	}

	// What the user chose is left alone: both fields exist because a cluster
	// can force them.
	chosen := NodeShellSpec{Namespace: "kube-system", Image: "registry.internal/busybox:1.36"}.withDefaults()
	if chosen.Namespace != "kube-system" || chosen.Image != "registry.internal/busybox:1.36" {
		t.Fatalf("a chosen spec was overwritten: %+v", chosen)
	}
}

func TestNodeShellPodIsAWayIntoTheMachineAndNothingMore(t *testing.T) {
	pod := nodeShellPod("worker-01", NodeShellSpec{}.withDefaults())

	if pod.Spec.NodeName != "worker-01" {
		t.Errorf("the pod would land on %q rather than the node asked for", pod.Spec.NodeName)
	}
	// The three namespaces, without which the pod is a container on the node
	// rather than a view of it.
	if !pod.Spec.HostPID || !pod.Spec.HostNetwork || !pod.Spec.HostIPC {
		t.Error("the pod does not share the node's namespaces")
	}
	if pod.Spec.RestartPolicy != corev1.RestartPolicyNever {
		t.Errorf("restart policy is %q; a shell that restarts itself is not a shell", pod.Spec.RestartPolicy)
	}

	// A node worth opening a shell on is often tainted -- a control-plane node,
	// or one that is misbehaving -- and a pod that would not schedule there is
	// no use.
	if len(pod.Spec.Tolerations) != 1 || pod.Spec.Tolerations[0].Operator != corev1.TolerationOpExists {
		t.Errorf("tolerations are %+v, want one that tolerates everything", pod.Spec.Tolerations)
	}

	if pod.Spec.ActiveDeadlineSeconds == nil || *pod.Spec.ActiveDeadlineSeconds <= 0 {
		t.Error("the pod has no deadline, so one orphaned by a crash would live forever")
	}
	// It has power over the machine; it is deliberately given no identity in
	// the cluster to go with it.
	if pod.Spec.AutomountServiceAccountToken == nil || *pod.Spec.AutomountServiceAccountToken {
		t.Error("the pod mounts a service account token it has no use for")
	}

	if len(pod.Spec.Containers) != 1 {
		t.Fatalf("the pod has %d containers, want 1", len(pod.Spec.Containers))
	}
	container := pod.Spec.Containers[0]
	if container.SecurityContext == nil || container.SecurityContext.Privileged == nil || !*container.SecurityContext.Privileged {
		t.Error("the container is not privileged, so the chroot would be refused")
	}
	if len(container.VolumeMounts) != 1 || container.VolumeMounts[0].MountPath != "/host" {
		t.Errorf("the host is mounted at %+v, want /host", container.VolumeMounts)
	}
	if len(pod.Spec.Volumes) != 1 || pod.Spec.Volumes[0].HostPath == nil || pod.Spec.Volumes[0].HostPath.Path != "/" {
		t.Errorf("the volume is %+v, want the whole host filesystem", pod.Spec.Volumes)
	}

	// Named after the app and the node, so that a pod somebody finds in their
	// cluster says what made it and what for.
	if pod.GenerateName == "" || pod.Labels["app.kubernetes.io/managed-by"] != "k8sdockside" {
		t.Errorf("the pod does not say what made it: %+v", pod.ObjectMeta)
	}
	if pod.Labels["k8sdockside.io/node-shell"] != "worker-01" {
		t.Errorf("the pod does not say which node it is for: %+v", pod.Labels)
	}
}

func TestPodReasonSaysTheMostUsefulThingAPodHas(t *testing.T) {
	message := &corev1.Pod{Status: corev1.PodStatus{Phase: corev1.PodFailed, Reason: "Evicted", Message: "The node was low on resource: memory"}}
	if got := podReason(message); got != "The node was low on resource: memory" {
		t.Errorf("a pod with a message reported %q", got)
	}

	reason := &corev1.Pod{Status: corev1.PodStatus{Phase: corev1.PodFailed, Reason: "Evicted"}}
	if got := podReason(reason); got != "Evicted" {
		t.Errorf("a pod with only a reason reported %q", got)
	}

	// A pod that says nothing at all still has its phase, which is better than
	// an empty sentence.
	bare := &corev1.Pod{Status: corev1.PodStatus{Phase: corev1.PodFailed}}
	if got := podReason(bare); got != "Failed" {
		t.Errorf("a silent pod reported %q", got)
	}
}
