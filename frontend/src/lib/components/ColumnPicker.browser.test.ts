import { expect, test } from 'vitest';
import { page } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';
import ColumnPicker from './ColumnPicker.svelte';
import { describeColumns } from '../columns';

const POD = describeColumns(['Name', 'Ready', 'Node', 'Age']);

/** Opens the panel, which every test here needs first. */
async function open(props: Record<string, unknown> = {}) {
    const seen = { toggled: [] as [string, boolean][], showall: 0, reset: 0 };
    render(ColumnPicker, {
        columns: POD,
        hidden: [],
        resized: false,
        ontoggle: (key: string, off: boolean) => seen.toggled.push([key, off]),
        onshowall: () => seen.showall++,
        onreset: () => seen.reset++,
        ...props,
    });
    await page.getByRole('button', { name: /Columns|of 4/ }).click();
    return seen;
}

test('every column is listed, ticked when it is shown', async () => {
    await open({ hidden: ['Node'] });

    await expect.element(page.getByRole('menuitemcheckbox', { name: 'Name' })).toBeVisible();
    expect(document.querySelectorAll('[role="menuitemcheckbox"]').length).toBe(4);
    expect(
        document.querySelector('[role="menuitemcheckbox"][aria-checked="false"]')?.textContent,
    ).toContain('Node');
});

test('unticking a column reports it to the caller', async () => {
    const seen = await open();

    await page.getByRole('menuitemcheckbox', { name: 'Node' }).click();

    expect(seen.toggled).toEqual([['Node', true]]);
});

test('ticking a hidden column back on reports it too', async () => {
    const seen = await open({ hidden: ['Node'] });

    await page.getByRole('menuitemcheckbox', { name: 'Node' }).click();

    expect(seen.toggled).toEqual([['Node', false]]);
});

// A table of nothing is a stack of blank rows, and this panel -- the only way
// the columns come back -- would by then list nothing to tick.
test('the last shown column cannot be unticked', async () => {
    const seen = await open({ hidden: ['Ready', 'Node', 'Age'] });

    const last = page.getByRole('menuitemcheckbox', { name: 'Name' });
    await expect.element(last).toBeDisabled();
    await last.click({ force: true });

    expect(seen.toggled).toEqual([]);
});

test('the trigger says how many columns are showing once some are not', async () => {
    await open({ hidden: ['Node'] });

    await expect.element(page.getByRole('button', { name: '3 of 4' })).toBeVisible();
});

test('showing them all, and resetting, reach the caller', async () => {
    const seen = await open({ hidden: ['Node'], resized: true });

    await page.getByRole('button', { name: 'Show all columns' }).click();
    await page.getByRole('button', { name: 'Reset widths and columns' }).click();

    expect(seen.showall).toBe(1);
    expect(seen.reset).toBe(1);
});

// Nothing to give back is nothing to offer: a table nobody has touched has both
// of these greyed out rather than doing nothing when pressed.
test('both are offered only when there is something to undo', async () => {
    await open();

    await expect.element(page.getByRole('button', { name: 'Show all columns' })).toBeDisabled();
    await expect
        .element(page.getByRole('button', { name: 'Reset widths and columns' }))
        .toBeDisabled();
});

test('a width dragged is reason enough to offer a reset', async () => {
    await open({ resized: true });

    await expect
        .element(page.getByRole('button', { name: 'Reset widths and columns' }))
        .toBeEnabled();
});

// Two columns can share a name -- a CRD printer column called "Name" beside the
// Name the app puts first -- and ticking one must not tick the other.
test('a repeated column name is ticked on its own', async () => {
    const seen = await open({ columns: describeColumns(['Name', 'Ready', 'Name']) });

    // Two rows read "Name"; the second is the CRD's own.
    const rows = [...document.querySelectorAll('[role="menuitemcheckbox"]')];
    (rows[2] as HTMLElement).click();

    expect(seen.toggled).toEqual([['Name#2', true]]);
});

test('Escape shuts the panel', async () => {
    await open();
    await expect.element(page.getByRole('menu', { name: 'Columns' })).toBeVisible();

    document
        .querySelector('[role="menu"]')
        ?.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }));

    await expect.poll(() => document.querySelector('[role="menu"]')).toBeNull();
});
