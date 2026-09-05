<!--
  What the detail panel shows for a Helm release, in place of a describe report.

  A release has no Kubernetes kind, so there is nothing for the describe path to
  read: what stands in for it is the release's own record, which Helm keeps in a
  Secret and already holds everything worth showing. See internal/kube/helmdetail.go.

  The order is the order the questions get asked. What is this and is it healthy,
  first, because that is why the drawer was opened. Then the values, which are
  the thing anyone actually came to check. Then the notes, the objects it owns
  and where it has been -- each behind a fold, because the panel is narrow and
  all four at once would be a scroll rather than a view.
-->
<script lang="ts">
    import { untrack } from 'svelte';
    import { helm, type ReleaseRef } from '../state/helm.svelte';
    import ErrorState from './ErrorState.svelte';

    let { release }: { release: ReleaseRef } = $props();

    let record = $derived(helm.stateOf(release));
    let detail = $derived(record.detail);

    /**
     * Whether the values shown are the overrides alone.
     *
     * Off by default, matching `helm get values --all`: the merged document is
     * the one that answers "what is this release doing", and the overrides on
     * their own answer the narrower "what did we change", which is the second
     * question rather than the first.
     */
    let userValuesOnly = $state(false);

    // A new release is a new drawer: read it, and drop the toggle back to where
    // it started rather than carrying one release's answer onto the next.
    //
    // The read is untracked because it writes what this component then displays.
    // Naming the store as a dependency would make the effect re-run on its own
    // result, and re-read forever. What it depends on is which release it is
    // pointed at, which is the three fields named here.
    $effect(() => {
        const ref = {
            contextId: release.contextId,
            namespace: release.namespace,
            name: release.name,
        };
        untrack(() => {
            userValuesOnly = false;
            void helm.load(ref);
        });
    });

    let values = $derived(userValuesOnly ? (detail?.userValues ?? '') : (detail?.values ?? ''));
    let resources = $derived(detail?.resources ?? []);
    let revisions = $derived(detail?.revisions ?? []);

    /**
     * The same four tones the resource tables use, so a status reads the same
     * colour here as it does in the row this drawer was opened from.
     */
    function toneOf(status: string): string {
        switch (status) {
            case 'deployed':
            case 'superseded':
                return status === 'deployed' ? 'ok' : 'info';
            case 'pending-install':
            case 'pending-upgrade':
            case 'pending-rollback':
            case 'uninstalling':
                return 'warn';
            case 'failed':
                return 'error';
            default:
                return '';
        }
    }

    /** An RFC3339 stamp as something readable, or a dash where there is none. */
    function when(stamp: string): string {
        if (!stamp) return '—';
        const at = new Date(stamp);
        if (Number.isNaN(at.getTime())) return stamp;
        return at.toLocaleString();
    }
</script>

