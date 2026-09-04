// The whole application state lives here: which kubeconfig files were found,
// what the user renamed and coloured each context, which tabs are open and in
// what order, and what the slide-in detail panel is showing.
//
// It is one object rather than several stores because nearly every action
// touches more than one of those things -- opening a tab selects a context,
// closing one may change the active tab, dragging tabs persists settings -- and
// splitting them would only move the coordination somewhere less obvious.

import {
    KubeconfigService,
    ResourceService,
    SettingsService,
} from '../../../bindings/github.com/roger/k8sdockside';
import type * as kube from '../../../bindings/github.com/roger/k8sdockside/internal/kube/models.js';
import type * as appconfig from '../../../bindings/github.com/roger/k8sdockside/internal/appconfig/models.js';
import { adoptFiles, adoptSettings, type ConfigFile, type Settings } from './adopt';
import { DASHBOARD, DEFAULT_COLLAPSED_GROUPS, NAV_GROUPS, SETTINGS, groupForKind, labelFor } from '../catalogue';
import { defaultColorFor } from '../colors';

export type DockSide = 'right' | 'bottom' | 'left';

/** One open tab. A tab is a resource kind viewed against one context. */
export interface Tab {
    /** `${contextId}#${kind}` -- a key for keyed each blocks, never parsed back. */
    id: string;
    contextId: string;
    kind: string;
    title: string;
}

/** The object the detail panel is describing. */
export interface DetailTarget {
    contextId: string;
    kind: string;
    namespace: string;
    name: string;
}

/**
 * Whether a cluster can be reached, as shown by the sidebar indicator.
 *
 * `unknown` is the resting state and is not a failure: contexts are only
 * probed when you touch them, so most of a long list has simply never been
 * asked. It has to read as "not checked", never as "broken".
 */
export type HealthStatus = 'unknown' | 'checking' | 'connected' | 'error';

/** One context's reachability, with the reason when it failed. */
export interface Health {
    status: HealthStatus;
    message: string;
}

const UNCHECKED: Health = { status: 'unknown', message: '' };

/**
 * A request to bring one context into view in the sidebar.
 *
 * The nonce is the point: asking for the same context twice has to register as
 * two separate requests, because clicking the tab you are already on is how you
 * ask "where is this cluster?" and it must answer every time.
 */
export interface Reveal {
    contextId: string;
    /** The resource kind to show, so the sidebar can scroll to its own row. */
    kind: string;
    nonce: number;
}

/**
 * The custom resources a cluster serves, as the sidebar's definitions section
 * needs them: loaded when that section is first opened for a context, and kept
 * for as long as the context is around.
 */
export interface CustomApiGroup {
    group: string;
    /** Non-null here, unlike the binding: normalised on the way in. */
    kinds: kube.CustomResourceKind[];
}

export interface CustomKinds {
    status: 'idle' | 'loading' | 'ready' | 'error';
    groups: CustomApiGroup[];
    message: string;
}

const NOT_LOADED: CustomKinds = { status: 'idle', groups: [], message: '' };

/** A transient message shown in the status bar. */
export interface Notice {
    text: string;
    tone: 'info' | 'error';
}

function defaultSettings(): Settings {
    return {
        manualFiles: [],
        manualFolders: [],
        excludedFiles: [],
        contexts: {},
        tabOrder: [],
        preferences: {
            theme: 'system',
            density: 'comfortable',
            restoreTabs: true,
            confirmSourceRemoval: false,
            showKubeconfigNames: false,
        },
        layout: { detailDock: 'right', detailSize: 520, sidebarWidth: 260, collapsedGroups: null, zoom: 1 },
    };
}

/**
 * The settings tab's colour. Every other tab is painted with its cluster's, and
 * the whole point of that is to tell clusters apart -- so the one tab that is
 * not about a cluster takes the app's own accent instead of borrowing a
 * cluster's identity.
 */
const NEUTRAL_TAB_COLOR = '#7b8794';

/** Zoom bounds, matching appconfig.MinZoom/MaxZoom on the Go side. */
const MIN_ZOOM = 0.5;
const MAX_ZOOM = 2;
const ZOOM_STEP = 0.1;


/** Adds or removes one label, whichever way the toggle should go. */
function toggled(groups: string[], label: string): string[] {
    return groups.includes(label) ? groups.filter((g) => g !== label) : [...groups, label];
}

/** Whether two folding lists say the same thing, regardless of order. */
function sameGroups(a: string[], b: string[]): boolean {
    return a.length === b.length && [...a].sort().join('\u0000') === [...b].sort().join('\u0000');
}

/** The key an API group's open state is remembered under. */
function apiGroupKey(contextId: string, group: string): string {
    return `${contextId}\u0000${group}`;
}

function tabId(contextId: string, kind: string): string {
    return `${contextId}#${kind}`;
}

/**
 * Whether a tab is the app-wide settings view rather than a look at a cluster.
 *
 * It belongs to no context, so every place that resolves a tab's contextId --
 * to colour it, to name it, to decide whether its cluster still exists -- has
 * to ask this first. Keyed off the kind rather than the empty contextId
 * because the kind is the thing that is actually meaningful.
 */
export function isSettingsTab(tab: { kind: string }): boolean {
    return tab.kind === SETTINGS;
}

/** The id the settings tab always has: one per window, belonging to nothing. */
const SETTINGS_TAB_ID = tabId('', SETTINGS);

