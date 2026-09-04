import { expect, test } from 'vitest';
import { page } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';
import SortableTable from './SortableTable.svelte';
import type { Row } from '../state/adopt';

function row(id: string, ...cells: ([string] | [string, string])[]): Row {
    return {
        id,
        name: id,
        namespace: '',
        cells: cells.map(([text, sort]) => ({ text, tone: '', sort: sort ?? '', pills: null })),
    };
}

/** The rendered text of one column, top to bottom. */
function columnText(index: number): string[] {
    return [...document.querySelectorAll('tbody tr')].map(
        (tr) => tr.children[index]?.textContent?.trim() ?? '',
    );
}

const AGES = ['Name', 'Last Seen'];

// Ages are the case the sort key exists for: as text, "2h" < "3d" < "5m".
const byAge = [
    row('a', ['a'], ['5m', '300']),
    row('b', ['b'], ['2h', '7200']),
    row('c', ['c'], ['3d', '259200']),
];

test('rows keep the order they were given until a header is clicked', async () => {
    render(SortableTable, { columns: AGES, rows: byAge });

    await expect.element(page.getByRole('row').nth(1)).toBeVisible();
    expect(columnText(1)).toEqual(['5m', '2h', '3d']);
});

test('clicking a header sorts by the cell sort key, not its text', async () => {
    render(SortableTable, { columns: AGES, rows: [byAge[1], byAge[2], byAge[0]] });

    await page.getByRole('button', { name: 'Last Seen' }).click();

    // Text order would have given 2h, 3d, 5m.
    await expect.poll(() => columnText(1)).toEqual(['5m', '2h', '3d']);
});

test('clicking the same header again reverses it', async () => {
    render(SortableTable, { columns: AGES, rows: byAge });

    await page.getByRole('button', { name: 'Last Seen' }).click();
    await page.getByRole('button', { name: 'Last Seen' }).click();

    await expect.poll(() => columnText(1)).toEqual(['3d', '2h', '5m']);
});

test('a column without sort keys falls back to its text', async () => {
    render(SortableTable, {
        columns: AGES,
        rows: [row('c', ['charlie'], ['1m', '60']), row('a', ['alpha'], ['2m', '120'])],
    });

    await page.getByRole('button', { name: 'Name' }).click();

    await expect.poll(() => columnText(0)).toEqual(['alpha', 'charlie']);
});

test('the sorted column is announced to assistive tech', async () => {
    render(SortableTable, { columns: AGES, rows: byAge });

    await page.getByRole('button', { name: 'Name' }).click();

    await expect.poll(() => document.querySelectorAll('th[aria-sort="ascending"]').length).toBe(1);
});

test('an empty table says so in the caller\'s words', async () => {
    render(SortableTable, { columns: AGES, rows: [], empty: 'Nothing to report.' });

    await expect.element(page.getByText('Nothing to report.')).toBeVisible();
});

test('clicking a row reports it to the caller', async () => {
    let picked: Row | null = null;
    render(SortableTable, { columns: AGES, rows: byAge, onselect: (r: Row) => (picked = r) });

    await page.getByRole('cell', { name: 'b' }).click();

    expect(picked).not.toBeNull();
    expect(picked!.id).toBe('b');
});
