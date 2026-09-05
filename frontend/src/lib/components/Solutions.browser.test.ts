import { beforeEach, expect, test, vi } from 'vitest';
import { page } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';
import type { Plugin } from '../plugins/types';

// What is worth testing in a real browser here is the state the whole feature
// exists for: the plugin is installed on this machine and the solution is not
// in this cluster. The sidebar has to say so without hiding the plugin, and the
// overview has to explain it rather than showing four empty tables.

const ARGO: Plugin = {
    id: 'argocd',
    name: 'Argo CD',
    tagline: 'GitOps continuous delivery',
    icon: 'rocket',
    author: '',
    docs: 'https://argo-cd.readthedocs.io',
    description: 'Argo CD keeps a cluster matching what is in Git.',
    requires: [
        { kind: 'crd:applications.argoproj.io', label: 'Applications', optional: false },
        { kind: 'crd:applicationsets.argoproj.io', label: 'Application Sets', optional: true },
    ],
    views: [
        {
            id: 'applications',
            label: 'Applications',
            icon: 'rocket',
            type: 'table',
            kind: 'crd:applications.argoproj.io',
            namespace: '',
            selector: '',
        },
    ],
    origin: 'builtin',
    pack: '',
    disabled: false,
};

const Summary = vi.fn();

vi.mock('../../../bindings/github.com/rogerwesterbo/k8sdockside', () => ({
    KubeconfigService: { Sync: vi.fn().mockResolvedValue([]), Files: vi.fn().mockResolvedValue([]) },
    ResourceService: {
        Describe: vi.fn().mockResolvedValue(''),
        Ping: vi.fn().mockResolvedValue(undefined),
        CustomResourceKinds: vi.fn().mockResolvedValue([]),
    },
    LogService: { Containers: vi.fn().mockResolvedValue([]), Open: vi.fn(), Close: vi.fn() },
    ThemeService: {
        List: vi.fn().mockResolvedValue({ themes: [], dir: '', folders: [], problems: [] }),
        Tokens: vi.fn().mockResolvedValue([]),
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
        Summary,
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
        SetTabOrder: vi.fn().mockResolvedValue({}),
        SetLayout: vi.fn().mockResolvedValue({}),
        SetPreferences: vi.fn().mockResolvedValue({}),
        SetContextPrefs: vi.fn().mockResolvedValue({}),
    },
}));

const { workspace } = await import('../state/workspace.svelte');
const PluginOverview = (await import('./PluginOverview.svelte')).default;

const PROD = '/home/u/.kube/config::admin@prod';

const TOKENS = `:root{--bg:#10151c;--bg-sidebar:#151b24;--bg-panel:#19202a;--bg-raised:#212b38;
--border:#46536a;--border-soft:rgba(255,255,255,.05);--bg-hover:rgba(255,255,255,.08);
--bg-active:rgba(255,255,255,.13);--text:#e8eef7;--text-dim:#a9b6c6;--text-faint:#8593a3;
--accent:#4a86ff;--accent-text:#fff;--ok:#5fd39b;--warn:#efb567;--error:#f4787f;
--radius:6px;--radius-sm:4px;--row-h:30px;--mono:monospace;--font:sans-serif;
font-family:var(--font);font-size:13px}
body{color:var(--text);margin:0;width:900px}
button{font:inherit;color:inherit;background:none;border:none;padding:0;cursor:pointer}`;

/** A summary as the Go side hands one over. */
function summary(over: Partial<Record<string, unknown>> = {}) {
    return {
        pluginId: 'argocd',
        installed: true,
        checked: true,
        requirements: [
            { kind: 'crd:applications.argoproj.io', label: 'Applications', optional: false, served: true, error: '' },
            { kind: 'crd:applicationsets.argoproj.io', label: 'Application Sets', optional: true, served: false, error: '' },
        ],
        cards: [
            {
                label: 'Applications by health',
                kind: 'crd:applications.argoproj.io',
                total: 42,
                grouped: true,
                buckets: [
                    { value: 'Degraded', count: 1, tone: 'error' },
                    { value: 'Progressing', count: 3, tone: 'warn' },
                    { value: 'Healthy', count: 38, tone: 'ok' },
                ],
                error: '',
            },
        ],
        error: '',
        ...over,
    };
}

