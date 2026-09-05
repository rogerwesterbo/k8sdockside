<!--
  A resource listing. Every kind renders through this one component because the
  backend returns them all in the same Table shape. Selecting a row opens the
  describe panel.

  The rows are live: the tab subscribes to a watch on the cluster and the backend
  pushes the whole table whenever anything changes, so nothing here polls or
  refreshes.
-->
<script lang="ts">
    import { untrack } from 'svelte';
    import { ResourceService } from '../../../bindings/github.com/roger/k8sdockside';
    import { type Row, type Table } from '../state/adopt';
    import { subscribe, type Subscription } from '../state/subscriptions';
    import { customKindFor, labelFor } from '../catalogue';
    import ContainerPills from './ContainerPills.svelte';
    import SortableTable from './SortableTable.svelte';
    import { alpha } from '../colors';
    import { workspace } from '../state/workspace.svelte';
    import ErrorState from './ErrorState.svelte';
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
    let loading = $state(true);
    let error = $state<string | null>(null);
    /** Bumped by the retry button; the subscribing effect reads it as a dependency. */
    let attempt = $state(0);

    /**
     * The namespace a solution plugin's view fixes, or "" when the tab is free
     * to filter for itself.
     *
     * A pinned view is answering a narrower question than "what is in this
     * cluster" -- "Argo CD's own workloads" is not a question about kube-system
     * -- so the picker is replaced with a statement of where you are. The pin is
     * applied by the backend either way; offering a control that the backend
     * would then override is the one thing that would be worse than not
     * offering it.
     */
    let pinned = $derived(workspace.pinnedNamespace(kind));

    let color = $derived(workspace.colorOf(contextId));
    let context = $derived(workspace.contexts.find((c) => c.id === contextId) ?? null);
    let selectedRowId = $derived(
        workspace.detailTarget
            ? `${workspace.detailTarget.kind}/${workspace.detailTarget.namespace}/${workspace.detailTarget.name}`
            : null,
    );

    let subscription: Subscription | null = null;

    // Open a watch for the tab's identity. The namespace is deliberately not a
    // dependency: the watch is cluster-wide and the filter is applied to its
    // cache, so changing it must not tear the watch down and start again.
    $effect(() => {
        const [id, k] = [contextId, kind];
        attempt;
        loading = true;
        error = null;
        table = null;

        // Read untracked: naming it as a dependency would reopen the watch on
        // every filter change, which is exactly what SetNamespace exists to
        // avoid. The effect below moves the filter on the open subscription.
        const sub = subscribe(
            id,
            k,
            untrack(() => namespace),
            (result) => {
                table = result;
                loading = false;
                // Rows arriving is proof the cluster is reachable, so the
                // sidebar indicator does not need its own request.
                workspace.reportHealth(id, 'connected');
            },
            (message) => {
                error = message;
                loading = false;
                workspace.reportHealth(id, 'error', message);
            },
        );
        subscription = sub;

        return () => {
            subscription = null;
            sub.close();
        };
    });

    // Move the filter on the open subscription. The next snapshot arrives with
    // the new namespace applied.
    $effect(() => {
        const ns = namespace;
        subscription?.setNamespace(ns);
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

    // Filtering only: the ordering is SortableTable's, and the backend has
    // already put the rows in this kind's natural order.
    let rows = $derived.by(() => {
        if (!table) return [];
        const needle = query.trim().toLowerCase();
        if (!needle) return table.rows;
        return table.rows.filter((row) => row.cells.some((cell) => cell.text.toLowerCase().includes(needle)));
    });

    function select(row: Row): void {
        workspace.openDetail({ contextId, kind, namespace: row.namespace, name: row.name });
    }

    // A CustomResourceDefinition is the one row that leads somewhere: its name
    // opens a tab listing the objects of that kind. A definition is named
    // "<plural>.<group>", which is exactly the kind string such a tab wants.
    let drillable = $derived(kind === 'customresourcedefinitions');

    function openInstances(row: Row, event: MouseEvent): void {
        event.stopPropagation();
        workspace.openTab(contextId, customKindFor(row.name));
    }
</script>

<!-- Most cells are their text. Two are not: a CustomResourceDefinition's name
     opens a tab of its objects, and a cell carrying containers is drawn as
     rectangles. The rectangles stay pictures here -- a press anywhere in the
     row, this column included, selects the row. -->
{#snippet bodyCell(row: Row, index: number)}
    {@const value = row.cells[index]}
    {#if drillable && index === 0}
        <button class="drill" onclick={(event) => openInstances(row, event)} title="List the {row.name} in this cluster">
            {row.cells[0]?.text}
        </button>
    {:else if value?.pills?.length}
        <ContainerPills pills={value.pills} />
    {:else}
        {value?.text ?? ''}
    {/if}
{/snippet}

<div class="view" style:--ctx-tint={alpha(color, 0.1)}>
    <div class="toolbar">
        {#if pinned}
            <span class="ns pinned" title="This view is fixed to the {pinned} namespace by the plugin that provides it">
                <Icon name="pin" size={12} />
                <span>{pinned}</span>
            </span>
        {:else if table?.namespaced}
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
            <ErrorState message={error} {context} onRetry={() => attempt++} />
        {:else if table?.error}
            <ErrorState message={table.error} {context} onRetry={() => attempt++} />
        {:else if loading && !table}
            <p class="status">Loading {labelFor(kind).toLowerCase()}…</p>
        {:else if table}
            <SortableTable
                columns={table.columns}
                {rows}
                {selectedRowId}
                onselect={select}
                empty={query.trim() ? `Nothing matches “${query}”.` : `No ${labelFor(kind).toLowerCase()} here.`}
                cell={bodyCell}
            />
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

    /* Where the picker would be, so the toolbar keeps its shape, but plainly a
       statement rather than a control. */
    .ns.pinned {
        gap: 5px;
        padding: 3px 9px;
        border-radius: var(--radius-sm);
        background: var(--bg-raised);
        box-shadow: inset 0 0 0 1px var(--border-soft);
        font-family: var(--mono);
        color: var(--text-faint);
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










    /* A name that opens a tab of its own, rather than just the describe panel. */
    .drill {
        font: inherit;
        color: var(--accent);
        text-align: left;
        max-width: 100%;
        overflow: hidden;
        text-overflow: ellipsis;
    }

    .drill:hover {
        text-decoration: underline;
    }






</style>
