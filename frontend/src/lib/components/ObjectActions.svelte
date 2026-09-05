<!--
  The bar of things you can do to the object the panel is describing.

  Which buttons appear is the catalogue's answer (../actions.ts) and what they
  do is the store's (../state/actions.svelte.ts). What is here is the middle:
  asking before the two that cannot be undone, holding the replica count while
  it is typed, and showing a drain as it works.

  A question replaces the bar rather than opening a dialog over it, so that
  "Delete web?" is read in the same place the button was pressed and cannot be
  mistaken for a question about something else.
-->
<script lang="ts">
    import { singularFor } from '../catalogue';
    import { actionsFor, type Action, type ActionId } from '../actions';
    import { actions } from '../state/actions.svelte';
    import { forwards, type PortOption } from '../state/forwards.svelte';
    import { workspace, type DetailTarget } from '../state/workspace.svelte';
    import Icon from './Icon.svelte';

    // Named `object` rather than `target`: `target` is one of Svelte's own
    // mount options, and a prop by that name is taken for the element to mount
    // into rather than passed through.
    let { object }: { object: DetailTarget } = $props();

    let available = $derived(actionsFor(object.kind));
    let facts = $derived(actions.stateOf(object));
    let drain = $derived(actions.drainOf(object));

    /** The action waiting on an answer -- a confirmation, a number, a port -- if any. */
    let asking = $state<ActionId | null>(null);
    let replicas = $state(0);
    let busy = $state(false);

    /**
     * What a forward could be opened on, read from the object when the form is
     * opened. A pod or a workload answers with its container ports, a service
     * with its own -- which are not the same thing, and the difference is why
     * this is a list from the cluster rather than a number field.
     */
    let ports = $state<PortOption[]>([]);
    let portsError = $state('');
    let loadingPorts = $state(false);
    /**
     * The chosen remote port, and the local one.
     *
     * Both are nullable because that is what a number field bound to an empty
     * box holds, and for the local one empty is a real answer: it means "any
     * free port", which is what the app asks for unless told otherwise.
     */
    let remotePort = $state<number | null>(null);
    let localPort = $state<number | null>(null);
    let openBrowser = $state(true);
    /** The confirmation's safe button, focused so a stray Enter cannot destroy. */
    let cancelEl = $state<HTMLButtonElement | null>(null);

    /** What the object is, for the questions: "pod web", "node wrkr01". */
    let subject = $derived(`${singularFor(object.kind)} ${object.name}`);

    // A new object is a new bar. Read what its buttons need to say, and drop
    // any half-asked question belonging to the object we have left.
    $effect(() => {
        const ref = { ...object };
        asking = null;
        ports = [];
        portsError = '';
        void actions.load(ref);
    });

    // Focus the safe answer as soon as a question appears.
    $effect(() => {
        if (asking) cancelEl?.focus();
    });

    /**
     * Cordon is the one button whose label is the cluster's answer rather than
     * ours: offering to cordon a node that is already cordoned is a button that
     * does nothing.
     */
    function labelOf(action: Action): string {
        if (action.id === 'cordon') return facts.cordoned ? 'Uncordon' : 'Cordon';
        return action.label;
    }

    function choose(action: Action): void {
        if (action.id === 'edit') {
            workspace.openEditor(object);
            return;
        }
        if (action.id === 'logs') {
            workspace.openLogs(object);
            return;
        }
        if (action.id === 'shell') {
            workspace.openShell(object);
            return;
        }
        if (action.form === 'ports') {
            asking = action.id;
            void loadPorts();
            return;
        }
        if (action.form === 'immediate') {
            void perform(action.id);
            return;
        }
        if (action.form === 'number') replicas = facts.replicas;
        asking = action.id;
    }

    /**
     * Runs one action and says how it went.
     *
     * The API server's own words are what a failure reports: a denied action
     * says which verb on which resource was refused, which is more use than
     * anything this component could write.
     */
    async function perform(id: ActionId, value = 0): Promise<void> {
        busy = true;
        try {
            switch (id) {
                case 'delete':
                    await actions.remove(object);
                    workspace.inform(`${subject} deleted`);
                    // Nothing is left to describe.
                    workspace.closeDetail();
                    return;
                case 'scale':
                    await actions.scale(object, value);
                    workspace.inform(`${subject} scaled to ${value}`);
                    break;
                case 'restart':
                    await actions.restart(object);
                    workspace.inform(`${subject} restarting`);
                    break;
                case 'cordon':
                    await actions.cordon(object, !facts.cordoned);
                    workspace.inform(`${subject} ${facts.cordoned ? 'cordoned' : 'uncordoned'}`);
                    break;
                case 'drain':
                    await actions.drain(object);
                    break;
            }
            asking = null;
        } catch (err) {
            workspace.fail(err instanceof Error ? err.message : String(err));
            asking = null;
        } finally {
            busy = false;
        }
    }

    /**
     * Reads the ports this object could be forwarded from.
     *
     * A failure is shown in the form rather than swallowed: "this service
     * selects no pods" is the answer to why there is nothing to choose, and it
     * is more use than an empty list.
     */
    async function loadPorts(): Promise<void> {
        loadingPorts = true;
        portsError = '';
        try {
            const found = await forwards.ports(object);
            ports = found;
            // The first port is the one almost always wanted, and a form that
            // opens on a chosen value is one field shorter to fill in.
            remotePort = found[0]?.port ?? null;
            localPort = null;
            openBrowser = true;
        } catch (err) {
            portsError = err instanceof Error ? err.message : String(err);
        } finally {
            loadingPorts = false;
        }
    }

    /**
     * Opens the forward the form describes.
     *
     * The local port is left empty by default, which means "any free one" --
     * the port is normally reached by the link the app puts beside it, and
     * choosing one by hand only matters when something else already expects it.
     */
    async function forward(): Promise<void> {
        const remote = remotePort ?? 0;
        // Empty means "any free port", which is the whole reason this field is
        // allowed to be empty.
        const local = localPort ?? 0;
        if (remote <= 0 || remote > 65535) {
            workspace.fail(`${remote} is not a port`);
            return;
        }
        if (local < 0 || local > 65535) {
            workspace.fail(`${local} is not a port`);
            return;
        }

        busy = true;
        try {
            const opened = await forwards.start(object, remote, local, openBrowser);
            workspace.inform(
                `Forwarding localhost:${opened.localPort} to ${object.name} on ${remote}`,
            );
            asking = null;
        } catch (err) {
            workspace.fail(err instanceof Error ? err.message : String(err));
        } finally {
            busy = false;
        }
    }

    /** How one port reads in the picker: its number, name and where it lands. */
    function portLabel(port: PortOption): string {
        const parts = [String(port.port)];
        if (port.name) parts.push(port.name);
        if (port.target && port.target !== String(port.port)) parts.push(`→ ${port.target}`);
        if (port.protocol && port.protocol !== 'TCP') parts.push(port.protocol);
        return parts.join(' · ');
    }

    /** The question each asked-for action puts. */
    function question(id: ActionId): string {
        if (id === 'drain') return `Drain ${subject}? Everything running on it will be moved.`;
        return `Delete ${subject}?`;
    }

    let asked = $derived(available.find((a) => a.id === asking) ?? null);
