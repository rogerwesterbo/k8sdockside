// How the YAML editor is put together.
//
// Apart from the component because it is a list of decisions rather than a
// view: which extensions are on, what the colours are, and how the line the
// parser objected to gets marked. The component wires those to a tab and a
// store; this says what an editor of ours *is*.

import { defaultKeymap, history, historyKeymap, indentWithTab } from '@codemirror/commands';
import { yaml } from '@codemirror/lang-yaml';
import {
    HighlightStyle,
    bracketMatching,
    codeFolding,
    foldGutter,
    foldKeymap,
    indentOnInput,
    indentUnit,
    syntaxHighlighting,
} from '@codemirror/language';
import { highlightSelectionMatches, search, searchKeymap } from '@codemirror/search';
import { Compartment, EditorState, StateEffect, StateField } from '@codemirror/state';
import {
    Decoration,
    EditorView,
    highlightActiveLine,
    highlightActiveLineGutter,
    keymap,
    lineNumbers,
} from '@codemirror/view';
import { tags } from '@lezer/highlight';

/** Turns line numbering on and off without rebuilding the editor. */
export const numbering = new Compartment();

/** Says which line the YAML parser stopped at. 0 for none. */
export const setBadLine = StateEffect.define<number>();

/**
 * The line the parser objected to, held in the editor's own state.
 *
 * A field rather than a decoration passed in from outside, because the line has
 * to survive edits: CodeMirror maps its own state through every change, so the
 * mark stays on the line it was put on as text is typed above it.
 */
const badLine = StateField.define<number>({
    create: () => 0,
    update(value, tr) {
        for (const effect of tr.effects) {
            if (effect.is(setBadLine)) return effect.value;
        }
        return value;
    },
});

/** Draws the mark, recomputed whenever the line or the document changes. */
const badLineMark = EditorView.decorations.compute([badLine, 'doc'], (state) => {
    const n = state.field(badLine);
    if (n < 1 || n > state.doc.lines) return Decoration.none;
    return Decoration.set([Decoration.line({ class: 'cm-badLine' }).range(state.doc.line(n).from)]);
});

/**
 * The colours, in the app's own tokens rather than a palette of this file's
 * own, so the editor follows the theme with everything else.
 */
const highlight = HighlightStyle.define([
    { tag: [tags.propertyName, tags.definition(tags.propertyName)], color: 'var(--accent)' },
    { tag: [tags.string, tags.special(tags.string)], color: 'var(--ok)' },
    { tag: [tags.number, tags.bool, tags.null], color: 'var(--warn)' },
    { tag: tags.comment, color: 'var(--text-faint)', fontStyle: 'italic' },
    { tag: [tags.keyword, tags.atom], color: 'var(--warn)' },
    { tag: tags.meta, color: 'var(--text-dim)' },
]);

const theme = EditorView.theme({
    '&': {
        height: '100%',
        fontSize: '12px',
        color: 'var(--text)',
        backgroundColor: 'var(--bg-panel)',
    },
    '&.cm-focused': { outline: 'none' },
    '.cm-scroller': {
        fontFamily: 'var(--mono)',
        lineHeight: '1.6',
        overflow: 'auto',
    },
    '.cm-content': { padding: '6px 0', caretColor: 'var(--text)' },
    '.cm-cursor, .cm-dropCursor': { borderLeftColor: 'var(--text)' },
    '.cm-gutters': {
        backgroundColor: 'var(--bg-panel)',
        color: 'var(--text-faint)',
        border: 'none',
        borderRight: '1px solid var(--border-soft)',
    },
    '.cm-activeLineGutter': { backgroundColor: 'transparent', color: 'var(--text-dim)' },
    '.cm-activeLine': { backgroundColor: 'var(--bg-hover)' },
    '.cm-foldGutter span': { color: 'var(--text-faint)', cursor: 'pointer' },
    '.cm-foldGutter span:hover': { color: 'var(--text)' },
    // A folded run reads as one thing that can be opened, rather than as text.
    '.cm-foldPlaceholder': {
        backgroundColor: 'var(--bg-active)',
        border: '1px solid var(--border)',
        borderRadius: 'var(--radius-sm)',
        color: 'var(--text-dim)',
        padding: '0 6px',
        margin: '0 2px',
    },
    '&.cm-focused .cm-selectionBackground, .cm-selectionBackground, ::selection': {
        backgroundColor: 'var(--bg-active)',
    },
    '.cm-selectionMatch': { backgroundColor: 'var(--bg-active)' },
    // The line the parser stopped at, marked the way the old gutter marked it.
    '.cm-badLine': { backgroundColor: 'color-mix(in srgb, var(--error) 14%, transparent)' },
    '.cm-panels': {
        backgroundColor: 'var(--bg-raised)',
        color: 'var(--text)',
        borderBottom: '1px solid var(--border)',
    },
    '.cm-panel.cm-search input, .cm-panel.cm-search button, .cm-panel.cm-search label': {
        fontFamily: 'var(--font)',
        fontSize: '11.5px',
    },
    '.cm-panel.cm-search input': {
        backgroundColor: 'var(--bg)',
        color: 'var(--text)',
        border: '1px solid var(--border)',
        borderRadius: 'var(--radius-sm)',
        padding: '2px 6px',
    },
    '.cm-panel.cm-search button': {
        backgroundColor: 'var(--bg-raised)',
        color: 'var(--text)',
        border: '1px solid var(--border)',
        borderRadius: 'var(--radius-sm)',
        backgroundImage: 'none',
    },
    '.cm-searchMatch': { backgroundColor: 'color-mix(in srgb, var(--warn) 30%, transparent)' },
    '.cm-searchMatch.cm-searchMatch-selected': {
        backgroundColor: 'color-mix(in srgb, var(--warn) 60%, transparent)',
    },
});

/** What an editor of ours is made of, given what to do when Save is pressed. */
export function extensions(options: { numbers: boolean; label: string; onSave: () => void }) {
    return [
        // The label goes on the content, which is the element that carries the
        // textbox role -- a label on the box around it names nothing.
        EditorView.contentAttributes.of({ 'aria-label': options.label }),

        // Search is bound first so its own Escape closes the panel rather than
        // the editor letting go of focus.
        keymap.of(searchKeymap),
        search({ top: true }),
        highlightSelectionMatches(),

        numbering.of(options.numbers ? lineNumbers() : []),
        codeFolding(),
        foldGutter(),
        keymap.of(foldKeymap),

        yaml(),
        syntaxHighlighting(highlight, { fallback: true }),
        bracketMatching(),
        indentOnInput(),
        // Two spaces, which is what the documents this opens are written in.
        indentUnit.of('  '),
        EditorState.tabSize.of(2),

        history(),
        keymap.of(historyKeymap),
        highlightActiveLine(),
        highlightActiveLineGutter(),

        badLine,
        badLineMark,
        theme,

        keymap.of([
            {
                key: 'Mod-s',
                preventDefault: true,
                run: () => {
                    options.onSave();
                    return true;
                },
            },
            // Tab indents rather than moving focus: in a YAML editor that is
            // the whole job of the key.
            indentWithTab,
            {
                // The way out for anyone on a keyboard, since Tab is taken. It
                // stops here rather than reaching the window, where it would
                // close the describe panel instead.
                key: 'Escape',
                run: (view) => {
                    view.contentDOM.blur();
                    return true;
                },
            },
        ]),
        keymap.of(defaultKeymap),
    ];
}
