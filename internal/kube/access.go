// Reading who may do what in a cluster: the RBAC objects laid out for the
// access overview, and the two questions a person can always ask about
// themselves -- who the cluster thinks they are, and what it lets them do.
//
// The overview draws a graph from subjects through bindings to roles, and the
// work of drawing it -- which role a binding points at, what a rule matches --
// is done in the frontend from the plain lists returned here. What this file
// does is read them, tolerate the parts the caller is not allowed to read, and
// hand back something with no nils in it.
package kube

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"

	authnv1 "k8s.io/api/authentication/v1"
	authzv1 "k8s.io/api/authorization/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// bootstrapLabel marks the roles and bindings the API server creates for
// itself. They are most of what a fresh cluster holds, and the overview hides
// them unless asked.
const bootstrapLabel = "kubernetes.io/bootstrapping"

// AccessRule is one rule of a role, or one rule the cluster says the caller
// holds. Every slice is present, possibly empty.
type AccessRule struct {
	Verbs           []string `json:"verbs"`
	APIGroups       []string `json:"apiGroups"`
	Resources       []string `json:"resources"`
	ResourceNames   []string `json:"resourceNames"`
	NonResourceURLs []string `json:"nonResourceURLs"`
}

// AccessRole is a Role or a ClusterRole.
type AccessRole struct {
	// Kind is "Role" or "ClusterRole".
	Kind string `json:"kind"`
	Name string `json:"name"`
	// Namespace is empty for a ClusterRole.
	Namespace string       `json:"namespace"`
	Rules     []AccessRule `json:"rules"`
	// Aggregated is a ClusterRole whose rules a controller fills in from other
	// roles -- admin, edit and view are the famous ones. Its rules are still
	// listed here, as the controller left them.
	Aggregated bool `json:"aggregated"`
	// Default is one of the roles the API server bootstraps for itself.
	Default bool `json:"default"`
}

// AccessSubject is who a binding grants to.
type AccessSubject struct {
	// Kind is "User", "Group" or "ServiceAccount".
	Kind string `json:"kind"`
	Name string `json:"name"`
	// Namespace is set for a ServiceAccount and empty otherwise.
	Namespace string `json:"namespace"`
}

// AccessBinding is a RoleBinding or a ClusterRoleBinding.
type AccessBinding struct {
	// Kind is "RoleBinding" or "ClusterRoleBinding".
	Kind string `json:"kind"`
	Name string `json:"name"`
	// Namespace is empty for a ClusterRoleBinding, and is the one namespace a
	// RoleBinding grants in -- even when the role it names is a ClusterRole.
	Namespace string `json:"namespace"`
	// RoleKind is "Role" or "ClusterRole".
	RoleKind string          `json:"roleKind"`
	RoleName string          `json:"roleName"`
	Subjects []AccessSubject `json:"subjects"`
	Default  bool            `json:"default"`
}

// AccessAccount is one ServiceAccount, listed so the overview can tell a
// binding naming an account that exists from one naming a typo.
type AccessAccount struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
}

// Identity is who the cluster says the caller is.
type Identity struct {
	Username string   `json:"username"`
	Groups   []string `json:"groups"`
	// Error is why the cluster could not say, typically because it predates
	// SelfSubjectReview (Kubernetes 1.28) or has it switched off.
	Error string `json:"error,omitempty"`
}

// AccessModel is everything the access overview is drawn from.
type AccessModel struct {
	Roles           []AccessRole    `json:"roles"`
	Bindings        []AccessBinding `json:"bindings"`
	ServiceAccounts []AccessAccount `json:"serviceAccounts"`
	Me              Identity        `json:"me"`
	// Unreadable says which lists the cluster refused and why. Somebody
	// allowed to read bindings but not cluster roles still gets a picture;
	// this is how they are told where its holes are.
	Unreadable []string `json:"unreadable"`
	Error      string   `json:"error,omitempty"`
}

// MyRules is what the cluster says the caller may do in one namespace.
type MyRules struct {
	Namespace string       `json:"namespace"`
	Rules     []AccessRule `json:"rules"`
	// Incomplete is the cluster admitting the list is not the whole story --
	// most often because a webhook authorizer does not enumerate its rules.
	Incomplete      bool   `json:"incomplete"`
	EvaluationError string `json:"evaluationError,omitempty"`
	Error           string `json:"error,omitempty"`
}