function message(err: unknown): string {
    if (err instanceof Error) return err.message;
    return String(err);
}

/**
 * debounce delays a call until the caller stops firing. Used so that dragging a
 * splitter or typing in the rename field does not write the settings file on
 * every frame or keystroke.
 */
function debounce<A extends unknown[]>(fn: (...args: A) => void, ms: number): (...args: A) => void {
    let timer: ReturnType<typeof setTimeout> | undefined;
    return (...args: A) => {
        clearTimeout(timer);
        timer = setTimeout(() => fn(...args), ms);
    };
}

class Workspace {
    /** Kubeconfig files from the last sync, each with its parsed contexts. */
    files = $state<ConfigFile[]>([]);
    /** Persisted user preferences, replaced wholesale by every backend write. */
    settings = $state<Settings>(defaultSettings());
    tabs = $state<Tab[]>([]);
    activeTabId = $state<string | null>(null);
    /** The context whose resource tree is showing, and whose settings the bottom panel edits. */
    selectedContextId = $state<string | null>(null);
    /** Contexts whose resource tree is expanded in the sidebar. */
    expanded = $state<string[]>([]);

    detailTarget = $state<DetailTarget | null>(null);
    detailText = $state('');
    detailLoading = $state(false);
    detailError = $state<string | null>(null);

    /** Reachability per context, written by both probes and tab outcomes. */
    health = $state<Record<string, Health>>({});
    /** Custom resource definitions per context, loaded on demand. */
    customKinds = $state<Record<string, CustomKinds>>({});
    /**
     * Which API groups are open, as `contextId\0group`. Not persisted: it is
     * where you are looking right now rather than how you like the sidebar, and
     * it would otherwise grow the settings file a line per group per cluster.
     */
    expandedApiGroups = $state<string[]>([]);

    /** The sidebar's standing request to scroll a context into view. */
    reveal = $state<Reveal | null>(null);
    private revealCount = 0;

    /**
     * Whether the OS is asking for a dark appearance. Only consulted while the
     * theme is `system`; kept up to date by watchSystemTheme so that changing
     * the OS setting repaints the app straight away.
     */
    systemPrefersDark = $state(true);

    syncing = $state(false);
    loaded = $state(false);
    notice = $state<Notice | null>(null);
    configPath = $state('');

    contexts = $derived(this.files.flatMap((f) => f.contexts));
    /** The folders being watched, listed in the sidebar so one can be dropped. */
    folders = $derived(this.settings.manualFolders);
    /** Files the user has hidden, so the sidebar can offer to show them again. */
    excluded = $derived(this.settings.excludedFiles);
    activeTab = $derived(this.tabs.find((t) => t.id === this.activeTabId) ?? null);
    selectedContext = $derived(this.contexts.find((c) => c.id === this.selectedContextId) ?? null);
    /** Files that could not be parsed, surfaced in the sidebar rather than dropped. */
    brokenFiles = $derived(this.files.filter((f) => f.error !== ''));
    /**
     * Whether any context on screen is open, which is what decides the
     * direction of the sidebar's expand/collapse toggle. It is checked against
     * the live contexts rather than the raw list, so ids left over from a
     * kubeconfig that has gone do not make the button offer to collapse
     * something nobody can see.
     */
    anyExpanded = $derived(this.contexts.some((c) => this.expanded.includes(c.id)));

    dock = $derived((this.settings.layout.detailDock || 'right') as DockSide);
    detailSize = $derived(this.settings.layout.detailSize || 520);
    sidebarWidth = $derived(this.settings.layout.sidebarWidth || 260);
    /**
     * The folded groups, falling back to the catalogue's defaults only while
     * the user has never chosen. `?? ` rather than `||` is load-bearing: an
     * empty list is a real choice and must not be defaulted over.
     */
    collapsedGroups = $derived(this.settings.layout.collapsedGroups ?? DEFAULT_COLLAPSED_GROUPS);
    /** The webview scale, 1 being normal size. */
    zoom = $derived(this.settings.layout.zoom || 1);
    /** The range the scale may be set to, for the settings view's slider. */
    readonly minZoom = MIN_ZOOM;
    readonly maxZoom = MAX_ZOOM;
    readonly zoomStep = ZOOM_STEP;

    /** What the user chose: system, light or dark. */
    theme = $derived(this.settings.preferences.theme);
    /**
     * The theme actually in force, with `system` resolved against the OS. It
     * tracks `systemPrefersDark`, which a media query listener keeps current,
     * so switching the OS appearance repaints without a restart.
     */
    resolvedTheme = $derived(
        this.theme === 'system' ? (this.systemPrefersDark ? 'dark' : 'light') : this.theme,
    );
    density = $derived(this.settings.preferences.density);
    restoreTabsOnLaunch = $derived(this.settings.preferences.restoreTabs);
    confirmSourceRemoval = $derived(this.settings.preferences.confirmSourceRemoval);
    /** Whether the sidebar groups contexts under the kubeconfig they came from. */
    showKubeconfigNames = $derived(this.settings.preferences.showKubeconfigNames);

    // ----- loading -------------------------------------------------------

    /** Loads settings and kubeconfigs, then restores the tabs from last time. */
    async load(): Promise<void> {
        try {
            this.settings = adoptSettings(await SettingsService.Get());
            this.configPath = await SettingsService.ConfigPath();
        } catch (err) {
            this.fail(`Could not read settings: ${message(err)}`);
        }
        await this.sync({ restoreTabs: true });
        this.loaded = true;
    }

