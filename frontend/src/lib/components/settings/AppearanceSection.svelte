<!--
  How the app looks: theme, zoom and table density.

  Zoom is the only size control, deliberately. A separate font-size setting was
  tried and removed: it moved only the text that inherits the root size, while
  the ~85 hardcoded px sizes across the components ignored it, so it appeared to
  do almost nothing. Zoom already scales every one of them, correctly, and two
  overlapping ways to make things bigger -- one of which half-works -- is worse
  than one that does.

  Nothing here is about Kubernetes, which is why it is a section of its own.
-->
<script lang="ts">
    import { workspace } from '../../state/workspace.svelte';
    import Icon from '../Icon.svelte';
    import SegmentedControl from './SegmentedControl.svelte';
    import SettingsRow from './SettingsRow.svelte';
    import SettingsSection from './SettingsSection.svelte';
    import Toggle from './Toggle.svelte';

    const DENSITIES = [
        { value: 'comfortable', label: 'Comfortable' },
        { value: 'compact', label: 'Compact' },
    ];

    // Set by the settings view so the theme row can hand the user over to the
    // Themes section rather than describing where it is.
    let { onshowthemes }: { onshowthemes?: () => void } = $props();

    /** The scales worth one click, rather than a drag to find them. */
    const PRESETS = [75, 100, 125, 150];

    let zoomPercent = $derived(Math.round(workspace.zoom * 100));
    // Compared as percentages: the store rounds the scale to two decimals, so
    // comparing floats here would miss the ends by a rounding error.
    let atMin = $derived(zoomPercent <= Math.round(workspace.minZoom * 100));
    let atMax = $derived(zoomPercent >= Math.round(workspace.maxZoom * 100));
</script>

