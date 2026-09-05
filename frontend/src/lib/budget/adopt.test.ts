import { describe, expect, test } from 'vitest';
import { adoptBudget, barsFor, type BudgetAmount } from './adopt';

/** An amount with everything filled in, for tests to vary one field of. */
function amount(over: Partial<BudgetAmount> = {}): BudgetAmount {
    return {
        label: 'CPU',
        unit: 'cores',
        capacity: 16,
        allocatable: 15,
        requested: 6,
        limits: 30,
        used: 3,
        hasCapacity: true,
        hasDemand: true,
        hasUsed: true,
        ...over,
    };
}

describe('barsFor', () => {
    test('measures everything against allocatable, not capacity', () => {
        // Allocatable is what the scheduler may actually hand out. Drawing
        // against capacity would show a node as having room the kubelet has
        // already reserved for itself.
        const bars = barsFor(amount());
        const requested = bars.find((b) => b.label === 'Requested');

        expect(requested?.percent).toBe(40); // 6 of 15, not 6 of 16
    });

    test('falls back to capacity when there is no allocatable', () => {
        const bars = barsFor(amount({ allocatable: 0, capacity: 12, requested: 6 }));

        expect(bars.find((b) => b.label === 'Requested')?.percent).toBe(50);
    });

    test('marks a value above the ceiling as overcommitted', () => {
        // Limits above allocatable is the normal, deliberate state of most
        // clusters, but it is the thing worth seeing at a glance.
        const bars = barsFor(amount({ allocatable: 15, limits: 30 }));
        const limits = bars.find((b) => b.label === 'Limits');

        expect(limits?.overcommitted).toBe(true);
        expect(limits?.percent).toBe(100); // the bar stops at full width
        expect(limits?.value).toBe(30); // the number does not
    });

    test('leaves out requests and limits where they mean nothing', () => {
        const bars = barsFor(amount({ label: 'Pods', hasDemand: false }));

        expect(bars.map((b) => b.label)).not.toContain('Requested');
        expect(bars.map((b) => b.label)).not.toContain('Limits');
        expect(bars.map((b) => b.label)).toContain('Used');
    });

    test('leaves out usage when nothing measured it', () => {
        const bars = barsFor(amount({ hasUsed: false }));

        expect(bars.map((b) => b.label)).not.toContain('Used');
    });

    test('compares the bars to each other when there is no ceiling', () => {
        // A namespace with no quota. There is no capacity to be a fraction of,
        // but requested against limits against used is still worth seeing, so
        // the bars are scaled to the largest of themselves and flagged as
        // relative so nothing reads them as "% of the cluster".
        const bars = barsFor(amount({ hasCapacity: false, allocatable: 0, capacity: 0, requested: 1.2, limits: 0.6, used: 0.3 }));

        const requested = bars.find((b) => b.label === 'Requested');
        const limits = bars.find((b) => b.label === 'Limits');
        const used = bars.find((b) => b.label === 'Used');

        expect(requested?.percent).toBe(100); // the largest sets the scale
        expect(limits?.percent).toBe(50);
        expect(used?.percent).toBe(25);
        expect(bars.every((b) => b.relative)).toBe(true);
        expect(bars.every((b) => b.track)).toBe(true);
        // Nothing is over anything when there is no ceiling to be over.
        expect(bars.every((b) => !b.overcommitted)).toBe(true);
    });

    test('draws no track for a lone bar with no ceiling', () => {
        // Pods in an unquota'd namespace: one value, nothing to compare it to.
        // Scaling it against itself would draw a full bar and mean nothing.
        const bars = barsFor(amount({ label: 'Pods', hasDemand: false, hasCapacity: false, allocatable: 0, capacity: 0, used: 4 }));

        expect(bars).toHaveLength(1);
        expect(bars[0].track).toBe(false);
        expect(bars[0].value).toBe(4); // the number is still shown
    });

    test('keeps the track against a real ceiling even at zero', () => {
        // "Nothing requested of 15 cores" is a fact worth drawing; an empty bar
        // there is honest rather than broken.
        const bars = barsFor(amount({ requested: 0 }));
        const requested = bars.find((b) => b.label === 'Requested');

        expect(requested?.percent).toBe(0);
        expect(requested?.track).toBe(true);
        expect(requested?.relative).toBe(false);
    });
});

describe('adoptBudget', () => {
    test('survives a reply with nothing in it', () => {
        // Wails renders a nil Go slice as null.
        const budget = adoptBudget({ scope: null, amounts: null, usage: null, error: '' } as never);

        expect(budget.amounts).toEqual([]);
        expect(budget.usage.source).toBe('');
        expect(budget.error).toBe('');
    });

    test('carries why the used column is empty', () => {
        const budget = adoptBudget({
            scope: { kind: 'cluster', name: '' },
            amounts: [],
            usage: { source: '', error: 'metrics-server: not found; prometheus: none found' },
            error: '',
        } as never);

        expect(budget.usage.error).toContain('metrics-server');
        expect(budget.usage.source).toBe('');
    });
});