// Access reads a cluster's RBAC objects and the caller's identity.
//
// Each list is read on its own and a refusal of one does not sink the rest:
// RBAC is exactly the area where people are allowed to see part of the
// picture. Only when every role and binding list fails is the call an error.
func (w *Watcher) Access(kc Context) (AccessModel, error) {
	out := emptyAccessModel()

	err := w.withClient(kc, func(c *clusterClient) error {
		ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
		defer cancel()

		var (
			roles        *rbacv1.RoleList
			clusterRoles *rbacv1.ClusterRoleList
			bindings     *rbacv1.RoleBindingList
			clusterBinds *rbacv1.ClusterRoleBindingList
			accounts     []AccessAccount
			me           Identity
			mu           sync.Mutex
			wg           sync.WaitGroup
			refused      []string
		)
		refuse := func(what string, err error) {
			mu.Lock()
			defer mu.Unlock()
			refused = append(refused, fmt.Sprintf("%s: %v", what, err))
		}
		rbac := c.typed.RbacV1()

		wg.Go(func() {
			l, err := rbac.Roles(AllNamespaces).List(ctx, metav1.ListOptions{})
			if err != nil {
				refuse("Roles", err)
				return
			}
			roles = l
		})
		wg.Go(func() {
			l, err := rbac.ClusterRoles().List(ctx, metav1.ListOptions{})
			if err != nil {
				refuse("ClusterRoles", err)
				return
			}
			clusterRoles = l
		})
		wg.Go(func() {
			l, err := rbac.RoleBindings(AllNamespaces).List(ctx, metav1.ListOptions{})
			if err != nil {
				refuse("RoleBindings", err)
				return
			}
			bindings = l
		})
		wg.Go(func() {
			l, err := rbac.ClusterRoleBindings().List(ctx, metav1.ListOptions{})
			if err != nil {
				refuse("ClusterRoleBindings", err)
				return
			}
			clusterBinds = l
		})
		wg.Go(func() {
			l, err := c.typed.CoreV1().ServiceAccounts(AllNamespaces).List(ctx, metav1.ListOptions{})
			if err != nil {
				refuse("ServiceAccounts", err)
				return
			}
			for i := range l.Items {
				accounts = append(accounts, AccessAccount{Name: l.Items[i].Name, Namespace: l.Items[i].Namespace})
			}
		})
		wg.Go(func() {
			me = c.whoAmI(ctx)
		})
		wg.Wait()

		sort.Strings(refused)
		out = BuildAccessModel(roles, clusterRoles, bindings, clusterBinds, accounts)
		out.Me = me
		if refused != nil {
			out.Unreadable = refused
		}
		if roles == nil && clusterRoles == nil && bindings == nil && clusterBinds == nil {
			return fmt.Errorf("the cluster let us read none of its roles or bindings (%s)", strings.Join(refused, "; "))
		}
		return nil
	})

	if err != nil {
		out.Error = err.Error()
	}
	return out, err
}

// MyAccess asks the cluster what the caller may do in one namespace.
//
// This goes through SelfSubjectRulesReview, which every authenticated user may
// create, so it answers even for somebody who can read no RBAC objects at all
// -- which is precisely who most needs the answer. The review requires a
// namespace; an empty one is read as "default".
func (w *Watcher) MyAccess(kc Context, namespace string) (MyRules, error) {
	if namespace == "" {
		namespace = "default"
	}
	out := MyRules{Namespace: namespace, Rules: []AccessRule{}}

	err := w.withClient(kc, func(c *clusterClient) error {
		ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
		defer cancel()

		review, err := c.typed.AuthorizationV1().SelfSubjectRulesReviews().Create(ctx,
			&authzv1.SelfSubjectRulesReview{Spec: authzv1.SelfSubjectRulesReviewSpec{Namespace: namespace}},
			metav1.CreateOptions{})
		if err != nil {
			return err
		}
		out.Rules = reviewRules(review.Status)
		out.Incomplete = review.Status.Incomplete
		out.EvaluationError = review.Status.EvaluationError
		return nil
	})

	if err != nil {
		out.Error = err.Error()
	}
	return out, err
}

// reviewRules flattens a rules review into the same rule shape roles use, so
// the frontend draws both with one matrix.
func reviewRules(status authzv1.SubjectRulesReviewStatus) []AccessRule {
	out := make([]AccessRule, 0, len(status.ResourceRules)+len(status.NonResourceRules))
	for _, r := range status.ResourceRules {
		out = append(out, AccessRule{
			Verbs:           nonNil(r.Verbs),
			APIGroups:       nonNil(r.APIGroups),
			Resources:       nonNil(r.Resources),
			ResourceNames:   nonNil(r.ResourceNames),
			NonResourceURLs: []string{},
		})
	}
	for _, r := range status.NonResourceRules {
		out = append(out, AccessRule{
			Verbs:           nonNil(r.Verbs),
			APIGroups:       []string{},
			Resources:       []string{},
			ResourceNames:   []string{},
			NonResourceURLs: nonNil(r.NonResourceURLs),
		})
	}
	return out
}

// whoAmI asks the cluster who the caller is. A failure is carried in the
// result rather than returned: not knowing your own name is no reason not to
// draw the graph.
func (c *clusterClient) whoAmI(ctx context.Context) Identity {
	review, err := c.typed.AuthenticationV1().SelfSubjectReviews().Create(ctx, &authnv1.SelfSubjectReview{}, metav1.CreateOptions{})
	if err != nil {
		return Identity{Groups: []string{}, Error: err.Error()}
	}
	return Identity{
		Username: review.Status.UserInfo.Username,
		Groups:   nonNil(review.Status.UserInfo.Groups),
	}
}

