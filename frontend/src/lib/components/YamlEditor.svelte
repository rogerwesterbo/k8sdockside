<!--
  The YAML editor: one object, as the cluster has it, editable and saved back.

  CodeMirror rather than a textarea. A textarea holds one flat string, which
  makes two things impossible rather than merely absent: finding text with the
  matches marked, and folding a block away. Both are what you want in a document
  that is mostly nesting you are not reading. What the swap costs is a
  dependency; what it buys is search, folding, YAML highlighting and indent
  handling that someone else maintains.

  The document itself lives in the editors store, not here: this component is
  destroyed and rebuilt every time you switch dock tabs, and a half-finished
  edit must not go with it.
-->
<script lang="ts">
    import { EditorView } from '@codemirror/view';
    import { untrack } from 'svelte';
    import { singularFor } from '../catalogue';
    import { alpha } from '../colors';
    import { extensions, numbering, setBadLine } from '../editor/setup';
    import { lineNumbers } from '@codemirror/view';
    import { editors } from '../state/editor.svelte';
    import { workspace, type DockTab } from '../state/workspace.svelte';
    import ErrorState from './ErrorState.svelte';
    import Icon from './Icon.svelte';

    interface Props {
        tab: DockTab;
    }

    let { tab }: Props = $props();

    let doc = $derived(editors.doc(tab.id));
    let dirty = $derived(editors.isDirty(tab.id));
    let color = $derived(workspace.colorOf(tab.contextId));
    let context = $derived(workspace.contexts.find((c) => c.id === tab.contextId) ?? null);
    let canSave = $derived(doc.status === 'ready' && dirty && doc.check.valid && !doc.saving);

    let host = $state<HTMLElement | null>(null);
    let view: EditorView | null = null;
    /**
     * True while the store's text is being written into the editor, so the
     * update listener does not send it straight back and start a loop.
     */
    let applying = false;

    // Reads the object the first time this tab is opened. A document already in
    // the store is left exactly as it was, which is what makes switching away
    // from a half-finished edit safe.
    $effect(() => {
        void editors.load(tab.id, tab);
    });

    // Builds the editor once, on the element it lives in. Its starting text is
    // read untracked: this must not rebuild on every keystroke.
    $effect(() => {
        const parent = host;
        if (!parent) return;

        view = new EditorView({
            parent,
            doc: untrack(() => editors.doc(tab.id).text),
            extensions: [
                ...extensions({
                    numbers: untrack(() => workspace.showLineNumbers),
                    label: `${tab.name} as YAML`,
                    onSave: () => void save(),
                }),
                EditorView.updateListener.of((update) => {
                    if (update.docChanged && !applying) {
                        editors.edit(tab.id, update.state.doc.toString());
                    }
                }),
            ],
        });

        return () => {
            view?.destroy();
            view = null;
        };
    });

    // The store's text, written in when it changes for a reason other than
    // typing: the first read, a reload, and what the API server gave back after
    // a save. Compared first, so an echo of the user's own keystroke is a
    // no-op rather than a cursor thrown back to the start.
    $effect(() => {
        const text = doc.text;
        if (!view || view.state.doc.toString() === text) return;
        applying = true;
        view.dispatch({ changes: { from: 0, to: view.state.doc.length, insert: text } });
        applying = false;
    });

    // The line the parser stopped at, handed to the editor to mark.
    $effect(() => {
        const line = doc.check.valid ? 0 : doc.check.line;
        view?.dispatch({ effects: setBadLine.of(line) });
    });

    // Line numbering follows the setting without the editor being rebuilt.
    $effect(() => {
        const on = workspace.showLineNumbers;
        view?.dispatch({ effects: numbering.reconfigure(on ? lineNumbers() : []) });
    });

    async function save(): Promise<void> {
        if (!canSave) return;
        if (await editors.save(tab.id, tab)) {
            workspace.inform(`${singularFor(tab.kind)} ${tab.name} saved`);
        }
    }

    /** Re-reads the object, throwing away whatever was typed against it. */
    function reload(): void {
        if (dirty && !confirm(`Discard your changes to ${tab.name} and re-read it from the cluster?`)) {
            return;
        }
        void editors.load(tab.id, tab, { force: true });
    }
</script>

