<!--
  How the app looks: theme, zoom, table density and font size.

  Nothing here is about Kubernetes, which is why it is a section of its own.
-->
<script lang="ts">
    import { workspace } from '../../state/workspace.svelte';
    import SegmentedControl from './SegmentedControl.svelte';
    import SettingsRow from './SettingsRow.svelte';
    import SettingsSection from './SettingsSection.svelte';

    const THEMES = [
        { value: 'system', label: 'System', icon: 'monitor' },
        { value: 'light', label: 'Light', icon: 'sun' },
        { value: 'dark', label: 'Dark', icon: 'moon' },
    ];

    const DENSITIES = [
        { value: 'comfortable', label: 'Comfortable' },
        { value: 'compact', label: 'Compact' },
    ];

    let zoomPercent = $derived(Math.round(workspace.zoom * 100));
</script>

<SettingsSection title="Appearance">
    <SettingsRow
        label="Theme"
        hint={workspace.theme === 'system'
            ? `Following the system, which is currently ${workspace.resolvedTheme}.`
            : 'Set for this app alone, whatever the system is doing.'}
    >
        <SegmentedControl
            options={THEMES}
            value={workspace.theme}
            label="Theme"
            onchange={(v) => workspace.setTheme(v as 'system' | 'light' | 'dark')}
        />
    </SettingsRow>

    <SettingsRow
        label="Zoom"
        hint="Scales the whole window, the sidebar included. ⌘+ and ⌘− do the same from anywhere, and ⌘0 comes back to 100%."
    >
        <div class="stepper">
            <button onclick={() => workspace.zoomOut()} aria-label="Zoom out">−</button>
            <span class="value">{zoomPercent}%</span>
            <button onclick={() => workspace.zoomIn()} aria-label="Zoom in">+</button>
            <button class="reset" onclick={() => workspace.resetZoom()} disabled={zoomPercent === 100}>Reset</button>
        </div>
    </SettingsRow>

    <SettingsRow label="Table density" hint="How tall a row is in a resource listing.">
        <SegmentedControl
            options={DENSITIES}
            value={workspace.density}
            label="Table density"
            onchange={(v) => workspace.setDensity(v as 'comfortable' | 'compact')}
        />
    </SettingsRow>

    <SettingsRow
        label="Font size"
        hint="The base size everything else is drawn against. Separate from zoom: this changes the text without changing the size of the window's furniture."
    >
        <div class="stepper">
            <input
                type="range"
                min="11"
                max="18"
                step="1"
                value={workspace.fontSize}
                aria-label="Font size in pixels"
                oninput={(e) => workspace.setFontSize(Number((e.currentTarget as HTMLInputElement).value))}
            />
            <span class="value">{workspace.fontSize}px</span>
        </div>
    </SettingsRow>
</SettingsSection>

<style>
    .stepper {
        display: flex;
        align-items: center;
        gap: 8px;
    }

    .stepper button {
        display: grid;
        place-items: center;
        min-width: 26px;
        height: 26px;
        padding: 0 8px;
        border-radius: var(--radius-sm);
        background: var(--bg-raised);
        box-shadow: inset 0 0 0 1px var(--border);
        color: var(--text);
        font-size: 13px;
    }

    .stepper button:hover:not(:disabled) {
        background: var(--bg-hover);
    }

    .stepper button:disabled {
        opacity: 0.4;
        cursor: default;
    }

    .stepper .reset {
        font-size: 11px;
        color: var(--text-dim);
    }

    .value {
        min-width: 42px;
        text-align: center;
        font-family: var(--mono);
        font-size: 11px;
        color: var(--text-dim);
    }

    input[type='range'] {
        width: 120px;
        accent-color: var(--accent);
    }
</style>
