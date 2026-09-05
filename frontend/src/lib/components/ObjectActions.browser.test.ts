import { beforeEach, expect, test, vi } from 'vitest';
import { page } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';
import ObjectActions from './ObjectActions.svelte';

// Hoisted, because vi.mock's factory is lifted above everything else in the
// file: a plain `let` up here would not exist yet when the factory runs.
const drainEvents = vi.hoisted(() => ({ handler: (_event: { data: unknown }) => {} }));
vi.mock('@wailsio/runtime', async (importOriginal) => {
    const actual = await importOriginal<typeof import('@wailsio/runtime')>();
    return {
        ...actual,
        Events: {
            On: vi.fn((name: string, handler: (event: { data: unknown }) => void) => {
                if (name === 'node:drain') drainEvents.handler = handler;
            }),
        },
    };
});
const deliver = (event: { data: unknown }) => drainEvents.handler(event);
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
const { actions } = await import('../state/actions.svelte');
const { helm } = await import('../state/helm.svelte');
const { ActionService, HelmService, PortForwardService, TerminalService } = await import(
    '../../../bindings/github.com/rogerwesterbo/k8sdockside'
);

const PROD = '/home/u/.kube/prod::admin@prod';
const POD = { contextId: PROD, kind: 'pods', namespace: 'default', name: 'web' };
const NODE = { contextId: PROD, kind: 'nodes', namespace: '', name: 'wrkr01' };
const DEPLOYMENT = { contextId: PROD, kind: 'deployments', namespace: 'default', name: 'web' };
const SERVICE = { contextId: PROD, kind: 'services', namespace: 'web', name: 'api' };
const RELEASE = { contextId: PROD, kind: 'helmreleases', namespace: 'default', name: 'ingress-nginx' };

const settle = () => new Promise((r) => setTimeout(r, 60));

beforeEach(async () => {
    for (const ref of [POD, NODE, DEPLOYMENT, SERVICE, RELEASE]) actions.forget(ref);
    workspace.closeDetail();
    // A test that opened something in the dock has to have it taken away again,
    // and the write that goes with it has to land before the next test starts.
    // The store replaces its whole settings object with whatever the backend
    // answers, so a dock write still in flight arrives in the middle of the
    // next test and takes with it anything that test had set directly.
    if (workspace.dockTabs.length > 0) {
        workspace.closeAllDockTabs();
        await new Promise((r) => setTimeout(r, 320));
    }
    workspace.notice = null;
    helm.forget(RELEASE);
    // Where helm is, re-answered per test: one of them takes it away.
    helm.probed = false;
    vi.mocked(HelmService.Tool).mockReset().mockResolvedValue({
        found: true,
        path: '/usr/bin/helm',
        version: 'v3.16.2',
        configured: false,
        reason: '',
    });
    vi.mocked(HelmService.Uninstall).mockReset().mockResolvedValue('');
    vi.mocked(HelmService.Rollback).mockReset().mockResolvedValue('');
    vi.mocked(ActionService.ObjectState).mockReset().mockResolvedValue({ scalable: false, replicas: 0, cordoned: false, containers: [] });
    vi.mocked(ActionService.Delete).mockReset().mockResolvedValue(undefined);
    vi.mocked(ActionService.Scale).mockReset().mockResolvedValue(undefined);
    vi.mocked(ActionService.Cordon).mockReset().mockResolvedValue(undefined);
    vi.mocked(ActionService.Drain).mockReset().mockResolvedValue('drain-1');
    vi.mocked(ActionService.CancelDrain).mockReset();
    vi.mocked(PortForwardService.Ports).mockReset().mockResolvedValue([]);
    vi.mocked(PortForwardService.Start).mockReset().mockResolvedValue({
        id: 'pf-1',
        contextId: PROD,
        kind: 'services',
        namespace: 'web',
        name: 'api',
        remotePort: 80,
        localPort: 51234,
        random: true,
        browser: false,
        state: 'active',
        error: '',
        pod: 'api-7f9',
        podPort: 8080,
        note: '',
    });
    vi.mocked(TerminalService.Launch).mockReset().mockResolvedValue(undefined);
    vi.mocked(TerminalService.LaunchNode).mockReset().mockResolvedValue(undefined);
});