    /**
     * Rescans every kubeconfig source. `restoreTabs` is for the initial load,
     * which reopens last session's tabs; a later sync keeps the tabs that are
     * open and only drops those whose context has gone.
     */
    async sync({ restoreTabs = false } = {}): Promise<void> {
        this.syncing = true;
        try {
            this.files = adoptFiles(await KubeconfigService.Sync());
            this.pruneHealth();
            this.pruneCustomKinds();
            if (restoreTabs) {
                this.restoreTabs();
            } else {
                this.dropTabsForMissingContexts();
                this.recheckHealth();
                this.recheckCustomKinds();
                // Only for a sync the user asked for: a rescan that finds
                // nothing new looks identical to one that did not run.
                this.inform(
                    `${this.contexts.length} context${this.contexts.length === 1 ? '' : 's'} ` +
                        `in ${this.files.length} file${this.files.length === 1 ? '' : 's'}`,
                );
            }
            this.ensureSelection();
        } catch (err) {
            this.fail(`Sync failed: ${message(err)}`);
        } finally {
            this.syncing = false;
        }
    }

    /** Opens the native picker and adds whichever kubeconfig the user chose. */
    async addFile(): Promise<void> {
        try {
            this.files = adoptFiles(await KubeconfigService.BrowseForFile());
            this.settings = adoptSettings(await SettingsService.Get());
            this.ensureSelection();
        } catch (err) {
            // Picking several files at once can partly succeed. The message
            // names what failed, but the ones that worked are already stored,
            // so the sidebar has to be brought up to date regardless.
            this.fail(message(err));
            await this.reload();
        }
    }

    /** Watches a folder, adding every kubeconfig in it. */
    async addFolder(): Promise<void> {
        try {
            this.files = adoptFiles(await KubeconfigService.BrowseForFolder());
            this.settings = adoptSettings(await SettingsService.Get());
            this.ensureSelection();
        } catch (err) {
            this.fail(message(err));
        }
    }

    /** Shows a hidden kubeconfig again. */
    async restoreFile(path: string): Promise<void> {
        try {
            this.files = adoptFiles(await KubeconfigService.RestoreFile(path));
            this.settings = adoptSettings(await SettingsService.Get());
            this.ensureSelection();
        } catch (err) {
            this.fail(message(err));
        }
    }

    /**
     * Asks before dropping a kubeconfig source, when the user has turned that
     * on. Both removals are already undoable -- a hidden file is listed under
     * Hidden and a folder can be re-added -- so this is off by default and
     * exists for people who would rather not have to undo.
     */
    private confirmRemoval(question: string): boolean {
        if (!this.confirmSourceRemoval) return true;
        if (typeof window === 'undefined' || !window.confirm) return true;
        return window.confirm(question);
    }

    /** Stops watching a folder; its configs leave the sidebar with it. */
    async removeFolder(path: string): Promise<void> {
        if (!this.confirmRemoval(`Stop watching ${path}?\n\nIts kubeconfigs leave the sidebar with it.`)) {
            return;
        }
        try {
            this.files = adoptFiles(await KubeconfigService.RemoveFolder(path));
            this.settings = adoptSettings(await SettingsService.Get());
            this.dropTabsForMissingContexts();
            this.ensureSelection();
        } catch (err) {
            this.fail(message(err));
        }
    }

    /** Re-reads the current file list without rescanning the disk. */
    private async reload(): Promise<void> {
        try {
            this.files = adoptFiles(await KubeconfigService.Files());
            this.settings = adoptSettings(await SettingsService.Get());
            this.ensureSelection();
        } catch {
            // Already reporting the failure that got us here.
        }
    }

    /** Forgets a kubeconfig the user added, closing any tabs that depended on it. */
    async removeFile(path: string): Promise<void> {
        if (!this.confirmRemoval(`Remove ${path}?\n\nAny tabs open against its contexts will close.`)) {
            return;
        }
        try {
            this.files = adoptFiles(await KubeconfigService.RemoveFile(path));
            this.settings = adoptSettings(await SettingsService.Get());
            this.dropTabsForMissingContexts();
            this.ensureSelection();
        } catch (err) {
            this.fail(message(err));
        }
    }

    // ----- contexts ------------------------------------------------------

    /** The name to show for a context: the user's alias, or the kubeconfig name. */
    displayName(context: kube.Context): string {
        return this.settings.contexts[context.id]?.alias?.trim() || context.name;
    }

    /**
     * The colour for a context: the user's choice, or one derived from its id.
     *
     * An empty id is the settings tab, which belongs to no cluster. Deriving a
     * colour for it would paint it as if it did, and always the same one, so
     * it is given the neutral accent instead.
     */
    colorOf(contextId: string): string {
        if (!contextId) return NEUTRAL_TAB_COLOR;
        return this.settings.contexts[contextId]?.color || defaultColorFor(contextId);
    }

    /** True if the user has set an alias or colour for this context. */
    isCustomised(contextId: string): boolean {
        const prefs = this.settings.contexts[contextId];
        return Boolean(prefs?.alias || prefs?.color);
    }

