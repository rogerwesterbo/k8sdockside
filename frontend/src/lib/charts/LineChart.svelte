<!--
  One time-series chart, drawn as inline SVG.

  Inline rather than a charting library for two reasons. The obvious one is
  weight — this is a few hundred lines against a few hundred kilobytes. The one
  that actually decided it is themes: every colour here is a CSS custom property,
  so a chart repaints with the rest of the app when the theme changes, with no
  library configuration to keep in step across thirteen palettes.

  The series colours are --chart-1..8, assigned in order and never cycled. That
  order is not a preference: it is what keeps neighbouring lines apart under
  protanopia and deuteranopia. A ninth series folds into the eighth rather than
  being given an invented hue.

  Values are also written out — in the legend, at the line's end, and in the
  tooltip — rather than left to be read off the axis. That is deliberate: some
  of the series colours sit below 3:1 against the lighter surfaces, so the chart
  never relies on colour alone to carry a number.
-->
<script lang="ts">
    import { formatMoment, formatTick, formatTime, formatValue, ticksFor, type Unit } from './format';

    export interface Series {
        name: string;
        points: { t: number; v: number }[];
    }

    interface Props {
        series: Series[];
        unit?: Unit;
        /** How far back the window reaches, for wording the time axis. */
        spanMinutes?: number;
        height?: number;
        /** Dimmed while a refresh is in flight, holding the previous render. */
        stale?: boolean;
    }

    let { series, unit = '', spanMinutes = 60, height = 132, stale = false }: Props = $props();

    /** How many colours the palette has; a ninth series folds into the last. */
    const SLOTS = 8;

    // The plot box. Left gutter fits an axis label like "512 MiB"; the right
    // one leaves room for the value at the end of each line.
    const PAD = { top: 10, right: 8, bottom: 18, left: 46 };
    /** The viewBox width. The SVG scales to its container; only the ratio matters. */
    const W = 480;

    let hoverX = $state<number | null>(null);
    let plot = $state<SVGRectElement>();

    let drawn = $derived(series.filter((s) => s.points.length > 0));

    /** The window the chart covers, taken from the data rather than the clock. */
    let bounds = $derived.by(() => {
        let minT = Infinity;
        let maxT = -Infinity;
        let maxV = 0;
        for (const s of drawn) {
            for (const p of s.points) {
                if (p.t < minT) minT = p.t;
                if (p.t > maxT) maxT = p.t;
                if (p.v > maxV) maxV = p.v;
            }
        }
        if (!Number.isFinite(minT)) return { minT: 0, maxT: 1, maxV: 1 };
        // A flat line at zero still needs a scale to be drawn against.
        return { minT, maxT: maxT === minT ? minT + 1 : maxT, maxV: maxV > 0 ? maxV : 1 };
    });

    let ticks = $derived(ticksFor(bounds.maxV));
    /** The top of the scale is the highest tick, so the axis is not cut off. */
    let top = $derived(Math.max(ticks[ticks.length - 1] ?? 1, bounds.maxV));

    let innerW = $derived(W - PAD.left - PAD.right);
    let innerH = $derived(height - PAD.top - PAD.bottom);

    function x(t: number): number {
        return PAD.left + ((t - bounds.minT) / (bounds.maxT - bounds.minT)) * innerW;
    }

    function y(v: number): number {
        return PAD.top + innerH - (v / top) * innerH;
    }

    /** The colour of the nth series. Assigned in order, never cycled. */
    function colorOf(index: number): string {
        return `var(--chart-${Math.min(index, SLOTS - 1) + 1})`;
    }

    function path(s: Series): string {
        return s.points.map((p, i) => `${i === 0 ? 'M' : 'L'}${x(p.t).toFixed(2)},${y(p.v).toFixed(2)}`).join(' ');
    }

    /** The time positions every series shares, for the crosshair to snap to. */
    let stops = $derived.by(() => {
        const all = new Set<number>();
        for (const s of drawn) for (const p of s.points) all.add(p.t);
        return [...all].sort((a, b) => a - b);
    });

    /** The data position nearest the pointer — readers aim at a time, not a line. */
    let hovered = $derived.by(() => {
        if (hoverX === null || stops.length === 0) return null;
        const wanted = bounds.minT + ((hoverX - PAD.left) / innerW) * (bounds.maxT - bounds.minT);
        let best = stops[0];
        for (const stop of stops) {
            if (Math.abs(stop - wanted) < Math.abs(best - wanted)) best = stop;
        }
        return best;
    });

    /** Every series' value at the hovered time; the pointer never has to find a line. */
    let readout = $derived.by(() => {
        if (hovered === null) return [];
        return drawn.map((s, i) => {
            const at = s.points.find((p) => p.t === hovered);
            return { name: s.name, color: colorOf(i), value: at?.v ?? null };
        });
    });

    /** The last value of each series, written at the end of its line. */
    function endOf(s: Series): { x: number; y: number; v: number } | null {
        const last = s.points[s.points.length - 1];
        return last ? { x: x(last.t), y: y(last.v), v: last.v } : null;
    }

    function track(event: PointerEvent): void {
        if (!plot) return;
        const box = plot.getBoundingClientRect();
        hoverX = ((event.clientX - box.left) / box.width) * innerW + PAD.left;
    }

    /** Keyboard reading: the same numbers, without a pointer. */
    function step(event: KeyboardEvent): void {
        if (stops.length === 0) return;
        const at = hovered === null ? stops.length - 1 : stops.indexOf(hovered);
        let next = at;
        if (event.key === 'ArrowLeft') next = Math.max(0, at - 1);
        else if (event.key === 'ArrowRight') next = Math.min(stops.length - 1, at + 1);
        else if (event.key === 'Home') next = 0;
        else if (event.key === 'End') next = stops.length - 1;
        else return;

        event.preventDefault();
        hoverX = x(stops[next]);
    }
