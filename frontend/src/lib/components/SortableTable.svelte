<!--
  A sortable listing of rows in the shape the backend produces for every kind.

  It is shared by the resource tabs and the dashboard's events panel so there is
  one sorting implementation rather than two that can disagree: sorting by a
  cell's key rather than its text is the sort of detail that gets fixed in one
  copy and not the other.

  Sorting only: filtering, loading and error states belong to the caller, which
  knows what it is listing and what an empty one means.
-->
<script lang="ts">
    import type { Row } from '../state/adopt';
    import type { Snippet } from 'svelte';
    import Icon from './Icon.svelte';

    interface Props {
        columns: string[];
        rows: Row[];
        /** Row to mark as selected, if the caller tracks one. */
        selectedRowId?: string | null;
        /** Called when a row is clicked. */
        onselect?: (row: Row) => void;
        /** What to show in place of rows when there are none. */
        empty?: string;
        /** Overrides how one cell renders, for callers with a link in a column. */
        cell?: Snippet<[Row, number]>;
        /**
         * The rows that are ticked, by id, when the caller offers a selection.
         * The checkbox column is drawn only when this is given: a selection
         * with nothing to do to it would be clutter.
         */
        picked?: Set<string> | null;
        /**
         * Called with the rows to tick or untick. Shift on a checkbox sweeps
         * from the last row ticked to this one in the order on screen -- which
         * only this component knows, since it is the one sorting.
         */
        onpick?: (rows: Row[], on: boolean) => void;
        /**
         * The column the rows are sorted by, and which way. Bindable, so a
         * caller can keep a sort across the table's lifetime: a resource tab
         * is destroyed when another is brought forward and built again on the
         * way back, and a sort that reset each time made "the newest pods"
         * something to ask for again at every switch.
         *
         * Null means "the order the caller gave", which the backend has
         * already put in each kind's natural order -- events most recent
         * first, everything else by namespace and name. Sorting before the
         * user asks would undo that.
         */
        sortColumn?: number | null;
        sortDescending?: boolean;
    }

    let {
        columns,
        rows,
        selectedRowId = null,
        onselect,
        empty = 'Nothing here.',
        cell,
        picked = null,
        onpick,
        sortColumn = $bindable(null),
        sortDescending = $bindable(false),
    }: Props = $props();

    /** The row last ticked on its own, which is where a shift-click sweeps from. */
    let anchor: string | null = null;

    function pick(row: Row, at: number, event: MouseEvent): void {
        if (!picked) return;
        const on = !picked.has(row.id);
        if (event.shiftKey && anchor !== null) {
            const from = sorted.findIndex((r) => r.id === anchor);
            if (from !== -1) {
                const [a, b] = from < at ? [from, at] : [at, from];
                onpick?.(sorted.slice(a, b + 1), on);
                return;
            }
        }
        anchor = row.id;
        onpick?.([row], on);
    }

    /**
     * A click on the row opens it. With ⌘ or Ctrl held it ticks it instead,
     * and with Shift it sweeps: the gestures every file list has taught, so
     * the checkboxes are the visible way and not the only one.
     */
    function press(row: Row, at: number, event: MouseEvent): void {
        if (picked && (event.metaKey || event.ctrlKey || event.shiftKey)) {
            pick(row, at, event);
            return;
        }
        onselect?.(row);
    }

    let allPicked = $derived.by(() => {
        const set = picked;
        return !!set && sorted.length > 0 && sorted.every((r) => set.has(r.id));
    });
    let somePicked = $derived.by(() => {
        const set = picked;
        return !!set && !allPicked && sorted.some((r) => set.has(r.id));
    });

    /** The header checkbox: everything on screen, or nothing. */
    function pickAll(): void {
        onpick?.(sorted, !allPicked);
    }

    /** Puts a checkbox in its third state, which is a property and not an attribute. */
    function indeterminate(node: HTMLInputElement, value: boolean) {
        node.indeterminate = value;
        return {
            update(next: boolean) {
                node.indeterminate = next;
            },
        };
    }

    /**
     * What a cell sorts by: its sort key where it has one, its text otherwise.
     * An age reads "3d" and sorts by seconds; a volume reads "500Mi" and sorts
     * by bytes. Comparing the text would order them as words.
     */
    function sortKey(value: { text: string; sort: string } | undefined): string {
        return value?.sort || value?.text || '';
    }

    let sorted = $derived.by(() => {
        if (sortColumn === null) return rows;

        const column = Math.min(sortColumn, Math.max(0, (columns.length || 1) - 1));
        const out = [...rows].sort((a, b) =>
            sortKey(a.cells[column]).localeCompare(sortKey(b.cells[column]), undefined, {
                numeric: true,
                sensitivity: 'base',
            }),
        );
        return sortDescending ? out.reverse() : out;
    });

    function sortBy(index: number): void {
        if (sortColumn === index) {
            sortDescending = !sortDescending;
        } else {
            sortColumn = index;
            sortDescending = false;
        }
    }
