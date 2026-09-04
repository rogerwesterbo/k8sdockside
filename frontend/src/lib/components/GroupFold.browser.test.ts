import { beforeEach, describe, expect, test } from 'vitest';
import { render } from 'vitest-browser-svelte';
import ContextTree from './ContextTree.svelte';
import { NAV_GROUPS } from '../catalogue';

/** Every row the tree shows: the sections' items plus the pinned dashboard. */
const ALL_ITEMS = NAV_GROUPS.flatMap((g) => g.items).length + 1;
import { workspace } from '../state/workspace.svelte';

const TOKENS = `:root{--bg:#10151c;--bg-sidebar:#151b24;--bg-raised:#212b38;--border:#28323f;
--bg-hover:rgba(255,255,255,.055);--text:#dee5ee;--text-dim:#8d9aaa;--text-faint:#616e7d;
--accent:#4a86ff;--ok:#4cc38a;--error:#e5646d;--radius:6px;--radius-sm:4px;--row-h:30px;
--font:-apple-system,sans-serif;font-family:var(--font);font-size:13px}
body{color:var(--text);margin:0;width:280px}
button{font:inherit;color:inherit;background:none;border:none;padding:0;cursor:pointer}`;

const CTX = { id: 'x', name: 'admin@prod', cluster: 'prod', user: 'admin',
    namespace: '', server: '', file: '/c', current: false };

const height = () => Math.round((document.querySelector('.context') as HTMLElement).getBoundingClientRect().height);
const items = () => document.querySelectorAll('.tree .item').length;
const settle = () => new Promise((r) => setTimeout(r, 120));

beforeEach(() => {
    document.body.innerHTML = '';
    const style = document.createElement('style');
    style.textContent = TOKENS;
    document.head.appendChild(style);
    workspace.files = [{ path: '/c', source: 'manual', error: '', contexts: [CTX] }];
    workspace.expanded = ['x'];
});

// Why the defaults fold anything at all. With close to fifty kinds on offer a
// context cannot be made to fit the sidebar outright -- doing that would mean
// folding away Workloads or Network, which is worse than scrolling. What the
// defaults have to earn is a materially shorter tree whose everyday sections
// are all still there.
test('the default folding hides a large part of the tree', async () => {
    workspace.settings.layout.collapsedGroups = [];
    workspace.settings.contexts = {};
    render(ContextTree, { props: { context: CTX } });
    await settle();
    const openHeight = height();

    document.body.innerHTML = '';
    workspace.settings.layout.collapsedGroups = null;
    render(ContextTree, { props: { context: CTX } });
    await settle();

    expect(height()).toBeLessThan(openHeight * 0.75);
});

// Every section starts shut, so opening a context shows the dashboard and a
// list of headings rather than fifty rows. The dashboard is the exception, and
// the reason it was taken out of a section of its own.
test('by default a context shows only the dashboard and the headings', async () => {
    workspace.settings.layout.collapsedGroups = null;
    workspace.settings.contexts = {};
    render(ContextTree, { props: { context: CTX } });
    await settle();

    const shown = [...document.querySelectorAll('.tree .item')].map((el) => el.textContent?.trim());
    expect(shown).toEqual(['Dashboard']);
    expect(document.querySelectorAll('button.group')).toHaveLength(NAV_GROUPS.length);
});

test('the dashboard cannot be folded away', async () => {
    workspace.settings.layout.collapsedGroups = null;
    workspace.settings.contexts = {};
    render(ContextTree, { props: { context: CTX } });
    await settle();

    // No heading owns it, so there is nothing that could hide it.
    const headings = [...document.querySelectorAll('button.group')].map((b) => b.textContent);
    expect(headings.some((h) => h?.includes('Overview'))).toBe(false);
});

test('unfolding everything gives back every item', async () => {
    workspace.settings.layout.collapsedGroups = [];
    workspace.settings.contexts = {};
    render(ContextTree, { props: { context: CTX } });
    await settle();

    expect(items()).toBe(ALL_ITEMS);
});

test('a folded group hides its items but keeps its heading', async () => {
    workspace.settings.layout.collapsedGroups = ['Workloads'];
    workspace.settings.contexts = {};
    render(ContextTree, { props: { context: CTX } });
    await settle();

    const workloads = NAV_GROUPS.find((g) => g.label === 'Workloads')!;
    expect(items()).toBe(ALL_ITEMS - workloads.items.length);
    expect(document.body.textContent).toContain('Workloads');
});

