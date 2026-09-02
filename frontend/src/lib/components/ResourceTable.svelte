<!--
  A resource listing. Every kind renders through this one component because the
  backend returns them all in the same Table shape. Selecting a row opens the
  describe panel.
-->
<script lang="ts">
    import { ResourceService } from '../../../bindings/github.com/roger/k8sdockside';
    import { adoptTable, type Row, type Table } from '../state/adopt';
    import { labelFor } from '../catalogue';
    import { alpha } from '../colors';
    import { workspace } from '../state/workspace.svelte';
    import Icon from './Icon.svelte';

    interface Props {
        contextId: string;
        kind: string;
    }

    let { contextId, kind }: Props = $props();

    let table = $state<Table | null>(null);
    let namespaces = $state<string[]>([]);
    let namespace = $state('');
    let query = $state('');
    let sortColumn = $state(0);
    let sortDescending = $state(false);
    let loading = $state(true);
    let error = $state<string | null>(null);

    let color = $derived(workspace.colorOf(contextId));
    let selectedRowId = $derived(
        workspace.detailTarget
            ? `${workspace.detailTarget.kind}/${workspace.detailTarget.namespace}/${workspace.detailTarget.name}`
            : null,
    );

    // Reload whenever the tab's identity or the namespace filter changes. The
    // cancelled flag stops a slow response from overwriting a newer one.
    $effect(() => {
        const [id, k, ns] = [contextId, kind, namespace];
        let cancelled = false;
        loading = true;
        error = null;

        ResourceService.Table(id, k, ns)
            .then((result) => {
                if (!cancelled) table = adoptTable(result);
            })
            .catch((err: unknown) => {
                if (!cancelled) error = err instanceof Error ? err.message : String(err);
            })
            .finally(() => {
                if (!cancelled) loading = false;
            });

        return () => {
            cancelled = true;
        };
    });

    // The namespace list belongs to the context, not the kind, so it is fetched
    // separately and survives switching between resource tabs.
    $effect(() => {
        const id = contextId;
        let cancelled = false;
        ResourceService.Namespaces(id)
            .then((result) => {
                if (!cancelled) namespaces = result ?? [];
            })
            .catch(() => {
                if (!cancelled) namespaces = [];
            });
        return () => {
            cancelled = true;
        };
    });

    let rows = $derived.by(() => {
        if (!table) return [];
        const needle = query.trim().toLowerCase();
        const filtered = needle
            ? table.rows.filter((row) => row.cells.some((cell) => cell.text.toLowerCase().includes(needle)))
            : table.rows;

        const column = Math.min(sortColumn, Math.max(0, (table.columns.length || 1) - 1));
        const sorted = [...filtered].sort((a, b) =>
            (a.cells[column]?.text ?? '').localeCompare(b.cells[column]?.text ?? '', undefined, {
                numeric: true,
                sensitivity: 'base',
            }),
        );
        return sortDescending ? sorted.reverse() : sorted;
    });

    function sortBy(index: number): void {
        if (sortColumn === index) {
            sortDescending = !sortDescending;
        } else {
            sortColumn = index;
            sortDescending = false;
        }
    }

    function select(row: Row): void {
        workspace.openDetail({ contextId, kind, namespace: row.namespace, name: row.name });
    }
</script>

<div class="view" style:--ctx-tint={alpha(color, 0.1)}>
    <div class="toolbar">
        {#if table?.namespaced}
            <label class="ns">
                <span>Namespace</span>
                <select bind:value={namespace}>
                    <option value="">All namespaces</option>
                    {#each namespaces as ns (ns)}
                        <option value={ns}>{ns}</option>
                    {/each}
                </select>
            </label>
        {/if}

        <div class="search">
            <Icon name="search" size={13} />
            <input type="search" bind:value={query} placeholder="Filter {labelFor(kind).toLowerCase()}" spellcheck="false" />
        </div>

        <span class="count">{rows.length}{table && rows.length !== table.rows.length ? ` of ${table.rows.length}` : ''}</span>
    </div>

    <div class="scroll">
        {#if error}
            <p class="status error"><Icon name="alert" size={14} /> {error}</p>
        {:else if table?.error}
            <p class="status error"><Icon name="alert" size={14} /> {table.error}</p>
        {:else if loading && !table}
            <p class="status">Loading {labelFor(kind).toLowerCase()}…</p>
        {:else if table}
            <table>
                <thead>
                    <tr>
                        {#each table.columns as column, index (column)}
                            <th class:sorted={sortColumn === index}>
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
                    {#each rows as row (row.id)}
                        <tr class:selected={selectedRowId === row.id} onclick={() => select(row)}>
                            {#each row.cells as cell, index (index)}
                                <td class={cell.tone}>{cell.text}</td>
                            {/each}
                        </tr>
                    {:else}
                        <tr class="none">
                            <td colspan={table.columns.length}>
                                {query.trim() ? `Nothing matches “${query}”.` : `No ${labelFor(kind).toLowerCase()} here.`}
                            </td>
                        </tr>
                    {/each}
                </tbody>
            </table>
        {/if}
    </div>
</div>

<style>
    .view {
        display: flex;
        flex-direction: column;
        height: 100%;
        min-height: 0;
    }

    .toolbar {
        display: flex;
        align-items: center;
        gap: 14px;
        height: 38px;
        padding: 0 16px;
        flex: 0 0 auto;
        background: var(--ctx-tint);
        border-bottom: 1px solid var(--border);
    }

    .ns {
        display: flex;
        align-items: center;
        gap: 7px;
        font-size: 11px;
        color: var(--text-dim);
    }

    select {
        font: inherit;
        font-size: 12px;
        color: var(--text);
        background: var(--bg);
        border: 1px solid var(--border);
        border-radius: var(--radius-sm);
        padding: 3px 6px;
        outline: none;
        max-width: 190px;
    }

    select:focus {
        border-color: var(--accent);
    }

    .search {
        display: flex;
        align-items: center;
        gap: 6px;
        color: var(--text-faint);
        flex: 1 1 auto;
        max-width: 300px;
    }

    .search input {
        flex: 1 1 auto;
        min-width: 0;
        padding: 3px 7px;
        font-size: 12px;
    }

    .count {
        margin-left: auto;
        font-size: 11px;
        color: var(--text-faint);
        font-variant-numeric: tabular-nums;
    }

    .scroll {
        flex: 1 1 auto;
        overflow: auto;
        min-height: 0;
    }

    .status {
        display: flex;
        align-items: center;
        gap: 8px;
        padding: 22px 16px;
        color: var(--text-dim);
    }

    .status.error {
        color: var(--error);
    }

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
        padding: 6px 12px;
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