test('an ordinary object offers to be edited and deleted', async () => {
    render(ObjectActions, { object: POD });

    await expect.element(page.getByRole('button', { name: 'Edit' })).toBeVisible();
    await expect.element(page.getByRole('button', { name: 'Delete' })).toBeVisible();
    expect(page.getByRole('button', { name: 'Scale' }).elements()).toHaveLength(0);
});

test('a node offers what only a node can do', async () => {
    render(ObjectActions, { object: NODE });

    await expect.element(page.getByRole('button', { name: 'Cordon' })).toBeVisible();
    await expect.element(page.getByRole('button', { name: 'Drain' })).toBeVisible();
});

// The one button whose label is the cluster's answer rather than ours: offering
// to cordon a node that is already cordoned is a button that does nothing.
test('a cordoned node offers to be uncordoned instead', async () => {
    vi.mocked(ActionService.ObjectState).mockResolvedValue({ scalable: false, replicas: 0, cordoned: true, containers: [] });

    render(ObjectActions, { object: NODE });

    await expect.element(page.getByRole('button', { name: 'Uncordon' })).toBeVisible();
    expect(page.getByRole('button', { name: 'Cordon', exact: true }).elements()).toHaveLength(0);
});

test('cordoning a node sends it', async () => {
    render(ObjectActions, { object: NODE });
    await expect.element(page.getByRole('button', { name: 'Cordon' })).toBeVisible();

    await page.getByRole('button', { name: 'Cordon' }).click();

    await vi.waitFor(() => expect(ActionService.Cordon).toHaveBeenCalledWith(PROD, 'wrkr01', true));
});

test('deleting asks first, and names what it would delete', async () => {
    render(ObjectActions, { object: POD });

    await page.getByRole('button', { name: 'Delete' }).click();

    await expect.element(page.getByText(/Delete Pod web\?/)).toBeVisible();
    expect(ActionService.Delete).not.toHaveBeenCalled();
});

// The destructive button must not be the one a stray Enter lands on.
test('the question puts focus on Cancel, not on Delete', async () => {
    render(ObjectActions, { object: POD });

    await page.getByRole('button', { name: 'Delete' }).click();
    await settle();

    expect(document.activeElement?.textContent?.trim()).toBe('Cancel');
});

test('cancelling puts the buttons back and deletes nothing', async () => {
    render(ObjectActions, { object: POD });
    await page.getByRole('button', { name: 'Delete' }).click();
    await expect.element(page.getByRole('button', { name: 'Cancel' })).toBeVisible();

    await page.getByRole('button', { name: 'Cancel' }).click();

    await expect.element(page.getByRole('button', { name: 'Edit' })).toBeVisible();
    expect(ActionService.Delete).not.toHaveBeenCalled();
});

test('Escape cancels the question too', async () => {
    render(ObjectActions, { object: POD });
    await page.getByRole('button', { name: 'Delete' }).click();
    await expect.element(page.getByText(/Delete Pod web\?/)).toBeVisible();

    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }));

    await expect.element(page.getByRole('button', { name: 'Edit' })).toBeVisible();
    expect(ActionService.Delete).not.toHaveBeenCalled();
});

test('confirming deletes the object and closes the panel over it', async () => {
    await workspace.openDetail(POD);
    render(ObjectActions, { object: POD });
    await page.getByRole('button', { name: 'Delete' }).click();

    await page.getByRole('button', { name: 'Delete', exact: true }).click();

    await vi.waitFor(() =>
        expect(ActionService.Delete).toHaveBeenCalledWith(PROD, 'pods', 'default', 'web'),
    );
    // Nothing is left to describe, so the panel goes rather than showing a 404.
    await vi.waitFor(() => expect(workspace.detailTarget).toBeNull());
});

