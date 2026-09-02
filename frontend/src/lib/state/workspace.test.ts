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
    },
    SettingsService: {
        Get: vi.fn().mockResolvedValue({}),
        ConfigPath: vi.fn().mockResolvedValue(''),
        SetTabOrder: vi.fn().mockResolvedValue({}),
        SetLayout: vi.fn().mockResolvedValue({}),
    },
}));

const { workspace } = await import('./workspace.svelte');

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
