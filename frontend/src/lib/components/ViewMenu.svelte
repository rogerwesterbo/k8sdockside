<!--
  The View menu in the title bar.

  It exists because every panel can be hidden and one of them -- the cluster
  tree -- is how everything else gets opened. A keyboard shortcut is not an
  answer to "it has disappeared and I do not know why": the way back has to be
  something you can see. So the panels are listed here with a tick beside the
  ones that are showing, and "Reset layout" is underneath for an arrangement
  that has gone wrong in a way no single toggle undoes.

  Settings is here too, for the same reason. Its other route in is a button in
  the cluster tree, which is exactly the thing that may be hidden.

  Drawn by the app rather than by the platform: this window wears its own title
  bar, so a native menu would be a second strip of chrome above it on Windows
  and Linux with a different look from everything below. What is here instead
  renders the same everywhere and takes the user's theme.
-->
<script lang="ts">
    import { PANE_LABELS, workspace, type PaneId } from '../state/workspace.svelte';
    import Icon from './Icon.svelte';

    let open = $state(false);
    let menuEl = $state<HTMLElement | null>(null);
    let buttonEl = $state<HTMLButtonElement | null>(null);

    /** The modifier as this platform writes it, since the point is to be read. */
    const MOD = navigator.platform.startsWith('Mac') ? '⌘' : 'Ctrl+';

    /**
     * The panels, in the order they appear on screen from left to right.
     *
     * Main is not here: it is what the window is, and the others are arranged
     * around it. There is nothing to show or hide.
     */
    const PANELS: { pane: PaneId; shortcut?: string }[] = [
        { pane: 'left', shortcut: `${MOD}B` },
        { pane: 'right' },
        { pane: 'bottom' },
    ];

    /**
     * What the left panel is called here.
     *
     * "Explorer" rather than "Left panel" when it is holding the cluster tree,
     * because that is what somebody looking for the missing thing is looking
     * for -- and it stops being true the moment the tree is dragged elsewhere.
     */
    function labelFor(pane: PaneId): string {
        const holdsTree = workspace.paneOf(workspace.CLUSTERS_TAB_ID) === pane;
        return holdsTree ? `${PANE_LABELS[pane]} (Explorer)` : PANE_LABELS[pane];
    }

    /** A panel with nothing in it has nothing to show, so it cannot be toggled. */
    function isEmpty(pane: PaneId): boolean {
        return workspace.panes[pane].tabs.length === 0;
    }

    function close(): void {
        open = false;
    }

    function run(action: () => void): void {
        action();
        close();
        // Back to the button, so the keyboard is where it was before the menu.
        buttonEl?.focus();
    }

    function onKeyDown(event: KeyboardEvent): void {
        if (event.key === 'Escape') {
            event.stopPropagation();
            close();
            buttonEl?.focus();
            return;
        }
        if (event.key !== 'ArrowDown' && event.key !== 'ArrowUp') return;

        event.preventDefault();
        const items = [...(menuEl?.querySelectorAll('button:not(:disabled)') ?? [])];
        const at = items.indexOf(document.activeElement as HTMLButtonElement);
        const next = (at + (event.key === 'ArrowDown' ? 1 : -1) + items.length) % items.length;
        (items[next] as HTMLButtonElement | undefined)?.focus();
    }

    // Focus the first item, so the menu is usable without the mouse that opened it.
    $effect(() => {
        if (open && menuEl) menuEl.querySelector<HTMLButtonElement>('button:not(:disabled)')?.focus();
    });
</script>

<svelte:window onclick={close} onresize={close} />

<!-- The click that opens the menu must not reach the window handler that
     closes it again, and neither must a click on an item inside it. This
     wrapper is a bystander in that: everything inside it is a button, so
     there is nothing here for a keyboard to need a handler for. -->
