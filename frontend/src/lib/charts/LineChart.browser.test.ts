import { beforeEach, expect, test, vi } from 'vitest';
import { page } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';
import LineChart from './LineChart.svelte';

const TOKENS = `:root{--bg:#10151c;--bg-panel:#19202a;--bg-raised:#212b38;--border:#46536a;
--border-soft:rgba(255,255,255,.05);--bg-hover:rgba(255,255,255,.08);--text:#e8eef7;
--text-dim:#a9b6c6;--text-faint:#8593a3;--accent:#4a86ff;--radius:6px;--radius-sm:4px;
--mono:monospace;--font:sans-serif;
--chart-1:#3987e5;--chart-2:#d95926;--chart-3:#199e70;--chart-4:#c98500;
--chart-5:#d55181;--chart-6:#008300;--chart-7:#9085e9;--chart-8:#e66767;
--chart-grid:rgba(255,255,255,.1);font-family:var(--font);font-size:13px}
body{color:var(--text);background:var(--bg);margin:0;width:520px}
button{font:inherit;color:inherit;background:none;border:none;padding:0;cursor:pointer}`;

/** A series of `n` points, `from` seconds apart, rising to `peak`. */
function ramp(name: string, n: number, peak: number) {
    return {
        name,
        points: Array.from({ length: n }, (_, i) => ({ t: 1700000000 + i * 30, v: (peak * (i + 1)) / n })),
    };
}

beforeEach(() => {
    document.body.innerHTML = '';
    const style = document.createElement('style');
    style.textContent = TOKENS;
    document.head.appendChild(style);
});

test('draws one path per series, in palette order', async () => {
    render(LineChart, { props: { series: [ramp('a', 5, 1), ramp('b', 5, 2)], unit: 'cores' } });

    const paths = document.querySelectorAll('path.line');
    expect(paths).toHaveLength(2);
    // Assigned in order and never cycled: that order is what keeps neighbouring
    // lines apart for a colourblind reader.
    expect(getComputedStyle(paths[0]).stroke).toBe('rgb(57, 135, 229)');
    expect(getComputedStyle(paths[1]).stroke).toBe('rgb(217, 89, 38)');
});

// Some series colours sit below 3:1 on the lighter themes, so a number is never
// left to be read off the axis alone.
test('writes the current value out, not only the line', async () => {
    render(LineChart, { props: { series: [ramp('only', 4, 0.5)], unit: 'cores' } });

    await expect.element(page.getByText('0.5 cores')).toBeVisible();
});

// One series is already named by the chart's own title; a legend box repeating
// it is ink doing nothing.
test('names its series only when there is more than one', async () => {
    render(LineChart, { props: { series: [ramp('solo', 3, 1)], unit: 'count' } });
    expect(document.querySelectorAll('.legend .key')).toHaveLength(0);

    document.body.innerHTML = '';
    render(LineChart, { props: { series: [ramp('server', 3, 1), ramp('redis', 3, 2)], unit: 'count' } });
    expect(document.querySelectorAll('.legend .key')).toHaveLength(2);
    await expect.element(page.getByText('server')).toBeVisible();
});

test('says so rather than drawing an empty frame', async () => {
    render(LineChart, { props: { series: [] } });

    await expect.element(page.getByText('No data in this window')).toBeVisible();
    expect(document.querySelector('svg')).toBeNull();
});

// A series whose every sample Prometheus could not compute arrives empty; it
// should not contribute a flat line at zero.
test('ignores a series with no points', async () => {
    render(LineChart, { props: { series: [ramp('real', 3, 1), { name: 'empty', points: [] }] } });

    expect(document.querySelectorAll('path.line')).toHaveLength(1);
});

// Readers aim at a time, never at a 2px line, so the crosshair snaps to the
// nearest sample and every series reports at once.
test('the keyboard reads the same values hovering gives', async () => {
    render(LineChart, {
        props: { series: [ramp('server', 4, 1), ramp('redis', 4, 2)], unit: 'count' },
    });

    const svg = document.querySelector('svg') as SVGElement;
    svg.focus();
    svg.dispatchEvent(new KeyboardEvent('keydown', { key: 'End', bubbles: true }));

    // Both series in one readout: the pointer never has to land on a line.
    const readout = await vi.waitUntil(() => document.querySelector('.readout'));
    expect(readout?.querySelectorAll('.row')).toHaveLength(2);
    expect(readout?.textContent).toContain('server');
    expect(readout?.textContent).toContain('redis');
});

test('holds the previous render while refetching rather than blanking', async () => {
    render(LineChart, { props: { series: [ramp('a', 4, 1)], stale: true } });

    const chart = document.querySelector('.chart') as HTMLElement;
    expect(chart.classList.contains('stale')).toBe(true);
    // The lines are still there: no skeleton, no layout jump.
    expect(document.querySelectorAll('path.line')).toHaveLength(1);
});