test('a refused delete says why and leaves the object alone', async () => {
    vi.mocked(ActionService.Delete).mockRejectedValue(new Error('pods "web" is forbidden'));
    await workspace.openDetail(POD);
    render(ObjectActions, { object: POD });
    await page.getByRole('button', { name: 'Delete' }).click();

    await page.getByRole('button', { name: 'Delete', exact: true }).click();

    await vi.waitFor(() => expect(workspace.notice?.text).toContain('forbidden'));
    expect(workspace.detailTarget).not.toBeNull();
});

test('scale opens a field holding the count the workload is at', async () => {
    vi.mocked(ActionService.ObjectState).mockResolvedValue({ scalable: true, replicas: 3, cordoned: false, containers: [] });
    render(ObjectActions, { object: DEPLOYMENT });
    await expect.element(page.getByRole('button', { name: 'Scale' })).toBeVisible();

    await page.getByRole('button', { name: 'Scale' }).click();

    await expect.element(page.getByRole('spinbutton', { name: /Replicas/ })).toHaveValue(3);
});

test('scaling sends the number that was typed', async () => {
    vi.mocked(ActionService.ObjectState).mockResolvedValue({ scalable: true, replicas: 3, cordoned: false, containers: [] });
    render(ObjectActions, { object: DEPLOYMENT });
    await page.getByRole('button', { name: 'Scale' }).click();

    await page.getByRole('spinbutton', { name: /Replicas/ }).fill('5');
    await page.getByRole('button', { name: 'Apply' }).click();

    await vi.waitFor(() =>
        expect(ActionService.Scale).toHaveBeenCalledWith(PROD, 'deployments', 'default', 'web', 5),
    );
});

test('draining asks first, then reports as it goes', async () => {
    render(ObjectActions, { object: NODE });
    await page.getByRole('button', { name: 'Drain' }).click();
    await page.getByRole('button', { name: 'Drain', exact: true }).click();
    await vi.waitFor(() => expect(ActionService.Drain).toHaveBeenCalledWith(PROD, 'wrkr01'));

    deliver({
        data: {
            drainId: 'drain-1',
            node: 'wrkr01',
            phase: 'evicting',
            evicted: 3,
            total: 4,
            refused: [],
            error: '',
            done: false,
        },
    });

    await expect.element(page.getByText(/3 of 4/)).toBeVisible();
});

// The whole reason the drain reports refusals rather than forcing them: the
// user has to be told which pods were left, and why.
test('the pods a drain would not move are named', async () => {
    render(ObjectActions, { object: NODE });
    await page.getByRole('button', { name: 'Drain' }).click();
    await page.getByRole('button', { name: 'Drain', exact: true }).click();
    await vi.waitFor(() => expect(ActionService.Drain).toHaveBeenCalled());

    deliver({
        data: {
            drainId: 'drain-1',
            node: 'wrkr01',
            phase: 'done',
            evicted: 2,
            total: 2,
            refused: [
                { pod: { namespace: 'default', name: 'debug' }, reason: 'nothing manages it' },
            ],
            error: '',
            done: true,
        },
    });

    await expect.element(page.getByText(/default\/debug/)).toBeVisible();
    await expect.element(page.getByText(/nothing manages it/)).toBeVisible();
});

test('a drain in flight can be called off', async () => {
    render(ObjectActions, { object: NODE });
    await page.getByRole('button', { name: 'Drain' }).click();
    await page.getByRole('button', { name: 'Drain', exact: true }).click();
    await vi.waitFor(() => expect(ActionService.Drain).toHaveBeenCalled());

    await page.getByRole('button', { name: 'Stop' }).click();

    expect(ActionService.CancelDrain).toHaveBeenCalledWith('drain-1');
});

// A Helm release is a decoded Secret rather than an object, so it gets none of
// the object verbs -- an Edit would open a Secret full of gzipped base64, and a
// Delete would leave everything the release installed running.
test('a Helm release gets Helm verbs rather than object ones', async () => {
    render(ObjectActions, { object: RELEASE });
    await settle();

    const labels = page.getByRole('button').elements().map((b) => b.textContent?.trim());
    expect(labels).toEqual(['Values', 'Rollback', 'Uninstall']);
});

