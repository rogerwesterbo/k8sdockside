import { beforeEach, describe, expect, test, vi } from 'vitest';

// The workspace talks to the Go side the moment it does anything, so the
// bindings are stubbed. What is under test is which tabs survive a close and
// where focus lands -- none of which involves a cluster.
vi.mock('../../../bindings/github.com/roger/k8sdockside', () => ({
    KubeconfigService: {
        Sync: vi.fn().mockResolvedValue([]),
        Files: vi.fn().mockResolvedValue([]),
    },
    ResourceService: {
        Describe: vi.fn().mockResolvedValue(''),
        Ping: vi.fn().mockResolvedValue(undefined),
        CustomResourceKinds: vi.fn().mockResolvedValue([]),
    },
    SettingsService: {
        Get: vi.fn().mockResolvedValue({}),
        ConfigPath: vi.fn().mockResolvedValue(''),
        SetTabOrder: vi.fn().mockResolvedValue({}),
        SetDock: vi.fn().mockResolvedValue({}),
        SetLayout: vi.fn().mockResolvedValue({}),
        SetPreferences: vi.fn().mockResolvedValue({}),
        SetContextPrefs: vi.fn().mockResolvedValue({}),
    },
}));

const { workspace, isSettingsTab } = await import('./workspace.svelte');
const { changes } = await import('./changes.svelte');
const { SETTINGS } = await import('../catalogue');
const { ResourceService, KubeconfigService, SettingsService } = await import(
    '../../../bindings/github.com/roger/k8sdockside',
);

const PROD = '/home/u/.kube/prod::admin@prod';
const STAGING = '/home/u/.kube/staging::admin@staging';

/** Opens tabs in order and returns their ids. */
function open(...pairs: [string, string][]): string[] {
    for (const [contextId, kind] of pairs) {
        workspace.openTab(contextId, kind);
    }
    return workspace.tabs.map((t) => t.id);
}

beforeEach(() => {
    workspace.closeAllTabs();
    workspace.health = {};
    vi.mocked(ResourceService.Ping).mockReset().mockResolvedValue(undefined);
    expect(workspace.tabs).toHaveLength(0);
});

describe('closeTab', () => {
    test('moves focus to the tab on the right', () => {
        const [, , third] = open([PROD, 'pods'], [PROD, 'nodes'], [PROD, 'services']);
        workspace.activateTab(workspace.tabs[1].id);

        workspace.closeTab(workspace.tabs[1].id);

        expect(workspace.tabs.map((t) => t.kind)).toEqual(['pods', 'services']);
        expect(workspace.activeTabId).toBe(third);
    });

    test('falls back to the left when the last tab closes', () => {
        const [first] = open([PROD, 'pods'], [PROD, 'nodes']);
        workspace.activateTab(workspace.tabs[1].id);

        workspace.closeTab(workspace.tabs[1].id);

        expect(workspace.activeTabId).toBe(first);
    });

    test('leaves focus alone when another tab closes', () => {
        const [first] = open([PROD, 'pods'], [PROD, 'nodes']);
        workspace.activateTab(first);

        workspace.closeTab(workspace.tabs[1].id);

        expect(workspace.activeTabId).toBe(first);
    });

    test('ignores a tab that is not open', () => {
        open([PROD, 'pods']);
        workspace.closeTab('nonsense');
        expect(workspace.tabs).toHaveLength(1);
    });
});

describe('closeOtherTabs', () => {
    test('keeps only the named tab and focuses it', () => {
        const [, second] = open([PROD, 'pods'], [PROD, 'nodes'], [STAGING, 'pods']);
        workspace.activateTab(workspace.tabs[0].id);

        workspace.closeOtherTabs(second);

        expect(workspace.tabs.map((t) => t.id)).toEqual([second]);
        expect(workspace.activeTabId).toBe(second);
    });

    test('scoped to a context, spares the other clusters', () => {
        open([PROD, 'pods'], [PROD, 'nodes'], [STAGING, 'pods'], [STAGING, 'nodes']);
        const keep = workspace.tabs[0].id;

        workspace.closeOtherTabs(keep, PROD);

        // Both staging tabs survive; only the other prod tab goes.
        expect(workspace.tabs.map((t) => `${t.contextId}#${t.kind}`)).toEqual([
            `${PROD}#pods`,
            `${STAGING}#pods`,
            `${STAGING}#nodes`,
        ]);
    });

    test('scoped close does not disturb focus on a spared cluster', () => {
        open([PROD, 'pods'], [PROD, 'nodes'], [STAGING, 'pods']);
        const stagingTab = workspace.tabs[2].id;
        workspace.activateTab(stagingTab);

        workspace.closeOtherTabs(workspace.tabs[0].id, PROD);

        expect(workspace.activeTabId).toBe(stagingTab);
    });
});

describe('closeAllTabs', () => {
    test('empties the strip and clears the active tab', () => {
        open([PROD, 'pods'], [STAGING, 'nodes']);

        workspace.closeAllTabs();

        expect(workspace.tabs).toEqual([]);
        expect(workspace.activeTabId).toBeNull();
    });

    test('scoped to a context, closes only that cluster', () => {
        open([PROD, 'pods'], [PROD, 'nodes'], [STAGING, 'pods']);

        workspace.closeAllTabs(PROD);

        expect(workspace.tabs.map((t) => t.contextId)).toEqual([STAGING]);
        expect(workspace.activeTabId).toBe(workspace.tabs[0].id);
    });

    test('closing a cluster whose tabs are all inactive leaves focus alone', () => {
        open([STAGING, 'pods'], [PROD, 'pods']);
        const stagingTab = workspace.tabs[0].id;
        workspace.activateTab(stagingTab);

        workspace.closeAllTabs(PROD);

        expect(workspace.activeTabId).toBe(stagingTab);
    });

    test('does nothing when the context has no tabs open', () => {
        open([PROD, 'pods']);
        const before = workspace.tabs.map((t) => t.id);

        workspace.closeAllTabs(STAGING);

        expect(workspace.tabs.map((t) => t.id)).toEqual(before);
    });
});

