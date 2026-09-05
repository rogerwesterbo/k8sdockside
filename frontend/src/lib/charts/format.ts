// How a metric's number is written.
//
// Units matter more here than they look. A chart labelled "Memory" whose axis
// reads 536870912 is a chart nobody can use, and one reading 0.4823 cores is a
// chart that makes you count decimal places. Every value on screen -- the axis,
// the legend, the tooltip -- goes through here so the three always agree.

/** The units a plugin may declare, matching plugins.ChartUnits on the Go side. */
export type Unit = '' | 'cores' | 'bytes' | 'bytes/s' | 'percent' | 'ops/s' | 'seconds' | 'count';

/** Binary prefixes: memory is reported in bytes and read in MiB. */
const BYTE_STEPS = ['B', 'KiB', 'MiB', 'GiB', 'TiB', 'PiB'];

/**
 * A value written for a reader, with its unit.
 *
 * Significant figures rather than a fixed number of decimals: 0.42 cores and
 * 12.3 cores are both worth three characters, while 0.4200 and 12.3000 are the
 * same information spent differently.
 */
export function formatValue(value: number, unit: Unit): string {
    if (!Number.isFinite(value)) return '—';

    switch (unit) {
        case 'bytes':
        case 'bytes/s': {
            const { scaled, suffix } = scaleBytes(value);
            return `${trim(scaled)} ${suffix}${unit === 'bytes/s' ? '/s' : ''}`;
        }
        case 'percent':
            return `${trim(value * 100)}%`;
        case 'cores':
            // Below a hundredth of a core, millicores are how Kubernetes itself
            // writes it and how the reader thinks about it.
            return value > 0 && value < 0.01 ? `${Math.round(value * 1000)}m` : `${trim(value)} cores`;
        case 'ops/s':
            return `${trim(value)}/s`;
        case 'seconds':
            return formatSeconds(value);
        case 'count':
            return trim(value);
        default:
            return trim(value);
    }
}

/** The same value with no unit, for an axis tick where the unit is in the title. */
export function formatTick(value: number, unit: Unit): string {
    if (!Number.isFinite(value)) return '';

    switch (unit) {
        case 'bytes':
        case 'bytes/s': {
            const { scaled, suffix } = scaleBytes(value);
            return `${trim(scaled)} ${suffix}`;
        }
        case 'percent':
            return `${trim(value * 100)}%`;
        case 'seconds':
            return formatSeconds(value);
        default:
            return trim(value);
    }
}

function scaleBytes(value: number): { scaled: number; suffix: string } {
    const sign = value < 0 ? -1 : 1;
    let scaled = Math.abs(value);
    let step = 0;
    while (scaled >= 1024 && step < BYTE_STEPS.length - 1) {
        scaled /= 1024;
        step++;
    }
    return { scaled: sign * scaled, suffix: BYTE_STEPS[step] };
}

function formatSeconds(value: number): string {
    if (value < 1) return `${trim(value * 1000)} ms`;
    if (value < 60) return `${trim(value)} s`;
    return `${trim(value / 60)} min`;
}

/** Three significant figures, without a trailing `.00`. */
function trim(value: number): string {
    const magnitude = Math.abs(value);
    let text: string;
    if (magnitude === 0) text = '0';
    else if (magnitude >= 100) text = value.toFixed(0);
    else if (magnitude >= 10) text = value.toFixed(1);
    else if (magnitude >= 1) text = value.toFixed(2);
    else text = value.toPrecision(2);

    // toFixed and toPrecision both leave zeros that say nothing.
    if (text.includes('.')) text = text.replace(/\.?0+$/, '');
    // Group thousands: an axis reading 12000 and one reading 12,000 are not
    // equally quick to read.
    const [whole, fraction] = text.split('.');
    const grouped = whole.replace(/\B(?=(\d{3})+(?!\d))/g, ',');
    return fraction ? `${grouped}.${fraction}` : grouped;
}

/** A moment on the time axis, as a clock reading. */
export function formatTime(unixSeconds: number): string {
    return new Date(unixSeconds * 1000).toLocaleTimeString(undefined, {
        hour: '2-digit',
        minute: '2-digit',
    });
}

/** A moment for the tooltip, where the date matters on a long range. */
export function formatMoment(unixSeconds: number, spanMinutes: number): string {
    const at = new Date(unixSeconds * 1000);
    const clock = at.toLocaleTimeString(undefined, { hour: '2-digit', minute: '2-digit', second: '2-digit' });
    if (spanMinutes <= 24 * 60) return clock;
    return `${at.toLocaleDateString(undefined, { month: 'short', day: 'numeric' })} ${clock}`;
}

/**
 * Axis ticks at round numbers covering [0, max].
 *
 * Always anchored at zero: these are rates and sizes, where the distance from
 * nothing is the thing being read, and a chart whose axis starts at 0.38 turns
 * a flat line into a mountain range.
 */
export function ticksFor(max: number, count = 3): number[] {
    if (!Number.isFinite(max) || max <= 0) return [0, 1];

    const rough = max / count;
    const magnitude = 10 ** Math.floor(Math.log10(rough));
    const step = [1, 2, 2.5, 5, 10].map((m) => m * magnitude).find((s) => s >= rough) ?? magnitude * 10;

    // Up to the first round number at or above the data, never the last one
    // below it: the top tick is what the plot is scaled against, so one that
    // fell short would draw the tallest line above its own axis.
    const top = Math.ceil(max / step - 1e-9);
    const ticks: number[] = [];
    for (let i = 0; i <= top; i++) {
        // Multiplied rather than accumulated, and rounded: adding 0.2 three
        // times gives 0.6000000000000001, which is what the axis would print.
        ticks.push(round(i * step, step));
    }
    return ticks;
}

/** Rounds a tick to the precision its own step implies. */
function round(value: number, step: number): number {
    const places = Math.max(0, -Math.floor(Math.log10(step)) + 1);
    return Number(value.toFixed(Math.min(places, 12)));
}
