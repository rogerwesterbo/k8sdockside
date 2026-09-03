import { beforeEach, expect, test, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';

// The shortcuts are handled on the window, so this drives real key events
// against the mounted app shell rather than calling the store directly.
// Only Window.SetZoom is replaced: the generated bindings import the rest of
// the runtime, and stubbing the whole module breaks them.
vi.mock('@wailsio/runtime', async (importOriginal) => {
    const actual = await importOriginal<typeof import('@wailsio/runtime')>();
    return { ...actual, Window: { ...actual.Window, SetZoom: vi.fn().mockResolvedValue(undefined) } };
});

const { workspace } = await import('../state/workspace.svelte');
const App = (await import('../../App.svelte')).default;

function press(key: string, code = ''): void {
    window.dispatchEvent(new KeyboardEvent('keydown', { key, code, metaKey: true, bubbles: true, cancelable: true }));
}

beforeEach(async () => {
    document.body.innerHTML = '';
    workspace.settings.layout.zoom = 1;
    render(App);
    await new Promise((r) => setTimeout(r, 60));
});

test('cmd + = zooms in', () => {
    press('=');
    expect(workspace.zoom).toBeGreaterThan(1);
});

test('cmd + + zooms in, for layouts where plus needs no shift', () => {
    press('+');
    expect(workspace.zoom).toBeGreaterThan(1);
});

test('cmd + - zooms out', () => {
    press('-');
    expect(workspace.zoom).toBeLessThan(1);
});

test('cmd + 0 returns to normal size', () => {
    press('=');
    press('=');
    expect(workspace.zoom).toBeGreaterThan(1);

    press('0');
    expect(workspace.zoom).toBe(1);
});

test('the numeric keypad works too', () => {
    press('+', 'NumpadAdd');
    expect(workspace.zoom).toBeGreaterThan(1);
});

test('the same keys without the modifier are left alone', () => {
    window.dispatchEvent(new KeyboardEvent('keydown', { key: '-', bubbles: true, cancelable: true }));
    expect(workspace.zoom).toBe(1);
});

// Typing a minus into the context filter must not shrink the window.
test('the shortcut is claimed, so the browser does not also act on it', () => {
    const event = new KeyboardEvent('keydown', { key: '-', metaKey: true, bubbles: true, cancelable: true });
    window.dispatchEvent(event);
    expect(event.defaultPrevented).toBe(true);
});

// The title bar is drawn in CSS pixels but the macOS traffic lights over it are
// not, so zooming out has to make the bar taller in CSS terms to compensate.
test('zooming out grows the title bar so the traffic lights still fit', async () => {
    press('-');
    await new Promise((r) => setTimeout(r, 60));

    const declared = document.documentElement.style.getPropertyValue('--topbar-h');
    expect(parseFloat(declared)).toBeGreaterThan(44);
});

// Zooming in needs no compensation: everything inside #app scales, so the bar
// grows on its own and goes on containing the traffic lights. Only zooming out
// has to be corrected for, which is why the height is a max rather than a
// straight division.
test('zooming in leaves the title bar to grow with everything else', async () => {
    press('=');
    press('=');
    await new Promise((r) => setTimeout(r, 60));

    expect(parseFloat(document.documentElement.style.getPropertyValue('--topbar-h'))).toBe(44);
});
