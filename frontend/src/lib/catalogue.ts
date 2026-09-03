// The resource catalogue: what the sidebar offers under a selected context, and
// what each entry opens as a tab. The `kind` strings are the contract with the
// Go side (internal/kube/resources.go) -- ResourceService.Table is called with
// exactly these values.

export const DASHBOARD = 'dashboard';

/**
 * Helm releases: a kind the sidebar offers that no Kubernetes API serves. It is
 * read from Helm's own release Secrets rather than watched, so the few places
 * that have to know the difference name it from here.
 */
export const HELM_RELEASES = 'helmreleases';

export interface NavItem {
    /** Resource kind passed to the backend, or DASHBOARD for the overview. */
    kind: string;
    label: string;
    icon: string;
}

export interface NavGroup {
    label: string;
    items: NavItem[];
}

export const NAV_GROUPS: NavGroup[] = [
    {
        label: 'Overview',
        items: [{ kind: DASHBOARD, label: 'Dashboard', icon: 'dashboard' }],
    },
    {
        label: 'Cluster',
        items: [
            { kind: 'nodes', label: 'Nodes', icon: 'server' },
            { kind: 'namespaces', label: 'Namespaces', icon: 'layers' },
            { kind: 'events', label: 'Events', icon: 'bell' },
            { kind: 'leases', label: 'Leases', icon: 'clock' },
        ],
    },
    {
        label: 'Workloads',
        items: [
            { kind: 'pods', label: 'Pods', icon: 'box' },
            { kind: 'deployments', label: 'Deployments', icon: 'rocket' },
            // Beside Deployments, which is what owns them.
            { kind: 'replicasets', label: 'Replica Sets', icon: 'copies' },
            { kind: 'statefulsets', label: 'Stateful Sets', icon: 'database' },
            { kind: 'daemonsets', label: 'Daemon Sets', icon: 'repeat' },
            { kind: 'replicationcontrollers', label: 'Replication Controllers', icon: 'copies' },
            { kind: 'jobs', label: 'Jobs', icon: 'check' },
            { kind: 'cronjobs', label: 'Cron Jobs', icon: 'clock' },
            { kind: 'horizontalpodautoscalers', label: 'Horizontal Pod Autoscalers', icon: 'scale' },
        ],
    },
    {
        label: 'Network',
        items: [
            { kind: 'services', label: 'Services', icon: 'share' },
            { kind: 'endpointslices', label: 'Endpoint Slices', icon: 'share' },
            // Deprecated in Kubernetes 1.33 in favour of Endpoint Slices, but
            // still what many clusters and controllers carry.
            { kind: 'endpoints', label: 'Endpoints', icon: 'share' },
            { kind: 'ingresses', label: 'Ingresses', icon: 'globe' },
            { kind: 'ingressclasses', label: 'Ingress Classes', icon: 'globe' },
            { kind: 'networkpolicies', label: 'Network Policies', icon: 'shield' },
        ],
    },
    {
        label: 'Gateway API',
        items: [
            { kind: 'gatewayclasses', label: 'Gateway Classes', icon: 'gateway' },
            { kind: 'gateways', label: 'Gateways', icon: 'gateway' },
            { kind: 'httproutes', label: 'HTTP Routes', icon: 'route' },
            { kind: 'grpcroutes', label: 'gRPC Routes', icon: 'route' },
            { kind: 'referencegrants', label: 'Reference Grants', icon: 'grant' },
        ],
    },
    {
        label: 'Config',
        items: [
            { kind: 'configmaps', label: 'Config Maps', icon: 'sliders' },
            { kind: 'secrets', label: 'Secrets', icon: 'lock' },
            { kind: 'resourcequotas', label: 'Resource Quotas', icon: 'gauge' },
            { kind: 'limitranges', label: 'Limit Ranges', icon: 'sliders' },
        ],
    },
    {
        label: 'Storage',
        items: [
            { kind: 'persistentvolumeclaims', label: 'Persistent Volume Claims', icon: 'drive' },
            { kind: 'persistentvolumes', label: 'Persistent Volumes', icon: 'drive' },
            { kind: 'storageclasses', label: 'Storage Classes', icon: 'database' },
        ],
    },
    {
        // Who may do what.
        label: 'Access',
        items: [
            { kind: 'serviceaccounts', label: 'Service Accounts', icon: 'grant' },
            { kind: 'roles', label: 'Roles', icon: 'policy' },
            { kind: 'rolebindings', label: 'Role Bindings', icon: 'link' },
            { kind: 'clusterroles', label: 'Cluster Roles', icon: 'policy' },
            { kind: 'clusterrolebindings', label: 'Cluster Role Bindings', icon: 'link' },
        ],
    },
    {
        // Not Kubernetes objects: Helm keeps its releases in Secrets, which the
        // backend decodes. See internal/kube/helm.go.
        label: 'Helm',
        items: [{ kind: 'helmreleases', label: 'Helm Releases', icon: 'helm' }],
    },
    {
        // What may be evicted, and in what order things get scheduled.
        label: 'Scheduling',
        items: [
            { kind: 'poddisruptionbudgets', label: 'Pod Disruption Budgets', icon: 'shield' },
            { kind: 'priorityclasses', label: 'Priority Classes', icon: 'priority' },
            { kind: 'runtimeclasses', label: 'Runtime Classes', icon: 'chip' },
        ],
    },
    {
        // Everything that intercepts a write on its way into the API server.
        // The policy kinds are far newer than the webhooks, so on many clusters
        // these tabs will report that the kind is not served -- which is an
        // ordinary answer here, not a failure.
        label: 'Admission',
        items: [
            { kind: 'mutatingwebhookconfigurations', label: 'Mutating Webhooks', icon: 'webhook' },
            { kind: 'validatingwebhookconfigurations', label: 'Validating Webhooks', icon: 'webhook' },
            { kind: 'mutatingadmissionpolicies', label: 'Mutating Admission Policies', icon: 'policy' },
            { kind: 'mutatingadmissionpolicybindings', label: 'Mutating Policy Bindings', icon: 'link' },
            { kind: 'validatingadmissionpolicies', label: 'Validating Admission Policies', icon: 'policy' },
            { kind: 'validatingadmissionpolicybindings', label: 'Validating Policy Bindings', icon: 'link' },
        ],
    },
    {
        label: 'Definitions',
        items: [{ kind: 'customresourcedefinitions', label: 'Custom Resource Definitions', icon: 'puzzle' }],
    },
];