</script>

<div class="chart" class:stale>
    {#if drawn.length === 0}
        <p class="empty">No data in this window</p>
    {:else}
        <!-- The chart is a graphic that can also be read: role="img" is what it
             is, and the key handler is how someone without a pointer gets the
             same numbers hovering gives. The rules below only see a
             non-interactive role with listeners on it. -->
        <!-- svelte-ignore a11y_no_noninteractive_tabindex -->
        <!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
        <svg
            viewBox="0 0 {W} {height}"
            preserveAspectRatio="none"
            role="img"
            aria-label="{drawn.length === 1 ? drawn[0].name || 'series' : `${drawn.length} series`} over the last {spanMinutes} minutes"
            tabindex="0"
            onpointermove={track}
            onpointerleave={() => (hoverX = null)}
            onkeydown={step}
            onblur={() => (hoverX = null)}
        >
            <!-- Gridlines: hairline, solid, one step off the surface. Recessive
                 enough that the lines are what the eye lands on. -->
            {#each ticks as tick (tick)}
                <line class="grid" x1={PAD.left} x2={W - PAD.right} y1={y(tick)} y2={y(tick)} />
                <text class="tick" x={PAD.left - 6} y={y(tick)} text-anchor="end" dominant-baseline="middle">
                    {formatTick(tick, unit)}
                </text>
            {/each}

            <text class="tick" x={PAD.left} y={height - 5}>{formatTime(bounds.minT)}</text>
            <text class="tick" x={W - PAD.right} y={height - 5} text-anchor="end">{formatTime(bounds.maxT)}</text>

            {#each drawn as s, i (s.name + i)}
                <path class="line" d={path(s)} stroke={colorOf(i)} />
            {/each}

            <!-- The value at the end of each line: the direct label that keeps a
                 number reachable without hovering, which is what the lighter
                 series colours need to be legible on the lighter themes. -->
            {#if drawn.length === 1}
                {@const end = endOf(drawn[0])}
                {#if end}
                    <circle class="cap" cx={end.x} cy={end.y} r="3.5" fill={colorOf(0)} />
                {/if}
            {/if}

            {#if hovered !== null}
                <line class="crosshair" x1={x(hovered)} x2={x(hovered)} y1={PAD.top} y2={PAD.top + innerH} />
                {#each drawn as s, i (s.name + i)}
                    {@const at = s.points.find((p) => p.t === hovered)}
                    {#if at}
                        <circle class="cap" cx={x(at.t)} cy={y(at.v)} r="3.5" fill={colorOf(i)} />
                    {/if}
                {/each}
            {/if}

            <!-- The hit area, over everything: a 2px line is not something to
                 aim at, so the whole plot is the target. -->
            <rect
                bind:this={plot}
                class="hit"
                x={PAD.left}
                y={PAD.top}
                width={innerW}
                height={innerH}
                fill="transparent"
            />
        </svg>

        <!-- Values lead, names follow: here the reader has the series and wants
             the number. Names and values wear text tokens; only the key carries
             the colour. -->
        {#if hovered !== null}
            <div class="readout" role="status">
                <span class="moment">{formatMoment(hovered, spanMinutes)}</span>
                {#each readout as row (row.name)}
                    <span class="row">
                        <span class="key" style:background={row.color}></span>
                        <span class="value">{row.value === null ? '—' : formatValue(row.value, unit)}</span>
                        {#if row.name}<span class="name">{row.name}</span>{/if}
                    </span>
                {/each}
            </div>
        {:else}
            <!-- The legend doubles as the resting readout: with one series its
                 name is already the chart's title, so only the current value is
                 worth the room. -->
            <div class="legend">
                {#each drawn as s, i (s.name + i)}
                    {@const end = endOf(s)}
                    <span class="row">
                        {#if drawn.length > 1}
                            <span class="key" style:background={colorOf(i)}></span>
                            <span class="name">{s.name || `series ${i + 1}`}</span>
                        {/if}
                        <span class="value">{end ? formatValue(end.v, unit) : '—'}</span>
                    </span>
                {/each}
            </div>
        {/if}
    {/if}
</div>

<style>
    .chart {
        position: relative;
    }

    /* A refresh holds the previous render rather than blanking: no skeleton, no
       layout jump, nothing to re-read when the new numbers land. */
    .stale {
        opacity: 0.55;
        transition: opacity 120ms ease;
    }

    svg {
        display: block;
        width: 100%;
        height: auto;
        overflow: visible;
    }

    svg:focus-visible {
        outline: 2px solid var(--accent);
        outline-offset: 2px;
        border-radius: var(--radius-sm);
    }

    .grid {
        stroke: var(--chart-grid);
        stroke-width: 1;
        /* Solid, never dashed: a dashed gridline competes with the data. */
        vector-effect: non-scaling-stroke;
    }

    .line {
        fill: none;
        stroke-width: 2;
        stroke-linejoin: round;
        stroke-linecap: round;
        vector-effect: non-scaling-stroke;
    }

    /* The surface ring keeps an end-cap legible where two lines cross. */
    .cap {
        stroke: var(--bg-panel);
        stroke-width: 2;
        vector-effect: non-scaling-stroke;
    }

    .crosshair {
        stroke: var(--text-faint);
        stroke-width: 1;
        vector-effect: non-scaling-stroke;
    }

    /* Axis text wears a text token, never a series colour. */
    .tick {
        fill: var(--text-faint);
        font-size: 9px;
        font-family: var(--mono);
    }

    .empty {
        margin: 0;
        padding: 24px 0;
        text-align: center;
        font-size: 11.5px;
        color: var(--text-faint);
    }

    .legend,
    .readout {
        display: flex;
        flex-wrap: wrap;
        align-items: baseline;
        gap: 4px 12px;
        margin-top: 6px;
        font-size: 11px;
        min-height: 16px;
    }

    .row {
        display: flex;
        align-items: baseline;
        gap: 5px;
        min-width: 0;
    }

    /* A short stroke rather than a filled box: at this density a box is
       data-weight ink doing a label's job. */
    .key {
        width: 10px;
        height: 2px;
        border-radius: 1px;
        flex: 0 0 auto;
        transform: translateY(-3px);
    }

    .value {
        font-family: var(--mono);
        color: var(--text);
        font-variant-numeric: tabular-nums;
    }

    .name {
        color: var(--text-faint);
        overflow: hidden;
        text-overflow: ellipsis;
        white-space: nowrap;
        max-width: 18ch;
    }

    .moment {
        font-family: var(--mono);
        color: var(--text-faint);
    }
</style>