    /**
     * Records an alias and colour. The UI updates immediately and the write is
     * debounced, because this is called from a text field on every keystroke.
     */
    setContextPrefs(contextId: string, alias: string, color: string): void {
        // The folding override is a separate preference stored on the same
        // record; renaming or recolouring a context must not discard it.
        const collapsedGroups = this.settings.contexts[contextId]?.collapsedGroups ?? null;
        const prefs = { alias, color, collapsedGroups };
        if (!alias && !color && collapsedGroups === null) {
            delete this.settings.contexts[contextId];
        } else {
            this.settings.contexts[contextId] = prefs;
        }
        this.persistContextPrefs(contextId, prefs);
    }

    private persistContextPrefs = debounce((contextId: string, prefs: appconfig.ContextPrefs) => {
        SettingsService.SetContextPrefs(contextId, prefs)
            .then((saved) => {
                this.settings = adoptSettings(saved);
            })
            .catch((err: unknown) => this.fail(`Could not save context settings: ${message(err)}`));
    }, 300);

    /** Clears the alias and colour, returning the context to its defaults. */
    resetContextPrefs(contextId: string): void {
        this.setContextPrefs(contextId, '', '');
    }

    /** Selects a context in the sidebar and expands its resource tree. */
    selectContext(contextId: string): void {
        this.selectedContextId = contextId;
        if (!this.expanded.includes(contextId)) {
            this.expanded = [...this.expanded, contextId];
        }
        void this.probe(contextId);
    }

    /**
     * What clicking a context's name does: show it, and close it again if it is
     * the one already showing.
     *
     * The second click is the point -- opening a context and then having the
     * same click do nothing is a dead end. But it only closes the context you
     * are already on: clicking a *different* one means "show me this instead",
     * and folding away what you just reached for would be the opposite of that.
     */
    activateContext(contextId: string): void {
        if (this.selectedContextId === contextId && this.isExpanded(contextId)) {
            // Left selected: its settings panel is about the context, not about
            // whether its resource tree happens to be open.
            this.expanded = this.expanded.filter((id) => id !== contextId);
            return;
        }
        this.selectContext(contextId);
    }

    toggleExpanded(contextId: string): void {
        const opening = !this.expanded.includes(contextId);
        this.expanded = opening
            ? [...this.expanded, contextId]
            : this.expanded.filter((id) => id !== contextId);
        // Opening a context is a reason to know whether it answers; collapsing
        // one is not.
        if (opening) void this.probe(contextId);
    }

    isExpanded(contextId: string): boolean {
        return this.expanded.includes(contextId);
    }

    /**
     * Opens every context's resource tree.
     *
     * Note what this deliberately does not do: probe. `toggleExpanded` checks a
     * cluster when you open it, which is affordable one at a time, but building
     * a client can run an exec credential plugin -- so routing "expand all"
     * through it would launch a subprocess per context from a single click.
     * Unfolding the tree is a view operation; it asks no cluster anything.
     *
     * Assigning the whole list rather than merging also drops any stale ids
     * left behind by contexts that have since left the kubeconfig.
     */
    expandAll(): void {
        this.expanded = this.contexts.map((c) => c.id);
    }

    /** Closes every context's resource tree. */
    collapseAll(): void {
        this.expanded = [];
    }

    // ----- tabs ----------------------------------------------------------

    /**
     * Opens a tab, or focuses it if this context/kind pair is already open.
     *
     * A new tab lands immediately right of the selected one rather than at the
     * far end of the strip: it was almost always opened from the view you were
     * looking at, so that is where you will look for it, and a long strip means
     * the end of it may not even be on screen. With nothing selected there is
     * no "beside", so it goes last.
     */
    openTab(contextId: string, kind: string): void {
        const id = tabId(contextId, kind);
        if (!this.tabs.some((t) => t.id === id)) {
            const title = kind === DASHBOARD ? 'Dashboard' : labelFor(kind);
            const tab: Tab = { id, contextId, kind, title };
            const at = this.tabs.findIndex((t) => t.id === this.activeTabId);

            this.tabs =
                at === -1
                    ? [...this.tabs, tab]
                    : [...this.tabs.slice(0, at + 1), tab, ...this.tabs.slice(at + 1)];
            this.persistTabOrder();
        }
        this.activateTab(id);
    }

    /**
     * Opens the app-wide settings, or focuses it if already open.
     *
     * It goes to the far end of the strip rather than beside the current tab,
     * unlike openTab: it was not opened *from* the view you were looking at,
     * and pushing it into the middle of a row of clusters would break up the
     * grouping the user built by hand.
     */
    openSettings(): void {
        if (!this.tabs.some((t) => t.id === SETTINGS_TAB_ID)) {
            this.tabs = [...this.tabs, { id: SETTINGS_TAB_ID, contextId: '', kind: SETTINGS, title: 'Settings' }];
            this.persistTabOrder();
        }
        this.activateTab(SETTINGS_TAB_ID);
    }

    /** Focuses a tab and selects the context it belongs to. */
    activateTab(id: string): void {
        const changed = this.activeTabId !== id;
        this.activeTabId = id;

        const tab = this.tabs.find((t) => t.id === id);
        // The settings tab has no cluster to select, no tree section to unfold
        // and nothing to scroll to. Left unguarded it would deselect whatever
        // context the sidebar was showing every time you looked at settings.
        if (tab && !isSettingsTab(tab)) {
            this.selectContext(tab.contextId);
            this.showSectionFor(tab);
            // Asked for here rather than in selectContext, which the sidebar
            // also calls: a context clicked in the sidebar is already under the
            // pointer, and scrolling it would move it out from under them.
            this.reveal = { contextId: tab.contextId, kind: tab.kind, nonce: ++this.revealCount };
        }
        // Only when the view actually changed: the panel describes an object in
        // the tab we just left, so keeping it open over a different one would
        // be misleading. Re-clicking the tab you are on should leave it alone.
        if (changed) {
            this.closeDetail();
        }
    }

