import { beforeEach, expect, test, vi } from 'vitest';
import { page } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';
import Pane from './Pane.svelte';

// A pane renders the view its active tab names, so this mounts real tables.
// Stubbing the subscription is what lets them exist without a cluster behind
// them; these tests are about where a view sits, not about its rows.
vi.mock('../state/subscriptions', () => ({
    subscribe: vi.fn(() => ({ setNamespace: vi.fn(), close: vi.fn() })),
}));

// Dragging a view from one pane into another, which is the whole point of
// panes: where a thing sits is the user's answer, not the view type's.
//
// The real backend answers every settings write with the whole settings file,
// and the store adopts that answer whole. A mock answering `{}` says instead
// that every other section is empty, so a debounced write landing a quarter of
// a second into a test undoes whatever it had just set -- the dock folds itself
// back up, a preference goes back to its default -- which is a race the test
// loses about half the time. So the settings mock keeps what it is given and
// hands all of it back, the way the file does.
const settingsFile = vi.hoisted(() => {
    const saved: Record<string, unknown> = {};
    return {
        keep: (section: string) =>
            vi.fn((value: unknown) => {
                saved[section] = value;
                return Promise.resolve({ ...saved });
            }),
        keepPrefsFor: () =>
            vi.fn((contextId: string, prefs: unknown) => {
                saved.contexts = { ...(saved.contexts as object), [contextId]: prefs };
                return Promise.resolve({ ...saved });
            }),
    };
});
vi.mock('../../../bindings/github.com/rogerwesterbo/k8sdockside', () => ({
    HelmService: {
        Releases: vi.fn().mockResolvedValue({ kind: 'helmreleases', columns: [], rows: [], namespaced: true, error: '' }),
        Detail: vi.fn().mockResolvedValue({
            name: '', namespace: '', revision: 1, status: 'deployed',
            chart: '', chartName: '', chartVersion: '', appVersion: '',
            description: '', firstDeployed: '', updated: '', notes: '',
            values: '', userValues: '', resources: [], revisions: [],
        }),
        Tool: vi.fn().mockResolvedValue({ found: true, path: '/usr/bin/helm', version: 'v3.16.2', configured: false, reason: '' }),
        Upgrade: vi.fn().mockResolvedValue(''),
        Rollback: vi.fn().mockResolvedValue(''),
        Uninstall: vi.fn().mockResolvedValue(''),
        ChartVersions: vi.fn().mockResolvedValue([]),
    },
    KubeconfigService: { Sync: vi.fn().mockResolvedValue([]), Files: vi.fn().mockResolvedValue([]) },
    ResourceService: {
        Describe: vi.fn().mockResolvedValue(''),
        Namespaces: vi.fn().mockResolvedValue(['default']),
        ResourceYAML: vi.fn().mockResolvedValue('kind: Pod\n'),
        ApplyYAML: vi.fn().mockResolvedValue('kind: Pod\n'),
        CheckYAML: vi.fn().mockResolvedValue({ valid: true, message: '', line: 0 }),
    },
    LogService: {
        Containers: vi.fn().mockResolvedValue([]),
        Open: vi.fn().mockResolvedValue('logs-1'),
        Close: vi.fn(),
    },
    MetricsService: {
        Source: vi.fn().mockResolvedValue({ endpoint: {}, configured: '', available: false, error: '' }),
        SetEndpoint: vi.fn().mockResolvedValue({ endpoint: {}, configured: '', available: false, error: '' }),
        Rediscover: vi.fn().mockResolvedValue({ endpoint: {}, configured: '', available: false, error: '' }),
        Charts: vi.fn().mockResolvedValue({ source: { endpoint: {}, available: false, error: '', configured: '' }, charts: [], range: 60 }),
        Attachments: vi.fn().mockResolvedValue([]),
    },
    PluginService: {
        List: vi.fn().mockResolvedValue({ plugins: [], dir: '', folders: [], problems: [] }),
        Reload: vi.fn().mockResolvedValue({ plugins: [], dir: '', folders: [], problems: [] }),
        Summary: vi.fn().mockResolvedValue({ pluginId: '', installed: false, checked: true, requirements: [], cards: [], error: '' }),
    },
    ThemeService: {
        List: vi.fn().mockResolvedValue({ themes: [], dir: '', folders: [], problems: [] }),
        Tokens: vi.fn().mockResolvedValue([]),
    },
    TerminalService: {
        Containers: vi.fn().mockResolvedValue([]),
        Open: vi.fn().mockResolvedValue({ id: 'term-1', namespace: 'default', pod: 'web', container: 'app', node: '' }),
        OpenNode: vi.fn().mockResolvedValue({ id: 'term-1', namespace: 'default', pod: '', container: '', node: 'wrkr01' }),
        Send: vi.fn(),
        Resize: vi.fn(),
        Close: vi.fn(),
        Externals: vi.fn().mockResolvedValue({ terminals: [], kubectl: '', reason: '' }),
        Launch: vi.fn().mockResolvedValue(undefined),
        LaunchNode: vi.fn().mockResolvedValue(undefined),
    },
    PortForwardService: {
        List: vi.fn().mockResolvedValue([]),
        Ports: vi.fn().mockResolvedValue([]),
        Start: vi.fn().mockResolvedValue({ id: 'pf-1', localPort: 51234, state: 'active' }),
        Reconnect: vi.fn().mockResolvedValue({ id: 'pf-1', localPort: 51234, state: 'active' }),
        Stop: vi.fn(),
        Forget: vi.fn().mockResolvedValue(undefined),
        Open: vi.fn().mockResolvedValue(undefined),
        URL: vi.fn().mockResolvedValue(''),
    },
    SettingsService: {
        Get: vi.fn().mockResolvedValue({}),
        ConfigPath: vi.fn().mockResolvedValue(''),
        SetContextPrefs: settingsFile.keepPrefsFor(),
        SetPanes: settingsFile.keep('panes'),
        SetLayout: settingsFile.keep('layout'),
        SetPreferences: settingsFile.keep('preferences'),
    },
}));

