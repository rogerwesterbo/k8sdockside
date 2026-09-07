// Building the merge patch a bulk edit sends.
//
// Three ways in -- a label, an annotation, a field by path -- and one thing
// out: an RFC 7386 merge patch, where a value sets and null removes. Three
// rather than a single path field because a label key can hold dots
// (`app.kubernetes.io/name`) and a path is split on them; saying which it is
// keeps the key whole. Nothing here knows about a cluster: it is a table of
// what the form's fields mean, which is what makes it testable without one.

export type PatchTarget = 'label' | 'annotation' | 'field';
export type PatchMove = 'set' | 'remove';

export interface PatchSpec {
    target: PatchTarget;
    /** A label or annotation key, or a dotted path such as `spec.replicas`. */
    key: string;
    move: PatchMove;
    /** The value to set, as typed. Ignored by a removal. */
    value: string;
}

/**
 * What a typed field value means: JSON where it parses -- `2`, `true`,
 * `null`, `{"a":1}` -- and the text itself otherwise, so `web` needs no quotes
 * and `"2"` is how to ask for the string two.
 */
export function parseValue(text: string): unknown {
    const trimmed = text.trim();
    if (trimmed === '') return '';
    try {
        return JSON.parse(trimmed) as unknown;
    } catch {
        return text;
    }
}

/** The segments a spec addresses, or null when its key cannot be used. */
export function pathOf(spec: PatchSpec): string[] | null {
    const key = spec.key.trim();
    if (key === '') return null;
    switch (spec.target) {
        case 'label':
            return ['metadata', 'labels', key];
        case 'annotation':
            return ['metadata', 'annotations', key];
        case 'field': {
            const parts = key.split('.').map((part) => part.trim());
            return parts.every((part) => part !== '') ? parts : null;
        }
    }
}

/**
 * The merge patch for one spec, or null while it cannot be built.
 *
 * A label or annotation is always text, whatever was typed: `2` on a label is
 * the string two, since that is all a label can hold. A field takes the typed
 * value for what it parses as.
 */
export function buildMergePatch(spec: PatchSpec): Record<string, unknown> | null {
    const path = pathOf(spec);
    if (!path) return null;
    let leaf: unknown = null;
    if (spec.move === 'set') {
        leaf = spec.target === 'field' ? parseValue(spec.value) : spec.value;
    }
    return path.reduceRight<unknown>((inner, part) => ({ [part]: inner }), leaf) as Record<string, unknown>;
}
