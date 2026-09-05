<!--
  The YAML editor: one object, as the cluster has it, editable and saved back --
  or one Helm release's values, which is the same gesture on a different kind of
  document.

  A release is not an object, so its document is its user-supplied values and a
  save is `helm upgrade` rather than an apply. Two things follow that this
  component has to show and an object's editor does not: which chart the upgrade
  fetches, and which version of it. Neither can be derived -- Helm's release
  record keeps a chart's name and version but not where it came from -- so they
  are fields, filled in as far as the release can fill them. See
  ../state/editor.svelte.ts.

  CodeMirror rather than a textarea. A textarea holds one flat string, which
  makes two things impossible rather than merely absent: finding text with the
  matches marked, and folding a block away. Both are what you want in a document
  that is mostly nesting you are not reading. What the swap costs is a
  dependency; what it buys is search, folding, YAML highlighting and indent
  handling that someone else maintains.

  The document itself lives in the editors store, not here: this component is
  destroyed and rebuilt every time you switch dock tabs, and a half-finished
  edit must not go with it.
-->
<script lang="ts">
    import { EditorView } from '@codemirror/view';
    import { untrack } from 'svelte';
    import { singularFor } from '../catalogue';
    import { alpha } from '../colors';
    import { extensions, numbering, setBadLine } from '../editor/setup';
    import { lineNumbers } from '@codemirror/view';
    import { editors } from '../state/editor.svelte';
    import { helm } from '../state/helm.svelte';
    import { workspace, type DockTab } from '../state/workspace.svelte';
    import ErrorState from './ErrorState.svelte';
    import Icon from './Icon.svelte';

    interface Props {
        tab: DockTab;
    }

    let { tab }: Props = $props();

    let doc = $derived(editors.doc(tab.id));
    let dirty = $derived(editors.isDirty(tab.id));
    let color = $derived(workspace.colorOf(tab.contextId));
    let context = $derived(workspace.contexts.find((c) => c.id === tab.contextId) ?? null);

    /** Whether this tab holds a Helm release's values rather than an object. */
    let isRelease = $derived(tab.view === 'helmvalues');
    /**
     * Whether helm is on this machine, which only a release needs.
     *
     * Probed once when a release tab opens rather than read at the button: the
     * answer changes when the setting does, and `helm.probe` is what the
     * settings view calls then.
     */
    let helmMissing = $derived(isRelease && helm.probed && !helm.tool.found);

    /**
     * An upgrade is worth offering when there is something to change *and* a
     * chart to fetch. The version may be empty -- helm reads that as "whatever
     * the repository calls latest", which is a real thing to ask for.
     *
     * A release is savable even when the document is not dirty: bumping the
     * version alone is a change, and it is not in the text.
     */
    let changed = $derived(isRelease ? dirty || doc.version !== '' : dirty);
    let canSave = $derived(
        doc.status === 'ready' &&
            changed &&
            doc.check.valid &&
            !doc.saving &&
            (!isRelease || (doc.chart.trim() !== '' && !helmMissing)),
    );

    /** The versions the repositories offer for the chart in the field. */
    let versions = $state<string[]>([]);
    let lookingUp = $state(false);

    // Ask where helm is the first time a release tab is opened. Nothing else in
    // this component needs it, so an object's editor never makes the call.
    $effect(() => {
        if (isRelease && !helm.probed) void helm.probe();
    });

    /**
     * Asks the repositories what versions of this chart exist.
     *
     * On demand rather than as the field is typed: it reaches the network, and
     * a lookup per keystroke against a repo index would be both slow and rude.
     * Finding nothing is an ordinary answer -- an OCI or local chart is in no
     * index -- so it is reported quietly and the field stays typeable.
     */
    async function lookUpVersions(): Promise<void> {
        const chart = doc.chart.trim();
        if (!chart || lookingUp) return;
        lookingUp = true;
        try {
            const found = await helm.versions(chart);
            versions = found.map((v) => v.version);
            if (versions.length === 0) {
                workspace.inform(`No repository on this machine offers ${chart}`);
            }
        } catch (err) {
            workspace.fail(err instanceof Error ? err.message : String(err));
        } finally {
            lookingUp = false;
        }
    }

    let host = $state<HTMLElement | null>(null);
    let view: EditorView | null = null;
    /**
     * True while the store's text is being written into the editor, so the
     * update listener does not send it straight back and start a loop.
     */
    let applying = false;

    // Reads the object the first time this tab is opened. A document already in
    // the store is left exactly as it was, which is what makes switching away
    // from a half-finished edit safe.
    $effect(() => {
        void editors.load(tab.id, tab);
    });

    // Builds the editor once, on the element it lives in. Its starting text is
    // read untracked: this must not rebuild on every keystroke.
    $effect(() => {
        const parent = host;
        if (!parent) return;

        view = new EditorView({
            parent,
            doc: untrack(() => editors.doc(tab.id).text),
            extensions: [
                ...extensions({
                    numbers: untrack(() => workspace.showLineNumbers),
                    label: `${tab.name} as YAML`,
                    onSave: () => void save(),
                }),
                EditorView.updateListener.of((update) => {
                    if (update.docChanged && !applying) {
                        editors.edit(tab.id, update.state.doc.toString());
                    }
                }),
            ],
        });

        return () => {
            view?.destroy();
            view = null;
        };
    });

    // The store's text, written in when it changes for a reason other than
    // typing: the first read, a reload, and what the API server gave back after
    // a save. Compared first, so an echo of the user's own keystroke is a
    // no-op rather than a cursor thrown back to the start.
    $effect(() => {
        const text = doc.text;
        if (!view || view.state.doc.toString() === text) return;
        applying = true;
        view.dispatch({ changes: { from: 0, to: view.state.doc.length, insert: text } });
        applying = false;
    });

    // The line the parser stopped at, handed to the editor to mark.
    $effect(() => {
        const line = doc.check.valid ? 0 : doc.check.line;
        view?.dispatch({ effects: setBadLine.of(line) });
    });

    // Line numbering follows the setting without the editor being rebuilt.
    $effect(() => {
        const on = workspace.showLineNumbers;
        view?.dispatch({ effects: numbering.reconfigure(on ? lineNumbers() : []) });
    });

    async function save(): Promise<void> {
        if (!canSave) return;
        if (await editors.save(tab.id, tab)) {
            workspace.inform(
                isRelease
                    ? `${tab.name} upgraded to ${doc.chart}${doc.version ? ` ${doc.version}` : ''}`
                    : `${singularFor(tab.kind)} ${tab.name} saved`,
            );
        }
    }

    /** Why Upgrade is or is not available, as the button's tooltip. */
    function upgradeHint(): string {
        if (helmMissing) return 'helm was not found on this machine';
        if (doc.chart.trim() === '') return 'Name the chart this release should be upgraded to';
        if (!changed) return 'Nothing to upgrade';
        return 'Run helm upgrade with these values (⌘S)';
    }

    /** Re-reads the document, throwing away whatever was typed against it. */
    function reload(): void {
        if (dirty && !confirm(`Discard your changes to ${tab.name} and re-read it from the cluster?`)) {
            return;
        }
        void editors.load(tab.id, tab, { force: true });
    }
