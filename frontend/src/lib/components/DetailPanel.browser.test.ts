import { beforeEach, expect, test, vi } from 'vitest';
import { page } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';
import DetailPanel from './DetailPanel.svelte';

vi.mock('../../../bindings/github.com/roger/k8sdockside', () => ({
    KubeconfigService: { Sync: vi.fn().mockResolvedValue([]), Files: vi.fn().mockResolvedValue([]) },
    ResourceService: {
        Describe: vi.fn().mockResolvedValue('Name: web'),
        ResourceYAML: vi.fn().mockResolvedValue('kind: Pod\n'),
        CheckYAML: vi.fn().mockResolvedValue({ valid: true, message: '', line: 0 }),
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
const { ResourceService } = await import('../../../bindings/github.com/roger/k8sdockside');

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
