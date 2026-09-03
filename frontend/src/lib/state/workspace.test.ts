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
    },
    SettingsService: {
        Get: vi.fn().mockResolvedValue({}),
        ConfigPath: vi.fn().mockResolvedValue(''),
        SetTabOrder: vi.fn().mockResolvedValue({}),
        SetLayout: vi.fn().mockResolvedValue({}),
    },
}));

const { workspace } = await import('./workspace.svelte');
const { ResourceService } = await import('../../../bindings/github.com/roger/k8sdockside');

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