describe('openTab placement', () => {
    test('opens immediately right of the selected tab', () => {
        open([PROD, 'pods'], [PROD, 'nodes'], [PROD, 'services']);
        workspace.activateTab(workspace.tabs[0].id); // back to pods

        workspace.openTab(PROD, 'events');

        expect(workspace.tabs.map((t) => t.kind)).toEqual(['pods', 'events', 'nodes', 'services']);
    });

    test('the new tab becomes the selected one', () => {
        open([PROD, 'pods'], [PROD, 'nodes']);
        workspace.activateTab(workspace.tabs[0].id);

        workspace.openTab(PROD, 'events');

        expect(workspace.activeTabId).toBe(workspace.tabs[1].id);
        expect(workspace.tabs[1].kind).toBe('events');
    });

    test('a run of opens keeps the order they were opened in', () => {
        // Each new tab becomes selected, so inserting to its right leaves the
        // strip reading oldest to newest rather than reversing it.
        open([PROD, 'pods'], [PROD, 'nodes'], [PROD, 'services']);

        expect(workspace.tabs.map((t) => t.kind)).toEqual(['pods', 'nodes', 'services']);
    });

    test('appends when nothing is selected', () => {
        open([PROD, 'pods'], [PROD, 'nodes']);
        workspace.activateTab(workspace.tabs[0].id);
        workspace.closeAllTabs();

        workspace.openTab(PROD, 'services');
        // Deactivate without closing, the state a restored session starts in.
        workspace.activeTabId = null;
        workspace.openTab(PROD, 'events');

        expect(workspace.tabs.map((t) => t.kind)).toEqual(['services', 'events']);
    });

    test('re-opening a tab that is already open only focuses it', () => {
        open([PROD, 'pods'], [PROD, 'nodes'], [PROD, 'services']);
        const before = workspace.tabs.map((t) => t.id);
        workspace.activateTab(workspace.tabs[0].id);

        workspace.openTab(PROD, 'services');

        expect(workspace.tabs.map((t) => t.id)).toEqual(before);
        expect(workspace.activeTabId).toBe(before[2]);
    });

    test('opens beside the selected tab even when it belongs to another cluster', () => {
        open([PROD, 'pods'], [PROD, 'nodes']);
        workspace.activateTab(workspace.tabs[0].id);

        workspace.openTab(STAGING, 'pods');

        expect(workspace.tabs.map((t) => `${t.contextId}#${t.kind}`)).toEqual([
            `${PROD}#pods`,
            `${STAGING}#pods`,
            `${PROD}#nodes`,
        ]);
    });
});

describe('health', () => {
    /** A promise whose settling this test controls, to observe the in-flight state. */
    function withheld<T = undefined>() {
        let settle!: (value: T) => void;
        let fail!: (reason: unknown) => void;
        const promise = new Promise<T>((resolve, reject) => {
            settle = resolve;
            fail = reject;
        });
        return { promise, settle, fail };
    }

    test('a context nobody has looked at has no status', () => {
        expect(workspace.healthOf(PROD).status).toBe('unknown');
    });

    test('a probe in flight reads as checking', async () => {
        const gate = withheld();
        vi.mocked(ResourceService.Ping).mockReturnValueOnce(gate.promise as never);

        const probing = workspace.probe(PROD);
        expect(workspace.healthOf(PROD).status).toBe('checking');

        gate.settle(undefined);
        await probing;
    });

    test('a cluster that answers is connected', async () => {
        await workspace.probe(PROD);

        expect(workspace.healthOf(PROD).status).toBe('connected');
        expect(workspace.healthOf(PROD).message).toBe('');
    });

    test('a cluster that refuses is in error, and keeps the reason', async () => {
        vi.mocked(ResourceService.Ping).mockRejectedValueOnce(new Error('dial tcp: connection refused'));

        await workspace.probe(PROD);

        expect(workspace.healthOf(PROD).status).toBe('error');
        expect(workspace.healthOf(PROD).message).toBe('dial tcp: connection refused');
    });

    test('probing a context whose status is already known does not ask again', async () => {
        await workspace.probe(PROD);
        await workspace.probe(PROD);

        expect(ResourceService.Ping).toHaveBeenCalledTimes(1);
    });

    test('a forced probe asks again, so refresh can recheck', async () => {
        await workspace.probe(PROD);
        await workspace.probe(PROD, { force: true });

        expect(ResourceService.Ping).toHaveBeenCalledTimes(2);
    });

    test('two probes racing on one context only produce one request', async () => {
        const gate = withheld();
        vi.mocked(ResourceService.Ping).mockReturnValueOnce(gate.promise as never);

        const first = workspace.probe(PROD);
        const second = workspace.probe(PROD);
        gate.settle(undefined);
        await Promise.all([first, second]);

        expect(ResourceService.Ping).toHaveBeenCalledTimes(1);
    });

    test('a tab reporting a failure turns the indicator red without a second request', () => {
        workspace.reportHealth(PROD, 'error', 'the server could not find the requested resource');

        expect(workspace.healthOf(PROD).status).toBe('error');
        expect(workspace.healthOf(PROD).message).toBe('the server could not find the requested resource');
        expect(ResourceService.Ping).not.toHaveBeenCalled();
    });

    test('a tab that loads reports the context as connected', () => {
        workspace.reportHealth(PROD, 'connected');

        expect(workspace.healthOf(PROD).status).toBe('connected');
    });

    test('a tab outcome overrides an earlier probe, being the newer evidence', async () => {
        await workspace.probe(PROD);
        expect(workspace.healthOf(PROD).status).toBe('connected');

        workspace.reportHealth(PROD, 'error', 'connection refused');

        expect(workspace.healthOf(PROD).status).toBe('error');
    });

    test('opening a tab probes the cluster it belongs to', () => {
        workspace.openTab(PROD, 'pods');

        expect(ResourceService.Ping).toHaveBeenCalledWith(PROD);
    });

    test('selecting a context in the sidebar probes it', () => {
        workspace.selectContext(STAGING);

        expect(ResourceService.Ping).toHaveBeenCalledWith(STAGING);
    });

    test('a context that leaves the kubeconfig loses its status', async () => {
        await workspace.probe(PROD);
        expect(workspace.healthOf(PROD).status).toBe('connected');

        // Sync is mocked to find nothing, so every context has gone.
        await workspace.sync();

        expect(workspace.health).toEqual({});
    });
});