    /**
     * Unfolds the section an activated tab's resource is listed under, so that
     * the tree shows where the tab you are looking at actually lives.
     *
     * It does nothing when the section is already open, which matters: writing
     * unconditionally would give the context its own folding on every tab
     * click, pinning it against later changes applied to every cluster.
     */
    private showSectionFor(tab: Tab): void {
        const group = groupForKind(tab.kind);
        if (group === null || !this.isGroupCollapsed(tab.contextId, group)) return;
        this.toggleGroup(tab.contextId, group);
    }

    /** Closes a tab, moving focus to its neighbour so the view is never blank. */
    closeTab(id: string): void {
        this.retainTabs((tab) => tab.id !== id);
    }

    /**
     * Closes every tab but one. Pass a context to spare the tabs belonging to
     * other clusters -- "clear out staging, leave prod alone".
     */
    closeOtherTabs(id: string, withinContextId?: string): void {
        this.retainTabs(
            (tab) => tab.id === id || (withinContextId !== undefined && tab.contextId !== withinContextId),
        );
    }

    /** Closes every tab, or every tab belonging to one context. */
    closeAllTabs(withinContextId?: string): void {
        this.retainTabs((tab) => withinContextId !== undefined && tab.contextId !== withinContextId);
    }

    /**
     * Keeps the tabs matching `keep` and drops the rest, which is every closing
     * operation there is. Closing one tab and closing nine differ only in the
     * predicate; what they share -- where focus lands, that the detail panel
     * describes an object that may no longer be on screen, and that the order
     * is written once -- is the part worth having in one place.
     */
    private retainTabs(keep: (tab: Tab) => boolean): void {
        const survivors = this.tabs.filter(keep);
        if (survivors.length === this.tabs.length) return;

        const hadActive = this.activeTabId !== null;
        const stillActive = survivors.some((t) => t.id === this.activeTabId);
        const successor = hadActive && !stillActive ? this.successorFor(this.activeTabId, keep) : null;

        this.tabs = survivors;

        if (hadActive && !stillActive) {
            this.activeTabId = successor?.id ?? null;
            if (successor) this.selectContext(successor.contextId);
            this.closeDetail();
        }
        this.persistTabOrder();
    }

    /**
     * The tab that takes over when the active one closes: the first survivor to
     * its right, else the nearest to its left, so focus moves the short way and
     * lands where the eye already is.
     */
    private successorFor(activeId: string | null, keep: (tab: Tab) => boolean): Tab | null {
        const index = this.tabs.findIndex((t) => t.id === activeId);
        if (index === -1) return null;

        for (let i = index + 1; i < this.tabs.length; i++) {
            if (keep(this.tabs[i])) return this.tabs[i];
        }
        for (let i = index - 1; i >= 0; i--) {
            if (keep(this.tabs[i])) return this.tabs[i];
        }
        return null;
    }

    /** Reorders tabs after a drag. Both indices are positions in `tabs`. */
    moveTab(from: number, to: number): void {
        if (from === to || from < 0 || to < 0 || from >= this.tabs.length || to >= this.tabs.length) {
            return;
        }
        const next = [...this.tabs];
        const [moved] = next.splice(from, 1);
        next.splice(to, 0, moved);
        this.tabs = next;
        this.persistTabOrder();
    }

    /**
     * Debounced because dragging a tab reorders on every pointer move; without
     * it a single drag would write the settings file dozens of times.
     */
    private persistTabOrder = debounce(() => {
        const order = this.tabs.map((t) => ({ contextId: t.contextId, kind: t.kind }));
        SettingsService.SetTabOrder($state.snapshot(order))
            .then((saved) => {
                this.settings = adoptSettings(saved);
            })
            .catch((err: unknown) => this.fail(`Could not save tab order: ${message(err)}`));
    }, 250);

    /**
     * Reopens the tabs from the previous session, skipping any whose context is
     * no longer in a kubeconfig.
     */
    private restoreTabs(): void {
        // Turned off, the strip starts empty -- but the *order* is left on disk
        // untouched, so switching restore back on brings back the session it
        // was switched off during rather than nothing.
        if (!this.settings.preferences.restoreTabs) {
            this.tabs = [];
            this.activeTabId = null;
            return;
        }

        const known = new Set(this.contexts.map((c) => c.id));
        this.tabs = this.settings.tabOrder
            // The settings tab has no context, so it can never be "known" --
            // it is kept on its own terms.
            .filter((ref) => ref.kind === SETTINGS || known.has(ref.contextId))
            .map((ref) => ({
                id: tabId(ref.contextId, ref.kind),
                contextId: ref.contextId,
                kind: ref.kind,
                title: ref.kind === DASHBOARD ? 'Dashboard' : labelFor(ref.kind),
            }));
        this.activeTabId = this.tabs[0]?.id ?? null;
        if (this.tabs[0] && !isSettingsTab(this.tabs[0])) {
            this.selectContext(this.tabs[0].contextId);
        }
    }

