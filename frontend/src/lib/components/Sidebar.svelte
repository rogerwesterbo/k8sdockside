<!--
  The left navigation: every kubeconfig found on disk, grouped by the file it
  came from, with the settings panel for the selected context pinned to the
  bottom.
-->
<script lang="ts">
    import { workspace } from '../state/workspace.svelte';
    import ContextSettings from './ContextSettings.svelte';
    import ContextTree from './ContextTree.svelte';
    import Icon from './Icon.svelte';

    let filter = $state('');

    let files = $derived(
        workspace.files
            .map((file) => ({
                ...file,
                contexts: file.contexts.filter((c) => matches(c.name, workspace.displayName(c))),
            }))
            // A file is worth showing if it still has a matching context, or if
            // it is broken -- an unreadable kubeconfig is something the user
            // needs to see rather than something to filter away.
            .filter((file) => file.contexts.length > 0 || file.error !== ''),
    );

    let total = $derived(workspace.contexts.length);

    function matches(...values: string[]): boolean {
        const needle = filter.trim().toLowerCase();
        if (!needle) return true;
        return values.some((v) => v.toLowerCase().includes(needle));
    }

    function basename(path: string): string {
        return path.split('/').pop() || path;
    }
</script>

<aside class="sidebar" style:width="{workspace.sidebarWidth}px">
    <header class="top">
        <span class="heading">Clusters</span>
        <span class="count">{total}</span>
        <button
            class="action"
            class:spinning={workspace.syncing}
            onclick={() => workspace.sync()}
            disabled={workspace.syncing}
            title="Rescan ~/.kube, $KUBECONFIG and your added files"
            aria-label="Sync kubeconfig files"
        >
            <Icon name="refresh" size={15} />
        </button>
        <button class="action" onclick={() => workspace.addFile()} title="Add a kubeconfig file" aria-label="Add a kubeconfig file">
            <Icon name="plus" size={15} />
        </button>
    </header>

    {#if total > 6}
        <div class="search">
            <Icon name="search" size={13} />
            <input type="search" bind:value={filter} placeholder="Filter contexts" spellcheck="false" />
        </div>
    {/if}

    <div class="scroll">
        {#each files as file (file.path)}
            <div class="file">
                <div class="file-head" title={file.path}>
                    <Icon name="file" size={12} />
                    <span class="file-name">{basename(file.path)}</span>
                    <span class="source">{file.source}</span>
                    {#if file.source === 'manual'}
                        <button
                            class="remove"
                            onclick={() => workspace.removeFile(file.path)}
                            title="Stop tracking this file"
                            aria-label="Stop tracking {basename(file.path)}"
                        >
                            <Icon name="close" size={12} />
                        </button>
                    {/if}
                </div>

                {#if file.error}
                    <p class="file-error"><Icon name="alert" size={12} />{file.error}</p>
                {/if}

                {#each file.contexts as context (context.id)}
                    <ContextTree {context} />
                {/each}
            </div>
        {/each}

        {#if workspace.loaded && files.length === 0}
            <div class="empty">
                {#if filter.trim()}
                    <p>No context matches “{filter}”.</p>
                {:else}
                    <p>No kubeconfig found.</p>
                    <p class="hint">
                        k8sdockside looks in <code>~/.kube</code> and <code>$KUBECONFIG</code>. Add a file to get started.
                    </p>
                    <button class="cta" onclick={() => workspace.addFile()}>
                        <Icon name="plus" size={14} /> Add kubeconfig
                    </button>
                {/if}
            </div>
        {/if}
    </div>

    {#if workspace.selectedContext}
        <ContextSettings context={workspace.selectedContext} />
    {/if}
</aside>

<style>
    .sidebar {
        display: flex;
        flex-direction: column;
        min-width: 0;
        height: 100%;
        background: var(--bg-sidebar);
        border-right: 1px solid var(--border);
    }

    .top {
        display: flex;
        align-items: center;
        gap: 6px;
        height: 38px;
        padding: 0 8px 0 12px;
        flex: 0 0 auto;
        border-bottom: 1px solid var(--border-soft);
    }

    .heading {
        font-size: 11px;
        letter-spacing: 0.08em;
        text-transform: uppercase;
        color: var(--text-dim);
    }

    .count {
        font-size: 10px;
        color: var(--text-faint);
        background: var(--bg-raised);
        border-radius: 8px;
        padding: 1px 6px;
        margin-right: auto;
    }

    .action {
        display: grid;
        place-items: center;
        width: 24px;
        height: 24px;
        border-radius: var(--radius-sm);
        color: var(--text-dim);
    }

    .action:hover:not(:disabled) {
        background: var(--bg-hover);
        color: var(--text);
    }

    .action:disabled {
        cursor: default;
    }

    .action.spinning {
        animation: spin 900ms linear infinite;
        color: var(--accent);
    }

    @keyframes spin {
        to {
            transform: rotate(360deg);
        }
    }

    .search {
        display: flex;
        align-items: center;
        gap: 6px;
        padding: 8px 12px 2px;
        color: var(--text-faint);
    }

    .search input {
        flex: 1 1 auto;
        min-width: 0;
        background: var(--bg);
        padding: 4px 7px;
        font-size: 12px;
    }

    .scroll {
        flex: 1 1 auto;
        overflow-y: auto;
        padding: 6px 8px 12px;
    }

    .file + .file {
        margin-top: 10px;
    }

    .file-head {
        display: flex;
        align-items: center;
        gap: 5px;
        padding: 6px 4px 4px;
        color: var(--text-faint);
        font-size: 10px;
    }

    .file-name {
        overflow: hidden;
        text-overflow: ellipsis;
        white-space: nowrap;
        letter-spacing: 0.03em;
    }

    .source {
        margin-left: auto;
        flex: 0 0 auto;
        text-transform: uppercase;
        letter-spacing: 0.06em;
        font-size: 9px;
        opacity: 0.75;
    }

    .remove {
        display: grid;
        place-items: center;
        width: 16px;
        height: 16px;
        border-radius: 3px;
        color: var(--text-faint);
        flex: 0 0 auto;
    }

    .remove:hover {
        background: var(--bg-hover);
        color: var(--error);
    }

    .file-error {
        display: flex;
        align-items: center;
        gap: 6px;
        margin: 0 4px 4px;
        font-size: 11px;
        color: var(--error);
    }

    .empty {
        padding: 28px 14px;
        text-align: center;
        color: var(--text-dim);
    }

    .empty p {
        margin: 0 0 8px;
    }

    .hint {
        font-size: 11px;
        color: var(--text-faint);
        line-height: 1.6;
    }

    .hint code {
        font-family: var(--mono);
        font-size: 10.5px;
        background: var(--bg-raised);
        border-radius: 3px;
        padding: 1px 4px;
    }

    .cta {
        display: inline-flex;
        align-items: center;
        gap: 6px;
        margin-top: 6px;
        padding: 6px 12px;
        border-radius: var(--radius-sm);
        background: var(--bg-raised);
        color: var(--text);
    }

    .cta:hover {
        background: var(--bg-active);
    }
</style>
