import { beforeEach, describe, expect, test, vi } from 'vitest';

const Releases = vi.fn();
const Subscribe = vi.fn();
const Unsubscribe = vi.fn();

vi.mock('@wailsio/runtime', () => ({ Events: { On: vi.fn() } }));
vi.mock('../../../bindings/github.com/rogerwesterbo/k8sdockside', () => ({
    LogService: {
        Containers: vi.fn().mockResolvedValue([]),
        Open: vi.fn().mockResolvedValue('logs-1'),
        Close: vi.fn(),
    },
    HelmService: { Releases },
    ResourceService: { Subscribe, Unsubscribe, SetNamespace: vi.fn() },
}));

const { subscribe } = await import('./subscriptions');

const EMPTY = { kind: 'helmreleases', columns: ['Name'], rows: [], namespaced: true, error: '' };

beforeEach(() => {
    Releases.mockReset().mockResolvedValue(EMPTY);
    Subscribe.mockReset().mockResolvedValue('sub-1');
    Unsubscribe.mockReset();
});

describe('kinds that are read rather than watched', () => {
    test('Helm releases are fetched, never subscribed to', async () => {
        subscribe('ctx', 'helmreleases', '', vi.fn(), vi.fn());
        await vi.waitFor(() => expect(Releases).toHaveBeenCalledWith('ctx', ''));

        expect(Subscribe).not.toHaveBeenCalled();
    });

    test('the rows come back through the same callback as any other kind', async () => {
        const onTable = vi.fn();
        subscribe('ctx', 'helmreleases', '', onTable, vi.fn());

        await vi.waitFor(() => expect(onTable).toHaveBeenCalledOnce());
    });

    test('a failure is reported rather than thrown', async () => {
        Releases.mockRejectedValueOnce(new Error('forbidden'));
        const onError = vi.fn();

        subscribe('ctx', 'helmreleases', '', vi.fn(), onError);

        await vi.waitFor(() => expect(onError).toHaveBeenCalledWith('forbidden'));
    });

    test('changing the namespace re-reads', async () => {
        const sub = subscribe('ctx', 'helmreleases', '', vi.fn(), vi.fn());
        await vi.waitFor(() => expect(Releases).toHaveBeenCalledTimes(1));

        sub.setNamespace('prod');

        await vi.waitFor(() => expect(Releases).toHaveBeenCalledWith('ctx', 'prod'));
    });

    // A tab closed while the read is in flight must not paint into a view the
    // user has already left.
    test('closing stops a read in flight from calling back', async () => {
        let settle!: (t: unknown) => void;
        Releases.mockReturnValueOnce(new Promise((r) => { settle = r; }));
        const onTable = vi.fn();

        const sub = subscribe('ctx', 'helmreleases', '', onTable, vi.fn());
        sub.close();
        settle(EMPTY);
        await new Promise((r) => setTimeout(r, 10));

        expect(onTable).not.toHaveBeenCalled();
    });

    test('every other kind still opens a watch', async () => {
        subscribe('ctx', 'pods', '', vi.fn(), vi.fn());

        await vi.waitFor(() => expect(Subscribe).toHaveBeenCalledWith('ctx', 'pods', ''));
        expect(Releases).not.toHaveBeenCalled();
    });
});
