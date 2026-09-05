<!--
  The solution plugins installed on this machine, and everything about
  installing more.

  Deliberately the same shape as the Themes section: same folder, same starter
  file, same reload, same list of what would not load. The two extension points
  are the same promise made twice — drop a JSON file in a folder and the app
  knows about your thing — and someone who has installed a theme should not have
  to learn a second set of motions to install a plugin.
-->
<script lang="ts">
    import { workspace } from '../../state/workspace.svelte';
    import Icon from '../Icon.svelte';
    import SettingsSection from './SettingsSection.svelte';

    let showFormat = $state(false);

    let builtin = $derived(workspace.plugins.filter((p) => p.origin === 'builtin'));
    let installed = $derived(workspace.plugins.filter((p) => p.origin !== 'builtin'));

    function fileOf(origin: string): string {
        const at = Math.max(origin.lastIndexOf('/'), origin.lastIndexOf('\\'));
        return at >= 0 ? origin.slice(at + 1) : origin;
    }

    function required(plugin: { requires: { optional: boolean }[] }): number {
        return plugin.requires.filter((r) => !r.optional).length;
    }
</script>

<SettingsSection
    title="Plugins"
    lede="A solution plugin gives something installed in your clusters — Argo CD, Flux, Prometheus — a place of its own in the sidebar, instead of leaving its custom resources scattered through the definitions tree under group names. Like a theme, it is a JSON file that names things the app already knows how to show: it cannot ship code or queries."