/**
 * The groups that start folded on a fresh install.
 *
 * The full tree is around 1200px tall, half again the height of the sidebar, so
 * something has to give or a single cluster no longer fits on screen. These are
 * the specialist ones: a cluster without the Gateway API installed, or without
 * admission policies, gets nothing from those rows. Unfolding one is a click
 * and it is remembered, so this only decides the first run.
 *
 * The names live here, beside the groups they refer to, rather than in the Go
 * settings store -- which remembers only the strings it is handed.
 */
export const DEFAULT_COLLAPSED_GROUPS = ['Gateway API', 'Scheduling', 'Admission', 'Access', 'Definitions'];

/**
 * The prefix marking a kind that names a custom resource rather than one of the
 * kinds listed above: `crd:<plural>.<group>`.
 *
 * A tab's kind is an opaque string everywhere else -- it is persisted, reordered
 * and restored without anything looking inside it -- so a custom resource can be
 * a tab without any of that machinery learning a second shape. This module and
 * internal/kube/kinds.go are the only two places that parse it.
 */
export const CUSTOM_PREFIX = 'crd:';

export interface CustomKind {
    plural: string;
    group: string;
}

/** Splits a `crd:` kind into its plural and group, or null if it is not one. */
export function parseCustomKind(kind: string): CustomKind | null {
    if (!kind.startsWith(CUSTOM_PREFIX)) return null;

    const rest = kind.slice(CUSTOM_PREFIX.length);
    const dot = rest.indexOf('.');
    if (dot <= 0 || dot === rest.length - 1) return null;

    return { plural: rest.slice(0, dot), group: rest.slice(dot + 1) };
}

/** The kind string that opens the instance tab for a CustomResourceDefinition. */
export function customKindFor(definitionName: string): string {
    return CUSTOM_PREFIX + definitionName;
}

const GROUP_OF = new Map<string, string>(
    NAV_GROUPS.flatMap((group) => group.items.map((item) => [item.kind, group.label] as const)),
);

const BY_KIND = new Map<string, NavItem>(
    NAV_GROUPS.flatMap((group) => group.items).map((item) => [item.kind, item]),
);

/**
 * The sidebar section a kind is listed under, or null for one that is not in
 * the tree at all -- a custom resource, which is opened from the definitions
 * table rather than from a nav entry.
 */
export function groupForKind(kind: string): string | null {
    return GROUP_OF.get(kind) ?? null;
}

/**
 * The tab title for a kind; falls back to the raw kind so nothing renders blank.
 *
 * A custom resource is titled from its plural, which is the only name we have
 * without asking the cluster -- the CRD's own display kind lives on the server.
 */
export function labelFor(kind: string): string {
    const known = BY_KIND.get(kind);
    if (known) return known.label;

    const custom = parseCustomKind(kind);
    if (custom) return custom.plural.charAt(0).toUpperCase() + custom.plural.slice(1);

    return kind;
}

/** The icon name for a kind. Custom resources all share one. */
export function iconFor(kind: string): string {
    const known = BY_KIND.get(kind);
    if (known) return known.icon;
    return parseCustomKind(kind) ? 'puzzle' : 'box';
}

/**
 * The singular noun used when describing one row of a kind, e.g. the header of
 * the slide-in detail panel.
 *
 * Only a double s takes the "-es" plural here: "Ingresses" and "Classes" lose
 * two letters, everything else loses one. Matching a single "ses" would be
 * wrong for any singular already ending in "se" -- it turns "Leases" into
 * "Leas".
 */
export function singularFor(kind: string): string {
    const label = labelFor(kind);
    return label.endsWith('sses') ? label.slice(0, -2) : label.replace(/s$/, '');
}