beforeEach(() => {
    document.body.innerHTML = '';
    const style = document.createElement('style');
    style.textContent = TOKENS;
    document.head.appendChild(style);

    Summary.mockReset().mockResolvedValue(summary());
    workspace.pluginCatalogue = { plugins: [structuredClone(ARGO)], dir: '', folders: [], problems: [] };
    workspace.closeAllTabs();
});

test('the overview leads with the counts when the solution is installed', async () => {
    render(PluginOverview, { contextId: PROD, kind: 'plugin:argocd/overview' });

    await expect.element(page.getByText('installed here')).toBeVisible();
    await expect.element(page.getByText('42')).toBeVisible();
    // Worst first: the one Degraded application is why the tile is on screen.
    await expect.element(page.getByText('Degraded')).toBeVisible();
});

// The state the whole feature exists for.
test('a cluster without the CRDs is explained, not left as an empty table', async () => {
    Summary.mockResolvedValue(
        summary({
            installed: false,
            requirements: [
                {
                    kind: 'crd:applications.argoproj.io',
                    label: 'Applications',
                    optional: false,
                    served: false,
                    error: '',
                },
            ],
            cards: [
                {
                    label: 'Applications by health',
                    kind: 'crd:applications.argoproj.io',
                    total: 0,
                    grouped: true,
                    buckets: [],
                    error: 'this cluster does not serve crd:applications.argoproj.io',
                },
            ],
        }),
    );

    render(PluginOverview, { contextId: PROD, kind: 'plugin:argocd/overview' });

    await expect.element(page.getByText('not installed here')).toBeVisible();
    await expect.element(page.getByText('does not look installed in this cluster')).toBeVisible();
    // The missing kind is named, so the reader knows what to go and install.
    await expect.element(page.getByText('crd:applications.argoproj.io').first()).toBeVisible();
});

// "Could not ask" and "asked and was told no" call for opposite reactions.
test('an unreachable cluster does not read as "not installed"', async () => {
    Summary.mockResolvedValue(
        summary({
            installed: false,
            checked: false,
            error: 'dial tcp 10.0.0.1:6443: connect: connection refused',
            requirements: [
                {
                    kind: 'crd:applications.argoproj.io',
                    label: 'Applications',
                    optional: false,
                    served: false,
                    error: 'connection refused',
                },
            ],
        }),
    );

    render(PluginOverview, { contextId: PROD, kind: 'plugin:argocd/overview' });

    await expect.element(page.getByText('could not check')).toBeVisible();
    await expect.element(page.getByText('This cluster did not answer')).toBeVisible();
    await expect.element(page.getByText('not installed here')).not.toBeInTheDocument();
});

test('an optional requirement is marked as optional rather than as a failure', async () => {
    render(PluginOverview, { contextId: PROD, kind: 'plugin:argocd/overview' });

    await expect.element(page.getByText('Application Sets')).toBeVisible();
    await expect.element(page.getByText('optional')).toBeVisible();
    // It is missing, yet the plugin still reads as installed.
    await expect.element(page.getByText('installed here')).toBeVisible();
});

test('a view can be opened from the overview', async () => {
    render(PluginOverview, { contextId: PROD, kind: 'plugin:argocd/overview' });

    await page.getByRole('button', { name: 'Applications' }).click();

    expect(workspace.activeTab?.kind).toBe('plugin:argocd/applications');
});

// A tab restored after its plugin's folder was dropped. An empty page would
// read as "Argo CD has nothing", which is the wrong thing to conclude.
test('a tab whose plugin is gone says so', async () => {
    workspace.pluginCatalogue = { plugins: [], dir: '', folders: [], problems: [] };

    render(PluginOverview, { contextId: PROD, kind: 'plugin:argocd/overview' });

    await expect.element(page.getByText('This plugin is not installed')).toBeVisible();
    expect(Summary).not.toHaveBeenCalled();
});
