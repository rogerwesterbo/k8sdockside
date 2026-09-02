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

/** A transient message shown in the status bar. */
export interface Notice {
    text: string;
    tone: 'info' | 'error';
}

function defaultSettings(): Settings {
    return {
        manualFiles: [],
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

    syncing = $state(false);
    loaded = $state(false);
    notice = $state<Notice | null>(null);
    configPath = $state('');

    contexts = $derived(this.files.flatMap((f) => f.contexts));
    activeTab = $derived(this.tabs.find((t) => t.id === this.activeTabId) ?? null);
    selectedContext = $derived(this.contexts.find((c) => c.id === this.selectedContextId) ?? null);
    /** Files that could not be parsed, surfaced in the sidebar rather than dropped. */
    brokenFiles = $derived(this.files.filter((f) => f.error !== ''));

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
            if (restoreTabs) {
                this.restoreTabs();
            } else {
                this.dropTabsForMissingContexts();
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
            this.fail(message(err));
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
    }

    toggleExpanded(contextId: string): void {
        this.expanded = this.expanded.includes(contextId)
            ? this.expanded.filter((id) => id !== contextId)
            : [...this.expanded, contextId];
    }

    isExpanded(contextId: string): boolean {
        return this.expanded.includes(contextId);
    }

    // ----- tabs ----------------------------------------------------------

    /** Opens a tab, or focuses it if this context/kind pair is already open. */
    openTab(contextId: string, kind: string): void {
        const id = tabId(contextId, kind);
        if (!this.tabs.some((t) => t.id === id)) {
            const title = kind === DASHBOARD ? 'Dashboard' : labelFor(kind);
            this.tabs = [...this.tabs, { id, contextId, kind, title }];
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
        const index = this.tabs.findIndex((t) => t.id === id);
        if (index === -1) return;

        this.tabs = this.tabs.filter((t) => t.id !== id);
        if (this.activeTabId === id) {
            const next = this.tabs[index] ?? this.tabs[index - 1] ?? null;
            this.activeTabId = next?.id ?? null;
            if (next) this.selectContext(next.contextId);
            this.closeDetail();
        }
        this.persistTabOrder();
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
