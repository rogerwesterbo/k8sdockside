import { expect, test } from 'vitest';

// The change signal is a leaf: it talks to no cluster and to no other store,
// so there is nothing to stub. Each test names its own object, because
// revisions only ever go up and are never reset.
const { changes } = await import('./changes.svelte');

const PROD = '/home/u/.kube/prod::admin@prod';
const STAGING = '/home/u/.kube/staging::admin@staging';

/** One object, named after the test that owns it. */
function ref(name: string, namespace = 'default', contextId = PROD) {
    return { contextId, kind: 'pods', namespace, name };
}

test('an object nobody has written is at revision zero', () => {
    expect(changes.revision(ref('untouched'))).toBe(0);
});

test('there is no revision when there is no object', () => {
    expect(changes.revision(null)).toBe(0);
});

test('writing an object moves its revision on', () => {
    const web = ref('moves-on');
    const before = changes.revision(web);

    changes.changed(web);

    expect(changes.revision(web)).toBe(before + 1);
});

test('a second write moves it on again', () => {
    const web = ref('twice');

    changes.changed(web);
    changes.changed(web);

    expect(changes.revision(web)).toBe(2);
});

// The three tests below are the whole reason the signal is keyed rather than
// global: a panel describing one object must not re-read because a different
// one was saved.
test('another object in the same namespace is left alone', () => {
    changes.changed(ref('written'));

    expect(changes.revision(ref('bystander'))).toBe(0);
});

test('the same name in another namespace is a different object', () => {
    changes.changed(ref('shared-name', 'default'));

    expect(changes.revision(ref('shared-name', 'kube-system'))).toBe(0);
});

test('the same name in another cluster is a different object', () => {
    changes.changed(ref('same-everywhere', 'default', PROD));

    expect(changes.revision(ref('same-everywhere', 'default', STAGING))).toBe(0);
});

test('the same name of another kind is a different object', () => {
    const pod = { contextId: PROD, kind: 'pods', namespace: 'default', name: 'web' };
    const service = { contextId: PROD, kind: 'services', namespace: 'default', name: 'web' };

    changes.changed(pod);

    expect(changes.revision(service)).toBe(0);
});

// A cluster-scoped object has no namespace. It must still be told apart from
// whatever shares its name inside one.
test('a cluster-scoped object is told apart from a namespaced one', () => {
    changes.changed({ contextId: PROD, kind: 'nodes', namespace: '', name: 'node-1' });

    expect(changes.revision(ref('node-1'))).toBe(0);
});