// The way in to the values editor. Without this the whole upgrade path is code
// nothing reaches.
test('a release opens its values in the dock', async () => {
    render(ObjectActions, { object: RELEASE });
    await settle();

    await page.getByRole('button', { name: 'Values' }).click();

    expect(
        workspace.dockTabs.some((t) => t.view === 'helmvalues' && t.name === 'ingress-nginx'),
    ).toBe(true);
    expect(workspace.dockOpen).toBe(true);
});

// One release is many objects, which is what makes this worth spelling out
// rather than asking "Uninstall X?".
test('uninstalling asks first, and says what it will take with it', async () => {
    render(ObjectActions, { object: RELEASE });
    await settle();

    await page.getByRole('button', { name: 'Uninstall' }).click();

    await expect.element(page.getByText(/Everything the release installed will be removed/)).toBeVisible();
    expect(HelmService.Uninstall).not.toHaveBeenCalled();
});

// The only way an uninstall is undoable at all.
test('an uninstall can keep the history so the release can be rolled back', async () => {
    render(ObjectActions, { object: RELEASE });
    await settle();

    await page.getByRole('button', { name: 'Uninstall' }).click();
    await page.getByRole('checkbox', { name: /Keep the history/ }).click();
    await page.getByRole('button', { name: 'Uninstall' }).click();

    await vi.waitFor(() =>
        expect(HelmService.Uninstall).toHaveBeenCalledWith(PROD, 'default', 'ingress-nginx', true),
    );
});

// A button that cannot work says why on itself rather than failing when it is
// pressed. Reading the release still works -- that needs no helm at all.
test('without helm the two that need it are disabled, with the reason', async () => {
    vi.mocked(HelmService.Tool).mockResolvedValue({
        found: false,
        path: '',
        version: '',
        configured: false,
        reason: 'helm was not found on your PATH',
    });
    helm.probed = false;

    render(ObjectActions, { object: RELEASE });
    await settle();

    await expect.element(page.getByRole('button', { name: 'Rollback' })).toBeDisabled();
    await expect.element(page.getByRole('button', { name: 'Uninstall' })).toBeDisabled();
    // Values reads the release, which the app does itself.
    await expect.element(page.getByRole('button', { name: 'Values' })).toBeEnabled();
});

// The way in to the logs. Without this the log view is code nothing reaches.
test('a pod offers its logs, and pressing it opens them in the dock', async () => {
    render(ObjectActions, { object: POD });

    await page.getByRole('button', { name: 'Logs' }).click();

    expect(workspace.dockTabs.map((t) => t.view)).toEqual(['logs']);
    expect(workspace.dockTabs[0].name).toBe('web');
    expect(workspace.dockOpen).toBe(true);
});

// Two views onto one object, not one tab that changes what it shows.
test('logs and the editor are two dock tabs on the same object', async () => {
    render(ObjectActions, { object: POD });

    await page.getByRole('button', { name: 'Logs' }).click();
    await page.getByRole('button', { name: 'Edit' }).click();

    expect(workspace.dockTabs.map((t) => t.view).sort()).toEqual(['edit', 'logs']);
});

// ---- shells and forwards ---------------------------------------------------

test('a shell opens in the dock without asking anything first', async () => {
    render(ObjectActions, { object: POD });
    await expect.element(page.getByRole('button', { name: 'Shell' })).toBeVisible();

    await page.getByRole('button', { name: 'Shell' }).click();

    // Which shell, and which terminal, are settings answered once -- a shell
    // you have to fill in a form for is not a shell anybody would use.
    await vi.waitFor(() => expect(workspace.dockTabs.some((t) => t.view === 'shell')).toBe(true));
    workspace.closeAllDockTabs();
});

