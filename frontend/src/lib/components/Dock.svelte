<!--
  The dock at the foot of the window: a strip of tabs that is always there, and
  under it whatever the selected one is showing -- an object's YAML, its logs,
  or a shell running in it.

  It belongs to the window rather than to the view above it. Switching clusters,
  opening another tab or closing every tab there is leaves the dock exactly as
  it was, because what is in it is a document you are part way through, and the
  one thing that must not lose an edit is looking at something else for a
  moment.

  The strip stays on screen even with nothing open. That is the difference
  between a dock and a panel: it is a place things go, and a place has to be
  visible before you can put anything in it.
-->
<script lang="ts">
    import { singularFor } from '../catalogue';
    import { editors } from '../state/editor.svelte';
    import { isDocumentView, workspace } from '../state/workspace.svelte';
    import Icon from './Icon.svelte';
    import LogView from './LogView.svelte';
    import TabStrip, { type StripTab } from './TabStrip.svelte';
    import TerminalView from './TerminalView.svelte';
    import YamlEditor from './YamlEditor.svelte';

    /** Below this the editor shows its toolbar and three lines, which is not an editor. */
    const MIN_SIZE = 160;
    /** Room the view above must keep, however far the dock is dragged. */
    const HEADROOM = 220;

    let active = $derived(workspace.activeDockTab);
    let open = $derived(workspace.dockOpen && active !== null);

    // Every dock tab names its cluster, unlike the strip above: an object's
    // name says nothing about which cluster it is in, and "edit the wrong
    // cluster's ingress" is the mistake this whole feature could cause.
    function contextName(contextId: string): string {
        const context = workspace.contexts.find((c) => c.id === contextId);
        return context ? workspace.displayName(context) : contextId;
    }

    let tabs = $derived(
        workspace.dockTabs.map(
            (tab): StripTab => ({
                id: tab.id,
                title: tab.title,
                subtitle: contextName(tab.contextId),
                icon:
                    tab.view === 'logs'
                        ? 'rows'
                        : tab.view === 'shell'
                          ? 'terminal'
                          : tab.view === 'helmvalues'
                            ? 'helm'
                            : 'edit',
                color: workspace.colorOf(tab.contextId),
                hint: `${singularFor(tab.kind)} ${tab.name}${tab.namespace ? ` in ${tab.namespace}` : ''} — ${contextName(tab.contextId)}`,
                // Only an editor has unsaved work; closing a log view loses
                // nothing that was not already in the cluster.
                modified: isDocumentView(tab.view) && editors.isDirty(tab.id),
            }),
        ),
    );

    /** Whether any other cluster has a tab here, which is what the scoped menu items are for. */
    let severalClusters = $derived(new Set(workspace.dockTabs.map((t) => t.contextId)).size > 1);

    function run(action: () => void, dismiss: () => void): void {
        action();
        dismiss();
    }

    let resizing = $state(false);
    let dockEl = $state<HTMLElement | null>(null);

    function startResize(event: PointerEvent): void {
        event.preventDefault();
        resizing = true;
        (event.currentTarget as HTMLElement).setPointerCapture(event.pointerId);
    }

    function onResize(event: PointerEvent): void {
        if (!resizing || !dockEl) return;
        // Measured from the dock's own bottom edge, so the arithmetic does not
        // depend on where the status bar or the window happens to be.
        const size = dockEl.getBoundingClientRect().bottom - event.clientY;
        workspace.setDockSize(Math.max(MIN_SIZE, Math.min(size, window.innerHeight - HEADROOM)));
    }

    function endResize(event: PointerEvent): void {
        resizing = false;
        (event.currentTarget as HTMLElement).releasePointerCapture(event.pointerId);
    }

    /** Arrow keys resize the dock for anyone not using a pointer. */
    function onHandleKey(event: KeyboardEvent): void {
        const step = event.shiftKey ? 48 : 16;
        if (event.key === 'ArrowUp') {
            event.preventDefault();
            workspace.setDockSize(Math.min(workspace.dockSize + step, window.innerHeight - HEADROOM));
        } else if (event.key === 'ArrowDown') {
            event.preventDefault();
            workspace.setDockSize(Math.max(MIN_SIZE, workspace.dockSize - step));
        }
    }
</script>

