import { beforeEach, expect, test } from 'vitest';
import { render } from 'vitest-browser-svelte';
import ContextSettings from './ContextSettings.svelte';
import { workspace } from '../state/workspace.svelte';

// The Metrics row points a cluster at a Prometheus, and the only thing that
// ever reads that endpoint is a plugin's charts. With every charting plugin
// switched off it configures nothing, so it should not be on screen -- and
// reading it costs a Services listing hunting for a Prometheus nobody asked
// about, which is worse than the words.

const CTX = { id: 'x', name: 'admin@prod', cluster: 'prod', user: 'admin',
    namespace: '', server: '', file: '/c', current: false };

const settle = () => new Promise((r) => setTimeout(r, 60));
const metricsRow = () =>
    [...document.querySelectorAll('.field')].find((el) => el.querySelector('span')?.textContent?.trim() === 'Metrics');

beforeEach(() => {
    document.body.innerHTML = '';
    workspace.settings.contexts = {};
    workspace.metricsSources = {};
});

test('the Metrics row is there while something still charts', async () => {
    workspace.metricsAttachments = ['dashboard'];

    render(ContextSettings, { props: { context: CTX } });
    await settle();

    expect(metricsRow()).toBeDefined();
});

test('switching off every charting plugin takes the Metrics row with it', async () => {
    workspace.metricsAttachments = [];

    render(ContextSettings, { props: { context: CTX } });
    await settle();

    expect(metricsRow()).toBeUndefined();
    expect(document.body.textContent).not.toContain('Prometheus');
});
