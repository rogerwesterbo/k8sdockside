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

const PROD = '/home/u/.kube/prod::admin@prod';

beforeEach(() => {
    workspace.closeAllDockTabs();
    workspace.closeDetail();
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
