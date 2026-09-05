import { beforeEach, describe, expect, test, vi } from 'vitest';
import { DEFAULT_THEME_ID, applyTheme, pickTheme, restoreTheme, type Theme } from './apply';

function theme(id: string, extra: Partial<Theme> = {}): Theme {
    return {
        id,
        name: id,
        tagline: '',
        base: 'dark',
        author: '',
        tokens: {},
        resolved: { bg: '#101010', text: '#f0f0f0', accent: '#4a86ff' },
        origin: 'builtin',
        pack: '',
        warnings: [],
        ...extra,
    };
}

beforeEach(() => {
    const root = document.documentElement;
    root.removeAttribute('style');
    delete root.dataset.theme;
    delete root.dataset.themeBase;
    localStorage.clear();
});

describe('applyTheme', () => {
    test('writes every token onto the document root', () => {
        applyTheme(theme('nord'));

        const style = document.documentElement.style;
        expect(style.getPropertyValue('--bg')).toBe('#101010');
        expect(style.getPropertyValue('--text')).toBe('#f0f0f0');
        expect(style.getPropertyValue('--accent')).toBe('#4a86ff');
    });

    test('stamps the id and the base, which is what color-scheme keys off', () => {
        applyTheme(theme('morning', { base: 'light' }));

        expect(document.documentElement.dataset.theme).toBe('morning');
        expect(document.documentElement.dataset.themeBase).toBe('light');
    });

    // Anything that is not the string 'light' is dark: a base is one of two
    // things, and a theme file that says something else has already been
    // normalised on the way in.
    test('an unrecognised base is treated as dark', () => {
        applyTheme(theme('odd', { base: 'sepia' }));
        expect(document.documentElement.dataset.themeBase).toBe('dark');
    });

    test('a failing cache does not stop the theme being applied', () => {
        const setItem = vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
            throw new Error('quota exceeded');
        });

        expect(() => applyTheme(theme('nord'))).not.toThrow();
        expect(document.documentElement.dataset.theme).toBe('nord');

        setItem.mockRestore();
    });
});

// The cache exists so that a user on a light theme does not get a flash of dark
// at every launch, which is otherwise unavoidable: the theme lives in a file the
// Go side has to be asked for, and the first frames are drawn before it answers.
describe('restoreTheme', () => {
    test('repaints from the last theme applied', () => {
        applyTheme(theme('driftwood', { base: 'light', resolved: { bg: '#f3ece1' } }));

        document.documentElement.removeAttribute('style');
        delete document.documentElement.dataset.theme;
        restoreTheme();

        expect(document.documentElement.style.getPropertyValue('--bg')).toBe('#f3ece1');
        expect(document.documentElement.dataset.theme).toBe('driftwood');
        expect(document.documentElement.dataset.themeBase).toBe('light');
    });

    test('does nothing when there is no cache', () => {
        restoreTheme();
        expect(document.documentElement.dataset.theme).toBeUndefined();
    });

    // Everything read here is untrusted -- it is whatever is in storage, which
    // may have been written by an older version or edited by hand -- so it is
    // discarded rather than half-applied.
    test.each([
        ['not JSON at all', 'nonsense{'],
        ['not an object', '"a string"'],
        ['no id', JSON.stringify({ resolved: { bg: '#fff' } })],
        ['no tokens', JSON.stringify({ id: 'x' })],
    ])('discards a cache that is %s', (_label, raw) => {
        localStorage.setItem('k8sdockside.theme', raw);

        restoreTheme();

        expect(document.documentElement.dataset.theme).toBeUndefined();
        expect(document.documentElement.getAttribute('style')).toBeNull();
    });

    // The cache is the one path where a value reaches a style declaration
    // without having been through the Go validator, so it does its own checking.
    test('skips token names and values it does not like the look of', () => {
        localStorage.setItem(
            'k8sdockside.theme',
            JSON.stringify({
                id: 'nasty',
                base: 'dark',
                resolved: {
                    bg: '#101010',
                    'bg: red; --evil': '#fff',
                    'Bad-Name': '#fff',
                    text: { not: 'a string' },
                },
            }),
        );

        restoreTheme();

        const style = document.documentElement.style;
        expect(style.getPropertyValue('--bg')).toBe('#101010');
        expect(style.getPropertyValue('--text')).toBe('');
        expect(style.getPropertyValue('--Bad-Name')).toBe('');
        expect(document.documentElement.getAttribute('style')).not.toContain('evil');
    });
});

describe('pickTheme', () => {
    const themes = [theme(DEFAULT_THEME_ID), theme('nord')];

    test('finds the theme by id', () => {
        expect(pickTheme(themes, 'nord')?.id).toBe('nord');
    });

    test('falls back to the default when the id names nothing', () => {
        expect(pickTheme(themes, 'gone')?.id).toBe(DEFAULT_THEME_ID);
    });

    // A user theme can replace the default by taking its id, and a folder full
    // of themes can be the only thing installed; neither should leave the app
    // with no colours at all.
    test('falls back to whatever is first when even the default is absent', () => {
        expect(pickTheme([theme('only-one')], 'gone')?.id).toBe('only-one');
    });

    test('is null with nothing to pick from', () => {
        expect(pickTheme([], 'anything')).toBeNull();
    });
});
