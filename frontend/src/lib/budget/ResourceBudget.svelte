<!--
  What a cluster, a node or a namespace has, what has been promised out of it,
  and what is actually being used.

  The five numbers are kept apart on purpose. Capacity and allocatable say what
  exists, requests and limits say what the scheduler has booked and what those
  pods may grow into, and usage says what is really happening. A cluster can be
  full by requests and idle by usage at the same time, and that gap is the whole
  reason for putting them on one screen.

  Usage is the only part that needs anything installed. Where neither
  metrics-server nor Prometheus answered, the used bar is left out and the
  reason is written under the heading — a bar sitting at zero would read as a
  cluster doing nothing.
-->
<script lang="ts">
    import { ResourceService } from '../../../bindings/github.com/roger/k8sdockside';
    import { formatValue, type Unit } from '../charts/format';
    import { adoptBudget, barsFor, ceilingOf, type Bar, type Budget } from './adopt';

    interface Props {
        contextId: string;
        /** `cluster`, `node` or `namespace`. */
        scope: string;
        /** The node or namespace. Ignored for a cluster. */
        name?: string;
        title?: string;
        /** Narrow layout for the detail panel. */
        compact?: boolean;
    }

    let { contextId, scope, name = '', title = 'Resources', compact = false }: Props = $props();

    /** Requests move when anything is scheduled, so this does not sit stale. */
    const REFRESH_MS = 30_000;

    let budget = $state<Budget | null>(null);
    let error = $state<string | null>(null);
    let firstLoad = $state(true);

    async function load(): Promise<void> {
        try {
            budget = adoptBudget(await ResourceService.Budget(contextId, scope, name));
            error = null;
        } catch (err: unknown) {
            error = err instanceof Error ? err.message : String(err);
        } finally {
            firstLoad = false;
        }
    }

    $effect(() => {
        // Named so the effect re-runs when any of them change.
        contextId;
        scope;
        name;

        let live = true;
        void (async () => {
            if (live) await load();
        })();

        const timer = setInterval(() => {
            if (live) void load();
        }, REFRESH_MS);

        return () => {
            live = false;
            clearInterval(timer);
        };
    });

    let amounts = $derived(budget?.amounts ?? []);
    let usage = $derived(budget?.usage ?? null);

    /** The ceiling written out, for the line beside each dimension's name. */
    function ceilingText(amount: (typeof amounts)[number], bars: Bar[]): string {
        const ceiling = ceilingOf(amount);
        if (ceiling <= 0) {
            // Say which it is. A namespace with no quota is not capped at zero,
            // and the bars beside this text are proportions of each other
            // rather than of anything the namespace owns.
            return bars.some((b) => b.track) ? 'no quota — bars are relative' : 'no quota';
        }

        const unit = amount.unit as Unit;
        const written = formatValue(ceiling, unit);
        // Where the two differ, saying so is the point: the gap is what the
        // kubelet keeps back and it is otherwise invisible.
        if (amount.capacity > 0 && amount.allocatable > 0 && amount.capacity !== amount.allocatable) {
            return `${written} allocatable of ${formatValue(amount.capacity, unit)}`;
        }
        return `${written} allocatable`;
    }
</script>

