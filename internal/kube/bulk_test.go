package kube

import (
	"context"
	"errors"
	"strings"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	clienttesting "k8s.io/client-go/testing"
)

// Bulk delete runs against client-go's fake dynamic client: the loop is the
// thing under test, and a fake that refuses one name is the case that matters.

var podsGVR = schema.GroupVersionResource{Version: "v1", Resource: "pods"}

func podsMapping() *meta.RESTMapping {
	return &meta.RESTMapping{
		Resource:         podsGVR,
		GroupVersionKind: schema.GroupVersionKind{Version: "v1", Kind: "Pod"},
		Scope:            meta.RESTScopeNamespace,
	}
}

func bulkPod(namespace, name string) runtime.Object {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "Pod",
		"metadata":   map[string]any{"namespace": namespace, "name": name},
	}}
}

func fakePods(objs ...runtime.Object) *dynamicfake.FakeDynamicClient {
	return dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		runtime.NewScheme(), map[schema.GroupVersionResource]string{podsGVR: "PodList"}, objs...,
	)
}

func gone(t *testing.T, client *dynamicfake.FakeDynamicClient, namespace, name string) bool {
	t.Helper()
	_, err := client.Resource(podsGVR).Namespace(namespace).Get(context.Background(), name, metav1.GetOptions{})
	return apierrors.IsNotFound(err)
}

func TestDeleteEachRemovesEveryObjectAndCountsTheGoneOnes(t *testing.T) {
	client := fakePods(bulkPod("default", "web-1"), bulkPod("default", "web-2"), bulkPod("kube-system", "dns"))

	report := deleteEach(client, podsMapping(), []ObjectRef{
		{Namespace: "default", Name: "web-1"},
		{Namespace: "default", Name: "web-2"},
		{Namespace: "kube-system", Name: "dns"},
		// Finished between being ticked and the button: what was asked for
		// already holds, so it is not a failure to report.
		{Namespace: "default", Name: "ghost"},
	})

	if report.Deleted != 4 || len(report.Failures) != 0 {
		t.Fatalf("report = %+v, want 4 deleted and no failures", report)
	}
	for _, ref := range [][2]string{{"default", "web-1"}, {"default", "web-2"}, {"kube-system", "dns"}} {
		if !gone(t, client, ref[0], ref[1]) {
			t.Errorf("%s/%s is still there", ref[0], ref[1])
		}
	}
}

func TestDeleteEachReportsRefusalsAndGoesOnWithTheRest(t *testing.T) {
	client := fakePods(bulkPod("default", "web-1"), bulkPod("default", "locked"), bulkPod("default", "web-2"))
	client.PrependReactor("delete", "pods", func(action clienttesting.Action) (bool, runtime.Object, error) {
		if action.(clienttesting.DeleteAction).GetName() == "locked" {
			return true, nil, apierrors.NewForbidden(schema.GroupResource{Resource: "pods"}, "locked", errors.New("protected"))
		}
		return false, nil, nil
	})

	report := deleteEach(client, podsMapping(), []ObjectRef{
		{Namespace: "default", Name: "web-1"},
		{Namespace: "default", Name: "locked"},
		{Namespace: "default", Name: "web-2"},
	})

	if report.Deleted != 2 {
		t.Errorf("deleted = %d, want 2: one refusal must not stop the rest", report.Deleted)
	}
	if len(report.Failures) != 1 || report.Failures[0].Name != "locked" {
		t.Fatalf("failures = %+v, want exactly the locked pod", report.Failures)
	}
	if !strings.Contains(report.Failures[0].Error, "forbidden") {
		t.Errorf("failure reads %q, want the API server's own words", report.Failures[0].Error)
	}
	if !gone(t, client, "default", "web-1") || !gone(t, client, "default", "web-2") {
		t.Error("the pods beside the refused one were not deleted")
	}
	if gone(t, client, "default", "locked") {
		t.Error("the refused pod was deleted anyway")
	}
}

// Requests finish in whatever order the cluster answers; the report reads in
// the table's order so the names can be found again.
func TestDeleteEachOrdersFailuresLikeTheTable(t *testing.T) {
	client := fakePods()
	client.PrependReactor("delete", "pods", func(clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewForbidden(schema.GroupResource{Resource: "pods"}, "", errors.New("no"))
	})

	report := deleteEach(client, podsMapping(), []ObjectRef{
		{Namespace: "kube-system", Name: "dns"},
		{Namespace: "default", Name: "web-2"},
		{Namespace: "default", Name: "web-1"},
	})

	var got []string
	for _, f := range report.Failures {
		got = append(got, f.Namespace+"/"+f.Name)
	}
	want := "default/web-1 default/web-2 kube-system/dns"
	if strings.Join(got, " ") != want {
		t.Errorf("failures in order %v, want %s", got, want)
	}
	if report.Deleted != 0 {
		t.Errorf("deleted = %d with everything refused, want 0", report.Deleted)
	}
}
