package kube

// Reading one Helm release in full: the values it was installed with, the notes
// it printed, the objects it rendered, and the revisions behind it.
//
// helm.go decodes six summary fields out of a release Secret and drops the rest
// on the floor, because a table of releases has no business holding a manifest.
// A drawer opened on one release is the opposite case: that payload is exactly
// what was asked for.
//
// The caution the listing exercises is kept where it belongs, which is the
// cache rather than the screen. Everything here is read on demand, handed to
// the caller and forgotten. Showing one release's values to the person who
// opened it is a different thing from keeping every release's resident in an
// informer for the life of the app -- and anyone who can open this drawer can
// already read the Secret it comes from.

import (
	"context"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	sigsyaml "sigs.k8s.io/yaml"
)

// The labels Helm 3 stamps on every release Secret it writes, from its own
// storage driver. Selecting on them is what makes reading one release's history
// a narrow call rather than a listing of every release in the namespace.
const (
	helmOwnerLabel   = "owner"
	helmNameLabel    = "name"
	helmVersionLabel = "version"
	helmOwner        = "helm"
)

// maxRevisions caps how much history is read back.
//
// Helm keeps ten revisions by default and prunes as it goes, but the limit is
// configurable and a long-lived release on a cluster that raised it carries
// however many it was told to. Each one is a gzipped payload that has to be
// decompressed to be read at all, so the list is cut to the newest before any
// of that happens -- which is the half anyone reading a history is looking at.
const maxRevisions = 50

// HelmResource is one object a release rendered, as listed in the drawer.
//
// Identity only: what the release owns is the question being answered here, and
// the objects themselves are a tab away in their own kinds.
type HelmResource struct {
	Kind       string `json:"kind"`
	APIVersion string `json:"apiVersion"`
	Name       string `json:"name"`
	Namespace  string `json:"namespace"`
}

// HelmRevision is one entry in a release's history.
type HelmRevision struct {
	Revision   int64  `json:"revision"`
	Status     string `json:"status"`
	Chart      string `json:"chart"`
	AppVersion string `json:"appVersion"`
	Updated    string `json:"updated"`
	// Description is Helm's own log entry for the revision: "Install complete",
	// "Upgrade complete", "Rollback to 3". It is the column that makes a
	// history readable, because the rest of the row repeats itself.
	Description string `json:"description"`
	// Current marks the revision the release is on, which is the one there is
	// no point offering to roll back to.
	Current bool `json:"current"`
}

// HelmReleaseDetail is one release read in full, for the drawer.
type HelmReleaseDetail struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Revision  int64  `json:"revision"`
	Status    string `json:"status"`
	// Chart is the "name-version" string the table shows. The two halves are
	// carried separately as well, because an upgrade needs to ask the repo
	// about the chart by name and offer its versions.
	Chart         string `json:"chart"`
	ChartName     string `json:"chartName"`
	ChartVersion  string `json:"chartVersion"`
	AppVersion    string `json:"appVersion"`
	Description   string `json:"description"`
	FirstDeployed string `json:"firstDeployed"`
	Updated       string `json:"updated"`
	// Notes is the rendered NOTES.txt, which is the one part of a chart written
	// to be read by a person and is otherwise only ever seen once, scrolling
	// past at install time.
	Notes string `json:"notes"`
	// Values is the chart's defaults with the user's overrides merged over
	// them; UserValues is the overrides alone. Both as YAML, because that is
	// what a values file is and what an editor has to open on.
	//
	// The distinction is the drawer's "user-supplied values only" toggle, and
	// it is not cosmetic: the merged document is the one to read to understand
	// what the release is doing, and the overrides are the only half there is
	// any point editing -- an upgrade sends those, not the chart's own
	// defaults.
	Values     string         `json:"values"`
	UserValues string         `json:"userValues"`
	Resources  []HelmResource `json:"resources"`
	Revisions  []HelmRevision `json:"revisions"`
}

// detailJSON is the whole of Helm's release record that this reads -- which,
// unlike releaseJSON next door, is most of it.
//
// The two values documents are kept as raw JSON rather than decoded into maps.
// That is load bearing rather than fussy: decoding a JSON number into an `any`
// makes it a float64, and a values file carrying an integer larger than 2^53 --
// an id, a byte count -- would come back out of the round trip as a different
// number than the one the user wrote.
type detailJSON struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Version   int64  `json:"version"`
	Info      struct {
		Status        string `json:"status"`
		FirstDeployed string `json:"first_deployed"`
		LastDeployed  string `json:"last_deployed"`
		Description   string `json:"description"`
		Notes         string `json:"notes"`
	} `json:"info"`
	Chart struct {
		Metadata struct {
			Name       string `json:"name"`
			Version    string `json:"version"`
			AppVersion string `json:"appVersion"`
		} `json:"metadata"`
		Values jsontext.Value `json:"values"`
	} `json:"chart"`
	Config   jsontext.Value `json:"config"`
	Manifest string         `json:"manifest"`
}

