import { beforeEach, expect, test } from 'vitest';
import { render } from 'vitest-browser-svelte';
import Sidebar from './Sidebar.svelte';
import { workspace } from '../state/workspace.svelte';

// Scrolling a context into view is DOM behaviour: the store can only say that a
// reveal was requested, not that anything useful moved.
//
// These reveal with no kind, which is the fallback the sidebar takes when the
// tab's own row is not showing -- its section shut, or its API group. Revealing
// the row itself is covered in RevealItem.browser.test.ts. These run against a
// real sidebar at a realistic height.
//
// The design tokens have to be injected. Without them `height: var(--row-h)` is
// invalid, every row collapses, and the sidebar lays out nothing like the app --
// which is how an earlier version of these tests passed while the app was
// visibly broken.
const TOKENS = `:root{--bg:#10151c;--bg-sidebar:#151b24;--bg-panel:#19202a;--bg-raised:#212b38;
--bg-hover:rgba(255,255,255,.055);--bg-active:rgba(255,255,255,.09);--border:#28323f;
--border-soft:rgba(255,255,255,.07);--text:#dee5ee;--text-dim:#8d9aaa;--text-faint:#616e7d;
--accent:#4a86ff;--ok:#4cc38a;--warn:#e0a458;--error:#e5646d;--radius:6px;--radius-sm:4px;
--row-h:30px;--font:-apple-system,sans-serif;--mono:Menlo,monospace;font-family:var(--font);font-size:13px}
body{color:var(--text);margin:0;background:#151b24}
button{font:inherit;color:inherit;background:none;border:none;padding:0;cursor:pointer}
input{font:inherit;color:var(--text);background:var(--bg);border:1px solid var(--border);border-radius:4px;padding:5px 8px}`;

const FILE = '/home/u/.kube/many.config';
// Enough contexts that the list overflows on its own, whatever else the
// sidebar is or is not drawing. At 24 it only just did, and the margin was the
// file heading above them -- so hiding kubeconfig names by default was enough
// to make the list fit, and every assertion here about something being out of
// view quietly stopped meaning anything.
const CONTEXTS = Array.from({ length: 40 }, (_, i) => ({
    id: `${FILE}::ctx-${i}`, name: `admin@cluster-${i}`, cluster: `cluster-${i}`,
    user: 'admin', namespace: '', server: '', file: FILE, current: false,
}));
const TOP = CONTEXTS[0];
const BOTTOM = CONTEXTS[CONTEXTS.length - 1];

const scroller = () => document.querySelector('.scroll') as HTMLElement;

function row(contextId: string): HTMLElement {
    const index = CONTEXTS.findIndex((c) => c.id === contextId);
    return document.querySelectorAll('.context .head')[index] as HTMLElement;
}

/** How much of the viewport is left below a context's row for its tree. */
function roomBelow(contextId: string): number {
    return scroller().getBoundingClientRect().bottom - row(contextId).getBoundingClientRect().bottom;
}

function visible(contextId: string): boolean {
    const view = scroller().getBoundingClientRect();
    const box = row(contextId).getBoundingClientRect();
    return box.top >= view.top - 1 && box.bottom <= view.bottom + 1;
}

const settle = () => new Promise((r) => setTimeout(r, 700));

beforeEach(() => {
    document.body.innerHTML = '';
    const style = document.createElement('style');
    style.textContent = TOKENS;
    document.head.appendChild(style);

    workspace.closeAllTabs();
    workspace.reveal = null;
    workspace.collapseAll();
    workspace.files = [{ path: FILE, source: 'manual', error: '', contexts: CONTEXTS }];
    render(Sidebar);
    (document.querySelector('.sidebar') as HTMLElement).style.height = '820px';
});

// Guards the trap these tests fell into once: with no design tokens the rows
// have no height, the sidebar lays out nothing like the app, and assertions
// about visibility become meaningless.
test('the harness lays the sidebar out like the app does', () => {
    expect(row(TOP.id).getBoundingClientRect().height).toBe(30);
    expect(scroller().scrollHeight).toBeGreaterThan(scroller().clientHeight);
});

test('a context far down the list starts out of view', () => {
    expect(visible(BOTTOM.id)).toBe(false);
});

// Activating a tab now reveals that tab's own row rather than the context
// header, so these check the row arrives -- the header may well be off screen
// above it, which is fine and was the point.
test('switching down to a tab on the bottom context shows that tab\'s row', async () => {
    workspace.openTab(TOP.id, 'pods');
    workspace.openTab(BOTTOM.id, 'pods');
    workspace.activateTab(`${TOP.id}#pods`);
    await settle();

    workspace.activateTab(`${BOTTOM.id}#pods`);
    await settle();

    const row = document.querySelector(`.context:last-of-type [data-kind="pods"]`) as HTMLElement;
    expect(row).toBeTruthy();
    const view = scroller().getBoundingClientRect();
    const box = row.getBoundingClientRect();
    expect(box.top).toBeGreaterThanOrEqual(view.top - 1);
    expect(box.bottom).toBeLessThanOrEqual(view.bottom + 1);
});

test('a context already fully in view is not scrolled away from', async () => {
    workspace.reveal = { kind: '', contextId: TOP.id, nonce: 1 };
    await settle();

    expect(scroller().scrollTop).toBe(0);
});

test('a reveal flashes the row, so the eye can find it', async () => {
    workspace.reveal = { kind: '', contextId: BOTTOM.id, nonce: 1 };

    await expect.poll(() => row(BOTTOM.id).classList.contains('flash')).toBe(true);
    expect(row(TOP.id).classList.contains('flash')).toBe(false);
});