    /** Drops tabs whose context disappeared from disk between syncs. */
    private dropTabsForMissingContexts(): void {
        const known = new Set(this.contexts.map((c) => c.id));
        // The settings tab survives every sync: it does not depend on a
        // kubeconfig, so losing every cluster must not close it.
        const kept = this.tabs.filter((t) => isSettingsTab(t) || known.has(t.contextId));
        if (kept.length === this.tabs.length) return;

        this.tabs = kept;
        if (!kept.some((t) => t.id === this.activeTabId)) {
            this.activeTabId = kept[0]?.id ?? null;
            this.closeDetail();
        }
        this.persistTabOrder();
    }

    /** Keeps a context selected if there is one, so the sidebar is never idle. */
    private ensureSelection(): void {
        if (this.selectedContextId && this.contexts.some((c) => c.id === this.selectedContextId)) {
            return;
        }
        const current = this.contexts.find((c) => c.current) ?? this.contexts[0];
        this.selectedContextId = current?.id ?? null;
        if (current) {
            this.selectContext(current.id);
        }
    }

    // ----- custom resources ----------------------------------------------

    /** What a context's definitions section should show. */
    customKindsFor(contextId: string): CustomKinds {
        return this.customKinds[contextId] ?? NOT_LOADED;
    }

    /**
     * Fetches the definitions a cluster serves.
     *
     * Called when the definitions section is opened rather than when a context
     * is, for the same reason probing is lazy: this is a request to a cluster,
     * and a kubeconfig with twenty contexts should not make twenty of them
     * because the sidebar was expanded. The answer is kept, so opening and
     * closing the section costs nothing after the first time.
     */
    async loadCustomKinds(contextId: string, { force = false } = {}): Promise<void> {
        const status = this.customKindsFor(contextId).status;
        if (status === 'loading') return;
        if (!force && status !== 'idle') return;

        this.customKinds[contextId] = { status: 'loading', groups: [], message: '' };
        try {
            const groups = await ResourceService.CustomResourceKinds(contextId);
            // Normalised here rather than guarded at every use: the generated
            // bindings type both the list and each group's kinds as nullable.
            this.customKinds[contextId] = {
                status: 'ready',
                groups: (groups ?? []).map((g) => ({ group: g.group, kinds: g.kinds ?? [] })),
                message: '',
            };
        } catch (err) {
            this.customKinds[contextId] = { status: 'error', groups: [], message: message(err) };
        }
    }

    /** Whether one API group's definitions are showing, for one context. */
    isApiGroupExpanded(contextId: string, group: string): boolean {
        return this.expandedApiGroups.includes(apiGroupKey(contextId, group));
    }

    toggleApiGroup(contextId: string, group: string): void {
        this.expandedApiGroups = toggled(this.expandedApiGroups, apiGroupKey(contextId, group));
    }

    /**
     * Re-reads the definitions of contexts that already had them, so the sync
     * button is the way back from a cluster that was unreachable when its
     * section was first opened.
     *
     * Without it a failure is permanent: the sidebar asks only while a context
     * has no answer, and an error counts as one. Contexts nobody has opened the
     * section for are left alone, which is what keeps this lazy.
     */
    private recheckCustomKinds(): void {
        for (const id of Object.keys(this.customKinds)) {
            void this.loadCustomKinds(id, { force: true });
        }
    }

    /** Forgets the definitions of contexts that are no longer in a kubeconfig. */
    private pruneCustomKinds(): void {
        const known = new Set(this.contexts.map((c) => c.id));
        const kept: Record<string, CustomKinds> = {};
        for (const [id, loaded] of Object.entries(this.customKinds)) {
            if (known.has(id)) kept[id] = loaded;
        }
        this.customKinds = kept;
    }

    // ----- health --------------------------------------------------------

    /** How a context last responded. Never probed is `unknown`, not an error. */
    healthOf(contextId: string): Health {
        return this.health[contextId] ?? UNCHECKED;
    }

    /**
     * Records what we now know about a cluster.
     *
     * Tabs call this as well as probes, and that is the point: a dashboard that
     * has just failed to load is better evidence than any ping, and routing
     * both through one map is what stops the sidebar indicator and the error
     * page in the tab from disagreeing.
     */
    reportHealth(contextId: string, status: HealthStatus, detail = ''): void {
        this.health[contextId] = { status, message: detail };
    }

    /**
     * Checks whether a cluster answers, for the sidebar indicator.
     *
     * Probing is lazy and deliberately so: building a client can run an exec
     * credential plugin, and a kubeconfig with twenty contexts would otherwise
     * launch twenty subprocesses at startup for clusters the user never asked
     * about. So a context is probed when it is touched -- selected, expanded or
     * opened in a tab -- and not again unless something asks it to be.
     */
    async probe(contextId: string, { force = false } = {}): Promise<void> {
        const status = this.healthOf(contextId).status;
        // A probe already in flight will report for both callers.
        if (status === 'checking') return;
        if (!force && status !== 'unknown') return;

        this.reportHealth(contextId, 'checking');
        try {
            await ResourceService.Ping(contextId);
            this.reportHealth(contextId, 'connected');
        } catch (err) {
            this.reportHealth(contextId, 'error', message(err));
        }
    }

    /** Forgets the status of contexts that are no longer in any kubeconfig. */
    private pruneHealth(): void {
        const known = new Set(this.contexts.map((c) => c.id));
        const kept: Record<string, Health> = {};
        for (const [id, health] of Object.entries(this.health)) {
            if (known.has(id)) kept[id] = health;
        }
        this.health = kept;
    }

