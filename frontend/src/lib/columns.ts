// The columns of a resource table, and what the user changed about them.
//
// Two things are the user's to decide per kind per context: how wide each
// column is, and which ones are shown at all. Both are stored by column *key*
// rather than by position -- see appconfig.ColumnPrefs for why -- and this
// module is the one place that turns a kind's column names into those keys, so
// what the table draws and what the settings file holds cannot drift apart.

/** How wide each column was dragged, and which are turned off. By column key. */
export interface ColumnPrefs {
    widths: Record<string, number>;
    hidden: string[];
}

/** One column as the table draws it: its heading, where it sits, and its key. */
export interface Column {
    /** The heading the backend sent. */
    name: string;
    /**
     * Where it sits in the table the backend sent. Kept through hiding: a row's
     * cells arrive in the backend's order, and the sort is by that index, so a
     * shown column has to remember which cell is its own.
     */
    index: number;
    /** What the settings file calls it -- see columnKeys. */
    key: string;
}

/** The range a column may be dragged to. Mirrors appconfig's own bounds. */
export const MIN_COLUMN_WIDTH = 48;
export const MAX_COLUMN_WIDTH = 1600;

/** Nothing set: every column shown, each at the width its contents want. */
export function noColumnPrefs(): ColumnPrefs {
    return { widths: {}, hidden: [] };
}

/**
 * The settings key for each column, in the backend's order.
 *
 * The name, except where a kind declares the same name twice -- a CRD printer
 * column called "Name" beside the Name the app puts first -- in which case the
 * repeats carry an occurrence suffix. Without it the two would share a width
 * and hide together, and hiding "Name" would take the wrong one with it.
 */
export function columnKeys(columns: string[]): string[] {
    const seen = new Map<string, number>();
    return columns.map((name) => {
        const nth = (seen.get(name) ?? 0) + 1;
        seen.set(name, nth);
        return nth === 1 ? name : `${name}#${nth}`;
    });
}

/** Every column of a table, keyed and in the order the backend sent them. */
export function describeColumns(columns: string[]): Column[] {
    const keys = columnKeys(columns);
    return columns.map((name, index) => ({ name, index, key: keys[index] }));
}

/**
 * The columns actually drawn: everything the user has not turned off.
 *
 * Never empty. A table with no columns is a stack of blank rows with no way
 * back -- the picker it would be turned on from lists nothing to tick -- so the
 * last column stands whatever the settings file says. That can only be reached
 * by hand-editing the file; the picker itself refuses the last tick.
 */
export function visibleColumns(columns: string[], hidden: string[]): Column[] {
    const all = describeColumns(columns);
    if (all.length === 0) return all;
    const off = new Set(hidden);
    const shown = all.filter((column) => !off.has(column.key));
    return shown.length > 0 ? shown : [all[0]];
}

/** Puts a width inside the range a column may be dragged to. */
export function clampColumnWidth(px: number): number {
    return Math.round(Math.min(MAX_COLUMN_WIDTH, Math.max(MIN_COLUMN_WIDTH, px)));
}
