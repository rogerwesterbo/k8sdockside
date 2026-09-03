package kube

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// The kinds added alongside the original set. Each is checked the same way: a
// literal object in, the cells the sidebar will show out. What is worth pinning
// here is the columns that are computed or renamed rather than read straight
// off a field -- those are the ones a refactor can quietly change.

// project renders one object through a kind's real columns.
func project(t *testing.T, kind string, namespaced bool, fields map[string]any) map[string]string {
	t.Helper()
	cols, ok := builtinColumns[kind]
	if !ok {
		t.Fatalf("no columns defined for %s", kind)
	}
	table := buildLiveTable(kind, withNamespace(cols, namespaced), namespaced, []*unstructured.Unstructured{obj(fields)})
	return cellsByHeader(t, table, 0)
}

func want(t *testing.T, cells map[string]string, column, expected string) {
	t.Helper()
	if cells[column] != expected {
		t.Errorf("%s = %q, want %q", column, cells[column], expected)
	}
}

// Every kind the UI can name has to be both resolvable to an API resource and
// renderable. Adding one without the other is the mistake this catches.
func TestEveryBuiltinKindHasColumnsAndAMapping(t *testing.T) {
	for kind := range builtinKinds {
		if _, ok := builtinColumns[kind]; !ok {
			t.Errorf("kind %q has an API mapping but no columns", kind)
		}
	}
	for kind := range builtinColumns {
		if _, ok := builtinKinds[kind]; !ok {
			t.Errorf("kind %q has columns but no API mapping", kind)
		}
	}
}

func TestReplicaSetsShowDesiredCurrentAndReady(t *testing.T) {
	cells := project(t, KindReplicaSets, true, map[string]any{
		"metadata": map[string]any{"name": "api-7d9f", "namespace": "prod"},
		"spec": map[string]any{
			"replicas": int64(3),
			"template": map[string]any{"spec": map[string]any{
				"containers": []any{map[string]any{"name": "api", "image": "ghcr.io/acme/api:1.4"}},
			}},
		},
		"status": map[string]any{"replicas": int64(3), "readyReplicas": int64(2)},
	})

	want(t, cells, "Desired", "3")
	want(t, cells, "Current", "3")
	want(t, cells, "Ready", "2/3")
	want(t, cells, "Image", "ghcr.io/acme/api:1.4")
}

func TestReplicationControllersReadTheirOwnTemplate(t *testing.T) {
	cells := project(t, KindReplicationControllers, true, map[string]any{
		"metadata": map[string]any{"name": "legacy", "namespace": "default"},
		"spec": map[string]any{
			"replicas": int64(2),
			"template": map[string]any{"spec": map[string]any{
				"containers": []any{map[string]any{"name": "web", "image": "nginx:1.27"}},
			}},
		},
		"status": map[string]any{"replicas": int64(2), "readyReplicas": int64(2)},
	})

	want(t, cells, "Ready", "2/2")
	want(t, cells, "Image", "nginx:1.27")
}

func TestHorizontalPodAutoscalersNameWhatTheyScale(t *testing.T) {
	cells := project(t, KindHPAs, true, map[string]any{
		"metadata": map[string]any{"name": "api", "namespace": "prod"},
		"spec": map[string]any{
			"scaleTargetRef": map[string]any{"kind": "Deployment", "name": "api"},
			"minReplicas":    int64(2),
			"maxReplicas":    int64(10),
		},
		"status": map[string]any{"currentReplicas": int64(4)},
	})

	// The reference is the whole point of an HPA row: which workload moves.
	want(t, cells, "Reference", "Deployment/api")
	want(t, cells, "Min", "2")
	want(t, cells, "Max", "10")
	want(t, cells, "Replicas", "4")
}

func TestPodDisruptionBudgetsShowHowMuchDisruptionIsAllowed(t *testing.T) {
	cells := project(t, KindPDBs, true, map[string]any{
		"metadata": map[string]any{"name": "api", "namespace": "prod"},
		"spec":     map[string]any{"minAvailable": int64(2)},
		"status":   map[string]any{"disruptionsAllowed": int64(1)},
	})

	want(t, cells, "Min Available", "2")
	want(t, cells, "Allowed Disruptions", "1")
}

func TestPriorityClassesReadValueAndGlobalDefault(t *testing.T) {
	cells := project(t, KindPriorityClasses, false, map[string]any{
		"metadata":      map[string]any{"name": "system-cluster-critical"},
		"value":         int64(2000000000),
		"globalDefault": false,
	})

	want(t, cells, "Value", "2000000000")
	want(t, cells, "Global Default", "false")
}

func TestRuntimeClassesShowTheirHandler(t *testing.T) {
	cells := project(t, KindRuntimeClasses, false, map[string]any{
		"metadata": map[string]any{"name": "gvisor"},
		"handler":  "runsc",
	})

	want(t, cells, "Handler", "runsc")
}

