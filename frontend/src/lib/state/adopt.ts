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

import type * as kube from '../../../bindings/github.com/rogerwesterbo/k8sdockside/internal/kube/models.js';
import type * as appconfig from '../../../bindings/github.com/rogerwesterbo/k8sdockside/internal/appconfig/models.js';
import { DEFAULT_THEME_ID } from '../theme/apply';

/** A kubeconfig file and the contexts parsed out of it. */
export interface ConfigFile {
    path: string;
    source: string;
    contexts: kube.Context[];
    error: string;
}

/**
 * How a shell opens, resolved. Every field here has a value: the store fills
 * in what a settings file does not say, and the fallbacks below only cover a
 * service call that failed before it reached the store.
 */
export interface TerminalSettings {
    /** 'app' opens the terminal in the dock, 'external' the user's own. */
    mode: 'app' | 'external';
    /** The id of the external terminal to use; empty means this machine's default. */
    external: string;
    /** The shells tried in a container, in order, until one of them runs. */
    shells: string[];
    /** The image a node shell's debug pod runs, and where it is created. */
    nodeImage: string;
    nodeNamespace: string;
    fontSize: number;
    scrollback: number;
}

/**
 * How helm is run, resolved. Every field has a value here for the same reason
 * TerminalSettings does: the store fills in what the file does not say, and the
 * fallbacks below only cover a service call that failed before it got there.
 */
export interface HelmSettings {
    /** Where helm is, when the user has said. Empty means "find it". */
    path: string;
    /** Hold a change open until what it wrote reports ready. */
    wait: boolean;
    /** Roll a failed upgrade back. Implies wait, which the store enforces. */
    atomic: boolean;
    /** How long a command is given, in seconds. */
    timeoutSeconds: number;
}

/** One forward the user set up, as it is remembered between sessions. */
export interface SavedForward {
    id: string;
    contextId: string;
    kind: string;
    namespace: string;
    name: string;
    remotePort: number;
    localPort: number;
    random: boolean;
    browser: boolean;
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
        /** How a shell opens, and what it opens with. */
        terminal: TerminalSettings;
        /** Where helm is, and how it is run. */
        helm: HelmSettings;
    };
    /**
     * The forwards the user set up. The live state of each lives in the
     * forwards store, which the backend feeds; this is only what survives a
     * restart, and nothing in the app reads it directly.
     */
    portForwards: SavedForward[];
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
            terminal: {
                // 'app' is the answer that always works: the terminal in the
                // dock needs nothing installed on this machine.
                mode: (settings.preferences?.terminal?.mode || 'app') as 'app' | 'external',
                // Deliberately kept as written even when nothing on this
                // machine answers to it -- see appconfig.Terminal.External.
                external: settings.preferences?.terminal?.external ?? '',
                shells: [...(settings.preferences?.terminal?.shells ?? ['bash', 'sh'])],
                nodeImage: settings.preferences?.terminal?.nodeImage || 'busybox',
                nodeNamespace: settings.preferences?.terminal?.nodeNamespace || 'default',
                fontSize: settings.preferences?.terminal?.fontSize || 12,
                scrollback: settings.preferences?.terminal?.scrollback || 5000,
            },
            helm: {
                // Empty is the right default rather than an omission: helm is
                // normally on PATH, and a path guessed here would be wrong more
                // often than the search is.
                path: settings.preferences?.helm?.path ?? '',
                // helm's own defaults, so a release changed from this app
                // behaves the way the same command would from a shell.
                wait: settings.preferences?.helm?.wait ?? false,
                atomic: settings.preferences?.helm?.atomic ?? false,
                // Zero from the store means never chosen. Five minutes is
                // helm's own default for --wait.
                timeoutSeconds: settings.preferences?.helm?.timeoutSeconds || 300,
            },
        },
        portForwards: (settings.portForwards ?? []).map((forward) => ({
            id: forward.id,
            contextId: forward.contextId,
            kind: forward.kind,
            namespace: forward.namespace,
            name: forward.name,
            remotePort: forward.remotePort,
            localPort: forward.localPort,
            random: forward.random,
            browser: forward.browser,
        })),
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
        events: adoptTable(overview.events),
    };
}
