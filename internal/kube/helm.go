package kube

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// Helm releases, which are not a Kubernetes kind at all.
//
// Helm 3 keeps each revision of a release as a Secret whose payload is the
// release JSON, gzipped and base64-encoded -- inside the base64 the API server
// already applies to every Secret value. So this reads Secrets and decodes them
// rather than watching a resource.
//
// It is a one-shot read rather than a watch, and deliberately: that payload
// carries the rendered manifest and the chart values, which routinely hold
// credentials, and the informer cache is exactly where those must not sit. The
// decode keeps six summary fields and drops everything else on the floor before
// returning. Releases change when someone deploys, so nothing is lost by not
// streaming them.

// HelmReleaseSecretType marks a Secret as one of Helm 3's release records.
const HelmReleaseSecretType = "helm.sh/release.v1"

// maxReleasePayload caps how much a single release may decompress to. A gzip
// stream can claim to be far larger than it is, and nothing here should be able
// to exhaust memory because a cluster held an unusual object.
const maxReleasePayload = 8 << 20

// HelmRelease is the summary of one release: everything shown in the table, and
// nothing else from the payload.
type HelmRelease struct {
	Name       string
	Namespace  string
	Revision   int64
	Status     string
	Chart      string
	AppVersion string
	Updated    string
}

// releaseJSON is the subset of Helm's release record worth reading. Every other
// field -- config, manifest, hooks -- is deliberately absent, so decoding
// cannot retain them even by accident.
type releaseJSON struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Version   int64  `json:"version"`
	Info      struct {
		Status       string `json:"status"`
		LastDeployed string `json:"last_deployed"`
	} `json:"info"`
	Chart struct {
		Metadata struct {
			Name       string `json:"name"`
			Version    string `json:"version"`
			AppVersion string `json:"appVersion"`
		} `json:"metadata"`
	} `json:"chart"`
}

// decodeHelmRelease turns one release Secret into its summary.
func decodeHelmRelease(u *unstructured.Unstructured) (HelmRelease, error) {
	if nestedString(u, "type") != HelmReleaseSecretType {
		return HelmRelease{}, errors.New("not a Helm release secret")
	}

	encoded := nestedString(u, "data", "release")
	if encoded == "" {
		return HelmRelease{}, errors.New("release secret has no payload")
	}

	// Two layers of base64: the API server's, then Helm's own.
	outer, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return HelmRelease{}, fmt.Errorf("decoding release: %w", err)
	}
	payload, err := base64.StdEncoding.DecodeString(string(outer))
	if err != nil {
		// Older Helm writes the gzip stream without the inner encoding.
		payload = outer
	}

	raw, err := gunzip(payload)
	if err != nil {
		return HelmRelease{}, err
	}

	var record releaseJSON
	if err := json.Unmarshal(raw, &record); err != nil {
		return HelmRelease{}, fmt.Errorf("parsing release: %w", err)
	}

	chart := record.Chart.Metadata.Name
	if v := record.Chart.Metadata.Version; v != "" {
		chart += "-" + v
	}
	namespace := record.Namespace
	if namespace == "" {
		namespace = u.GetNamespace()
	}

	return HelmRelease{
		Name:       record.Name,
		Namespace:  namespace,
		Revision:   record.Version,
		Status:     record.Info.Status,
		Chart:      chart,
		AppVersion: record.Chart.Metadata.AppVersion,
		Updated:    record.Info.LastDeployed,
	}, nil
}

// gunzip decompresses a release payload, refusing one that claims to be absurd.
func gunzip(payload []byte) ([]byte, error) {
	zr, err := gzip.NewReader(bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("release is not gzipped: %w", err)
	}
	defer func() { _ = zr.Close() }()

	raw, err := io.ReadAll(io.LimitReader(zr, maxReleasePayload))
	if err != nil {
		return nil, fmt.Errorf("reading release: %w", err)
	}
	return raw, nil
}

// helmTable projects release secrets into the table the UI renders, keeping the
// current revision of each release.
//
// A release that cannot be decoded is skipped rather than failing the listing:
// one unreadable record -- an older Helm, a hand-edited secret -- should not
// cost the user the sight of every other release they have.
func helmTable(secrets []*unstructured.Unstructured) Table {
	current := map[string]HelmRelease{}
	for _, s := range secrets {
		release, err := decodeHelmRelease(s)
		if err != nil || release.Name == "" {
			continue
		}
		key := release.Namespace + "/" + release.Name
		if existing, seen := current[key]; !seen || release.Revision > existing.Revision {
			current[key] = release
		}
	}

	releases := make([]HelmRelease, 0, len(current))
	for _, r := range current {
		releases = append(releases, r)
	}
	sort.Slice(releases, func(i, j int) bool {
		if releases[i].Namespace != releases[j].Namespace {
			return releases[i].Namespace < releases[j].Namespace
		}
		return releases[i].Name < releases[j].Name
	})

	rows := make([]Row, 0, len(releases))
	for _, r := range releases {
		rows = append(rows, Row{
			ID:        rowID(KindHelmReleases, r.Namespace, r.Name),
			Name:      r.Name,
			Namespace: r.Namespace,
			Cells: []Cell{
				plain(r.Name),
				muted(r.Namespace),
				number(int(r.Revision)),
				status(r.Status),
				muted(r.Chart),
				muted(r.AppVersion),
				timeCell(parseTime(r.Updated)),
			},
		})
	}

	return Table{
		Kind:       KindHelmReleases,
		Columns:    []string{"Name", "Namespace", "Revision", "Status", "Chart", "App Version", "Updated"},
		Rows:       rows,
		Namespaced: true,
	}
}

// HelmReleases lists the releases installed in a cluster.
func (w *Watcher) HelmReleases(kc Context, namespace string) (Table, error) {
	var table Table
	err := w.withClient(kc, func(c *clusterClient) error {
		ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
		defer cancel()

		items, _, err := c.list(ctx, KindSecrets, metav1.ListOptions{
			// Asking the API server to filter means the payloads of unrelated
			// secrets never cross the wire at all.
			FieldSelector: "type=" + HelmReleaseSecretType,
		})
		if err != nil {
			return err
		}

		pointers := make([]*unstructured.Unstructured, 0, len(items))
		for i := range items {
			if namespace != AllNamespaces && items[i].GetNamespace() != namespace {
				continue
			}
			pointers = append(pointers, &items[i])
		}
		table = helmTable(pointers)
		return nil
	})
	return table, err
}
