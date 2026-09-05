// The resource catalogue: what the sidebar offers under a selected context, and
// what each entry opens as a tab. The `kind` strings are the contract with the
// Go side (internal/kube/resources.go) -- ResourceService.Table is called with
// exactly these values.

export const DASHBOARD = 'dashboard';

/**
 * The app-wide settings view, which opens as a tab like any other but belongs
 * to no cluster. It is a sentinel kind for the same reason DASHBOARD is: the
 * tab machinery already keys everything off `kind`, and a second concept of
 * "special tab" would have to be threaded through all of it.
 *
 * Its tabs carry an empty contextId, which the workspace guards for -- see
 * isSettingsTab there. The value is namespaced so it can never collide with a
 * resource kind coming back from the cluster.
 */
export const SETTINGS = '__settings__';

/**
 * Helm releases: a kind the sidebar offers that no Kubernetes API serves. It is
 * read from Helm's own release Secrets rather than watched, so the few places
 * that have to know the difference name it from here.
 */
export const HELM_RELEASES = 'helmreleases';

/**
 * The section whose children are fetched from the cluster: the API groups it
 * serves, and the definitions under each.
 */
export const DEFINITIONS_GROUP = 'Custom Resource Definitions';

/**
 * The section holding the solution plugins: Argo CD, Flux, Prometheus, and
 * whatever the user has installed.
 *
 * Its contents come from neither this list nor the cluster, but from the plugin
 * files on this machine -- so, like the definitions section, it is a heading
 * here and rows built somewhere else. See ContextTree.
 */
export const SOLUTIONS_GROUP = 'Solutions';

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

/**
 * The dashboard, which sits at the top of a context's tree on its own rather
 * than inside a section.
 *
 * A section of one is a heading you have to open to reach a single row, and the
 * cluster overview is the thing most often wanted straight after expanding a
 * context -- so it is always there, and never folded away.
 */
export const DASHBOARD_ITEM: NavItem = { kind: DASHBOARD, label: 'Dashboard', icon: 'dashboard' };

export const NAV_GROUPS: NavGroup[] = [
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
        // Empty on purpose: what goes here is the solution plugins installed on
        // this machine, which this list cannot know about. It is a section so
        // that it folds, is remembered folded, and sits in the order below --
        // above the definitions, because a plugin is the tidy way to look at
        // custom resources and the definitions tree is the raw one.
        label: SOLUTIONS_GROUP,
        items: [],
    },
    {
        // The one section whose contents come from the cluster rather than from
        // this list: below "All definitions" the sidebar shows the API groups
        // the cluster serves and the kinds under each. See ContextTree.
        label: DEFINITIONS_GROUP,
        items: [{ kind: 'customresourcedefinitions', label: 'All definitions', icon: 'puzzle' }],
    },
];

/**
 * The sections that start folded on a fresh install: all of them.
 *
 * Expanding a context then shows the dashboard and a dozen headings rather than
 * fifty rows, and you open the one you want. Derived from the sections
 * themselves rather than listed, so a section added later is folded too instead
 * of quietly appearing open.
 */
export const DEFAULT_COLLAPSED_GROUPS = NAV_GROUPS.map((group) => group.label);

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

/**
 * The prefix marking a tab opened on a solution plugin's view:
 * `plugin:<pluginId>/<viewId>`.
 *
 * Exactly the trick `crd:` plays, and for exactly the same reason: a tab's kind
 * is persisted, reordered, restored and rendered by machinery that never looks
 * inside it, so a plugin view becomes a tab without any of that learning a
 * second shape. Parsed here and in internal/plugins, nowhere else.
 */
export const PLUGIN_PREFIX = 'plugin:';

export interface PluginView {
    pluginId: string;
    viewId: string;
}

/** Splits a `plugin:` kind into its plugin and view, or null if it is not one. */
export function parsePluginKind(kind: string): PluginView | null {
    if (!kind.startsWith(PLUGIN_PREFIX)) return null;

    const rest = kind.slice(PLUGIN_PREFIX.length);
    const slash = rest.indexOf('/');
    if (slash <= 0 || slash === rest.length - 1) return null;

    return { pluginId: rest.slice(0, slash), viewId: rest.slice(slash + 1) };
}

/** The kind string that opens one of a plugin's views. */
export function pluginKindFor(pluginId: string, viewId: string): string {
    return `${PLUGIN_PREFIX}${pluginId}/${viewId}`;
}

/** The view id the overview takes, matching plugins.OverviewID on the Go side. */
export const PLUGIN_OVERVIEW = 'overview';

/** True for the kind that opens a plugin's landing page rather than a listing. */
export function isPluginOverview(kind: string): boolean {
    return parsePluginKind(kind)?.viewId === PLUGIN_OVERVIEW;
}

/**
 * How a plugin view is titled and iconed.
 *
 * This is a registry rather than something derived from the kind string,
 * because unlike a `crd:` kind -- whose plural is right there in it -- a plugin
 * view's label lives in a file on disk. The workspace fills it in when the
 * plugin catalogue loads, and everything that titles a tab goes on calling
 * labelFor() without knowing plugins exist.
 *
 * A kind with no entry still renders: it falls back to the view id, which is
 * what a tab restored before its plugin has loaded shows for an instant, and
 * what a tab whose plugin has been uninstalled shows for good.
 */
const PLUGIN_VIEWS = new Map<string, NavItem>();

/** Replaces what is known about the installed plugins' views. */
export function registerPluginViews(items: NavItem[]): void {
    PLUGIN_VIEWS.clear();
    for (const item of items) {
        PLUGIN_VIEWS.set(item.kind, item);
    }
}

// The dashboard is deliberately absent: it belongs to no section, so there is
// never one to unfold in order to reach it.
const GROUP_OF = new Map<string, string>(
    NAV_GROUPS.flatMap((group) => group.items.map((item) => [item.kind, group.label] as const)),
);

const BY_KIND = new Map<string, NavItem>(
    [DASHBOARD_ITEM, ...NAV_GROUPS.flatMap((group) => group.items)].map((item) => [item.kind, item]),
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
    if (kind === SETTINGS) return 'Settings';

    const known = BY_KIND.get(kind);
    if (known) return known.label;

    const plugin = PLUGIN_VIEWS.get(kind);
    if (plugin) return plugin.label;

    const custom = parseCustomKind(kind);
    if (custom) return custom.plural.charAt(0).toUpperCase() + custom.plural.slice(1);

    // A plugin view whose plugin is not loaded: the view id is the only name we
    // have, and it reads better than the whole `plugin:x/y` string.
    const view = parsePluginKind(kind);
    if (view) return view.viewId;

    return kind;
}

/** The icon name for a kind. Custom resources all share one. */
export function iconFor(kind: string): string {
    if (kind === SETTINGS) return 'settings';

    const known = BY_KIND.get(kind);
    if (known) return known.icon;

    const plugin = PLUGIN_VIEWS.get(kind);
    if (plugin) return plugin.icon;

    return parseCustomKind(kind) || parsePluginKind(kind) ? 'puzzle' : 'box';
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
