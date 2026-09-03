import { beforeEach, expect, test, vi } from 'vitest';
import { page } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';

vi.mock('@wailsio/runtime', async (importOriginal) => {
    const actual = await importOriginal<typeof import('@wailsio/runtime')>();
    return { ...actual, Window: { ...actual.Window, SetZoom: vi.fn().mockResolvedValue(undefined) } };
});
const { workspace } = await import('../state/workspace.svelte');
const App = (await import('../../App.svelte')).default;

const CSS = `html,body{margin:0;height:100%;overflow:hidden}
:root{--bg:#10151c;--bg-sidebar:#151b24;--bg-panel:#19202a;--bg-raised:#212b38;
--bg-hover:rgba(255,255,255,.055);--bg-active:rgba(255,255,255,.09);--border:#28323f;
--border-soft:rgba(255,255,255,.07);--text:#dee5ee;--text-dim:#8d9aaa;--text-faint:#616e7d;
--accent:#4a86ff;--ok:#4cc38a;--warn:#e0a458;--error:#e5646d;--radius:6px;--radius-sm:4px;
--row-h:30px;--font:-apple-system,sans-serif;--mono:Menlo,monospace;font-family:var(--font);font-size:13px}
#app{height:100%;zoom:var(--app-zoom,1)}
body > div{height:100%;zoom:var(--app-zoom,1)}
button{font:inherit;color:inherit;background:none;border:none;padding:0;cursor:pointer}`;

const CTX = { id: 'c0', name: 'admin@prod', cluster: 'c0', user: 'admin',
    namespace: '', server: '', file: '/c', current: false };

const settle = () => new Promise((r) => setTimeout(r, 150));
const treeLabels = () => [...document.querySelectorAll('.tree .item')].map((el) => el.textContent?.trim());

beforeEach(async () => {
    await page.viewport(1200, 800);
    document.body.innerHTML = '';
    const style = document.createElement('style');
    style.textContent = CSS;
    document.head.appendChild(style);

    workspace.closeAllTabs();
    workspace.settings.layout.zoom = 1;
    workspace.settings.layout.collapsedGroups = [];
    workspace.settings.contexts = {};
    workspace.files = [{ path: '/c', source: 'manual', error: '', contexts: [CTX] }];
    workspace.expanded = ['c0'];
    workspace.selectedContextId = 'c0';
    render(App);
    await settle();
});

test('clicking a tab reveals the section its resource lives in', async () => {
    workspace.openTab(CTX.id, 'mutatingwebhookconfigurations');
    workspace.openTab(CTX.id, 'pods');
    await settle();

    // Put the section away while its tab is still open.
    workspace.toggleGroup(CTX.id, 'Admission');
    await settle();
    expect(treeLabels()).not.toContain('Mutating Webhooks');

    // Now go back to that tab by clicking it in the tab strip.
    const tab = [...document.querySelectorAll('button, [role="tab"]')]
        .find((el) => el.textContent?.trim().startsWith('Mutating Webhooks')) as HTMLElement;
    expect(tab).toBeTruthy();
    tab.click();
    await settle();

    expect(treeLabels()).toContain('Mutating Webhooks');
});

test('clicking a tab whose section is already open changes nothing', async () => {
    workspace.openTab(CTX.id, 'pods');
    await settle();

    const tab = [...document.querySelectorAll('button, [role="tab"]')]
        .find((el) => el.textContent?.trim().startsWith('Pods')) as HTMLElement;
    tab.click();
    await settle();

    expect(workspace.hasFoldingOverride(CTX.id)).toBe(false);
});
