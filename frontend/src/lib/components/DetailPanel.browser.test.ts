import { beforeEach, expect, test, vi } from 'vitest';
import { page } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';
import DetailPanel from './DetailPanel.svelte';

vi.mock('../../../bindings/github.com/rogerwesterbo/k8sdockside', () => ({
    KubeconfigService: { Sync: vi.fn().mockResolvedValue([]), Files: vi.fn().mockResolvedValue([]) },
    ResourceService: {
        Describe: vi.fn().mockResolvedValue('Name: web'),
        ResourceYAML: vi.fn().mockResolvedValue('kind: Pod\n'),
        CheckYAML: vi.fn().mockResolvedValue({ valid: true, message: '', line: 0 }),
    },
    ActionService: {
        ObjectState: vi.fn().mockResolvedValue({ scalable: false, replicas: 0, cordoned: false, containers: [] }),
        Delete: vi.fn().mockResolvedValue(undefined),
        Scale: vi.fn().mockResolvedValue(undefined),
        Restart: vi.fn().mockResolvedValue(undefined),
        Cordon: vi.fn().mockResolvedValue(undefined),
        Drain: vi.fn().mockResolvedValue('drain-1'),
        CancelDrain: vi.fn(),
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
        SetContextPrefs: vi.fn().mockResolvedValue({}),
        SetTabOrder: vi.fn().mockResolvedValue({}),
        SetDock: vi.fn().mockResolvedValue({}),
        SetLayout: vi.fn().mockResolvedValue({}),
        SetPreferences: vi.fn().mockResolvedValue({}),
    },
}));

const { workspace } = await import('../state/workspace.svelte');
const { changes } = await import('../state/changes.svelte');
const { ResourceService } = await import('../../../bindings/github.com/rogerwesterbo/k8sdockside');

const PROD = '/home/u/.kube/prod::admin@prod';
const WEB = { contextId: PROD, kind: 'pods', namespace: 'default', name: 'web' };

/** Long enough for an effect to notice and its describe to answer. */
const settle = () => new Promise((r) => setTimeout(r, 150));

beforeEach(() => {
    workspace.closeAllDockTabs();
    workspace.closeDetail();
    vi.mocked(ResourceService.Describe).mockReset().mockResolvedValue('Name: web\nStatus: Running');
});

test('the panel offers to edit what it is describing', async () => {
    render(DetailPanel);
    await workspace.openDetail({ contextId: PROD, kind: 'pods', namespace: 'default', name: 'web' });

    await page.getByRole('button', { name: 'Edit' }).click();

    // The object is now open in the dock, under its own name.
    expect(workspace.dockTabs.map((t) => t.name)).toEqual(['web']);
    expect(workspace.dockOpen).toBe(true);
});

// A Helm release is not a Kubernetes object -- it is a Secret the backend
// decodes -- so there is nothing here for an editor to open.
test('a Helm release has no edit button', async () => {
    render(DetailPanel);
    await workspace.openDetail({
        contextId: PROD,
        kind: 'helmreleases',
        namespace: 'default',
        name: 'ingress-nginx',
    });

    await expect.element(page.getByRole('heading', { name: 'ingress-nginx' })).toBeVisible();
    expect(page.getByRole('button', { name: 'Edit' }).elements()).toHaveLength(0);
});

// The bug this exists for: the panel is describing an object, the object is
// edited in the dock beside it, and the panel goes on showing what the cluster
// had before the save.
test('an object written elsewhere brings the report up to date', async () => {
    render(DetailPanel);
    await workspace.openDetail(WEB);
    await expect.element(page.getByText('Status: Running')).toBeVisible();

    vi.mocked(ResourceService.Describe).mockResolvedValue('Name: web\nStatus: Pending');
    changes.changed(WEB);

    await expect.element(page.getByText('Status: Pending')).toBeVisible();
});

test('an object the panel is not describing leaves it alone', async () => {
    render(DetailPanel);
    await workspace.openDetail(WEB);
    await expect.element(page.getByText('Status: Running')).toBeVisible();

    changes.changed({ ...WEB, name: 'api' });
    await settle();

    expect(ResourceService.Describe).toHaveBeenCalledTimes(1);
});

// Re-reading must not put the panel back into its loading state: the report it
// already has is a moment out of date, and blanking it flickers on every save.
test('the report stays on screen while it is re-read', async () => {
    render(DetailPanel);
    await workspace.openDetail(WEB);

    let answer: (text: string) => void = () => {};
    vi.mocked(ResourceService.Describe).mockReturnValueOnce(
        new Promise<string>((resolve) => {
            answer = resolve;
        }) as never,
    );
    changes.changed(WEB);
    await settle();

    await expect.element(page.getByText('Status: Running')).toBeVisible();
    expect(page.getByText('Describing web').elements()).toHaveLength(0);

    answer('Name: web\nStatus: Pending');
    await expect.element(page.getByText('Status: Pending')).toBeVisible();
});
