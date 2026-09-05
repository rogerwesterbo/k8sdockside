// Putting a theme on the screen.
//
// A theme reaches the app as a flat map of token name to colour, already
// resolved against its base by the Go side -- see internal/themes. All that is
// left here is writing it onto the document root, which is where every
// component's `var(--token)` resolves against, and remembering it for next
// time.
//
// Nothing in this file knows what the themes are. That is deliberate: the
// built-in palettes and a stranger's downloaded one arrive in the same shape
// through the same path, so there is no code here that only the built-ins take.

/** A theme as the app works with it, adopted from the Go binding. */
export interface Theme {
    id: string;
    name: string;
    tagline: string;
    /** `dark` or `light`. Decides `color-scheme`, and what the theme inherits. */
    base: string;
    author: string;
    /** The theme's own tokens, before inheritance. Shown, not applied. */
    tokens: Record<string, string>;
    /** Every token, with the theme's gaps filled in. This is what gets applied. */
    resolved: Record<string, string>;
    /** `builtin`, or the path of the file it was read from. */
    origin: string;
    /** The collection it arrived in, empty for a theme that came on its own. */
    pack: string;
    /** Readability problems, shown against the theme rather than hiding it. */
    warnings: string[];
}

/** The whole gallery, plus where user themes come from and what failed. */
export interface ThemeCatalogue {
    themes: Theme[];
    /** The folder user themes are read from by default. */
    dir: string;
    /** Extra folders the user has added. */
    folders: string[];
    problems: { path: string; message: string }[];
}

/** One themeable colour, with what it is for. Documented in the settings view. */
export interface ThemeToken {
    name: string;
    help: string;
}

/**
 * The theme a fresh install wears, and what an id nothing answers to falls back
 * to. It matches themes.DefaultID on the Go side; the two are checked against
 * each other by a test rather than shared, because a constant that crosses the
 * binding would have to be a service call to read.
 */
export const DEFAULT_THEME_ID = 'k8sdockside-dark';

/**
 * Where the last applied theme is kept so the next launch can paint with it
 * before the Go side has answered. See restoreTheme.
 */
const CACHE_KEY = 'k8sdockside.theme';

/**
 * Writes a token set onto the document root.
 *
 * Tokens are set as custom properties rather than swapped stylesheets because
 * that is already how every component gets its colours: there is no rule
 * anywhere that names a theme, so adding one costs nothing but the file.
 *
 * `data-theme` is set alongside them for two reasons: it is what makes the
 * current theme visible when inspecting the DOM, and it is the seam a future
 * rule that genuinely has to know the theme could hang off. `data-theme-base`
 * carries the light/dark answer, which is what `color-scheme` needs -- and
 * `color-scheme` is what decides the colour of the things CSS cannot reach:
 * native scrollbars, the text caret, and form controls we have not styled.
 */
export function applyTheme(theme: Theme): void {
    if (typeof document === 'undefined') return;

    const root = document.documentElement;
    for (const [name, value] of Object.entries(theme.resolved ?? {})) {
        root.style.setProperty(`--${name}`, value);
    }
    root.dataset.theme = theme.id;
    root.dataset.themeBase = theme.base === 'light' ? 'light' : 'dark';

    cacheTheme(theme);
}

/**
 * Remembers the applied theme so the next launch can use it immediately.
 *
 * Without this the app spends its first frames in whatever style.css says --
 * which is the dark palette -- and a user on a light theme gets a flash of dark
 * every single launch, because the themes cannot arrive until the service call
 * does. The cache is not the source of truth and is never trusted for anything
 * but painting: the moment the real catalogue arrives it is applied over the
 * top, and if the theme has since been deleted or changed, that is what wins.
 */
function cacheTheme(theme: Theme): void {
    try {
        localStorage.setItem(
            CACHE_KEY,
            JSON.stringify({ id: theme.id, base: theme.base, resolved: theme.resolved }),
        );
    } catch {
        // Storage can be unavailable or full. A flash on next launch is not
        // worth failing a theme change over.
    }
}

/**
 * Paints the last theme used, before anything has loaded. Called from the entry
 * point rather than a component, because by the time a component mounts the
 * first frame has already been drawn in the wrong colours.
 *
 * Everything it reads is treated as untrusted -- it is whatever is in storage,
 * which may be from an older version of the app -- so a malformed cache is
 * discarded rather than half-applied.
 */
export function restoreTheme(): void {
    if (typeof document === 'undefined' || typeof localStorage === 'undefined') return;

    let cached: unknown;
    try {
        const raw = localStorage.getItem(CACHE_KEY);
        if (!raw) return;
        cached = JSON.parse(raw);
    } catch {
        return;
    }

    if (typeof cached !== 'object' || cached === null) return;
    const { id, base, resolved } = cached as Record<string, unknown>;
    if (typeof id !== 'string' || typeof resolved !== 'object' || resolved === null) return;

    const root = document.documentElement;
    for (const [name, value] of Object.entries(resolved as Record<string, unknown>)) {
        // Only tokens, and only strings: this is the one place a value reaches
        // a style declaration without having been through the Go validator.
        if (typeof value !== 'string' || !/^[a-z][a-z0-9-]*$/.test(name)) continue;
        root.style.setProperty(`--${name}`, value);
    }
    root.dataset.theme = id;
    root.dataset.themeBase = base === 'light' ? 'light' : 'dark';
}

/**
 * The theme with the given id, falling back to the default and then to whatever
 * is first. The fallback is not a formality: a theme can be removed from under
 * a settings file that still names it, by deleting the file, dropping the
 * folder, or opening the same config on another machine.
 */
export function pickTheme(themes: Theme[], id: string): Theme | null {
    return (
        themes.find((t) => t.id === id) ??
        themes.find((t) => t.id === DEFAULT_THEME_ID) ??
        themes[0] ??
        null
    );
}