{#if record.loading && !detail}
    <p class="status">Reading {release.name}…</p>
{:else if record.error}
    <ErrorState message={record.error} compact />
{:else if detail}
    <div class="release">
        <dl class="facts">
            <div><dt>Chart</dt><dd class="selectable">{detail.chart}</dd></div>
            <div><dt>Status</dt><dd class={toneOf(detail.status)}>{detail.status}</dd></div>
            {#if detail.appVersion}
                <div><dt>App version</dt><dd class="selectable">{detail.appVersion}</dd></div>
            {/if}
            <div><dt>Revision</dt><dd>{detail.revision}</dd></div>
            <div><dt>Namespace</dt><dd class="selectable">{detail.namespace}</dd></div>
            <div><dt>Updated</dt><dd>{when(detail.updated)}</dd></div>
            <div><dt>First deployed</dt><dd>{when(detail.firstDeployed)}</dd></div>
            {#if detail.description}
                <!-- Helm's own log entry for the revision: "Upgrade complete",
                     or the reason it is not. On a failed release this line is
                     the whole answer. -->
                <div><dt>Last action</dt><dd class="selectable">{detail.description}</dd></div>
            {/if}
        </dl>

        <details class="fold" open>
            <summary>
                <span>Values</span>
                <span class="count">{userValuesOnly ? 'user-supplied' : 'merged'}</span>
            </summary>

            <label class="toggle">
                <input type="checkbox" bind:checked={userValuesOnly} />
                User-supplied values only
            </label>

            {#if values}
                <pre class="selectable">{values}</pre>
            {:else if userValuesOnly}
                <!-- Worth saying rather than showing an empty box: a release
                     installed with no overrides is running the chart exactly as
                     it ships, which is a fact about it. -->
                <p class="empty">Installed with no overrides — the chart's own defaults, unchanged.</p>
            {:else}
                <p class="empty">This chart declares no values.</p>
            {/if}
        </details>

        {#if detail.notes}
            <details class="fold">
                <summary><span>Notes</span></summary>
                <pre class="selectable notes">{detail.notes}</pre>
            </details>
        {/if}

        <details class="fold" open>
            <summary>
                <span>Resources</span>
                <span class="count">{resources.length}</span>
            </summary>

            {#if resources.length > 0}
                <table>
                    <tbody>
                        {#each resources as resource (resource.apiVersion + resource.kind + resource.namespace + resource.name)}
                            <tr>
                                <td class="kind">{resource.kind}</td>
                                <td class="selectable">{resource.name}</td>
                                <td class="ns">{resource.namespace || '—'}</td>
                            </tr>
                        {/each}
                    </tbody>
                </table>
            {:else}
                <p class="empty">This release rendered no objects.</p>
            {/if}
        </details>

        <details class="fold">
            <summary>
                <span>History</span>
                <span class="count">{revisions.length}</span>
            </summary>

            <table>
                <tbody>
                    {#each revisions as revision (revision.revision)}
                        <tr class:current={revision.current}>
                            <td class="rev">{revision.revision}</td>
                            <td class={toneOf(revision.status)}>{revision.status}</td>
                            <td class="ns">{revision.chart}</td>
                            <td class="selectable">{revision.description}</td>
                        </tr>
                    {/each}
                </tbody>
            </table>
        </details>
    </div>
{/if}

<style>
    .release {
        padding: 12px 14px 20px;
        display: flex;
        flex-direction: column;
        gap: 14px;
        min-width: 0;
    }

    .status {
        padding: 18px 16px;
        color: var(--text-dim);
    }

    /* The facts, as label-over-value pairs that wrap rather than a two-column
       grid: the panel is resizable down to something narrow, and a fixed
       column would truncate a chart name long before it had to. */
    .facts {
        display: grid;
        grid-template-columns: repeat(auto-fit, minmax(140px, 1fr));
        gap: 10px 16px;
        margin: 0;
    }

    .facts div {
        min-width: 0;
    }

    dt {
        font-size: 10px;
        letter-spacing: 0.06em;
        text-transform: uppercase;
        color: var(--text-faint);
    }

    dd {
        margin: 2px 0 0;
        font-size: 12px;
        overflow-wrap: anywhere;
    }

    /* The four tones the tables use, so a status is the same colour wherever
       it is met. */
    dd.ok,
    td.ok {
        color: var(--ok);
    }

    dd.warn,
    td.warn {
        color: var(--warn);
    }

    dd.error,
    td.error {
        color: var(--error);
    }

    dd.info,
    td.info {
        color: var(--text-dim);
    }

    .fold {
        border-top: 1px solid var(--border);
        padding-top: 10px;
        min-width: 0;
    }

    summary {
        display: flex;
        align-items: baseline;
        gap: 8px;
        cursor: pointer;
        font-size: 11px;
        font-weight: 600;
        letter-spacing: 0.04em;
        text-transform: uppercase;
        color: var(--text-dim);
        list-style-position: outside;
    }

    summary:hover {
        color: var(--text);
    }

    .count {
        font-weight: 400;
        letter-spacing: 0;
        text-transform: none;
        color: var(--text-faint);
    }

    .toggle {
        display: flex;
        align-items: center;
        gap: 6px;
        margin: 10px 0 6px;
        font-size: 11px;
        color: var(--text-dim);
        cursor: pointer;
    }

    /* Values and notes are documents rather than fields: they keep their own
       whitespace and scroll sideways inside the fold rather than forcing the
       panel to. */
    pre {
        margin: 8px 0 0;
        padding: 10px 12px;
        background: var(--bg);
        border: 1px solid var(--border-soft);
        border-radius: var(--radius-sm);
        font-family: var(--mono);
        font-size: 11px;
        line-height: 1.6;
        color: var(--text-dim);
        white-space: pre;
        overflow-x: auto;
        max-height: 420px;
    }

    pre.notes {
        white-space: pre-wrap;
        overflow-wrap: anywhere;
    }

    .empty {
        margin: 8px 0 0;
        font-size: 11.5px;
        color: var(--text-faint);
    }

    table {
        width: 100%;
        margin-top: 8px;
        border-collapse: collapse;
        font-size: 11.5px;
    }

    td {
        padding: 3px 8px 3px 0;
        vertical-align: top;
        overflow-wrap: anywhere;
    }

    .kind,
    .rev {
        color: var(--text-faint);
        white-space: nowrap;
    }

    .ns {
        color: var(--text-dim);
    }

    /* The revision the release is actually on, which is the one the rest of the
       drawer is describing. */
    tr.current td {
        color: var(--text);
        font-weight: 600;
    }
</style>