test('clicking a heading folds it, and clicking again brings it back', async () => {
    workspace.settings.layout.collapsedGroups = [];
    workspace.settings.contexts = {};
    render(ContextTree, { props: { context: CTX } });
    await settle();
    const all = items();

    const heading = [...document.querySelectorAll('button.group')]
        .find((b) => b.textContent?.includes('Workloads')) as HTMLElement;
    heading.click();
    await settle();
    expect(items()).toBeLessThan(all);

    heading.click();
    await settle();
    expect(items()).toBe(all);
});

test('a folded group says how much it is hiding', async () => {
    workspace.settings.layout.collapsedGroups = ['Admission'];
    workspace.settings.contexts = {};
    render(ContextTree, { props: { context: CTX } });
    await settle();

    const heading = [...document.querySelectorAll('button.group')]
        .find((b) => b.textContent?.includes('Admission'))!;
    expect(heading.querySelector('.tally')?.textContent).toBe('6');
});

test('clicking a heading folds it for this cluster only', async () => {
    workspace.settings.layout.collapsedGroups = [];
    workspace.settings.contexts = {};
    render(ContextTree, { props: { context: CTX } });
    await settle();

    const heading = [...document.querySelectorAll('button.group')]
        .find((b) => b.textContent?.includes('Network')) as HTMLElement;
    heading.click();
    await settle();

    expect(workspace.isGroupCollapsed(CTX.id, 'Network')).toBe(true);
    // The shared default, and so every other cluster, is untouched.
    expect(workspace.collapsedGroups).not.toContain('Network');
});

test('a group folded differently here than elsewhere is marked', async () => {
    workspace.settings.layout.collapsedGroups = [];
    workspace.settings.contexts = {};
    render(ContextTree, { props: { context: CTX } });
    await settle();

    const heading = [...document.querySelectorAll('button.group')]
        .find((b) => b.textContent?.includes('Network')) as HTMLElement;
    heading.click();
    await settle();

    const after = [...document.querySelectorAll('button.group')]
        .find((b) => b.textContent?.includes('Network'))!;
    expect(after.classList.contains('local')).toBe(true);
    expect(after.getAttribute('title')).toContain('this cluster');
});

test('alt-clicking applies the change to every cluster', async () => {
    workspace.settings.layout.collapsedGroups = [];
    workspace.settings.contexts = {};
    render(ContextTree, { props: { context: CTX } });
    await settle();

    const heading = [...document.querySelectorAll('button.group')]
        .find((b) => b.textContent?.includes('Network')) as HTMLElement;
    heading.dispatchEvent(new MouseEvent('click', { altKey: true, bubbles: true }));
    await settle();

    expect(workspace.collapsedGroups).toContain('Network');
    expect(workspace.hasFoldingOverride(CTX.id)).toBe(false);
});


test('the sections toggle opens and shuts every section of one context', async () => {
    workspace.settings.layout.collapsedGroups = null;
    workspace.settings.contexts = {};
    render(ContextTree, { props: { context: CTX } });
    await settle();
    expect(items()).toBe(1);

    const toggle = document.querySelector('button.sections') as HTMLElement;
    expect(toggle).toBeTruthy();
    toggle.click();
    await settle();
    expect(items()).toBe(ALL_ITEMS);

    (document.querySelector('button.sections') as HTMLElement).click();
    await settle();
    expect(items()).toBe(1);
});

test('the sections toggle is absent while the context is closed', async () => {
    workspace.expanded = [];
    render(ContextTree, { props: { context: CTX } });
    await settle();

    expect(document.querySelector('button.sections')).toBeNull();
});

describe('clicking the context name', () => {
    const name = () => document.querySelector('.head .label') as HTMLElement;
    const treeShowing = () => document.querySelector('.tree') !== null;

    beforeEach(async () => {
        document.body.innerHTML = '';
        workspace.expanded = [];
        workspace.selectedContextId = null;
        workspace.settings.contexts = {};
        workspace.settings.layout.collapsedGroups = null;
        render(ContextTree, { props: { context: CTX } });
        await settle();
    });

    test('opens the context', async () => {
        expect(treeShowing()).toBe(false);

        name().click();
        await settle();

        expect(treeShowing()).toBe(true);
    });

    test('a second click closes it again rather than doing nothing', async () => {
        name().click();
        await settle();

        name().click();
        await settle();

        expect(treeShowing()).toBe(false);
    });

    test('the twisty still works on its own', async () => {
        (document.querySelector('.head .twisty') as HTMLElement).click();
        await settle();
        expect(treeShowing()).toBe(true);

        (document.querySelector('.head .twisty') as HTMLElement).click();
        await settle();
        expect(treeShowing()).toBe(false);
    });
});
