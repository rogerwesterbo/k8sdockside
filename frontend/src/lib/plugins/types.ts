// What a solution plugin is, as the app works with it.
//
// A plugin teaches k8sdockside about something installed in a cluster -- Argo
// CD, Flux, Prometheus -- and gives it a place of its own in the sidebar. Like
// a theme it is a JSON file and nothing else: it names kinds the app already
// knows how to list and says how to arrange and summarise them, so installing
// someone else's is as safe as installing their theme.
//
// The one distinction worth holding on to is that a plugin is installed on
// *this machine*, while the solution it describes is installed in a *cluster*.
// Those come apart constantly, and everything below that talks about "detected"
// or "installed" is about the second.

/** One entry under a plugin in the sidebar, and one tab when opened. */
export interface PluginViewSpec {
    id: string;
    label: string;
    icon: string;
    type: string;
    /** The kind this view lists: a built-in name, or a `crd:` custom resource. */
    kind: string;
    /** Fixed for this view; the tab's own namespace filter is not offered. */
    namespace: string;
    selector: string;
}

/** A kind the plugin needs the cluster to serve. */
export interface PluginRequirement {
    kind: string;
    label: string;
    optional: boolean;
}

export interface Plugin {
    id: string;
    name: string;
    tagline: string;
    icon: string;
    author: string;
    docs: string;
    description: string;
    requires: PluginRequirement[];
    views: PluginViewSpec[];
    /** `builtin`, or the path of the file it was read from. */
    origin: string;
    /** The collection it arrived in, empty for one that came on its own. */
    pack: string;
    /**
     * Switched off in Settings. A disabled plugin is still in the catalogue --
     * that is where it gets switched back on -- but nothing offers it: no
     * sidebar rows, no charts, no overview. See `workspace.enabledPlugins`.
     */
    disabled: boolean;
}

/** Everything installed on this machine, and what would not load. */
export interface PluginCatalogue {
    plugins: Plugin[];
    dir: string;
    folders: string[];
    problems: { path: string; message: string }[];
}

/** One of a plugin's requirements, checked against a cluster. */
export interface Presence {
    kind: string;
    label: string;
    optional: boolean;
    served: boolean;
    /** Set when we could not find out, as opposed to finding out it is absent. */
    error: string;
}

/** One slice of a card: a value the grouped field took, and how many had it. */
export interface Bucket {
    /** Empty means the field was absent on those objects. */
    value: string;
    count: number;
    tone: string;
}

/** One live tile on the overview. */
export interface CardResult {
    label: string;
    kind: string;
    total: number;
    buckets: Bucket[];
    /** Whether this card divides its count at all. */
    grouped: boolean;
    /** Why it has no number, if it has none. */
    error: string;
}

/** A plugin's overview for one context. */
export interface PluginSummary {
    pluginId: string;
    /** Every required kind is served by this cluster. */
    installed: boolean;
    /** Whether we managed to ask; false leaves `installed` meaningless. */
    checked: boolean;
    requirements: Presence[];
    cards: CardResult[];
    error: string;
}

/** What a `plugin:` tab kind actually means. */
export interface ResolvedView {
    kind: string;
    namespace: string;
    selector: string;
    pluginId: string;
    pluginName: string;
    viewId: string;
    label: string;
    icon: string;
    overview: boolean;
}

/** The catalogue before anything has loaded. */
export function emptyPluginCatalogue(): PluginCatalogue {
    return { plugins: [], dir: '', folders: [], problems: [] };
}