</script>

<table>
    <thead>
        <tr>
            {#if picked}
                <th class="pick">
                    <input
                        type="checkbox"
                        checked={allPicked}
                        use:indeterminate={somePicked}
                        onchange={pickAll}
                        aria-label="Select all"
                        title={allPicked ? 'Clear the selection' : 'Select every row shown'}
                    />
                </th>
            {/if}
            <!-- By position, not by name: a CRD may declare a printer column
                 called "Name" beside the Name the app puts first, and a keyed
                 block throws on the repeat where an unkeyed one renders it. -->
            {#each columns as column, index}
                <th class:sorted={sortColumn === index} aria-sort={sortColumn === index ? (sortDescending ? 'descending' : 'ascending') : 'none'}>
                    <button onclick={() => sortBy(index)}>
                        {column}
                        {#if sortColumn === index}
                            <Icon name={sortDescending ? 'chevron-down' : 'chevron-right'} size={11} />
                        {/if}
                    </button>
                </th>
            {/each}
        </tr>
    </thead>
    <tbody>
        {#each sorted as row, at (row.id)}
            <tr
                class:selected={selectedRowId === row.id}
                class:picked={picked?.has(row.id)}
                onclick={(event) => press(row, at, event)}
            >
                {#if picked}
                    <td class="pick">
                        <input
                            type="checkbox"
                            checked={picked.has(row.id)}
                            onclick={(event) => {
                                event.stopPropagation();
                                pick(row, at, event);
                            }}
                            aria-label="Select {row.name}"
                        />
                    </td>
                {/if}
                {#each row.cells as value, index (index)}
                    <td class={value.tone}>
                        {#if cell}{@render cell(row, index)}{:else}{value.text}{/if}
                    </td>
                {/each}
            </tr>
        {:else}
            <tr class="none">
                <td colspan={columns.length + (picked ? 1 : 0)}>{empty}</td>
            </tr>
        {/each}
    </tbody>
</table>

<style>
    table {
        width: 100%;
        border-collapse: collapse;
        font-size: 12px;
    }

    thead th {
        position: sticky;
        top: 0;
        z-index: 1;
        background: var(--bg);
        border-bottom: 1px solid var(--border);
        text-align: left;
        font-weight: 500;
        padding: 0;
        white-space: nowrap;
    }

    thead button {
        display: flex;
        align-items: center;
        gap: 4px;
        width: 100%;
        padding: 7px 12px;
        color: var(--text-faint);
        font-size: 11px;
        letter-spacing: 0.03em;
    }

    thead button:hover {
        color: var(--text);
    }

    th.sorted button {
        color: var(--text);
    }

    tbody tr {
        cursor: default;
        border-bottom: 1px solid var(--border-soft);
    }

    tbody tr:hover {
        background: var(--bg-hover);
    }

    tbody tr.selected {
        background: var(--bg-active);
    }

    /* Ticked rows read as one set: the accent, kept apart from hover so a
       sweep can still be seen while the pointer moves over it. */
    tbody tr.picked {
        background: color-mix(in srgb, var(--accent) 14%, transparent);
    }

    tbody tr.picked:hover {
        background: color-mix(in srgb, var(--accent) 22%, transparent);
    }

    th.pick,
    td.pick {
        width: 30px;
        padding: 0 0 0 12px;
        vertical-align: middle;
    }

    .pick input {
        display: block;
        margin: 0;
        accent-color: var(--accent);
        cursor: pointer;
    }

    td {
        /* The density preference, set on the root by the shell. This is what
           "compact" actually changes -- the row height follows its cells. */
        padding: var(--cell-pad-y, 6px) 12px;
        white-space: nowrap;
        overflow: hidden;
        text-overflow: ellipsis;
        max-width: 340px;
    }

    /* The tone the backend put on the cell, so status colouring is decided once. */
    td.ok {
        color: var(--ok);
    }

    td.warn {
        color: var(--warn);
    }

    td.error {
        color: var(--error);
    }

    td.info {
        color: var(--text-dim);
    }

    tr.none td {
        padding: 22px 16px;
        color: var(--text-faint);
        text-align: left;
    }

    tr.none:hover {
        background: none;
    }
</style>
