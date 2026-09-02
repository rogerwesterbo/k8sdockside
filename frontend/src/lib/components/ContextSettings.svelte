<!--
  The settings panel pinned to the bottom of the sidebar. It edits the selected
  context: what it is called in this app, and the colour that identifies it in
  the sidebar and on its tabs. Neither change touches the kubeconfig file.
-->
<script lang="ts">
    import type * as kube from '../../../bindings/github.com/roger/k8sdockside/internal/kube/models.js';
    import { CONTEXT_COLORS, isValidColor } from '../colors';
    import { workspace } from '../state/workspace.svelte';
    import Icon from './Icon.svelte';

    interface Props {
        context: kube.Context;
    }

    let { context }: Props = $props();

    let color = $derived(workspace.colorOf(context.id));
    let alias = $derived(workspace.settings.contexts[context.id]?.alias ?? '');
    let customised = $derived(workspace.isCustomised(context.id));

    // The typed hex field is only committed once it parses, so a half-typed
    // value like "#3d7" does not repaint the whole UI mid-keystroke.
    let hexDraft = $state('');
    let editingHex = $state(false);

    function rename(event: Event): void {
        const value = (event.currentTarget as HTMLInputElement).value;
        workspace.setContextPrefs(context.id, value, workspace.settings.contexts[context.id]?.color ?? '');
    }

    function pick(value: string): void {
        workspace.setContextPrefs(context.id, alias, value);
        editingHex = false;
    }

    function commitHex(): void {
        if (isValidColor(hexDraft)) {
            pick(hexDraft.trim().toLowerCase());
        }
        editingHex = false;
    }
</script>

<section class="settings" style:--ctx-color={color}>
    <header>
        <span class="title">Context settings</span>
        {#if customised}
            <button class="reset" onclick={() => workspace.resetContextPrefs(context.id)} title="Restore the kubeconfig name and the default colour">
                <Icon name="undo" size={13} />
                Reset
            </button>
        {/if}
    </header>

    <label class="field">
        <span>Display name</span>
        <input type="text" value={alias} placeholder={context.name} oninput={rename} spellcheck="false" />
    </label>

    <div class="field">
        <span>Colour</span>
        <div class="swatches">
            {#each CONTEXT_COLORS as option (option.value)}
                <button
                    class="swatch"
                    class:chosen={color.toLowerCase() === option.value.toLowerCase()}
                    style:background={option.value}
                    title={option.name}
                    aria-label={option.name}
                    onclick={() => pick(option.value)}
                ></button>
            {/each}

            {#if editingHex}
                <input
                    class="hex"
                    type="text"
                    bind:value={hexDraft}
                    placeholder="#4a86ff"
                    spellcheck="false"
                    onblur={commitHex}
                    onkeydown={(e) => {
                        if (e.key === 'Enter') commitHex();
                        if (e.key === 'Escape') editingHex = false;
                    }}
                />
            {:else}
                <button
                    class="swatch custom"
                    title="Enter a hex colour"
                    aria-label="Enter a hex colour"
                    onclick={() => {
                        hexDraft = color;
                        editingHex = true;
                    }}
                >
                    <Icon name="plus" size={12} />
                </button>
            {/if}
        </div>
    </div>

    <dl class="meta">
        <div><dt>Context</dt><dd class="selectable">{context.name}</dd></div>
        {#if context.cluster}<div><dt>Cluster</dt><dd class="selectable">{context.cluster}</dd></div>{/if}
        {#if context.server}<div><dt>Server</dt><dd class="selectable">{context.server}</dd></div>{/if}
        {#if context.namespace}<div><dt>Namespace</dt><dd class="selectable">{context.namespace}</dd></div>{/if}
        <div><dt>File</dt><dd class="selectable" title={context.file}>{context.file}</dd></div>
    </dl>
</section>

<style>
    .settings {
        flex: 0 0 auto;
        border-top: 1px solid var(--border);
        background: var(--bg-panel);
        padding: 10px 12px 12px;
        display: flex;
        flex-direction: column;
        gap: 10px;
        max-height: 46%;
        overflow-y: auto;
    }

    header {
        display: flex;
        align-items: center;
        justify-content: space-between;
        gap: 8px;
    }

    .title {
        font-size: 10px;
        letter-spacing: 0.08em;
        text-transform: uppercase;
        color: var(--text-faint);
    }

    .reset {
        display: flex;
        align-items: center;
        gap: 4px;
        font-size: 11px;
        color: var(--text-dim);
        padding: 2px 5px;
        border-radius: var(--radius-sm);
    }

    .reset:hover {
        background: var(--bg-hover);
        color: var(--text);
    }

    .field {
        display: flex;
        flex-direction: column;
        gap: 5px;
    }

    .field > span {
        font-size: 11px;
        color: var(--text-dim);
    }

    .swatches {
        display: grid;
        grid-template-columns: repeat(auto-fill, minmax(20px, 1fr));
        gap: 5px;
    }

    .swatch {
        height: 20px;
        border-radius: var(--radius-sm);
        box-shadow: inset 0 0 0 1px rgba(0, 0, 0, 0.35);
        transition: transform 90ms ease;
    }

    .swatch:hover {
        transform: scale(1.12);
    }

    .swatch.chosen {
        box-shadow:
            inset 0 0 0 1px rgba(0, 0, 0, 0.35),
            0 0 0 2px var(--bg-panel),
            0 0 0 3px var(--text);
    }

    .swatch.custom {
        display: grid;
        place-items: center;
        background: var(--bg-raised);
        color: var(--text-dim);
        box-shadow: inset 0 0 0 1px var(--border);
    }

    .hex {
        grid-column: 1 / -1;
        font-family: var(--mono);
        font-size: 11px;
    }

    .meta {
        display: flex;
        flex-direction: column;
        gap: 3px;
        margin: 2px 0 0;
        padding-top: 8px;
        border-top: 1px solid var(--border-soft);
        font-size: 11px;
    }

    .meta > div {
        display: grid;
        grid-template-columns: 62px 1fr;
        gap: 8px;
        min-width: 0;
    }

    dt {
        color: var(--text-faint);
    }

    dd {
        margin: 0;
        color: var(--text-dim);
        overflow: hidden;
        text-overflow: ellipsis;
        white-space: nowrap;
        font-family: var(--mono);
        font-size: 10.5px;
    }
</style>
