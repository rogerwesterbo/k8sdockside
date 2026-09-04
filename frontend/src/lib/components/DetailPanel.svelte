<!--
  The describe panel. It slides in from whichever edge it is docked to when a
  row is selected, and the user can move it to another edge or resize it; both
  choices are persisted.
-->
<script lang="ts">
    import { fly } from 'svelte/transition';
    import { singularFor } from '../catalogue';
    import { alpha } from '../colors';
    import { changes } from '../state/changes.svelte';
    import { workspace, type DockSide } from '../state/workspace.svelte';
    import ErrorState from './ErrorState.svelte';
    import Icon from './Icon.svelte';
    import ObjectActions from './ObjectActions.svelte';

    const DOCKS: { side: DockSide; icon: string; label: string }[] = [
        { side: 'left', icon: 'dock-left', label: 'Dock left' },
        { side: 'bottom', icon: 'dock-bottom', label: 'Dock bottom' },
        { side: 'right', icon: 'dock-right', label: 'Dock right' },
    ];

    const MIN_SIZE = 260;

    let target = $derived(workspace.detailTarget);
    let dock = $derived(workspace.dock);
    let color = $derived(target ? workspace.colorOf(target.contextId) : 'var(--accent)');
    /**
     * Keeps the report level with the object.
     *
     * The object can be written from somewhere else while this panel is open --
     * the editor in the dock is the usual place, and it is often open on
     * exactly what is being described here. Reading the object's revision is
     * the subscription; when it has moved past the one the report was read at,
     * the panel reads again.
     */
    $effect(() => {
        if (target && changes.revision(target) !== workspace.detailRevision) {
            void workspace.refreshDetail();
        }
    });

    let panel = $state<HTMLElement | null>(null);
    let resizing = $state(false);

    /** The direction the panel flies in from, matching the edge it is docked to. */
    let flyFrom = $derived(
        dock === 'bottom'
            ? { y: 24, x: 0 }
            : { x: dock === 'right' ? 24 : -24, y: 0 },
    );

    function startResize(event: PointerEvent): void {
        if (!panel) return;
        event.preventDefault();
        resizing = true;
        (event.currentTarget as HTMLElement).setPointerCapture(event.pointerId);
    }

    function onResize(event: PointerEvent): void {
        if (!resizing || !panel) return;
        const rect = panel.getBoundingClientRect();
        // Measure from the panel's own outer edge, so the maths is the same
        // wherever the stage happens to sit in the window.
        const size =
            dock === 'right'
                ? rect.right - event.clientX
                : dock === 'left'
                  ? event.clientX - rect.left
                  : rect.bottom - event.clientY;

        const limit = dock === 'bottom' ? window.innerHeight - 160 : window.innerWidth - 420;
        workspace.setDetailSize(Math.max(MIN_SIZE, Math.min(size, limit)));
    }

    function endResize(event: PointerEvent): void {
        resizing = false;
        (event.currentTarget as HTMLElement).releasePointerCapture(event.pointerId);
    }

    /** Arrow keys resize the panel for anyone not using a pointer. */
    function onHandleKey(event: KeyboardEvent): void {
        const step = event.shiftKey ? 48 : 16;
        const grow = dock === 'bottom' ? 'ArrowUp' : dock === 'right' ? 'ArrowLeft' : 'ArrowRight';
        const shrink = dock === 'bottom' ? 'ArrowDown' : dock === 'right' ? 'ArrowRight' : 'ArrowLeft';

        if (event.key === grow) {
            event.preventDefault();
            workspace.setDetailSize(workspace.detailSize + step);
        } else if (event.key === shrink) {
            event.preventDefault();
            workspace.setDetailSize(Math.max(MIN_SIZE, workspace.detailSize - step));
        }
    }
</script>

<svelte:window
    onkeydown={(e) => {
        if (e.key === 'Escape' && workspace.detailTarget) workspace.closeDetail();
    }}
/>

