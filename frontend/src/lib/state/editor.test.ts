import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest';

// The editors store talks to Go for all three of its jobs -- reading an object,
// checking what has been typed, and writing it back -- so the bindings are
// stubbed. What is under test is what happens to a document in between.
vi.mock('../../../bindings/github.com/roger/k8sdockside', () => ({
    ResourceService: {
        ResourceYAML: vi.fn().mockResolvedValue(''),
        ApplyYAML: vi.fn().mockResolvedValue(''),
        CheckYAML: vi.fn().mockResolvedValue({ valid: true, message: '', line: 0 }),
    },
}));

const { editors } = await import('./editor.svelte');
const { changes } = await import('./changes.svelte');
const { ResourceService } = await import('../../../bindings/github.com/roger/k8sdockside');

const TAB = 'edit:cfg::prod#pods#default#web';
const TARGET = { contextId: 'cfg::prod', kind: 'pods', namespace: 'default', name: 'web' };

const LIVE = 'apiVersion: v1\nkind: Pod\nmetadata:\n  name: web\n';

/** Runs the debounced check and lets its call settle. */
async function settle(): Promise<void> {
    await vi.advanceTimersByTimeAsync(300);
}

beforeEach(() => {
    editors.forget(TAB);
    vi.useFakeTimers();
    vi.mocked(ResourceService.ResourceYAML).mockReset().mockResolvedValue(LIVE);
    vi.mocked(ResourceService.ApplyYAML).mockReset().mockResolvedValue(LIVE);
    vi.mocked(ResourceService.CheckYAML)
        .mockReset()
        .mockResolvedValue({ valid: true, message: '', line: 0 });
});

afterEach(() => {
    vi.useRealTimers();
});

describe('opening a document', () => {
    test('reads the object and holds it as what the cluster has', async () => {
        await editors.load(TAB, TARGET);

        const doc = editors.doc(TAB);
        expect(doc.status).toBe('ready');
        expect(doc.text).toBe(LIVE);
        expect(doc.original).toBe(LIVE);
        expect(editors.isDirty(TAB)).toBe(false);
    });

    // The component that renders a document is destroyed every time you switch
    // dock tabs, and it loads on mount. Loading again must not be how an edit
    // is lost.
    test('opening it again leaves an edit alone; reloading throws it away', async () => {
        await editors.load(TAB, TARGET);
        editors.edit(TAB, 'edited: yes\n');

        await editors.load(TAB, TARGET);
        expect(editors.doc(TAB).text).toBe('edited: yes\n');
        expect(ResourceService.ResourceYAML).toHaveBeenCalledTimes(1);

        await editors.load(TAB, TARGET, { force: true });
        expect(editors.doc(TAB).text).toBe(LIVE);
    });

    test('a cluster that will not answer leaves the document in its error state', async () => {
        vi.mocked(ResourceService.ResourceYAML).mockRejectedValueOnce(new Error('connection refused'));

        await editors.load(TAB, TARGET);

        expect(editors.doc(TAB).status).toBe('error');
        expect(editors.doc(TAB).error).toBe('connection refused');
    });
});

