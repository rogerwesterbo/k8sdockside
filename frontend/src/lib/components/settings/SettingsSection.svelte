<!--
  A section's heading and its rows. Every section opens with one of these, so
  the four share a width, a rhythm and a type scale.
-->
<script lang="ts">
    import type { Snippet } from 'svelte';

    interface Props {
        title: string;
        /** A sentence of context for the whole section, where one earns its place. */
        lede?: string;
        children: Snippet;
    }

    let { title, lede, children }: Props = $props();
</script>

<section>
    <h2>{title}</h2>
    {#if lede}<p class="lede">{lede}</p>{/if}
    {@render children()}
</section>

<style>
    section {
        max-width: 760px;
        /* Named so SettingsRow can reflow against the panel's width rather than
           the window's -- the sidebar and the detail panel both change how much
           room this has without the window changing at all. */
        container: settings-panel / inline-size;
    }

    h2 {
        margin: 0 0 4px;
        font-size: 16px;
        font-weight: 600;
    }

    .lede {
        margin: 0 0 12px;
        font-size: 12px;
        line-height: 1.6;
        color: var(--text-dim);
        max-width: 62ch;
    }
</style>
