<!--
  The application shell: sidebar, tab strip, the active view, and the docked
  detail panel. All of the state it renders lives in the workspace store.
-->
<script lang="ts">
    import { onMount } from 'svelte';
    import { DASHBOARD, SETTINGS } from './lib/catalogue';
    import Dashboard from './lib/components/Dashboard.svelte';
    import DetailPanel from './lib/components/DetailPanel.svelte';
    import Dock from './lib/components/Dock.svelte';
    import Icon from './lib/components/Icon.svelte';
    import ResourceTable from './lib/components/ResourceTable.svelte';
    import SettingsView from './lib/components/settings/SettingsView.svelte';
    import Sidebar from './lib/components/Sidebar.svelte';
    import TabBar from './lib/components/TabBar.svelte';
    import TopBar from './lib/components/TopBar.svelte';
    import { workspace } from './lib/state/workspace.svelte';

    const MIN_SIDEBAR = 200;
    const MAX_SIDEBAR = 520;

    let draggingSidebar = $state(false);

    onMount(() => {
        workspace.load();
        // Returned so the media query listener is torn down with the shell.
        return workspace.watchSystemTheme();
    });

    // Zoom is applied as CSS on the app's own element, not through the window.
    //
    // Wails' native zoom cannot do this job on macOS: it clamps the scale to a
    // minimum of 1.0, so zooming out is silently discarded, and what it does
    // apply is -[WKWebView setMagnification:], which scales the rendered
    // surface without reflowing -- so the page keeps its full layout width and
    // spills out of the window with its own scrollbars, outside anything CSS
    // can say about overflow.
    //
    // CSS zoom reflows instead. Everything sized in pixels scales together, the
    // sidebar included, and the regions that scroll go on scrolling. It is set
    // on #app rather than the root because a percentage height under a zoomed
    // root ignores that zoom -- see the note in public/style.css.
    $effect(() => {
        const scale = workspace.zoom;
        document.documentElement.style.setProperty('--app-zoom', String(scale));

        // The title bar is the one thing that must not scale away. It is drawn
        // here in CSS pixels, while the macOS traffic lights over it keep their
        // real size -- so zooming out has to make it proportionally taller to
        // go on containing them. Zooming in it grows on its own.
        document.documentElement.style.setProperty('--topbar-h', `${Math.max(44, 44 / scale)}px`);
    });

    // The appearance preferences are applied to the document root rather than
    // scoped to a component, because they are what every component's own tokens
    // resolve against. `data-theme` is the seam the light palette hangs off:
    // set here, read only by style.css.
    $effect(() => {
        const root = document.documentElement;
        root.dataset.theme = workspace.resolvedTheme;
        const compact = workspace.density === 'compact';
        root.style.setProperty('--row-h', compact ? '24px' : '30px');
        root.style.setProperty('--cell-pad-y', compact ? '3px' : '6px');
    });

    function onZoomKey(event: KeyboardEvent): void {
        if (!event.metaKey && !event.ctrlKey) return;
        // `code` as well as `key`, so the numeric keypad works and so that a
        // layout where + needs shift is still recognised.
        switch (event.key) {
            case '+':
            case '=':
                event.preventDefault();
                workspace.zoomIn();
                return;
            case '-':
            case '_':
                event.preventDefault();
                workspace.zoomOut();
                return;
            case '0':
                event.preventDefault();
                workspace.resetZoom();
                return;
        }
        if (event.key === ',') {
            event.preventDefault();
            workspace.openSettings();
            return;
        }
        if (event.code === 'NumpadAdd') {
            event.preventDefault();
            workspace.zoomIn();
        } else if (event.code === 'NumpadSubtract') {
            event.preventDefault();
            workspace.zoomOut();
        }
    }

    // Notices are informational; they should not need dismissing by hand.
    $effect(() => {
        if (!workspace.notice) return;
        const timer = setTimeout(() => workspace.dismissNotice(), 6000);
        return () => clearTimeout(timer);
    });

    function startSidebarResize(event: PointerEvent): void {
        event.preventDefault();
        draggingSidebar = true;
        (event.currentTarget as HTMLElement).setPointerCapture(event.pointerId);
    }

    function onSidebarResize(event: PointerEvent): void {
        if (!draggingSidebar) return;
        workspace.setSidebarWidth(Math.max(MIN_SIDEBAR, Math.min(event.clientX, MAX_SIDEBAR)));
    }

    function endSidebarResize(event: PointerEvent): void {
        draggingSidebar = false;
        (event.currentTarget as HTMLElement).releasePointerCapture(event.pointerId);
    }

    function onSidebarKey(event: KeyboardEvent): void {
        const step = event.shiftKey ? 48 : 16;
        if (event.key === 'ArrowLeft') {
            event.preventDefault();
            workspace.setSidebarWidth(Math.max(MIN_SIDEBAR, workspace.sidebarWidth - step));
        } else if (event.key === 'ArrowRight') {
            event.preventDefault();
            workspace.setSidebarWidth(Math.min(MAX_SIDEBAR, workspace.sidebarWidth + step));
        }
    }
