import { beforeEach, expect, test, vi } from 'vitest';
import { page } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';

// The table's rows arrive through a subscription. Stubbing that is what lets a
// test say "the cluster holds this" without a cluster.
const pushed = vi.hoisted(() => ({ send: (_table: unknown) => {} }));
vi.mock('../state/subscriptions', () => ({
    subscribe: vi.fn((_c: string, _k: string, _n: string, onTable: (t: unknown) => void) => {
        pushed.send = onTable;
        return { setNamespace: vi.fn(), close: vi.fn() };
    }),
}));
vi.mock('../../../bindings/github.com/roger/k8sdockside', () => ({
    KubeconfigService: { Sync: vi.fn().mockResolvedValue([]), Files: vi.fn().mockResolvedValue([]) },
    ResourceService: {
        Namespaces: vi.fn().mockResolvedValue(['default']),
        Describe: vi.fn().mockResolvedValue(''),
    },
    ActionService: {
        ObjectState: vi.fn().mockResolvedValue({ scalable: false, replicas: 0, cordoned: false, containers: [] }),
    },
    LogService: {
        Containers: vi.fn().mockResolvedValue([]),
        Open: vi.fn().mockResolvedValue('logs-1'),
        Close: vi.fn(),
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

const ResourceTable = (await import('./ResourceTable.svelte')).default;
const { workspace } = await import('../state/workspace.svelte');

const PROD = '/home/u/.kube/prod::admin@prod';

const plain = (text: string) => ({ text, tone: '', sort: '', pills: null });

/** A pods table with one row, whose Containers cell holds rectangles. */
function podsTable(pills: { label: string; tone: string; detail: string }[] | null) {
    return {
        kind: 'pods',
        columns: ['Name', 'Ready', 'Containers', 'Status'],
        namespaced: true,
        error: '',
        rows: [
            {
                id: 'pods/default/web',
                name: 'web',
                namespace: 'default',
                cells: [
                    plain('web'),
                    plain('2/2'),
                    { text: 'app sidecar', tone: '', sort: '0002', pills },
                    plain('Running'),
                ],
            },
        ],
    };
}

beforeEach(() => {
    workspace.closeDetail();
});

test('a cell carrying containers is drawn as rectangles, not as its text', async () => {
    render(ResourceTable, { contextId: PROD, kind: 'pods' });

    pushed.send(
        podsTable([
            { label: 'app', tone: 'ok', detail: 'Running' },
            { label: 'sidecar', tone: 'error', detail: 'CrashLoopBackOff' },
        ]),
    );

    await expect.element(page.getByRole('img', { name: /app — Running/ })).toBeVisible();
    await expect.element(page.getByRole('img', { name: /sidecar — CrashLoopBackOff/ })).toBeVisible();
});

// Every other kind sends null here, and must go on rendering as it always has.
test('a cell with no containers still shows its text', async () => {
    render(ResourceTable, { contextId: PROD, kind: 'pods' });

    pushed.send(podsTable(null));

    await expect.element(page.getByRole('cell', { name: 'app sidecar' })).toBeVisible();
});

// The rectangles are a picture in the table: pressing one must select the row
// underneath, the way pressing anywhere else in it does.
test('the rectangles in the table are not buttons', async () => {
    render(ResourceTable, { contextId: PROD, kind: 'pods' });

    pushed.send(podsTable([{ label: 'app', tone: 'ok', detail: 'Running' }]));
    await expect.element(page.getByRole('img', { name: /app/ })).toBeVisible();

    expect(page.getByRole('button', { name: /app — Running/ }).elements()).toHaveLength(0);
});