describe('expanding and collapsing every context', () => {
    /** Puts two contexts on the store, since expandAll works off the real list. */
    function seed(): void {
        workspace.files = [
            {
                path: '/home/u/.kube/prod',
                source: 'manual',
                error: '',
                contexts: [
                    {
                        id: PROD,
                        name: 'admin@prod',
                        cluster: 'prod',
                        user: 'admin',
                        namespace: '',
                        server: '',
                        file: '/home/u/.kube/prod',
                        current: false,
                    },
                ],
            },
            {
                path: '/home/u/.kube/staging',
                source: 'manual',
                error: '',
                contexts: [
                    {
                        id: STAGING,
                        name: 'admin@staging',
                        cluster: 'staging',
                        user: 'admin',
                        namespace: '',
                        server: '',
                        file: '/home/u/.kube/staging',
                        current: false,
                    },
                ],
            },
        ];
    }

    test('expandAll opens every context in the sidebar', () => {
        seed();
        workspace.collapseAll();

        workspace.expandAll();

        expect(workspace.isExpanded(PROD)).toBe(true);
        expect(workspace.isExpanded(STAGING)).toBe(true);
    });

    // The whole reason probing is lazy is that building a client can run an
    // exec credential plugin. Unfolding the tree must not be a way to set
    // twenty of those going at once.
    test('expandAll probes nothing, because unfolding is not touching a cluster', () => {
        seed();
        workspace.collapseAll();

        workspace.expandAll();

        expect(ResourceService.Ping).not.toHaveBeenCalled();
    });

    test('collapseAll closes everything', () => {
        seed();
        workspace.expandAll();

        workspace.collapseAll();

        expect(workspace.isExpanded(PROD)).toBe(false);
        expect(workspace.isExpanded(STAGING)).toBe(false);
        expect(workspace.expanded).toEqual([]);
    });

    test('anyExpanded drives which way the toggle goes', () => {
        seed();
        workspace.collapseAll();
        expect(workspace.anyExpanded).toBe(false);

        workspace.expandAll();
        expect(workspace.anyExpanded).toBe(true);
    });

    test('one open context is enough for the toggle to offer collapsing', () => {
        seed();
        workspace.collapseAll();

        workspace.toggleExpanded(PROD);

        expect(workspace.anyExpanded).toBe(true);
    });

    test('expandAll leaves contexts that are already open alone', () => {
        seed();
        workspace.collapseAll();
        workspace.toggleExpanded(PROD);

        workspace.expandAll();

        // No duplicate entry for the one that was already open.
        expect(workspace.expanded.filter((id) => id === PROD)).toHaveLength(1);
    });
});

describe('revealing the context a tab belongs to', () => {
    test('activating a tab asks for its context to be revealed', () => {
        open([PROD, 'pods']);
        workspace.openTab(STAGING, 'nodes');

        workspace.activateTab(`${PROD}#pods`);

        expect(workspace.reveal?.contextId).toBe(PROD);
    });

    // Clicking the tab you are already on is how you ask "where is this
    // cluster?", so it has to reveal again rather than do nothing.
    test('re-activating the same tab reveals again', () => {
        open([PROD, 'pods']);
        workspace.activateTab(`${PROD}#pods`);
        const first = workspace.reveal?.nonce;

        workspace.activateTab(`${PROD}#pods`);

        expect(workspace.reveal?.nonce).not.toBe(first);
        expect(workspace.reveal?.contextId).toBe(PROD);
    });

    // Selecting in the sidebar means the user is already pointing at the row;
    // scrolling it would move what is under their cursor.
    test('selecting a context in the sidebar does not ask for a reveal', () => {
        workspace.reveal = null;

        workspace.selectContext(STAGING);

        expect(workspace.reveal).toBeNull();
    });

    test('the nonce climbs, so consecutive reveals are always distinguishable', () => {
        open([PROD, 'pods'], [STAGING, 'pods']);
        const seen = new Set<number>();

        for (const tab of [`${PROD}#pods`, `${STAGING}#pods`, `${PROD}#pods`]) {
            workspace.activateTab(tab);
            seen.add(workspace.reveal!.nonce);
        }

        expect(seen.size).toBe(3);
    });
});

describe('the default folding', () => {
    beforeEach(() => {
        workspace.settings.contexts = {};
    });

    test('the specialist groups start folded on a fresh install', () => {
        workspace.settings.layout.collapsedGroups = null;

        expect(workspace.isGroupCollapsed(PROD, 'Gateway API')).toBe(true);
        expect(workspace.isGroupCollapsed(PROD, 'Admission')).toBe(true);
        expect(workspace.isGroupCollapsed(PROD, 'Scheduling')).toBe(true);
    });

    // Everything starts folded now: expanding a context shows the dashboard and
    // a list of headings, and you open the one you are after.
    test('every section starts folded, not just the specialist ones', () => {
        workspace.settings.layout.collapsedGroups = null;

        expect(workspace.isGroupCollapsed(PROD, 'Workloads')).toBe(true);
        expect(workspace.isGroupCollapsed(PROD, 'Cluster')).toBe(true);
        expect(workspace.isGroupCollapsed(PROD, 'Config')).toBe(true);
    });

    // The distinction the store goes to trouble to preserve: an empty list is a
    // choice, and re-applying the defaults over it would undo the user's work
    // every time they restarted.
    test('an explicitly empty list means nothing is folded, not "use the defaults"', () => {
        workspace.settings.layout.collapsedGroups = [];

        expect(workspace.isGroupCollapsed(PROD, 'Gateway API')).toBe(false);
        expect(workspace.isGroupCollapsed(PROD, 'Admission')).toBe(false);
    });

    test('a stored list is used exactly as it is', () => {
        workspace.settings.layout.collapsedGroups = ['Workloads'];

        expect(workspace.isGroupCollapsed(PROD, 'Workloads')).toBe(true);
        expect(workspace.isGroupCollapsed(PROD, 'Gateway API')).toBe(false);
    });

    test('toggling folds an open group', () => {
        workspace.settings.layout.collapsedGroups = [];

        workspace.toggleGroup(PROD, 'Workloads');

        expect(workspace.isGroupCollapsed(PROD, 'Workloads')).toBe(true);
    });

    test('toggling unfolds a folded group', () => {
        workspace.settings.layout.collapsedGroups = ['Workloads'];

        workspace.toggleGroup(PROD, 'Workloads');

        expect(workspace.isGroupCollapsed(PROD, 'Workloads')).toBe(false);
    });

});

