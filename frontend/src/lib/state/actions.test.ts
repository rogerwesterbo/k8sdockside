import { beforeEach, describe, expect, test, vi } from 'vitest';

const ObjectState = vi.fn();
const Delete = vi.fn();
const Scale = vi.fn();
const Restart = vi.fn();
const Cordon = vi.fn();
const Drain = vi.fn();
const CancelDrain = vi.fn();

// The drain event handler is captured as it registers, so a test can deliver a
// report exactly as Wails would.
let deliver: (event: { data: unknown }) => void = () => {};
vi.mock('@wailsio/runtime', () => ({
    Events: {
        On: vi.fn((_name: string, handler: (event: { data: unknown }) => void) => {
            deliver = handler;
        }),
    },
}));
vi.mock('../../../bindings/github.com/roger/k8sdockside', () => ({
    LogService: {
        Containers: vi.fn().mockResolvedValue([]),
        Open: vi.fn().mockResolvedValue('logs-1'),
        Close: vi.fn(),
    },
    ActionService: { ObjectState, Delete, Scale, Restart, Cordon, Drain, CancelDrain },
}));

const { actions } = await import('./actions.svelte');
const { changes } = await import('./changes.svelte');

const NODE = { contextId: 'cfg::prod', kind: 'nodes', namespace: '', name: 'wrkr01' };
const DEPLOYMENT = { contextId: 'cfg::prod', kind: 'deployments', namespace: 'default', name: 'web' };

const IDLE = { scalable: false, replicas: 0, cordoned: false, containers: [] };

/** One drain report, as the backend sends it. */
function report(over: Record<string, unknown> = {}) {
    return {
        data: {
            drainId: 'drain-1',
            node: 'wrkr01',
            phase: 'evicting',
            evicted: 0,
            total: 4,
            refused: [],
            error: '',
            done: false,
            ...over,
        },
    };
}

beforeEach(() => {
    actions.forget(NODE);
    actions.forget(DEPLOYMENT);
    ObjectState.mockReset().mockResolvedValue(IDLE);
    Delete.mockReset().mockResolvedValue(undefined);
    Scale.mockReset().mockResolvedValue(undefined);
    Restart.mockReset().mockResolvedValue(undefined);
    Cordon.mockReset().mockResolvedValue(undefined);
    Drain.mockReset().mockResolvedValue('drain-1');
    CancelDrain.mockReset();
});

describe('what the bar knows about an object', () => {
    test('an object it has not read yet is neither scalable nor cordoned', () => {
        expect(actions.stateOf(NODE)).toEqual(IDLE);
    });

    test('loading reads the object and holds what it said', async () => {
        ObjectState.mockResolvedValue({ scalable: true, replicas: 3, cordoned: false, containers: [] });

        await actions.load(DEPLOYMENT);

        expect(ObjectState).toHaveBeenCalledWith('cfg::prod', 'deployments', 'default', 'web');
        expect(actions.stateOf(DEPLOYMENT).replicas).toBe(3);
    });

    // A cluster that will not answer must not stop the bar being drawn: Edit
    // and Delete need nothing from it.
    test('a read that fails leaves the defaults rather than throwing', async () => {
        ObjectState.mockRejectedValue(new Error('forbidden'));

        await expect(actions.load(NODE)).resolves.toBeUndefined();
        expect(actions.stateOf(NODE)).toEqual(IDLE);
    });
});

