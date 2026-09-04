import { beforeEach, expect, test, vi } from 'vitest';
import { page } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';
import Sidebar from './Sidebar.svelte';
import { NAV_GROUPS } from '../catalogue';
import { workspace } from '../state/workspace.svelte';

vi.mock('@wailsio/runtime', async (importOriginal) => {
    const actual = await importOriginal<typeof import('@wailsio/runtime')>();
    return { ...actual, Window: { ...actual.Window, SetZoom: vi.fn().mockResolvedValue(undefined) } };
});

const TOKENS = `:root{--bg:#10151c;--bg-sidebar:#151b24;--bg-panel:#19202a;--bg-raised:#212b38;
--bg-hover:rgba(255,255,255,.055);--bg-active:rgba(255,255,255,.09);--border:#28323f;
--border-soft:rgba(255,255,255,.07);--text:#dee5ee;--text-dim:#8d9aaa;--text-faint:#616e7d;
--accent:#4a86ff;--ok:#4cc38a;--warn:#e0a458;--error:#e5646d;--radius:6px;--radius-sm:4px;
--row-h:30px;--font:-apple-system,sans-serif;--mono:Menlo,monospace;
font-family:var(--font);font-size:13px}
body{color:var(--text);margin:0}
button{font:inherit;color:inherit;background:none;border:none;padding:0;cursor:pointer}
input{font:inherit;color:var(--text);background:var(--bg);border:1px solid var(--border);border-radius:4px;padding:5px 8px}`;

const CTX = { id: 'c0', name: 'admin@prod', cluster: 'c0', user: 'admin',
    namespace: '', server: '', file: '/c', current: false };

const settle = () => new Promise((r) => setTimeout(r, 700));
const scroller = () => document.querySelector('.scroll') as HTMLElement;

/** The sidebar row for one resource kind, if it is rendered at all. */
function rowFor(label: string): HTMLElement | undefined {
    return [...document.querySelectorAll('.tree .item')]
        .find((el) => el.textContent?.trim() === label) as HTMLElement | undefined;
}

function inView(el: HTMLElement): boolean {
    const view = scroller().getBoundingClientRect();
    const box = el.getBoundingClientRect();
    return box.top >= view.top - 1 && box.bottom <= view.bottom + 1;
}

beforeEach(async () => {
    await page.viewport(320, 700);
    document.body.innerHTML = '';
    const style = document.createElement('style');
    style.textContent = TOKENS;
    document.head.appendChild(style);

    workspace.closeAllTabs();
    workspace.settings.layout.zoom = 1;
    workspace.files = [{ path: '/c', source: 'manual', error: '', contexts: [CTX] }];
    workspace.expanded = ['c0'];
    workspace.selectedContextId = 'c0';
    // Everything open, so the tree is as long as it really gets.
    workspace.settings.contexts = { c0: { alias: '', color: '', collapsedGroups: [] } };
    workspace.settings.layout.collapsedGroups = [];
    render(Sidebar);
    (document.querySelector('.sidebar') as HTMLElement).style.height = '700px';
    await settle();
});

test('the tree is long enough that a row near the end is off screen', () => {
    expect(NAV_GROUPS.length).toBeGreaterThan(8);
    const row = rowFor('All definitions');
    expect(row).toBeTruthy();
    expect(inView(row!)).toBe(false);
});

// Activating a tab should show the row it belongs to. Bringing the cluster's
// own name into view says nothing about where in fifty rows that tab lives.
test('activating a tab brings its own row into view', async () => {
    workspace.openTab(CTX.id, 'customresourcedefinitions');
    workspace.openTab(CTX.id, 'pods');
    await settle();
    scroller().scrollTop = 0;
    await settle();

    workspace.activateTab(`${CTX.id}#customresourcedefinitions`);
    await settle();

    expect(inView(rowFor('All definitions')!)).toBe(true);
});
