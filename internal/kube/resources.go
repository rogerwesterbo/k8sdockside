package kube

// Pill is one small rectangle drawn in place of text: a pod's container, with
// its state carried by colour so a screen of pods can be scanned rather than
// read. Label and Detail are what a tooltip and a screen reader get.
type Pill struct {
	Label  string `json:"label"`
	Tone   string `json:"tone"`
	Detail string `json:"detail"`
}

// Cell is one table cell. Tone lets the frontend colour a value without having
// to know what the value means: "ok", "warn", "error", "info" or "" for plain.
type Cell struct {
	Text string `json:"text"`
	Tone string `json:"tone"`
	// Pills, where a cell is drawn as rectangles rather than written out. Empty
	// for every kind but one, and an empty one renders as Text exactly as
	// before.
	Pills []Pill `json:"pills"`
	// Sort is compared in place of Text where the two do not share an order.
	// An age reads "3d" but belongs in seconds; a volume reads "500Mi" but
	// belongs in bytes. Sorting the text would put "5m" before "2h" before
	// "3d". Empty means Text is already in the right order.
	Sort string `json:"sort"`
}

// Row is one resource in a listing. Name and Namespace are carried separately
// from Cells because they identify the object for a follow-up Describe call,
// regardless of which columns the kind happens to render.
type Row struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Cells     []Cell `json:"cells"`
}

// Table is a generic resource listing. Every kind returns this shape so the UI
// renders them all through one component.
type Table struct {
	Kind       string   `json:"kind"`
	Columns    []string `json:"columns"`
	Rows       []Row    `json:"rows"`
	Namespaced bool     `json:"namespaced"`
	Error      string   `json:"error"`
}

// Stat is a ready/total counter on the dashboard.
type Stat struct {
	Label string `json:"label"`
	Ready int    `json:"ready"`
	Total int    `json:"total"`
}

// Event is a cluster event as shown on the dashboard and in the events table.
type Event struct {
	Type    string `json:"type"`
	Reason  string `json:"reason"`
	Object  string `json:"object"`
	Message string `json:"message"`
	Age     string `json:"age"`
}

// Overview is the dashboard payload for one context.
type Overview struct {
	ContextID    string   `json:"contextId"`
	Context      string   `json:"context"`
	Cluster      string   `json:"cluster"`
	Server       string   `json:"server"`
	Version      string   `json:"version"`
	Distribution string   `json:"distribution"`
	Namespaces   []string `json:"namespaces"`
	Stats        []Stat   `json:"stats"`
	// Events is the same Table the events tab renders, capped to what the
	// dashboard has room for, so both are sorted and sortable by the same code
	// rather than by two implementations that can disagree.
	Events Table  `json:"events"`
	Error  string `json:"error"`
}

// Resource kinds the UI can open a tab for. These strings are the contract
// between the frontend nav catalogue and the table builders in stub.go.
const (
	KindNodes       = "nodes"
	KindNamespaces  = "namespaces"
	KindPods        = "pods"
	KindDeployments = "deployments"
	KindStatefulSet = "statefulsets"
	KindDaemonSets  = "daemonsets"
	KindJobs        = "jobs"
	KindCronJobs    = "cronjobs"
	KindServices    = "services"
	KindIngresses   = "ingresses"
	KindConfigMaps  = "configmaps"
	KindSecrets     = "secrets"
	KindPVCs        = "persistentvolumeclaims"
	KindEvents      = "events"

	// Workload kinds beyond the original set.
	KindReplicaSets            = "replicasets"
	KindReplicationControllers = "replicationcontrollers"
	KindHPAs                   = "horizontalpodautoscalers"

	// Namespace-level configuration and coordination.
	KindResourceQuotas = "resourcequotas"
	KindLimitRanges    = "limitranges"
	KindLeases         = "leases"

	// Scheduling and availability policy.
	KindPDBs            = "poddisruptionbudgets"
	KindPriorityClasses = "priorityclasses"
	KindRuntimeClasses  = "runtimeclasses"

	// Admission control. The policy kinds are much newer than the webhook ones
	// and a cluster may serve neither; that is reported as "does not serve"
	// rather than as a failure.
	KindMutatingWebhooks                  = "mutatingwebhookconfigurations"
	KindValidatingWebhooks                = "validatingwebhookconfigurations"
	KindMutatingAdmissionPolicies         = "mutatingadmissionpolicies"
	KindMutatingAdmissionPolicyBindings   = "mutatingadmissionpolicybindings"
	KindValidatingAdmissionPolicies       = "validatingadmissionpolicies"
	KindValidatingAdmissionPolicyBindings = "validatingadmissionpolicybindings"

	// Networking beyond Services and Ingresses. Endpoints is deprecated as of
	// Kubernetes 1.33 in favour of EndpointSlices, but is still what many
	// clusters and controllers actually carry, so both are offered.
	KindEndpointSlices  = "endpointslices"
	KindEndpoints       = "endpoints"
	KindIngressClasses  = "ingressclasses"
	KindNetworkPolicies = "networkpolicies"

	// Cluster-wide storage, beside the namespaced claims.
	KindStorageClasses = "storageclasses"
	KindPVs            = "persistentvolumes"

	// Who may do what.
	KindServiceAccounts     = "serviceaccounts"
	KindRoles               = "roles"
	KindRoleBindings        = "rolebindings"
	KindClusterRoles        = "clusterroles"
	KindClusterRoleBindings = "clusterrolebindings"

	// Not a Kubernetes kind: Helm keeps its releases in Secrets, and this one is
	// served by decoding them rather than by watching a resource. See helm.go.
	KindHelmReleases = "helmreleases"
)
