import { beforeEach, expect, test, vi } from 'vitest';
import { page } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';
import HelmRelease from './HelmRelease.svelte';

const Detail = vi.fn();

vi.mock('../../../bindings/github.com/rogerwesterbo/k8sdockside', () => ({
    HelmService: { Detail: (...args: unknown[]) => Detail(...args) },
}));

const ROOK = { contextId: '/home/u/.kube/prod::admin@prod', namespace: 'rook-ceph', name: 'rook-ceph' };

/** A release as the backend decodes one. See internal/kube/helmdetail.go. */
function release(over: Record<string, unknown> = {}) {
    return {
        name: 'rook-ceph',
        namespace: 'rook-ceph',
        revision: 3,
        status: 'deployed',
        chart: 'rook-ceph-v1.19.8',
        chartName: 'rook-ceph',
        chartVersion: 'v1.19.8',
        appVersion: 'v1.19.8',
        description: 'Upgrade complete',
        firstDeployed: '2026-06-01T09:00:00Z',
        updated: '2026-08-05T11:27:04Z',
        notes: 'The Rook Operator has been installed.',
        values: 'allowLoopDevices: false\ncsi:\n  provisionerReplicas: 3\n',
        userValues: 'csi:\n  provisionerReplicas: 3\n',
        resources: [
            { kind: 'ClusterRole', apiVersion: 'rbac.authorization.k8s.io/v1', name: 'rook-ceph-global', namespace: '' },
            { kind: 'ServiceAccount', apiVersion: 'v1', name: 'rook-ceph-system', namespace: 'rook-ceph' },
        ],
        revisions: [
            { revision: 3, status: 'deployed', chart: 'rook-ceph-v1.19.8', appVersion: 'v1.19.8', updated: '2026-08-05T11:27:04Z', description: 'Upgrade complete', current: true },
            { revision: 2, status: 'superseded', chart: 'rook-ceph-v1.19.0', appVersion: 'v1.19.0', updated: '2026-07-01T10:00:00Z', description: 'Upgrade complete', current: false },
        ],
        ...over,
    };
}

beforeEach(() => {
    Detail.mockReset().mockResolvedValue(release());
});

test('the drawer opens on what the release is and whether it is healthy', async () => {
    render(HelmRelease, { release: ROOK });

    await expect.element(page.getByText('rook-ceph-v1.19.8').first()).toBeVisible();
    await expect.element(page.getByText('deployed').first()).toBeVisible();
    // Helm's own log entry for the revision, which on a failed release is the
    // whole answer to what went wrong.
    await expect.element(page.getByText('Upgrade complete').first()).toBeVisible();
});

// The values are what anyone actually opened this for, and the default is the
// merged document: what the release is doing, rather than what was typed.
test('values open merged, and the toggle narrows them to the overrides', async () => {
    render(HelmRelease, { release: ROOK });

    await expect.element(page.getByText('allowLoopDevices: false')).toBeVisible();

    await page.getByRole('checkbox', { name: 'User-supplied values only' }).click();

    // The chart's own default is gone; the override that survives is the one
    // somebody chose.
    await expect.element(page.getByText('allowLoopDevices: false')).not.toBeInTheDocument();
    await expect.element(page.getByText('provisionerReplicas: 3')).toBeVisible();
});

// A release installed with no overrides is running the chart exactly as it
// ships, which is a fact about it rather than an empty box.
test('a release with no overrides says so rather than showing nothing', async () => {
    Detail.mockResolvedValue(release({ userValues: '' }));
    render(HelmRelease, { release: ROOK });

    await expect.element(page.getByText('allowLoopDevices: false')).toBeVisible();
    await page.getByRole('checkbox', { name: 'User-supplied values only' }).click();

    await expect.element(page.getByText(/Installed with no overrides/)).toBeVisible();
});

test('the objects the release rendered are listed', async () => {
    render(HelmRelease, { release: ROOK });

    await expect.element(page.getByText('rook-ceph-global')).toBeVisible();
    await expect.element(page.getByText('rook-ceph-system')).toBeVisible();
    await expect.element(page.getByText('ClusterRole')).toBeVisible();
});

test('the history is there, newest first, with the current revision marked', async () => {
    render(HelmRelease, { release: ROOK });

    await page.getByText('History').click();

    await expect.element(page.getByText('rook-ceph-v1.19.0')).toBeVisible();
    // The revision the rest of the drawer is describing reads apart from the
    // ones behind it.
    const current = page.getByRole('row').elements().find((row) => row.className.includes('current'));
    expect(current?.textContent).toContain('3');
});

// A cluster that will not answer has to say so where the release would have
// been, rather than leaving a drawer that looks like an empty release.
test('a failed read is reported in the drawer', async () => {
    Detail.mockRejectedValue(new Error('secrets is forbidden'));
    render(HelmRelease, { release: ROOK });

    await expect.element(page.getByText(/forbidden/)).toBeVisible();
});

// The three fields that name a release, and only those: the drawer reads one
// release rather than searching for it. Switching between two of them is driven
// by the panel above, and is tested there.
test('the drawer reads the release it is pointed at, once', async () => {
    render(HelmRelease, { release: ROOK });

    await expect.element(page.getByText('rook-ceph-v1.19.8').first()).toBeVisible();
    expect(Detail).toHaveBeenCalledTimes(1);
    expect(Detail).toHaveBeenCalledWith(ROOK.contextId, 'rook-ceph', 'rook-ceph');
});