describe('per-context folding', () => {
    beforeEach(() => {
        workspace.settings.layout.collapsedGroups = ['Gateway API'];
        workspace.settings.contexts = {};
    });

    test('a context nobody has folded anything in follows the defaults', () => {
        expect(workspace.isGroupCollapsed(PROD, 'Gateway API')).toBe(true);
        expect(workspace.isGroupCollapsed(STAGING, 'Gateway API')).toBe(true);
    });

    // The whole point: folding a section is about the cluster you are looking
    // at, not about every cluster you have.
    test('folding a group changes only the context it was folded in', () => {
        workspace.toggleGroup(PROD, 'Network');

        expect(workspace.isGroupCollapsed(PROD, 'Network')).toBe(true);
        expect(workspace.isGroupCollapsed(STAGING, 'Network')).toBe(false);
    });

    test('unfolding a group changes only that context too', () => {
        workspace.toggleGroup(PROD, 'Gateway API');

        expect(workspace.isGroupCollapsed(PROD, 'Gateway API')).toBe(false);
        expect(workspace.isGroupCollapsed(STAGING, 'Gateway API')).toBe(true);
    });

    // A context that has been folded in keeps what it was given, even if it
    // happens to agree with the defaults: it has its own answer now, and a
    // later "apply everywhere" elsewhere should not quietly move it.
    test('a context keeps its own folding once it has one', () => {
        workspace.toggleGroup(PROD, 'Network');
        workspace.toggleGroup(PROD, 'Network');

        expect(workspace.hasFoldingOverride(PROD)).toBe(true);
        expect(workspace.isGroupCollapsed(PROD, 'Network')).toBe(false);
    });

    test('each context can end up folded differently', () => {
        // Baseline folds only Gateway API, so each toggle here folds something
        // new in one context and leaves the other where it was.
        workspace.toggleGroup(PROD, 'Admission');
        workspace.toggleGroup(STAGING, 'Workloads');

        expect(workspace.isGroupCollapsed(PROD, 'Admission')).toBe(true);
        expect(workspace.isGroupCollapsed(STAGING, 'Admission')).toBe(false);
        expect(workspace.isGroupCollapsed(STAGING, 'Workloads')).toBe(true);
        expect(workspace.isGroupCollapsed(PROD, 'Workloads')).toBe(false);
    });

    test('folding survives beside an alias and colour', () => {
        workspace.setContextPrefs(PROD, 'Prod', '#ff0000');

        workspace.toggleGroup(PROD, 'Admission');

        expect(workspace.settings.contexts[PROD].alias).toBe('Prod');
        expect(workspace.settings.contexts[PROD].color).toBe('#ff0000');
        expect(workspace.hasFoldingOverride(PROD)).toBe(true);
    });

    describe('applying one to every cluster', () => {
        test('sets the shared default and brings every context into line', () => {
            workspace.toggleGroup(PROD, 'Workloads');
            workspace.toggleGroup(STAGING, 'Network');

            workspace.toggleGroup(PROD, 'Admission', { allContexts: true });

            // Every context now shows the same thing, including the one that
            // had been folded differently a moment ago.
            expect(workspace.isGroupCollapsed(PROD, 'Admission')).toBe(true);
            expect(workspace.isGroupCollapsed(STAGING, 'Admission')).toBe(true);
            expect(workspace.isGroupCollapsed(STAGING, 'Network')).toBe(false);
        });

        test('clears the per-context folding, so nothing is left disagreeing', () => {
            workspace.toggleGroup(PROD, 'Workloads');
            expect(workspace.hasFoldingOverride(PROD)).toBe(true);

            workspace.toggleGroup(PROD, 'Network', { allContexts: true });

            expect(workspace.hasFoldingOverride(PROD)).toBe(false);
            expect(workspace.hasFoldingOverride(STAGING)).toBe(false);
        });

        test('reaches contexts that do not exist yet', () => {
            workspace.toggleGroup(PROD, 'Network', { allContexts: true });

            expect(workspace.isGroupCollapsed('/new/config::admin@new', 'Network')).toBe(true);
        });
    });
});

describe('zoom', () => {
    beforeEach(() => {
        workspace.settings.layout.zoom = 1;
    });

    test('starts at normal size', () => {
        expect(workspace.zoom).toBe(1);
    });

    test('zooming in and out steps the scale', () => {
        workspace.zoomIn();
        expect(workspace.zoom).toBeGreaterThan(1);

        workspace.zoomOut();
        expect(workspace.zoom).toBe(1);
    });

    test('reset returns to normal size from either direction', () => {
        workspace.zoomIn();
        workspace.zoomIn();
        workspace.resetZoom();
        expect(workspace.zoom).toBe(1);

        workspace.zoomOut();
        workspace.resetZoom();
        expect(workspace.zoom).toBe(1);
    });

    test('will not zoom out past the point the title bar stops fitting', () => {
        for (let i = 0; i < 40; i++) workspace.zoomOut();
        expect(workspace.zoom).toBeGreaterThanOrEqual(0.5);
    });

    test('will not zoom in without limit', () => {
        for (let i = 0; i < 40; i++) workspace.zoomIn();
        expect(workspace.zoom).toBeLessThanOrEqual(2);
    });

    test('setting the scale directly is clamped the same way', () => {
        // The settings view's slider and presets go through this, so it must
        // not be a way around the bounds the steppers respect.
        workspace.setZoom(9);
        expect(workspace.zoom).toBe(workspace.maxZoom);

        workspace.setZoom(0.01);
        expect(workspace.zoom).toBe(workspace.minZoom);

        workspace.setZoom(1.25);
        expect(workspace.zoom).toBe(1.25);
    });
});

describe('showing the section an activated tab lives in', () => {
    beforeEach(() => {
        workspace.closeAllTabs();
        workspace.settings.layout.collapsedGroups = ['Admission'];
        workspace.settings.contexts = {};
    });

    test('activating a tab unfolds the section its resource is listed under', () => {
        workspace.openTab(PROD, 'mutatingwebhookconfigurations');
        workspace.openTab(PROD, 'pods');
        // Fold it back with the tab already open, then return to that tab.
        workspace.toggleGroup(PROD, 'Admission');
        expect(workspace.isGroupCollapsed(PROD, 'Admission')).toBe(true);

        workspace.activateTab(`${PROD}#mutatingwebhookconfigurations`);

        expect(workspace.isGroupCollapsed(PROD, 'Admission')).toBe(false);
    });

    test('it unfolds only for the context the tab belongs to', () => {
        workspace.openTab(PROD, 'mutatingwebhookconfigurations');
        workspace.openTab(PROD, 'pods');
        workspace.toggleGroup(PROD, 'Admission');

        workspace.activateTab(`${PROD}#mutatingwebhookconfigurations`);

        expect(workspace.isGroupCollapsed(STAGING, 'Admission')).toBe(true);
    });

    // Activating a tab in a section that is already open must not quietly give
    // the context its own folding, or every tab click would pin it.
    test('a tab in an open section leaves the folding alone', () => {
        workspace.openTab(PROD, 'pods');

        workspace.activateTab(`${PROD}#pods`);

        expect(workspace.hasFoldingOverride(PROD)).toBe(false);
    });

    test('a custom resource tab is harmless, having no section', () => {
        workspace.openTab(PROD, 'crd:certificates.cert-manager.io');

        expect(() => workspace.activateTab(`${PROD}#crd:certificates.cert-manager.io`)).not.toThrow();
        expect(workspace.hasFoldingOverride(PROD)).toBe(false);
    });

    // The dashboard sits outside the sections, so activating its tab has
    // nothing to unfold -- and must not invent a folding for the context.
    test('the dashboard needs no section opened for it', () => {
        workspace.openTab(PROD, 'dashboard');
        workspace.openTab(PROD, 'pods');

        workspace.activateTab(`${PROD}#dashboard`);

        expect(workspace.hasFoldingOverride(PROD)).toBe(false);
    });
});

