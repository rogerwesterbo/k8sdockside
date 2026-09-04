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

        <!-- One button rather than two: with nothing open it expands, and with
             anything open it collapses, which covers both jobs without a fifth
             control crowding the header at narrow sidebar widths. -->
        {#if total > 0}
            {@const collapsing = workspace.anyExpanded}
            <button
                class="action"
                onclick={() => (collapsing ? workspace.collapseAll() : workspace.expandAll())}
                title={collapsing ? 'Collapse all contexts' : 'Expand all contexts'}
                aria-label={collapsing ? 'Collapse all contexts' : 'Expand all contexts'}
            >
                <Icon name={collapsing ? 'collapse-all' : 'expand-all'} size={15} />
            </button>
            <!-- The tree control and the kubeconfig-source controls do
                 different jobs; the rule keeps them from reading as one row. -->
            <span class="divider"></span>
        {/if}

        <button
            class="action"
            class:spinning={workspace.syncing}
            onclick={() => workspace.sync()}
            disabled={workspace.syncing}
            title="Rescan ~/.kube, $KUBECONFIG, your added files and watched folders, and recheck the clusters already looked at"
            aria-label="Sync kubeconfig files"
        >
            <Icon name="refresh" size={15} />
        </button>
        <button
            class="action"
            onclick={() => workspace.addFile()}
            title="Add kubeconfig files"
            aria-label="Add kubeconfig files"
        >
            <Icon name="plus" size={15} />
        </button>
        <button
            class="action"
            onclick={() => workspace.addFolder()}
            title="Watch a folder of kubeconfigs"
            aria-label="Watch a folder of kubeconfigs"
        >
            <Icon name="folder-plus" size={15} />
        </button>

        <!-- Separated from the kubeconfig controls beside it: those act on the
             list below, this opens a view of its own. -->
        <span class="divider"></span>
        <button
            class="action"
            onclick={() => workspace.openSettings()}
            title="Application settings (⌘,)"
            aria-label="Application settings"
        >
            <Icon name="settings" size={15} />
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
            <!-- Grouped by file, or one flat list of contexts. A file that
                 could not be parsed is always headed, whatever the setting:
                 it has no contexts to show in its place, so hiding its name
                 would remove it from the sidebar without saying so. -->
            {@const headed = workspace.showKubeconfigNames || file.error !== ''}
            <div class="file" class:flat={!headed}>
                {#if headed}
                <div class="file-head" title={file.path}>
                    <Icon name="file" size={12} />
                    <span class="file-name">{basename(file.path)}</span>
                    <button
                        class="remove"
                        onclick={() => workspace.removeFile(file.path)}
                        title={file.source === 'manual'
                            ? 'Stop tracking this file'
                            : 'Hide this file. Discovery would find it again, so it is remembered as hidden.'}
                        aria-label="Remove {basename(file.path)}"
                    >
                            <Icon name="close" size={12} />
                    </button>
                </div>
                {/if}

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
                        k8sdockside looks in <code>~/.kube</code> and <code>$KUBECONFIG</code>. Add files, or point it
                        at a folder and it will take every kubeconfig in there — whatever they are named.
                    </p>
                    <div class="ctas">
                        <button class="cta" onclick={() => workspace.addFile()}>
                            <Icon name="plus" size={14} /> Add kubeconfigs
                        </button>
                        <button class="cta" onclick={() => workspace.addFolder()}>
                            <Icon name="folder-plus" size={14} /> Add a folder
                        </button>
                    </div>
                {/if}
            </div>
        {/if}
    </div>

    {#if workspace.folders.length > 0 || workspace.excluded.length > 0}
        <div class="sources">
            {#if workspace.folders.length > 0}
                <p class="group">Watched folders</p>
                {#each workspace.folders as folder (folder)}
                    <div class="source-row">
                        <Icon name="folder" size={12} />
                        <span class="path" title={folder}>{basename(folder)}</span>
                        <button
                            class="drop"
                            onclick={() => workspace.removeFolder(folder)}
                            title="Stop watching {folder}"
                            aria-label="Stop watching {folder}"
                        >
                            <Icon name="close" size={11} />
                        </button>
                    </div>
                {/each}
            {/if}

            <!-- A discovered file cannot be forgotten, only hidden -- so the
                 hiding has to be visible, or it is state nobody can undo. -->
            {#if workspace.excluded.length > 0}
                <p class="group">Hidden</p>
                {#each workspace.excluded as path (path)}
                    <div class="source-row">
                        <Icon name="file" size={12} />
                        <span class="path" title={path}>{basename(path)}</span>
                        <button
                            class="drop restore"
                            onclick={() => workspace.restoreFile(path)}
                            title="Show {path} again"
                            aria-label="Show {basename(path)} again"
                        >
                            <Icon name="undo" size={11} />
                        </button>
                    </div>
                {/each}
            {/if}
        </div>
    {/if}

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
        /* With minimums on the rows inside, the column can be asked for more
           than it has; clipping is better than spilling over the status bar. */
        overflow: hidden;
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

    .divider {
        width: 1px;
        height: 15px;
        flex: 0 0 auto;
        background: var(--border);
        margin: 0 1px;
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
        /* A floor for the tree, so the panels below can never squeeze it down
           to nothing however short the window or high the zoom. */
        min-height: 140px;
        overflow-y: auto;
        padding: 6px 8px 12px;
    }

    .file + .file {
        margin-top: 10px;
    }

    /* Without its heading a file is not a group any more, so the gap that
       separated it from the one above would read as a stray blank line. */
    .file.flat + .file.flat {
        margin-top: 0;
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
        flex: 1 1 auto;
        min-width: 0;
        overflow: hidden;
        text-overflow: ellipsis;
        white-space: nowrap;
        letter-spacing: 0.03em;
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

    .ctas {
        display: flex;
        flex-wrap: wrap;
        gap: 8px;
        justify-content: center;
    }

    /* The watched folders sit above the context settings, out of the scrolling
       tree: they are about where configs come from, not about one cluster. */
    .sources {
        flex: 0 0 auto;
        max-height: 180px;
        overflow-y: auto;
        padding: 8px 10px;
        border-top: 1px solid var(--border);
    }

    .sources .group {
        margin: 0 0 4px;
        font-size: 10px;
        letter-spacing: 0.05em;
        text-transform: uppercase;
        color: var(--text-faint);
    }

    .source-row {
        display: flex;
        align-items: center;
        gap: 6px;
        padding: 3px 2px;
        font-size: 11px;
        color: var(--text-dim);
    }

    .source-row .path {
        flex: 1 1 auto;
        min-width: 0;
        overflow: hidden;
        text-overflow: ellipsis;
        white-space: nowrap;
    }

    .drop {
        flex: 0 0 auto;
        color: var(--text-faint);
        opacity: 0.7;
    }

    .drop:hover {
        color: var(--error);
        opacity: 1;
    }

    .drop.restore:hover {
        color: var(--accent);
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
