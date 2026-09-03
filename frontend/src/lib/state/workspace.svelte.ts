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
import { DASHBOARD, labelFor } from '../catalogue';
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
    nonce: number;
}

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
        layout: { detailDock: 'right', detailSize: 520, sidebarWidth: 260 },
    };
}

function tabId(contextId: string, kind: string): string {
    return `${contextId}#${kind}`;
}

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
    /** The sidebar's standing request to scroll a context into view. */
    reveal = $state<Reveal | null>(null);
    private revealCount = 0;

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
            if (restoreTabs) {
                this.restoreTabs();
            } else {
                this.dropTabsForMissingContexts();
                this.recheckHealth();
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

    /** Stops watching a folder; its configs leave the sidebar with it. */
    async removeFolder(path: string): Promise<void> {
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

    /** The colour for a context: the user's choice, or one derived from its id. */
    colorOf(contextId: string): string {
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
        const prefs = { alias, color };
        if (!alias && !color) {
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

    /** Focuses a tab and selects the context it belongs to. */
    activateTab(id: string): void {
        const changed = this.activeTabId !== id;
        this.activeTabId = id;

        const tab = this.tabs.find((t) => t.id === id);
        if (tab) {
            this.selectContext(tab.contextId);
            // Asked for here rather than in selectContext, which the sidebar
            // also calls: a context clicked in the sidebar is already under the
            // pointer, and scrolling it would move it out from under them.
            this.reveal = { contextId: tab.contextId, nonce: ++this.revealCount };
        }
        // Only when the view actually changed: the panel describes an object in
        // the tab we just left, so keeping it open over a different one would
        // be misleading. Re-clicking the tab you are on should leave it alone.
        if (changed) {
            this.closeDetail();
        }
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
        const known = new Set(this.contexts.map((c) => c.id));
        this.tabs = this.settings.tabOrder
            .filter((ref) => known.has(ref.contextId))
            .map((ref) => ({
                id: tabId(ref.contextId, ref.kind),
                contextId: ref.contextId,
                kind: ref.kind,
                title: ref.kind === DASHBOARD ? 'Dashboard' : labelFor(ref.kind),
            }));
        this.activeTabId = this.tabs[0]?.id ?? null;
        if (this.tabs[0]) {
            this.selectContext(this.tabs[0].contextId);
        }
    }

    /** Drops tabs whose context disappeared from disk between syncs. */
    private dropTabsForMissingContexts(): void {
        const known = new Set(this.contexts.map((c) => c.id));
        const kept = this.tabs.filter((t) => known.has(t.contextId));
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

    /** Moves the detail panel to another edge of the window. */
    setDock(side: DockSide): void {
        this.settings.layout.detailDock = side;
        this.persistLayout();
    }

    setDetailSize(px: number): void {
        this.settings.layout.detailSize = Math.round(px);
        this.persistLayout();
    }

    setSidebarWidth(px: number): void {
        this.settings.layout.sidebarWidth = Math.round(px);
        this.persistLayout();
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
