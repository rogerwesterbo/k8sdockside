// The resource catalogue: what the sidebar offers under a selected context, and
// what each entry opens as a tab. The `kind` strings are the contract with the
// Go side (internal/kube/resources.go) -- ResourceService.Table is called with
// exactly these values.

export const DASHBOARD = 'dashboard';

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
        ],
    },
    {
        label: 'Workloads',
        items: [
            { kind: 'pods', label: 'Pods', icon: 'box' },
            { kind: 'deployments', label: 'Deployments', icon: 'rocket' },
            { kind: 'statefulsets', label: 'Stateful Sets', icon: 'database' },
            { kind: 'daemonsets', label: 'Daemon Sets', icon: 'repeat' },
            { kind: 'jobs', label: 'Jobs', icon: 'check' },
            { kind: 'cronjobs', label: 'Cron Jobs', icon: 'clock' },
        ],
    },
    {
        label: 'Network',
        items: [
            { kind: 'services', label: 'Services', icon: 'share' },
            { kind: 'ingresses', label: 'Ingresses', icon: 'globe' },
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
        ],
    },
    {
        label: 'Storage',
        items: [{ kind: 'persistentvolumeclaims', label: 'Persistent Volume Claims', icon: 'drive' }],
    },
    {
        label: 'Definitions',
        items: [{ kind: 'customresourcedefinitions', label: 'Custom Resource Definitions', icon: 'puzzle' }],
    },
];

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

const BY_KIND = new Map<string, NavItem>(
    NAV_GROUPS.flatMap((group) => group.items).map((item) => [item.kind, item]),
);

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
 */
export function singularFor(kind: string): string {
    const label = labelFor(kind);
    return label.endsWith('ses') ? label.slice(0, -2) : label.replace(/s$/, '');
}
