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
            return;
        }
        if (event.key === 'ContextMenu' || (event.shiftKey && event.key === 'F10')) {
            event.preventDefault();
            openMenuFromKeyboard(event, workspace.tabs[index]);
        }
    }

    function onPointerDown(event: MouseEvent, id: string): void {
        if (event.button === 1) {
            event.preventDefault();
            workspace.closeTab(id);
        }
    }

    // ----- scrolling ---------------------------------------------------
    //
    // Tabs keep their width and the strip scrolls, rather than compressing to
    // fit: a title carries the cluster and the resource kind, which is the pair
    // you are reading the strip to tell apart, and neither survives being
    // squeezed to a few characters.

    let stripEl = $state<HTMLElement | null>(null);
    let canLeft = $state(false);
    let canRight = $state(false);

    function measure(): void {
        if (!stripEl) return;
        const { scrollLeft, scrollWidth, clientWidth } = stripEl;
        // A pixel of slack: fractional widths mean scrollLeft rarely reaches
        // the exact end, which would leave the arrow lit with nowhere to go.
        canLeft = scrollLeft > 1;
        canRight = scrollLeft + clientWidth < scrollWidth - 1;
    }

    function nudge(direction: -1 | 1): void {
        if (!stripEl) return;
        stripEl.scrollBy({ left: direction * Math.max(140, stripEl.clientWidth * 0.6), behavior: 'smooth' });
    }

    /**
     * A vertical wheel scrolls the strip sideways, which is what a one-line
     * horizontal scroller should do and what browsers do not do for it.
     * Genuine horizontal input is left alone to scroll natively.
     */
    function onWheel(event: WheelEvent): void {
        if (!stripEl || Math.abs(event.deltaX) > Math.abs(event.deltaY)) return;
        event.preventDefault();
        stripEl.scrollLeft += event.deltaY;
    }

    // Attached by hand rather than as an attribute so the listener is
    // explicitly non-passive; a passive one cannot preventDefault, and the
    // page behind would scroll instead of the strip.
    $effect(() => {
        const el = stripEl;
        if (!el) return;
        el.addEventListener('wheel', onWheel, { passive: false });
        return () => el.removeEventListener('wheel', onWheel);
    });

    // Re-measure when the window or the sidebar changes the strip's width.
    $effect(() => {
        const el = stripEl;
        if (!el) return;
        const observer = new ResizeObserver(measure);
        observer.observe(el);
        return () => observer.disconnect();
    });

    // ...and when tabs are added or removed, which changes the content width
    // without changing the container's.
    $effect(() => {
        workspace.tabs.length;
        measure();
    });

    /**
     * Keep the active tab on screen. Reordering, restoring a session or closing
     * a neighbour can all leave it scrolled out of sight, and a tab you cannot
     * see is a tab you cannot tell is selected.
     */
    $effect(() => {
        const active = workspace.activeTabId;
        if (!active || !stripEl) return;

        const tab = stripEl.querySelector<HTMLElement>(`[data-tab-id="${CSS.escape(active)}"]`);
        // `block: nearest` keeps this from scrolling the page vertically as a
        // side effect of nudging the strip sideways.
        tab?.scrollIntoView({ behavior: 'smooth', block: 'nearest', inline: 'nearest' });
    });

    /** The tab the context menu was opened on, and where to draw it. */
    interface TabMenu {
        tabId: string;
        contextId: string;
        title: string;
        x: number;
        y: number;
    }

    let menu = $state<TabMenu | null>(null);
    let menuEl = $state<HTMLElement | null>(null);

    /**
     * Opens the menu on a tab, activating it first: acting on a tab whose
     * contents you cannot see is how the wrong one gets closed.
     */
    function openMenu(event: MouseEvent, tab: { id: string; contextId: string; title: string }): void {
        event.preventDefault();
        // The window handler below dismisses on any right-click; without this
        // the same event would close the menu we are opening.
        event.stopPropagation();
        workspace.activateTab(tab.id);
        menu = { tabId: tab.id, contextId: tab.contextId, title: tab.title, x: event.clientX, y: event.clientY };
    }

    /** Shift+F10 and the Menu key open it from the keyboard, at the tab itself. */
    function openMenuFromKeyboard(event: KeyboardEvent, tab: { id: string; contextId: string; title: string }): void {
        const box = (event.currentTarget as HTMLElement).getBoundingClientRect();
        workspace.activateTab(tab.id);
        menu = { tabId: tab.id, contextId: tab.contextId, title: tab.title, x: box.left, y: box.bottom };
    }

    function closeMenu(): void {
        menu = null;
    }

    function run(action: () => void): void {
        action();
        closeMenu();
    }

    // Keep the menu inside the window: a right-click near the right edge would
    // otherwise draw it off-screen where it cannot be read or dismissed.
    $effect(() => {
        if (!menu || !menuEl) return;
        const box = menuEl.getBoundingClientRect();
        const overflowX = box.right - window.innerWidth + 8;
        const overflowY = box.bottom - window.innerHeight + 8;
        if (overflowX > 0) menu.x -= overflowX;
        if (overflowY > 0) menu.y -= overflowY;
    });

    // Focus the first item so the menu is usable without the mouse that opened it.
    $effect(() => {
        if (menu && menuEl) menuEl.querySelector('button')?.focus();
    });

    function onMenuKeyDown(event: KeyboardEvent): void {
        if (event.key === 'Escape') {
            event.stopPropagation();
            closeMenu();
            return;
        }
        if (event.key !== 'ArrowDown' && event.key !== 'ArrowUp') return;

        event.preventDefault();
        const items = [...(menuEl?.querySelectorAll('button') ?? [])];
        const at = items.indexOf(document.activeElement as HTMLButtonElement);
        const next = (at + (event.key === 'ArrowDown' ? 1 : -1) + items.length) % items.length;
        items[next]?.focus();
    }
