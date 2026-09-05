import { beforeEach, expect, test } from 'vitest';
import { render } from 'vitest-browser-svelte';
import ContextTree from './ContextTree.svelte';
import { workspace } from '../state/workspace.svelte';
import type { Plugin } from '../plugins/types';

// A plugin switched off in Settings has to leave the sidebar. Leaving the row
// there greyed would be the worst of both: it still costs a presence check
// against the cluster, and it still says the app cares about something the user
// has told it not to.

const CTX = { id: 'x', name: 'admin@prod', cluster: 'prod', user: 'admin',
    namespace: '', server: '', file: '/c', current: false };

function plugin(id: string, name: string, disabled = false): Plugin {
    return {
        id, name, tagline: '', icon: 'puzzle', author: '', docs: '', description: '',
        requires: [], pack: '', origin: 'builtin', disabled,
        views: [{ id: 'things', label: 'Things', icon: 'box', type: 'table', kind: 'pods', namespace: '', selector: '' }],
    };
}

const settle = () => new Promise((r) => setTimeout(r, 120));
const rowNames = () => [...document.querySelectorAll('button.plugin')].map((el) => el.textContent?.trim() ?? '');

beforeEach(() => {
    document.body.innerHTML = '';
    workspace.files = [{ path: '/c', source: 'manual', error: '', contexts: [CTX] }];
    workspace.expanded = ['x'];
    workspace.settings.layout.collapsedGroups = [];
    workspace.settings.contexts = {};
    workspace.customKinds = {};
    workspace.expandedPlugins = [];
});

test('a switched-off plugin has no row in the sidebar', async () => {
    workspace.pluginCatalogue = {
        plugins: [plugin('argocd', 'Argo CD', true), plugin('flux', 'Flux')],
        dir: '', folders: [], problems: [],
    };

    render(ContextTree, { props: { context: CTX } });
    await settle();

    const names = rowNames();
    expect(names.some((n) => n.includes('Flux'))).toBe(true);
    expect(names.some((n) => n.includes('Argo CD'))).toBe(false);
});

// With every plugin switched off the section is empty, and has to say so the
// same way it does when nothing is installed at all.
test('switching off the last plugin leaves the section saying so', async () => {
    workspace.pluginCatalogue = {
        plugins: [plugin('argocd', 'Argo CD', true)],
        dir: '', folders: [], problems: [],
    };

    render(ContextTree, { props: { context: CTX } });
    await settle();

    expect(rowNames()).toEqual([]);
    expect(document.body.textContent).toContain('No plugins installed');
});
