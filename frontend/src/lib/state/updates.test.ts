import { beforeEach, describe, expect, test, vi } from 'vitest';

const Status = vi.fn();
const Check = vi.fn();
const MarkRead = vi.fn();
const OpenRelease = vi.fn();

const events = vi.hoisted(() => ({ handler: (_e: { data: unknown }) => {} }));
vi.mock('@wailsio/runtime', () => ({
    Events: {
        On: vi.fn((name: string, handler: (event: { data: unknown }) => void) => {
            if (name === 'update:status') events.handler = handler;
        }),
    },
}));
vi.mock('../../../bindings/github.com/rogerwesterbo/k8sdockside', () => ({
    UpdateService: { Status, Check, MarkRead, OpenRelease },
}));

const { updates } = await import('./updates.svelte');

const RELEASE = {
    version: 'v0.0.3',
    name: 'v0.0.3',
    url: 'https://github.com/rogerwesterbo/k8sdockside/releases/tag/v0.0.3',
    publishedAt: '2026-09-05T16:02:10Z',
};

/** A status as the backend sends it. */
function status(over: Record<string, unknown> = {}) {
    return { current: 'v0.0.2', latest: null, newer: false, unread: false, checkedAt: '', error: '', ...over };
}

const NEWS = status({ latest: RELEASE, newer: true, unread: true, checkedAt: '2026-09-06T10:00:00Z' });

beforeEach(() => {
    Status.mockReset().mockResolvedValue(status());
    Check.mockReset().mockResolvedValue(status());
    MarkRead.mockReset().mockResolvedValue(status());
    OpenRelease.mockReset().mockResolvedValue(undefined);
    updates.status = status({ current: '' });
    updates.loaded = false;
});

describe('loading', () => {
    test('reads what the backend already knows', async () => {
        Status.mockResolvedValueOnce(NEWS);

        await updates.load();

        expect(updates.loaded).toBe(true);
        expect(updates.available).toBe(true);
        expect(updates.unread).toBe(true);
        expect(updates.latest?.version).toBe('v0.0.3');
    });

    // The bell mounts with the window, before the backend may be ready to
    // answer, and the window must not lose its title bar over that.
    test('tolerates a backend that cannot answer yet', async () => {
        Status.mockRejectedValueOnce(new Error('not ready'));

        await expect(updates.load()).resolves.toBeUndefined();

        expect(updates.loaded).toBe(true);
        expect(updates.available).toBe(false);
    });
});

describe('a push from the backend', () => {
    test('replaces the status, so the bell changes without being asked', () => {
        events.handler({ data: NEWS });

        expect(updates.unread).toBe(true);
        expect(updates.status.checkedAt).toBe('2026-09-06T10:00:00Z');
    });

    test('with nothing in it is ignored', () => {
        events.handler({ data: NEWS });
        events.handler({ data: null });

        expect(updates.unread).toBe(true);
    });
});

describe('checking now', () => {
    test('asks the backend and keeps its answer, saying so while it waits', async () => {
        let answer: (value: unknown) => void = () => {};
        Check.mockReturnValueOnce(new Promise((resolve) => (answer = resolve)));

        const checking = updates.check();
        expect(updates.checking).toBe(true);

        answer(NEWS);
        await checking;

        expect(updates.checking).toBe(false);
        expect(updates.available).toBe(true);
        expect(Check).toHaveBeenCalledOnce();
    });

    test('a call that fails reports the failure beside what was known', async () => {
        updates.status = NEWS;
        Check.mockRejectedValueOnce(new Error('the application is closing'));

        await updates.check();

        expect(updates.checking).toBe(false);
        expect(updates.status.error).toBe('the application is closing');
        expect(updates.latest?.version).toBe('v0.0.3');
    });
});

describe('marking as read', () => {
    test('takes the backend answer, which is where the read mark lives', async () => {
        updates.status = NEWS;
        MarkRead.mockResolvedValueOnce({ ...NEWS, unread: false });

        await updates.markRead();

        expect(updates.unread).toBe(false);
        // Still available: read is not the same as gone.
        expect(updates.available).toBe(true);
    });

    test('a failure is the caller to report', async () => {
        MarkRead.mockRejectedValueOnce(new Error('disk full'));

        await expect(updates.markRead()).rejects.toThrow('disk full');
    });
});

test('opening the release hands off to the backend, which decides the address', async () => {
    await updates.openRelease();

    expect(OpenRelease).toHaveBeenCalledOnce();
});