</script>

<svelte:window
    onclick={closeMenu}
    oncontextmenu={closeMenu}
    onresize={closeMenu}
    onkeydown={(e) => e.key === 'Escape' && closeMenu()}
/>

<div class="strip">
<div class="tabbar" role="tablist" aria-label="Open views" bind:this={stripEl} onscroll={measure}>
    {#each workspace.tabs as tab, index (tab.id)}
        {@const color = workspace.colorOf(tab.contextId)}
        {@const active = workspace.activeTabId === tab.id}
        <div
            class="tab"
            data-tab-id={tab.id}
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
            oncontextmenu={(e) => openMenu(e, tab)}
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

    <!-- Overlaid rather than laid out beside the strip, so appearing and
         disappearing does not shift the tabs sideways under the pointer. -->
    <button
        class="nudge left"
        class:shown={canLeft}
        aria-label="Scroll tabs left"
        onclick={() => nudge(-1)}
    >
        <Icon name="chevron-left" size={14} />
    </button>
    <button
        class="nudge right"
        class:shown={canRight}
        aria-label="Scroll tabs right"
        onclick={() => nudge(1)}
    >
        <Icon name="chevron-right" size={14} />
    </button>
</div>

{#if menu}
    {@const scoped = showContext}
    <!-- Stops the window handler from closing the menu on the click that
         reaches an item, and keeps a right-click inside it from opening the
         browser's own menu. -->
    <div
        class="menu"
        role="menu"
        tabindex="-1"
        bind:this={menuEl}
        style:left="{menu.x}px"
        style:top="{menu.y}px"
        onclick={(e) => e.stopPropagation()}
        oncontextmenu={(e) => e.preventDefault()}
        onkeydown={onMenuKeyDown}
    >
        <button role="menuitem" onclick={() => run(() => workspace.closeTab(menu!.tabId))}>Close</button>
        <button role="menuitem" onclick={() => run(() => workspace.closeOtherTabs(menu!.tabId))}>
            Close Others
        </button>
        <button role="menuitem" onclick={() => run(() => workspace.closeAllTabs())}>Close All</button>

        {#if scoped}
            {@const cluster = contextName(menu.contextId)}
            <hr />
            <button role="menuitem" onclick={() => run(() => workspace.closeOtherTabs(menu!.tabId, menu!.contextId))}>
                Close Others in {cluster}
            </button>
            <button role="menuitem" onclick={() => run(() => workspace.closeAllTabs(menu!.contextId))}>
                Close All in {cluster}
            </button>
        {/if}
    </div>
{/if}

<style>
    .menu {
        position: fixed;
        z-index: 50;
        min-width: 170px;
        padding: 4px;
        display: flex;
        flex-direction: column;
        background: var(--bg-sidebar);
        border: 1px solid var(--border);
        border-radius: var(--radius);
        box-shadow: 0 10px 28px rgb(0 0 0 / 0.45);
    }

    .menu button {
        font: inherit;
        font-size: 12px;
        text-align: left;
        padding: 6px 10px;
        border-radius: var(--radius-sm);
        color: var(--text);
        white-space: nowrap;
    }

    .menu button:hover,
    .menu button:focus-visible {
        background: var(--bg-hover);
        outline: none;
    }

    .menu hr {
        border: 0;
        border-top: 1px solid var(--border);
        margin: 4px 6px;
    }

    /* The strip owns the width and the edge affordances; .tabbar inside it is
       purely the scrolling viewport. */
    .strip {
        position: relative;
        display: flex;
        flex: 0 0 auto;
        min-width: 0;
        background: var(--bg-sidebar);
        border-bottom: 1px solid var(--border);
    }

    .tabbar {
        display: flex;
        align-items: flex-end;
        gap: 2px;
        height: 38px;
        padding: 0 6px;
        flex: 1 1 auto;
        min-width: 0;
        overflow-x: auto;
        overflow-y: hidden;
        scrollbar-width: none;
        scroll-behavior: smooth;
        /* So scrolling a tab into view stops at the strip's padding rather than
           at the tab's own edge, which would leave it flush against the arrow
           and the scroll offset a few pixels short of the true end. */
        scroll-padding: 0 6px;
    }

    .nudge {
        position: absolute;
        top: 0;
        bottom: 1px;
        width: 30px;
        display: grid;
        place-items: center;
        color: var(--text-dim);
        /* visibility, not opacity alone: a transparent button is still in the
           accessibility tree and still a click target. */
        visibility: hidden;
        opacity: 0;
        transition:
            opacity 120ms ease,
            visibility 120ms ease;
        z-index: 2;
    }

    .nudge.shown {
        visibility: visible;
        opacity: 1;
    }

    .nudge:hover {
        color: var(--text);
    }

    /* The fade is on the button itself, so there is one element per edge and
       nothing to keep in sync when it appears. */
    .nudge.left {
        left: 0;
        justify-items: start;
        padding-left: 4px;
        background: linear-gradient(to right, var(--bg-sidebar) 55%, transparent);
    }

    .nudge.right {
        right: 0;
        justify-items: end;
        padding-right: 4px;
        background: linear-gradient(to left, var(--bg-sidebar) 55%, transparent);
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