<SettingsSection title="Appearance">
    <SettingsRow
        label="Theme"
        hint="Which palette the app wears. There is a section of its own for it, because choosing between thirteen of them is picking from pictures rather than setting a value on a line."
    >
        <button class="jump" onclick={() => onshowthemes?.()}>
            {workspace.activeTheme?.name ?? 'Loading…'}
            <Icon name="display" size={13} />
        </button>
    </SettingsRow>

    <SettingsRow
        label="Zoom"
        hint="Scales the whole window together — text, table rows, the sidebar and the spacing between them. This is the one control for making things bigger or smaller."
    >
        <div class="zoom">
            <div class="stepper">
                <button onclick={() => workspace.zoomOut()} disabled={atMin} aria-label="Zoom out">−</button>
                <input
                    type="range"
                    min={workspace.minZoom}
                    max={workspace.maxZoom}
                    step="0.05"
                    value={workspace.zoom}
                    aria-label="Zoom level"
                    aria-valuetext="{zoomPercent} percent"
                    oninput={(e) => workspace.setZoom(Number((e.currentTarget as HTMLInputElement).value))}
                />
                <button onclick={() => workspace.zoomIn()} disabled={atMax} aria-label="Zoom in">+</button>
                <span class="value">{zoomPercent}%</span>
            </div>

            <div class="presets">
                {#each PRESETS as preset (preset)}
                    <button
                        class="preset"
                        class:current={zoomPercent === preset}
                        onclick={() => workspace.setZoom(preset / 100)}
                    >
                        {preset}%
                    </button>
                {/each}
            </div>

            <p class="shortcuts">
                <kbd>⌘</kbd><kbd>+</kbd> and <kbd>⌘</kbd><kbd>−</kbd> from anywhere in the app,
                <kbd>⌘</kbd><kbd>0</kbd> back to 100%.
            </p>
        </div>
    </SettingsRow>

    <SettingsRow
        label="Show kubeconfig file names"
        hint="Groups the sidebar's contexts under the file they came from. Turned off, they are one flat list — worth it when every context lives in the same ~/.kube/config and the heading only repeats itself. A file that could not be read is still named either way."
    >
        <Toggle
            checked={workspace.showKubeconfigNames}
            label="Show kubeconfig file names in the sidebar"
            onchange={(v) => workspace.setShowKubeconfigNames(v)}
        />
    </SettingsRow>

    <SettingsRow
        label="Show line numbers"
        hint="Draws a numbered gutter down the side of the YAML editor in the dock. The numbers are also where a YAML error is marked, so turning them off leaves the message without a row to point at."
    >
        <Toggle
            checked={workspace.showLineNumbers}
            label="Show line numbers in the YAML editor"
            onchange={(v) => workspace.setShowLineNumbers(v)}
        />
    </SettingsRow>

    <SettingsRow label="Table density" hint="How tall a row is in a resource listing.">
        <SegmentedControl
            options={DENSITIES}
            value={workspace.density}
            label="Table density"
            onchange={(v) => workspace.setDensity(v as 'comfortable' | 'compact')}
        />
    </SettingsRow>

</SettingsSection>

<style>
    /* Reads as the current value with a way in, rather than as a button: what
       the row is mostly doing is telling you which theme is on. */
    .jump {
        display: flex;
        align-items: center;
        gap: 7px;
        padding: 5px 10px;
        border-radius: var(--radius-sm);
        background: var(--bg-raised);
        box-shadow: inset 0 0 0 1px var(--border);
        font-size: 12px;
        color: var(--text);
    }

    .jump:hover {
        background: var(--bg-hover);
    }

    .zoom {
        display: flex;
        flex-direction: column;
        align-items: flex-end;
        gap: 8px;
    }

    .stepper {
        display: flex;
        align-items: center;
        gap: 8px;
    }

    .stepper button {
        display: grid;
        place-items: center;
        width: 26px;
        height: 26px;
        flex: 0 0 auto;
        border-radius: var(--radius-sm);
        background: var(--bg-raised);
        box-shadow: inset 0 0 0 1px var(--border);
        color: var(--text);
        font-size: 14px;
        line-height: 1;
    }

    .stepper button:hover:not(:disabled) {
        background: var(--bg-hover);
    }

    .stepper button:disabled {
        opacity: 0.4;
        cursor: default;
    }

    input[type='range'] {
        width: 132px;
        accent-color: var(--accent);
    }

    .value {
        min-width: 46px;
        text-align: right;
        font-family: var(--mono);
        font-size: 12px;
        color: var(--text);
    }

    .presets {
        display: flex;
        gap: 4px;
    }

    .preset {
        padding: 3px 9px;
        border-radius: var(--radius-sm);
        font-family: var(--mono);
        font-size: 11px;
        color: var(--text-faint);
        box-shadow: inset 0 0 0 1px transparent;
    }

    .preset:hover {
        background: var(--bg-hover);
        color: var(--text);
    }

    .preset.current {
        color: var(--text);
        background: var(--bg-raised);
        box-shadow: inset 0 0 0 1px var(--border);
    }

    /* Right-aligned with the controls above it, and allowed to wrap rather
       than force the whole row wider on a narrow panel. */
    .shortcuts {
        margin: 0;
        text-align: right;
        font-size: 11px;
        line-height: 1.9;
        color: var(--text-faint);
        max-width: 30ch;
    }

    kbd {
        display: inline-block;
        min-width: 18px;
        padding: 1px 4px;
        margin: 0 1px;
        border-radius: 3px;
        background: var(--bg-raised);
        box-shadow: inset 0 0 0 1px var(--border);
        font-family: var(--mono);
        font-size: 10px;
        text-align: center;
        color: var(--text-dim);
    }

    /* On a narrow panel the row stacks, so the controls read left-to-right
       under their label rather than hugging an edge that is no longer there. */
    @container settings-panel (max-width: 640px) {
        .zoom {
            align-items: flex-start;
        }

        .shortcuts {
            text-align: left;
        }
    }
</style>
