import { describe, expect, test } from 'vitest';
import { classify } from './errors';

// The messages below are real client-go output. They are the whole point of
// this module: what a cluster failure looks like on the wire is not what a
// person needs to read, and the mapping between them is worth pinning down.

describe('classify', () => {
    test('reads a refused connection as a cluster that is not running', () => {
        const { headline, hint } = classify(
            'looking up namespaces: Get "https://localhost:6443/api?timeout=32s": ' +
                'dial tcp [::1]:6443: connect: connection refused',
        );

        expect(headline).toBe('Cannot reach the API server');
        expect(hint).toContain('localhost:6443');
    });

    test('names the host from the request URL rather than the dialled address', () => {
        // The dialled address is often an IP or [::1]; the URL is what the user
        // put in their kubeconfig and will recognise.
        const { hint } = classify(
            'Get "https://api.prod.example.com:6443/version": dial tcp 10.0.0.1:6443: connect: connection refused',
        );

        expect(hint).toContain('api.prod.example.com:6443');
        expect(hint).not.toContain('10.0.0.1');
    });

    test('reads a timeout as a cluster that is reachable but silent', () => {
        const { headline } = classify(
            'Get "https://10.204.146.226:6443/api?timeout=32s": dial tcp 10.204.146.226:6443: i/o timeout',
        );

        expect(headline).toBe('The cluster did not answer in time');
    });

    test('treats a deadline exceeded as a timeout too', () => {
        expect(classify('context deadline exceeded').headline).toBe('The cluster did not answer in time');
    });

    test('reads an unresolvable host as a name problem, not a reachability one', () => {
        const { headline } = classify(
            'Get "https://gone.example.com/version": dial tcp: lookup gone.example.com: no such host',
        );

        expect(headline).toBe('Cannot find the server');
    });

    test('reads a certificate failure as a trust problem', () => {
        const { headline } = classify(
            'Get "https://10.0.0.1:6443/version": x509: certificate signed by unknown authority',
        );

        expect(headline).toBe('The server certificate was not trusted');
    });

    test('reads a 401 as rejected credentials', () => {
        expect(classify('Unauthorized').headline).toBe('Not authorised');
    });

    test('reads a 403 as a permissions problem, not an authentication one', () => {
        const { headline } = classify(
            'pods is forbidden: User "system:anonymous" cannot list resource "pods" in API group ""',
        );

        expect(headline).toBe('Access denied');
    });

    test('reads an exec plugin failure as a credential helper problem', () => {
        const { headline } = classify(
            'getting credentials: exec: executable aws not found: it looks like you are trying to use ' +
                'a client-go credential plugin that is not installed',
        );

        expect(headline).toBe('The credential helper failed');
    });

    test('reads a missing context as a kubeconfig that changed underneath us', () => {
        const { headline } = classify(
            'unknown context "/home/u/.kube/config::admin@old" -- it may have been removed from the kubeconfig',
        );

        expect(headline).toBe('This context is no longer in the kubeconfig');
    });

    test('falls back to a generic headline rather than guessing', () => {
        const { headline, hint } = classify('the server could not find the requested resource');

        expect(headline).toBe('Something went wrong talking to this cluster');
        expect(hint).not.toBe('');
    });

    test('survives an empty message', () => {
        expect(classify('').headline).toBe('Something went wrong talking to this cluster');
    });

    test('matches case-insensitively, because client-go is inconsistent about it', () => {
        expect(classify('Connection Refused').headline).toBe('Cannot reach the API server');
    });
});