<section class="dock" class:open bind:this={dockEl} style:--size="{workspace.dockSize}px">
    {#if open}
        <!-- A focusable separator is the ARIA "window splitter" pattern; the
             a11y rules below only key off the role, which they treat as static. -->
        <!-- svelte-ignore a11y_no_noninteractive_tabindex -->
        <!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
        <div
            class="handle"
            class:active={resizing}
            role="separator"
            aria-label="Resize the dock"
            aria-orientation="horizontal"
            aria-valuenow={workspace.dockSize}
            aria-valuemin={MIN_SIZE}
            tabindex="0"
            onpointerdown={startResize}
            onpointermove={onResize}
            onpointerup={endResize}
            onpointercancel={endResize}
            onkeydown={onHandleKey}
        ></div>
    {/if}

    <TabStrip
        {tabs}
        activeId={workspace.activeDockTabId}
        label="Dock"
        rule="above"
        empty="Select a resource, then Edit, Logs or Shell in the details panel to open it here."
        onactivate={(id) => workspace.activateDockTab(id)}
        onclose={(id) => workspace.closeDockTab(id)}
        onmove={(from, to) => workspace.moveDockTab(from, to)}
        menu={dockMenu}
        trailing={controls}
    />

    {#if open && active}
        <div class="body">
            {#key active.id}
                {#if active.view === 'logs'}
                    <LogView tab={active} />
                {:else if active.view === 'shell'}
                    <TerminalView tab={active} />
                {:else}
                    <YamlEditor tab={active} />
                {/if}
            {/key}
        </div>
    {/if}
</section>

{#snippet controls()}
    <button
        class="fold"
        onclick={() => workspace.toggleDock()}
        disabled={workspace.dockTabs.length === 0}
        aria-expanded={open}
        title={open ? 'Hide the dock' : 'Show the dock'}
        aria-label={open ? 'Hide the dock' : 'Show the dock'}
    >
        <Icon name={open ? 'chevron-down' : 'chevron-up'} size={15} />
    </button>
{/snippet}

{#snippet dockMenu(tab: StripTab, dismiss: () => void)}
    {@const contextId = workspace.dockTabs.find((t) => t.id === tab.id)?.contextId ?? ''}
    <button role="menuitem" onclick={() => run(() => workspace.closeDockTab(tab.id), dismiss)}>Close</button>
    <button role="menuitem" onclick={() => run(() => workspace.closeOtherDockTabs(tab.id), dismiss)}>
        Close Others
    </button>
    <button role="menuitem" onclick={() => run(() => workspace.closeAllDockTabs(), dismiss)}>Close All</button>

    {#if severalClusters}
        {@const cluster = contextName(contextId)}
        <hr />
        <button role="menuitem" onclick={() => run(() => workspace.closeOtherDockTabs(tab.id, contextId), dismiss)}>
            Close Others in {cluster}
        </button>
        <button role="menuitem" onclick={() => run(() => workspace.closeAllDockTabs(contextId), dismiss)}>
            Close All in {cluster}
        </button>
    {/if}
{/snippet}

<style>
    .dock {
        position: relative;
        display: flex;
        flex-direction: column;
        flex: 0 0 auto;
        min-height: 0;
        background: var(--bg-panel);
    }

    /* Only the open dock has a height of its own; closed, it is its strip. */
    .dock.open {
        height: var(--size);
    }

    /* The grab strip sits over the seam between the dock and the view above,
       so the whole edge is grabbable rather than the one pixel of the rule. */
    .handle {
        position: absolute;
        left: 0;
        right: 0;
        top: -3px;
        height: 7px;
        z-index: 4;
        cursor: row-resize;
        background: transparent;
        transition: background 120ms ease;
    }

    .handle:hover,
    .handle.active,
    .handle:focus-visible {
        background: var(--accent);
    }

    .body {
        flex: 1 1 auto;
        min-height: 0;
        overflow: hidden;
    }

    .fold {
        display: grid;
        place-items: center;
        width: 26px;
        height: 26px;
        border-radius: var(--radius-sm);
        color: var(--text-dim);
    }

    .fold:hover:not(:disabled) {
        background: var(--bg-hover);
        color: var(--text);
    }

    .fold:disabled {
        opacity: 0.35;
        cursor: default;
    }
</style>
