import { expect, test } from 'vitest';
import { page } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';
import ContainerPills from './ContainerPills.svelte';

const pill = (label: string, tone: string, detail: string) => ({ label, tone, detail });

const HEALTHY = [pill('app', 'ok', 'Running'), pill('sidecar', 'ok', 'Running')];

test('one rectangle per container', async () => {
    render(ContainerPills, { pills: HEALTHY });

    await expect.poll(() => page.getByRole('img').elements()).toHaveLength(2);
});

// Colour is the whole point of the column, and it has to survive as something
// a test -- and a stylesheet -- can name.
test('each rectangle carries the tone it was given', async () => {
    render(ContainerPills, {
        pills: [pill('app', 'ok', 'Running'), pill('boom', 'error', 'CrashLoopBackOff')],
    });

    await expect.poll(() => page.getByRole('img').elements()).toHaveLength(2);
    const [first, second] = page.getByRole('img').elements();
    expect(first.className).toContain('ok');
    expect(second.className).toContain('error');
});

// Colour alone is not an accessible way to say anything, so the state rides
// along in the label a screen reader reads and a tooltip shows.
test('a rectangle says in words what its colour means', async () => {
    render(ContainerPills, { pills: [pill('app', 'error', 'OOMKilled (137)')] });

    await expect.element(page.getByRole('img', { name: /app/ })).toBeVisible();
    await expect.element(page.getByRole('img', { name: /OOMKilled \(137\)/ })).toBeVisible();
});

test('a cell with no containers draws nothing', async () => {
    render(ContainerPills, { pills: [] });

    expect(page.getByRole('img').elements()).toHaveLength(0);
});

// Go sends an empty slice as null, so the one thing this must not do is throw.
test('null is treated as no containers', async () => {
    render(ContainerPills, { pills: null });

    expect(page.getByRole('img').elements()).toHaveLength(0);
});

// A pod with thirty containers must not push the rest of the row off screen.
test('a great many containers are capped, and the count says so', async () => {
    const many = Array.from({ length: 30 }, (_, i) => pill(`c${i}`, 'ok', 'Running'));

    render(ContainerPills, { pills: many });

    await expect.element(page.getByText(/\+\d+/)).toBeVisible();
    expect(page.getByRole('img').elements().length).toBeLessThan(30);
});

// The panel's copy of the row doubles as the log picker, so it has to be able
// to be pressed. The table's copy must not be: a click there selects the row.
test('the rectangles are buttons when they can be chosen', async () => {
    render(ContainerPills, { pills: HEALTHY, selectable: true, selected: ['app'] });

    await expect.poll(() => page.getByRole('button').elements()).toHaveLength(2);
    const [chosen] = page.getByRole('button').elements();
    expect(chosen.getAttribute('aria-pressed')).toBe('true');
});

test('choosing one reports which it was', async () => {
    let chosen = '';
    render(ContainerPills, {
        pills: HEALTHY,
        selectable: true,
        selected: ['app'],
        onchoose: (name: string) => (chosen = name),
    });

    await page.getByRole('button', { name: /sidecar/ }).click();

    expect(chosen).toBe('sidecar');
});