>
    <h3>Built in</h3>
    <div class="gallery">
        {#each builtin as plugin (plugin.id)}
            {@render card(plugin)}
        {/each}
    </div>

    {#if installed.length > 0}
        <h3>Installed</h3>
        <div class="gallery">
            {#each installed as plugin (plugin.id)}
                {@render card(plugin)}
            {/each}
        </div>
    {/if}

    <h3>Your own plugins</h3>
    <p class="note">
        Drop a <code>.json</code> file into the plugins folder, or a whole folder of them. Files are read from the
        folder itself and one level into any subfolder, exactly as themes are.
    </p>

    <div class="path-row">
        <Icon name="folder" size={13} />
        <span class="path selectable">{workspace.pluginDir || '…'}</span>
        <button onclick={() => workspace.revealPluginDir()}>Open folder</button>
    </div>

    <div class="actions">
        <button class="primary" onclick={() => workspace.createExamplePlugin()}>
            <Icon name="plus" size={14} /> Write a starter plugin
        </button>
        <button onclick={() => workspace.addPluginFolder()}>
            <Icon name="folder-plus" size={14} /> Watch another folder
        </button>
        <button onclick={() => workspace.reloadPlugins()}>
            <Icon name="refresh" size={14} /> Reload
        </button>
    </div>

    {#if workspace.pluginFolders.length > 0}
        <h3>Extra folders</h3>
        <ul class="paths">
            {#each workspace.pluginFolders as folder (folder)}
                <li>
                    <Icon name="folder" size={13} />
                    <span class="path selectable">{folder}</span>
                    <button
                        class="drop"
                        onclick={() => workspace.removePluginFolder(folder)}
                        title="Stop reading plugins from {folder}"
                        aria-label="Stop reading plugins from {folder}"
                    >
                        <Icon name="close" size={12} />
                    </button>
                </li>
            {/each}
        </ul>
    {/if}

    {#if workspace.pluginProblems.length > 0}
        <h3>Would not load</h3>
        <ul class="problems">
            {#each workspace.pluginProblems as problem (problem.path + problem.message)}
                <li>
                    <Icon name="alert" size={13} />
                    <div>
                        <span class="path selectable">{problem.path}</span>
                        <p>{problem.message}</p>
                    </div>
                </li>
            {/each}
        </ul>
    {/if}

    <button class="disclose" onclick={() => (showFormat = !showFormat)} aria-expanded={showFormat}>
        <Icon name={showFormat ? 'dot' : 'plus'} size={12} />
        {showFormat ? 'Hide' : 'Show'} what a plugin file looks like
    </button>

    {#if showFormat}
        <p class="note">
            A view names a kind the app can already open: a built-in name like <code>deployments</code>, or
            <code>crd:&lt;plural&gt;.&lt;group&gt;</code> for a custom resource. <code>requires</code> is what the
            overview checks the cluster for, and <code>cards</code> are the live counts on it — grouped by a field
            path such as <code>status.health.status</code> or <code>status.conditions[Ready]</code>.
        </p>
        <pre class="example selectable">{`{
    "id": "acme",
    "name": "Acme Mesh",
    "tagline": "service mesh",
    "icon": "share",
    "docs": "https://example.com",
    "requires": [
        { "kind": "crd:meshes.acme.io", "label": "Meshes" }
    ],
    "views": [
        { "id": "meshes", "label": "Meshes", "icon": "share",
          "kind": "crd:meshes.acme.io" },
        { "id": "control-plane", "label": "Control plane", "icon": "server",
          "kind": "deployments", "namespace": "acme-system",
          "selector": "app.kubernetes.io/part-of=acme" }
    ],
    "cards": [
        { "label": "Meshes", "kind": "crd:meshes.acme.io",
          "groupBy": "status.conditions[Ready]",
          "tones": { "True": "ok", "False": "error" } }
    ]
}`}</pre>
    {/if}
</SettingsSection>

{#snippet card(plugin: import('../../plugins/types').Plugin)}
    <article class="plugin">
        <header>
            <Icon name={plugin.icon} size={16} />
            <div class="naming">
                <p class="name">{plugin.name}</p>
                <p class="tagline">{plugin.tagline || plugin.id}</p>
            </div>
        </header>
        <p class="counts">
            {plugin.views.length} view{plugin.views.length === 1 ? '' : 's'}
            · {required(plugin)} required kind{required(plugin) === 1 ? '' : 's'}
        </p>
        {#if plugin.origin !== 'builtin'}
            <p class="from" title={plugin.origin}>
                {#if plugin.pack}{plugin.pack} · {/if}{fileOf(plugin.origin)}
            </p>
        {/if}
    </article>
{/snippet}

<style>
    h3 {
        margin: 22px 0 8px;
        font-size: 11px;
        letter-spacing: 0.06em;
        text-transform: uppercase;
        color: var(--text-faint);
    }

    .note {
        margin: 0 0 12px;
        max-width: 70ch;
        font-size: 12px;
        line-height: 1.7;
        color: var(--text-faint);
    }

    .gallery {
        display: grid;
        grid-template-columns: repeat(auto-fill, minmax(210px, 1fr));
        gap: 10px;
    }

    .plugin {
        padding: 10px 12px;
        border-radius: var(--radius);
        background: var(--bg-panel);
        box-shadow: inset 0 0 0 1px var(--border-soft);
        min-width: 0;
    }

    .plugin header {
        display: flex;
        align-items: center;
        gap: 9px;
        min-width: 0;
    }

    .plugin header :global(svg) {
        flex: 0 0 auto;
        color: var(--text-dim);
    }

    .naming {
        min-width: 0;
    }

    .name {
        margin: 0;
        font-size: 12.5px;
        font-weight: 500;
        color: var(--text);
        overflow: hidden;
        text-overflow: ellipsis;
        white-space: nowrap;
    }

    .tagline,
    .counts,
    .from {
        margin: 2px 0 0;
        font-size: 11px;
        color: var(--text-faint);
        overflow: hidden;
        text-overflow: ellipsis;
        white-space: nowrap;
    }

    .counts {
        margin-top: 7px;
    }

    .from {
        font-family: var(--mono);
        font-size: 10px;
    }

    .path-row {
        display: flex;
        align-items: center;
        gap: 8px;
        padding: 7px 10px;
        margin-bottom: 12px;
        border-radius: var(--radius-sm);
        background: var(--bg-raised);
        box-shadow: inset 0 0 0 1px var(--border-soft);
    }

    .path {
        flex: 1 1 auto;
        min-width: 0;
        font-family: var(--mono);
        font-size: 11px;
        color: var(--text-dim);
        overflow: hidden;
        text-overflow: ellipsis;
        white-space: nowrap;
    }

    .path-row button {
        flex: 0 0 auto;
        padding: 3px 9px;
        border-radius: var(--radius-sm);
        font-size: 11px;
        color: var(--text-dim);
        box-shadow: inset 0 0 0 1px var(--border);
    }

    .path-row button:hover {
        background: var(--bg-hover);
        color: var(--text);
    }

    .actions {
        display: flex;
        flex-wrap: wrap;
        gap: 8px;
        margin-bottom: 12px;
    }

    .actions button {
        display: flex;
        align-items: center;
        gap: 6px;
        padding: 6px 11px;
        border-radius: var(--radius-sm);
        font-size: 12px;
        color: var(--text-dim);
        box-shadow: inset 0 0 0 1px var(--border);
    }

    .actions button:hover {
        background: var(--bg-hover);
        color: var(--text);
    }

    .actions .primary {
        background: var(--accent);
        color: var(--accent-text);
        box-shadow: none;
    }

    .actions .primary:hover {
        filter: brightness(1.08);
        background: var(--accent);
        color: var(--accent-text);
    }

    .paths,
    .problems {
        list-style: none;
        margin: 0 0 12px;
        padding: 0;
    }

    .paths li,
    .problems li {
        display: flex;
        align-items: center;
        gap: 8px;
        padding: 6px 2px;
        border-bottom: 1px solid var(--border-soft);
    }

    .problems li {
        align-items: flex-start;
    }

    .problems :global(svg) {
        flex: 0 0 auto;
        margin-top: 2px;
        color: var(--error);
    }

    .problems div {
        min-width: 0;
    }

    .problems p {
        margin: 2px 0 0;
        font-size: 11.5px;
        color: var(--text-dim);
    }

    .drop {
        display: grid;
        place-items: center;
        width: 20px;
        height: 20px;
        flex: 0 0 auto;
        border-radius: var(--radius-sm);
        color: var(--text-faint);
    }

    .drop:hover {
        background: var(--bg-hover);
        color: var(--error);
    }

    .disclose {
        display: flex;
        align-items: center;
        gap: 6px;
        margin-top: 6px;
        font-size: 12px;
        color: var(--text-dim);
    }

    .disclose:hover {
        color: var(--text);
    }

    .example {
        margin: 10px 0 0;
        padding: 12px 14px;
        border-radius: var(--radius);
        background: var(--bg-raised);
        box-shadow: inset 0 0 0 1px var(--border-soft);
        font-family: var(--mono);
        font-size: 11px;
        line-height: 1.7;
        color: var(--text-dim);
        overflow-x: auto;
    }

    code {
        font-family: var(--mono);
        font-size: 11px;
        background: var(--bg-raised);
        border-radius: 3px;
        padding: 1px 5px;
    }
</style>
