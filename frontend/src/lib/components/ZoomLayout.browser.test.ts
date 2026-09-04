import { beforeEach, expect, test, vi } from 'vitest';
import { page } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';

// Zoom has to be applied by CSS rather than by the window: Wails clamps its
// native zoom to a minimum of 1.0 on macOS, so zooming out is discarded, and
// what it does apply is setMagnification -- which scales the rendered surface
// without reflowing, so the content overflows the window instead of adapting.
//
// These check the two things that failure looked like from outside: nothing
// shrinks when zooming out, and zooming in overflows.
vi.mock('@wailsio/runtime', async (importOriginal) => {
    const actual = await importOriginal<typeof import('@wailsio/runtime')>();
    return { ...actual, Window: { ...actual.Window, SetZoom: vi.fn().mockResolvedValue(undefined) } };
});
const { workspace } = await import('../state/workspace.svelte');
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

test('zooming out actually shrinks the interface', async () => {
    const before = width('.sidebar');

    workspace.zoomOut();
    workspace.zoomOut();
    await settle();

    expect(width('.sidebar')).toBeLessThan(before);
});

test('zooming in widens the navigation rather than overflowing the window', async () => {
    const before = width('.sidebar');

    workspace.zoomIn();
    workspace.zoomIn();
    await settle();

    expect(width('.sidebar')).toBeGreaterThan(before);
    // The whole window must not gain scrollbars; regions scroll, the shell does not.
    expect(document.documentElement.scrollWidth).toBeLessThanOrEqual(document.documentElement.clientWidth);
});

test('the status bar stays on screen at maximum zoom', async () => {
    for (let i = 0; i < 12; i++) workspace.zoomIn();
    await settle();

    const bar = (document.querySelector('.statusbar') as HTMLElement).getBoundingClientRect();
    expect(bar.bottom).toBeLessThanOrEqual(window.innerHeight + 1);
    expect(bar.height).toBeGreaterThan(0);
});

test('the title bar keeps its real height when zoomed out, so the traffic lights fit', async () => {
    for (let i = 0; i < 6; i++) workspace.zoomOut();
    await settle();

    // Measured in device pixels, which is the space the native buttons occupy.
    const bar = (document.querySelector('header.topbar') as HTMLElement).getBoundingClientRect();
    expect(bar.height).toBeGreaterThanOrEqual(43);
});

// A short window is the case that actually clips: .content hides its overflow
// and the welcome panel has none of its own, so its lower half becomes
// unreachable. Zooming in produces the same squeeze, which is what makes this
// an accessibility problem rather than a cosmetic one.
test('the welcome panel stays reachable in a window too short for it', async () => {
    await page.viewport(900, 260);
    await settle();

    const content = document.querySelector('.content') as HTMLElement;
    const welcome = content.querySelector('.welcome') as HTMLElement | null;
    expect(welcome).not.toBeNull();

    const clipped = welcome!.getBoundingClientRect().bottom > content.getBoundingClientRect().bottom + 1;
    const reachable = welcome!.scrollHeight > welcome!.clientHeight
        ? getComputedStyle(welcome!).overflowY !== 'visible'
        : true;

    expect(clipped && !reachable).toBe(false);
});

// The distinction that took measuring to find: a percentage height under a
// zoomed *root* ignores the zoom, so the shell renders past the window and the
// whole thing gains scrollbars -- the symptom this all started from. Zooming
// #app instead keeps `height: 100%` meaning the window.
//
// Checked with elementFromPoint because coordinates under zoom are reported in
// more than one space, and a rectangle comparison can be read the wrong way.
test.each([2, 0.5])('the shell still reaches the bottom of the window at %sx zoom', async (scale) => {
    workspace.settings.layout.zoom = scale;
    await settle();

    const atBottom = document.elementFromPoint(30, window.innerHeight - 3);
    expect(atBottom?.closest('.shell')).not.toBeNull();
});

test.each([2, 0.5])('the window itself never scrolls at %sx zoom', async (scale) => {
    workspace.settings.layout.zoom = scale;
    await settle();

    const root = document.documentElement;
    expect(root.scrollHeight).toBeLessThanOrEqual(root.clientHeight + 1);
    expect(root.scrollWidth).toBeLessThanOrEqual(root.clientWidth + 1);
});

// The sidebar stacks a tree above a settings panel. The panel does not shrink,
// so at high zoom in a short window it keeps its size and the tree absorbs the
// whole loss -- collapsing to a single heading, which is the point at which
// zooming to read better stops being usable.
test('the context tree stays usable when zoomed in on a short window', async () => {
    await page.viewport(1100, 620);
    const ctxs = Array.from({ length: 8 }, (_, i) => ({
        id: `c${i}`, name: `admin@cluster-${i}`, cluster: `c${i}`, user: 'admin',
        namespace: '', server: '', file: '/c', current: false,
    }));
    workspace.files = [{ path: '/c', source: 'manual', error: '', contexts: ctxs }];
    workspace.selectedContextId = 'c0';
    workspace.expanded = ['c0'];
    workspace.settings.layout.zoom = 1.5;
    await settle();

    const tree = document.querySelector('.scroll') as HTMLElement;
    // Enough for the context row plus a few of its resources, in device pixels.
    expect(tree.getBoundingClientRect().height).toBeGreaterThan(180);
});

test('the settings panel is still visible when the tree claims its minimum', async () => {
    await page.viewport(1100, 620);
    workspace.selectedContextId = 'c0';
    workspace.settings.layout.zoom = 1.5;
    await settle();

    const settings = document.querySelector('.settings') as HTMLElement | null;
    expect(settings).not.toBeNull();
    expect(settings!.getBoundingClientRect().height).toBeGreaterThan(60);
});