<!-- svelte-ignore a11y_no_static_element_interactions -->
<!-- svelte-ignore a11y_click_events_have_key_events -->
<div class="host" onclick={(e) => e.stopPropagation()}>
    <button
        class="trigger"
        bind:this={buttonEl}
        aria-haspopup="menu"
        aria-expanded={open}
        onclick={() => (open = !open)}
    >
        View
        <Icon name="chevron-down" size={12} />
    </button>

    {#if open}
        <!-- A menu is not focusable itself -- its items are, and the effect
             below puts the focus on one as it opens -- so the rule asking for a
             tabindex here does not apply. -->
        <!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
        <!-- svelte-ignore a11y_interactive_supports_focus -->
        <div class="menu" role="menu" aria-label="View" bind:this={menuEl} onkeydown={onKeyDown}>
            {#each PANELS as panel (panel.pane)}
                {@const shown = workspace.isPaneOpen(panel.pane)}
                {@const empty = isEmpty(panel.pane)}
                <button
                    role="menuitemcheckbox"
                    aria-checked={shown}
                    disabled={empty}
                    title={empty ? 'Nothing is open in this panel' : undefined}
                    onclick={() => run(() => workspace.togglePane(panel.pane))}
                >
                    <span class="tick">{#if shown}<Icon name="check" size={13} />{/if}</span>
                    <span class="label">{labelFor(panel.pane)}</span>
                    {#if panel.shortcut}<span class="key">{panel.shortcut}</span>{/if}
                </button>
            {/each}

            <hr />

            <button role="menuitem" onclick={() => run(() => workspace.resetLayout())}>
                <span class="tick"></span>
                <span class="label">Reset layout</span>
            </button>

            <hr />

            <button role="menuitem" onclick={() => run(() => workspace.openHelp())}>
                <span class="tick"></span>
                <span class="label">Help</span>
                <span class="key">F1</span>
            </button>
            <button role="menuitem" onclick={() => run(() => workspace.openKubernetesPrimer())}>
                <span class="tick"></span>
                <span class="label">Kubernetes primer</span>
            </button>
            <button role="menuitem" onclick={() => run(() => workspace.openSettings())}>
                <span class="tick"></span>
                <span class="label">Settings</span>
                <span class="key">{MOD},</span>
            </button>
        </div>
    {/if}
</div>

<style>
    .host {
        position: relative;
        display: flex;
        align-items: center;
        /* The title bar drags the window; a menu in it must not. */
        --wails-draggable: no-drag;
        z-index: 5;
    }

    .trigger {
        display: flex;
        align-items: center;
        gap: 3px;
        height: 24px;
        padding: 0 7px 0 9px;
        border-radius: var(--radius-sm);
        font-size: 12px;
        color: var(--text-dim);
    }

    .trigger:hover,
    .trigger[aria-expanded='true'] {
        background: var(--bg-hover);
        color: var(--text);
    }

    .menu {
        position: absolute;
        top: calc(100% + 4px);
        /* Anchored to the trigger's right edge, not its left. The trigger is
           at the right end of the title bar -- the left of that bar belongs to
           the macOS traffic lights and the middle to the title -- so a menu
           growing rightwards from it grows off the edge of the window and
           takes half of every label with it. */
        right: 0;
        min-width: 230px;
        padding: 4px;
        border: 1px solid var(--border);
        border-radius: var(--radius);
        background: var(--bg-raised);
        box-shadow: 0 8px 24px rgb(0 0 0 / 0.35);
    }

    .menu button {
        display: flex;
        align-items: center;
        gap: 8px;
        width: 100%;
        height: 26px;
        padding: 0 8px;
        border-radius: var(--radius-sm);
        font-size: 12px;
        color: var(--text);
        text-align: left;
    }

    .menu button:hover:not(:disabled),
    .menu button:focus-visible {
        background: var(--bg-hover);
    }

    .menu button:disabled {
        opacity: 0.4;
        cursor: default;
    }

    /* A fixed column for the tick, so the labels line up whether or not one is
       there and nothing shifts sideways as a panel is toggled. */
    .tick {
        display: grid;
        place-items: center;
        width: 14px;
        flex: 0 0 auto;
        color: var(--accent);
    }

    .label {
        flex: 1 1 auto;
    }

    .key {
        flex: 0 0 auto;
        font-size: 11px;
        color: var(--text-faint);
    }

    hr {
        height: 1px;
        margin: 4px 6px;
        border: 0;
        background: var(--border-soft);
    }
</style>
