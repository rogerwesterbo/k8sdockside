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
import { DEFAULT_THEME_ID } from '../theme/apply';

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
    /** Extra folders themes are read from, on top of the default one. */
    themeFolders: string[];
    /** Extra folders solution plugins are read from, on top of the default one. */
    pluginFolders: string[];
    contexts: Record<string, { alias: string; color: string; metrics: string; collapsedGroups: string[] | null }>;
    tabOrder: { contextId: string; kind: string }[];
    /**
     * The bottom dock: what it has open, whether it is showing it, and how tall
     * it stands. One record with one writer -- see appconfig.Dock for why.
     */
    dock: {
        open: boolean;
        size: number;
        tabs: { type: string; contextId: string; kind: string; namespace: string; name: string }[];
    };
    /**
     * The app-wide preferences the settings view edits. Unlike `layout`, every
     * field here is resolved: `restoreTabs` arrives nullable from Go and is
     * settled to a boolean on the way in, because "never chosen" is a storage
     * concern and nothing downstream should have to know about it.
     */
    preferences: {
        /**
         * The id of the chosen theme. A free string rather than a union: the
         * themes are data, not code, and the set of valid ids is whatever is
         * installed at the time -- see internal/themes.
         */
        theme: string;
        density: 'comfortable' | 'compact';
        restoreTabs: boolean;
        confirmSourceRemoval: boolean;
        showKubeconfigNames: boolean;
        showLineNumbers: boolean;
        /** How far back a metrics chart looks, in minutes. */
        metricsRange: number;
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
        themeFolders: [...(settings.themeFolders ?? [])],
        pluginFolders: [...(settings.pluginFolders ?? [])],
        contexts: Object.fromEntries(
            Object.entries(settings.contexts ?? {}).map(([id, prefs]) => [
                id,
                {
                    alias: prefs?.alias ?? '',
                    color: prefs?.color ?? '',
                    metrics: prefs?.metrics ?? '',
                    // null is "follows the global folding" and must survive.
                    collapsedGroups: prefs?.collapsedGroups ?? null,
                },
            ]),
        ),
        tabOrder: (settings.tabOrder ?? []).map((tab) => ({ contextId: tab.contextId, kind: tab.kind })),
        dock: {
            // Closed until an editor is opened, which is what an older file
            // reads as too.
            open: settings.dock?.open ?? false,
            size: settings.dock?.size || 320,
            tabs: (settings.dock?.tabs ?? []).map((tab) => ({
                type: tab.type,
                contextId: tab.contextId,
                kind: tab.kind,
                namespace: tab.namespace,
                name: tab.name,
            })),
        },
        preferences: {
            // The store normalises these, so the fallbacks only cover a
            // service call that failed before it reached the store. An id that
            // names no installed theme is kept as it is and resolved where the
            // theme is applied, not here.
            theme: settings.preferences?.theme || DEFAULT_THEME_ID,
            density: (settings.preferences?.density || 'comfortable') as 'comfortable' | 'compact',
            // null is "never chosen", and the default is on. `??` rather than
            // `||`: an explicit false is a choice and must survive.
            restoreTabs: settings.preferences?.restoreTabs ?? true,
            confirmSourceRemoval: settings.preferences?.confirmSourceRemoval ?? false,
            // Off by default: most people keep every context in one
            // ~/.kube/config, where a heading per file only repeats itself.
            showKubeconfigNames: settings.preferences?.showKubeconfigNames ?? false,
            // On by default, and nullable on the Go side for exactly that
            // reason -- see RestoreTabs above.
            showLineNumbers: settings.preferences?.showLineNumbers ?? true,
            // Zero from the store means never chosen. An hour is long enough to
            // show a rollout and short enough to still show a spike.
            metricsRange: settings.preferences?.metricsRange || 60,
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