test('a shell goes to your own terminal when that is what you chose', async () => {
    workspace.settings.preferences.terminal = { ...workspace.terminal, mode: 'external' };
    render(ObjectActions, { object: POD });
    await expect.element(page.getByRole('button', { name: 'Shell' })).toBeVisible();

    await page.getByRole('button', { name: 'Shell' }).click();

    await vi.waitFor(() =>
        expect(TerminalService.Launch).toHaveBeenCalledWith(PROD, 'pods', 'default', 'web', '', ''),
    );
    // And nothing opens in the dock: the shell is over there.
    expect(workspace.dockTabs.some((t) => t.view === 'shell')).toBe(false);
    workspace.settings.preferences.terminal = { ...workspace.terminal, mode: 'app' };
});

test('a node shell says which node rather than which pod', async () => {
    workspace.settings.preferences.terminal = { ...workspace.terminal, mode: 'external' };
    render(ObjectActions, { object: NODE });

    await page.getByRole('button', { name: 'Shell' }).click();

    await vi.waitFor(() => expect(TerminalService.LaunchNode).toHaveBeenCalledWith(PROD, 'wrkr01'));
    workspace.settings.preferences.terminal = { ...workspace.terminal, mode: 'app' };
});

test('forwarding offers the ports the object actually has', async () => {
    vi.mocked(PortForwardService.Ports).mockResolvedValue([
        { name: 'http', port: 80, protocol: 'TCP', target: '8080' },
        { name: '', port: 9090, protocol: 'TCP', target: '' },
    ]);
    render(ObjectActions, { object: SERVICE });
    await expect.element(page.getByRole('button', { name: 'Forward' })).toBeVisible();

    await page.getByRole('button', { name: 'Forward' }).click();

    // A service port reads with where it lands on the pod, which is rarely the
    // same number.
    const picker = page.getByRole('combobox', { name: 'Port' });
    await expect.element(picker).toBeVisible();

    const options = [...(picker.element() as HTMLSelectElement).options].map((o) => o.textContent?.trim());
    expect(options).toEqual(['80 · http · → 8080', '9090']);
});

test('forwarding sends what was chosen, with a blank local port meaning any', async () => {
    vi.mocked(PortForwardService.Ports).mockResolvedValue([
        { name: 'http', port: 80, protocol: 'TCP', target: '8080' },
    ]);
    render(ObjectActions, { object: SERVICE });
    await page.getByRole('button', { name: 'Forward' }).click();
    await expect.element(page.getByRole('combobox', { name: 'Port' })).toBeVisible();

    await page.getByRole('button', { name: 'Forward', exact: true }).click();

    await vi.waitFor(() =>
        // 0 is "any free port", and true is the browser checkbox, which is on
        // unless it is turned off.
        expect(PortForwardService.Start).toHaveBeenCalledWith(PROD, 'services', 'web', 'api', 80, 0, true),
    );
});

test('a local port typed by hand is the one asked for', async () => {
    vi.mocked(PortForwardService.Ports).mockResolvedValue([
        { name: 'http', port: 80, protocol: 'TCP', target: '8080' },
    ]);
    render(ObjectActions, { object: SERVICE });
    await page.getByRole('button', { name: 'Forward' }).click();
    await expect.element(page.getByRole('spinbutton', { name: 'Local' })).toBeVisible();

    await page.getByRole('spinbutton', { name: 'Local' }).fill('8081');
    await page.getByRole('checkbox', { name: 'Open a browser' }).click();
    await page.getByRole('button', { name: 'Forward', exact: true }).click();

    await vi.waitFor(() =>
        expect(PortForwardService.Start).toHaveBeenCalledWith(PROD, 'services', 'web', 'api', 80, 8081, false),
    );
});

test('an object whose ports could not be read says so rather than offering none', async () => {
    vi.mocked(PortForwardService.Ports).mockRejectedValue(
        new Error('service api selects no pods -- its endpoints are set by hand'),
    );
    render(ObjectActions, { object: SERVICE });

    await page.getByRole('button', { name: 'Forward' }).click();

    await expect.element(page.getByText(/selects no pods/)).toBeVisible();
});
