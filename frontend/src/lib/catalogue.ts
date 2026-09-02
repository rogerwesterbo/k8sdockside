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
];

const BY_KIND = new Map<string, NavItem>(
    NAV_GROUPS.flatMap((group) => group.items).map((item) => [item.kind, item]),
);

/** The tab title for a kind; falls back to the raw kind so nothing renders blank. */
export function labelFor(kind: string): string {
    return BY_KIND.get(kind)?.label ?? kind;
}

/** The icon name for a kind. */
export function iconFor(kind: string): string {
    return BY_KIND.get(kind)?.icon ?? 'box';
}

/**
 * The singular noun used when describing one row of a kind, e.g. the header of
 * the slide-in detail panel.
 */
export function singularFor(kind: string): string {
    const label = labelFor(kind);
    return label.endsWith('ses') ? label.slice(0, -2) : label.replace(/s$/, '');
}
