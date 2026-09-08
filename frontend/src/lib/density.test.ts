import { describe, expect, test } from 'vitest';
import { rowMetrics } from './density';

const DENSITIES = ['compact', 'comfortable', 'spacious'];

describe('row metrics', () => {
    test('every density the settings offer has metrics', () => {
        for (const density of DENSITIES) {
            expect(rowMetrics(density).height).toMatch(/^\d+px$/);
            expect(rowMetrics(density).padding).toMatch(/^\d+px$/);
        }
    });

    // The control reads as a scale, so it had better be one. A "spacious" that
    // was shorter than "comfortable" would be a typo nothing else would catch.
    test('they get taller in the order the control lists them', () => {
        const heights = DENSITIES.map((d) => parseInt(rowMetrics(d).height, 10));
        expect(heights).toEqual([...heights].sort((a, b) => a - b));
        expect(new Set(heights).size).toBe(heights.length);
    });

    // The row height is what a list measures with; the padding is what actually
    // produces it. A padding that did not grow with the height would leave the
    // text pinned to the top of a taller row.
    test('the padding grows with the height', () => {
        const pads = DENSITIES.map((d) => parseInt(rowMetrics(d).padding, 10));
        expect(pads).toEqual([...pads].sort((a, b) => a - b));
    });

    // An unset --row-h collapses every row in the sidebar to its content, which
    // is a broken window rather than a wrong preference.
    test('a density this build does not know falls back rather than to nothing', () => {
        expect(rowMetrics('roomy')).toEqual(rowMetrics('comfortable'));
        expect(rowMetrics('')).toEqual(rowMetrics('comfortable'));
    });
});
