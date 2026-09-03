import { beforeEach, expect, test, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';

vi.mock('@wailsio/runtime', async (importOriginal) => {
    const actual = await importOriginal<typeof import('@wailsio/runtime')>();
    return { ...actual, Window: { ...actual.Window, SetZoom: vi.fn().mockResolvedValue(undefined) } };
});
const { workspace } = await import('../state/workspace.svelte');
const App = (await import('../../App.svelte')).default;

const CTX = { id: 'c0', name: 'admin@prod', cluster: 'c0', user: 'admin',
    namespace: '', server: '', file: '/c', current: false };

const settle = () => new Promise((r) => setTimeout(r, 150));
const watermark = () => document.querySelector('.welcome-stage');

beforeEach(async () => {
    document.body.innerHTML = '';
    workspace.closeAllTabs();
    workspace.files = [{ path: '/c', source: 'manual', error: '', contexts: [CTX] }];
    workspace.loaded = true;
    render(App);
    await settle();
});

test('the idle screen carries the logo behind it', async () => {
    expect(watermark()).not.toBeNull();

    const image = getComputedStyle(watermark()!, '::before').backgroundImage;
    expect(image).toContain('k8s_dockside_harbour_scene_no_text.svg');
});

// The reason it lives on the welcome panel and not on .content: a watermark
// behind a table of pod names is read against every row.
test('opening a tab takes the logo away with the welcome panel', async () => {
    workspace.openTab(CTX.id, 'pods');
    await settle();

    expect(watermark()).toBeNull();
});

test('the logo does not intercept the pointer', async () => {
    expect(getComputedStyle(watermark()!, '::before').pointerEvents).toBe('none');
});
