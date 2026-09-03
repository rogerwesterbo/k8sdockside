import { expect, test, vi } from 'vitest';
import { page } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';
import ErrorState from './ErrorState.svelte';

const REFUSED =
    'looking up namespaces: Get "https://localhost:6443/api?timeout=32s": ' +
    'dial tcp [::1]:6443: connect: connection refused';

test('leads with a plain reading of the failure, not the wire error', async () => {
    render(ErrorState, { props: { message: REFUSED } });

    await expect.element(page.getByRole('heading', { name: 'Cannot reach the API server' })).toBeVisible();
});

test('keeps the raw error on screen, because that is what gets pasted into an issue', async () => {
    render(ErrorState, { props: { message: REFUSED } });

    await expect.element(page.getByText(REFUSED, { exact: false })).toBeVisible();
});

test('names the cluster the failure belongs to', async () => {
    render(ErrorState, {
        props: {
            message: REFUSED,
            context: {
                id: 'x',
                name: 'admin@kubevirt',
                cluster: 'kubevirt',
                user: 'admin',
                namespace: '',
                server: 'https://localhost:6443',
                file: '/home/u/.kube/kubevirt.config',
                current: true,
            },
        },
    });

    await expect.element(page.getByText('admin@kubevirt')).toBeVisible();
    await expect.element(page.getByText('/home/u/.kube/kubevirt.config')).toBeVisible();
});

test('offers a retry when the caller can reload, and calls back on click', async () => {
    const retry = vi.fn();
    render(ErrorState, { props: { message: REFUSED, onRetry: retry } });

    await page.getByRole('button', { name: 'Try again' }).click();

    expect(retry).toHaveBeenCalledOnce();
});

test('offers no retry when the caller has nothing to reload', async () => {
    render(ErrorState, { props: { message: REFUSED } });

    await expect.element(page.getByRole('heading')).toBeVisible();
    expect(document.querySelectorAll('button')).toHaveLength(0);
});
