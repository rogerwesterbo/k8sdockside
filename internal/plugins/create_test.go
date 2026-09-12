package plugins

import (
	"strings"
	"testing"
)

func creator(write bool, readable ...string) Plugin {
	return Plugin{ID: "p", Name: "P", UI: &UI{Write: write, Readable: readable}}
}

const rule = `{"apiVersion":"monitoring.coreos.com/v1","kind":"PrometheusRule","metadata":{"name":"mine","namespace":"elsewhere","resourceVersion":"12","uid":"u"},"spec":{"groups":[]},"status":{"x":1}}`

func TestNewObjectIsPutWhereTheRequestSays(t *testing.T) {
	p := creator(true, "crd:prometheusrules.monitoring.coreos.com")
	obj, err := p.NewObject("crd:prometheusrules.monitoring.coreos.com", "monitoring", rule)
	if err != nil {
		t.Fatal(err)
	}
	meta := obj["metadata"].(map[string]any)
	if meta["namespace"] != "monitoring" {
		t.Errorf("namespace = %v, want the request's", meta["namespace"])
	}
	for _, gone := range []string{"resourceVersion", "uid"} {
		if _, ok := meta[gone]; ok {
			t.Errorf("metadata.%s was kept; the server refuses a create carrying it", gone)
		}
	}
	if _, ok := obj["status"]; ok {
		t.Error("status was kept")
	}
}

func TestNewObjectRefuses(t *testing.T) {
	kind := "crd:prometheusrules.monitoring.coreos.com"
	cases := map[string]struct {
		plugin Plugin
		kind   string
		text   string
		want   string
	}{
		"undeclared kind": {creator(true), kind, rule, "does not declare"},
		"read only":       {creator(false, kind), kind, rule, "write"},
		"rbac":            {creator(true, "crd:roles.rbac.authorization.k8s.io"), "crd:roles.rbac.authorization.k8s.io", rule, "may not create"},
		"not json":        {creator(true, kind), kind, "kind: x", "not a JSON object"},
		"no kind":         {creator(true, kind), kind, `{"apiVersion":"v1","metadata":{"name":"a"}}`, "no kind"},
		"no name":         {creator(true, kind), kind, `{"apiVersion":"v1","kind":"X","metadata":{}}`, "neither metadata.name"},
		"no metadata":     {creator(true, kind), kind, `{"apiVersion":"v1","kind":"X"}`, "no metadata"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := c.plugin.NewObject(c.kind, "ns", c.text)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("err = %v, want it to mention %q", err, c.want)
			}
		})
	}
}

// The built-in Prometheus plugin's rule builder creates PrometheusRules, so
// the built-in must be allowed to.
func TestBuiltinPrometheusMayCreateRules(t *testing.T) {
	cat := Load(t.TempDir(), nil, nil)
	p, ok := cat.Find("prometheus")
	if !ok {
		t.Fatal("no prometheus built-in")
	}
	if _, err := p.NewObject("crd:prometheusrules.monitoring.coreos.com", "monitoring", rule); err != nil {
		t.Fatal(err)
	}
}
