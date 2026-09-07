package kube

import (
	"context"
	"errors"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/types"

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
	return labelledPod(namespace, name, map[string]any{"app": name})
}

func labelledPod(namespace, name string, labels map[string]any) runtime.Object {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "Pod",
		"metadata":   map[string]any{"namespace": namespace, "name": name, "labels": labels},
	}}
}

func labelsOf(t *testing.T, client *dynamicfake.FakeDynamicClient, namespace, name string) map[string]string {
	t.Helper()
	got, err := client.Resource(podsGVR).Namespace(namespace).Get(context.Background(), name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("reading %s/%s back: %v", namespace, name, err)
	}
	return got.GetLabels()
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

	if report.Done != 4 || len(report.Failures) != 0 {
		t.Fatalf("report = %+v, want 4 done and no failures", report)
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

	if report.Done != 2 {
		t.Errorf("done = %d, want 2: one refusal must not stop the rest", report.Done)
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
	if report.Done != 0 {
		t.Errorf("done = %d with everything refused, want 0", report.Done)
	}
}

// One merge patch, applied to every ticked object: a value sets, null removes,
// and what the patch does not mention is left alone.
func TestPatchEachSetsAndRemovesOnEveryObject(t *testing.T) {
	client := fakePods(
		labelledPod("default", "web-1", map[string]any{"app": "web", "team": "old"}),
		labelledPod("default", "web-2", map[string]any{"app": "web", "tier": "front"}),
	)
	patch := []byte(`{"metadata":{"labels":{"team":"platform","tier":null}}}`)

	report := patchEach(client, podsMapping(), []ObjectRef{
		{Namespace: "default", Name: "web-1"},
		{Namespace: "default", Name: "web-2"},
	}, patch)

	if report.Done != 2 || len(report.Failures) != 0 {
		t.Fatalf("report = %+v, want both done", report)
	}
	for _, name := range []string{"web-1", "web-2"} {
		labels := labelsOf(t, client, "default", name)
		if labels["team"] != "platform" {
			t.Errorf("%s team = %q, want platform", name, labels["team"])
		}
		if _, still := labels["tier"]; still {
			t.Errorf("%s still carries tier, want it removed by the null", name)
		}
		if labels["app"] != "web" {
			t.Errorf("%s app = %q, want the label the patch did not mention left alone", name, labels["app"])
		}
	}
}

// Unlike a delete, patching what has gone is a failure: the label was wanted
// on it, and there is nothing to put it on.
func TestPatchEachReportsWhatIsNotThere(t *testing.T) {
	client := fakePods(bulkPod("default", "web-1"))

	report := patchEach(client, podsMapping(), []ObjectRef{
		{Namespace: "default", Name: "web-1"},
		{Namespace: "default", Name: "ghost"},
	}, []byte(`{"metadata":{"labels":{"team":"web"}}}`))

	if report.Done != 1 || len(report.Failures) != 1 || report.Failures[0].Name != "ghost" {
		t.Fatalf("report = %+v, want one done and the ghost refused", report)
	}
	if !strings.Contains(report.Failures[0].Error, "not found") {
		t.Errorf("failure reads %q, want the API server's not-found", report.Failures[0].Error)
	}
}

func TestPatchEachSendsAMergePatch(t *testing.T) {
	client := fakePods(bulkPod("default", "web-1"))
	var sent types.PatchType
	client.PrependReactor("patch", "pods", func(action clienttesting.Action) (bool, runtime.Object, error) {
		sent = action.(clienttesting.PatchAction).GetPatchType()
		return false, nil, nil
	})

	patchEach(client, podsMapping(), []ObjectRef{{Namespace: "default", Name: "web-1"}}, []byte(`{"spec":{"x":1}}`))

	if sent != types.MergePatchType {
		t.Errorf("patch type = %q, want a JSON merge patch, the one form every kind accepts", sent)
	}
}

// The YAML editor's patch and the form's JSON are read by the same parser,
// and come out as the same compact JSON.
func TestMergePatchFromYAMLReadsYAMLAndJSONAlike(t *testing.T) {
	yamlText := "metadata:\n  labels:\n    team: platform\n    old: null\nspec:\n  replicas: 2\n"
	want := `{"metadata":{"labels":{"old":null,"team":"platform"}},"spec":{"replicas":2}}`

	if got := MergePatchFromYAML(yamlText); got.JSON != want || got.Error != "" || got.Empty {
		t.Errorf("from YAML: %+v, want %s", got, want)
	}
	if got := MergePatchFromYAML(want); got.JSON != want {
		t.Errorf("from JSON: %+v, want it passed through", got)
	}
}

func TestMergePatchFromYAMLKeepsQuotedNumbersAsText(t *testing.T) {
	got := MergePatchFromYAML("metadata:\n  labels:\n    build: \"2\"\n")
	if want := `{"metadata":{"labels":{"build":"2"}}}`; got.JSON != want {
		t.Errorf("got %s, want %s: the quotes said text", got.JSON, want)
	}
}

// Comments and blank lines are what the editor starts with, and are not a
// mistake -- only not a patch yet.
func TestMergePatchFromYAMLCallsCommentsEmpty(t *testing.T) {
	for _, text := range []string{"", "   \n", "# a merge patch\n# spec:\n#   replicas: 2\n"} {
		if got := MergePatchFromYAML(text); !got.Empty || got.Error != "" {
			t.Errorf("MergePatchFromYAML(%q) = %+v, want empty and no error", text, got)
		}
	}
}

func TestMergePatchFromYAMLRefusesWhatIsNotAMapping(t *testing.T) {
	for _, text := range []string{"- a\n- b\n", "just words\n", "a: 1\nb: 2\na: 3\n", "a: [\n"} {
		got := MergePatchFromYAML(text)
		if got.Error == "" || got.JSON != "" {
			t.Errorf("MergePatchFromYAML(%q) = %+v, want a refusal", text, got)
		}
	}
	// A repeated key names its line, as the editor's check does.
	if got := MergePatchFromYAML("a: 1\nb: 2\na: 3\n"); got.Line != 3 {
		t.Errorf("line = %d, want 3: %+v", got.Line, got)
	}
}

// The bytes the objects receive, or the reason nothing is sent to any of them.
func TestMergePatchBytesStopsBeforeAnythingIsTouched(t *testing.T) {
	if _, err := mergePatchBytes("# nothing\n"); err == nil {
		t.Error("an empty patch was accepted")
	}
	if _, err := mergePatchBytes("- a\n"); err == nil {
		t.Error("a list was accepted as a patch")
	}
	body, err := mergePatchBytes("spec:\n  replicas: 2\n")
	if err != nil || string(body) != `{"spec":{"replicas":2}}` {
		t.Errorf("got %s, %v; want the compact JSON", body, err)
	}
}
