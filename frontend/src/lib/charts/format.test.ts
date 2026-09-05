import { describe, expect, test } from 'vitest';
import { formatTick, formatValue, ticksFor } from './format';

// A chart labelled "Memory" whose axis reads 536870912 is a chart nobody can
// use, and one reading 0.4823 cores makes you count decimal places.
describe('formatValue', () => {
    test('writes bytes in the binary prefixes memory is read in', () => {
        expect(formatValue(536870912, 'bytes')).toBe('512 MiB');
        expect(formatValue(1024, 'bytes')).toBe('1 KiB');
        expect(formatValue(0, 'bytes')).toBe('0 B');
        expect(formatValue(1536, 'bytes/s')).toBe('1.5 KiB/s');
    });

    // Millicores are how Kubernetes writes a small CPU value and how the reader
    // thinks about it; 0.0042 cores is the same number said worse.
    test('writes a small CPU value in millicores', () => {
        expect(formatValue(0.0042, 'cores')).toBe('4m');
        expect(formatValue(0.42, 'cores')).toBe('0.42 cores');
        expect(formatValue(12.3, 'cores')).toBe('12.3 cores');
    });

    test('writes the other units', () => {
        expect(formatValue(0.87, 'percent')).toBe('87%');
        expect(formatValue(1420, 'ops/s')).toBe('1,420/s');
        expect(formatValue(0.025, 'seconds')).toBe('25 ms');
        expect(formatValue(90, 'seconds')).toBe('1.5 min');
        expect(formatValue(42, 'count')).toBe('42');
    });

    test('groups thousands, because 12000 and 12,000 do not read alike', () => {
        expect(formatValue(12000, 'count')).toBe('12,000');
        expect(formatValue(1234567, '')).toBe('1,234,567');
    });

    test('leaves no trailing zeros', () => {
        expect(formatValue(1.5, 'count')).toBe('1.5');
        expect(formatValue(2, 'count')).toBe('2');
        expect(formatValue(2.0, '')).toBe('2');
    });

    test('says so when there is no number', () => {
        expect(formatValue(NaN, 'bytes')).toBe('—');
        expect(formatValue(Infinity, 'cores')).toBe('—');
    });
});

describe('formatTick', () => {
    // The axis carries no unit suffix — the chart's title already said it —
    // but the scale still has to be right.
    test('scales without repeating the unit', () => {
        expect(formatTick(536870912, 'bytes')).toBe('512 MiB');
        expect(formatTick(0.5, 'percent')).toBe('50%');
        expect(formatTick(0.42, 'cores')).toBe('0.42');
    });
});

// Anchored at zero on purpose: these are rates and sizes, where the distance
// from nothing is what is being read, and an axis starting at 0.38 turns a flat
// line into a mountain range.
describe('ticksFor', () => {
    test('starts at zero and lands on round numbers', () => {
        expect(ticksFor(0.42)).toEqual([0, 0.2, 0.4, 0.6]);
        expect(ticksFor(100)).toEqual([0, 50, 100]);
        expect(ticksFor(9)).toEqual([0, 5, 10]);
    });

    // The top tick is what the plot is scaled against, so one that fell short
    // would draw the tallest line above its own axis.
    test('the top tick is never below the data', () => {
        expect(ticksFor(7)).toEqual([0, 2.5, 5, 7.5]);
    });

    test('covers the whole range', () => {
        for (const max of [0.001, 1, 7, 42, 1023, 536870912]) {
            const ticks = ticksFor(max);
            expect(ticks[0]).toBe(0);
            expect(ticks[ticks.length - 1]).toBeGreaterThanOrEqual(max);
        }
    });

    // A series that is flat at zero still needs a scale to be drawn against.
    test('gives a scale to an empty or flat chart', () => {
        expect(ticksFor(0)).toEqual([0, 1]);
        expect(ticksFor(NaN)).toEqual([0, 1]);
    });
});
