<!--
  One kubeconfig context in the sidebar, plus the resource tree it expands into.
  Clicking a resource opens (or focuses) a tab for it.
-->
<script lang="ts">
    import type * as kube from '../../../bindings/github.com/roger/k8sdockside/internal/kube/models.js';
    import { DASHBOARD_ITEM, DEFINITIONS_GROUP, NAV_GROUPS } from '../catalogue';
    import { classify } from '../errors';
    import { alpha } from '../colors';
    import { workspace, type Health } from '../state/workspace.svelte';
    import Icon from './Icon.svelte';

    interface Props {
        context: kube.Context;
    }

    let { context }: Props = $props();

    /** The context's own row, and the whole block, for the reveal below. */
    let head = $state<HTMLElement>();
    let root = $state<HTMLElement>();

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

        // The tab's own row where it is showing, and the context's row only as
        // a fallback. With fifty rows under a context, bringing the cluster's
        // name into view says nothing about where in them the tab lives -- the
        // sidebar moves, lands on the wrong thing, and reads as a flicker.
        const target = rowFor(request.kind) ?? head;

        const still = window.matchMedia('(prefers-reduced-motion: reduce)').matches;
        const behavior: ScrollBehavior = still ? 'auto' : 'smooth';
        const scroller = scrollParent(target);

        if (scroller) {
            const view = scroller.getBoundingClientRect();
            const box = target.getBoundingClientRect();
            if (box.top < view.top || box.bottom > view.bottom) {
                scroller.scrollTo({ top: scroller.scrollTop + (box.top - view.top) - REVEAL_MARGIN, behavior });
            }
        } else {
            // No scrolling ancestor: nothing to move, but still flash.
            target.scrollIntoView({ block: 'nearest', behavior });
        }

        // Removed and re-added around a forced reflow, the standard way to
        // restart an animation that may still be running from a prior reveal.
        target.classList.remove('flash');
        void target.offsetWidth;
        target.classList.add('flash');
    });

    /**
     * This context's row for one kind, when the tree is showing it. Absent for a
     * kind whose section is shut, or a custom resource whose API group is -- in
     * which case the reveal falls back to the context itself.
     */
    function rowFor(kind: string): HTMLElement | null {
        if (!kind || !root) return null;
        return root.querySelector<HTMLElement>(`[data-kind="${CSS.escape(kind)}"]`);
    }

    // The modifier key is named for the platform, since the whole point of the
    // hint is that someone can act on it.
    const ALT = navigator.platform.startsWith('Mac') ? '\u2325' : 'Alt';

    function groupTitle(label: string, folded: boolean, local: boolean): string {
        const action = folded ? `Show ${label}` : `Hide ${label}`;
        const here = `${action} for this cluster (${ALT}-click for every cluster)`;
        return local ? `${here} \u2014 currently set differently here than elsewhere` : here;
    }

    // The definitions come from the cluster, so they are asked for whenever the
    // section is actually showing -- not when its heading is clicked.
    //
    // Clicking is only one of the ways it comes to be open: it may have been
    // left open in this context's folding from a previous session, or unfolded
    // on its own when a tab for one of its kinds was activated. Hanging the
    // fetch off the click missed all of those, and the section then sat empty
    // until it was collapsed and opened again. Keyed off the state, every route
    // in loads it.
    //
    // Still lazy: nothing is asked of a cluster whose definitions section is
    // shut, and loadCustomKinds does nothing once a context has an answer.
    $effect(() => {
        if (!expanded || workspace.isGroupCollapsed(context.id, DEFINITIONS_GROUP)) return;
        void workspace.loadCustomKinds(context.id);
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

<div class="context" bind:this={root} class:selected style:--ctx-color={color} style:--ctx-tint={alpha(color, 0.16)}>
    <div class="head" bind:this={head}>
        <button
            class="twisty"
            onclick={() => workspace.toggleExpanded(context.id)}
            aria-label={expanded ? 'Collapse' : 'Expand'}
            aria-expanded={expanded}
        >
            <Icon name={expanded ? 'chevron-down' : 'chevron-right'} size={14} />
        </button>

        <button
            class="label"
            onclick={() => workspace.activateContext(context.id)}
            aria-expanded={expanded}
            title={context.server || context.name}
        >
            <span class="swatch" style:background={color}></span>
            <span class="name">{workspace.displayName(context)}</span>
            {#if context.current}
                <span class="badge" title="current-context in this kubeconfig">current</span>
            {/if}
        </button>

        <!-- Only while the context is open: collapsing the sections of a
             context you cannot see is a control with nothing to act on. -->
        {#if expanded}
            {@const shutting = workspace.anyGroupOpen(context.id)}
            <button
                class="sections"
                onclick={() => (shutting
                    ? workspace.collapseAllGroups(context.id)
                    : workspace.expandAllGroups(context.id))}
                title={shutting ? 'Collapse every section here' : 'Expand every section here'}
                aria-label={shutting ? 'Collapse every section here' : 'Expand every section here'}
            >
                <Icon name={shutting ? 'collapse-all' : 'expand-all'} size={13} />
            </button>
        {/if}

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
            <!-- Outside the sections: the overview is what is most often wanted
                 straight after opening a context, and a section holding one row
                 is a heading you have to open to reach a single thing. -->
            <button
                class="item"
                data-kind={DASHBOARD_ITEM.kind}
                class:open={isOpen(DASHBOARD_ITEM.kind)}
                onclick={() => workspace.openTab(context.id, DASHBOARD_ITEM.kind)}
            >
                <Icon name={DASHBOARD_ITEM.icon} size={15} />
                <span>{DASHBOARD_ITEM.label}</span>
            </button>

            {#each NAV_GROUPS as group (group.label)}
                {@const folded = workspace.isGroupCollapsed(context.id, group.label)}
                {@const local = workspace.groupDiffersFromGlobal(context.id, group.label)}
                <div class="group-row">
                <button
                    class="group"
                    class:folded
                    class:local
                    onclick={(event) => workspace.toggleGroup(context.id, group.label, {
                        allContexts: event.altKey,
                    })}
                    aria-expanded={!folded}
                    title={groupTitle(group.label, folded, local)}
                >
                    <Icon name={folded ? 'chevron-right' : 'chevron-down'} size={11} />
                    <span>{group.label}</span>
                    <!-- The count only earns its place when the group is shut:
                         open, the items themselves say how many there are. -->
                    {#if folded}<span class="tally">{group.items.length}</span>{/if}
                </button>

                <!-- Only the definitions section can go stale: its contents are
                     the cluster's, not this list's, and they are read once. A
                     CRD installed since is a deliberate act, so asking again is
                     offered rather than done by holding a watch open.

                     Nested inside the heading it would fold the section on its
                     way through, so it sits beside it and stops the click. -->
                {#if !folded && group.label === DEFINITIONS_GROUP}
                    {@const reading = workspace.customKindsFor(context.id).status === 'loading'}
                    <button
                        class="reload"
                        class:spinning={reading}
                        disabled={reading}
                        onclick={(event) => {
                            event.stopPropagation();
                            void workspace.loadCustomKinds(context.id, { force: true });
                        }}
                        title="Read this cluster's definitions again"
                        aria-label="Read this cluster's definitions again"
                    >
                        <Icon name="refresh" size={12} />
                    </button>
                {/if}
                </div>

                {#if !folded}
                    {#each group.items as item (item.kind)}
                        <button
                            class="item"
                            data-kind={item.kind}
                            class:open={isOpen(item.kind)}
                            onclick={() => workspace.openTab(context.id, item.kind)}
                        >
                            <Icon name={item.icon} size={15} />
                            <span>{item.label}</span>
                        </button>
                    {/each}

                    <!-- The definitions section continues into the cluster: the
                         API groups it serves, each holding its own kinds. -->
                    {#if group.label === DEFINITIONS_GROUP}
                        {@const loaded = workspace.customKindsFor(context.id)}
                        {#if loaded.status === 'loading'}
                            <p class="note">Looking for definitions…</p>
                        {:else if loaded.status === 'error'}
                            <!-- Named rather than generic. "Could not read
                                 definitions" leaves no permission, no network
                                 and none installed looking identical, and they
                                 call for different reactions -- so this uses
                                 the same reading of the error the error pages
                                 do, with the wire text on hover. -->
                            <p class="note failed" title={loaded.message}>
                                {classify(loaded.message).headline}
                            </p>
                        {:else if loaded.status === 'ready' && loaded.groups.length === 0}
                            <p class="note">No custom resources installed</p>
                        {:else}
                            {#each loaded.groups as api (api.group)}
                                {@const open = workspace.isApiGroupExpanded(context.id, api.group)}
                                <button
                                    class="api"
                                    onclick={() => workspace.toggleApiGroup(context.id, api.group)}
                                    aria-expanded={open}
                                    title={api.group}
                                >
                                    <Icon name={open ? 'chevron-down' : 'chevron-right'} size={11} />
                                    <span>{api.group}</span>
                                    <span class="tally">{api.kinds.length}</span>
                                </button>

                                {#if open}
                                    {#each api.kinds as custom (custom.kind)}
                                        <button
                                            class="item nested"
                                            data-kind={custom.kind}
                                            class:open={isOpen(custom.kind)}
                                            onclick={() => workspace.openTab(context.id, custom.kind)}
                                        >
                                            <Icon name="puzzle" size={14} />
                                            <span>{custom.label}</span>
                                        </button>
                                    {/each}
                                {/if}
                            {/each}
                        {/if}
                    {/if}
                {/if}
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
    .flash::after {
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

    .sections {
        display: grid;
        place-items: center;
        width: 18px;
        height: 18px;
        flex: 0 0 auto;
        border-radius: 3px;
        color: var(--text-faint);
    }

    .sections:hover {
        background: var(--bg-active);
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

    /* The heading and the definitions section's refresh sit on one line. */
    .group-row {
        display: flex;
        align-items: center;
        margin: 8px 0 2px;
    }

    .group-row .group {
        margin: 0;
    }

    .group {
        display: flex;
        align-items: center;
        gap: 4px;
        width: 100%;
        margin: 8px 0 2px;
        /* One chevron width less than the items, so the arrow lines up with
           where the group's contents begin rather than indenting past them. */
        padding-left: calc(var(--indent) - 15px);
        padding-right: 8px;
        /* Sized to be read rather than merely noticed: at 10px these were the
           smallest text in the app while being the thing you navigate by. */
        font-size: 11.5px;
        letter-spacing: 0.05em;
        text-transform: uppercase;
        font-weight: 600;
        color: var(--text-dim);
        text-align: left;
        border-radius: var(--radius-sm);
        min-height: 22px;
    }

    .group:hover {
        background: var(--bg-hover);
        color: var(--text-dim);
    }

    .group span:first-of-type {
        overflow: hidden;
        text-overflow: ellipsis;
        white-space: nowrap;
    }

    /* A group this cluster has been set differently from the rest. Without a
       mark, a sidebar that disagrees with every other cluster looks like a bug
       rather than something the user asked for. */
    .group.local span:first-of-type::after {
        content: '';
        display: inline-block;
        width: 4px;
        height: 4px;
        margin-left: 5px;
        vertical-align: middle;
        border-radius: 50%;
        background: var(--ctx-color);
    }

    /* How much is hidden, so a folded group is not a dead end. */
    .tally {
        margin-left: auto;
        font-size: 9px;
        letter-spacing: 0;
        color: var(--text-faint);
        background: var(--bg-raised);
        border-radius: 7px;
        padding: 0 5px;
        line-height: 13px;
    }

    .item {
        display: flex;
        align-items: center;
        /* So the reveal flash can be drawn over it. */
        position: relative;
        gap: 8px;
        width: 100%;
        height: 26px;
        padding-left: var(--indent);
        padding-right: 8px;
        color: var(--text-dim);
        border-radius: var(--radius-sm);
        text-align: left;
    }

    /* One API group inside the definitions section: a heading, but a level in
       rather than a peer of the section headings, so it is indented and
       lower-key than they are. */
    .reload {
        display: grid;
        place-items: center;
        width: 20px;
        height: 20px;
        flex: 0 0 auto;
        margin-right: 4px;
        border-radius: 3px;
        color: var(--text-faint);
    }

    .reload:hover:not(:disabled) {
        background: var(--bg-active);
        color: var(--text);
    }

    .reload:disabled {
        cursor: default;
    }

    .reload.spinning {
        animation: spin 900ms linear infinite;
        color: var(--accent);
    }

    @keyframes spin {
        to {
            transform: rotate(360deg);
        }
    }

    .api {
        display: flex;
        align-items: center;
        gap: 5px;
        width: 100%;
        min-height: 24px;
        padding-left: var(--indent);
        padding-right: 8px;
        color: var(--text-dim);
        border-radius: var(--radius-sm);
        text-align: left;
        font-family: var(--mono);
        font-size: 11px;
    }

    .api:hover {
        background: var(--bg-hover);
        color: var(--text);
    }

    .api span:first-of-type {
        overflow: hidden;
        text-overflow: ellipsis;
        white-space: nowrap;
    }

    /* A definition sits one level in from its API group. */
    .item.nested {
        padding-left: calc(var(--indent) + 16px);
    }

    .note {
        margin: 2px 0 4px;
        padding-left: var(--indent);
        font-size: 11px;
        color: var(--text-faint);
    }

    .note.failed {
        color: var(--error);
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
