<!--
  An on/off switch. A real checkbox underneath, so it is reachable by keyboard
  and announced as a checkbox, with the track and knob drawn over it.
-->
<script lang="ts">
    interface Props {
        checked: boolean;
        label: string;
        onchange: (value: boolean) => void;
    }

    let { checked, label, onchange }: Props = $props();
</script>

<label class="toggle">
    <input
        type="checkbox"
        {checked}
        aria-label={label}
        onchange={(e) => onchange((e.currentTarget as HTMLInputElement).checked)}
    />
    <span class="track"><span class="knob"></span></span>
</label>

<style>
    .toggle {
        position: relative;
        display: inline-flex;
        flex: 0 0 auto;
        cursor: pointer;
    }

    input {
        position: absolute;
        inset: 0;
        margin: 0;
        opacity: 0;
        cursor: pointer;
    }

    .track {
        display: block;
        width: 36px;
        height: 20px;
        border-radius: 999px;
        background: var(--bg-raised);
        box-shadow: inset 0 0 0 1px var(--border);
        transition: background 120ms ease;
    }

    .knob {
        display: block;
        width: 14px;
        height: 14px;
        margin: 3px;
        border-radius: 50%;
        background: var(--text-faint);
        transition:
            transform 120ms ease,
            background 120ms ease;
    }

    input:checked + .track {
        background: var(--accent);
        box-shadow: none;
    }

    input:checked + .track .knob {
        transform: translateX(16px);
        background: var(--accent-text);
    }

    input:focus-visible + .track {
        outline: 2px solid var(--accent);
        outline-offset: 2px;
    }
</style>
