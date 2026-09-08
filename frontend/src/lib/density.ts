// How tall a table row is, in pixels, for each density the settings offer.
//
// A module rather than a constant in App.svelte because the two properties it
// produces have to agree -- the row height is what a list measures with, the
// cell padding is what actually produces that height -- and an invariant worth
// stating is worth testing. The shell reads these onto the document root and
// every table follows through var(--row-h) and var(--cell-pad-y); no component
// knows what "spacious" means.

import type { Density } from './state/adopt';

/** The two custom properties a density sets. */
export interface RowMetrics {
    /** var(--row-h): a row's height, which the sidebar's rows also follow. */
    height: string;
    /** var(--cell-pad-y): the padding above and below a table cell. */
    padding: string;
}

/**
 * Padding is a shade under half the difference from the height, because a cell
 * has a line of text between its two paddings. Compact fits about a quarter
 * more rows in the same pane than comfortable; spacious is for a screen read
 * from further away than arm's length, where zoom -- which scales the sidebar
 * and the charts along with the table -- is the wrong tool.
 */
const METRICS: Record<Density, RowMetrics> = {
    compact: { height: '24px', padding: '3px' },
    comfortable: { height: '30px', padding: '6px' },
    spacious: { height: '38px', padding: '10px' },
};

/**
 * The metrics for one density.
 *
 * A density this build does not know falls back to comfortable rather than to
 * nothing: an unset --row-h collapses every row in the sidebar to its content,
 * which is a broken window rather than a wrong preference. The store normalises
 * unknown values already, so this covers the settings file of a newer build
 * being read by an older one.
 */
export function rowMetrics(density: string): RowMetrics {
    return METRICS[density as Density] ?? METRICS.comfortable;
}
