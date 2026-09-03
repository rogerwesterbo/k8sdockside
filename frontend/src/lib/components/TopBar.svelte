<!--
  The window's title bar.

  It exists for two reasons. The obvious one is that the app should say what it
  is. The other is layout: on macOS the traffic lights are drawn over the top
  left of the web view, and without a bar of their own they land on top of the
  sidebar's heading. Giving them a full-width strip moves everything else out
  from under them.

  The title is centred on the window rather than in the space left over, so it
  does not drift as the sidebar is resized.
-->
<header class="topbar">
    <div class="title">
        <img src="/k8s_dockside_logo_concept.svg" alt="" width="18" height="18" />
        <span>K8S Dockside</span>
    </div>
</header>

<style>
    .topbar {
        position: relative;
        display: flex;
        align-items: center;
        flex: 0 0 auto;
        height: 44px;
        background: var(--bg-sidebar);
        border-bottom: 1px solid var(--border);
        /* Dragging the bar moves the window, as a title bar should. Wails reads
           this property off the element under the pointer. */
        --wails-draggable: drag;
    }

    .title {
        position: absolute;
        left: 50%;
        transform: translateX(-50%);
        display: flex;
        align-items: center;
        gap: 8px;
        font-size: 13px;
        font-weight: 600;
        letter-spacing: 0.02em;
        color: var(--text);
        pointer-events: none;
    }

    .title img {
        display: block;
        opacity: 0.95;
    }
</style>
