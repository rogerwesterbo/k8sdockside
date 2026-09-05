import { beforeEach, expect, test, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { workspace } from '../../state/workspace.svelte';
import type { Plugin } from '../../plugins/types';

// Settings is the one place a switched-off plugin still appears: it is where it
// gets switched back on. Everywhere else it is gone, so if the card were hidden
// here too there would be no way back.

function plugin(id: string, name: string, disabled = false): Plugin {
    return {
        id, name, tagline: 'a solution', icon: 'puzzle', author: '', docs: '', description: '',
        requires: [{ kind: 'crd:things.acme.io', label: 'Things', optional: false }],
        pack: '', origin: 'builtin', disabled,
        views: [{ id: 'things', label: 'Things', icon: 'box', type: 'table', kind: 'pods', namespace: '', selector: '' }],
    };
}

const settle = () => new Promise((r) => setTimeout(r, 60));
const switchFor = (name: string) =>
    [...document.querySelectorAll('article.plugin')]
        .find((el) => el.textContent?.includes(name))
        ?.querySelector('input[type="checkbox"]') as HTMLInputElement | undefined;

beforeEach(() => {
    document.body.innerHTML = '';
    workspace.pluginCatalogue = {
        plugins: [plugin('argocd', 'Argo CD'), plugin('flux', 'Flux', true)],
        dir: '/p', folders: [], problems: [],
    };
});

test('every plugin card carries a switch showing whether it is on', async () => {
    const PluginsSection = (await import('./PluginsSection.svelte')).default;
    render(PluginsSection);
    await settle();

    expect(switchFor('Argo CD')?.checked).toBe(true);
    // Still listed, and still reachable, precisely because it is switched off.
    expect(switchFor('Flux')).toBeDefined();
    expect(switchFor('Flux')?.checked).toBe(false);
});

test('flipping the switch sends the wanted state, not a toggle', async () => {
    const sent = vi.spyOn(workspace, 'setPluginEnabled').mockResolvedValue(undefined);
    const PluginsSection = (await import('./PluginsSection.svelte')).default;
    render(PluginsSection);
    await settle();

    switchFor('Argo CD')!.click();
    await settle();

    expect(sent).toHaveBeenCalledWith('argocd', false);
    sent.mockRestore();
});