func TestLeasesShowTheCurrentHolder(t *testing.T) {
	cells := project(t, KindLeases, true, map[string]any{
		"metadata": map[string]any{"name": "kube-scheduler", "namespace": "kube-system"},
		"spec":     map[string]any{"holderIdentity": "node-1_1a2b3c"},
	})

	want(t, cells, "Holder", "node-1_1a2b3c")
}

func TestResourceQuotasSummariseHardAndUsed(t *testing.T) {
	cells := project(t, KindResourceQuotas, true, map[string]any{
		"metadata": map[string]any{"name": "team", "namespace": "prod"},
		"status": map[string]any{
			"hard": map[string]any{"cpu": "10", "memory": "20Gi"},
			"used": map[string]any{"cpu": "3", "memory": "6Gi"},
		},
	})

	// A quota with nothing in these columns tells the reader nothing, so both
	// have to be summarised rather than counted.
	want(t, cells, "Hard", "cpu=10, memory=20Gi")
	want(t, cells, "Used", "cpu=3, memory=6Gi")
}

func TestLimitRangesListTheTypesTheyConstrain(t *testing.T) {
	cells := project(t, KindLimitRanges, true, map[string]any{
		"metadata": map[string]any{"name": "defaults", "namespace": "prod"},
		"spec": map[string]any{"limits": []any{
			map[string]any{"type": "Container"},
			map[string]any{"type": "Pod"},
		}},
	})

	want(t, cells, "Limit Types", "Container, Pod")
}

func TestWebhookConfigurationsCountTheirWebhooks(t *testing.T) {
	for _, kind := range []string{KindMutatingWebhooks, KindValidatingWebhooks} {
		cells := project(t, kind, false, map[string]any{
			"metadata": map[string]any{"name": "cert-manager"},
			"webhooks": []any{map[string]any{"name": "a"}, map[string]any{"name": "b"}},
		})
		want(t, cells, "Webhooks", "2")
	}
}

func TestAdmissionPoliciesShowTheirFailurePolicy(t *testing.T) {
	mutating := project(t, KindMutatingAdmissionPolicies, false, map[string]any{
		"metadata": map[string]any{"name": "add-sidecar"},
		"spec":     map[string]any{"failurePolicy": "Fail", "reinvocationPolicy": "IfNeeded"},
	})
	want(t, mutating, "Failure Policy", "Fail")
	want(t, mutating, "Reinvocation", "IfNeeded")

	validating := project(t, KindValidatingAdmissionPolicies, false, map[string]any{
		"metadata": map[string]any{"name": "require-labels"},
		"spec": map[string]any{
			"failurePolicy": "Ignore",
			"validations":   []any{map[string]any{"expression": "true"}},
		},
	})
	want(t, validating, "Failure Policy", "Ignore")
	want(t, validating, "Validations", "1")
}

func TestPolicyBindingsNameThePolicyTheyBind(t *testing.T) {
	for _, kind := range []string{KindMutatingAdmissionPolicyBindings, KindValidatingAdmissionPolicyBindings} {
		cells := project(t, kind, false, map[string]any{
			"metadata": map[string]any{"name": "binding"},
			"spec":     map[string]any{"policyName": "require-labels"},
		})
		want(t, cells, "Policy", "require-labels")
	}
}

// The admission and scheduling kinds are cluster-scoped; getting this wrong
// puts an always-empty Namespace column in front of every row.
func TestClusterScopedKindsAreNotGivenANamespaceColumn(t *testing.T) {
	for _, kind := range []string{
		KindPriorityClasses, KindRuntimeClasses, KindMutatingWebhooks, KindValidatingWebhooks,
		KindMutatingAdmissionPolicies, KindValidatingAdmissionPolicies,
		KindMutatingAdmissionPolicyBindings, KindValidatingAdmissionPolicyBindings,
	} {
		cols := withNamespace(builtinColumns[kind], false)
		for _, c := range cols {
			if c.Name == "Namespace" {
				t.Errorf("%s has a Namespace column but is cluster-scoped", kind)
			}
		}
	}
}

func TestEndpointSlicesSummariseTheirEndpoints(t *testing.T) {
	cells := project(t, KindEndpointSlices, true, map[string]any{
		"metadata":    map[string]any{"name": "api-abc", "namespace": "prod"},
		"addressType": "IPv4",
		"ports":       []any{map[string]any{"port": int64(8080), "name": "http"}},
		"endpoints": []any{
			map[string]any{"addresses": []any{"10.0.0.1"}, "conditions": map[string]any{"ready": true}},
			map[string]any{"addresses": []any{"10.0.0.2"}, "conditions": map[string]any{"ready": false}},
		},
	})

	want(t, cells, "Address Type", "IPv4")
	want(t, cells, "Ports", "8080")
	// The count that matters is how many are actually taking traffic.
	want(t, cells, "Ready", "1/2")
}