const { workspace } = await import('../state/workspace.svelte');

const PROD = '/home/u/.kube/prod::admin@prod';

function withClusters(): void {
    workspace.files = [
        {
            path: '/home/u/.kube/prod',
            source: 'manual',
            error: '',
            contexts: [{ id: PROD, name: 'admin@prod' } as never],
        },
    ];
}

const WEB = { contextId: PROD, kind: 'pods', namespace: 'default', name: 'web' };

/**
 * Drags one tab onto another pane's strip.
 *
 * Dispatched by hand rather than driven through the pointer: the strips are in
 * two separately rendered components here, and what is being checked is the
 * exchange between them -- that a dragstart in one is understood as a move by
 * the other -- rather than whether Chromium's own drag gesture works.
 */
function dragOnto(tab: Element, strip: Element): void {
    const dataTransfer = new DataTransfer();
    tab.dispatchEvent(new DragEvent('dragstart', { bubbles: true, cancelable: true, dataTransfer }));
    strip.dispatchEvent(new DragEvent('dragover', { bubbles: true, cancelable: true, dataTransfer }));
    strip.dispatchEvent(new DragEvent('drop', { bubbles: true, cancelable: true, dataTransfer }));
    tab.dispatchEvent(new DragEvent('dragend', { bubbles: true, cancelable: true, dataTransfer }));
}

const stripFor = (label: string) => document.querySelector(`[role="tablist"][aria-label="${label}"]`)!;

beforeEach(() => {
    document.body.innerHTML = '';
    workspace.closeAllTabsIn('main');
    workspace.closeAllTabsIn('right');
    workspace.closeAllTabsIn('bottom');
    withClusters();
});

test('an editor dragged from the foot of the window into the middle goes there', async () => {
    render(Pane, { pane: 'main' });
    render(Pane, { pane: 'bottom' });
    workspace.openEditor(WEB);
    await expect.element(page.getByRole('tab', { name: /web/ })).toBeVisible();

    dragOnto(await page.getByRole('tab', { name: /web/ }).element(), stripFor('Open views'));

    expect(workspace.panes.bottom.tabs).toHaveLength(0);
    expect(workspace.panes.main.tabs.map((t) => t.name)).toEqual(['web']);
});

test('a list dragged out of the middle leaves it, and the pane it lands in shows it', async () => {
    render(Pane, { pane: 'main' });
    render(Pane, { pane: 'bottom' });
    workspace.openTab(PROD, 'pods');
    await expect.element(page.getByRole('tab', { name: /Pods/ })).toBeVisible();

    dragOnto(await page.getByRole('tab', { name: /Pods/ }).element(), stripFor('Dock'));

    expect(workspace.panes.main.tabs).toHaveLength(0);
    expect(workspace.panes.bottom.tabs.map((t) => t.kind)).toEqual(['pods']);
    // Landing in a pane brings it forward there, so the drop shows its result.
    expect(workspace.panes.bottom.activeId).toBe(workspace.panes.bottom.tabs[0].id);
});

// The move must not be mistaken for a reorder, which is what a drag within one
// strip is. Nothing should leave the pane it started in.
test('dragging a tab within its own strip still only reorders it', async () => {
    render(Pane, { pane: 'main' });
    workspace.openTab(PROD, 'pods');
    workspace.openTab(PROD, 'nodes');
    await expect.element(page.getByRole('tab', { name: /Nodes/ })).toBeVisible();

    dragOnto(await page.getByRole('tab', { name: /Nodes/ }).element(), stripFor('Open views'));

    expect(workspace.panes.main.tabs).toHaveLength(2);
    expect(workspace.panes.bottom.tabs).toHaveLength(0);
});

// The reason a tab id says what it shows rather than which one it is.
test('a moved editor is still the same editor', async () => {
    render(Pane, { pane: 'main' });
    render(Pane, { pane: 'bottom' });
    workspace.openEditor(WEB);
    await expect.element(page.getByRole('tab', { name: /web/ })).toBeVisible();
    const before = workspace.panes.bottom.tabs[0].id;

    dragOnto(await page.getByRole('tab', { name: /web/ }).element(), stripFor('Open views'));

    expect(workspace.panes.main.tabs[0].id).toBe(before);
    expect(workspace.isEditing(WEB)).toBe(true);
});
