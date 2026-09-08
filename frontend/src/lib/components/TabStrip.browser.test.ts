import { beforeEach, expect, test, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';

import TabStrip from './TabStrip.svelte';
import { endTabDrag } from '../state/tabdrag.svelte';

// Enough tabs to overflow the narrow body below, which is the whole point:
// everything here is about the strip once it scrolls.
const TOKENS = `:root{--bg:#10151c;--bg-sidebar:#151b24;--bg-panel:#19202a;--bg-raised:#212b38;
--border:#46536a;--border-soft:rgba(255,255,255,.05);--bg-hover:rgba(255,255,255,.08);
--text:#e8eef7;--text-dim:#a9b6c6;--text-faint:#8593a3;--accent:#4a86ff;--ok:#5fd39b;
--warn:#efb567;--error:#f4787f;--radius:6px;--radius-sm:4px;--mono:monospace;
--font:-apple-system,sans-serif;font-family:var(--font);font-size:13px}
body{color:var(--text);margin:0;width:320px}
button{font:inherit;color:inherit;background:none;border:none;padding:0;cursor:pointer}`;

function tabs(count: number, title = (i: number) => `A tab with a long enough name ${i}`) {
    return Array.from({ length: count }, (_, i) => ({
        id: `tab-${i}`,
        title: title(i),
        icon: 'box',
        color: '#4a86ff',
    }));
}

function mount(count = 12, title?: (i: number) => string) {
    const onmove = vi.fn();
    render(TabStrip, {
        props: {
            tabs: tabs(count, title),
            activeId: 'tab-0',
            label: 'Open views',
            pane: 'main' as never,
            onactivate: () => {},
            onclose: () => {},
            onmove,
        },
    });
    const bar = document.querySelector<HTMLElement>('[role="tablist"]')!;
    return { bar, onmove };
}

/** One frame of the drag's auto-scroll, plus room for it to be scheduled. */
const frame = () => new Promise((r) => setTimeout(r, 60));

/** Drags a tab and holds the pointer at one edge of the strip, as a user does. */
function dragTo(tab: Element, clientX: number): DataTransfer {
    const dataTransfer = new DataTransfer();
    tab.dispatchEvent(new DragEvent('dragstart', { bubbles: true, cancelable: true, dataTransfer }));
    tab.dispatchEvent(new DragEvent('dragover', { bubbles: true, cancelable: true, dataTransfer, clientX }));
    return dataTransfer;
}

beforeEach(() => {
    document.body.innerHTML = '';
    document.head.querySelectorAll('style[data-t]').forEach((n) => n.remove());
    const style = document.createElement('style');
    style.dataset.t = 'y';
    style.textContent = TOKENS;
    document.head.appendChild(style);
    endTabDrag();
});

// The bug this exists for: nothing scrolls an overflowing element while a drag
// is in flight, so the tabs past the right edge -- the last one among them --
// could not be reached, and a tab could not be dragged to the end.
test('holding a dragged tab at the right edge scrolls the strip', async () => {
    const { bar } = mount();
    expect(bar.scrollWidth).toBeGreaterThan(bar.clientWidth);
    expect(bar.scrollLeft).toBe(0);

    const box = bar.getBoundingClientRect();
    dragTo(bar.querySelector('.tab')!, box.right - 4);
    await frame();

    expect(bar.scrollLeft).toBeGreaterThan(0);
});

test('and holding it at the left edge scrolls back', async () => {
    const { bar } = mount();
    // Put it at the far end without animating: the strip sets
    // scroll-behavior: smooth, and an animation still settling would fight the
    // scroll being measured here.
    bar.scrollTo({ left: bar.scrollWidth - bar.clientWidth, behavior: 'instant' });
    await frame();
    const from = bar.scrollLeft;
    expect(from).toBeGreaterThan(0);

    const box = bar.getBoundingClientRect();
    dragTo(bar.querySelector('.tab')!, box.left + 4);
    await frame();

    expect(bar.scrollLeft).toBeLessThan(from);
});

test('a drag held in the middle of the strip scrolls nothing', async () => {
    // Only the edges mean "show me what is past this". A strip that crept
    // sideways while the pointer sat still over a tab would be unusable.
    const { bar } = mount();
    const box = bar.getBoundingClientRect();

    dragTo(bar.querySelector('.tab')!, (box.left + box.right) / 2);
    await frame();

    expect(bar.scrollLeft).toBe(0);
});

test('the scroll stops when the drag ends', async () => {
    const { bar } = mount();
    const box = bar.getBoundingClientRect();
    const tab = bar.querySelector('.tab')!;

    dragTo(tab, box.right - 4);
    await frame();
    tab.dispatchEvent(new DragEvent('dragend', { bubbles: true, cancelable: true }));
    const settled = bar.scrollLeft;

    await frame();
    await frame();

    expect(bar.scrollLeft).toBe(settled);
});

// The arrows are drawn over the ends of the strip, so a drag reaching for the
// tabs behind one lands on a button rather than on a tab.
test('the scroll arrow does not swallow a drag held over it', async () => {
    const { bar } = mount();
    const arrow = document.querySelector<HTMLElement>('.nudge.right')!;
    const box = arrow.getBoundingClientRect();

    const dataTransfer = new DataTransfer();
    bar.querySelector('.tab')!.dispatchEvent(
        new DragEvent('dragstart', { bubbles: true, cancelable: true, dataTransfer }),
    );
    arrow.dispatchEvent(
        new DragEvent('dragover', {
            bubbles: true,
            cancelable: true,
            dataTransfer,
            clientX: (box.left + box.right) / 2,
        }),
    );
    await frame();

    expect(bar.scrollLeft).toBeGreaterThan(0);
});

// A strip that fits needs none of this, and must not acquire a scroll it has
// nowhere to go with.
test('a strip with room for its tabs stays put', async () => {
    const { bar } = mount(2, (i) => `t${i}`);
    expect(bar.scrollWidth).toBe(bar.clientWidth);
    const box = bar.getBoundingClientRect();

    dragTo(bar.querySelector('.tab')!, box.right - 4);
    await frame();

    expect(bar.scrollLeft).toBe(0);
});

// ----- reaching every position ------------------------------------------
//
// A strip is not a row of tabs and nothing else: arrows are drawn over its
// ends, there is padding at each end and a gap between every pair, and the
// pane's controls sit past them all. Reordering used to need the pointer to
// land on a particular neighbouring tab, so the parts of the strip that are not
// tab were dead -- which is most of the end of one that scrolls, and why a tab
// could not be taken out of, or into, the last position.

/** Presses on a tab, then holds the pointer at one x within the strip. */
function dragFrom(bar: HTMLElement, tabIndex: number, clientX: number) {
    const tabs = bar.querySelectorAll<HTMLElement>('.tab');
    const dataTransfer = new DataTransfer();
    tabs[tabIndex].dispatchEvent(
        new DragEvent('dragstart', { bubbles: true, cancelable: true, dataTransfer }),
    );
    bar.dispatchEvent(new DragEvent('dragover', { bubbles: true, cancelable: true, dataTransfer, clientX }));
}

const midOf = (bar: HTMLElement, index: number) => {
    const box = bar.querySelectorAll<HTMLElement>('.tab')[index].getBoundingClientRect();
    return box.left + box.width / 2;
};

test('the last tab can be moved somewhere else', async () => {
    const { bar, onmove } = mount(8);
    bar.scrollTo({ left: bar.scrollWidth - bar.clientWidth, behavior: 'instant' });
    await frame();

    // Onto the left half of the third tab, which is where it should land.
    dragFrom(bar, 7, midOf(bar, 2) - 8);

    expect(onmove).toHaveBeenCalledWith(7, 2);
});

test('a tab can be taken to the last position', async () => {
    const { bar, onmove } = mount(8);

    dragFrom(bar, 0, midOf(bar, 7) + 8);

    expect(onmove).toHaveBeenCalledWith(0, 7);
});

test('the end of the strip is a position, arrow and all', async () => {
    // The right-hand end of a scrolling strip is arrow, padding and controls
    // rather than tab. Holding a drag there means "further along", and used to
    // mean nothing at all.
    const { bar, onmove } = mount(8);
    const box = bar.getBoundingClientRect();

    dragFrom(bar, 0, box.right - 2);

    expect(onmove).toHaveBeenCalledWith(0, 1);
});

// The whole bug, end to end: with more tabs than fit, holding a dragged tab at
// the right-hand end walks it along as the strip scrolls, and it can be put in
// the last position -- which needs both halves of the fix, since the tabs it
// has to pass are off screen when the drag starts and the end of the strip is
// not tab.
test('holding at the end walks a tab all the way to last', async () => {
    const { bar, onmove } = mount(8);
    const dataTransfer = new DataTransfer();
    const x = bar.getBoundingClientRect().right - 2;

    bar.querySelectorAll<HTMLElement>('.tab')[0].dispatchEvent(
        new DragEvent('dragstart', { bubbles: true, cancelable: true, dataTransfer }),
    );
    // A held pointer still gets dragover events; the strip scrolls under it and
    // each one lands further along. Held until the strip runs out of scroll,
    // which is where the last position finally comes under the pointer.
    const end = bar.scrollWidth - bar.clientWidth;
    for (let i = 0; i < 200 && bar.scrollLeft < end; i++) {
        bar.dispatchEvent(new DragEvent('dragover', { bubbles: true, cancelable: true, dataTransfer, clientX: x }));
        await new Promise((r) => setTimeout(r, 25));
    }
    bar.dispatchEvent(new DragEvent('dragover', { bubbles: true, cancelable: true, dataTransfer, clientX: x }));

    expect(bar.scrollLeft).toBe(end);
    expect(onmove).toHaveBeenLastCalledWith(0, 7);
});

test('the gap between two tabs is a position, not a hole', async () => {
    const { bar, onmove } = mount(8);
    const first = bar.querySelectorAll<HTMLElement>('.tab')[3].getBoundingClientRect();

    // One pixel left of a tab's leading edge: in the gap before it.
    dragFrom(bar, 0, first.left - 1);

    expect(onmove).toHaveBeenCalledWith(0, 3);
});

test('a tab dragged onto its own position is left alone', async () => {
    const { bar, onmove } = mount(8);

    dragFrom(bar, 3, midOf(bar, 3));

    expect(onmove).not.toHaveBeenCalled();
});

// Reaching a neighbour is enough to displace it. Asking for its midpoint would
// mean dragging as far as half a tab before anything happened -- and on a strip
// that scrolls that point is often off screen, so the swap could not be asked
// for at all.
test('a neighbour is displaced on being reached, not on being halved', async () => {
    const { bar, onmove } = mount(8);
    const neighbour = bar.querySelectorAll<HTMLElement>('.tab')[2].getBoundingClientRect();

    // Just inside its trailing edge: over the tab, nowhere near its middle.
    dragFrom(bar, 3, neighbour.right - 4);

    expect(onmove).toHaveBeenCalledWith(3, 2);
});