describe('folding every section of one context at once', () => {
    beforeEach(() => {
        workspace.settings.layout.collapsedGroups = [];
        workspace.settings.contexts = {};
    });

    test('collapsing shuts every section for that context', () => {
        workspace.collapseAllGroups(PROD);

        expect(workspace.isGroupCollapsed(PROD, 'Workloads')).toBe(true);
        expect(workspace.isGroupCollapsed(PROD, 'Network')).toBe(true);
        expect(workspace.anyGroupOpen(PROD)).toBe(false);
    });

    test('expanding opens every section for that context', () => {
        workspace.collapseAllGroups(PROD);

        workspace.expandAllGroups(PROD);

        expect(workspace.isGroupCollapsed(PROD, 'Workloads')).toBe(false);
        expect(workspace.anyGroupOpen(PROD)).toBe(true);
    });

    test('it is one context at a time, like folding a single section', () => {
        workspace.collapseAllGroups(PROD);

        expect(workspace.isGroupCollapsed(STAGING, 'Workloads')).toBe(false);
    });

    test('anyGroupOpen reports whether there is anything to collapse', () => {
        workspace.collapseAllGroups(PROD);
        expect(workspace.anyGroupOpen(PROD)).toBe(false);

        workspace.toggleGroup(PROD, 'Network');
        expect(workspace.anyGroupOpen(PROD)).toBe(true);
    });
});

describe('the custom resource definitions a cluster serves', () => {
    const GROUPS = [
        { group: 'cert-manager.io', kinds: [
            { kind: 'crd:certificates.cert-manager.io', label: 'Certificate', group: 'cert-manager.io', plural: 'certificates', scoped: true },
        ] },
        { group: 'vitistack.io', kinds: [
            { kind: 'crd:machines.vitistack.io', label: 'Machine', group: 'vitistack.io', plural: 'machines', scoped: true },
        ] },
    ];

    beforeEach(() => {
        workspace.customKinds = {};
        vi.mocked(ResourceService.CustomResourceKinds).mockReset().mockResolvedValue(GROUPS as never);
    });

    test('a context nobody has opened the section for has asked for nothing', () => {
        expect(workspace.customKindsFor(PROD).status).toBe('idle');
        expect(ResourceService.CustomResourceKinds).not.toHaveBeenCalled();
    });

    test('loading reports progress and then the groups', async () => {
        const loading = workspace.loadCustomKinds(PROD);
        expect(workspace.customKindsFor(PROD).status).toBe('loading');

        await loading;

        expect(workspace.customKindsFor(PROD).status).toBe('ready');
        expect(workspace.customKindsFor(PROD).groups.map((g) => g.group)).toEqual([
            'cert-manager.io',
            'vitistack.io',
        ]);
    });

    test('a failure is kept so the section can explain itself', async () => {
        vi.mocked(ResourceService.CustomResourceKinds).mockRejectedValueOnce(new Error('forbidden'));

        await workspace.loadCustomKinds(PROD);

        expect(workspace.customKindsFor(PROD).status).toBe('error');
        expect(workspace.customKindsFor(PROD).message).toBe('forbidden');
    });

    // Opening and closing the section repeatedly must not re-ask the cluster.
    test('a context already loaded is not asked again', async () => {
        await workspace.loadCustomKinds(PROD);
        await workspace.loadCustomKinds(PROD);

        expect(ResourceService.CustomResourceKinds).toHaveBeenCalledTimes(1);
    });

    test('a forced reload asks again, so a newly installed operator shows up', async () => {
        await workspace.loadCustomKinds(PROD);
        await workspace.loadCustomKinds(PROD, { force: true });

        expect(ResourceService.CustomResourceKinds).toHaveBeenCalledTimes(2);
    });

    test('each context is loaded separately', async () => {
        await workspace.loadCustomKinds(PROD);

        expect(workspace.customKindsFor(STAGING).status).toBe('idle');
    });

    // Without this a failure is permanent: the section keys off state, and an
    // errored context is not idle, so nothing would ask again.
    test('a sync re-reads the definitions of contexts that had them', async () => {
        vi.mocked(ResourceService.CustomResourceKinds).mockRejectedValueOnce(new Error('unreachable'));
        await workspace.loadCustomKinds(PROD);
        expect(workspace.customKindsFor(PROD).status).toBe('error');

        vi.mocked(KubeconfigService.Sync).mockResolvedValueOnce([
            { path: '/c', source: 'manual', error: '', contexts: [{ id: PROD, name: 'p', cluster: 'c', user: 'u', namespace: '', server: '', file: '/c', current: false }] },
        ] as never);
        await workspace.sync();
        await vi.waitFor(() => expect(workspace.customKindsFor(PROD).status).toBe('ready'));
    });

    test('a sync does not start reading for contexts nobody asked about', async () => {
        vi.mocked(KubeconfigService.Sync).mockResolvedValueOnce([
            { path: '/c', source: 'manual', error: '', contexts: [{ id: STAGING, name: 's', cluster: 'c', user: 'u', namespace: '', server: '', file: '/c', current: false }] },
        ] as never);

        await workspace.sync();

        expect(ResourceService.CustomResourceKinds).not.toHaveBeenCalled();
    });

    test('a context that leaves the kubeconfig loses what was loaded for it', async () => {
        await workspace.loadCustomKinds(PROD);
        expect(workspace.customKindsFor(PROD).status).toBe('ready');

        await workspace.sync();

        expect(workspace.customKindsFor(PROD).status).toBe('idle');
    });
});

describe('expanding an API group inside the definitions section', () => {
    beforeEach(() => {
        workspace.expandedApiGroups = [];
    });

    test('groups start closed', () => {
        expect(workspace.isApiGroupExpanded(PROD, 'vitistack.io')).toBe(false);
    });

    test('opening one leaves the others closed', () => {
        workspace.toggleApiGroup(PROD, 'vitistack.io');

        expect(workspace.isApiGroupExpanded(PROD, 'vitistack.io')).toBe(true);
        expect(workspace.isApiGroupExpanded(PROD, 'cert-manager.io')).toBe(false);
    });

    test('the same group in another context is its own', () => {
        workspace.toggleApiGroup(PROD, 'vitistack.io');

        expect(workspace.isApiGroupExpanded(STAGING, 'vitistack.io')).toBe(false);
    });

    test('toggling again closes it', () => {
        workspace.toggleApiGroup(PROD, 'vitistack.io');
        workspace.toggleApiGroup(PROD, 'vitistack.io');

        expect(workspace.isApiGroupExpanded(PROD, 'vitistack.io')).toBe(false);
    });
});

