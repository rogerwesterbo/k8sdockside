import { beforeEach, expect, test, vi } from 'vitest';
import { page } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';

// The sidebar's width is a decision, not a suggestion.
//
// It sits in a flex row beside the view, and a flex item shrinks by default. A
// wide thing to its right -- a log line, a table with many columns -- would
// otherwise take the row over its width and squeeze the context list down to a
// strip of colour, with dragging it wider undone the moment you let go.
vi.mock('@wailsio/runtime', async (importOriginal) => {
    const actual = await importOriginal<typeof import('@wailsio/runtime')>();
    return { ...actual, Window: { ...actual.Window, SetZoom: vi.fn().mockResolvedValue(undefined) } };
});
const { workspace } = await import('../state/workspace.svelte');
const { logs } = await import('../state/logs.svelte');

const PROD = '/home/u/.kube/prod::admin@prod';
const App = (await import('../../App.svelte')).default;

const SHELL_CSS = `html,body{margin:0;height:100%;overflow:hidden}
:root{--bg:#10151c;--bg-sidebar:#151b24;--bg-panel:#19202a;--bg-raised:#212b38;
--bg-hover:rgba(255,255,255,.055);--bg-active:rgba(255,255,255,.09);--border:#28323f;
--border-soft:rgba(255,255,255,.07);--text:#dee5ee;--text-dim:#8d9aaa;--text-faint:#616e7d;
--accent:#4a86ff;--ok:#4cc38a;--warn:#e0a458;--error:#e5646d;--radius:6px;--radius-sm:4px;
--row-h:30px;--font:-apple-system,sans-serif;--mono:Menlo,monospace;
font-family:var(--font);font-size:13px}
#app{height:100%;zoom:var(--app-zoom,1)}
body > div{height:100%;zoom:var(--app-zoom,1)}
button{font:inherit;color:inherit;background:none;border:none;padding:0;cursor:pointer}`;

/**
 * The harness renders into nested wrappers of its own. Without a height on each
 * of them the shell is unbounded, everything simply grows, and any assertion
 * about clipping or overflow passes for the wrong reason.
 */
function constrainToViewport(): void {
    let node = document.querySelector('.shell')?.parentElement;
    while (node && node !== document.body) {
        node.style.height = '100%';
        node = node.parentElement;
    }
    document.body.style.height = '100%';
}

const width = (sel: string) => Math.round((document.querySelector(sel) as HTMLElement).getBoundingClientRect().width);
const settle = () => new Promise((r) => setTimeout(r, 150));

beforeEach(async () => {
    await page.viewport(1440, 900);
    document.body.innerHTML = '';
    document.documentElement.style.zoom = '';
    const style = document.createElement('style');
    style.textContent = SHELL_CSS;
    document.head.appendChild(style);
    workspace.settings.layout.zoom = 1;
    render(App);
    await settle();
    constrainToViewport();
    await settle();
});


/**
 * The window as the screenshot showed it: a wide describe panel open beside a
 * wide table, with the dock holding a log underneath. This is the arrangement
 * that squeezed the context list down to a strip of colour.
 */
async function crowdTheWindow(): Promise<void> {
    await page.viewport(1440, 900);
    workspace.settings.layout.detailDock = 'right';
    workspace.settings.layout.detailSize = 929;
    await workspace.openDetail({
        contextId: PROD,
        kind: 'pods',
        namespace: 'kube-system',
        name: 'cilium-envoy-9wn8t',
    });
    workspace.openLogs({ contextId: PROD, kind: 'pods', namespace: 'kube-system', name: 'cilium-envoy-9wn8t' });
    await settle();
}

/** The sidebar's own width, without the border the rect includes. */
function sidebarWidth(): number {
    const el = document.querySelector('.sidebar') as HTMLElement;
    return Math.round(parseFloat(getComputedStyle(el).width));
}

test('the sidebar keeps its width when the panel beside it is wide', async () => {
    workspace.setSidebarWidth(345);
    await settle();

    await crowdTheWindow();

    expect(sidebarWidth()).toBe(345);
});

test('the sidebar can still be widened with the window crowded', async () => {
    await crowdTheWindow();

    workspace.setSidebarWidth(400);
    await settle();

    expect(sidebarWidth()).toBe(400);
});

// Regions scroll; the window does not.
test('a crowded window does not push the whole shell sideways', async () => {
    await crowdTheWindow();

    expect(document.documentElement.scrollWidth).toBeLessThanOrEqual(
        document.documentElement.clientWidth,
    );
});

// The other half of the same problem: a panel dragged out wide, or restored
// from a session on a bigger screen, must not leave the view it belongs to
// with nothing. Every region gives the others room.
test('the describe panel leaves the table something to be', async () => {
    await crowdTheWindow();

    // Enough to be a table rather than a column of ellipses. The stylesheet
    // reserves a little more than this; what is asserted is the requirement,
    // not the constant that satisfies it.
    const content = document.querySelector('.pane.main > .body') as HTMLElement;
    expect(content.getBoundingClientRect().width).toBeGreaterThanOrEqual(300);
});

test('a panel wider than the window it is restored into is reined in', async () => {
    workspace.settings.layout.detailSize = 4000;
    await crowdTheWindow();

    const panel = document.querySelector('.panel') as HTMLElement;
    const stage = document.querySelector('.stage') as HTMLElement;
    expect(panel.getBoundingClientRect().width).toBeLessThan(stage.getBoundingClientRect().width);
});
