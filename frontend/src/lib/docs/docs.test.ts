import { expect, test } from 'vitest';
import { DASHBOARD, HELM_RELEASES, NAV_GROUPS, DASHBOARD_ITEM } from '../catalogue';
import { PATHS } from '../components/Icon.svelte';
import { HELP } from './help';
import { KUBERNETES_PRIMER } from './kubernetes';
import type { Page } from './types';

// The pages are data, so the mistakes they can carry are data mistakes: a
// "show me" that names a kind the sidebar does not list, an icon that is not
// drawn, a section id that collides. Each would be a dead button somewhere
// deep in a page nobody proof-reads twice, so they are checked here instead.

const PAGES: Page[] = [HELP, KUBERNETES_PRIMER];

const KINDS = new Set<string>([
    DASHBOARD,
    DASHBOARD_ITEM.kind,
    HELM_RELEASES,
    ...NAV_GROUPS.flatMap((group) => group.items.map((item) => item.kind)),
]);

function shownKinds(page: Page): string[] {
    const out: string[] = [];
    for (const section of page.sections) {
        for (const block of section.blocks) {
            if (block.type === 'actions') {
                for (const action of block.actions) if (action.kind === 'show') out.push(action.resource);
            }
            if (block.type === 'terms') {
                for (const term of block.terms) if (term.resource) out.push(term.resource);
            }
        }
    }
    return out;
}

test.each(PAGES.map((p) => [p.title, p] as const))('%s only offers to show kinds the sidebar lists', (_title, page) => {
    const unknown = shownKinds(page).filter((kind) => !KINDS.has(kind));
    expect(unknown).toEqual([]);
});

test.each(PAGES.map((p) => [p.title, p] as const))('%s names only icons that exist', (_title, page) => {
    const missing = page.sections.filter((s) => !(s.icon in PATHS)).map((s) => `${s.id}:${s.icon}`);
    expect(missing).toEqual([]);
});

test.each(PAGES.map((p) => [p.title, p] as const))('%s has unique section ids', (_title, page) => {
    const ids = page.sections.map((s) => s.id);
    expect(new Set(ids).size).toBe(ids.length);
});

test.each(PAGES.map((p) => [p.title, p] as const))('%s links only to https pages', (_title, page) => {
    const bad: string[] = [];
    for (const section of page.sections) {
        for (const block of section.blocks) {
            if (block.type === 'links') for (const link of block.links) if (!link.href.startsWith('https://')) bad.push(link.href);
            if (block.type === 'terms') for (const term of block.terms) if (term.href && !term.href.startsWith('https://')) bad.push(term.href);
        }
    }
    expect(bad).toEqual([]);
});