describe('clicking a context name', () => {
    beforeEach(() => {
        workspace.expanded = [];
        workspace.selectedContextId = null;
    });

    test('opens a closed context and selects it', () => {
        workspace.activateContext(PROD);

        expect(workspace.isExpanded(PROD)).toBe(true);
        expect(workspace.selectedContextId).toBe(PROD);
    });

    // The complaint this fixes: a second click did nothing at all.
    test('clicking the one already open closes it again', () => {
        workspace.activateContext(PROD);

        workspace.activateContext(PROD);

        expect(workspace.isExpanded(PROD)).toBe(false);
    });

    test('closing it does not deselect it, so its settings stay put', () => {
        workspace.activateContext(PROD);

        workspace.activateContext(PROD);

        expect(workspace.selectedContextId).toBe(PROD);
    });

    test('it opens again on the next click', () => {
        workspace.activateContext(PROD);
        workspace.activateContext(PROD);

        workspace.activateContext(PROD);

        expect(workspace.isExpanded(PROD)).toBe(true);
    });

    // Clicking a different context means "show me this one", never "fold it":
    // folding what you just reached for would be the opposite of the intent.
    test('switching to another open context selects it rather than closing it', () => {
        workspace.activateContext(PROD);
        workspace.activateContext(STAGING);
        expect(workspace.isExpanded(STAGING)).toBe(true);

        // PROD is still open but no longer selected; clicking it should select.
        workspace.activateContext(PROD);

        expect(workspace.isExpanded(PROD)).toBe(true);
        expect(workspace.selectedContextId).toBe(PROD);
    });
});

describe('the settings tab', () => {
    /** One context on disk, so the sync-driven pruning has something to keep. */
    function seedProd(): void {
        workspace.files = [
            {
                path: '/home/u/.kube/prod',
                source: 'manual',
                error: '',
                contexts: [
                    {
                        id: PROD,
                        name: 'admin@prod',
                        cluster: 'prod',
                        user: 'admin',
                        namespace: '',
                        server: '',
                        file: '/home/u/.kube/prod',
                        current: false,
                    },
                ],
            },
        ];
    }

    beforeEach(() => {
        workspace.files = [];
    });

    test('opens once, however many times it is asked for', () => {
        workspace.openSettings();
        workspace.openSettings();

        expect(workspace.tabs.filter((t) => t.kind === SETTINGS)).toHaveLength(1);
        expect(workspace.activeTab?.kind).toBe(SETTINGS);
    });

    test('goes to the end of the strip rather than beside the current tab', () => {
        open([PROD, 'pods'], [PROD, 'nodes']);
        workspace.activateTab(workspace.tabs[0].id);

        workspace.openSettings();

        expect(workspace.tabs.at(-1)?.kind).toBe(SETTINGS);
    });

    test('carries no context, so it is not painted as a cluster', () => {
        workspace.openSettings();
        const tab = workspace.tabs.find((t) => t.kind === SETTINGS);

        expect(tab?.contextId).toBe('');
        expect(isSettingsTab(tab!)).toBe(true);
        // Distinct from what any real context would be given.
        expect(workspace.colorOf('')).not.toBe(workspace.colorOf(PROD));
    });

    test('activating it leaves the sidebar on the context it was showing', () => {
        seedProd();
        open([PROD, 'pods']);
        expect(workspace.selectedContextId).toBe(PROD);

        workspace.openSettings();

        expect(workspace.activeTab?.kind).toBe(SETTINGS);
        expect(workspace.selectedContextId).toBe(PROD);
    });

    test('survives a sync that drops every cluster', async () => {
        seedProd();
        open([PROD, 'pods']);
        workspace.openSettings();

        // Every kubeconfig has gone.
        vi.mocked(KubeconfigService.Sync).mockResolvedValueOnce([]);
        await workspace.sync();

        expect(workspace.tabs.map((t) => t.kind)).toEqual([SETTINGS]);
    });

    test('closes with "close all tabs", but not with a context-scoped close', () => {
        seedProd();
        open([PROD, 'pods']);
        workspace.openSettings();

        workspace.closeAllTabs(PROD);
        expect(workspace.tabs.map((t) => t.kind)).toEqual([SETTINGS]);

        workspace.closeAllTabs();
        expect(workspace.tabs).toHaveLength(0);
    });
});

describe('preferences', () => {
    beforeEach(() => {
        workspace.settings.preferences = {
            theme: 'system',
            density: 'comfortable',
            restoreTabs: true,
            confirmSourceRemoval: false,
            showKubeconfigNames: false,
            showLineNumbers: true,
        };
    });

    test('a chosen theme is used as-is', () => {
        workspace.setTheme('light');
        expect(workspace.resolvedTheme).toBe('light');

        workspace.setTheme('dark');
        expect(workspace.resolvedTheme).toBe('dark');
    });

    test('system follows what the OS is asking for', () => {
        workspace.setTheme('system');

        workspace.systemPrefersDark = true;
        expect(workspace.resolvedTheme).toBe('dark');

        workspace.systemPrefersDark = false;
        expect(workspace.resolvedTheme).toBe('light');
    });

    test('a chosen theme ignores the OS', () => {
        workspace.setTheme('light');
        workspace.systemPrefersDark = true;

        expect(workspace.resolvedTheme).toBe('light');
    });

    test('turning tab restore off is a choice that sticks', () => {
        workspace.setRestoreTabs(false);
        // `??` rather than `||` on the way in: false must not read as unset.
        expect(workspace.restoreTabsOnLaunch).toBe(false);
    });
});

describe('restoring tabs at launch', () => {
    beforeEach(() => {
        workspace.files = [
            {
                path: '/home/u/.kube/prod',
                source: 'manual',
                error: '',
                contexts: [
                    {
                        id: PROD,
                        name: 'admin@prod',
                        cluster: 'prod',
                        user: 'admin',
                        namespace: '',
                        server: '',
                        file: '/home/u/.kube/prod',
                        current: false,
                    },
                ],
            },
        ];
        vi.mocked(KubeconfigService.Sync).mockResolvedValue(workspace.files);
    });

    test('reopens last session, settings tab included', async () => {
        workspace.settings.tabOrder = [
            { contextId: PROD, kind: 'pods' },
            { contextId: '', kind: SETTINGS },
        ];
        workspace.settings.preferences.restoreTabs = true;

        await workspace.sync({ restoreTabs: true });

        expect(workspace.tabs.map((t) => t.kind)).toEqual(['pods', SETTINGS]);
    });

    test('drops a remembered tab whose cluster has gone, but keeps settings', async () => {
        workspace.settings.tabOrder = [
            { contextId: STAGING, kind: 'pods' },
            { contextId: '', kind: SETTINGS },
        ];
        workspace.settings.preferences.restoreTabs = true;

        await workspace.sync({ restoreTabs: true });

        expect(workspace.tabs.map((t) => t.kind)).toEqual([SETTINGS]);
    });

    test('turned off, it starts empty but leaves the remembered order alone', async () => {
        const order = [{ contextId: PROD, kind: 'pods' }];
        workspace.settings.tabOrder = order;
        workspace.settings.preferences.restoreTabs = false;

        await workspace.sync({ restoreTabs: true });

        expect(workspace.tabs).toHaveLength(0);
        // The order is what turning it back on has to restore, so the launch
        // that skipped it must not have overwritten it.
        expect(workspace.settings.tabOrder).toEqual(order);
    });

    test('a settings tab restored first does not select a context', async () => {
        workspace.settings.tabOrder = [{ contextId: '', kind: SETTINGS }];
        workspace.settings.preferences.restoreTabs = true;
        workspace.selectedContextId = null;

        await workspace.sync({ restoreTabs: true });

        expect(workspace.activeTab?.kind).toBe(SETTINGS);
        // ensureSelection may still pick one for the sidebar; what must not
        // happen is the settings tab claiming the empty id as a context.
        expect(workspace.selectedContextId).not.toBe('');
    });
});

