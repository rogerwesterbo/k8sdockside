import { expect, test } from 'vitest';
import { inline } from './inline';

// The documentation pages are written as text with two marks -- **bold** and
// `code` -- and nothing else, so a page can be read in the source as easily
// as on screen. This turns one string into the runs a template draws.

test('plain text is one run', () => {
    expect(inline('Pods run containers.')).toEqual([{ kind: 'text', text: 'Pods run containers.' }]);
});

test('bold and code become their own runs, in order', () => {
    expect(inline('Open **Settings** and press `Reload`.')).toEqual([
        { kind: 'text', text: 'Open ' },
        { kind: 'bold', text: 'Settings' },
        { kind: 'text', text: ' and press ' },
        { kind: 'code', text: 'Reload' },
        { kind: 'text', text: '.' },
    ]);
});

test('an unclosed mark is left as written rather than swallowing the rest', () => {
    expect(inline('a ** b')).toEqual([{ kind: 'text', text: 'a ** b' }]);
    expect(inline('a ` b')).toEqual([{ kind: 'text', text: 'a ` b' }]);
});

test('marks inside code are not interpreted', () => {
    expect(inline('`**not bold**`')).toEqual([{ kind: 'code', text: '**not bold**' }]);
});