</script>

<svelte:window onkeydown={onZoomKey} />

<div class="shell">
    <TopBar />

    <div class="body">
        <Sidebar />

        <!-- A focusable separator is the ARIA "window splitter" pattern; the
             a11y rules below only key off the role, which they treat as static. -->
        <!-- svelte-ignore a11y_no_noninteractive_tabindex -->
        <!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
        <div
            class="sidebar-handle"
            class:active={draggingSidebar}
            role="separator"
            aria-label="Resize sidebar"
            aria-orientation="vertical"
            aria-valuenow={workspace.sidebarWidth}
            aria-valuemin={MIN_SIDEBAR}
            aria-valuemax={MAX_SIDEBAR}
            tabindex="0"
            onpointerdown={startSidebarResize}
            onpointermove={onSidebarResize}
            onpointerup={endSidebarResize}
            onpointercancel={endSidebarResize}
            onkeydown={onSidebarKey}
        ></div>

        <main>
            <TabBar />

            <div class="stage" class:stack={workspace.dock === 'bottom'}>
                <div class="content">
                    {#if workspace.activeTab}
                        {#key workspace.activeTab.id}
                            {#if workspace.activeTab.kind === SETTINGS}
                                <SettingsView />
                            {:else if workspace.activeTab.kind === DASHBOARD}
                                <Dashboard contextId={workspace.activeTab.contextId} />
                            {:else}
                                <ResourceTable
                                    contextId={workspace.activeTab.contextId}
                                    kind={workspace.activeTab.kind}
                                />
                            {/if}
                        {/key}
                    {:else}
                        <div class="welcome-stage">
                            <div class="welcome">
                            <h1>K8s Dockside</h1>
                            {#if workspace.contexts.length > 0}
                                <p>Pick a context in the sidebar, then choose a view to open it as a tab.</p>
                                <p class="hint">
                                    Tabs take the colour of their context, so you always know which cluster you are
                                    looking at. Drag them to reorder.
                                </p>
                            {:else if workspace.loaded}
                                <p>No kubeconfig contexts yet.</p>
                                <p class="hint">
                                    Nothing turned up in <code>~/.kube</code> or <code>$KUBECONFIG</code>. Use the
                                    sidebar to add kubeconfig files, or point it at a folder and it will take every
                                    one in there, whatever they are named.
                                </p>
                            {:else}
                                <p>Looking for kubeconfig files…</p>
                            {/if}
                            </div>
                        </div>
                    {/if}
                </div>

                <DetailPanel />
            </div>

            <!-- Under the view rather than beside the sidebar: it is about the
                 object you were just looking at, and it keeps the sidebar
                 whole-height so the context list is never shortened by it. -->
            <Dock />
        </main>
    </div>

    <footer class="statusbar">
        {#if workspace.notice}
            <span class="notice" class:error={workspace.notice.tone === 'error'}>
                {#if workspace.notice.tone === 'error'}<Icon name="alert" size={12} />{/if}
                {workspace.notice.text}
            </span>
            <button class="dismiss" onclick={() => workspace.dismissNotice()} aria-label="Dismiss message">
                <Icon name="close" size={11} />
            </button>
        {:else if workspace.selectedContext}
            <span class="dot" style:background={workspace.colorOf(workspace.selectedContext.id)}></span>
            <span>{workspace.displayName(workspace.selectedContext)}</span>
            {#if workspace.selectedContext.server}
                <span class="dim mono">{workspace.selectedContext.server}</span>
            {/if}
        {/if}

        <span class="spacer"></span>
        <span class="dim">{workspace.contexts.length} contexts · {workspace.files.length} files</span>
        {#if workspace.configPath}
            <span class="dim mono" title="Where your context names, colours and layout are stored">
                {workspace.configPath}
            </span>
        {/if}
    </footer>
</div>

<style>
    .shell {
        display: flex;
        flex-direction: column;
        height: 100%;
    }

    .body {
        display: flex;
        flex: 1 1 auto;
        min-height: 0;
    }

    /* Sits over the sidebar's right border so the whole seam is grabbable. */
    .sidebar-handle {
        width: 5px;
        margin-left: -3px;
        margin-right: -2px;
        z-index: 3;
        cursor: col-resize;
        background: transparent;
        transition: background 120ms ease;
    }

    .sidebar-handle:hover,
    .sidebar-handle.active,
    .sidebar-handle:focus-visible {
        background: var(--accent);
    }

    main {
        display: flex;
        flex-direction: column;
        /* Basis 0 rather than auto: what is in the view must not decide how
           much of the window the view asks for. With `auto` the basis is the
           content's own width, so one wide table made the whole row overflow
           and the sidebar paid for it. */
        flex: 1 1 0;
        min-width: 0;
        min-height: 0;
    }

    .stage {
        display: flex;
        flex: 1 1 auto;
        min-height: 0;
        min-width: 0;
    }

    .stage.stack {
        flex-direction: column;
    }

    .content {
        flex: 1 1 auto;
        min-width: 0;
        min-height: 0;
        overflow: hidden;
    }

    /* The logo as a watermark behind the idle screen. It fills the empty
       content area rather than sitting at a fixed size, so the app looks like
       itself when nothing is open -- and goes with the panel the moment a tab
       is, rather than sitting behind a table of pod names. */
    .welcome-stage {
        position: relative;
        display: flex;
        align-items: center;
        justify-content: center;
        height: 100%;
        overflow-y: auto;
    }

    /* The image lives on a pseudo-element so it alone can be faded. Setting
       opacity on the container would take the text down with it, and the text
       is the part that has to stay readable -- which is the whole difference
       between a background and a picture.

       `contain`-style sizing keeps the mark whole at any window size, and it is
       kept out of the way of the pointer so nothing here is selectable. */
    .welcome-stage::before {
        content: '';
        position: absolute;
        inset: 0;
        background-image: url('/k8s_dockside_harbour_scene_no_text.svg');
        background-repeat: no-repeat;
        background-position: center;
        /* Scaled past the edges rather than fitted, because the artwork is a
           framed illustration and not a transparent mark. Shown whole it brings
           two things that do not belong behind this text: the rounded card edge
           reads as a stray rectangle, and the illustration's own title repeats
           the heading in front of it. Overscaling crops both away and leaves
           the harbour scene. */
        background-size: 124% 158%;
        background-position: center 32%;
        opacity: 0.09;
        pointer-events: none;
    }

    .welcome {
        /* Above the watermark, not through it. */
        position: relative;
        z-index: 1;
        max-width: 520px;
        padding: 64px 32px;
        margin: 0 auto;
        text-align: center;
    }

    .welcome h1 {
        margin: 0 0 12px;
        font-size: 22px;
        font-weight: 600;
    }

    .welcome p {
        margin: 0 0 10px;
        color: var(--text-dim);
    }

    .welcome .hint {
        font-size: 12px;
        color: var(--text-faint);
        line-height: 1.7;
    }

    .welcome code {
        font-family: var(--mono);
        font-size: 11px;
        background: var(--bg-raised);
        border-radius: 3px;
        padding: 1px 5px;
    }

    .statusbar {
        display: flex;
        align-items: center;
        gap: 10px;
        height: 24px;
        padding: 0 12px;
        flex: 0 0 auto;
        background: var(--bg-sidebar);
        border-top: 1px solid var(--border);
        font-size: 11px;
        color: var(--text-dim);
        white-space: nowrap;
        overflow: hidden;
    }

    .spacer {
        flex: 1 1 auto;
    }

    .dot {
        width: 8px;
        height: 8px;
        border-radius: 2px;
        flex: 0 0 auto;
    }

    .dim {
        color: var(--text-faint);
        overflow: hidden;
        text-overflow: ellipsis;
    }

    .mono {
        font-family: var(--mono);
        font-size: 10px;
    }

    .notice {
        display: flex;
        align-items: center;
        gap: 6px;
        overflow: hidden;
        text-overflow: ellipsis;
    }

    .notice.error {
        color: var(--error);
    }

    .dismiss {
        display: grid;
        place-items: center;
        width: 16px;
        height: 16px;
        border-radius: 3px;
        color: var(--text-faint);
    }

    .dismiss:hover {
        background: var(--bg-hover);
        color: var(--text);
    }
</style>