    /**
     * Re-probes the contexts already carrying a status, so that the sync button
     * refreshes what is on screen. Contexts never checked stay unchecked: a
     * rescan is not a reason to start waking clusters the user has not asked
     * about.
     */
    private recheckHealth(): void {
        for (const id of Object.keys(this.health)) {
            void this.probe(id, { force: true });
        }
    }

    // ----- detail panel --------------------------------------------------

    /** Slides in the describe panel for one object. */
    async openDetail(target: DetailTarget): Promise<void> {
        this.detailTarget = target;
        this.detailLoading = true;
        this.detailError = null;
        try {
            this.detailText = await ResourceService.Describe(
                target.contextId,
                target.kind,
                target.namespace,
                target.name,
            );
        } catch (err) {
            this.detailText = '';
            this.detailError = message(err);
        } finally {
            this.detailLoading = false;
        }
    }

    closeDetail(): void {
        this.detailTarget = null;
        this.detailText = '';
        this.detailError = null;
    }

    /**
     * Moves the detail panel to another edge of the window. Called both by
     * dragging the panel and by the settings view, which sets the edge the next
     * panel will open on.
     */
    setDock(side: DockSide): void {
        this.settings.layout.detailDock = side;
        this.persistLayout();
    }

    setDetailSize(px: number): void {
        this.settings.layout.detailSize = Math.round(px);
        this.persistLayout();
    }

    /** The groups folded for one context: its own list, or the shared default. */
    collapsedGroupsFor(contextId: string): string[] {
        return this.settings.contexts[contextId]?.collapsedGroups ?? this.collapsedGroups;
    }

    /** Whether this context has diverged from the global folding. */
    hasFoldingOverride(contextId: string): boolean {
        return (this.settings.contexts[contextId]?.collapsedGroups ?? null) !== null;
    }

    /** Whether one group is folded differently here than everywhere else. */
    groupDiffersFromGlobal(contextId: string, label: string): boolean {
        if (!this.hasFoldingOverride(contextId)) return false;
        return this.isGroupCollapsed(contextId, label) !== this.collapsedGroups.includes(label);
    }

    /** Whether a sidebar resource group is folded for one context. */
    isGroupCollapsed(contextId: string, label: string): boolean {
        return this.collapsedGroupsFor(contextId).includes(label);
    }

    /**
     * Folds or unfolds one resource group for one context.
     *
     * Scoped to the context because that is what a section being folded means
     * to the person looking at it -- this cluster serves no Gateway API, so put
     * those rows away here. `allContexts` is the deliberate bulk version, for
     * when the answer really is the same everywhere.
     */
    toggleGroup(contextId: string, label: string, { allContexts = false } = {}): void {
        const next = toggled(this.collapsedGroupsFor(contextId), label);

        if (allContexts) {
            // "Make every cluster look like this": the shared default moves,
            // and the per-context answers are cleared so none is left quietly
            // disagreeing with what was just asked for. It also reaches
            // clusters added later, which nothing per-context can.
            this.settings.layout.collapsedGroups = next;
            this.persistLayout();
            for (const contextId of Object.keys(this.settings.contexts)) {
                this.setFoldingOverride(contextId, null);
            }
            return;
        }

        // The ordinary click is about the cluster in front of you. It is kept
        // even when it agrees with the shared default, because this context now
        // has an answer of its own and a later "apply to every cluster" should
        // not silently move it back.
        this.setFoldingOverride(contextId, next);
    }

    /**
     * Records (or clears) one context's own folding. Clearing returns it to the
     * shared default.
     */
    private setFoldingOverride(contextId: string, groups: string[] | null): void {
        const prefs = this.settings.contexts[contextId];
        const merged = {
            alias: prefs?.alias ?? '',
            color: prefs?.color ?? '',
            collapsedGroups: groups,
        };

        if (!merged.alias && !merged.color && groups === null) {
            delete this.settings.contexts[contextId];
        } else {
            this.settings.contexts[contextId] = merged;
        }
        this.persistContextPrefs(contextId, merged);
    }

    /** Whether any of a context's sections is open, and so worth collapsing. */
    anyGroupOpen(contextId: string): boolean {
        const folded = this.collapsedGroupsFor(contextId);
        return NAV_GROUPS.some((group) => !folded.includes(group.label));
    }

    /** Shuts every section for one context. */
    collapseAllGroups(contextId: string): void {
        this.setFoldingOverride(contextId, NAV_GROUPS.map((group) => group.label));
    }

    /** Opens every section for one context. */
    expandAllGroups(contextId: string): void {
        this.setFoldingOverride(contextId, []);
    }

    /** Returns a context to following the shared default folding. */
    clearFoldingOverride(contextId: string): void {
        this.setFoldingOverride(contextId, null);
    }

    zoomIn(): void {
        this.setZoom(this.zoom + ZOOM_STEP);
    }

    zoomOut(): void {
        this.setZoom(this.zoom - ZOOM_STEP);
    }

    resetZoom(): void {
        this.setZoom(1);
    }

    /**
     * Sets the scale directly, for the settings view's slider. The steppers go
     * through zoomIn/zoomOut instead.
     *
     * Clamped at both ends. The floor is not arbitrary: the window's title bar
     * is drawn by the frontend and so shrinks with the zoom, while the macOS
     * traffic lights over it do not -- far enough down and they no longer fit.
     */
    setZoom(scale: number): void {
        const next = Math.round(Math.min(MAX_ZOOM, Math.max(MIN_ZOOM, scale)) * 100) / 100;
        if (next === this.zoom) return;
        this.settings.layout.zoom = next;
        this.persistLayout();
    }