describe('typing', () => {
    test('an edit is dirty at once and checked a moment later', async () => {
        await editors.load(TAB, TARGET);

        editors.edit(TAB, 'kind: Pod\n');

        expect(editors.isDirty(TAB)).toBe(true);
        // Not yet: the check is debounced, so a word costs one call.
        expect(ResourceService.CheckYAML).not.toHaveBeenCalled();

        await settle();
        expect(ResourceService.CheckYAML).toHaveBeenCalledWith('kind: Pod\n');
    });

    test('a run of keystrokes costs one check', async () => {
        await editors.load(TAB, TARGET);

        editors.edit(TAB, 'k');
        editors.edit(TAB, 'ki');
        editors.edit(TAB, 'kind');
        await settle();

        expect(ResourceService.CheckYAML).toHaveBeenCalledTimes(1);
        expect(ResourceService.CheckYAML).toHaveBeenCalledWith('kind');
    });

    test('broken YAML is remembered with the line it broke on', async () => {
        await editors.load(TAB, TARGET);
        vi.mocked(ResourceService.CheckYAML).mockResolvedValue({
            valid: false,
            message: 'mapping values are not allowed in this context',
            line: 3,
        });

        editors.edit(TAB, 'a: b\nc: d: e\n');
        await settle();

        expect(editors.doc(TAB).check).toEqual({
            valid: false,
            message: 'mapping values are not allowed in this context',
            line: 3,
        });
    });

    // A slow check that lands after the next keystroke would otherwise mark a
    // document broken because of text that is no longer in it.
    test('a check that answers about text already replaced is dropped', async () => {
        await editors.load(TAB, TARGET);
        // Cast because the bindings return a CancellablePromise, which nothing
        // here cancels: a plain promise is what a stubbed call answers with.
        vi.mocked(ResourceService.CheckYAML).mockImplementationOnce((() => {
            editors.edit(TAB, 'moved on\n');
            return Promise.resolve({ valid: false, message: 'stale', line: 9 });
        }) as unknown as typeof ResourceService.CheckYAML);

        editors.edit(TAB, 'first\n');
        await settle();

        expect(editors.doc(TAB).check.valid).toBe(true);
    });

    test('reverting goes back to what the cluster gave', async () => {
        await editors.load(TAB, TARGET);
        editors.edit(TAB, 'edited: yes\n');

        editors.revert(TAB);

        expect(editors.doc(TAB).text).toBe(LIVE);
        expect(editors.isDirty(TAB)).toBe(false);
    });
});

describe('saving', () => {
    test('takes what the server stored, not what was sent', async () => {
        // The server adds what defaulting and admission control decided, and a
        // new resourceVersion. Keeping the sent text would make the next save a
        // conflict against a version nobody else had touched.
        const stored = `${LIVE}  resourceVersion: "4822"\n`;
        vi.mocked(ResourceService.ApplyYAML).mockResolvedValue(stored);

        await editors.load(TAB, TARGET);
        editors.edit(TAB, 'apiVersion: v1\nkind: Pod\n');

        expect(await editors.save(TAB, TARGET)).toBe(true);

        const doc = editors.doc(TAB);
        expect(ResourceService.ApplyYAML).toHaveBeenCalledWith(
            'cfg::prod',
            'pods',
            'default',
            'web',
            'apiVersion: v1\nkind: Pod\n',
        );
        expect(doc.text).toBe(stored);
        expect(doc.saved).toBe(true);
        expect(editors.isDirty(TAB)).toBe(false);
    });

    test('a refused save keeps the text and says why', async () => {
        vi.mocked(ResourceService.ApplyYAML).mockRejectedValue(
            new Error('Operation cannot be fulfilled: the object has been modified'),
        );

        await editors.load(TAB, TARGET);
        editors.edit(TAB, 'edited: yes\n');

        expect(await editors.save(TAB, TARGET)).toBe(false);

        const doc = editors.doc(TAB);
        expect(doc.text).toBe('edited: yes\n');
        expect(doc.error).toContain('has been modified');
        // Still dirty: the edit is still the only copy of what was written.
        expect(editors.isDirty(TAB)).toBe(true);
    });

    test('a document that never loaded is not saved', async () => {
        expect(await editors.save(TAB, TARGET)).toBe(false);
        expect(ResourceService.ApplyYAML).not.toHaveBeenCalled();
    });

    // The object may be on screen elsewhere -- the describe panel most of all
    // -- and what is there is now what the cluster had a moment ago. Saying so
    // here rather than in the editor component means every future write path
    // gets it without having to remember.
    test('a successful save says the object changed', async () => {
        await editors.load(TAB, TARGET);
        editors.edit(TAB, 'edited: yes\n');
        const before = changes.revision(TARGET);

        await editors.save(TAB, TARGET);

        expect(changes.revision(TARGET)).toBe(before + 1);
    });

    test('a refused save says nothing: the cluster still has what it had', async () => {
        vi.mocked(ResourceService.ApplyYAML).mockRejectedValue(new Error('admission webhook denied'));
        await editors.load(TAB, TARGET);
        editors.edit(TAB, 'edited: yes\n');
        const before = changes.revision(TARGET);

        await editors.save(TAB, TARGET);

        expect(changes.revision(TARGET)).toBe(before);
    });
});

test('closing a tab drops its document', async () => {
    await editors.load(TAB, TARGET);
    editors.edit(TAB, 'edited: yes\n');

    editors.forget(TAB);

    expect(editors.doc(TAB).status).toBe('loading');
    expect(editors.isDirty(TAB)).toBe(false);
});
