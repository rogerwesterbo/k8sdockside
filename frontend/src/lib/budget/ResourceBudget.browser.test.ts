import { expect, test, vi, beforeEach } from 'vitest';
import { render } from 'vitest-browser-svelte';

// The service is mocked because the component's job is to render a budget, and
// what is worth checking is what it says when a number is missing -- which is
// the ordinary case on a cluster with no metrics stack.
const Budget = vi.hoisted(() => vi.fn());
vi.mock('../../../bindings/github.com/roger/k8sdockside', () => ({
    ResourceService: { Budget },
}));

import ResourceBudget from './ResourceBudget.svelte';

const settle = () => new Promise((r) => setTimeout(r, 80));

function reply(over: Record<string, unknown> = {}) {
    return {
        scope: { kind: 'cluster', name: '' },
        amounts: [
            {
                label: 'CPU', unit: 'cores',
                capacity: 16, allocatable: 15, requested: 6, limits: 30, used: 3,
                hasCapacity: true, hasDemand: true, hasUsed: true,
            },
        ],
        usage: { source: 'metrics-server', error: '' },
        error: '',
        ...over,
    };
}

beforeEach(() => {
    document.body.innerHTML = '';
    Budget.mockReset();
});

test('shows requested, limits and used together', async () => {
    Budget.mockResolvedValue(reply());

    render(ResourceBudget, { props: { contextId: 'x', scope: 'cluster' } });
    await settle();

    const text = document.body.textContent ?? '';
    expect(text).toContain('Requested');
    expect(text).toContain('Limits');
    expect(text).toContain('Used');
});

test('says why the used column is empty rather than showing zero', async () => {
    // No metrics-server and no Prometheus. Everything else still renders.
    Budget.mockResolvedValue(
        reply({
            amounts: [
                {
                    label: 'CPU', unit: 'cores',
                    capacity: 16, allocatable: 15, requested: 6, limits: 30, used: 0,
                    hasCapacity: true, hasDemand: true, hasUsed: false,
                },
            ],
            usage: { source: '', error: 'metrics-server: not found; prometheus: none found in this cluster' },
        }),
    );

    render(ResourceBudget, { props: { contextId: 'x', scope: 'cluster' } });
    await settle();

    const text = document.body.textContent ?? '';
    expect(text).toContain('Requested');
    // The warning names what is missing, and does not claim usage is zero.
    expect(text).toContain('metrics-server');
    expect(text).not.toContain('Used');
});

test('names the source when one answered', async () => {
    Budget.mockResolvedValue(reply());

    render(ResourceBudget, { props: { contextId: 'x', scope: 'cluster' } });
    await settle();

    expect(document.body.textContent ?? '').toContain('metrics-server');
});

test('reports a budget that could not be read at all', async () => {
    Budget.mockRejectedValue(new Error('the server rejected the request'));

    render(ResourceBudget, { props: { contextId: 'x', scope: 'cluster' } });
    await settle();

    expect(document.body.textContent ?? '').toContain('the server rejected the request');
});

test('draws bars for a namespace with no quota', async () => {
    // The reported bug: an unquota'd namespace has no ceiling, so every bar sat
    // at zero width and the section looked broken even though the numbers were
    // right there beside it.
    Budget.mockResolvedValue(
        reply({
            scope: { kind: 'namespace', name: 'test1' },
            amounts: [
                {
                    label: 'CPU', unit: 'cores',
                    capacity: 0, allocatable: 0, requested: 1.22, limits: 0.06, used: 1.08,
                    hasCapacity: false, hasDemand: true, hasUsed: true,
                },
                {
                    label: 'Pods', unit: '',
                    capacity: 0, allocatable: 0, requested: 0, limits: 0, used: 4,
                    hasCapacity: false, hasDemand: false, hasUsed: true,
                },
            ],
        }),
    );

    render(ResourceBudget, { props: { contextId: 'x', scope: 'namespace', name: 'test1' } });
    await settle();

    const widths = [...document.querySelectorAll('.fill')].map((el) =>
        parseFloat((el as HTMLElement).style.width),
    );
    expect(widths.length).toBeGreaterThan(0);
    // The largest sets the scale, and nothing is left at zero width.
    expect(Math.max(...widths)).toBe(100);
    expect(widths.every((w) => w > 0)).toBe(true);

    // Pods is alone with no ceiling, so it gets no track rather than a full one.
    expect(document.querySelectorAll('.track').length).toBe(3);
    expect(document.body.textContent).toContain('no quota');
});
