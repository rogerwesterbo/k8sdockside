// Inline marks for the documentation pages.
//
// Two marks and no more: **bold** and `code`. Enough for a sentence that
// names a button and a file, and little enough that a page in the source
// reads as prose. Anything else the pages need -- lists, code blocks, links
// -- is a block of its own in the content model, not a mark in a string.

/** One run of a sentence, drawn as text, as bold, or as code. */
export interface Run {
    kind: 'text' | 'bold' | 'code';
    text: string;
}

/**
 * Splits a sentence into runs. A mark that is opened and never closed is
 * left as written: a stray asterisk should not swallow the rest of the page.
 */
export function inline(text: string): Run[] {
    const runs: Run[] = [];
    let plain = '';
    let at = 0;

    const flush = (): void => {
        if (plain) runs.push({ kind: 'text', text: plain });
        plain = '';
    };

    while (at < text.length) {
        if (text.startsWith('**', at)) {
            const end = text.indexOf('**', at + 2);
            if (end > at + 2) {
                flush();
                runs.push({ kind: 'bold', text: text.slice(at + 2, end) });
                at = end + 2;
                continue;
            }
        } else if (text[at] === '`') {
            const end = text.indexOf('`', at + 1);
            if (end > at + 1) {
                flush();
                runs.push({ kind: 'code', text: text.slice(at + 1, end) });
                at = end + 1;
                continue;
            }
        }
        plain += text[at];
        at++;
    }
    flush();
    return runs;
}
