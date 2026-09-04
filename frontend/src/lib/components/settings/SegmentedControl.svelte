<!--
  A row of mutually exclusive choices, for the settings with two or three of
  them: the theme, the density, the dock edge. A radio group underneath, so
  arrow keys move between the options and only the selected one is a tab stop.
-->
<script lang="ts">
    import Icon from '../Icon.svelte';

    interface Option {
        value: string;
        label: string;
        icon?: string;
    }

    interface Props {
        options: Option[];
        value: string;
        /** Announced for the group as a whole, since the options only carry their own labels. */
        label: string;
        onchange: (value: string) => void;
    }

    let { options, value, label, onchange }: Props = $props();

    /**
     * Arrow keys move the selection, which is how a radio group behaves: with
     * roving tabindex only the selected option is a tab stop, so without this
     * the others could not be reached from the keyboard at all.
     */
    function onKey(event: KeyboardEvent): void {
        const step =
            event.key === 'ArrowRight' || event.key === 'ArrowDown'
                ? 1
                : event.key === 'ArrowLeft' || event.key === 'ArrowUp'
                  ? -1
                  : 0;
        if (step === 0) return;

        event.preventDefault();
        const at = options.findIndex((o) => o.value === value);
        const next = options[(at + step + options.length) % options.length];
        onchange(next.value);

        // Focus follows selection, as in a radio group. The DOM has not been
        // updated yet, so the move waits for the render that the change causes.
        const group = (event.currentTarget as HTMLElement).parentElement;
        queueMicrotask(() => group?.querySelector<HTMLElement>('[aria-checked="true"]')?.focus());
    }
</script>

<div class="segmented" role="radiogroup" aria-label={label}>
    {#each options as option (option.value)}
        <button
            role="radio"
            aria-checked={value === option.value}
            class:selected={value === option.value}
            tabindex={value === option.value ? 0 : -1}
            onclick={() => onchange(option.value)}
            onkeydown={onKey}
        >
            {#if option.icon}<Icon name={option.icon} size={14} />{/if}
            {option.label}
        </button>
    {/each}
</div>

<style>
    .segmented {
        display: inline-flex;
        padding: 2px;
        border-radius: var(--radius);
        background: var(--bg-raised);
        box-shadow: inset 0 0 0 1px var(--border);
    }

    button {
        display: flex;
        align-items: center;
        gap: 5px;
        padding: 5px 11px;
        border-radius: var(--radius-sm);
        font-size: 12px;
        color: var(--text-dim);
        white-space: nowrap;
    }

    button:hover:not(.selected) {
        color: var(--text);
    }

    button.selected {
        background: var(--bg-panel);
        color: var(--text);
        box-shadow: 0 1px 2px rgb(0 0 0 / 0.25);
    }

    button:focus-visible {
        outline: 2px solid var(--accent);
        outline-offset: -1px;
    }
</style>
