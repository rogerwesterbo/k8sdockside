import { describe, expect, test } from 'vitest';
import {
    DEFAULT_FILTER,
    assessRules,
    buildGraph,
    connected,
    describeRule,
    grantsFor,
    index,
    isMe,
    matrix,
    parseResource,
    ruleAllows,
    whoCan,
    type Access,
    type Rule,
} from './model';

function rule(verbs: string[], resources: string[], apiGroups = [''], resourceNames: string[] = []): Rule {
    return { verbs, apiGroups, resources, resourceNames, nonResourceURLs: [] };
}

/**
 * A small cluster: a namespace-scoped reader, a ClusterRole reused per
 * namespace, cluster-admin handed to one group, and a binding to a role that
 * does not exist.
 */
function cluster(): Access {
    return {
        roles: [
            {
                kind: 'ClusterRole',
                name: 'cluster-admin',
                namespace: '',
                rules: [rule(['*'], ['*'], ['*'])],
                aggregated: false,
                default: true,
            },
            {
                kind: 'ClusterRole',
                name: 'secret-reader',
                namespace: '',
                rules: [rule(['get', 'list'], ['secrets'])],
                aggregated: false,
                default: false,
            },
            {
                kind: 'Role',
                name: 'pod-reader',
                namespace: 'shop',
                rules: [rule(['get', 'list', 'watch'], ['pods', 'pods/log'])],
                aggregated: false,
                default: false,
            },
            {
                kind: 'ClusterRole',
                name: 'system:controller:thing',
                namespace: '',
                rules: [rule(['get'], ['pods'])],
                aggregated: false,
                default: true,
            },
        ],
        bindings: [
            {
                kind: 'ClusterRoleBinding',
                name: 'admins',
                namespace: '',
                roleKind: 'ClusterRole',
                roleName: 'cluster-admin',
                subjects: [{ kind: 'Group', name: 'ops', namespace: '' }],
                default: false,
            },
            {
                kind: 'RoleBinding',
                name: 'secrets-in-shop',
                namespace: 'shop',
                roleKind: 'ClusterRole',
                roleName: 'secret-reader',
                subjects: [{ kind: 'ServiceAccount', name: 'robot', namespace: 'shop' }],
                default: false,
            },
            {
                kind: 'RoleBinding',
                name: 'readers',
                namespace: 'shop',
                roleKind: 'Role',
                roleName: 'pod-reader',
                subjects: [
                    { kind: 'User', name: 'alice', namespace: '' },
                    { kind: 'Group', name: 'system:serviceaccounts:shop', namespace: '' },
                ],
                default: false,
            },
            {
                kind: 'RoleBinding',
                name: 'dangling',
                namespace: 'shop',
                roleKind: 'Role',
                roleName: 'gone',
                subjects: [{ kind: 'User', name: 'bob', namespace: '' }],
                default: false,
            },
            {
                kind: 'ClusterRoleBinding',
                name: 'system:controller:thing',
                namespace: '',
                roleKind: 'ClusterRole',
                roleName: 'system:controller:thing',
                subjects: [{ kind: 'ServiceAccount', name: 'thing', namespace: 'kube-system' }],
                default: true,
            },
        ],
        serviceAccounts: [
            { name: 'robot', namespace: 'shop' },
            { name: 'thing', namespace: 'kube-system' },
        ],
        me: { username: 'alice', groups: ['ops', 'system:authenticated'], error: '' },
        unreadable: [],
        error: '',
    };
}

describe('rule matching', () => {
    test('follows the authorizer on groups, subresources and wildcards', () => {
        expect(ruleAllows(rule(['get'], ['pods']), 'get', parseResource('pods'))).toBe(true);
        expect(ruleAllows(rule(['get'], ['pods']), 'get', parseResource('pods/log'))).toBe(false);
        expect(ruleAllows(rule(['get'], ['*/log']), 'get', parseResource('pods/log'))).toBe(true);
        expect(ruleAllows(rule(['get'], ['pods']), 'get', parseResource('pods.apps'))).toBe(false);
        expect(ruleAllows(rule(['*'], ['*'], ['*']), 'delete', parseResource('deployments.apps/scale'))).toBe(true);
    });

    test('reads kubectl resource names', () => {
        expect(parseResource('deployments.apps/scale')).toEqual({
            resource: 'deployments',
            group: 'apps',
            subresource: 'scale',
        });
        expect(parseResource('httproutes.gateway.networking.k8s.io')).toEqual({
            resource: 'httproutes',
            group: 'gateway.networking.k8s.io',
            subresource: '',
        });
    });
});