{#if target}
    <aside
        class="panel {dock}"
        bind:this={panel}
        style:--ctx-color={color}
        style:--ctx-tint={alpha(color, 0.12)}
        style:--size="{workspace.detailSize}px"
        transition:fly={{ ...flyFrom, duration: 170 }}
        aria-label="{singularFor(target.kind)} details"
    >
        <!-- A focusable separator is the ARIA "window splitter" pattern; the
             a11y rules below only key off the role, which they treat as static. -->
        <!-- svelte-ignore a11y_no_noninteractive_tabindex -->
        <!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
        <div
            class="handle"
            class:active={resizing}
            role="separator"
            aria-label="Resize details panel"
            aria-orientation={dock === 'bottom' ? 'horizontal' : 'vertical'}
            aria-valuenow={workspace.detailSize}
            aria-valuemin={MIN_SIZE}
            tabindex="0"
            onpointerdown={startResize}
            onpointermove={onResize}
            onpointerup={endResize}
            onpointercancel={endResize}
            onkeydown={onHandleKey}
        ></div>

        <header>
            <div class="ident">
                <span class="kind">{singularFor(target.kind)}</span>
                <h2 class="selectable">{target.name}</h2>
                {#if target.namespace}
                    <span class="ns">in {target.namespace}</span>
                {/if}
            </div>

            <div class="controls">
                <div class="docks" role="group" aria-label="Panel position">
                    {#each DOCKS as option (option.side)}
                        <button
                            class:on={dock === option.side}
                            title={option.label}
                            aria-label={option.label}
                            aria-pressed={dock === option.side}
                            onclick={() => workspace.setDock(option.side)}
                        >
                            <Icon name={option.icon} size={14} />
                        </button>
                    {/each}
                </div>
                <button class="close" onclick={() => workspace.closeDetail()} title="Close (Esc)" aria-label="Close details">
                    <Icon name="close" size={15} />
                </button>
            </div>
        </header>

        <!-- What can be done to the object, under the line that says what it
             is. Editing lives here too: it is an action like the rest, and the
             header was doing identity, editing and window management at once. -->
        <ObjectActions object={target} />

        <div class="body">
            {#if workspace.detailLoading}
                <p class="status">Describing {target.name}…</p>
            {:else if workspace.detailError}
                <ErrorState message={workspace.detailError} compact />
            {:else}
                <pre class="selectable">{workspace.detailText}</pre>
            {/if}
        </div>
    </aside>
{/if}

<style>
    .panel {
        position: relative;
        display: flex;
        flex-direction: column;
        min-width: 0;
        min-height: 0;
        background: var(--bg-panel);
        flex: 0 0 auto;
    }

    .panel.right {
        width: var(--size);
        border-left: 1px solid var(--border);
    }

    .panel.left {
        width: var(--size);
        border-right: 1px solid var(--border);
        order: -1;
    }

    .panel.bottom {
        height: var(--size);
        border-top: 1px solid var(--border);
    }

    /* The grab strip sits just outside the panel's content, on the edge that
       faces the resource table. */
    .handle {
        position: absolute;
        z-index: 2;
        background: transparent;
        transition: background 120ms ease;
    }

    .panel.right .handle,
    .panel.left .handle {
        top: 0;
        bottom: 0;
        width: 7px;
        cursor: col-resize;
    }

    .panel.right .handle {
        left: -3px;
    }

    .panel.left .handle {
        right: -3px;
    }

    .panel.bottom .handle {
        left: 0;
        right: 0;
        height: 7px;
        top: -3px;
        cursor: row-resize;
    }

    .handle:hover,
    .handle.active,
    .handle:focus-visible {
        background: var(--ctx-color);
    }

    header {
        display: flex;
        align-items: flex-start;
        gap: 12px;
        padding: 10px 12px 10px 14px;
        border-bottom: 1px solid var(--border);
        background: var(--ctx-tint);
        border-left: 3px solid var(--ctx-color);
        flex: 0 0 auto;
    }

    .ident {
        min-width: 0;
        flex: 1 1 auto;
    }

    .kind {
        display: block;
        font-size: 10px;
        letter-spacing: 0.08em;
        text-transform: uppercase;
        color: var(--text-faint);
    }

    h2 {
        margin: 2px 0 0;
        font-size: 14px;
        font-weight: 600;
        overflow-wrap: anywhere;
    }

    .ns {
        font-size: 11px;
        color: var(--text-dim);
    }

    .controls {
        display: flex;
        align-items: center;
        gap: 8px;
        flex: 0 0 auto;
    }

    .docks {
        display: flex;
        gap: 1px;
        background: var(--bg);
        border: 1px solid var(--border);
        border-radius: var(--radius-sm);
        padding: 1px;
    }

    .docks button {
        display: grid;
        place-items: center;
        width: 22px;
        height: 20px;
        border-radius: 3px;
        color: var(--text-faint);
    }

    .docks button:hover {
        color: var(--text);
        background: var(--bg-hover);
    }

    .docks button.on {
        color: var(--text);
        background: var(--bg-active);
    }

    .close {
        display: grid;
        place-items: center;
        width: 24px;
        height: 24px;
        border-radius: var(--radius-sm);
        color: var(--text-dim);
    }

    .close:hover {
        background: var(--bg-hover);
        color: var(--text);
    }

    .body {
        flex: 1 1 auto;
        overflow: auto;
        min-height: 0;
    }

    pre {
        margin: 0;
        padding: 14px 16px;
        font-family: var(--mono);
        font-size: 11.5px;
        line-height: 1.65;
        color: var(--text-dim);
        white-space: pre;
    }

    .status {
        display: flex;
        align-items: center;
        gap: 8px;
        padding: 18px 16px;
        color: var(--text-dim);
    }

    .status.error {
        color: var(--error);
    }
</style>
