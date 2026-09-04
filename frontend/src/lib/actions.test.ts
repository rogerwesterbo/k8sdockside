import { describe, expect, test } from 'vitest';
import { actionsFor, type ActionId } from './actions';
import { DASHBOARD, HELM_RELEASES, SETTINGS, customKindFor } from './catalogue';

/** The action ids a kind offers, which is all these tests care about. */
function ids(kind: string): ActionId[] {
    return actionsFor(kind).map((a) => a.id);
}

describe('what every object can do', () => {
    test.each(['configmaps', 'secrets', 'clusterroles', 'persistentvolumes'])(
        '%s can be edited and deleted',
        (kind) => {
            expect(ids(kind)).toEqual(['edit', 'delete']);
        },
    );

    // A custom resource is an object like any other. Nothing here is compiled
    // in per kind, so a CRD nobody has heard of gets the same two.
    test('so can a custom resource', () => {
        expect(ids(customKindFor('certificates.cert-manager.io'))).toEqual(['edit', 'delete']);
    });

    // A Helm release is a Secret the backend decodes, not a Kubernetes object:
    // there is nothing here to edit and nothing to delete that would mean what
    // it looks like it means.
    test('a Helm release offers nothing', () => {
        expect(actionsFor(HELM_RELEASES)).toEqual([]);
    });

    test.each([DASHBOARD, SETTINGS])('the %s view offers nothing', (kind) => {
        expect(actionsFor(kind)).toEqual([]);
    });
});

describe('what particular kinds can do', () => {
    test('a pod offers its logs', () => {
        expect(ids('pods')).toEqual(['edit', 'logs', 'delete']);
    });

    // A workload's logs are every container of every pod its selector finds,
    // which is the view worth having while a rollout is happening.
    test.each(['deployments', 'statefulsets', 'daemonsets', 'jobs', 'cronjobs'])(
        'a %s offers its logs too',
        (kind) => {
            expect(ids(kind)).toContain('logs');
        },
    );

    // Nothing here has containers, so there is nothing to follow.
    test.each(['configmaps', 'nodes', 'services', 'secrets'])('a %s offers no logs', (kind) => {
        expect(ids(kind)).not.toContain('logs');
    });

    test('a node can be cordoned and drained', () => {
        expect(ids('nodes')).toEqual(['edit', 'cordon', 'drain', 'delete']);
    });

    test.each(['deployments', 'statefulsets'])('a %s can be scaled and restarted', (kind) => {
        expect(ids(kind)).toEqual(['edit', 'logs', 'scale', 'restart', 'delete']);
    });

    // A DaemonSet runs one pod per node, so there is no replica count to set --
    // but it does roll.
    test('a daemonset restarts but does not scale', () => {
        expect(ids('daemonsets')).toEqual(['edit', 'logs', 'restart', 'delete']);
    });

    // A ReplicaSet has a replica count, but rolling one means nothing: the
    // Deployment above it owns the template that a restart would stamp.
    test('a replicaset scales but does not restart', () => {
        expect(ids('replicasets')).toEqual(['edit', 'logs', 'scale', 'delete']);
    });
});

describe('how the bar treats each one', () => {
    test('delete asks first, and is marked as the dangerous one', () => {
        const del = actionsFor('pods').find((a) => a.id === 'delete');
        expect(del?.form).toBe('confirm');
        expect(del?.tone).toBe('danger');
    });

    // Draining a node moves every workload off it. That is not a one-click
    // action however routine it feels.
    test('drain asks first too', () => {
        expect(actionsFor('nodes').find((a) => a.id === 'drain')?.form).toBe('confirm');
    });

    test('scale asks for a number', () => {
        expect(actionsFor('deployments').find((a) => a.id === 'scale')?.form).toBe('number');
    });

    test('cordon and restart just happen', () => {
        expect(actionsFor('nodes').find((a) => a.id === 'cordon')?.form).toBe('immediate');
        expect(actionsFor('deployments').find((a) => a.id === 'restart')?.form).toBe('immediate');
    });

    test('every action has a label and an icon', () => {
        for (const kind of ['nodes', 'deployments', 'daemonsets', 'pods']) {
            for (const action of actionsFor(kind)) {
                expect(action.label, `${kind}/${action.id}`).not.toBe('');
                expect(action.icon, `${kind}/${action.id}`).not.toBe('');
            }
        }
    });
});