describe('who can', () => {
    test('a RoleBinding to a ClusterRole grants only in its namespace', () => {
        const a = cluster();
        const idx = index(a);
        const inShop = whoCan(a, idx, { verb: 'get', resource: 'secrets', namespace: 'shop' }).map((x) => x.subject.name);
        const elsewhere = whoCan(a, idx, { verb: 'get', resource: 'secrets', namespace: 'other' }).map(
            (x) => x.subject.name,
        );

        expect(inShop).toEqual(['ops', 'robot']);
        expect(elsewhere).toEqual(['ops']);
    });

    test('names the route', () => {
        const a = cluster();
        const [robot] = whoCan(a, index(a), { verb: 'list', resource: 'secrets', namespace: 'shop' }).filter(
            (x) => x.subject.name === 'robot',
        );
        expect(robot.routes[0].binding.name).toBe('secrets-in-shop');
        expect(robot.routes[0].role.name).toBe('secret-reader');
        expect(robot.routes[0].scope).toBe('shop');
    });
});

describe('grants for a subject', () => {
    // The group binding reaches every service account in shop without naming
    // any of them -- which is the grant people miss reading YAML.
    test('include the groups a service account is in by construction', () => {
        const a = cluster();
        const grants = grantsFor(a, index(a), { kind: 'ServiceAccount', name: 'robot', namespace: 'shop' });
        expect(grants.map((g) => [g.binding.name, g.viaGroup])).toEqual([
            ['secrets-in-shop', ''],
            ['readers', 'system:serviceaccounts:shop'],
        ]);
    });

    test('recognise the caller and their groups', () => {
        const { me } = cluster();
        expect(isMe({ kind: 'User', name: 'alice', namespace: '' }, me)).toBe(true);
        expect(isMe({ kind: 'Group', name: 'ops', namespace: '' }, me)).toBe(true);
        expect(isMe({ kind: 'User', name: 'bob', namespace: '' }, me)).toBe(false);
    });
});

describe('the graph', () => {
    test('hides the system objects unless asked', () => {
        const a = cluster();
        const idx = index(a);
        const plain = buildGraph(a, idx, DEFAULT_FILTER);
        const all = buildGraph(a, idx, { ...DEFAULT_FILTER, showSystem: true });

        expect(plain.bindings.map((b) => b.name)).not.toContain('system:controller:thing');
        expect(all.bindings.map((b) => b.name)).toContain('system:controller:thing');
    });

    test('marks a binding to a missing role', () => {
        const a = cluster();
        const g = buildGraph(a, index(a), DEFAULT_FILTER);
        expect(g.roles.find((r) => r.name === 'gone')?.missing).toBe(true);
    });

    test('a namespace keeps its own bindings and the cluster-wide ones', () => {
        const a = cluster();
        const idx = index(a);
        const other = buildGraph(a, idx, { ...DEFAULT_FILTER, namespace: 'other' });
        expect(other.bindings.map((b) => b.name)).toEqual(['admins']);

        const bare = buildGraph(a, idx, { ...DEFAULT_FILTER, namespace: 'other', hideClusterWide: true });
        expect(bare.bindings).toEqual([]);
    });

    test('carries risk back to whoever holds the role', () => {
        const a = cluster();
        const g = buildGraph(a, index(a), DEFAULT_FILTER);
        expect(g.subjects.find((s) => s.name === 'ops')?.risk).toBe('critical');
        expect(g.subjects.find((s) => s.name === 'robot')?.risk).toBe('high');
    });

    test('marks the chains that reach the caller', () => {
        const a = cluster();
        const g = buildGraph(a, index(a), { ...DEFAULT_FILTER, onlyMe: true });
        expect(g.bindings.map((b) => b.name).sort()).toEqual(['admins', 'readers']);
        expect(g.subjects.filter((s) => s.me).map((s) => s.name).sort()).toEqual(['alice', 'ops']);
    });

    test('lights a role and whoever reaches it, but not their other roles', () => {
        const a = cluster();
        const g = buildGraph(a, index(a), DEFAULT_FILTER);
        const lit = connected(g, 'r:Role/shop/pod-reader');
        expect(lit.has('s:User//alice')).toBe(true);
        expect(lit.has('r:ClusterRole//cluster-admin')).toBe(false);
    });
});

describe('words and risk', () => {
    test('rules read as sentences', () => {
        expect(describeRule(rule(['*'], ['*'], ['*']))).toBe('Can do anything to anything');
        expect(describeRule(rule(['get', 'list', 'watch'], ['pods']))).toBe('Can read pods');
        expect(describeRule(rule(['get'], ['configmaps'], [''], ['app']))).toBe(
            'Can get configmaps -- only those named app',
        );
    });

    test('risk ranks what matters', () => {
        expect(assessRules([rule(['get', 'list'], ['pods'])]).level).toBe('none');
        expect(assessRules([rule(['create'], ['configmaps'])]).level).toBe('low');
        expect(assessRules([rule(['get'], ['secrets'])]).level).toBe('high');
        expect(assessRules([rule(['impersonate'], ['users'])]).level).toBe('critical');
    });

    test('the matrix merges rules and marks named-only access', () => {
        const rows = matrix([rule(['get'], ['pods']), rule(['delete'], ['pods'], [''], ['one'])]);
        expect(rows).toHaveLength(1);
        expect(rows[0].cells.get).toBe('yes');
        expect(rows[0].cells.delete).toBe('some');
        expect(rows[0].names).toEqual(['one']);
    });
});
