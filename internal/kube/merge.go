package kube

// Replaying an edit onto an object the cluster has moved on from.
//
// A save is an Update carrying the resourceVersion the editor opened with, so
// that two people editing the same object cannot silently overwrite one
// another. That guard is right, and it is also what made anything with a
// controller behind it painful to edit: a controller writing status bumps
// resourceVersion every few seconds, so the version in the editor goes stale
// long before anyone finishes typing, and the save is refused over a change
// that never touched what was being edited.
//
// So a refused save is no longer the end of it. The object is read again and
// the question that actually matters is asked: did the cluster change anything
// this edit also changed? If not -- the ordinary case, where a controller moved
// status and the person moved spec -- the edit is replayed onto the current
// object and sent again. If so, the save still fails, now naming the fields
// that genuinely disagree rather than a resourceVersion nobody was looking at.
//
// The diffs are JSON merge patches (RFC 7386) over the decoded documents: a map
// of what changed, with nil meaning removed. That is the shape kubectl computes
// for an edit, and it is small enough to compute here rather than take a
// dependency for.

import (
	"reflect"
	"sort"
	"strings"
)

// serverFields are the parts of an object the cluster maintains for itself.
//
// A change to one of these is never a reason to refuse an edit. resourceVersion
// moves on every write to the object, generation on every spec change the
// server accepts, managedFields on every write by anyone, and status belongs to
// whatever controller owns the object rather than to the person editing it.
// Ignoring them is what makes the retry worth attempting at all: on a live
// object they are almost always the only things that moved.
var serverFields = [][]string{
	{"status"},
	{"metadata", "resourceVersion"},
	{"metadata", "generation"},
	{"metadata", "managedFields"},
}

// diffFields is what turns `from` into `to`: a JSON merge patch, in which a nil
// value means the key was removed.
//
// Nested maps are descended into rather than replaced wholesale, which is the
// whole point: an edit to spec.replicas has to read as a change to that one
// field, not as a change to the whole of spec, or every edit would collide with
// every other.
func diffFields(from, to map[string]any) map[string]any {
	patch := map[string]any{}

	for key, want := range to {
		have, present := from[key]
		if present && reflect.DeepEqual(have, want) {
			continue
		}
		haveMap, haveIsMap := have.(map[string]any)
		wantMap, wantIsMap := want.(map[string]any)
		if present && haveIsMap && wantIsMap {
			if inner := diffFields(haveMap, wantMap); len(inner) > 0 {
				patch[key] = inner
			}
			continue
		}
		patch[key] = want
	}

	// Lists are compared whole and never descended into. A merge patch cannot
	// express "the third element changed", and guessing which elements
	// correspond -- containers by name, ports by number, and no rule at all for
	// a custom resource's own lists -- is exactly the kind of guess that
	// silently writes the wrong thing.
	for key := range from {
		if _, present := to[key]; !present {
			patch[key] = nil
		}
	}
	return patch
}

// applyFields writes a merge patch into a document, in place.
func applyFields(target, patch map[string]any) {
	for key, value := range patch {
		if value == nil {
			delete(target, key)
			continue
		}
		patchMap, patchIsMap := value.(map[string]any)
		targetMap, targetIsMap := target[key].(map[string]any)
		if patchIsMap && targetIsMap {
			applyFields(targetMap, patchMap)
			continue
		}
		target[key] = value
	}
}

// withoutFields removes the given paths from a merge patch, so that a change
// the cluster made to its own bookkeeping does not read as a change at all.
func withoutFields(patch map[string]any, paths [][]string) map[string]any {
	for _, path := range paths {
		removePath(patch, path)
	}
	return patch
}

// removePath deletes one nested key, and any parent left empty by the deletion:
// a metadata entry holding nothing but a dropped resourceVersion is not a
// change to metadata.
func removePath(fields map[string]any, path []string) {
	if len(path) == 0 {
		return
	}
	if len(path) == 1 {
		delete(fields, path[0])
		return
	}
	nested, ok := fields[path[0]].(map[string]any)
	if !ok {
		return
	}
	removePath(nested, path[1:])
	if len(nested) == 0 {
		delete(fields, path[0])
	}
}

// conflictingPaths lists the fields two merge patches both change, in dotted
// form, unless they happen to change them to the same thing.
//
// Two patches touching the same key are only a conflict at the leaves. Both
// changing something under `spec` is ordinary; both changing `spec.replicas`,
// to different values, is the thing worth stopping for.
func conflictingPaths(mine, theirs map[string]any) []string {
	found := collide(mine, theirs, "")
	sort.Strings(found)
	return found
}

func collide(mine, theirs map[string]any, prefix string) []string {
	var out []string
	for key, mineValue := range mine {
		theirsValue, contested := theirs[key]
		if !contested {
			continue
		}
		path := key
		if prefix != "" {
			path = prefix + "." + key
		}
		mineMap, mineIsMap := mineValue.(map[string]any)
		theirsMap, theirsIsMap := theirsValue.(map[string]any)
		if mineIsMap && theirsIsMap {
			out = append(out, collide(mineMap, theirsMap, path)...)
			continue
		}
		// Landing on the same value is agreement, not a conflict -- two people
		// scaling to 3 have not disagreed about anything.
		if reflect.DeepEqual(mineValue, theirsValue) {
			continue
		}
		out = append(out, path)
	}
	return out
}

// listPaths renders conflicting field names for a message, capped so that an
// edit colliding with a rewrite of the whole object does not produce an error
// nobody can read.
func listPaths(paths []string) string {
	const most = 5
	if len(paths) > most {
		return strings.Join(paths[:most], ", ") + ", and " +
			plural(int64(len(paths)-most), "other field")
	}
	return strings.Join(paths, ", ")
}