</script>

<svelte:document onkeydown={(e) => e.key === 'Escape' && asking && (asking = null)} />

{#if available.length > 0}
    <div class="bar">
        {#if asked && asked.form === 'confirm'}
            <p class="question">{question(asked.id)}</p>
            <div class="answers">
                <button bind:this={cancelEl} class="plain" onclick={() => (asking = null)}>Cancel</button>
                <button class="go" class:danger={asked.tone === 'danger'} disabled={busy} onclick={() => perform(asked.id)}>
                    {asked.label}
                </button>
            </div>
        {:else if asked && asked.form === 'ports'}
            {#if loadingPorts}
                <p class="question">Reading {subject}'s ports…</p>
            {:else if portsError}
                <p class="question failed">{portsError}</p>
            {:else if ports.length === 0}
                <p class="question">
                    {subject} declares no ports. You can still forward one by typing it.
                </p>
            {/if}

            {#if !loadingPorts && !portsError}
                <label class="field">
                    Port
                    {#if ports.length > 0}
                        <select bind:value={remotePort}>
                            {#each ports as port (port.port + port.name)}
                                <option value={port.port}>{portLabel(port)}</option>
                            {/each}
                        </select>
                    {:else}
                        <input type="number" min="1" max="65535" bind:value={remotePort} />
                    {/if}
                </label>

                <label class="field">
                    Local
                    <input
                        type="number"
                        min="0"
                        max="65535"
                        placeholder="any free port"
                        bind:value={localPort}
                    />
                </label>

                <label class="check">
                    <input type="checkbox" bind:checked={openBrowser} />
                    Open a browser
                </label>
            {/if}

            <div class="answers">
                <button bind:this={cancelEl} class="plain" onclick={() => (asking = null)}>Cancel</button>
                <button
                    class="go"
                    disabled={busy || loadingPorts || !remotePort || remotePort <= 0}
                    onclick={() => forward()}
                >
                    Forward
                </button>
            </div>
        {:else if asked && asked.form === 'number'}
            <label class="scale">
                Replicas
                <input type="number" min="0" bind:value={replicas} />
            </label>
            <div class="answers">
                <button bind:this={cancelEl} class="plain" onclick={() => (asking = null)}>Cancel</button>
                <button class="go" disabled={busy} onclick={() => perform('scale', replicas)}>Apply</button>
            </div>
        {:else}
            {#each available as action (action.id)}
                <button
                    class:danger={action.tone === 'danger'}
                    class:last={action.tone === 'danger'}
                    disabled={busy}
                    title={labelOf(action)}
                    onclick={() => choose(action)}
                >
                    <Icon name={action.icon} size={13} />
                    {labelOf(action)}
                </button>
            {/each}
        {/if}
    </div>

    {#if drain}
        <!-- A drain outlives the click that started it, so it reports under the
             bar rather than in a notice that would have gone by the time the
             first pod is evicted. -->
        <div class="drain" class:failed={drain.error !== ''}>
            <div class="line">
                {#if drain.error}
                    <span class="what">Drain failed: {drain.error}</span>
                {:else if drain.done}
                    <span class="what">Drained — {drain.evicted} of {drain.total} pods moved</span>
                {:else if drain.phase === 'evicting'}
                    <span class="what">Draining — {drain.evicted} of {drain.total} pods moved</span>
                {:else}
                    <span class="what">Draining — {drain.phase}</span>
                {/if}

                {#if !drain.done}
                    <button class="plain" onclick={() => actions.cancelDrain(object)}>Stop</button>
                {/if}
            </div>

            {#if drain.total > 0 && !drain.error}
                <div class="track" role="progressbar" aria-valuenow={drain.evicted} aria-valuemin={0} aria-valuemax={drain.total}>
                    <div class="fill" style:width="{(drain.evicted / drain.total) * 100}%"></div>
                </div>
            {/if}

            {#if drain.refused.length > 0}
                <ul class="refused">
                    {#each drain.refused as refusal (refusal.pod.namespace + refusal.pod.name)}
                        <li>
                            <strong>{refusal.pod.namespace}/{refusal.pod.name}</strong>
                            — left behind: {refusal.reason}
                        </li>
                    {/each}
                </ul>
            {/if}
        </div>
    {/if}
{/if}

<style>
    .bar {
        display: flex;
        align-items: center;
        gap: 6px;
        padding: 8px 12px;
        border-bottom: 1px solid var(--border);
        flex: 0 0 auto;
        min-height: 40px;
    }

    button {
        display: flex;
        align-items: center;
        gap: 6px;
        height: 24px;
        padding: 0 10px;
        border-radius: var(--radius-sm);
        background: var(--bg-raised);
        box-shadow: inset 0 0 0 1px var(--border);
        font-size: 12px;
        color: var(--text);
        white-space: nowrap;
    }

    button:hover:not(:disabled) {
        background: var(--bg-hover);
    }

    button:disabled {
        opacity: 0.5;
    }

    /* The one that cannot be undone is pushed to the far end and coloured
       apart, so it is never the button next to the one you meant. */
    .last {
        margin-left: auto;
    }

    .danger {
        color: var(--error);
    }

    .danger:hover:not(:disabled) {
        background: color-mix(in srgb, var(--error) 16%, transparent);
    }

    .plain {
        background: none;
        box-shadow: none;
        color: var(--text-dim);
    }

    .question {
        margin: 0;
        font-size: 12px;
        color: var(--text);
        min-width: 0;
        overflow-wrap: anywhere;
    }

    .answers {
        display: flex;
        gap: 6px;
        margin-left: auto;
        flex: 0 0 auto;
    }

    .scale {
        display: flex;
        align-items: center;
        gap: 8px;
        font-size: 12px;
        color: var(--text-dim);
    }

    .field {
        display: flex;
        align-items: center;
        gap: 6px;
        font-size: 12px;
        color: var(--text-dim);
        white-space: nowrap;
    }

    .field select,
    .field input {
        height: 24px;
        padding: 0 6px;
        border-radius: var(--radius-sm);
        background: var(--bg);
        box-shadow: inset 0 0 0 1px var(--border);
        color: var(--text);
        font: inherit;
        font-size: 12px;
        max-width: 170px;
    }

    .check {
        display: flex;
        align-items: center;
        gap: 6px;
        font-size: 12px;
        color: var(--text-dim);
        white-space: nowrap;
    }

    .question.failed {
        color: var(--error);
    }

    .scale input {
        width: 72px;
        height: 24px;
        padding: 0 8px;
        border-radius: var(--radius-sm);
        background: var(--bg);
        box-shadow: inset 0 0 0 1px var(--border);
        color: var(--text);
        font: inherit;
        font-size: 12px;
    }

    .drain {
        padding: 8px 12px 10px;
        border-bottom: 1px solid var(--border);
        background: var(--bg);
        font-size: 11.5px;
        color: var(--text-dim);
    }

    .line {
        display: flex;
        align-items: center;
        gap: 8px;
    }

    .what {
        flex: 1 1 auto;
        min-width: 0;
        overflow-wrap: anywhere;
    }

    .failed .what {
        color: var(--error);
    }

    .track {
        margin-top: 6px;
        height: 3px;
        border-radius: 2px;
        background: var(--bg-active);
        overflow: hidden;
    }

    .fill {
        height: 100%;
        background: var(--ok);
        transition: width 180ms ease;
    }

    .refused {
        margin: 8px 0 0;
        padding: 0 0 0 14px;
        display: flex;
        flex-direction: column;
        gap: 3px;
    }

    .refused li {
        color: var(--warn);
    }

    .refused strong {
        font-weight: 600;
        color: var(--text);
    }
</style>
