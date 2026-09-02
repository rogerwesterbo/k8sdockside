<!--
  A single-stroke icon set, kept inline so the app has no icon dependency and
  every glyph inherits currentColor. Each icon is a list of path commands drawn
  on a 24x24 grid.
-->
<script lang="ts" module>
    const PATHS: Record<string, string[]> = {
        // Resource kinds
        dashboard: ['M4 4h6v6H4z', 'M14 4h6v6h-6z', 'M4 14h6v6H4z', 'M14 14h6v6h-6z'],
        server: ['M4 5h16v5H4z', 'M4 14h16v5H4z', 'M7.5 7.5h.01', 'M7.5 16.5h.01'],
        layers: ['M12 3 3 8l9 5 9-5-9-5z', 'M3 13l9 5 9-5'],
        bell: ['M18 9a6 6 0 1 0-12 0c0 5-2 6-2 6h16s-2-1-2-6', 'M10.3 20a2 2 0 0 0 3.4 0'],
        box: ['M20 8 12 4 4 8v8l8 4 8-4V8z', 'M4 8l8 4 8-4', 'M12 12v8'],
        rocket: ['M3 7l9-4 9 4-9 4-9-4z', 'M3 12l9 4 9-4', 'M3 17l9 4 9-4'],
        database: [
            'M12 3c4.4 0 8 1.3 8 3s-3.6 3-8 3-8-1.3-8-3 3.6-3 8-3z',
            'M4 6v12c0 1.7 3.6 3 8 3s8-1.3 8-3V6',
            'M4 12c0 1.7 3.6 3 8 3s8-1.3 8-3',
        ],
        repeat: ['M17 2l4 4-4 4', 'M3 11V9a4 4 0 0 1 4-4h14', 'M7 22l-4-4 4-4', 'M21 13v2a4 4 0 0 1-4 4H3'],
        check: ['M9 11.5l2.5 2.5L20 5.5', 'M20 12v7a2 2 0 0 1-2 2H6a2 2 0 0 1-2-2V6a2 2 0 0 1 2-2h9'],
        clock: ['M12 3a9 9 0 1 0 0 18 9 9 0 0 0 0-18z', 'M12 7v5l3 2'],
        share: [
            'M18 8a3 3 0 1 0 0-6 3 3 0 0 0 0 6z',
            'M6 15a3 3 0 1 0 0-6 3 3 0 0 0 0 6z',
            'M18 22a3 3 0 1 0 0-6 3 3 0 0 0 0 6z',
            'M8.6 13.4l6.8 3.9',
            'M15.4 6.7l-6.8 3.9',
        ],
        globe: ['M12 3a9 9 0 1 0 0 18 9 9 0 0 0 0-18z', 'M3 12h18', 'M12 3a14 14 0 0 1 0 18 14 14 0 0 1 0-18z'],
        sliders: ['M4 7h10', 'M18 7h2', 'M4 17h4', 'M12 17h8', 'M16 5v4', 'M8 15v4'],
        lock: ['M6 10h12v10H6z', 'M9 10V7a3 3 0 0 1 6 0v3'],
        drive: ['M5 12l2-6h10l2 6v6H5v-6z', 'M5 12h14', 'M8 15.5h.01', 'M11 15.5h.01'],
        // A gateway: two posts with traffic passing between them.
        gateway: ['M5 4v16', 'M19 4v16', 'M9 12h6', 'M12.5 9l3 3-3 3'],
        // A route: one entry branching to several backends.
        route: ['M4 12h4', 'M8 12c4 0 4-6 8-6h4', 'M8 12c4 0 4 6 8 6h4', 'M20 6h.01', 'M20 18h.01'],
        // A grant: a permission crossing a boundary.
        grant: ['M4 5h7v14H4z', 'M15 12h5', 'M17.5 9.5l2.5 2.5-2.5 2.5'],
        // A custom resource: a piece that fits into the API.
        puzzle: [
            'M10 4h4v2.5a1.5 1.5 0 1 0 3 0V4h3v4h-2.5a1.5 1.5 0 1 0 0 3H20v4h-4v-2.5a1.5 1.5 0 1 0-3 0V15H9v-4H6.5a1.5 1.5 0 1 0 0-3H9V4z',
        ],

        // Interface
        'chevron-left': ['M14.5 6l-6 6 6 6'],
        'chevron-right': ['M9.5 6l6 6-6 6'],
        'chevron-down': ['M6 9.5l6 6 6-6'],
        refresh: ['M20.5 12a8.5 8.5 0 1 1-2.6-6.1', 'M20.5 4v5h-5'],
        plus: ['M12 5v14', 'M5 12h14'],
        // A folder with a plus: add every kubeconfig in a directory.
        'folder-plus': [
            'M3 7a1 1 0 0 1 1-1h5l2 2.5h8a1 1 0 0 1 1 1V18a1 1 0 0 1-1 1H4a1 1 0 0 1-1-1V7z',
            'M12 11.5v5',
            'M9.5 14h5',
        ],
        folder: ['M3 7a1 1 0 0 1 1-1h5l2 2.5h8a1 1 0 0 1 1 1V18a1 1 0 0 1-1 1H4a1 1 0 0 1-1-1V7z'],
        close: ['M6.5 6.5l11 11', 'M17.5 6.5l-11 11'],
        alert: ['M12 3.5l9.5 17h-19l9.5-17z', 'M12 9.5v4.5', 'M12 17.5h.01'],
        file: ['M14 3H7a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2V8l-5-5z', 'M14 3v5h5'],
        search: ['M11 4.5a6.5 6.5 0 1 0 0 13 6.5 6.5 0 0 0 0-13z', 'M20 20l-4.4-4.4'],
        trash: ['M4 7h16', 'M9.5 7V4.5h5V7', 'M6.5 7l1 13h9l1-13'],
        undo: ['M3.5 8h11a5 5 0 0 1 0 10H8', 'M7.5 4l-4 4 4 4'],
        'dock-right': ['M4 5h16v14H4z', 'M14.5 5v14'],
        'dock-bottom': ['M4 5h16v14H4z', 'M4 14.5h16'],
        'dock-left': ['M4 5h16v14H4z', 'M9.5 5v14'],
        pin: ['M9 4h6', 'M10 4l-.5 7-3 2.5h11l-3-2.5-.5-7', 'M12 13.5V20'],
    };
</script>

<script lang="ts">
    interface Props {
        name: string;
        size?: number;
        /** Stroke width; thinner reads better at small sizes on dark backgrounds. */
        weight?: number;
    }

    let { name, size = 16, weight = 1.6 }: Props = $props();

    // An unknown name falls back to a generic glyph rather than rendering an
    // empty box, so a typo in a catalogue entry is visible but not disruptive.
    let paths = $derived(PATHS[name] ?? PATHS.box);
</script>

<svg
    class="icon"
    width={size}
    height={size}
    viewBox="0 0 24 24"
    fill="none"
    stroke="currentColor"
    stroke-width={weight}
    stroke-linecap="round"
    stroke-linejoin="round"
    aria-hidden="true"
>
    {#each paths as d (d)}
        <path {d} />
    {/each}
</svg>

<style>
    .icon {
        display: block;
        flex: 0 0 auto;
    }
</style>