    setSidebarWidth(px: number): void {
        this.settings.layout.sidebarWidth = Math.round(px);
        this.persistLayout();
    }

    // ----- preferences ---------------------------------------------------

    /**
     * Starts tracking the OS appearance, and returns the teardown. Called from
     * the shell on mount rather than from the store's constructor: the store is
     * built at import time, which in the unit tests is a jsdom without
     * matchMedia.
     */
    watchSystemTheme(): () => void {
        if (typeof window === 'undefined' || !window.matchMedia) return () => {};

        const query = window.matchMedia('(prefers-color-scheme: dark)');
        this.systemPrefersDark = query.matches;

        const onChange = (event: MediaQueryListEvent) => {
            this.systemPrefersDark = event.matches;
        };
        query.addEventListener('change', onChange);
        return () => query.removeEventListener('change', onChange);
    }

    setTheme(theme: 'system' | 'light' | 'dark'): void {
        this.updatePreferences({ theme });
    }

    setDensity(density: 'comfortable' | 'compact'): void {
        this.updatePreferences({ density });
    }

    setRestoreTabs(restoreTabs: boolean): void {
        this.updatePreferences({ restoreTabs });
    }

    setConfirmSourceRemoval(confirmSourceRemoval: boolean): void {
        this.updatePreferences({ confirmSourceRemoval });
    }

    setShowKubeconfigNames(showKubeconfigNames: boolean): void {
        this.updatePreferences({ showKubeconfigNames });
    }

    /**
     * Applies a change to the preference block and saves it. The UI updates
     * first and the write is debounced, matching how the layout is persisted:
     * the font size comes from a slider, so this is called on every step of a
     * drag.
     */
    private updatePreferences(patch: Partial<Settings['preferences']>): void {
        this.settings.preferences = { ...this.settings.preferences, ...patch };
        this.persistPreferences();
    }

    private persistPreferences = debounce(() => {
        SettingsService.SetPreferences($state.snapshot(this.settings.preferences))
            .then((saved) => {
                this.settings = adoptSettings(saved);
            })
            .catch((err: unknown) => this.fail(`Could not save preferences: ${message(err)}`));
    }, 250);

    // ----- layout defaults, as edited from the settings view --------------

    /**
     * Sets the folding every context follows unless it has an override of its
     * own. Distinct from toggleGroup, which records a choice against one
     * cluster -- this is the shared baseline behind all of them.
     */
    setDefaultCollapsedGroups(groups: string[]): void {
        this.settings.layout.collapsedGroups = [...groups];
        this.persistLayout();
    }

    /** Folds or unfolds one group in the shared default. */
    toggleDefaultGroup(label: string): void {
        this.setDefaultCollapsedGroups(toggled(this.collapsedGroups, label));
    }

    /** How many contexts have overridden the shared folding with their own. */
    foldingOverrideCount = $derived(
        Object.values(this.settings.contexts).filter((prefs) => prefs.collapsedGroups !== null).length,
    );

    /**
     * Drops every context's folding override, returning all of them to the
     * shared default.
     *
     * The writes are issued here rather than through setFoldingOverride, which
     * looks like the obvious way to do it but is wrong: persistContextPrefs is
     * one shared debounce, so a loop through it would send only the last
     * context and leave every other override on disk to reappear on the next
     * read. Each is awaited in turn instead, and only the last result is
     * adopted -- every call returns the whole settings, so the final one is
     * already the complete picture.
     */
    async clearAllFoldingOverrides(): Promise<void> {
        const entries = Object.entries(this.settings.contexts).filter(
            ([, prefs]) => prefs.collapsedGroups !== null,
        );
        if (entries.length === 0) return;

        try {
            let saved: appconfig.Settings | null = null;
            for (const [id, prefs] of entries) {
                // Matches setFoldingOverride: a context left with nothing set
                // is forgotten rather than kept as an empty record.
                const cleared = { alias: prefs.alias, color: prefs.color, collapsedGroups: null };
                saved = await SettingsService.SetContextPrefs(id, cleared);
            }
            if (saved) this.settings = adoptSettings(saved);
            this.inform(
                `${entries.length} context${entries.length === 1 ? '' : 's'} now follow the default folding`,
            );
        } catch (err) {
            this.fail(`Could not clear the folding overrides: ${message(err)}`);
            await this.reloadSettings();
        }
    }

    /** Re-reads the settings after a write that may have partly succeeded. */
    private async reloadSettings(): Promise<void> {
        try {
            this.settings = adoptSettings(await SettingsService.Get());
        } catch {
            // Already reporting the failure that got us here.
        }
    }

    private persistLayout = debounce(() => {
        SettingsService.SetLayout($state.snapshot(this.settings.layout))
            .then((saved) => {
                this.settings = adoptSettings(saved);
            })
            .catch((err: unknown) => this.fail(`Could not save layout: ${message(err)}`));
    }, 400);

    // ----- notices -------------------------------------------------------

    private fail(text: string): void {
        this.notice = { text, tone: 'error' };
    }

    inform(text: string): void {
        this.notice = { text, tone: 'info' };
    }

    dismissNotice(): void {
        this.notice = null;
    }
}

export const workspace = new Workspace();
