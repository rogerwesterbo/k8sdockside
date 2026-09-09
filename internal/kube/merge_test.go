package kube

import (
	"reflect"
	"testing"
)

// The case this exists for: a controller wrote status while somebody edited
// spec. Nothing about that is a disagreement, and a save refused over it is a
// save refused for no reason the user can act on.
func TestAControllerWritingStatusIsNotAConflict(t *testing.T) {
	opened := map[string]any{
		"spec":   map[string]any{"nodes": int64(3), "version": "1.31"},
		"status": map[string]any{"phase": "Provisioning", "observedGeneration": int64(1)},
		"metadata": map[string]any{
			"name": "prod", "resourceVersion": "1000", "generation": int64(1),
		},
	}
	edited := map[string]any{
		"spec":   map[string]any{"nodes": int64(5), "version": "1.31"},
		"status": map[string]any{"phase": "Provisioning", "observedGeneration": int64(1)},
		"metadata": map[string]any{
			"name": "prod", "resourceVersion": "1000", "generation": int64(1),
		},
	}
	// What the cluster did in the meantime: only its own bookkeeping.
	current := map[string]any{
		"spec":   map[string]any{"nodes": int64(3), "version": "1.31"},
		"status": map[string]any{"phase": "Ready", "observedGeneration": int64(2)},
		"metadata": map[string]any{
			"name": "prod", "resourceVersion": "1042", "generation": int64(2),
		},
	}

	mine := diffFields(opened, edited)
	theirs := withoutFields(diffFields(opened, current), serverFields)

	if clashes := conflictingPaths(mine, theirs); len(clashes) > 0 {
		t.Fatalf("refused the save over %v, but the controller only touched its own fields", clashes)
	}

	applyFields(current, mine)
	spec := current["spec"].(map[string]any)
	if spec["nodes"] != int64(5) {
		t.Errorf("nodes = %v, want the edit to have landed", spec["nodes"])
	}
	if got := current["metadata"].(map[string]any)["resourceVersion"]; got != "1042" {
		t.Errorf("resourceVersion = %v, want the cluster's current one", got)
	}
	if got := current["status"].(map[string]any)["phase"]; got != "Ready" {
		t.Errorf("phase = %v, want the controller's write kept", got)
	}
}

// The guard the resourceVersion check exists for has to survive: two people
// editing the same field is exactly what must not be merged silently.
func TestTwoEditsToTheSameFieldStillCollide(t *testing.T) {
	opened := map[string]any{"spec": map[string]any{"nodes": int64(3)}}
	edited := map[string]any{"spec": map[string]any{"nodes": int64(5)}}
	current := map[string]any{"spec": map[string]any{"nodes": int64(9)}}

	mine := diffFields(opened, edited)
	theirs := withoutFields(diffFields(opened, current), serverFields)

	clashes := conflictingPaths(mine, theirs)
	if len(clashes) != 1 || clashes[0] != "spec.nodes" {
		t.Fatalf("clashes = %v, want exactly spec.nodes", clashes)
	}
}

// Neighbouring fields under one parent are not a collision. Treating them as
// one would refuse most real edits, since almost everything lives under spec.
func TestDifferentFieldsUnderTheSameParentDoNotCollide(t *testing.T) {
	opened := map[string]any{"spec": map[string]any{"nodes": int64(3), "version": "1.31"}}
	edited := map[string]any{"spec": map[string]any{"nodes": int64(5), "version": "1.31"}}
	current := map[string]any{"spec": map[string]any{"nodes": int64(3), "version": "1.32"}}

	mine := diffFields(opened, edited)
	theirs := withoutFields(diffFields(opened, current), serverFields)

	if clashes := conflictingPaths(mine, theirs); len(clashes) > 0 {
		t.Fatalf("clashes = %v, want none: one moved nodes and the other version", clashes)
	}
}

// Both arriving at the same value is agreement. Refusing it would be a
// conflict message about a save that would change nothing.
func TestTheSameChangeMadeTwiceIsNotAConflict(t *testing.T) {
	opened := map[string]any{"spec": map[string]any{"nodes": int64(3)}}
	edited := map[string]any{"spec": map[string]any{"nodes": int64(5)}}
	current := map[string]any{"spec": map[string]any{"nodes": int64(5)}}

	mine := diffFields(opened, edited)
	theirs := withoutFields(diffFields(opened, current), serverFields)

	if clashes := conflictingPaths(mine, theirs); len(clashes) > 0 {
		t.Fatalf("clashes = %v, want none: both set it to 5", clashes)
	}
}

func TestDiffRecordsARemovedKeyAsNil(t *testing.T) {
	opened := map[string]any{"metadata": map[string]any{
		"labels": map[string]any{"team": "infra", "tier": "gold"},
	}}
	edited := map[string]any{"metadata": map[string]any{
		"labels": map[string]any{"team": "infra"},
	}}

	patch := diffFields(opened, edited)
	labels := patch["metadata"].(map[string]any)["labels"].(map[string]any)
	value, present := labels["tier"]
	if !present || value != nil {
		t.Fatalf("tier = %v (present %v), want an explicit nil so the key is removed", value, present)
	}

	applyFields(opened, patch)
	got := opened["metadata"].(map[string]any)["labels"].(map[string]any)
	if _, still := got["tier"]; still {
		t.Error("tier survived a patch that removed it")
	}
}

