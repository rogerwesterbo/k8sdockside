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

/** The root font size, matching appconfig.DefaultFontSize and style.css. */
const DEFAULT_FONT_SIZE = 13;

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
    manualFolders: string[];
    excludedFiles: string[];
    contexts: Record<string, { alias: string; color: string; collapsedGroups: string[] | null }>;
    tabOrder: { contextId: string; kind: string }[];
    /**
     * The app-wide preferences the settings view edits. Unlike `layout`, every
     * field here is resolved: `restoreTabs` arrives nullable from Go and is
     * settled to a boolean on the way in, because "never chosen" is a storage
     * concern and nothing downstream should have to know about it.
     */
    preferences: {
        theme: 'system' | 'light' | 'dark';
        density: 'comfortable' | 'compact';
        fontSize: number;
        restoreTabs: boolean;
        confirmSourceRemoval: boolean;
    };
    layout: {
        detailDock: string;
        detailSize: number;
        sidebarWidth: number;
        zoom: number;
        /** null means the user has never chosen; an empty list means "fold nothing". */
        collapsedGroups: string[] | null;
    };
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
    /** The same shape a resource tab renders, so both sort through one path. */
    events: Table;
}

export function adoptSettings(settings: appconfig.Settings): Settings {
    return {
        manualFiles: [...(settings.manualFiles ?? [])],
        manualFolders: [...(settings.manualFolders ?? [])],
        excludedFiles: [...(settings.excludedFiles ?? [])],
        contexts: Object.fromEntries(
            Object.entries(settings.contexts ?? {}).map(([id, prefs]) => [
                id,
                {
                    alias: prefs?.alias ?? '',
                    color: prefs?.color ?? '',
                    // null is "follows the global folding" and must survive.
                    collapsedGroups: prefs?.collapsedGroups ?? null,
                },
            ]),
        ),
        tabOrder: (settings.tabOrder ?? []).map((tab) => ({ contextId: tab.contextId, kind: tab.kind })),
        preferences: {
            // The store normalises these, so the fallbacks only cover a
            // service call that failed before it reached the store.
            theme: (settings.preferences?.theme || 'system') as 'system' | 'light' | 'dark',
            density: (settings.preferences?.density || 'comfortable') as 'comfortable' | 'compact',
            fontSize: settings.preferences?.fontSize || DEFAULT_FONT_SIZE,
            // null is "never chosen", and the default is on. `??` rather than
            // `||`: an explicit false is a choice and must survive.
            restoreTabs: settings.preferences?.restoreTabs ?? true,
            confirmSourceRemoval: settings.preferences?.confirmSourceRemoval ?? false,
        },
        layout: {
            ...settings.layout,
            // Preserved as null rather than defaulted to []: the two mean
            // different things to the sidebar.
            collapsedGroups: settings.layout?.collapsedGroups ?? null,
            zoom: settings.layout?.zoom || 1,
        },
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
        events: adoptTable(overview.events),
    };
}
