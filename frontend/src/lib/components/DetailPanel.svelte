<!--
  The describe panel: what a tab whose view is `details` renders.

  It used to dock to an edge of the window with a size, a resize handle and
  three buttons of its own, all of which were a second layout system for one
  panel. It is a tab now, so where it sits is which pane holds it, how big it is
  is that pane's size, and it is moved by the same drag that moves everything
  else. What is left here is the report and what can be done to the object.
-->
<script lang="ts">
    import { HELM_RELEASES, singularFor } from '../catalogue';
    import MetricsPanel from '../charts/MetricsPanel.svelte';
    import ResourceBudget from '../budget/ResourceBudget.svelte';
    import { alpha } from '../colors';
    import { actions } from '../state/actions.svelte';
    import { changes } from '../state/changes.svelte';
    import { workspace } from '../state/workspace.svelte';
    import ContainerPills from './ContainerPills.svelte';
    import ErrorState from './ErrorState.svelte';
    import HelmRelease from './HelmRelease.svelte';
    import ObjectActions from './ObjectActions.svelte';

    let target = $derived(workspace.detailTarget);
    /**
     * The two kinds that own a resource budget of their own.
     *
     * A node holds hardware and a namespace holds a quota; everything else is
     * accounted for inside one of those two, so a budget on it would either
     * repeat the parent's numbers or invent a denominator.
     */
    const BUDGET_SCOPES: Record<string, string> = { nodes: 'node', namespaces: 'namespace' };
    let budgetScope = $derived(target ? (BUDGET_SCOPES[target.kind] ?? '') : '');
    let color = $derived(target ? workspace.colorOf(target.contextId) : 'var(--accent)');
    /**
     * Keeps the report level with the object.
     *
     * The object can be written from somewhere else while this panel is open --
     * the editor in the dock is the usual place, and it is often open on
     * exactly what is being described here. Reading the object's revision is
     * the subscription; when it has moved past the one the report was read at,
     * the panel reads again.
     */
    $effect(() => {
        if (target && changes.revision(target) !== workspace.detailRevision) {
            void workspace.refreshDetail();
        }
    });

    /**
     * Whether this is a Helm release, which is described by its own record
     * rather than by a report read off an object. See HelmRelease.svelte.
     */
    let isRelease = $derived(target?.kind === HELM_RELEASES);

    /**
     * The object's containers, read by the action bar below and shown again
     * here. Empty for everything that is not a pod.
     */
    let containers = $derived(target ? actions.stateOf(target).containers : []);

</script>

<svelte:window
    onkeydown={(e) => {
        if (e.key === 'Escape' && workspace.detailTarget) workspace.closeDetail();
    }}
/>

{#if target}
    <section
        class="panel"
        style:--ctx-color={color}
        style:--ctx-tint={alpha(color, 0.12)}
        aria-label="{singularFor(target.kind)} details"
    >
        <header>
            <div class="ident">
                <span class="kind">{singularFor(target.kind)}</span>
                <h2 class="selectable">{target.name}</h2>
                {#if target.namespace}
                    <span class="ns">in {target.namespace}</span>
                {/if}
                {#if containers.length > 0}
                    <!-- The same squares the table draws, so a pod reads the
                         same wherever you meet it. -->
                    <span class="containers">
                        <ContainerPills pills={containers} />
                    </span>
                {/if}
            </div>
        </header>

        <!-- What can be done to the object, under the line that says what it
             is. Editing lives here too: it is an action like the rest. -->
        <ObjectActions object={target} />

        <div class="body">
            <!-- Above the describe report, not below it: what a pod is *doing*
                 is what someone opening this panel mid-incident came for, and
                 the report is long enough that anything under it is out of
                 sight. Draws nothing at all unless an installed plugin has
                 charts for this kind and the cluster has a Prometheus. -->
            <!-- Above the charts for a node or a namespace: how full it is
                 right now is what someone opening this came to find out, and
                 unlike the charts it needs nothing installed to answer. -->
            {#if budgetScope}
                <ResourceBudget
                    contextId={target.contextId}
                    scope={budgetScope}
                    name={target.name}
                    title="Resources"
                    compact
                />
            {/if}

            <MetricsPanel
                contextId={target.contextId}
                attach={target.kind}
                namespace={target.namespace}
                name={target.name}
                compact
            />

            {#if isRelease}
                <HelmRelease
                    release={{
                        contextId: target.contextId,
                        namespace: target.namespace,
                        name: target.name,
                    }}
                />
            {:else if workspace.detailLoading}
                <p class="status">Describing {target.name}…</p>
            {:else if workspace.detailError}
                <ErrorState message={workspace.detailError} compact />
            {:else}
                <pre class="selectable">{workspace.detailText}</pre>
            {/if}
        </div>
    </section>
{/if}

<style>
    .panel {
        display: flex;
        flex-direction: column;
        min-width: 0;
        min-height: 0;
        height: 100%;
        background: var(--bg-panel);
    }

    header {
        display: flex;
        align-items: flex-start;
        gap: 12px;
        padding: 10px 12px 10px 14px;
        border-bottom: 1px solid var(--border);
        background: var(--ctx-tint);
        border-left: 3px solid var(--ctx-color);
        flex: 0 0 auto;
    }

    .ident {
        min-width: 0;
        flex: 1 1 auto;
    }

    .kind {
        display: block;
        font-size: 10px;
        letter-spacing: 0.08em;
        text-transform: uppercase;
        color: var(--text-faint);
    }

    h2 {
        margin: 2px 0 0;
        font-size: 14px;
        font-weight: 600;
        overflow-wrap: anywhere;
    }

    .ns {
        font-size: 11px;
        color: var(--text-dim);
    }

    .containers {
        display: block;
        margin-top: 5px;
    }

    .body {
        flex: 1 1 auto;
        overflow: auto;
        min-height: 0;
    }

    pre {
        margin: 0;
        padding: 14px 16px;
        font-family: var(--mono);
        font-size: 11.5px;
        line-height: 1.65;
        color: var(--text-dim);
        white-space: pre;
    }

    .status {
        display: flex;
        align-items: center;
        gap: 8px;
        padding: 18px 16px;
        color: var(--text-dim);
    }
</style>