// BuildAccessModel turns the typed lists into the overview's model. Any list
// may be nil, meaning it could not be read. Exported for its tests.
func BuildAccessModel(
	roles *rbacv1.RoleList,
	clusterRoles *rbacv1.ClusterRoleList,
	bindings *rbacv1.RoleBindingList,
	clusterBinds *rbacv1.ClusterRoleBindingList,
	accounts []AccessAccount,
) AccessModel {
	out := emptyAccessModel()

	if roles != nil {
		for i := range roles.Items {
			r := &roles.Items[i]
			out.Roles = append(out.Roles, AccessRole{
				Kind:      "Role",
				Name:      r.Name,
				Namespace: r.Namespace,
				Rules:     accessRules(r.Rules),
				Default:   r.Labels[bootstrapLabel] != "",
			})
		}
	}
	if clusterRoles != nil {
		for i := range clusterRoles.Items {
			r := &clusterRoles.Items[i]
			out.Roles = append(out.Roles, AccessRole{
				Kind:       "ClusterRole",
				Name:       r.Name,
				Rules:      accessRules(r.Rules),
				Aggregated: r.AggregationRule != nil,
				Default:    r.Labels[bootstrapLabel] != "",
			})
		}
	}
	if bindings != nil {
		for i := range bindings.Items {
			b := &bindings.Items[i]
			out.Bindings = append(out.Bindings, AccessBinding{
				Kind:      "RoleBinding",
				Name:      b.Name,
				Namespace: b.Namespace,
				RoleKind:  b.RoleRef.Kind,
				RoleName:  b.RoleRef.Name,
				Subjects:  accessSubjects(b.Subjects, b.Namespace),
				Default:   b.Labels[bootstrapLabel] != "",
			})
		}
	}
	if clusterBinds != nil {
		for i := range clusterBinds.Items {
			b := &clusterBinds.Items[i]
			out.Bindings = append(out.Bindings, AccessBinding{
				Kind:     "ClusterRoleBinding",
				Name:     b.Name,
				RoleKind: b.RoleRef.Kind,
				RoleName: b.RoleRef.Name,
				Subjects: accessSubjects(b.Subjects, ""),
				Default:  b.Labels[bootstrapLabel] != "",
			})
		}
	}
	if accounts != nil {
		out.ServiceAccounts = accounts
	}

	// Cluster-wide first, then by namespace and name: the order the overview
	// lists them in, so the frontend has nothing to sort.
	sort.Slice(out.Roles, func(i, j int) bool {
		a, b := out.Roles[i], out.Roles[j]
		if a.Kind != b.Kind {
			return a.Kind == "ClusterRole"
		}
		if a.Namespace != b.Namespace {
			return a.Namespace < b.Namespace
		}
		return a.Name < b.Name
	})
	sort.Slice(out.Bindings, func(i, j int) bool {
		a, b := out.Bindings[i], out.Bindings[j]
		if a.Kind != b.Kind {
			return a.Kind == "ClusterRoleBinding"
		}
		if a.Namespace != b.Namespace {
			return a.Namespace < b.Namespace
		}
		return a.Name < b.Name
	})
	sort.Slice(out.ServiceAccounts, func(i, j int) bool {
		a, b := out.ServiceAccounts[i], out.ServiceAccounts[j]
		if a.Namespace != b.Namespace {
			return a.Namespace < b.Namespace
		}
		return a.Name < b.Name
	})
	return out
}

func emptyAccessModel() AccessModel {
	return AccessModel{
		Roles:           []AccessRole{},
		Bindings:        []AccessBinding{},
		ServiceAccounts: []AccessAccount{},
		Me:              Identity{Groups: []string{}},
		Unreadable:      []string{},
	}
}

func accessRules(rules []rbacv1.PolicyRule) []AccessRule {
	out := make([]AccessRule, 0, len(rules))
	for _, r := range rules {
		out = append(out, AccessRule{
			Verbs:           nonNil(r.Verbs),
			APIGroups:       nonNil(r.APIGroups),
			Resources:       nonNil(r.Resources),
			ResourceNames:   nonNil(r.ResourceNames),
			NonResourceURLs: nonNil(r.NonResourceURLs),
		})
	}
	return out
}

// accessSubjects copies a binding's subjects. A ServiceAccount subject with no
// namespace is one a modern API server refuses, but an old object can still
// carry one; it is read as the binding's own namespace, which is what the
// authorizer did with it. Users and groups are cluster-wide whatever they say.
func accessSubjects(subjects []rbacv1.Subject, bindingNamespace string) []AccessSubject {
	out := make([]AccessSubject, 0, len(subjects))
	for _, s := range subjects {
		ns := ""
		if s.Kind == rbacv1.ServiceAccountKind {
			ns = s.Namespace
			if ns == "" {
				ns = bindingNamespace
			}
		}
		out = append(out, AccessSubject{Kind: s.Kind, Name: s.Name, Namespace: ns})
	}
	return out
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
