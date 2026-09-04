<!--
  The tab strip above the view. Each tab is coloured with its context's colour
  -- fully saturated when active, tinted when not -- so you can always see which
  cluster a tab is talking to.

  Everything a strip of tabs does (dragging, scrolling, the right-click menu)
  lives in TabStrip, which the dock at the foot of the window uses too. What is
  here is only what these particular tabs are: which of them are open, what
  closing one means, and when a tab has to name its cluster.
-->
<script lang="ts">
    import { iconFor } from '../catalogue';
    import { isSettingsTab, workspace } from '../state/workspace.svelte';
    import TabStrip, { type StripTab } from './TabStrip.svelte';

    // The context name only earns space on a tab when tabs from more than one
    // context are open; otherwise the colour alone is enough.
    // Counted over cluster tabs only. The settings tab has no context, so
    // including its empty id would make a strip showing one cluster plus
    // settings look like two clusters and start labelling every tab.
    let showContext = $derived(
        new Set(workspace.tabs.filter((t) => !isSettingsTab(t)).map((t) => t.contextId)).size > 1,
    );

    function contextName(contextId: string): string {
        const context = workspace.contexts.find((c) => c.id === contextId);
        return context ? workspace.displayName(context) : contextId;
    }

    let tabs = $derived(
        workspace.tabs.map(
            (tab): StripTab => ({
                id: tab.id,
                title: tab.title,
                subtitle: showContext && !isSettingsTab(tab) ? contextName(tab.contextId) : undefined,
                icon: iconFor(tab.kind),
                color: workspace.colorOf(tab.contextId),
                hint: isSettingsTab(tab)
                    ? 'Application settings'
                    : `${tab.title} — ${contextName(tab.contextId)}`,
            }),
        ),
    );

    /** The context a strip tab belongs to, for the menu's cluster-scoped items. */
    function contextOf(id: string): string {
        return workspace.tabs.find((t) => t.id === id)?.contextId ?? '';
    }

    function run(action: () => void, dismiss: () => void): void {
        action();
        dismiss();
    }
</script>

<TabStrip
    {tabs}
    activeId={workspace.activeTabId}
    label="Open views"
    onactivate={(id) => workspace.activateTab(id)}
    onclose={(id) => workspace.closeTab(id)}
    onmove={(from, to) => workspace.moveTab(from, to)}
    menu={tabMenu}
/>

{#snippet tabMenu(tab: StripTab, dismiss: () => void)}
    {@const contextId = contextOf(tab.id)}
    <button role="menuitem" onclick={() => run(() => workspace.closeTab(tab.id), dismiss)}>Close</button>
    <button role="menuitem" onclick={() => run(() => workspace.closeOtherTabs(tab.id), dismiss)}>
        Close Others
    </button>
    <button role="menuitem" onclick={() => run(() => workspace.closeAllTabs(), dismiss)}>Close All</button>

    {#if showContext}
        {@const cluster = contextName(contextId)}
        <hr />
        <button role="menuitem" onclick={() => run(() => workspace.closeOtherTabs(tab.id, contextId), dismiss)}>
            Close Others in {cluster}
        </button>
        <button role="menuitem" onclick={() => run(() => workspace.closeAllTabs(contextId), dismiss)}>
            Close All in {cluster}
        </button>
    {/if}
{/snippet}
