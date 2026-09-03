<!--
  One kubeconfig context in the sidebar, plus the resource tree it expands into.
  Clicking a resource opens (or focuses) a tab for it.
-->
<script lang="ts">
    import type * as kube from '../../../bindings/github.com/roger/k8sdockside/internal/kube/models.js';
    import { NAV_GROUPS } from '../catalogue';
    import { alpha } from '../colors';
    import { workspace, type Health } from '../state/workspace.svelte';
    import Icon from './Icon.svelte';

    interface Props {
        context: kube.Context;
    }

    let { context }: Props = $props();

    /** The context's own row, which is what gets scrolled to and flashed. */
    let head = $state<HTMLElement>();

    let color = $derived(workspace.colorOf(context.id));
    let health = $derived(workspace.healthOf(context.id));
    let expanded = $derived(workspace.isExpanded(context.id));
    let selected = $derived(workspace.selectedContextId === context.id);
    let activeTab = $derived(workspace.activeTab);

    function isOpen(kind: string): boolean {
        return activeTab?.contextId === context.id && activeTab.kind === kind;
    }

    /** How much room to leave above a revealed row, so it is not jammed to the edge. */
    const REVEAL_MARGIN = 8;

    /** The nearest ancestor that is actually scrolling, if any. */
    function scrollParent(el: HTMLElement): HTMLElement | null {
        for (let node = el.parentElement; node; node = node.parentElement) {
            const overflow = getComputedStyle(node).overflowY;
            if ((overflow === 'auto' || overflow === 'scroll') && node.scrollHeight > node.clientHeight) {
                return node;
            }
        }
        return null;
    }

    // Bring this context into view when a tab for it is clicked.
    //
    // A context is a header with its resource tree hanging below it, and what
    // the user wants to see is both. `scrollIntoView({block: 'nearest'})` does
    // the minimum to reveal the *header*, which going downwards means parking
    // its 30px strip flush on the bottom edge with the whole tree still off
    // screen -- indistinguishable from not having scrolled at all. Going
    // upwards it lands at the top and looks fine, which is why this only shows
    // up switching to a context below the one you are on.
    //
    // So: leave it alone when the row is already fully visible, and otherwise
    // put the row near the top, where its tree has somewhere to be.
    $effect(() => {
        const request = workspace.reveal;
        if (!request || request.contextId !== context.id || !head) return;
        request.nonce;

        const still = window.matchMedia('(prefers-reduced-motion: reduce)').matches;
        const behavior: ScrollBehavior = still ? 'auto' : 'smooth';
        const scroller = scrollParent(head);

        if (scroller) {
            const view = scroller.getBoundingClientRect();
            const box = head.getBoundingClientRect();
            if (box.top < view.top || box.bottom > view.bottom) {
                scroller.scrollTo({ top: scroller.scrollTop + (box.top - view.top) - REVEAL_MARGIN, behavior });
            }
        } else {
            // No scrolling ancestor: nothing to move, but still flash.
            head.scrollIntoView({ block: 'nearest', behavior });
        }

        // Removed and re-added around a forced reflow, the standard way to
        // restart an animation that may still be running from a prior reveal.
        head.classList.remove('flash');
        void head.offsetWidth;
        head.classList.add('flash');
    });

    function statusTitle(state: Health): string {
        switch (state.status) {
            case 'connected':
                return 'Connected';
            case 'checking':
                return 'Checking the connection…';
            case 'error':
                return `Cannot connect — ${state.message}`;
            default:
                // Unchecked contexts show no indicator, so there is nothing to
                // describe.
                return '';
        }
    }
</script>

