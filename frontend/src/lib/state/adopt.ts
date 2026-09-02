// The boundary between the generated bindings and the rest of the app.
//
// The binding generator types every Go slice and map as nullable, because a nil
// one serialises to JSON null. Left alone that spreads `?? []` through every
// component that touches a service result. These adapters resolve it once, on
// the way in, and hand back the plain non-null shapes the app works with.
//
// Copying here also insulates the app from how the bindings were generated:
// with some flags the models arrive as class instances, which Svelte's $state
// does not deep-proxy, so nested writes to them would never reach the UI.

import type * as kube from '../../../bindings/github.com/roger/k8sdockside/internal/kube/models.js';
import type * as appconfig from '../../../bindings/github.com/roger/k8sdockside/internal/appconfig/models.js';

/** A kubeconfig file and the contexts parsed out of it. */
export interface ConfigFile {
    path: string;
    source: string;
    contexts: kube.Context[];
    error: string;
}

/** The persisted user preferences. */
export interface Settings {
    manualFiles: string[];
    contexts: Record<string, { alias: string; color: string }>;
    tabOrder: { contextId: string; kind: string }[];
    layout: { detailDock: string; detailSize: number; sidebarWidth: number };
}

/** One resource in a listing. */
export interface Row {
    id: string;
    name: string;
    namespace: string;
    cells: kube.Cell[];
}

/** A resource listing. */
export interface Table {
    kind: string;
    columns: string[];
    rows: Row[];
    namespaced: boolean;
    error: string;
}

/** The dashboard payload for one context. */
export interface Overview {
    context: string;
    cluster: string;
    server: string;
    version: string;
    distribution: string;
    namespaces: string[];
    stats: kube.Stat[];
    gauges: kube.Gauge[];
    events: kube.Event[];
}

export function adoptSettings(settings: appconfig.Settings): Settings {
    return {
        manualFiles: [...(settings.manualFiles ?? [])],
        contexts: Object.fromEntries(
            Object.entries(settings.contexts ?? {}).map(([id, prefs]) => [
                id,
                { alias: prefs?.alias ?? '', color: prefs?.color ?? '' },
            ]),
        ),
        tabOrder: (settings.tabOrder ?? []).map((tab) => ({ contextId: tab.contextId, kind: tab.kind })),
        layout: { ...settings.layout },
    };
}

export function adoptFiles(files: kube.File[] | null): ConfigFile[] {
    return (files ?? []).map((file) => ({
        path: file.path,
        source: file.source,
        contexts: [...(file.contexts ?? [])],
        error: file.error,
    }));
}

export function adoptTable(table: kube.Table): Table {
    return {
        kind: table.kind,
        columns: [...(table.columns ?? [])],
        rows: (table.rows ?? []).map((row) => ({
            id: row.id,
            name: row.name,
            namespace: row.namespace,
            cells: [...(row.cells ?? [])],
        })),
        namespaced: table.namespaced,
        error: table.error,
    };
}

export function adoptOverview(overview: kube.Overview): Overview {
    return {
        context: overview.context,
        cluster: overview.cluster,
        server: overview.server,
        version: overview.version,
        distribution: overview.distribution,
        namespaces: [...(overview.namespaces ?? [])],
        stats: [...(overview.stats ?? [])],
        gauges: [...(overview.gauges ?? [])],
        events: [...(overview.events ?? [])],
    };
}
