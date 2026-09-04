<!--
  A strip of tabs: the one on top of the view, and the one at the foot of the
  window that the dock hangs off. Both are the same object -- coloured by the
  cluster they belong to, dragged into order, scrolled when they no longer fit,
  and right-clicked for what to close -- so both are this component, given a
  different list.

  What it deliberately does not know is what a tab *is*. The caller hands it
  colours and titles already resolved and gets back "the user asked for this
  one", which is what lets the dock's tabs be objects while the strip above it
  holds resource kinds.
-->
<script lang="ts" module>
    /** One tab, as the strip needs it. */
    export interface StripTab {
        id: string;
        title: string;
        /** Smaller type after the title, usually the cluster. Omitted when it would only repeat. */
        subtitle?: string;
        icon: string;
        /** The colour the tab is painted with: its context's. */
        color: string;
        /** The tooltip. */
        hint?: string;
        /** Draws an unsaved-changes marker in place of the close button. */
        modified?: boolean;
    }
</script>

<script lang="ts">
    import type { Snippet } from 'svelte';
    import { alpha, textOn } from '../colors';
    import Icon from './Icon.svelte';

    interface Props {
        tabs: StripTab[];
        activeId: string | null;
        /** Names the strip for a screen reader: "Open views", "Dock". */
        label: string;
        onactivate: (id: string) => void;
        onclose: (id: string) => void;
        /** Both indices are positions in `tabs`. */
        onmove: (from: number, to: number) => void;
        /** The right-click menu's items. `dismiss` closes the menu. */
        menu?: Snippet<[StripTab, () => void]>;
        /** Controls pinned to the end of the strip, past the tabs. */
        trailing?: Snippet;
        /** Shown in place of the tabs while there are none. */
        empty?: string;
        /**
         * Which edge carries the strip's dividing line: the one facing what it
         * belongs to. The strip above a view rules below itself; the dock's,
         * at the foot of the window, rules above.
         */
        rule?: 'above' | 'below';
    }

    let {
        tabs,
        activeId,
        label,
        onactivate,
        onclose,
        onmove,
        menu,
        trailing,
        empty,
        rule = 'below',
    }: Props = $props();

    /** Index of the tab currently being dragged, or null when not dragging. */
    let dragIndex = $state<number | null>(null);

    function startDrag(event: DragEvent, index: number): void {
        dragIndex = index;
        event.dataTransfer?.setData('text/plain', tabs[index].id);
        if (event.dataTransfer) event.dataTransfer.effectAllowed = 'move';
    }

    /**
     * Reorders as the pointer passes over a neighbour, so the strip shows the
     * result directly instead of an insertion marker. The write to disk is
     * debounced by the store, so a whole drag costs one save.
     */
    function dragOver(event: DragEvent, index: number): void {
        event.preventDefault();
        if (event.dataTransfer) event.dataTransfer.dropEffect = 'move';
        if (dragIndex === null || dragIndex === index) return;
        onmove(dragIndex, index);
        dragIndex = index;
    }

    /** Alt+Arrow moves the focused tab, so reordering is not drag-only. */
    function onKeyDown(event: KeyboardEvent, index: number): void {
        if (event.altKey && (event.key === 'ArrowLeft' || event.key === 'ArrowRight')) {
            event.preventDefault();
            onmove(index, index + (event.key === 'ArrowLeft' ? -1 : 1));
            return;
        }
        if (event.key === 'Enter' || event.key === ' ') {
            event.preventDefault();
            onactivate(tabs[index].id);
            return;
        }
        if (event.key === 'ContextMenu' || (event.shiftKey && event.key === 'F10')) {
            event.preventDefault();
            openMenuFromKeyboard(event, tabs[index]);
        }
    }

    function onPointerDown(event: MouseEvent, id: string): void {
        if (event.button === 1) {
            event.preventDefault();
            onclose(id);
        }
    }

    // ----- scrolling ---------------------------------------------------
    //
    // Tabs keep their width and the strip scrolls, rather than compressing to
    // fit: a title carries the cluster and what is open, which is the pair you
    // are reading the strip to tell apart, and neither survives being squeezed
    // to a few characters.

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
        tabs.length;
        measure();
    });

    /**
     * Keep the active tab on screen. Reordering, restoring a session or closing
     * a neighbour can all leave it scrolled out of sight, and a tab you cannot
     * see is a tab you cannot tell is selected.
     */
    $effect(() => {
        const active = activeId;
        if (!active || !stripEl) return;

        const tab = stripEl.querySelector<HTMLElement>(`[data-tab-id="${CSS.escape(active)}"]`);
        // `block: nearest` keeps this from scrolling the page vertically as a
        // side effect of nudging the strip sideways.
        tab?.scrollIntoView({ behavior: 'smooth', block: 'nearest', inline: 'nearest' });
    });

    /** The tab the context menu was opened on, and where to draw it. */
    interface OpenMenu {
        tab: StripTab;
        x: number;
        y: number;
    }

    let openMenuAt = $state<OpenMenu | null>(null);
    let menuEl = $state<HTMLElement | null>(null);

    /**
     * Opens the menu on a tab, activating it first: acting on a tab whose
     * contents you cannot see is how the wrong one gets closed.
     */
    function openMenu(event: MouseEvent, tab: StripTab): void {
        if (!menu) return;
        event.preventDefault();
        // The window handler below dismisses on any right-click; without this
        // the same event would close the menu we are opening.
        event.stopPropagation();
        onactivate(tab.id);
        openMenuAt = { tab, x: event.clientX, y: event.clientY };
    }

    /** Shift+F10 and the Menu key open it from the keyboard, at the tab itself. */
    function openMenuFromKeyboard(event: KeyboardEvent, tab: StripTab): void {
        if (!menu) return;
        const box = (event.currentTarget as HTMLElement).getBoundingClientRect();
        onactivate(tab.id);
        openMenuAt = { tab, x: box.left, y: box.bottom };
    }

    function closeMenu(): void {
        openMenuAt = null;
    }

    // Keep the menu inside the window: a right-click near the right edge would
    // otherwise draw it off-screen where it cannot be read or dismissed.
    $effect(() => {
        if (!openMenuAt || !menuEl) return;
        const box = menuEl.getBoundingClientRect();
        const overflowX = box.right - window.innerWidth + 8;
        const overflowY = box.bottom - window.innerHeight + 8;
        if (overflowX > 0) openMenuAt.x -= overflowX;
        if (overflowY > 0) openMenuAt.y -= overflowY;
    });

    // Focus the first item so the menu is usable without the mouse that opened it.
    $effect(() => {
        if (openMenuAt && menuEl) menuEl.querySelector('button')?.focus();
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

<div class="strip {rule}">
    <div class="scroller">
        <!-- Before the tab list rather than inside it: a tablist holding a
             paragraph is one a screen reader announces as a tab nobody can
             reach, and an empty list would take the width and squeeze it out. -->
        {#if tabs.length === 0 && empty}
            <p class="empty">{empty}</p>
        {/if}

        <div class="tabbar" role="tablist" aria-label={label} bind:this={stripEl} onscroll={measure}>
            {#each tabs as tab, index (tab.id)}
                {@const active = activeId === tab.id}
                <div
                    class="tab"
                    data-tab-id={tab.id}
                    class:active
                    class:dragging={dragIndex === index}
                    role="tab"
                    tabindex={active ? 0 : -1}
                    aria-selected={active}
                    title={tab.hint ?? tab.title}
                    draggable="true"
                    style:--tab-bg={active ? tab.color : alpha(tab.color, 0.18)}
                    style:--tab-fg={active ? textOn(tab.color) : 'var(--text-dim)'}
                    style:--tab-rule={tab.color}
                    onclick={() => onactivate(tab.id)}
                    onmousedown={(e) => onPointerDown(e, tab.id)}
                    oncontextmenu={(e) => openMenu(e, tab)}
                    onkeydown={(e) => onKeyDown(e, index)}
                    ondragstart={(e) => startDrag(e, index)}
                    ondragover={(e) => dragOver(e, index)}
                    ondragend={() => (dragIndex = null)}
                    ondrop={(e) => e.preventDefault()}
                >
                    <Icon name={tab.icon} size={14} />
                    <span class="title">{tab.title}</span>
                    {#if tab.subtitle}
                        <span class="context">{tab.subtitle}</span>
                    {/if}
                    <button
                        class="close"
                        class:modified={tab.modified}
                        aria-label={tab.modified
                            ? `Close ${tab.title} — unsaved changes`
                            : `Close ${tab.title}`}
                        onclick={(e) => {
                            e.stopPropagation();
                            onclose(tab.id);
                        }}
                    >
                        <!-- The unsaved marker is the close button wearing a
                             dot: one control in one place, which turns back
                             into a cross under the pointer rather than moving
                             out from under it. Both are drawn and one hidden,
                             because swapping the glyph on :hover cannot be
                             done from script. -->
                        <span class="mark"><Icon name="dot" size={13} /></span>
                        <span class="cross"><Icon name="close" size={12} /></span>
                    </button>
                </div>
            {/each}
        </div>

        <!-- Overlaid rather than laid out beside the tabs, so appearing and
             disappearing does not shift them sideways under the pointer. -->
        <button class="nudge left" class:shown={canLeft} aria-label="Scroll tabs left" onclick={() => nudge(-1)}>
            <Icon name="chevron-left" size={14} />
        </button>
        <button class="nudge right" class:shown={canRight} aria-label="Scroll tabs right" onclick={() => nudge(1)}>
            <Icon name="chevron-right" size={14} />
        </button>
    </div>

    {#if trailing}
        <div class="trailing">{@render trailing()}</div>
    {/if}
</div>

{#if openMenuAt && menu}
    <!-- Stops the window handler from closing the menu on the click that
         reaches an item, and keeps a right-click inside it from opening the
         browser's own menu. -->
    <div
        class="menu"
        role="menu"
        tabindex="-1"
        bind:this={menuEl}
        style:left="{openMenuAt.x}px"
        style:top="{openMenuAt.y}px"
        onclick={(e) => e.stopPropagation()}
        oncontextmenu={(e) => e.preventDefault()}
        onkeydown={onMenuKeyDown}
    >
        {@render menu(openMenuAt.tab, closeMenu)}
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

    /* The items come from the caller as a snippet, so they are styled from
       here rather than scoped to their own component. */
    .menu :global(button) {
        font: inherit;
        font-size: 12px;
        text-align: left;
        padding: 6px 10px;
        border-radius: var(--radius-sm);
        color: var(--text);
        white-space: nowrap;
    }

    .menu :global(button:hover),
    .menu :global(button:focus-visible) {
        background: var(--bg-hover);
        outline: none;
    }

    .menu :global(hr) {
        border: 0;
        border-top: 1px solid var(--border);
        margin: 4px 6px;
    }

    /* The strip owns the width and the edge affordances; .tabbar inside it is
       purely the scrolling viewport. */
    .strip {
        display: flex;
        flex: 0 0 auto;
        min-width: 0;
        background: var(--bg-sidebar);
    }

    .strip.below {
        border-bottom: 1px solid var(--border);
    }

    .strip.above {
        border-top: 1px solid var(--border);
    }

    /* The scrolling region and its two edge buttons, which are positioned
       against it rather than against the strip -- otherwise the right-hand one
       would sit over whatever the caller pinned to the end. */
    .scroller {
        position: relative;
        display: flex;
        flex: 1 1 auto;
        min-width: 0;
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

    .trailing {
        display: flex;
        align-items: center;
        gap: 2px;
        padding: 0 6px;
        flex: 0 0 auto;
        z-index: 3;
    }

    /* A strip with nothing in it says so rather than looking broken. */
    .empty {
        margin: 0;
        align-self: center;
        padding: 0 6px;
        flex: 0 1 auto;
        font-size: 11.5px;
        color: var(--text-faint);
        white-space: nowrap;
        overflow: hidden;
        text-overflow: ellipsis;
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

    /* The two glyphs share one cell, so swapping them moves nothing. */
    .close .mark,
    .close .cross {
        grid-area: 1 / 1;
        display: grid;
        place-items: center;
    }

    .close .mark {
        display: none;
    }

    /* An unsaved document's marker is full strength -- it is information
       rather than an affordance -- and gives way to the cross the moment the
       button is aimed at. */
    .close.modified {
        opacity: 1;
    }

    .close.modified .mark {
        display: grid;
    }

    .close.modified .cross {
        display: none;
    }

    .close.modified:hover .mark {
        display: none;
    }

    .close.modified:hover .cross {
        display: grid;
    }
</style>
