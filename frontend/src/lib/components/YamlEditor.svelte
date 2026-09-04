<!--
  The YAML editor: one object, as the cluster has it, editable and saved back.

  It is a textarea with a gutter beside it rather than a code editor component,
  and that is a deliberate limit. What this has to do is show a document, count
  its lines, say where it stopped being YAML and send it back -- none of which
  needs a syntax highlighter, and all of which a textarea does natively,
  including selection, undo, find-as-you-type and the platform's own key
  bindings.

  The document itself lives in the editors store, not here: this component is
  destroyed and rebuilt every time you switch dock tabs, and a half-finished
  edit must not go with it.
-->
<script lang="ts">
    import { tick } from 'svelte';
    import { singularFor } from '../catalogue';
    import { alpha } from '../colors';
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
    /** The line numbers the gutter draws, one per line of the document. */
    let lineNumbers = $derived.by(() => {
        const count = doc.text.split('\n').length;
        return Array.from({ length: count }, (_, i) => i + 1);
    });
    let canSave = $derived(doc.status === 'ready' && dirty && doc.check.valid && !doc.saving);

    let textarea = $state<HTMLTextAreaElement | null>(null);
    let gutter = $state<HTMLElement | null>(null);

    // Reads the object the first time this tab is opened. A document already in
    // the store is left exactly as it was, which is what makes switching away
    // from a half-finished edit safe.
    $effect(() => {
        void editors.load(tab.id, tab);
    });

    /** Keeps the gutter level with the text it numbers. */
    function syncGutter(): void {
        if (gutter && textarea) gutter.scrollTop = textarea.scrollTop;
    }

    function onInput(event: Event): void {
        editors.edit(tab.id, (event.currentTarget as HTMLTextAreaElement).value);
    }

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

    async function onKeyDown(event: KeyboardEvent): Promise<void> {
        const el = event.currentTarget as HTMLTextAreaElement;

        if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === 's') {
            event.preventDefault();
            await save();
            return;
        }

        // Escape leaves the editor rather than reaching the window, where it
        // would close the describe panel instead -- and it is the way out for
        // anyone navigating by keyboard, since Tab is taken below.
        if (event.key === 'Escape') {
            event.stopPropagation();
            el.blur();
            return;
        }

        // Tab indents. In a YAML editor that is the whole job of the key, and
        // two spaces is what the documents this opens are written in.
        if (event.key === 'Tab' && !event.altKey && !event.ctrlKey && !event.metaKey) {
            event.preventDefault();
            const { selectionStart: start, selectionEnd: end, value } = el;
            editors.edit(tab.id, `${value.slice(0, start)}  ${value.slice(end)}`);
            // The value goes through the store, so the caret can only be put
            // back once Svelte has written it out again.
            await tick();
            el.selectionStart = start + 2;
            el.selectionEnd = start + 2;
        }
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
        <div class="pane">
            {#if workspace.showLineNumbers}
                <!-- Not a list to a screen reader: these are the same lines the
                     textarea already carries, counted for the eye alone. -->
                <div class="gutter" bind:this={gutter} aria-hidden="true">
                    {#each lineNumbers as n (n)}
                        <span class:bad={doc.check.line === n}>{n}</span>
                    {/each}
                </div>
            {/if}

            <textarea
                bind:this={textarea}
                value={doc.text}
                oninput={onInput}
                onkeydown={onKeyDown}
                onscroll={syncGutter}
                spellcheck="false"
                autocapitalize="off"
                autocomplete="off"
                wrap="off"
                aria-label="{tab.name} as YAML"
            ></textarea>
        </div>
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
        color: #fff;
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

    .pane {
        display: flex;
        flex: 1 1 auto;
        min-height: 0;
        min-width: 0;
        /* Every line-dependent measurement below has to agree between the
           gutter and the textarea, so they are declared once here and
           inherited by both. */
        font-family: var(--mono);
        font-size: 12px;
        line-height: 1.55;
    }

    .gutter {
        display: flex;
        flex-direction: column;
        flex: 0 0 auto;
        padding: 10px 8px 10px 12px;
        text-align: right;
        color: var(--text-faint);
        background: var(--bg);
        border-right: 1px solid var(--border-soft);
        overflow: hidden;
        user-select: none;
        font-variant-numeric: tabular-nums;
    }

    .gutter span {
        /* Fixed rather than inherited from line-height alone: a browser that
           rounds the two differently would drift a pixel a line, and by line
           three hundred the numbers would name the wrong rows. */
        height: calc(12px * 1.55);
        flex: 0 0 auto;
    }

    .gutter span.bad {
        color: var(--error);
        font-weight: 600;
    }

    textarea {
        flex: 1 1 auto;
        min-width: 0;
        padding: 10px 14px;
        border: 0;
        outline: none;
        resize: none;
        background: transparent;
        color: var(--text);
        font: inherit;
        line-height: inherit;
        tab-size: 2;
        white-space: pre;
        overflow: auto;
    }
</style>
