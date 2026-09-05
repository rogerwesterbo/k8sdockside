<!--
  A theme drawn as a miniature of the app: title bar, sidebar with a selected
  item, and a few rows with status pills.

  A row of flat colour swatches was the obvious thing and is a much worse one.
  What decides whether a theme is usable is not which colours are in it but how
  they sit together -- whether the selected sidebar row reads as selected,
  whether a Pending pill is distinguishable from a Running one, whether the
  small print survives the surface it is on. A miniature shows all three at a
  glance; twelve squares show none of them.

  Every colour here comes from the theme's own resolved tokens, applied inline
  rather than through :root. That is what lets thirteen previews sit on one
  screen each wearing a different palette, and it means the preview cannot drift
  from the theme -- it is drawing with exactly what the app would.
-->
<script lang="ts">
    import type { Theme } from '../../theme/apply';

    let { theme, size = 'full' }: { theme: Theme; size?: 'full' | 'tile' } = $props();

    let t = $derived(theme.resolved);

    // The rows of the fake pod listing. Names taken from real workloads so the
    // preview reads as a screenshot rather than as lorem ipsum, and one of each
    // status so all three status tokens are on show.
    const ROWS = [
        { name: 'argocd-server-7d9f', status: 'Running', tone: 'ok' },
        { name: 'cilium-agent-x2kl', status: 'Running', tone: 'ok' },
        { name: 'harbor-core-b81a', status: 'Pending', tone: 'warn' },
        { name: 'kubevirt-handler-9c', status: 'CrashLoop', tone: 'error' },
    ] as const;

    const NAV = ['Overview', 'Nodes', 'Pods', 'Services', 'Config'];
    const SELECTED = 'Pods';
</script>

<div
    class="preview {size}"
    style:background={t.bg}
    style:border-color={t.border}
    aria-hidden="true"
>
    <div class="chrome" style:background={t['bg-sidebar']} style:border-color={t.border}>
        <span class="dot" style:background={t['text-faint']}></span>
        <span class="dot" style:background={t['text-faint']}></span>
        <span class="dot" style:background={t['text-faint']}></span>
        <span class="title" style:color={t['text-dim']}>prod-cluster · pods</span>
    </div>

    <div class="body">
        <div class="rail" style:background={t['bg-sidebar']} style:border-color={t.border}>
            {#each NAV as item (item)}
                {#if item === SELECTED}
                    <span class="nav on" style:background={t.accent} style:color={t['accent-text']}>
                        {item}
                    </span>
                {:else}
                    <span class="nav" style:color={t['text-dim']}>{item}</span>
                {/if}
            {/each}
        </div>

        <div class="rows" style:background={t['bg-panel']}>
            {#each ROWS as row (row.name)}
                <div class="row" style:border-color={t['border-soft']}>
                    <span class="name" style:color={t.text}>{row.name}</span>
                    <span
                        class="pill"
                        style:color={t[row.tone]}
                        style:border-color={t[row.tone]}
                        style:background={t['bg-raised']}
                    >
                        {row.status}
                    </span>
                </div>
            {/each}
        </div>
    </div>
</div>

<style>
    /* Sized in px and never in ems: this is a picture of an interface, and it
       has to keep its proportions whatever the surrounding text is doing. */
    .preview {
        border-radius: var(--radius);
        border: 1px solid;
        overflow: hidden;
        user-select: none;
    }

    .chrome {
        display: flex;
        align-items: center;
        gap: 4px;
        height: 18px;
        padding: 0 7px;
        border-bottom: 1px solid;
    }

    .dot {
        width: 5px;
        height: 5px;
        border-radius: 50%;
        opacity: 0.6;
        flex: 0 0 auto;
    }

    .title {
        margin-left: 6px;
        font-size: 7.5px;
        white-space: nowrap;
        overflow: hidden;
        text-overflow: ellipsis;
    }

    .body {
        display: flex;
        min-height: 0;
    }

    .rail {
        display: flex;
        flex-direction: column;
        gap: 2px;
        flex: 0 0 auto;
        width: 74px;
        padding: 6px 5px;
        border-right: 1px solid;
    }

    .nav {
        padding: 2px 5px;
        border-radius: 3px;
        font-size: 7.5px;
        white-space: nowrap;
        overflow: hidden;
        text-overflow: ellipsis;
    }

    .nav.on {
        font-weight: 600;
    }

    .rows {
        display: flex;
        flex-direction: column;
        flex: 1 1 auto;
        min-width: 0;
        padding: 4px 0;
    }

    .row {
        display: flex;
        align-items: center;
        gap: 6px;
        padding: 3px 8px;
        border-bottom: 1px solid;
    }

    .row:last-child {
        border-bottom-color: transparent;
    }

    .name {
        flex: 1 1 auto;
        min-width: 0;
        font-size: 7.5px;
        white-space: nowrap;
        overflow: hidden;
        text-overflow: ellipsis;
    }

    .pill {
        flex: 0 0 auto;
        padding: 1px 5px;
        border-radius: 999px;
        border: 1px solid;
        font-size: 6.5px;
        white-space: nowrap;
    }

    /* The gallery tile is the same drawing with less room, so the sidebar gives
       up width first -- the rows and their pills are what tells two themes
       apart, and the nav labels are legible enough at this size to be read as
       labels rather than as text. */
    .preview.tile .rail {
        width: 58px;
    }
</style>