<div class="editor" style:--ctx-color={color} style:--ctx-tint={alpha(color, 0.1)}>
    <header>
        <div class="ident">
            <span class="kind">{singularFor(tab.kind)}</span>
            <span class="name selectable">{tab.name}</span>
            {#if tab.namespace}<span class="dim">in {tab.namespace}</span>{/if}
            {#if context}<span class="dim">· {workspace.displayName(context)}</span>{/if}
        </div>

        <div class="status" role="status">
            {#if doc.saving}
                <span class="dim">Saving…</span>
            {:else if !doc.check.valid}
                <span class="bad">
                    <Icon name="alert" size={12} />
                    {doc.check.line > 0 ? `Line ${doc.check.line}: ` : ''}{doc.check.message}
                </span>
            {:else if dirty}
                <span class="dim">Unsaved changes</span>
            {:else if doc.saved}
                <span class="good">Saved</span>
            {/if}
        </div>

        <div class="actions">
            <button class="ghost" onclick={reload} disabled={doc.status === 'loading'} title="Re-read this object from the cluster">
                <Icon name="refresh" size={13} />
                Reload
            </button>
            <button
                class="save"
                onclick={save}
                disabled={!canSave}
                title={dirty ? 'Save to the cluster (⌘S)' : 'No changes to save'}
            >
                <Icon name="save" size={13} />
                Save
            </button>
        </div>
    </header>

    {#if doc.error && doc.status === 'ready'}
        <!-- A refused save: the API server's own words, which is where the
             useful part is -- a conflict, an admission webhook, an RBAC rule. -->
        <p class="refused selectable">
            <Icon name="alert" size={13} />
            <span>{doc.error}</span>
        </p>
    {/if}

    {#if doc.status === 'loading'}
        <p class="loading">Reading {tab.name}…</p>
    {:else if doc.status === 'error'}
        <ErrorState message={doc.error} {context} onRetry={() => editors.load(tab.id, tab, { force: true })} compact />
    {:else}
        <!-- CodeMirror mounts itself in here. ⌘F searches, the fold arrows in
             the gutter collapse a block, and both are its own. -->
        <div class="pane" bind:this={host}></div>
    {/if}
</div>

<style>
    .editor {
        display: flex;
        flex-direction: column;
        height: 100%;
        min-height: 0;
        background: var(--bg-panel);
    }

    header {
        display: flex;
        align-items: center;
        gap: 14px;
        height: 34px;
        padding: 0 12px;
        flex: 0 0 auto;
        background: var(--ctx-tint);
        border-bottom: 1px solid var(--border);
        border-left: 3px solid var(--ctx-color);
    }

    .ident {
        display: flex;
        align-items: baseline;
        gap: 7px;
        min-width: 0;
        overflow: hidden;
        white-space: nowrap;
    }

    .kind {
        font-size: 10px;
        letter-spacing: 0.08em;
        text-transform: uppercase;
        color: var(--text-faint);
        flex: 0 0 auto;
    }

    .name {
        font-size: 13px;
        font-weight: 600;
        overflow: hidden;
        text-overflow: ellipsis;
    }

    .dim {
        font-size: 11px;
        color: var(--text-dim);
        overflow: hidden;
        text-overflow: ellipsis;
    }

    /* Between the identity and the buttons, so the eye passes over it on the
       way to Save rather than having to go looking. */
    .status {
        display: flex;
        align-items: center;
        gap: 6px;
        margin-left: auto;
        min-width: 0;
        font-size: 11.5px;
    }

    .status .bad {
        display: flex;
        align-items: center;
        gap: 5px;
        color: var(--error);
        overflow: hidden;
        text-overflow: ellipsis;
        white-space: nowrap;
    }

    .status .good {
        color: var(--ok);
    }

    .actions {
        display: flex;
        align-items: center;
        gap: 6px;
        flex: 0 0 auto;
    }

    .actions button {
        display: flex;
        align-items: center;
        gap: 6px;
        height: 24px;
        padding: 0 10px;
        border-radius: var(--radius-sm);
        font-size: 12px;
        color: var(--text-dim);
    }

    .ghost:hover:not(:disabled) {
        background: var(--bg-hover);
        color: var(--text);
    }

    .save {
        background: var(--accent);
        color: var(--accent-text);
    }

    .save:hover:not(:disabled) {
        filter: brightness(1.1);
    }

    .actions button:disabled {
        opacity: 0.45;
        cursor: default;
    }

    /* A disabled Save keeps its shape but gives up the accent: white on a
       faded blue is unreadable rather than merely quiet, and "nothing to save"
       should look like a resting control, not a broken one. */
    .save:disabled {
        background: var(--bg-raised);
        box-shadow: inset 0 0 0 1px var(--border);
        color: var(--text-faint);
        opacity: 1;
    }

    .refused {
        display: flex;
        align-items: flex-start;
        gap: 8px;
        margin: 0;
        padding: 8px 12px;
        flex: 0 0 auto;
        background: color-mix(in srgb, var(--error) 14%, transparent);
        color: var(--error);
        font-size: 12px;
        line-height: 1.5;
        max-height: 5.5em;
        overflow-y: auto;
    }

    .loading {
        margin: 0;
        padding: 18px 16px;
        color: var(--text-dim);
    }

    /* CodeMirror styles itself through the theme in ../editor/setup.ts, in
       the app's own tokens. All this has to do is give it a box to fill. */
    .pane {
        flex: 1 1 auto;
        min-height: 0;
        min-width: 0;
        overflow: hidden;
    }
</style>
