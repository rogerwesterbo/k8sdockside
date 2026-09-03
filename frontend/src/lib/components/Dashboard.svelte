<!-- The overview tab: what the cluster is, how much of it is healthy, and what has gone wrong lately. -->
<script lang="ts">
    import { ResourceService } from '../../../bindings/github.com/roger/k8sdockside';
    import type * as kube from '../../../bindings/github.com/roger/k8sdockside/internal/kube/models.js';
    import { adoptOverview, type Overview } from '../state/adopt';
    import { workspace } from '../state/workspace.svelte';
    import ErrorState from './ErrorState.svelte';
    import SortableTable from './SortableTable.svelte';

    interface Props {
        contextId: string;
    }

    let { contextId }: Props = $props();

    let overview = $state<Overview | null>(null);
    let error = $state<string | null>(null);
    let loading = $state(true);
    /** Bumped by the retry button; the loading effect reads it as a dependency. */
    let attempt = $state(0);

    let color = $derived(workspace.colorOf(contextId));
    let context = $derived(workspace.contexts.find((c) => c.id === contextId) ?? null);

    $effect(() => {
        const id = contextId;
        attempt;
        let cancelled = false;
        loading = true;
        error = null;

        ResourceService.Overview(id)
            .then((result) => {
                if (cancelled) return;
                overview = adoptOverview(result);
                // A dashboard that loaded is better evidence than any ping, so
                // it settles the sidebar indicator for this context.
                workspace.reportHealth(id, 'connected');
            })
            .catch((err: unknown) => {
                if (cancelled) return;
                error = err instanceof Error ? err.message : String(err);
                workspace.reportHealth(id, 'error', error);
            })
            .finally(() => {
                if (!cancelled) loading = false;
            });

        return () => {
            cancelled = true;
        };
    });

    function percent(used: number, capacity: number): number {
        if (capacity <= 0) return 0;
        return Math.min(100, Math.round((used / capacity) * 100));
    }

    function statTone(stat: kube.Stat): string {
        if (stat.total === 0) return 'muted';
        return stat.ready === stat.total ? 'ok' : 'warn';
    }
</script>

<div class="dashboard">
    {#if loading && !overview}
        <p class="status">Loading cluster overview…</p>
    {:else if error}
        <ErrorState message={error} {context} onRetry={() => attempt++} />
    {:else if overview}
        <header class="head" style:--ctx-color={color}>
            <h1>{overview.context}</h1>
            <dl>
                {#if overview.server}<div><dt>Server</dt><dd class="selectable">{overview.server}</dd></div>{/if}
                <div><dt>Version</dt><dd>{overview.version}</dd></div>
                <div><dt>Distribution</dt><dd>{overview.distribution}</dd></div>
            </dl>
        </header>

        <section class="stats">
            {#each overview.stats as stat (stat.label)}
                <article class="stat {statTone(stat)}">
                    <p class="value">
                        {stat.ready}<span class="of">/{stat.total}</span>
                    </p>
                    <p class="label">{stat.label}</p>
                </article>
            {/each}
        </section>

        <section class="gauges">
            <h2>Capacity</h2>
            {#each overview.gauges as gauge (gauge.label)}
                {@const pct = percent(gauge.used, gauge.capacity)}
                <div class="gauge">
                    <div class="gauge-head">
                        <span>{gauge.label}</span>
                        <span class="gauge-value">
                            {gauge.used}{gauge.unit ? ` ${gauge.unit}` : ''} of {gauge.capacity}{gauge.unit ? ` ${gauge.unit}` : ''}
                            <span class="pct">{pct}%</span>
                        </span>
                    </div>
                    <div class="track">
                        <div class="fill" class:high={pct >= 80} style:width="{pct}%" style:background={color}></div>
                    </div>
                </div>
            {/each}
        </section>

        <section class="events">
            <h2>Recent events</h2>
            {#if overview.events.error}
                <p class="status quiet">{overview.events.error}</p>
            {:else}
                <div class="events-table">
                    <SortableTable
                        columns={overview.events.columns}
                        rows={overview.events.rows}
                        empty="Nothing to report."
                    />
                </div>
            {/if}
        </section>
    {/if}
</div>

<style>
    .dashboard {
        height: 100%;
        overflow: auto;
        padding: 20px 24px 32px;
    }

    .status {
        display: flex;
        align-items: center;
        gap: 8px;
        color: var(--text-dim);
        padding: 24px 0;
    }

    .status.quiet {
        padding: 8px 0;
        font-size: 12px;
    }

    .head {
        border-left: 3px solid var(--ctx-color);
        padding-left: 14px;
        margin-bottom: 22px;
    }

    h1 {
        margin: 0 0 8px;
        font-size: 20px;
        font-weight: 600;
    }

    h2 {
        margin: 0 0 10px;
        font-size: 11px;
        letter-spacing: 0.08em;
        text-transform: uppercase;
        color: var(--text-faint);
        font-weight: 600;
    }

    .head dl {
        display: flex;
        flex-wrap: wrap;
        gap: 4px 22px;
        margin: 0;
        font-size: 12px;
    }

    .head dl > div {
        display: flex;
        gap: 7px;
    }

    dt {
        color: var(--text-faint);
    }

    dd {
        margin: 0;
        color: var(--text-dim);
        font-family: var(--mono);
        font-size: 11.5px;
    }

    .stats {
        display: grid;
        grid-template-columns: repeat(auto-fit, minmax(150px, 1fr));
        gap: 10px;
        margin-bottom: 26px;
    }

    .stat {
        background: var(--bg-panel);
        border: 1px solid var(--border);
        border-radius: var(--radius);
        padding: 14px 16px;
    }

    .value {
        margin: 0;
        font-size: 26px;
        font-weight: 600;
        line-height: 1.1;
        font-variant-numeric: tabular-nums;
    }

    .stat.ok .value {
        color: var(--ok);
    }

    .stat.warn .value {
        color: var(--warn);
    }

    .stat.muted .value {
        color: var(--text-dim);
    }

    .of {
        font-size: 15px;
        font-weight: 400;
        color: var(--text-faint);
    }

    .label {
        margin: 4px 0 0;
        font-size: 12px;
        color: var(--text-dim);
    }

    .gauges {
        margin-bottom: 26px;
    }

    .gauge + .gauge {
        margin-top: 12px;
    }

    .gauge-head {
        display: flex;
        justify-content: space-between;
        align-items: baseline;
        gap: 12px;
        font-size: 12px;
        margin-bottom: 5px;
    }

    .gauge-value {
        color: var(--text-faint);
        font-size: 11px;
        font-family: var(--mono);
    }

    .pct {
        color: var(--text-dim);
        margin-left: 6px;
    }

    .track {
        height: 6px;
        border-radius: 3px;
        background: var(--bg-raised);
        overflow: hidden;
    }

    .fill {
        height: 100%;
        border-radius: 3px;
        transition: width 220ms ease;
    }

    .fill.high {
        background: var(--warn) !important;
    }

    /* The events panel is the shared table; it only needs a frame and a bound
       on its height, since the dashboard scrolls as a whole. */
    .events-table {
        border: 1px solid var(--border);
        border-radius: var(--radius);
        overflow: auto;
        max-height: 340px;
    }
</style>
