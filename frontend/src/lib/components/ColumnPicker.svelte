<!--
  Which columns a table shows, and the way back from having changed them.

  Built like the namespace filter beside it in the same toolbar: a trigger, a
  panel under it that closes on a click anywhere else or on Escape, and arrow
  keys between the rows. It stays open while columns are ticked, since ticking
  three is the point.

  The last shown column cannot be unticked. A table of nothing is a stack of
  blank rows, and the only control that would bring the columns back is this
  panel -- which by then lists nothing to tick.
-->
<script lang="ts">
    import type { Column } from '../columns';
    import Icon from './Icon.svelte';

    interface Props {
        /** Every column the backend sent, keyed -- see ../columns.ts. */
        columns: Column[];
        /** The ones turned off, by column key. */
        hidden: string[];
        /** Whether any column has been dragged to a width, which Reset undoes. */
        resized: boolean;
        ontoggle: (key: string, hidden: boolean) => void;
        /** Ticks every column again, leaving the widths as they are. */
        onshowall: () => void;
        /** Puts the table back to how it arrives: every column, natural widths. */
        onreset: () => void;
    }

    let { columns, hidden, resized, ontoggle, onshowall, onreset }: Props = $props();

    let open = $state(false);
    let menuEl = $state<HTMLElement | null>(null);
    let buttonEl = $state<HTMLButtonElement | null>(null);

    let off = $derived(new Set(hidden));
    let showing = $derived(columns.filter((column) => !off.has(column.key)));
    /** The one column that may not be unticked, when it is the only one left. */
    let last = $derived(showing.length === 1 ? showing[0].key : null);

    /** What the trigger reads: silent until the user has actually changed something. */
    let summary = $derived(
        showing.length === columns.length
            ? 'Columns'
            : `${showing.length} of ${columns.length}`,
    );

    function toggle(column: Column): void {
        if (column.key === last) return;
        ontoggle(column.key, !off.has(column.key));
    }

    function onKeyDown(event: KeyboardEvent): void {
        if (event.key === 'Escape') {
            event.stopPropagation();
            open = false;
            buttonEl?.focus();
            return;
        }
        if (event.key !== 'ArrowDown' && event.key !== 'ArrowUp') return;

        event.preventDefault();
        const items = [...(menuEl?.querySelectorAll<HTMLElement>('button:not([disabled])') ?? [])];
        const at = items.indexOf(document.activeElement as HTMLElement);
        const next = (at + (event.key === 'ArrowDown' ? 1 : -1) + items.length) % items.length;
        items[next]?.focus();
    }

    // Focus goes into the panel, so it is usable without the mouse that opened it.
    $effect(() => {
        if (!open || !menuEl) return;
        menuEl.querySelector<HTMLElement>('button')?.focus();
    });
</script>

<svelte:window onclick={() => (open = false)} onresize={() => (open = false)} />

<!-- The click that opens the panel must not reach the window handler that
     closes it again, and neither must a click on a row inside it. -->
<!-- svelte-ignore a11y_no_static_element_interactions -->
<!-- svelte-ignore a11y_click_events_have_key_events -->
<div class="host" onclick={(e) => e.stopPropagation()}>
    <button
        class="trigger"
        class:changed={showing.length !== columns.length || resized}
        bind:this={buttonEl}
        aria-haspopup="menu"
        aria-expanded={open}
        title="Which columns to show. Drag a column's edge to resize it; both are remembered for this kind in this cluster."
        onclick={() => (open = !open)}
    >
        <Icon name="columns" size={13} />
        <span>{summary}</span>
    </button>

    {#if open}
        <!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
        <!-- svelte-ignore a11y_interactive_supports_focus -->
        <div class="menu" role="menu" aria-label="Columns" bind:this={menuEl} onkeydown={onKeyDown}>
            <div class="list">
                <!-- Unkeyed, by position: two columns can share a name, and a
                     keyed block throws on the repeat. -->
                {#each columns as column}
                    {@const on = !off.has(column.key)}
                    <button
                        role="menuitemcheckbox"
                        aria-checked={on}
                        disabled={column.key === last}
                        title={column.key === last ? 'A table needs at least one column' : ''}
                        onclick={() => toggle(column)}
                    >
                        <span class="tick">{#if on}<Icon name="check" size={13} />{/if}</span>
                        <span class="label">{column.name}</span>
                    </button>
                {/each}
            </div>

            <hr />

            <button class="plain" disabled={showing.length === columns.length} onclick={onshowall}>
                <span class="tick"></span>
                <span class="label">Show all columns</span>
            </button>
            <button
                class="plain"
                disabled={showing.length === columns.length && !resized}
                onclick={onreset}
            >
                <span class="tick"></span>
                <span class="label">Reset widths and columns</span>
            </button>
        </div>
    {/if}
</div>

<style>
    .host {
        position: relative;
        display: flex;
        align-items: center;
        z-index: 5;
    }

    .trigger {
        display: flex;
        align-items: center;
        gap: 5px;
        height: 24px;
        padding: 0 7px;
        border-radius: var(--radius-sm);
        font-size: 11px;
        color: var(--text-dim);
        white-space: nowrap;
    }

    .trigger:hover,
    .trigger[aria-expanded='true'] {
        color: var(--text);
        background: var(--bg-hover);
    }

    /* A table that is not showing what it arrives with says so, so a column
       somebody turned off a week ago is not read as a column that never was. */
    .trigger.changed {
        color: var(--text);
    }

    .menu {
        position: absolute;
        top: calc(100% + 4px);
        right: 0;
        min-width: 220px;
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
    .menu button:focus-visible:not(:disabled) {
        background: var(--bg-hover);
    }

    .menu button:disabled {
        color: var(--text-faint);
        cursor: default;
    }

    /* A fixed column for the tick, so the names line up whether or not one is
       there and nothing shifts sideways as a row is ticked. */
    .tick {
        display: grid;
        place-items: center;
        width: 14px;
        flex: 0 0 auto;
        color: var(--accent);
    }

    .label {
        flex: 1 1 auto;
        min-width: 0;
        overflow: hidden;
        text-overflow: ellipsis;
        white-space: nowrap;
    }

    .menu .plain .label {
        color: var(--text-dim);
    }

    .menu .plain:hover:not(:disabled) .label {
        color: var(--text);
    }

    /* Long lists scroll inside the panel rather than growing past the window. */
    .list {
        max-height: 320px;
        overflow: auto;
    }

    hr {
        height: 1px;
        margin: 4px 6px;
        border: 0;
        background: var(--border-soft);
    }
</style>