// decodeHelmDetail reads one release Secret into everything the drawer shows.
func decodeHelmDetail(u *unstructured.Unstructured) (HelmReleaseDetail, error) {
	raw, err := releasePayload(u)
	if err != nil {
		return HelmReleaseDetail{}, err
	}

	var record detailJSON
	if err := json.Unmarshal(raw, &record); err != nil {
		return HelmReleaseDetail{}, fmt.Errorf("parsing release: %w", err)
	}

	namespace := record.Namespace
	if namespace == "" {
		namespace = u.GetNamespace()
	}
	chart := record.Chart.Metadata.Name
	if v := record.Chart.Metadata.Version; v != "" {
		chart += "-" + v
	}

	return HelmReleaseDetail{
		Name:          record.Name,
		Namespace:     namespace,
		Revision:      record.Version,
		Status:        record.Info.Status,
		Chart:         chart,
		ChartName:     record.Chart.Metadata.Name,
		ChartVersion:  record.Chart.Metadata.Version,
		AppVersion:    record.Chart.Metadata.AppVersion,
		Description:   record.Info.Description,
		FirstDeployed: record.Info.FirstDeployed,
		Updated:       record.Info.LastDeployed,
		Notes:         record.Info.Notes,
		Values:        valuesYAML(mergeValues(record.Chart.Values, record.Config)),
		UserValues:    valuesYAML(record.Config),
		Resources:     manifestResources(record.Manifest),
		Revisions:     []HelmRevision{},
	}, nil
}

// mergeValues lays the user's overrides over the chart's defaults.
//
// Objects on both sides merge key by key; anything else -- a scalar, a list --
// is replaced whole, which is Helm's own rule: a list in a values file is not
// appended to, it is swapped out.
//
// It is close to what `helm get values --all` prints rather than identical to
// it. Helm's coalescing also deletes a key whose override is null and scopes
// subchart values as it descends; this keeps an explicit null visible, because
// a reader looking at the merged document is better served seeing that
// something was deliberately unset than not seeing it at all.
func mergeValues(defaults, override jsontext.Value) jsontext.Value {
	if len(override) == 0 {
		return defaults
	}
	if len(defaults) == 0 {
		return override
	}
	// Two scalars, a list against an object, or anything else that is not two
	// objects: there is nothing to merge inside, so the override wins whole.
	if defaults.Kind() != '{' || override.Kind() != '{' {
		return override
	}

	var base, over map[string]jsontext.Value
	if err := json.Unmarshal(defaults, &base); err != nil {
		return override
	}
	if err := json.Unmarshal(override, &over); err != nil {
		return defaults
	}

	for key, value := range over {
		if existing, found := base[key]; found {
			base[key] = mergeValues(existing, value)
			continue
		}
		base[key] = value
	}

	merged, err := json.Marshal(base)
	if err != nil {
		return override
	}
	return merged
}

// valuesYAML renders a raw values document as the YAML an editor opens on.
//
// Converted from the JSON directly rather than marshalled from a decoded map,
// for the reason detailJSON keeps them raw: a number is written back as it was
// stored rather than as whatever a float64 prints as.
func valuesYAML(v jsontext.Value) string {
	if len(v) == 0 {
		return ""
	}
	out, err := sigsyaml.JSONToYAML(v)
	if err != nil {
		return ""
	}

	text := strings.TrimSpace(string(out))
	// A release installed with no overrides stores an empty object, which
	// renders as "{}" -- a value, where what is meant is the absence of one.
	if text == "{}" || text == "null" {
		return ""
	}
	return text + "\n"
}

// manifestDoc is the identity of one object in a rendered manifest, and nothing
// else about it.
type manifestDoc struct {
	APIVersion string `yaml:"apiVersion"`
	Kind       string `yaml:"kind"`
	Metadata   struct {
		Name      string `yaml:"name"`
		Namespace string `yaml:"namespace"`
	} `yaml:"metadata"`
}

// manifestResources lists the objects a release rendered.
//
// A manifest is a stream of YAML documents, and a chart routinely renders empty
// ones -- a template guarded by an `if` that was false leaves its separator
// behind. Those are skipped rather than counted, along with anything that
// carries no kind or no name.
func manifestResources(manifest string) []HelmResource {
	out := []HelmResource{}
	if strings.TrimSpace(manifest) == "" {
		return out
	}

	// Charts occasionally render the same object twice -- a subchart and its
	// parent both declaring a ServiceAccount, say. Helm's own apply is
	// idempotent about it; a list that showed it twice would just read as a
	// mistake.
	seen := map[string]bool{}

	decoder := yaml.NewDecoder(strings.NewReader(manifest))
	for {
		var doc manifestDoc
		// Stops at the end of the stream, and also at the first document this
		// cannot read: go-yaml gives no way to resynchronise after a syntax
		// error, so what is listed is what could be read up to that point.
		if err := decoder.Decode(&doc); err != nil {
			break
		}
		if doc.Kind == "" || doc.Metadata.Name == "" {
			continue
		}

		key := doc.APIVersion + "/" + doc.Kind + "/" + doc.Metadata.Namespace + "/" + doc.Metadata.Name
		if seen[key] {
			continue
		}
		seen[key] = true

		out = append(out, HelmResource{
			Kind:       doc.Kind,
			APIVersion: doc.APIVersion,
			Name:       doc.Metadata.Name,
			Namespace:  doc.Metadata.Namespace,
		})
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].Kind != out[j].Kind {
			return out[i].Kind < out[j].Kind
		}
		return out[i].Name < out[j].Name
	})
	return out
}