// A list is replaced whole rather than merged element by element. A merge patch
// cannot express "the third element changed", and guessing is how the wrong
// container gets rewritten.
func TestListsAreReplacedWholeRatherThanMerged(t *testing.T) {
	opened := map[string]any{"spec": map[string]any{"pools": []any{"a", "b"}}}
	edited := map[string]any{"spec": map[string]any{"pools": []any{"a", "b", "c"}}}

	patch := diffFields(opened, edited)
	pools := patch["spec"].(map[string]any)["pools"]
	if !reflect.DeepEqual(pools, []any{"a", "b", "c"}) {
		t.Fatalf("pools = %v, want the whole new list", pools)
	}
}

// Two people editing the same list is a collision, precisely because the list
// cannot be merged: one of the two versions would be lost without a word.
func TestTwoEditsToTheSameListCollide(t *testing.T) {
	opened := map[string]any{"spec": map[string]any{"pools": []any{"a"}}}
	edited := map[string]any{"spec": map[string]any{"pools": []any{"a", "mine"}}}
	current := map[string]any{"spec": map[string]any{"pools": []any{"a", "theirs"}}}

	mine := diffFields(opened, edited)
	theirs := withoutFields(diffFields(opened, current), serverFields)

	clashes := conflictingPaths(mine, theirs)
	if len(clashes) != 1 || clashes[0] != "spec.pools" {
		t.Fatalf("clashes = %v, want spec.pools", clashes)
	}
}

// Dropping resourceVersion must not leave an empty metadata behind that reads
// as a change to metadata itself.
func TestRemovingBookkeepingTakesTheEmptyParentWithIt(t *testing.T) {
	patch := map[string]any{
		"metadata": map[string]any{"resourceVersion": "7", "generation": int64(2)},
		"status":   map[string]any{"phase": "Ready"},
	}

	got := withoutFields(patch, serverFields)
	if len(got) != 0 {
		t.Fatalf("patch = %v, want nothing left once the cluster's own fields are dropped", got)
	}
}

// A label the controller adds is not bookkeeping, and has to keep colliding
// when the user changed the same one.
func TestAnAnnotationTheClusterChangedStillCounts(t *testing.T) {
	opened := map[string]any{"metadata": map[string]any{
		"annotations": map[string]any{"note": "one"}, "resourceVersion": "1",
	}}
	edited := map[string]any{"metadata": map[string]any{
		"annotations": map[string]any{"note": "mine"}, "resourceVersion": "1",
	}}
	current := map[string]any{"metadata": map[string]any{
		"annotations": map[string]any{"note": "theirs"}, "resourceVersion": "2",
	}}

	mine := diffFields(opened, edited)
	theirs := withoutFields(diffFields(opened, current), serverFields)

	clashes := conflictingPaths(mine, theirs)
	if len(clashes) != 1 || clashes[0] != "metadata.annotations.note" {
		t.Fatalf("clashes = %v, want metadata.annotations.note", clashes)
	}
}

// A Secret is edited in its decoded form, so a replayed edit has to be applied
// to the decoded object too. Applied to the raw one, removing a key would land
// on a stringData field that is not there while the base64 it meant to remove
// stayed exactly where it was -- a deletion that silently did nothing.
func TestRemovingASecretKeyReplaysOntoTheDecodedObject(t *testing.T) {
	opened := map[string]any{
		"metadata":   map[string]any{"name": "creds", "resourceVersion": "1"},
		"stringData": map[string]any{"username": "admin", "retired": "old"},
	}
	edited := map[string]any{
		"metadata":   map[string]any{"name": "creds", "resourceVersion": "1"},
		"stringData": map[string]any{"username": "admin"},
	}
	// forEditing's view of the object after the cluster wrote to it.
	fresh := map[string]any{
		"metadata":   map[string]any{"name": "creds", "resourceVersion": "9"},
		"stringData": map[string]any{"username": "admin", "retired": "old"},
	}

	mine := diffFields(opened, edited)
	theirs := withoutFields(diffFields(opened, fresh), serverFields)
	if clashes := conflictingPaths(mine, theirs); len(clashes) > 0 {
		t.Fatalf("clashes = %v, want none: only resourceVersion moved", clashes)
	}

	applyFields(fresh, mine)
	text := fresh["stringData"].(map[string]any)
	if _, still := text["retired"]; still {
		t.Error("the removed key came back: the deletion did not survive the replay")
	}
	if text["username"] != "admin" {
		t.Errorf("username = %v, want it kept", text["username"])
	}
	if got := fresh["metadata"].(map[string]any)["resourceVersion"]; got != "9" {
		t.Errorf("resourceVersion = %v, want the cluster's current one", got)
	}
}
