import { describe, expect, test } from 'vitest';
import { DASHBOARD, DASHBOARD_ITEM, DEFAULT_COLLAPSED_GROUPS, NAV_GROUPS, groupForKind, iconFor, labelFor, singularFor } from './catalogue';
import { PATHS } from './components/Icon.svelte';

const ITEMS = NAV_GROUPS.flatMap((group) => group.items);
const KINDS = ITEMS.map((item) => item.kind);

describe('the resource catalogue', () => {
    test('never lists the same kind twice', () => {
        expect(KINDS).toHaveLength(new Set(KINDS).size);
    });

    // An icon name with no matching path renders an empty <svg>: the row still
    // lays out, so a typo here is invisible until someone looks closely.
    test('every nav item names an icon that exists', () => {
        const missing = ITEMS.filter((item) => !(item.icon in PATHS)).map((item) => `${item.kind}:${item.icon}`);
        expect(missing).toEqual([]);
    });

    test('every group has a label and at least one item', () => {
        for (const group of NAV_GROUPS) {
            expect(group.label).not.toBe('');
            expect(group.items.length).toBeGreaterThan(0);
        }
    });
});

describe('the kinds added beyond the original set', () => {
    const ADDED: [string, string][] = [
        ['replicasets', 'Replica Sets'],
        ['replicationcontrollers', 'Replication Controllers'],
        ['horizontalpodautoscalers', 'Horizontal Pod Autoscalers'],
        ['resourcequotas', 'Resource Quotas'],
        ['limitranges', 'Limit Ranges'],
        ['leases', 'Leases'],
        ['poddisruptionbudgets', 'Pod Disruption Budgets'],
        ['priorityclasses', 'Priority Classes'],
        ['runtimeclasses', 'Runtime Classes'],
        ['mutatingwebhookconfigurations', 'Mutating Webhooks'],
        ['validatingwebhookconfigurations', 'Validating Webhooks'],
        ['mutatingadmissionpolicies', 'Mutating Admission Policies'],
        ['mutatingadmissionpolicybindings', 'Mutating Policy Bindings'],
        ['validatingadmissionpolicies', 'Validating Admission Policies'],
        ['validatingadmissionpolicybindings', 'Validating Policy Bindings'],
    ];

    test.each(ADDED)('%s is offered in the sidebar as "%s"', (kind, label) => {
        expect(KINDS).toContain(kind);
        expect(labelFor(kind)).toBe(label);
        expect(iconFor(kind)).not.toBe('box');
    });

    test('the scheduling and admission groups exist', () => {
        const labels = NAV_GROUPS.map((g) => g.label);
        expect(labels).toContain('Scheduling');
        expect(labels).toContain('Admission');
    });

    // The detail panel titles one row with this, so "Leases" must not become
    // "Leafe" or similar through the plural-stripping.
    test('singular forms read correctly for the new kinds', () => {
        expect(singularFor('leases')).toBe('Lease');
        expect(singularFor('replicasets')).toBe('Replica Set');
        expect(singularFor('priorityclasses')).toBe('Priority Class');
    });
});

describe('groupForKind', () => {
    test('names the section a resource is listed under', () => {
        expect(groupForKind('pods')).toBe('Workloads');
        expect(groupForKind('mutatingwebhookconfigurations')).toBe('Admission');
        expect(groupForKind('priorityclasses')).toBe('Scheduling');
    });

    // The dashboard is not in a section any more: it sits at the top of the
    // tree on its own, so there is never a section to unfold to reach it.
    test('the dashboard belongs to no section', () => {
        expect(groupForKind(DASHBOARD)).toBeNull();
    });

    // Custom resources are opened from the definitions table and have no nav
    // entry, so there is no section to speak of.
    test('a custom resource belongs to no section', () => {
        expect(groupForKind('crd:certificates.cert-manager.io')).toBeNull();
    });

    test('an unknown kind belongs to no section', () => {
        expect(groupForKind('nonsense')).toBeNull();
    });

    test('every kind the sidebar offers can be traced back to its section', () => {
        for (const group of NAV_GROUPS) {
            for (const item of group.items) {
                expect(groupForKind(item.kind)).toBe(group.label);
            }
        }
    });
});

describe('the dashboard entry', () => {
    test('is offered on its own rather than inside a section', () => {
        expect(DASHBOARD_ITEM.kind).toBe(DASHBOARD);
        expect(NAV_GROUPS.flatMap((g) => g.items).map((i) => i.kind)).not.toContain(DASHBOARD);
    });

    test('still resolves its label and icon like any other kind', () => {
        expect(labelFor(DASHBOARD)).toBe('Dashboard');
        expect(iconFor(DASHBOARD)).toBe('dashboard');
    });

    test('there is no Overview section left', () => {
        expect(NAV_GROUPS.map((g) => g.label)).not.toContain('Overview');
    });
});

describe('the default folding', () => {
    // Every section starts shut. Expanding a context then shows the dashboard
    // and a short list of headings rather than fifty rows.
    test('folds every section', () => {
        expect([...DEFAULT_COLLAPSED_GROUPS].sort()).toEqual(NAV_GROUPS.map((g) => g.label).sort());
    });

    test('follows the sections rather than a list kept in step by hand', () => {
        expect(DEFAULT_COLLAPSED_GROUPS).toHaveLength(NAV_GROUPS.length);
    });
});
