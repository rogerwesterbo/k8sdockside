<!--
  One kubeconfig context in the sidebar, plus the resource tree it expands into.
  Clicking a resource opens (or focuses) a tab for it.
-->
<script lang="ts">
    import type * as kube from '../../../bindings/github.com/roger/k8sdockside/internal/kube/models.js';
    import { NAV_GROUPS } from '../catalogue';
    import { alpha } from '../colors';
    import { workspace } from '../state/workspace.svelte';
    import Icon from './Icon.svelte';

    interface Props {
        context: kube.Context;
    }

    let { context }: Props = $props();

    let color = $derived(workspace.colorOf(context.id));
    let expanded = $derived(workspace.isExpanded(context.id));
    let selected = $derived(workspace.selectedContextId === context.id);
    let activeTab = $derived(workspace.activeTab);

    function isOpen(kind: string): boolean {
        return activeTab?.contextId === context.id && activeTab.kind === kind;
    }
</script>

<div class="context" class:selected style:--ctx-color={color} style:--ctx-tint={alpha(color, 0.16)}>
    <div class="head">
        <button
            class="twisty"
            onclick={() => workspace.toggleExpanded(context.id)}
            aria-label={expanded ? 'Collapse' : 'Expand'}
            aria-expanded={expanded}
        >
            <Icon name={expanded ? 'chevron-down' : 'chevron-right'} size={14} />
        </button>

        <button class="label" onclick={() => workspace.selectContext(context.id)} title={context.server || context.name}>
            <span class="swatch" style:background={color}></span>
            <span class="name">{workspace.displayName(context)}</span>
            {#if context.current}
                <span class="badge" title="current-context in this kubeconfig">current</span>
            {/if}
        </button>
    </div>

    {#if expanded}
        <div class="tree">
            {#each NAV_GROUPS as group (group.label)}
                <p class="group">{group.label}</p>
                {#each group.items as item (item.kind)}
                    <button
                        class="item"
                        class:open={isOpen(item.kind)}
                        onclick={() => workspace.openTab(context.id, item.kind)}
                    >
                        <Icon name={item.icon} size={15} />
                        <span>{item.label}</span>
                    </button>
                {/each}
            {/each}
        </div>
    {/if}
</div>

<style>
    .context {
        --indent: 26px;
    }

    .head {
        display: flex;
        align-items: center;
        height: var(--row-h);
        border-radius: var(--radius-sm);
        position: relative;
    }

    .head:hover {
        background: var(--bg-hover);
    }

    .selected .head {
        background: var(--ctx-tint);
    }

    /* A colour bar on the selected context, matching its tabs. */
    .selected .head::before {
        content: '';
        position: absolute;
        left: 0;
        top: 4px;
        bottom: 4px;
        width: 2px;
        border-radius: 2px;
        background: var(--ctx-color);
    }

    .twisty {
        display: grid;
        place-items: center;
        width: var(--indent);
        height: 100%;
        color: var(--text-faint);
        flex: 0 0 auto;
    }

    .twisty:hover {
        color: var(--text);
    }

    .label {
        display: flex;
        align-items: center;
        gap: 7px;
        flex: 1 1 auto;
        min-width: 0;
        height: 100%;
        padding-right: 8px;
        text-align: left;
    }

    .swatch {
        width: 9px;
        height: 9px;
        border-radius: 2px;
        flex: 0 0 auto;
        box-shadow: 0 0 0 1px rgba(0, 0, 0, 0.35);
    }

    .name {
        overflow: hidden;
        text-overflow: ellipsis;
        white-space: nowrap;
        color: var(--text-dim);
    }

    .selected .name {
        color: var(--text);
    }

    .badge {
        flex: 0 0 auto;
        font-size: 9px;
        letter-spacing: 0.04em;
        text-transform: uppercase;
        color: var(--ctx-color);
        border: 1px solid var(--ctx-color);
        border-radius: 3px;
        padding: 0 4px;
        line-height: 14px;
        opacity: 0.85;
    }

    .tree {
        padding: 2px 0 6px;
    }

    .group {
        margin: 8px 0 2px;
        padding-left: var(--indent);
        font-size: 10px;
        letter-spacing: 0.08em;
        text-transform: uppercase;
        color: var(--text-faint);
    }

    .item {
        display: flex;
        align-items: center;
        gap: 8px;
        width: 100%;
        height: 26px;
        padding-left: var(--indent);
        padding-right: 8px;
        color: var(--text-dim);
        border-radius: var(--radius-sm);
        text-align: left;
    }

    .item:hover {
        background: var(--bg-hover);
        color: var(--text);
    }

    .item.open {
        background: var(--ctx-tint);
        color: var(--text);
    }

    .item span {
        overflow: hidden;
        text-overflow: ellipsis;
        white-space: nowrap;
    }
</style>