func TestEndpointsListTheAddressesBehindAService(t *testing.T) {
	cells := project(t, KindEndpoints, true, map[string]any{
		"metadata": map[string]any{"name": "api", "namespace": "prod"},
		"subsets": []any{map[string]any{
			"addresses": []any{map[string]any{"ip": "10.0.0.1"}, map[string]any{"ip": "10.0.0.2"}},
			"ports":     []any{map[string]any{"port": int64(8080)}},
		}},
	})

	want(t, cells, "Endpoints", "10.0.0.1, 10.0.0.2")
}

func TestIngressClassesNameTheirController(t *testing.T) {
	cells := project(t, KindIngressClasses, false, map[string]any{
		"metadata": map[string]any{
			"name":        "nginx",
			"annotations": map[string]any{"ingressclass.kubernetes.io/is-default-class": "true"},
		},
		"spec": map[string]any{"controller": "k8s.io/ingress-nginx"},
	})

	want(t, cells, "Controller", "k8s.io/ingress-nginx")
	want(t, cells, "Default", "true")
}

func TestNetworkPoliciesShowWhatTheySelectAndGovern(t *testing.T) {
	cells := project(t, KindNetworkPolicies, true, map[string]any{
		"metadata": map[string]any{"name": "deny-all", "namespace": "prod"},
		"spec": map[string]any{
			"podSelector": map[string]any{"matchLabels": map[string]any{"app": "api"}},
			"policyTypes": []any{"Ingress", "Egress"},
		},
	})

	want(t, cells, "Pod Selector", "app=api")
	want(t, cells, "Types", "Ingress, Egress")
}

func TestStorageClassesShowProvisionerAndDefault(t *testing.T) {
	cells := project(t, KindStorageClasses, false, map[string]any{
		"metadata": map[string]any{
			"name":        "gp3",
			"annotations": map[string]any{"storageclass.kubernetes.io/is-default-class": "true"},
		},
		"provisioner":       "ebs.csi.aws.com",
		"reclaimPolicy":     "Delete",
		"volumeBindingMode": "WaitForFirstConsumer",
	})

	want(t, cells, "Provisioner", "ebs.csi.aws.com")
	want(t, cells, "Default", "true")
	want(t, cells, "Reclaim", "Delete")
}

func TestPersistentVolumesShowWhatTheyAreBoundTo(t *testing.T) {
	cells := project(t, KindPVs, false, map[string]any{
		"metadata": map[string]any{"name": "pvc-123"},
		"spec": map[string]any{
			"capacity":                      map[string]any{"storage": "20Gi"},
			"storageClassName":              "gp3",
			"persistentVolumeReclaimPolicy": "Delete",
			"claimRef":                      map[string]any{"namespace": "prod", "name": "data-api-0"},
		},
		"status": map[string]any{"phase": "Bound"},
	})

	want(t, cells, "Capacity", "20Gi")
	want(t, cells, "Claim", "prod/data-api-0")
	want(t, cells, "Status", "Bound")
}

func TestServiceAccountsCountTheirSecrets(t *testing.T) {
	cells := project(t, KindServiceAccounts, true, map[string]any{
		"metadata": map[string]any{"name": "deployer", "namespace": "prod"},
		"secrets":  []any{map[string]any{"name": "deployer-token"}},
	})

	want(t, cells, "Secrets", "1")
}

func TestRolesCountTheRulesTheyGrant(t *testing.T) {
	for _, kind := range []string{KindRoles, KindClusterRoles} {
		cells := project(t, kind, kind == KindRoles, map[string]any{
			"metadata": map[string]any{"name": "reader", "namespace": "prod"},
			"rules": []any{
				map[string]any{"verbs": []any{"get", "list"}, "resources": []any{"pods"}},
				map[string]any{"verbs": []any{"get"}, "resources": []any{"configmaps"}},
			},
		})
		want(t, cells, "Rules", "2")
	}
}

func TestRoleBindingsNameWhatTheyBindAndToWhom(t *testing.T) {
	for _, kind := range []string{KindRoleBindings, KindClusterRoleBindings} {
		cells := project(t, kind, kind == KindRoleBindings, map[string]any{
			"metadata": map[string]any{"name": "readers", "namespace": "prod"},
			"roleRef":  map[string]any{"kind": "ClusterRole", "name": "view"},
			"subjects": []any{
				map[string]any{"kind": "ServiceAccount", "name": "deployer", "namespace": "prod"},
				map[string]any{"kind": "Group", "name": "devs"},
			},
		})
		want(t, cells, "Role", "ClusterRole/view")
		want(t, cells, "Subjects", "ServiceAccount/deployer, Group/devs")
	}
}
