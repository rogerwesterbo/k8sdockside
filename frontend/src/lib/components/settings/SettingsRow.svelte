<!--
  One labelled setting: a name, a sentence saying what it does, and whatever
  control sets it. Shared by every section so the four of them read as one page
  rather than four people's ideas of a form.
-->
<script lang="ts">
    import type { Snippet } from 'svelte';

    interface Props {
        label: string;
        /** What the setting does, and where it is not obvious, why you'd change it. */
        hint?: string;
        /** The control. Laid out to the right on a wide panel, below on a narrow one. */
        children: Snippet;
    }

    let { label, hint, children }: Props = $props();
</script>

<div class="row">
    <div class="text">
        <span class="label">{label}</span>
        {#if hint}<span class="hint">{hint}</span>{/if}
    </div>
    <div class="control">{@render children()}</div>
</div>

<style>
    .row {
        display: flex;
        align-items: flex-start;
        justify-content: space-between;
        gap: 24px;
        padding: 14px 0;
        border-bottom: 1px solid var(--border-soft);
    }

    .row:last-child {
        border-bottom: none;
    }

    .text {
        display: flex;
        flex-direction: column;
        gap: 3px;
        min-width: 0;
    }

    .label {
        font-size: 13px;
        color: var(--text);
    }

    .hint {
        font-size: 11.5px;
        line-height: 1.6;
        color: var(--text-faint);
        max-width: 46ch;
    }

    .control {
        flex: 0 0 auto;
        display: flex;
        align-items: center;
        gap: 8px;
    }

    /* Below ~640px the control no longer fits beside its label without
       squeezing the sentence to a word a line. */
    @container settings-panel (max-width: 640px) {
        .row {
            flex-direction: column;
            gap: 10px;
        }
    }
</style>