</script>

<div class="editor" style:--ctx-color={color} style:--ctx-tint={alpha(color, 0.1)}>
    <header>
        <div class="ident">
            <span class="kind">{isRelease ? 'Values' : singularFor(tab.kind)}</span>
            <span class="name selectable">{tab.name}</span>
            {#if tab.namespace}<span class="dim">in {tab.namespace}</span>{/if}
            {#if context}<span class="dim">· {workspace.displayName(context)}</span>{/if}
        </div>

        <div class="status" role="status">
            {#if doc.saving}
                <span class="dim">Saving…</span>
            {:else if !doc.check.valid}
                <span class="bad">
                    <Icon name="alert" size={12} />
                    {doc.check.line > 0 ? `Line ${doc.check.line}: ` : ''}{doc.check.message}
                </span>
            {:else if dirty}
                <span class="dim">Unsaved changes</span>
            {:else if doc.saved}
                <span class="good">Saved</span>
            {/if}
        </div>

        <div class="actions">
            <button class="ghost" onclick={reload} disabled={doc.status === 'loading'} title="Re-read this object from the cluster">
                <Icon name="refresh" size={13} />
                Reload
            </button>
            <button
                class="save"
                onclick={save}
                disabled={!canSave}
                title={isRelease ? upgradeHint() : dirty ? 'Save to the cluster (⌘S)' : 'No changes to save'}
            >
                <Icon name={isRelease ? 'rocket' : 'save'} size={13} />
                {isRelease ? 'Upgrade' : 'Save'}
            </button>
        </div>
    </header>

    {#if isRelease && doc.status === 'ready'}
        <!-- What an upgrade will fetch. Below the header rather than in it: the
             chart is not part of the release's identity, it is an argument to
             the thing the button does. -->
        <div class="chart">
            <label>
                <span>Chart</span>
                <input
                    type="text"
                    value={doc.chart}
                    oninput={(e) => editors.setChart(tab.id, { chart: e.currentTarget.value })}
                    placeholder="repo/chart, oci://…, or a path"
                    spellcheck="false"
                />
            </label>
            <label class="version">
                <span>Version</span>
                <input
                    type="text"
                    list="{tab.id}-versions"
                    value={doc.version}
                    oninput={(e) => editors.setChart(tab.id, { version: e.currentTarget.value })}
                    placeholder="latest"
                    spellcheck="false"
                />
                <datalist id="{tab.id}-versions">
                    {#each versions as version (version)}
                        <option value={version}></option>
                    {/each}
                </datalist>
            </label>
            <button class="ghost" onclick={lookUpVersions} disabled={lookingUp || doc.chart.trim() === ''}>
                <Icon name="search" size={12} />
                {lookingUp ? 'Looking…' : 'Versions'}
            </button>
        </div>

        {#if helmMissing}
            <!-- Not a failure of this view: everything above still works, and
                 the drawer behind it reads the release without helm at all. -->
            <p class="refused selectable">
                <Icon name="alert" size={13} />
                <span>{helm.tool.reason}</span>
            </p>
        {/if}
    {/if}

    {#if doc.error && doc.status === 'ready'}
        <!-- A refused save: the API server's own words, which is where the
             useful part is -- a conflict, an admission webhook, an RBAC rule. -->
        <p class="refused selectable">
            <Icon name="alert" size={13} />
            <span>{doc.error}</span>
        </p>
    {/if}

    {#if doc.status === 'loading'}
        <p class="loading">Reading {tab.name}…</p>
    {:else if doc.status === 'error'}
        <ErrorState message={doc.error} {context} onRetry={() => editors.load(tab.id, tab, { force: true })} compact />
    {:else}
        <!-- CodeMirror mounts itself in here. ⌘F searches, the fold arrows in
             the gutter collapse a block, and both are its own. -->
        <div class="pane" bind:this={host}></div>
    {/if}
</div>

<style>
    .editor {
        display: flex;
        flex-direction: column;
        height: 100%;
        min-height: 0;
        background: var(--bg-panel);
    }

    header {
        display: flex;
        align-items: center;
        gap: 14px;
        height: 34px;
        padding: 0 12px;
        flex: 0 0 auto;
        background: var(--ctx-tint);
        border-bottom: 1px solid var(--border);
        border-left: 3px solid var(--ctx-color);
    }

    .ident {
        display: flex;
        align-items: baseline;
        gap: 7px;
        min-width: 0;
        overflow: hidden;
        white-space: nowrap;
    }

    .kind {
        font-size: 10px;
        letter-spacing: 0.08em;
        text-transform: uppercase;
        color: var(--text-faint);
        flex: 0 0 auto;
    }

    .name {
        font-size: 13px;
        font-weight: 600;
        overflow: hidden;
        text-overflow: ellipsis;
    }

    .dim {
        font-size: 11px;
        color: var(--text-dim);
        overflow: hidden;
        text-overflow: ellipsis;
    }

    /* Between the identity and the buttons, so the eye passes over it on the
       way to Save rather than having to go looking. */
    .status {
        display: flex;
        align-items: center;
        gap: 6px;
        margin-left: auto;
        min-width: 0;
        font-size: 11.5px;
    }

    .status .bad {
        display: flex;
        align-items: center;
        gap: 5px;
        color: var(--error);
        overflow: hidden;
        text-overflow: ellipsis;
        white-space: nowrap;
    }

    .status .good {
        color: var(--ok);
    }

    .actions {
        display: flex;
        align-items: center;
        gap: 6px;
        flex: 0 0 auto;
    }

    .actions button {
        display: flex;
        align-items: center;
        gap: 6px;
        height: 24px;
        padding: 0 10px;
        border-radius: var(--radius-sm);
        font-size: 12px;
        color: var(--text-dim);
    }

    .ghost:hover:not(:disabled) {
        background: var(--bg-hover);
        color: var(--text);
    }

    .save {
        background: var(--accent);
        color: var(--accent-text);
    }

    .save:hover:not(:disabled) {
        filter: brightness(1.1);
    }

    .actions button:disabled {
        opacity: 0.45;
        cursor: default;
    }

    /* A disabled Save keeps its shape but gives up the accent: white on a
       faded blue is unreadable rather than merely quiet, and "nothing to save"
       should look like a resting control, not a broken one. */
    .save:disabled {
        background: var(--bg-raised);
        box-shadow: inset 0 0 0 1px var(--border);
        color: var(--text-faint);
        opacity: 1;
    }

    /* The chart an upgrade fetches, on its own strip under the header. The
       chart field takes the room because a reference is long -- an OCI URL or a
       path -- and a version is six characters. */
    .chart {
        display: flex;
        align-items: center;
        gap: 10px;
        padding: 7px 12px;
        flex: 0 0 auto;
        border-bottom: 1px solid var(--border);
        background: var(--bg);
    }

    .chart label {
        display: flex;
        align-items: center;
        gap: 6px;
        flex: 1 1 auto;
        min-width: 0;
        font-size: 11px;
        color: var(--text-faint);
    }

    .chart label.version {
        flex: 0 0 auto;
        width: 190px;
    }

    .chart span {
        flex: 0 0 auto;
        letter-spacing: 0.04em;
        text-transform: uppercase;
        font-size: 10px;
    }

    .chart input {
        flex: 1 1 auto;
        min-width: 0;
        padding: 3px 7px;
        border: 1px solid var(--border);
        border-radius: var(--radius-sm);
        background: var(--bg-panel);
        color: var(--text);
        font-family: var(--mono);
        font-size: 11.5px;
    }

    .chart input:focus-visible {
        outline: none;
        border-color: var(--ctx-color);
    }

    .refused {
        display: flex;
        align-items: flex-start;
        gap: 8px;
        margin: 0;
        padding: 8px 12px;
        flex: 0 0 auto;
        background: color-mix(in srgb, var(--error) 14%, transparent);
        color: var(--error);
        font-size: 12px;
        line-height: 1.5;
        max-height: 5.5em;
        overflow-y: auto;
    }

    .loading {
        margin: 0;
        padding: 18px 16px;
        color: var(--text-dim);
    }

    /* CodeMirror styles itself through the theme in ../editor/setup.ts, in
       the app's own tokens. All this has to do is give it a box to fill. */
    .pane {
        flex: 1 1 auto;
        min-height: 0;
        min-width: 0;
        overflow: hidden;
    }
</style>
