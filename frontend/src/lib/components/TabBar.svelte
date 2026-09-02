<!--
  The tab strip. Each tab is coloured with its context's colour -- fully
  saturated when active, tinted when not -- so you can always see which cluster
  a tab is talking to. Tabs are reordered by dragging; the order is persisted.
-->
<script lang="ts">
    import { iconFor } from '../catalogue';
    import { alpha, textOn } from '../colors';
    import { workspace } from '../state/workspace.svelte';
    import Icon from './Icon.svelte';

    /** Index of the tab currently being dragged, or null when not dragging. */
    let dragIndex = $state<number | null>(null);

    // The context name only earns space on a tab when tabs from more than one
    // context are open; otherwise the colour alone is enough.
    let showContext = $derived(new Set(workspace.tabs.map((t) => t.contextId)).size > 1);

    function contextName(contextId: string): string {
        const context = workspace.contexts.find((c) => c.id === contextId);
        return context ? workspace.displayName(context) : contextId;
    }

    function startDrag(event: DragEvent, index: number): void {
        dragIndex = index;
        event.dataTransfer?.setData('text/plain', workspace.tabs[index].id);
        if (event.dataTransfer) event.dataTransfer.effectAllowed = 'move';
    }

    /**
     * Reorders as the pointer passes over a neighbour, so the strip shows the
     * result directly instead of an insertion marker. The write to disk is
     * debounced, so a whole drag costs one save.
     */
    function dragOver(event: DragEvent, index: number): void {
        event.preventDefault();
        if (event.dataTransfer) event.dataTransfer.dropEffect = 'move';
        if (dragIndex === null || dragIndex === index) return;
        workspace.moveTab(dragIndex, index);
        dragIndex = index;
    }

    /** Alt+Arrow moves the focused tab, so reordering is not drag-only. */
    function onKeyDown(event: KeyboardEvent, index: number): void {
        if (event.altKey && (event.key === 'ArrowLeft' || event.key === 'ArrowRight')) {
            event.preventDefault();
            workspace.moveTab(index, index + (event.key === 'ArrowLeft' ? -1 : 1));
            return;
        }
        if (event.key === 'Enter' || event.key === ' ') {
            event.preventDefault();
            workspace.activateTab(workspace.tabs[index].id);
        }
    }

    function onPointerDown(event: MouseEvent, id: string): void {
        if (event.button === 1) {
            event.preventDefault();
            workspace.closeTab(id);
        }
    }
</script>

<div class="tabbar" role="tablist" aria-label="Open views">
    {#each workspace.tabs as tab, index (tab.id)}
        {@const color = workspace.colorOf(tab.contextId)}
        {@const active = workspace.activeTabId === tab.id}
        <div
            class="tab"
            class:active
            class:dragging={dragIndex === index}
            role="tab"
            tabindex={active ? 0 : -1}
            aria-selected={active}
            title="{tab.title} — {contextName(tab.contextId)}"
            draggable="true"
            style:--tab-bg={active ? color : alpha(color, 0.18)}
            style:--tab-fg={active ? textOn(color) : 'var(--text-dim)'}
            style:--tab-rule={color}
            onclick={() => workspace.activateTab(tab.id)}
            onmousedown={(e) => onPointerDown(e, tab.id)}
            onkeydown={(e) => onKeyDown(e, index)}
            ondragstart={(e) => startDrag(e, index)}
            ondragover={(e) => dragOver(e, index)}
            ondragend={() => (dragIndex = null)}
            ondrop={(e) => e.preventDefault()}
        >
            <Icon name={iconFor(tab.kind)} size={14} />
            <span class="title">{tab.title}</span>
            {#if showContext}
                <span class="context">{contextName(tab.contextId)}</span>
            {/if}
            <button
                class="close"
                aria-label="Close {tab.title}"
                onclick={(e) => {
                    e.stopPropagation();
                    workspace.closeTab(tab.id);
                }}
            >
                <Icon name="close" size={12} />
            </button>
        </div>
    {/each}
</div>

<style>
    .tabbar {
        display: flex;
        align-items: flex-end;
        gap: 2px;
        height: 38px;
        padding: 0 6px;
        flex: 0 0 auto;
        background: var(--bg-sidebar);
        border-bottom: 1px solid var(--border);
        overflow-x: auto;
        overflow-y: hidden;
        scrollbar-width: none;
    }

    .tabbar::-webkit-scrollbar {
        height: 0;
    }

    .tab {
        display: flex;
        align-items: center;
        gap: 7px;
        height: 30px;
        max-width: 240px;
        padding: 0 6px 0 10px;
        flex: 0 0 auto;
        border-radius: var(--radius) var(--radius) 0 0;
        background: var(--tab-bg);
        color: var(--tab-fg);
        cursor: default;
        position: relative;
        transition:
            background 110ms ease,
            color 110ms ease;
    }

    /* A full-strength rule along the top edge keeps an inactive tab's context
       identifiable even though its body is only tinted. */
    .tab::before {
        content: '';
        position: absolute;
        inset: 0 0 auto 0;
        height: 2px;
        border-radius: 2px 2px 0 0;
        background: var(--tab-rule);
    }

    .tab:not(.active):hover {
        background: color-mix(in srgb, var(--tab-rule) 32%, transparent);
        color: var(--text);
    }

    .tab.dragging {
        opacity: 0.55;
    }

    .title {
        overflow: hidden;
        text-overflow: ellipsis;
        white-space: nowrap;
    }

    .context {
        font-size: 11px;
        opacity: 0.72;
        overflow: hidden;
        text-overflow: ellipsis;
        white-space: nowrap;
        max-width: 90px;
    }

    .close {
        display: grid;
        place-items: center;
        width: 18px;
        height: 18px;
        border-radius: 3px;
        color: inherit;
        opacity: 0.6;
        flex: 0 0 auto;
    }

    .close:hover {
        opacity: 1;
        background: rgba(0, 0, 0, 0.25);
    }
</style>
