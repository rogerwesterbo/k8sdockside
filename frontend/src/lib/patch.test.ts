import { describe, expect, test } from 'vitest';
import { buildMergePatch, parseValue } from './patch';

describe('buildMergePatch', () => {
    test('a label is set under metadata.labels, as text whatever was typed', () => {
        expect(buildMergePatch({ target: 'label', move: 'set', key: 'team', value: '2' })).toEqual({
            metadata: { labels: { team: '2' } },
        });
    });

    test('a removal sends null, which is how a merge patch deletes', () => {
        expect(buildMergePatch({ target: 'label', move: 'remove', key: 'team', value: 'ignored' })).toEqual({
            metadata: { labels: { team: null } },
        });
    });

    test('a label key keeps its dots and slash whole', () => {
        expect(
            buildMergePatch({ target: 'label', move: 'set', key: 'app.kubernetes.io/name', value: 'web' }),
        ).toEqual({ metadata: { labels: { 'app.kubernetes.io/name': 'web' } } });
    });

    test('an annotation goes under metadata.annotations', () => {
        expect(buildMergePatch({ target: 'annotation', move: 'set', key: 'example.com/owner', value: 'me' })).toEqual({
            metadata: { annotations: { 'example.com/owner': 'me' } },
        });
    });

    test('a field path is split on dots and takes the value for what it parses as', () => {
        expect(buildMergePatch({ target: 'field', move: 'set', key: 'spec.replicas', value: '2' })).toEqual({
            spec: { replicas: 2 },
        });
        expect(buildMergePatch({ target: 'field', move: 'set', key: 'spec.suspend', value: 'true' })).toEqual({
            spec: { suspend: true },
        });
        expect(buildMergePatch({ target: 'field', move: 'set', key: 'spec.storageClassName', value: 'fast' })).toEqual({
            spec: { storageClassName: 'fast' },
        });
        expect(buildMergePatch({ target: 'field', move: 'set', key: 'spec.tag', value: '"2"' })).toEqual({
            spec: { tag: '2' },
        });
    });

    test('a field removal reaches down the path', () => {
        expect(buildMergePatch({ target: 'field', move: 'remove', key: 'spec.template.spec.nodeSelector', value: '' })).toEqual({
            spec: { template: { spec: { nodeSelector: null } } },
        });
    });

    test('nothing is built without a key, or with an empty path segment', () => {
        expect(buildMergePatch({ target: 'label', move: 'set', key: '', value: 'x' })).toBeNull();
        expect(buildMergePatch({ target: 'label', move: 'set', key: '   ', value: 'x' })).toBeNull();
        expect(buildMergePatch({ target: 'field', move: 'set', key: 'spec..replicas', value: '1' })).toBeNull();
        expect(buildMergePatch({ target: 'field', move: 'set', key: 'spec.', value: '1' })).toBeNull();
    });
});

describe('parseValue', () => {
    test('JSON is taken as JSON and anything else as text', () => {
        expect(parseValue('2')).toBe(2);
        expect(parseValue(' true ')).toBe(true);
        expect(parseValue('null')).toBeNull();
        expect(parseValue('{"a":1}')).toEqual({ a: 1 });
        expect(parseValue('web')).toBe('web');
        expect(parseValue('v1.2.3')).toBe('v1.2.3');
        expect(parseValue('')).toBe('');
    });
});