<section class="budget" class:compact>
    <header>
        <h2>{title}</h2>
        {#if usage?.source}
            <span class="source" title="Live usage is read from {usage.source}">{usage.source}</span>
        {/if}
    </header>

    {#if error}
        <p class="note failed">{error}</p>
    {:else if budget?.error}
        <p class="note failed">{budget.error}</p>
    {:else if firstLoad}
        <p class="note">Adding it up…</p>
    {:else}
        {#if usage && !usage.source && usage.error}
            <!-- Not a failure: a cluster with no metrics stack is a normal
                 cluster, and everything above still works. -->
            <p class="note quiet" title={usage.error}>
                No live usage — {usage.error}. Requests and limits come from the API server and are unaffected.
            </p>
        {/if}

        {#each amounts as amount (amount.label)}
            {@const bars = barsFor(amount)}
            <article class="amount">
                <div class="amount-head">
                    <span class="name">{amount.label}</span>
                    <span class="ceiling">{ceilingText(amount, bars)}</span>
                </div>

                {#each bars as bar (bar.label)}
                    <div class="bar">
                        <span class="bar-label">{bar.label}</span>
                        {#if bar.track}
                            <div class="track" class:relative={bar.relative}>
                                <div
                                    class="fill {bar.label.toLowerCase()}"
                                    class:over={bar.overcommitted}
                                    style:width="{bar.percent}%"
                                ></div>
                            </div>
                        {:else}
                            <span class="no-track"></span>
                        {/if}
                        <span class="bar-value">
                            {formatValue(bar.value, amount.unit as Unit)}
                            {#if bar.overcommitted}<span class="over-tag" title="More than the node can actually give out">over</span>{/if}
                        </span>
                    </div>
                {/each}
            </article>
        {/each}
    {/if}
</section>

<style>
    .budget {
        margin-bottom: 26px;
    }

    /* In the detail panel the section is the full width of the dock, so it
       brings its own gutters -- matching the describe report below it, which
       is what it has to line up with. */
    .budget.compact {
        padding: 14px 16px 0;
        margin-bottom: 18px;
    }

    header {
        display: flex;
        align-items: baseline;
        justify-content: space-between;
        gap: 10px;
    }

    h2 {
        margin: 0 0 10px;
        font-size: 11px;
        letter-spacing: 0.08em;
        text-transform: uppercase;
        color: var(--text-faint);
        font-weight: 600;
    }

    .source {
        font-size: 10.5px;
        font-family: var(--mono);
        color: var(--text-faint);
    }

    .note {
        color: var(--text-dim);
        font-size: 12px;
        margin: 0 0 12px;
    }

    .note.quiet {
        font-size: 11.5px;
        color: var(--text-faint);
    }

    .note.failed {
        color: var(--error, var(--warn));
    }

    .amount + .amount {
        margin-top: 16px;
    }

    .amount-head {
        display: flex;
        justify-content: space-between;
        align-items: baseline;
        gap: 12px;
        font-size: 12px;
        margin-bottom: 6px;
    }

    .ceiling {
        color: var(--text-faint);
        font-size: 11px;
        font-family: var(--mono);
    }

    /* Three columns so the bars line up under each other and can be read as
       one stack rather than three unrelated rows. */
    .bar {
        display: grid;
        grid-template-columns: 68px 1fr 110px;
        align-items: center;
        gap: 10px;
        margin-top: 4px;
    }

    .compact .bar {
        grid-template-columns: 60px 1fr 88px;
        gap: 8px;
    }

    .bar-label {
        font-size: 11px;
        color: var(--text-dim);
    }

    .bar-value {
        font-size: 11px;
        font-family: var(--mono);
        color: var(--text-faint);
        text-align: right;
        font-variant-numeric: tabular-nums;
    }

    .track {
        height: 6px;
        border-radius: 3px;
        background: var(--bg-raised);
        overflow: hidden;
    }

    /* Scaled against the other bars rather than a ceiling: a dashed end says
       the bar does not run out at a wall. */
    .track.relative {
        background: repeating-linear-gradient(
            90deg,
            var(--bg-raised) 0 4px,
            transparent 4px 7px
        );
    }

    .no-track {
        display: block;
    }

    .fill {
        height: 100%;
        border-radius: 3px;
        transition: width 220ms ease;
        background: var(--text-faint);
    }

    /* Requested is the booking, limits the ceiling those pods may reach, used
       what is really happening -- distinct enough to tell apart at a glance,
       and used is the one the eye should land on. */
    .fill.requested {
        background: var(--accent, #6ea8fe);
    }

    .fill.limits {
        background: var(--text-faint);
        opacity: 0.55;
    }

    .fill.used {
        background: var(--ok);
    }

    .fill.over {
        background: var(--warn) !important;
        opacity: 1;
    }

    .over-tag {
        color: var(--warn);
        margin-left: 5px;
        font-size: 10px;
        text-transform: uppercase;
        letter-spacing: 0.04em;
    }
</style>