describe('the one-shot actions', () => {
    test('scale sends the replica count', async () => {
        await actions.scale(DEPLOYMENT, 5);

        expect(Scale).toHaveBeenCalledWith('cfg::prod', 'deployments', 'default', 'web', 5);
    });

    test('cordon closes a node, and uncordon reopens it', async () => {
        await actions.cordon(NODE, true);
        expect(Cordon).toHaveBeenCalledWith('cfg::prod', 'wrkr01', true);

        await actions.cordon(NODE, false);
        expect(Cordon).toHaveBeenCalledWith('cfg::prod', 'wrkr01', false);
    });

    // Whatever else is on screen showing this object is now a moment out of
    // date, which is exactly what the change signal is for.
    test('a successful action says the object changed', async () => {
        const before = changes.revision(DEPLOYMENT);

        await actions.restart(DEPLOYMENT);

        expect(changes.revision(DEPLOYMENT)).toBe(before + 1);
    });

    test('a refused action says nothing changed, and carries the reason', async () => {
        Restart.mockRejectedValue(new Error('deployments.apps is forbidden'));
        const before = changes.revision(DEPLOYMENT);

        await expect(actions.restart(DEPLOYMENT)).rejects.toThrow('forbidden');

        expect(changes.revision(DEPLOYMENT)).toBe(before);
    });

    // Delete is the exception: the object is gone, so there is nothing to
    // re-read and the panel closes instead.
    test('deleting does not ask anything to re-read a deleted object', async () => {
        const before = changes.revision(DEPLOYMENT);

        await actions.remove(DEPLOYMENT);

        expect(Delete).toHaveBeenCalledWith('cfg::prod', 'deployments', 'default', 'web');
        expect(changes.revision(DEPLOYMENT)).toBe(before);
    });
});

describe('draining a node', () => {
    test('nothing is draining until one is started', () => {
        expect(actions.drainOf(NODE)).toBeNull();
    });

    test('starting one shows it as under way straight away', async () => {
        await actions.drain(NODE);

        expect(Drain).toHaveBeenCalledWith('cfg::prod', 'wrkr01');
        expect(actions.drainOf(NODE)?.done).toBe(false);
    });

    test('progress reports land on the node they belong to', async () => {
        await actions.drain(NODE);

        deliver(report({ evicted: 3, total: 4 }));

        expect(actions.drainOf(NODE)).toMatchObject({ evicted: 3, total: 4, phase: 'evicting' });
    });

    // Two nodes can drain at once. A report is routed by the ID the backend
    // gave, not by the node name, so nothing has to trust the payload's prose.
    test('a report for another drain is ignored', async () => {
        await actions.drain(NODE);

        deliver(report({ drainId: 'drain-9', evicted: 99 }));

        expect(actions.drainOf(NODE)?.evicted).toBe(0);
    });

    test('the pods it would not move are kept, with their reasons', async () => {
        await actions.drain(NODE);

        deliver(
            report({
                refused: [{ pod: { namespace: 'default', name: 'debug' }, reason: 'nothing manages it' }],
            }),
        );

        expect(actions.drainOf(NODE)?.refused).toHaveLength(1);
        expect(actions.drainOf(NODE)?.refused[0].reason).toContain('nothing manages it');
    });

    test('a finished drain is remembered as finished', async () => {
        await actions.drain(NODE);

        deliver(report({ phase: 'done', evicted: 4, done: true }));

        expect(actions.drainOf(NODE)?.done).toBe(true);
    });

    test('a failed drain keeps the reason it failed', async () => {
        await actions.drain(NODE);

        deliver(report({ phase: 'failed', error: 'nodes is forbidden', done: true }));

        expect(actions.drainOf(NODE)?.error).toBe('nodes is forbidden');
    });

    test('cancelling calls it off by the id the backend gave', async () => {
        await actions.drain(NODE);

        actions.cancelDrain(NODE);

        expect(CancelDrain).toHaveBeenCalledWith('drain-1');
    });

    test('cancelling a node that is not draining does nothing', () => {
        actions.cancelDrain(NODE);

        expect(CancelDrain).not.toHaveBeenCalled();
    });

    // A drain that would not start at all -- an unreachable cluster, no
    // permission -- has to say so rather than leave a bar spinning forever.
    test('a drain that will not start reports rather than hanging', async () => {
        Drain.mockRejectedValue(new Error('nodes is forbidden'));

        await expect(actions.drain(NODE)).rejects.toThrow('forbidden');
        expect(actions.drainOf(NODE)).toBeNull();
    });
});
