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
    import { SvelteSet } from 'svelte/reactivity';
    import { ResourceService } from '../../../bindings/github.com/rogerwesterbo/k8sdockside';
    import { type Row, type Table } from '../state/adopt';
    import { subscribe, type Subscription } from '../state/subscriptions';
    import { actions, type BulkReport } from '../state/actions.svelte';
    import { actionsFor } from '../actions';
    import { customKindFor, labelFor, singularFor } from '../catalogue';
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

    // ----- the selection ---------------------------------------------------

    /** The kind the rows are, which for a plugin view is not the tab's. */
    let listed = $derived(workspace.listedKind(kind));
    /**
     * Whether rows can be ticked. Only where a selection has something to do
     * -- a delete -- so a listing whose rows cannot be deleted from here does
     * not grow a column of checkboxes that lead nowhere.
     */
    let pickable = $derived(actionsFor(listed).some((a) => a.id === 'delete'));

    /** The ticked rows, by id. */
    const picked = new SvelteSet<string>();
    /** Whether the bulk delete is asking, and whether it is under way. */
    let askingDelete = $state(false);
    let deleting = $state(false);
    /** The question's safe button, focused so a stray Enter cannot destroy. */
    let cancelEl = $state<HTMLButtonElement | null>(null);

    // A different table is a different selection.
    $effect(() => {
        void [contextId, kind];
        picked.clear();
        askingDelete = false;
    });

    // Rows that leave the cluster leave the selection, or "3 selected" would
    // go on counting what is no longer there.
    $effect(() => {
        if (!table) return;
        const present = new Set(table.rows.map((row) => row.id));
        for (const id of untrack(() => [...picked])) {
            if (!present.has(id)) picked.delete(id);
        }
    });

    // Focus the safe answer as soon as the question appears.
    $effect(() => {
        if (askingDelete) cancelEl?.focus();
    });

    function pick(chosen: Row[], on: boolean): void {
        for (const row of chosen) {
            if (on) picked.add(row.id);
            else picked.delete(row.id);
        }
        if (picked.size === 0) askingDelete = false;
    }

    let pickedRows = $derived(table ? table.rows.filter((row) => picked.has(row.id)) : []);

    /** "pod" or "pods", after a count. */
    function nounFor(count: number): string {
        return (count === 1 ? singularFor(listed) : labelFor(listed)).toLowerCase();
    }

    /**
     * What the question names besides the count: every row when there are
     * few. Reading three names is a check; reading thirty is not.
     */
    let named = $derived(pickedRows.length <= 3 ? pickedRows.map((row) => row.name).join(', ') : '');

    /**
     * Deletes the ticked rows in one call.
     *
     * What went is unticked. What the cluster refused stays ticked, with the
     * reason in the notice, so it can be looked at or tried again; the panel
     * describing something that has just gone is closed, as after a single
     * delete.
     */
    async function deletePicked(): Promise<void> {
        const chosen = pickedRows;
        if (chosen.length === 0 || deleting) return;
        deleting = true;
        try {
            const report = await actions.removeMany(
                contextId,
                listed,
                chosen.map((row) => ({ namespace: row.namespace, name: row.name })),
            );
            const refused = new Set(report.failures.map((f) => `${f.namespace}/${f.name}`));
            for (const row of chosen) {
                if (!refused.has(`${row.namespace}/${row.name}`)) picked.delete(row.id);
            }

            const shown = workspace.detailTarget;
            if (
                shown &&
                shown.contextId === contextId &&
                shown.kind === listed &&
                !refused.has(`${shown.namespace}/${shown.name}`) &&
                chosen.some((row) => row.namespace === shown.namespace && row.name === shown.name)
            ) {
                workspace.closeDetail();
            }

            if (report.failures.length === 0) {
                workspace.inform(`${report.deleted} ${nounFor(report.deleted)} deleted`);
            } else {
                workspace.fail(refusals(report, chosen.length));
            }
        } catch (err) {
            workspace.fail(err instanceof Error ? err.message : String(err));
        } finally {
            askingDelete = false;
            deleting = false;
        }
    }

    /** How a partly refused delete reads: the tally, then the first refusal in the API server's words. */
    function refusals(report: BulkReport, asked: number): string {
        const [first, ...rest] = report.failures;
        let text = `${report.deleted} of ${asked} deleted. ${first.name}: ${first.error}`;
        if (rest.length > 0) text += `, and ${rest.length} more refused`;
        return text;
    }

    // ----- the watch ---------------------------------------------------------

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

    // Opened as what the row is, not as the tab's kind: a plugin view's rows
    // are the kind the view lists.
    function select(row: Row): void {
        workspace.openDetail({ contextId, kind: workspace.listedKind(kind), namespace: row.namespace, name: row.name });
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

<svelte:document onkeydown={(e) => e.key === 'Escape' && askingDelete && (askingDelete = false)} />

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

    {#if pickable && picked.size > 0}
        <!-- The selection's own bar, under the toolbar rather than in it, so
             the namespace picker and the filter stay where they were. The
             question replaces the buttons in the same place, the way the
             action bar's does: "Delete 3 pods?" is read where Delete was
             pressed and cannot be mistaken for a question about something
             else. -->
        <div class="selection" role="region" aria-label="Selected rows">
            {#if askingDelete}
                <p class="question">
                    Delete {pickedRows.length} {nounFor(pickedRows.length)}?
                    {#if named}<span class="names">{named}</span>{/if}
                </p>
                <div class="answers">
                    <button bind:this={cancelEl} class="plain" onclick={() => (askingDelete = false)}>Cancel</button>
                    <button class="danger" disabled={deleting} onclick={deletePicked}>
                        {deleting ? 'Deleting…' : 'Delete'}
                    </button>
                </div>
            {:else}
                <span class="tally">{picked.size} selected</span>
                <button class="plain" onclick={() => picked.clear()}>Clear</button>
                <div class="answers">
                    <button class="danger" onclick={() => (askingDelete = true)}>
                        <Icon name="trash" size={13} /> Delete
                    </button>
                </div>
            {/if}
        </div>
    {/if}

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
                picked={pickable ? picked : null}
                onpick={pick}
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

    .selection {
        display: flex;
        align-items: center;
        gap: 8px;
        min-height: 36px;
        padding: 4px 16px;
        flex: 0 0 auto;
        border-bottom: 1px solid var(--border);
        background: color-mix(in srgb, var(--accent) 8%, var(--bg));
        font-size: 12px;
    }

    .tally {
        color: var(--text);
        font-variant-numeric: tabular-nums;
    }

    .selection button {
        display: flex;
        align-items: center;
        gap: 6px;
        height: 24px;
        padding: 0 10px;
        border-radius: var(--radius-sm);
        background: var(--bg-raised);
        box-shadow: inset 0 0 0 1px var(--border);
        font-size: 12px;
        color: var(--text);
        white-space: nowrap;
    }

    .selection button:hover:not(:disabled) {
        background: var(--bg-hover);
    }

    .selection button:disabled {
        opacity: 0.5;
    }

    .selection .plain {
        background: none;
        box-shadow: none;
        color: var(--text-dim);
    }

    /* The one that cannot be undone sits at the far end and is coloured apart,
       exactly as it is in the action bar. */
    .selection .danger {
        color: var(--error);
    }

    .selection .danger:hover:not(:disabled) {
        background: color-mix(in srgb, var(--error) 16%, transparent);
    }

    .question {
        margin: 0;
        color: var(--text);
        min-width: 0;
        overflow-wrap: anywhere;
    }

    .names {
        margin-left: 6px;
        font-family: var(--mono);
        color: var(--text-dim);
    }

    .answers {
        display: flex;
        gap: 6px;
        margin-left: auto;
        flex: 0 0 auto;
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