// scopeNamespaces fills in the namespace of every rendered object that has one.
//
// A chart's templates usually do not write a namespace: Helm applies them into
// the release's namespace, and the manifest simply does not say so. Listing
// every object as namespaceless would be true to the document and useless to
// the reader; defaulting them all to the release's namespace would put one on
// the ClusterRoles. So the mapper is asked which of the two each kind is, once
// per kind rather than once per object.
//
// A kind the cluster does not serve is left as the manifest wrote it. That is a
// real case rather than a defensive one: a release that installs its own CRDs
// renders objects of those kinds, and on a first read they may not be
// established yet.
func scopeNamespaces(c *clusterClient, resources []HelmResource, releaseNamespace string) {
	namespaced := map[schema.GroupKind]bool{}

	for i := range resources {
		if resources[i].Namespace != "" {
			continue
		}
		gv, err := schema.ParseGroupVersion(resources[i].APIVersion)
		if err != nil {
			continue
		}

		gk := gv.WithKind(resources[i].Kind).GroupKind()
		scoped, asked := namespaced[gk]
		if !asked {
			mapping, err := c.mappingFor(gk)
			// Recorded either way. A failed lookup resets the discovery cache
			// on its way out, so asking again for every object of a kind the
			// cluster does not serve would be expensive as well as pointless.
			scoped = err == nil && mapping.Scope.Name() == meta.RESTScopeNameNamespace
			namespaced[gk] = scoped
		}
		if scoped {
			resources[i].Namespace = releaseNamespace
		}
	}
}

// helmReleaseSelector narrows a Secret listing to one release's revisions.
func helmReleaseSelector(name string) string {
	return helmOwnerLabel + "=" + helmOwner + "," + helmNameLabel + "=" + name
}

// revisionNumber reads a release Secret's revision from the label Helm writes
// alongside the payload, so the newest revisions can be picked out before
// anything is decompressed. A Secret without one sorts last, which is where a
// hand-made record belongs.
func revisionNumber(u *unstructured.Unstructured) int64 {
	n, err := strconv.ParseInt(u.GetLabels()[helmVersionLabel], 10, 64)
	if err != nil {
		return 0
	}
	return n
}

// readRevisions orders a release's Secrets newest first and summarises them,
// returning the current revision's Secret alongside the history.
//
// The summary decode is the table's, deliberately: a history row needs the six
// fields it keeps and none of the payload it drops, so only the one revision
// actually being opened is ever read in full.
func readRevisions(items []unstructured.Unstructured) (*unstructured.Unstructured, []HelmRevision) {
	if len(items) == 0 {
		return nil, []HelmRevision{}
	}

	sort.Slice(items, func(i, j int) bool {
		return revisionNumber(&items[i]) > revisionNumber(&items[j])
	})
	if len(items) > maxRevisions {
		items = items[:maxRevisions]
	}

	current := &items[0]
	highest := revisionNumber(current)

	revisions := make([]HelmRevision, 0, len(items))
	for i := range items {
		release, err := decodeHelmRelease(&items[i])
		if err != nil {
			continue
		}
		revisions = append(revisions, HelmRevision{
			Revision:    release.Revision,
			Status:      release.Status,
			Chart:       release.Chart,
			AppVersion:  release.AppVersion,
			Updated:     release.Updated,
			Description: release.Description,
			Current:     release.Revision == highest,
		})
	}
	return current, revisions
}

// HelmReleaseDetail reads one release in full, with the history behind it.
func (w *Watcher) HelmReleaseDetail(kc Context, namespace, name string) (HelmReleaseDetail, error) {
	if namespace == "" || name == "" {
		return HelmReleaseDetail{}, fmt.Errorf("a Helm release is named by a namespace and a name")
	}

	var out HelmReleaseDetail
	err := w.withClient(kc, func(c *clusterClient) error {
		ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
		defer cancel()

		items, err := c.listIn(ctx, KindSecrets, namespace, metav1.ListOptions{
			// Asking the API server for one release's records means no other
			// release's payload crosses the wire at all.
			LabelSelector: helmReleaseSelector(name),
		})
		if err != nil {
			return err
		}

		current, revisions := readRevisions(items)
		if current == nil {
			return fmt.Errorf("no Helm release named %q in %s", name, namespace)
		}

		detail, err := decodeHelmDetail(current)
		if err != nil {
			return err
		}
		if detail.Namespace == "" {
			detail.Namespace = namespace
		}
		if detail.Name == "" {
			detail.Name = name
		}
		scopeNamespaces(c, detail.Resources, detail.Namespace)
		detail.Revisions = revisions

		out = detail
		return nil
	})
	return out, err
}