describe('the dock', () => {
    /** The identity of one object, as the detail panel hands it over. */
    function object(contextId: string, name: string, namespace = 'default', kind = 'pods') {
        return { contextId, kind, namespace, name };
    }

    beforeEach(() => {
        workspace.closeAllDockTabs();
        workspace.settings.dock.open = false;
        expect(workspace.dockTabs).toHaveLength(0);
    });

    test('editing an object opens it, focuses it and unfolds the dock', () => {
        workspace.openEditor(object(PROD, 'web'));

        expect(workspace.dockTabs.map((t) => t.title)).toEqual(['web']);
        expect(workspace.activeDockTab?.name).toBe('web');
        expect(workspace.dockOpen).toBe(true);
        expect(workspace.isEditing(object(PROD, 'web'))).toBe(true);
    });

    test('editing the same object again focuses the tab it already has', () => {
        workspace.openEditor(object(PROD, 'web'));
        workspace.openEditor(object(PROD, 'api'));
        workspace.openEditor(object(PROD, 'web'));

        expect(workspace.dockTabs).toHaveLength(2);
        expect(workspace.activeDockTab?.name).toBe('web');
    });

    // A name is only unique within a namespace, and two clusters can both have
    // a "web". Either would otherwise reopen the other's document.
    test('the same name in another namespace or cluster is another tab', () => {
        workspace.openEditor(object(PROD, 'web', 'default'));
        workspace.openEditor(object(PROD, 'web', 'kube-system'));
        workspace.openEditor(object(STAGING, 'web', 'default'));

        expect(workspace.dockTabs).toHaveLength(3);
    });

    test('reopening the tab you are on does not fold the dock away', () => {
        workspace.openEditor(object(PROD, 'web'));
        workspace.openEditor(object(PROD, 'web'));

        expect(workspace.dockOpen).toBe(true);
    });

    test('clicking the tab you are on folds the dock, and again brings it back', () => {
        workspace.openEditor(object(PROD, 'web'));
        const id = workspace.activeDockTabId!;

        workspace.activateDockTab(id);
        expect(workspace.dockOpen).toBe(false);
        // The tab is still there and still the one selected -- only the room
        // it was taking has gone back.
        expect(workspace.activeDockTabId).toBe(id);

        workspace.activateDockTab(id);
        expect(workspace.dockOpen).toBe(true);
    });

    test('closing moves focus to the right, then to the left', () => {
        workspace.openEditor(object(PROD, 'one'));
        workspace.openEditor(object(PROD, 'two'));
        workspace.openEditor(object(PROD, 'three'));
        workspace.activateDockTab(workspace.dockTabs[1].id);

        workspace.closeDockTab(workspace.dockTabs[1].id);
        expect(workspace.activeDockTab?.name).toBe('three');

        workspace.closeDockTab(workspace.dockTabs[1].id);
        expect(workspace.activeDockTab?.name).toBe('one');
    });

    test('closing the last tab folds the dock away', () => {
        workspace.openEditor(object(PROD, 'web'));

        workspace.closeDockTab(workspace.dockTabs[0].id);

        expect(workspace.dockTabs).toHaveLength(0);
        expect(workspace.activeDockTabId).toBeNull();
        expect(workspace.dockOpen).toBe(false);
    });

    test('closing others can be scoped to one cluster', () => {
        workspace.openEditor(object(PROD, 'one'));
        workspace.openEditor(object(PROD, 'two'));
        workspace.openEditor(object(STAGING, 'three'));
        const keep = workspace.dockTabs[0].id;

        workspace.closeOtherDockTabs(keep, PROD);

        expect(workspace.dockTabs.map((t) => t.name)).toEqual(['one', 'three']);
    });

    test('closing all can be scoped to one cluster', () => {
        workspace.openEditor(object(PROD, 'one'));
        workspace.openEditor(object(STAGING, 'two'));

        workspace.closeAllDockTabs(PROD);

        expect(workspace.dockTabs.map((t) => t.contextId)).toEqual([STAGING]);
    });

    test('tabs are dragged into order', () => {
        workspace.openEditor(object(PROD, 'one'));
        workspace.openEditor(object(PROD, 'two'));

        workspace.moveDockTab(1, 0);
        expect(workspace.dockTabs.map((t) => t.name)).toEqual(['two', 'one']);

        // Off the end is not a move, and must not drop the tab.
        workspace.moveDockTab(0, 5);
        expect(workspace.dockTabs.map((t) => t.name)).toEqual(['two', 'one']);
    });

    // The point of the dock: what is open in it is a document you are part way
    // through, and looking at something else must not close it.
    test('the dock is untouched by what happens to the tabs above it', () => {
        open([PROD, 'pods'], [STAGING, 'nodes']);
        workspace.openEditor(object(PROD, 'web'));

        workspace.activateTab(workspace.tabs[1].id);
        workspace.selectContext(STAGING);
        workspace.closeAllTabs();

        expect(workspace.dockTabs.map((t) => t.name)).toEqual(['web']);
        expect(workspace.dockOpen).toBe(true);
    });

    test('a cluster that leaves the kubeconfig takes its dock tabs with it', async () => {
        workspace.openEditor(object(PROD, 'web'));

        // Nothing on disk this time round, so every context has gone.
        vi.mocked(KubeconfigService.Sync).mockResolvedValueOnce([]);
        await workspace.sync();

        expect(workspace.dockTabs).toHaveLength(0);
        expect(workspace.dockOpen).toBe(false);
    });
});