<div class="context" class:selected style:--ctx-color={color} style:--ctx-tint={alpha(color, 0.16)}>
    <div class="head" bind:this={head}>
        <button
            class="twisty"
            onclick={() => workspace.toggleExpanded(context.id)}
            aria-label={expanded ? 'Collapse' : 'Expand'}
            aria-expanded={expanded}
        >
            <Icon name={expanded ? 'chevron-down' : 'chevron-right'} size={14} />
        </button>

        <button class="label" onclick={() => workspace.selectContext(context.id)} title={context.server || context.name}>
            <span class="swatch" style:background={color}></span>
            <span class="name">{workspace.displayName(context)}</span>
            {#if context.current}
                <span class="badge" title="current-context in this kubeconfig">current</span>
            {/if}
        </button>

        <!-- Reachability sits at the far right, opposite the colour swatch, so
             "which cluster is this" and "can I reach it" never get confused for
             one another. -->
        {#if health.status === 'error'}
            <span class="status broken" title={statusTitle(health)} aria-label={statusTitle(health)}>
                <Icon name="alert" size={12} />
            </span>
        {:else if health.status === 'unknown'}
            <!-- Nothing drawn until a cluster has actually been checked: a mark
                 meaning "no news" is noise on a list this long. The slot is
                 still held open so names do not shift as results arrive. -->
            <span class="status" aria-hidden="true"></span>
        {:else}
            <span
                class="status dot {health.status}"
                title={statusTitle(health)}
                aria-label={statusTitle(health)}
            ></span>
        {/if}
    </div>

    {#if expanded}
        <div class="tree">
            {#each NAV_GROUPS as group (group.label)}
                <p class="group">{group.label}</p>
                {#each group.items as item (item.kind)}
                    <button
                        class="item"
                        class:open={isOpen(item.kind)}
                        onclick={() => workspace.openTab(context.id, item.kind)}
                    >
                        <Icon name={item.icon} size={15} />
                        <span>{item.label}</span>
                    </button>
                {/each}
            {/each}
        </div>
    {/if}
</div>

<style>
    .context {
        --indent: 26px;
    }

    .head {
        display: flex;
        align-items: center;
        height: var(--row-h);
        border-radius: var(--radius-sm);
        position: relative;
    }

    .head:hover {
        background: var(--bg-hover);
    }

    .selected .head {
        background: var(--ctx-tint);
    }

    /* A colour bar on the selected context, matching its tabs. */
    .selected .head::before {
        content: '';
        position: absolute;
        left: 0;
        top: 4px;
        bottom: 4px;
        width: 2px;
        border-radius: 2px;
        background: var(--ctx-color);
    }

    /* The reveal flash. It sits over the row rather than changing its
       background so that it works the same whether the context is selected or
       not, and it is the context's own colour so the row that lights up is
       recognisably the one whose tab was clicked. The global reduced-motion
       rule already collapses this to nothing. */
    .head.flash::after {
        content: '';
        position: absolute;
        inset: 0;
        border-radius: var(--radius-sm);
        background: var(--ctx-color);
        opacity: 0;
        pointer-events: none;
        animation: reveal-flash 700ms ease-out;
    }

    @keyframes reveal-flash {
        from {
            opacity: 0.4;
        }
        to {
            opacity: 0;
        }
    }

    .twisty {
        display: grid;
        place-items: center;
        width: var(--indent);
        height: 100%;
        color: var(--text-faint);
        flex: 0 0 auto;
    }

    .twisty:hover {
        color: var(--text);
    }

    .label {
        display: flex;
        align-items: center;
        gap: 7px;
        flex: 1 1 auto;
        min-width: 0;
        height: 100%;
        padding-right: 8px;
        text-align: left;
    }

    .swatch {
        width: 9px;
        height: 9px;
        border-radius: 2px;
        flex: 0 0 auto;
        box-shadow: 0 0 0 1px rgba(0, 0, 0, 0.35);
    }

    .name {
        overflow: hidden;
        text-overflow: ellipsis;
        white-space: nowrap;
        color: var(--text-dim);
    }

    .selected .name {
        color: var(--text);
    }

    .status {
        display: grid;
        place-items: center;
        width: 16px;
        flex: 0 0 auto;
        margin-right: 6px;
    }

    .status.dot::before {
        content: '';
        width: 7px;
        height: 7px;
        border-radius: 50%;
    }

    .status.dot.connected::before {
        /* A faint halo, so green reads as live rather than as another swatch. */
        background: var(--ok);
        box-shadow: 0 0 0 2px color-mix(in srgb, var(--ok) 22%, transparent);
    }

    .status.dot.checking::before {
        background: var(--accent);
        animation: pulse 1.1s ease-in-out infinite;
    }

    @keyframes pulse {
        50% {
            opacity: 0.25;
        }
    }

    /* A triangle rather than a red dot: in a list of twenty clusters a broken
       one has to be findable by shape, not only by colour. */
    .status.broken {
        color: var(--error);
    }

    .badge {
        flex: 0 0 auto;
        font-size: 9px;
        letter-spacing: 0.04em;
        text-transform: uppercase;
        color: var(--ctx-color);
        border: 1px solid var(--ctx-color);
        border-radius: 3px;
        padding: 0 4px;
        line-height: 14px;
        opacity: 0.85;
    }

    .tree {
        padding: 2px 0 6px;
    }

    .group {
        margin: 8px 0 2px;
        padding-left: var(--indent);
        font-size: 10px;
        letter-spacing: 0.08em;
        text-transform: uppercase;
        color: var(--text-faint);
    }

    .item {
        display: flex;
        align-items: center;
        gap: 8px;
        width: 100%;
        height: 26px;
        padding-left: var(--indent);
        padding-right: 8px;
        color: var(--text-dim);
        border-radius: var(--radius-sm);
        text-align: left;
    }

    .item:hover {
        background: var(--bg-hover);
        color: var(--text);
    }

    .item.open {
        background: var(--ctx-tint);
        color: var(--text);
    }

    .item span {
        overflow: hidden;
        text-overflow: ellipsis;
        white-space: nowrap;
    }
</style>
