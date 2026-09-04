package kube

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// helmSecret builds a release secret the way Helm 3 writes one: the JSON is
// gzipped, base64-encoded, and then stored in a Secret's data -- which the API
// base64-encodes again on the way out.
func helmSecret(t *testing.T, name string, payload string) *unstructured.Unstructured {
	t.Helper()
	var zipped bytes.Buffer
	gz := gzip.NewWriter(&zipped)
	if _, err := gz.Write([]byte(payload)); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	inner := base64.StdEncoding.EncodeToString(zipped.Bytes())

	return obj(map[string]any{
		"metadata": map[string]any{"name": name, "namespace": "prod"},
		"type":     HelmReleaseSecretType,
		"data":     map[string]any{"release": base64.StdEncoding.EncodeToString([]byte(inner))},
	})
}

const nginxRelease = `{
  "name": "ingress-nginx",
  "version": 7,
  "namespace": "prod",
  "info": {"status": "deployed", "last_deployed": "2026-08-01T10:00:00Z", "description": "Upgrade complete"},
  "chart": {"metadata": {"name": "ingress-nginx", "version": "4.11.3", "appVersion": "1.11.3"}}
}`

func TestHelmReleaseIsDecodedFromItsSecret(t *testing.T) {
	release, err := decodeHelmRelease(helmSecret(t, "sh.helm.release.v1.ingress-nginx.v7", nginxRelease))
	if err != nil {
		t.Fatalf("decodeHelmRelease: %v", err)
	}

	if release.Name != "ingress-nginx" {
		t.Errorf("name = %q, want ingress-nginx", release.Name)
	}
	if release.Revision != 7 {
		t.Errorf("revision = %d, want 7", release.Revision)
	}
	if release.Status != "deployed" {
		t.Errorf("status = %q, want deployed", release.Status)
	}
	if release.Chart != "ingress-nginx-4.11.3" {
		t.Errorf("chart = %q, want ingress-nginx-4.11.3", release.Chart)
	}
	if release.AppVersion != "1.11.3" {
		t.Errorf("app version = %q, want 1.11.3", release.AppVersion)
	}
}

// The reason this path exists at all: a release payload carries the rendered
// manifest and the chart values, which routinely hold credentials. Only the
// summary may survive the decode.
func TestDecodingKeepsNoPartOfTheReleasePayload(t *testing.T) {
	const withSecrets = `{
	  "name": "app", "version": 1,
	  "info": {"status": "deployed"},
	  "chart": {"metadata": {"name": "app", "version": "1.0.0"}},
	  "config": {"database": {"password": "hunter2"}},
	  "manifest": "apiVersion: v1\nkind: Secret\nstringData:\n  token: s3cret\n"
	}`

	release, err := decodeHelmRelease(helmSecret(t, "sh.helm.release.v1.app.v1", withSecrets))
	if err != nil {
		t.Fatalf("decodeHelmRelease: %v", err)
	}

	for _, field := range []string{release.Name, release.Status, release.Chart, release.AppVersion, release.Updated} {
		if field == "hunter2" || field == "s3cret" {
			t.Fatal("a decoded release carried part of its payload")
		}
	}
}

func TestOnlyTheCurrentRevisionOfEachReleaseIsListed(t *testing.T) {
	older := `{"name":"app","version":2,"info":{"status":"superseded"},"chart":{"metadata":{"name":"app","version":"1.0.0"}}}`
	newer := `{"name":"app","version":5,"info":{"status":"deployed"},"chart":{"metadata":{"name":"app","version":"1.2.0"}}}`

	table := helmTable([]*unstructured.Unstructured{
		helmSecret(t, "sh.helm.release.v1.app.v2", older),
		helmSecret(t, "sh.helm.release.v1.app.v5", newer),
	})

	if len(table.Rows) != 1 {
		t.Fatalf("rows = %d, want 1 -- a release has one current revision", len(table.Rows))
	}
	cells := cellsByHeader(t, table, 0)
	want(t, cells, "Revision", "5")
	want(t, cells, "Status", "deployed")
	want(t, cells, "Chart", "app-1.2.0")
}

func TestASecretThatIsNotAReleaseIsIgnored(t *testing.T) {
	notARelease := obj(map[string]any{
		"metadata": map[string]any{"name": "tls", "namespace": "prod"},
		"type":     "kubernetes.io/tls",
		"data":     map[string]any{"tls.crt": "aGk="},
	})

	if _, err := decodeHelmRelease(notARelease); err == nil {
		t.Error("a non-release secret decoded without complaint")
	}
}

func TestAnUndecodableReleaseIsSkippedRatherThanFailingTheTable(t *testing.T) {
	broken := obj(map[string]any{
		"metadata": map[string]any{"name": "sh.helm.release.v1.bad.v1", "namespace": "prod"},
		"type":     HelmReleaseSecretType,
		"data":     map[string]any{"release": "not base64 at all !!"},
	})
	good := helmSecret(t, "sh.helm.release.v1.ingress-nginx.v7", nginxRelease)

	table := helmTable([]*unstructured.Unstructured{broken, good})

	// One bad release must not cost the user the rest of the list.
	if len(table.Rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(table.Rows))
	}
}