describe('restoring the dock at launch', () => {
    beforeEach(() => {
        workspace.closeAllDockTabs();
        workspace.files = [
            {
                path: '/home/u/.kube/prod',
                source: 'manual',
                error: '',
                contexts: [
                    {
                        id: PROD,
                        name: 'admin@prod',
                        cluster: 'prod',
                        user: 'admin',
                        namespace: '',
                        server: '',
                        file: '/home/u/.kube/prod',
                        current: false,
                    },
                ],
            },
        ];
        vi.mocked(KubeconfigService.Sync).mockResolvedValue(workspace.files);
        workspace.settings.preferences.restoreTabs = true;
    });

    test('reopens the editors from last session', async () => {
        workspace.settings.dock.tabs = [
            { type: 'edit', contextId: PROD, kind: 'pods', namespace: 'default', name: 'web' },
        ];

        await workspace.sync({ restoreTabs: true });

        expect(workspace.dockTabs.map((t) => t.name)).toEqual(['web']);
        expect(workspace.activeDockTab?.namespace).toBe('default');
    });

    test('skips a tab whose cluster has gone, and a view this build does not have', async () => {
        workspace.settings.dock.tabs = [
            { type: 'edit', contextId: STAGING, kind: 'pods', namespace: 'default', name: 'gone' },
            { type: 'terminal', contextId: PROD, kind: 'pods', namespace: 'default', name: 'future' },
            { type: 'edit', contextId: PROD, kind: 'pods', namespace: 'default', name: 'web' },
        ];

        await workspace.sync({ restoreTabs: true });

        expect(workspace.dockTabs.map((t) => t.name)).toEqual(['web']);
    });

    test('turned off, the dock starts empty but the remembered tabs are left alone', async () => {
        const remembered = [
            { type: 'edit', contextId: PROD, kind: 'pods', namespace: 'default', name: 'web' },
        ];
        workspace.settings.dock.tabs = remembered;
        workspace.settings.preferences.restoreTabs = false;

        await workspace.sync({ restoreTabs: true });

        expect(workspace.dockTabs).toHaveLength(0);
        expect(workspace.settings.dock.tabs).toEqual(remembered);
    });
});

describe('saving settings', () => {
    // The bug this guards against: opening an editor adds a dock tab and
    // unfolds the dock, and any other settings write already on its way answers
    // with the file as it was before either happened. Adopting that answer
    // whole shut the dock again a quarter of a second after it opened.
    test('an answer from a write in flight does not roll back a later change', async () => {
        vi.useFakeTimers();
        let landed: (saved: unknown) => void = () => {};
        try {
            // The tab-order call answers with a file that predates the dock
            // being opened, which is what one in flight really carries...
            vi.mocked(SettingsService.SetTabOrder).mockResolvedValue({} as never);
            // ...while the dock's own call is still out, so its answer cannot
            // be what puts things right.
            vi.mocked(SettingsService.SetDock).mockReturnValue(
                new Promise((resolve) => {
                    landed = resolve;
                }) as never,
            );

            workspace.openTab(PROD, 'pods');
            const tab = workspace.activeTabId!;
            await vi.advanceTimersByTimeAsync(1000);

            // One gesture, two sections: the strip above loses a tab and the
            // dock below gains one.
            workspace.closeTab(tab);
            workspace.openEditor({ contextId: PROD, kind: 'pods', namespace: 'default', name: 'web' });
            await vi.advanceTimersByTimeAsync(1000);

            expect(workspace.dockTabs.map((t) => t.name)).toEqual(['web']);
            expect(workspace.dockOpen).toBe(true);
            // And what was sent is the dock as it actually stood.
            expect(SettingsService.SetDock).toHaveBeenCalledWith(
                expect.objectContaining({ open: true, tabs: [expect.objectContaining({ name: 'web' })] }),
            );
        } finally {
            landed({});
            vi.useRealTimers();
            vi.mocked(SettingsService.SetTabOrder).mockReset().mockResolvedValue({} as never);
            vi.mocked(SettingsService.SetDock).mockReset().mockResolvedValue({} as never);
        }
    });
});

describe('the detail panel', () => {
    const WEB = { contextId: PROD, kind: 'pods', namespace: 'default', name: 'web' };

    /** A describe that only answers when the returned function is called. */
    function heldDescribe(): (text: string) => void {
        let answer: (text: string) => void = () => {};
        vi.mocked(ResourceService.Describe).mockReturnValueOnce(
            new Promise<string>((resolve) => {
                answer = resolve;
            }) as never,
        );
        return answer!;
    }

    beforeEach(() => {
        workspace.closeDetail();
        vi.mocked(ResourceService.Describe).mockReset().mockResolvedValue('Name: web\nStatus: Running');
    });

    test('re-reading swaps the report for what the cluster has now', async () => {
        await workspace.openDetail(WEB);
        vi.mocked(ResourceService.Describe).mockResolvedValue('Name: web\nStatus: Pending');

        await workspace.refreshDetail();

        expect(workspace.detailText).toBe('Name: web\nStatus: Pending');
    });

    // Blanking to "Describing…" every time the object is saved makes the panel
    // flicker for as long as the cluster takes to answer. The report it already
    // has is a moment out of date, which is better than nothing at all.
    test('re-reading keeps the old report on screen until the new one lands', async () => {
        await workspace.openDetail(WEB);
        const answer = heldDescribe();

        const done = workspace.refreshDetail();

        expect(workspace.detailLoading).toBe(false);
        expect(workspace.detailText).toBe('Name: web\nStatus: Running');

        answer('Name: web\nStatus: Pending');
        await done;
        expect(workspace.detailText).toBe('Name: web\nStatus: Pending');
    });

    test('re-reading a closed panel touches no cluster', async () => {
        await workspace.refreshDetail();

        expect(ResourceService.Describe).not.toHaveBeenCalled();
    });

    // Two reads can be in flight at once now that a save can start one: the
    // slower must not be allowed to put the panel back to what it said before.
    test('a slow read overtaken by a newer one does not win', async () => {
        await workspace.openDetail(WEB);
        const slow = heldDescribe();

        const first = workspace.refreshDetail();
        vi.mocked(ResourceService.Describe).mockResolvedValue('Name: web\nStatus: Pending');
        await workspace.refreshDetail();
        slow('Name: web\nStatus: Running');
        await first;

        expect(workspace.detailText).toBe('Name: web\nStatus: Pending');
    });

    test('a read still in flight does not reopen a closed panel', async () => {
        const answer = heldDescribe();
        const opening = workspace.openDetail(WEB);

        workspace.closeDetail();
        answer('Name: web\nStatus: Running');
        await opening;

        expect(workspace.detailTarget).toBeNull();
        expect(workspace.detailText).toBe('');
    });

    // What the panel's report was read at. The panel compares it against the
    // object's revision to notice that a save has made it stale.
    test('opening it records the revision its report was read at', async () => {
        changes.changed(WEB);
        await workspace.openDetail(WEB);

        expect(workspace.detailRevision).toBe(changes.revision(WEB));
    });

    test('a save leaves the panel behind the object', async () => {
        await workspace.openDetail(WEB);

        changes.changed(WEB);

        expect(workspace.detailRevision).not.toBe(changes.revision(WEB));
    });

    test('re-reading catches it up', async () => {
        await workspace.openDetail(WEB);
        changes.changed(WEB);

        await workspace.refreshDetail();

        expect(workspace.detailRevision).toBe(changes.revision(WEB));
    });
});
