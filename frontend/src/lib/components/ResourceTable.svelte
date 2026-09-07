<!--
  A resource listing. Every kind renders through this one component because the
  backend returns them all in the same Table shape. Selecting a row opens the
  describe panel.

  The rows are live: the tab subscribes to a watch on the cluster and the backend
  pushes the whole table whenever anything changes, so nothing here polls or
  refreshes.
-->
<script lang="ts">
    import { EditorView } from '@codemirror/view';
    import { untrack } from 'svelte';
    import { SvelteSet } from 'svelte/reactivity';
    import { ResourceService } from '../../../bindings/github.com/rogerwesterbo/k8sdockside/internal/services';
    import { type Row, type Table } from '../state/adopt';
    import { subscribe, type Subscription } from '../state/subscriptions';
    import { actions, type BulkReport, type PatchPreview } from '../state/actions.svelte';
    import { actionsFor } from '../actions';
    import { extensions, setBadLine } from '../editor/setup';
    import { buildMergePatch, type PatchMove, type PatchTarget } from '../patch';
    import { customKindFor, labelFor, singularFor } from '../catalogue';
    import SegmentedControl from './settings/SegmentedControl.svelte';
    import ContainerPills from './ContainerPills.svelte';
    import SortableTable from './SortableTable.svelte';
    import { alpha } from '../colors';
    import { workspace } from '../state/workspace.svelte';
    import ErrorState from './ErrorState.svelte';
    import Icon from './Icon.svelte';
    import NamespacePicker from './NamespacePicker.svelte';

    interface Props {
        contextId: string;
        kind: string;
    }

    let { contextId, kind }: Props = $props();

    /**
     * What the YAML mode of the patch form starts with. Comments only, so
     * pressing Apply on the template sends nothing; the example is there to
     * be uncommented rather than typed from memory.
     */
    const YAML_TEMPLATE = `# One merge patch, applied to every ticked object. A value sets a field,
# null removes it, and a list replaces the whole list. For example:
#
# metadata:
#   labels:
#     team: platform
#   annotations:
#     example.com/owner: null
# spec:
#   replicas: 2
`;

    /**
     * How long after the last keystroke the YAML is read. The reading is a
     * call into Go, as the editor's check is; a quarter of a second means a
     * typed word costs one call.
     */
    const PREVIEW_DELAY = 250;

    let table = $state<Table | null>(null);
    /** Every namespace the cluster has, for the picker. */
    let available = $state<string[]>([]);
    /** The namespaces the table is narrowed to; none means all of them. */
    let selected = $state<string[]>([]);
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
    /** Which bulk action is asking, if any, and whether one is under way. */
    let asking = $state<'delete' | 'patch' | null>(null);
    let busy = $state(false);
    /** The delete question's safe button, focused so a stray Enter cannot destroy. */
    let cancelEl = $state<HTMLButtonElement | null>(null);
    /** The patch form's first field, focused as the form opens. */
    let keyEl = $state<HTMLInputElement | null>(null);

    /**
     * What the patch form is building. Not reset with the selection: the
     * label just put on one batch is very often wanted on the next.
     *
     * Three of the modes are fields that build the patch here; the fourth is
     * a YAML editor for a patch touching several places at once, read by the
     * backend as it is typed.
     */
    let patchMode = $state<PatchTarget | 'yaml'>('label');
    let patchMove = $state<PatchMove>('set');
    let patchKey = $state('');
    let patchValue = $state('');
    let patch = $derived(
        patchMode === 'yaml'
            ? null
            : buildMergePatch({ target: patchMode, move: patchMove, key: patchKey, value: patchValue }),
    );

    let patchYaml = $state(YAML_TEMPLATE);
    let preview = $state<PatchPreview>({ json: '', empty: true, error: '', line: 0 });
    let previewTimer: ReturnType<typeof setTimeout> | null = null;
    let yamlHost = $state<HTMLElement | null>(null);
    let yamlView: EditorView | null = null;

    /** The patch as it will travel: the fields' JSON, or the YAML as typed. */
    let patchText = $derived(patchMode === 'yaml' ? patchYaml : patch ? JSON.stringify(patch) : '');
    /** The JSON each object receives, for the preview line. */
    let previewJson = $derived(patchMode === 'yaml' ? preview.json : patch ? JSON.stringify(patch) : '');
    let canApply = $derived(
        patchMode === 'yaml' ? preview.json !== '' && preview.error === '' && !preview.empty : patch !== null,
    );

    function schedulePreview(text: string): void {
        if (previewTimer) clearTimeout(previewTimer);
        previewTimer = setTimeout(() => {
            previewTimer = null;
            void refreshPreview(text);
        }, PREVIEW_DELAY);
    }

    async function refreshPreview(text: string): Promise<void> {
        try {
            const result = await actions.previewPatch(text);
            // Another keystroke landed while this was in flight; its own
            // preview follows, and this one would be stale.
            if (text !== patchYaml) return;
            preview = result;
        } catch (err) {
            preview = { json: '', empty: false, error: err instanceof Error ? err.message : String(err), line: 0 };
        }
    }

    // The YAML editor: built when its mode is chosen, on the element it lives
    // in, and dropped when the mode is left. Its text outlives it, in
    // patchYaml, so switching modes and back loses nothing.
    $effect(() => {
        const parent = yamlHost;
        if (!parent) return;

        const view = new EditorView({
            parent,
            doc: untrack(() => patchYaml),
            extensions: [
                ...extensions({ numbers: false, label: 'Merge patch as YAML', onSave: () => void applyPatch() }),
                EditorView.updateListener.of((update) => {
                    if (update.docChanged) {
                        patchYaml = update.state.doc.toString();
                        schedulePreview(patchYaml);
                    }
                }),
            ],
        });
        yamlView = view;
        view.focus();
        // Whatever was in it when the mode was last left is read again now.
        void refreshPreview(untrack(() => patchYaml));

        return () => {
            if (previewTimer) clearTimeout(previewTimer);
            previewTimer = null;
            view.destroy();
            yamlView = null;
        };
    });

    // The line the parser stopped at, marked in the editor.
    $effect(() => {
        const line = preview.error ? preview.line : 0;
        yamlView?.dispatch({ effects: setBadLine.of(line) });
    });

    // A different table is a different selection.
    $effect(() => {
        void [contextId, kind];
        picked.clear();
        asking = null;
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

    // Focus follows the question: the safe answer for a delete, the first
    // field for a patch.
    $effect(() => {
        if (asking === 'delete') cancelEl?.focus();
        if (asking === 'patch') keyEl?.focus();
    });

    function pick(chosen: Row[], on: boolean): void {
        for (const row of chosen) {
            if (on) picked.add(row.id);
            else picked.delete(row.id);
        }
        if (picked.size === 0) asking = null;
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
        if (chosen.length === 0 || busy) return;
        busy = true;
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
                workspace.inform(`${report.done} ${nounFor(report.done)} deleted`);
            } else {
                workspace.fail(refusals(report, chosen.length, 'deleted'));
            }
        } catch (err) {
            workspace.fail(err instanceof Error ? err.message : String(err));
        } finally {
            asking = null;
            busy = false;
        }
    }

    /**
     * Applies the patch the form built to the ticked rows in one call.
     *
     * On success the selection stays, since the next patch is very often for
     * the same rows. When some were refused, the ones that took it are
     * unticked, so what is left ticked is exactly what still needs doing, and
     * the form stays open to try again.
     */
    async function applyPatch(): Promise<void> {
        const chosen = pickedRows;
        const body = patchText;
        if (!canApply || chosen.length === 0 || busy) return;
        busy = true;
        try {
            const report = await actions.patchMany(
                contextId,
                listed,
                chosen.map((row) => ({ namespace: row.namespace, name: row.name })),
                body,
            );
            if (report.failures.length === 0) {
                workspace.inform(`${report.done} ${nounFor(report.done)} patched`);
                asking = null;
                return;
            }
            const refused = new Set(report.failures.map((f) => `${f.namespace}/${f.name}`));
            for (const row of chosen) {
                if (!refused.has(`${row.namespace}/${row.name}`)) picked.delete(row.id);
            }
            workspace.fail(refusals(report, chosen.length, 'patched'));
        } catch (err) {
            workspace.fail(err instanceof Error ? err.message : String(err));
        } finally {
            busy = false;
        }
    }

    /** How a partly refused action reads: the tally, then the first refusal in the API server's words. */
    function refusals(report: BulkReport, asked: number, verb: string): string {
        const [first, ...rest] = report.failures;
        let text = `${report.done} of ${asked} ${verb}. ${first.name}: ${first.error}`;
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
        // every filter change, which is exactly what SetNamespaces exists to
        // avoid. The effect below moves the filter on the open subscription.
        const sub = subscribe(
            id,
            k,
            untrack(() => selected),
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
    // the new namespaces applied.
    $effect(() => {
        const chosen = selected;
        subscription?.setNamespaces(chosen);
    });

    // The namespace list belongs to the context, not the kind, so it is fetched
    // separately and survives switching between resource tabs.
    $effect(() => {
        const id = contextId;
        let cancelled = false;
        ResourceService.Namespaces(id)
            .then((result) => {
                if (!cancelled) available = result ?? [];
            })
            .catch(() => {
                if (!cancelled) available = [];
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

<svelte:document onkeydown={(e) => e.key === 'Escape' && asking && (asking = null)} />

<div class="view" style:--ctx-tint={alpha(color, 0.1)}>
    <div class="toolbar">
        {#if pinned}
            <span class="ns pinned" title="This view is fixed to the {pinned} namespace by the plugin that provides it">
                <Icon name="pin" size={12} />
                <span>{pinned}</span>
            </span>
        {:else if table?.namespaced}
            <NamespacePicker namespaces={available} {selected} onchange={(next) => (selected = next)} />
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
            {#if asking === 'delete'}
                <p class="question">
                    Delete {pickedRows.length} {nounFor(pickedRows.length)}?
                    {#if named}<span class="names">{named}</span>{/if}
                </p>
                <div class="answers">
                    <button bind:this={cancelEl} class="plain" onclick={() => (asking = null)}>Cancel</button>
                    <button class="danger" disabled={busy} onclick={deletePicked}>
                        {busy ? 'Deleting…' : 'Delete'}
                    </button>
                </div>
            {:else if asking === 'patch'}
                <!-- One merge patch for every ticked row. The form names what
                     it is patching -- a label key holds dots, a field path is
                     split on them -- and shows the patch it built, so what
                     each object receives is read before it is sent. -->
                <div class="patch">
                    <div class="row">
                        <span class="tally">Patch {pickedRows.length} {nounFor(pickedRows.length)}</span>
                        <SegmentedControl
                            label="What to patch"
                            options={[
                                { value: 'label', label: 'Label' },
                                { value: 'annotation', label: 'Annotation' },
                                { value: 'field', label: 'Field' },
                                { value: 'yaml', label: 'YAML' },
                            ]}
                            value={patchMode}
                            onchange={(value) => (patchMode = value as PatchTarget | 'yaml')}
                        />
                        {#if patchMode !== 'yaml'}
                            <select bind:value={patchMove} aria-label="Set or remove">
                                <option value="set">Set</option>
                                <option value="remove">Remove</option>
                            </select>
                            <input
                                bind:this={keyEl}
                                bind:value={patchKey}
                                placeholder={patchMode === 'field' ? 'spec.replicas' : 'key'}
                                aria-label={patchMode === 'field' ? 'Field path' : 'Key'}
                                spellcheck="false"
                                onkeydown={(e) => e.key === 'Enter' && applyPatch()}
                            />
                            {#if patchMove === 'set'}
                                <input
                                    bind:value={patchValue}
                                    placeholder="value"
                                    aria-label="Value"
                                    spellcheck="false"
                                    onkeydown={(e) => e.key === 'Enter' && applyPatch()}
                                />
                            {/if}
                        {/if}
                        <div class="answers">
                            <button class="plain" onclick={() => (asking = null)}>Cancel</button>
                            <button class="go" disabled={busy || !canApply} onclick={applyPatch}>
                                {busy ? 'Patching…' : 'Apply'}
                            </button>
                        </div>
                    </div>
                    {#if patchMode === 'yaml'}
                        <!-- CodeMirror mounts itself in here, with the same
                             setup as the object editor: highlighting, folding,
                             and ⌘S -- which applies, here. -->
                        <div class="yaml" bind:this={yamlHost}></div>
                    {/if}
                    <div class="row fine">
                        {#if patchMode === 'yaml' && preview.error}
                            <span class="bad">
                                <Icon name="alert" size={12} />
                                {preview.line > 0 ? `Line ${preview.line}: ` : ''}{preview.error}
                            </span>
                        {:else}
                            <code class="preview" title="The JSON merge patch each object receives">
                                {previewJson || '…'}
                            </code>
                        {/if}
                        <span class="hint">
                            {#if patchMode === 'yaml'}
                                A merge patch, written the way the object reads: a value sets a field, null removes
                                it, and a list replaces the whole list. YAML or JSON; ⌘S applies.
                            {:else}
                                Set writes the value, Remove clears the key. A field value that parses as JSON is
                                sent as such — <code>2</code>, <code>true</code> — and anything else as text. A list
                                replaces the whole list.
                            {/if}
                        </span>
                    </div>
                </div>
            {:else}
                <span class="tally">{picked.size} selected</span>
                <button class="plain" onclick={() => picked.clear()}>Clear</button>
                <div class="answers">
                    <button onclick={() => (asking = 'patch')}>
                        <Icon name="edit" size={13} /> Patch
                    </button>
                    <button class="danger" onclick={() => (asking = 'delete')}>
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
        flex-wrap: wrap;
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

    /* The button that writes, set apart from Cancel the way the action bar's
       is: filled, so the eye lands on it once the fields are in. */
    .selection .go {
        background: var(--accent);
        color: var(--accent-text);
        box-shadow: none;
    }

    .selection .go:hover:not(:disabled) {
        background: var(--accent);
        filter: brightness(1.08);
    }

    .patch {
        display: flex;
        flex-direction: column;
        gap: 6px;
        width: 100%;
        padding: 3px 0;
    }

    .patch .row {
        display: flex;
        align-items: center;
        flex-wrap: wrap;
        gap: 8px;
    }

    .patch select,
    .patch input {
        height: 24px;
        padding: 0 8px;
        font: inherit;
        font-size: 12px;
        color: var(--text);
        background: var(--bg);
        border: 1px solid var(--border);
        border-radius: var(--radius-sm);
        outline: none;
    }

    .patch input {
        width: 170px;
        font-family: var(--mono);
    }

    .patch select:focus,
    .patch input:focus {
        border-color: var(--accent);
    }

    .fine {
        font-size: 11px;
        color: var(--text-faint);
    }

    .bad {
        display: flex;
        align-items: center;
        gap: 5px;
        color: var(--error);
        white-space: nowrap;
        overflow: hidden;
        text-overflow: ellipsis;
        max-width: 60ch;
    }

    /* A box for the editor to fill: tall enough for a patch of a few
       fields, short enough that the table is still the page. */
    .yaml {
        width: 100%;
        height: 11em;
        border: 1px solid var(--border);
        border-radius: var(--radius-sm);
        background: var(--bg-panel);
        overflow: hidden;
    }

    .yaml:focus-within {
        border-color: var(--accent);
    }

    .preview {
        font-family: var(--mono);
        color: var(--text-dim);
        white-space: nowrap;
        overflow: hidden;
        text-overflow: ellipsis;
        max-width: 48ch;
    }

    .hint {
        min-width: 0;
        line-height: 1.5;
    }

    .hint code {
        font-family: var(--mono);
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
