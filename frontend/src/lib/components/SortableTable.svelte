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
    }

    let { columns, rows, selectedRowId = null, onselect, empty = 'Nothing here.', cell }: Props = $props();

    // Null means "the order the caller gave", which the backend has already put
    // in each kind's natural order -- events most recent first, everything else
    // by namespace and name. Sorting before the user asks would undo that.
    let sortColumn = $state<number | null>(null);
    let sortDescending = $state(false);

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
            {#each columns as column, index (column)}
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
        {#each sorted as row (row.id)}
            <tr class:selected={selectedRowId === row.id} onclick={() => onselect?.(row)}>
                {#each row.cells as value, index (index)}
                    <td class={value.tone}>
                        {#if cell}{@render cell(row, index)}{:else}{value.text}{/if}
                    </td>
                {/each}
            </tr>
        {:else}
            <tr class="none">
                <td colspan={columns.length}>{empty}</td>
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
