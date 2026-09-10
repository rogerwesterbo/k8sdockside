package kube

import (
	"encoding/json"
	"strings"
	"testing"

	authzv1 "k8s.io/api/authorization/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestBuildAccessModelCarriesBindingsToTheirRoles(t *testing.T) {
	roles := &rbacv1.RoleList{Items: []rbacv1.Role{{
		ObjectMeta: metav1.ObjectMeta{Name: "reader", Namespace: "shop"},
		Rules:      []rbacv1.PolicyRule{{Verbs: []string{"get", "list"}, APIGroups: []string{""}, Resources: []string{"pods"}}},
	}}}
	clusterRoles := &rbacv1.ClusterRoleList{Items: []rbacv1.ClusterRole{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "view", Labels: map[string]string{bootstrapLabel: "rbac-defaults"}},
			// A controller fills its rules in, which is what makes it aggregated.
			AggregationRule: &rbacv1.AggregationRule{},
		},
	}}
	bindings := &rbacv1.RoleBindingList{Items: []rbacv1.RoleBinding{{
		ObjectMeta: metav1.ObjectMeta{Name: "readers", Namespace: "shop"},
		RoleRef:    rbacv1.RoleRef{Kind: "Role", Name: "reader"},
		Subjects: []rbacv1.Subject{
			{Kind: "User", Name: "alice", Namespace: "ignored"},
			// No namespace: the binding's own is what the authorizer uses.
			{Kind: "ServiceAccount", Name: "robot"},
		},
	}}}
	clusterBinds := &rbacv1.ClusterRoleBindingList{Items: []rbacv1.ClusterRoleBinding{{
		ObjectMeta: metav1.ObjectMeta{Name: "everyone-views"},
		RoleRef:    rbacv1.RoleRef{Kind: "ClusterRole", Name: "view"},
		Subjects:   []rbacv1.Subject{{Kind: "Group", Name: "devs"}},
	}}}

	m := BuildAccessModel(roles, clusterRoles, bindings, clusterBinds, nil)

	if len(m.Roles) != 2 || m.Roles[0].Kind != "ClusterRole" || m.Roles[1].Kind != "Role" {
		t.Fatalf("roles = %+v, want the cluster role first", m.Roles)
	}
	if !m.Roles[0].Aggregated || !m.Roles[0].Default {
		t.Errorf("view = %+v, want aggregated and default", m.Roles[0])
	}
	if len(m.Bindings) != 2 || m.Bindings[0].Kind != "ClusterRoleBinding" {
		t.Fatalf("bindings = %+v, want the cluster binding first", m.Bindings)
	}

	rb := m.Bindings[1]
	if rb.RoleKind != "Role" || rb.RoleName != "reader" || rb.Namespace != "shop" {
		t.Errorf("role binding = %+v", rb)
	}
	if rb.Subjects[0].Namespace != "" {
		t.Errorf("a user carries no namespace, got %q", rb.Subjects[0].Namespace)
	}
	if rb.Subjects[1].Namespace != "shop" {
		t.Errorf("service account namespace = %q, want the binding's own", rb.Subjects[1].Namespace)
	}
}

// The frontend reads every list without a nil check, so an unreadable list and
// an empty rule both have to arrive as [] rather than null.
func TestBuildAccessModelHasNoNulls(t *testing.T) {
	clusterRoles := &rbacv1.ClusterRoleList{Items: []rbacv1.ClusterRole{{
		ObjectMeta: metav1.ObjectMeta{Name: "odd"},
		Rules:      []rbacv1.PolicyRule{{Verbs: []string{"get"}}},
	}}}

	m := BuildAccessModel(nil, clusterRoles, nil, nil, nil)
	raw, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "null") {
		t.Errorf("model has a null in it: %s", raw)
	}
}

func TestReviewRulesFlattensBothKinds(t *testing.T) {
	rules := reviewRules(authzv1.SubjectRulesReviewStatus{
		ResourceRules:    []authzv1.ResourceRule{{Verbs: []string{"get"}, APIGroups: []string{""}, Resources: []string{"pods"}}},
		NonResourceRules: []authzv1.NonResourceRule{{Verbs: []string{"get"}, NonResourceURLs: []string{"/healthz"}}},
	})

	if len(rules) != 2 {
		t.Fatalf("rules = %+v, want two", rules)
	}
	if rules[1].NonResourceURLs[0] != "/healthz" || len(rules[1].Resources) != 0 {
		t.Errorf("non-resource rule = %+v", rules[1])
	}
}
