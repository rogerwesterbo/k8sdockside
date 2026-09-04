// The colour a kubeconfig context is tagged with. It tints the context in the
// sidebar and every tab opened against it, so at a glance you can tell which
// cluster a tab is talking to -- which is the whole point of colouring them.

export interface ContextColor {
    name: string;
    value: string;
}

/**
 * The palette offered in the context settings panel. Every colour is dark
 * enough to carry white text and distinct enough from its neighbours to be
 * told apart in a row of tabs.
 */
export const CONTEXT_COLORS: ContextColor[] = [
    { name: 'Slate', value: '#4b5b6e' },
    { name: 'Blue', value: '#2f6fb3' },
    { name: 'Indigo', value: '#4c50b8' },
    { name: 'Violet', value: '#7a4bbd' },
    { name: 'Magenta', value: '#a83a8f' },
    { name: 'Crimson', value: '#b8384b' },
    { name: 'Rust', value: '#b3552a' },
    { name: 'Amber', value: '#a8802a' },
    { name: 'Olive', value: '#6f8a2e' },
    { name: 'Emerald', value: '#2f8f5b' },
    { name: 'Teal', value: '#2b8a86' },
    { name: 'Cyan', value: '#2a7f9e' },
];

/** A stable non-cryptographic hash, used to pick defaults from a string key. */
export function hashString(value: string): number {
    let h = 2166136261;
    for (let i = 0; i < value.length; i++) {
        h ^= value.charCodeAt(i);
        h = Math.imul(h, 16777619);
    }
    return h >>> 0;
}

/**
 * The colour a context gets before the user picks one. It is derived from the
 * context id so that a freshly synced kubeconfig already has distinguishable
 * clusters, and so the colour never moves around between runs.
 */
export function defaultColorFor(contextId: string): string {
    return CONTEXT_COLORS[hashString(contextId) % CONTEXT_COLORS.length].value;
}

interface RGB {
    r: number;
    g: number;
    b: number;
}

function parseHex(hex: string): RGB {
    const clean = hex.replace('#', '').trim();
    const full = clean.length === 3 ? clean.split('').map((c) => c + c).join('') : clean;
    const n = Number.parseInt(full, 16);
    if (full.length !== 6 || Number.isNaN(n)) {
        return { r: 75, g: 91, b: 110 }; // fall back to Slate rather than rendering nothing
    }
    return { r: (n >> 16) & 255, g: (n >> 8) & 255, b: n & 255 };
}

/** Relative luminance, used to decide whether text on a colour should be light or dark. */
function luminance({ r, g, b }: RGB): number {
    const channel = (c: number) => {
        const v = c / 255;
        return v <= 0.04045 ? v / 12.92 : Math.pow((v + 0.055) / 1.055, 2.4);
    };
    return 0.2126 * channel(r) + 0.7152 * channel(g) + 0.0722 * channel(b);
}

/** The text colour that stays readable on the given background. */
export function textOn(hex: string): string {
    return luminance(parseHex(hex)) > 0.45 ? '#10151c' : '#ffffff';
}

/** `hex` as an rgba() string, for tints and borders derived from a context colour. */
export function alpha(hex: string, a: number): string {
    const { r, g, b } = parseHex(hex);
    return `rgba(${r}, ${g}, ${b}, ${a})`;
}

/** True if the string is a colour we can render, used to validate typed input. */
export function isValidColor(hex: string): boolean {
    return /^#([0-9a-f]{3}|[0-9a-f]{6})$/i.test(hex.trim());
}
