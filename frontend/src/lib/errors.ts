// Turning client-go's wire errors into something a person can act on.
//
// Every failure to reach a cluster arrives as one string, assembled from
// whatever layer gave up first -- the resolver, the dialler, the TLS stack, the
// API server. Those strings are precise and worth keeping, but the first thing
// a reader needs is which *kind* of problem it is, because that is what decides
// what they do next: start the cluster, join the VPN, refresh a token, or fix a
// certificate. The raw text stays on screen underneath; this only leads with a
// plain reading of it.

/** A plain-language reading of one error, shown above the raw message. */
export interface Explanation {
    headline: string;
    hint: string;
}

const GENERIC: Explanation = {
    headline: 'Something went wrong talking to this cluster',
    hint: 'The full error from the cluster is below.',
};

/**
 * The host as written in the kubeconfig, pulled out of the request URL.
 *
 * Errors carry two addresses -- the URL that was requested and the address it
 * dialled -- and only the first is one the user will recognise. `[::1]:6443` or
 * a load balancer IP tells them nothing about which cluster this is.
 */
function hostFrom(message: string): string {
    return /https?:\/\/([^/"\s]+)/i.exec(message)?.[1] ?? '';
}

/**
 * A rule is a set of substrings to look for, in lower case, and how to explain
 * a match. The order is the priority: the first match wins, so a specific
 * reading has to come before any general one that would also match it.
 */
const RULES: { match: string[]; explain: (host: string) => Explanation }[] = [
    {
        match: ['connection refused'],
        explain: (host) => ({
            headline: 'Cannot reach the API server',
            hint: host
                ? `Nothing is listening at ${host}. Is the cluster running?`
                : 'Nothing is listening at that address. Is the cluster running?',
        }),
    },
    {
        match: ['no such host', 'server misbehaving'],
        explain: (host) => ({
            headline: 'Cannot find the server',
            hint: host
                ? `The name ${host} does not resolve. Check the server address in this kubeconfig.`
                : 'The server name in this kubeconfig does not resolve.',
        }),
    },
    {
        match: ['i/o timeout', 'context deadline exceeded', 'client.timeout', 'timeout exceeded'],
        explain: (host) => ({
            headline: 'The cluster did not answer in time',
            hint: host
                ? `No reply from ${host}. It may be unreachable from this network, or behind a VPN you are not on.`
                : 'No reply from the cluster. It may be behind a VPN you are not on.',
        }),
    },
    {
        match: ['x509', 'certificate', 'tls:'],
        explain: (host) => ({
            headline: 'The server certificate was not trusted',
            hint: host
                ? `The certificate ${host} presented is not signed by an authority this kubeconfig trusts, or it has expired.`
                : 'The server certificate is not signed by an authority this kubeconfig trusts, or it has expired.',
        }),
    },
    {
        match: ['getting credentials', 'credential plugin', 'exec plugin', 'exec: '],
        explain: () => ({
            headline: 'The credential helper failed',
            hint: 'This kubeconfig runs an external command to obtain credentials, and that command did not succeed.',
        }),
    },
    {
        match: ['unauthorized', 'invalid bearer token', 'asked for the client to provide credentials'],
        explain: () => ({
            headline: 'Not authorised',
            hint: 'The cluster rejected the credentials in this kubeconfig. A token or certificate may have expired.',
        }),
    },
    {
        match: ['forbidden', 'cannot list resource', 'is not allowed'],
        explain: () => ({
            headline: 'Access denied',
            hint: 'The credentials worked, but this user is not permitted to read that. Check the RBAC bindings.',
        }),
    },
    {
        match: ['unknown context'],
        explain: () => ({
            headline: 'This context is no longer in the kubeconfig',
            hint: 'It was removed or renamed on disk. Sync the sidebar to pick up the current contexts.',
        }),
    },
    {
        match: ['connection reset', 'broken pipe'],
        explain: () => ({
            headline: 'The connection to the cluster dropped',
            hint: 'The cluster closed the connection mid-request. It may be restarting, or a proxy in between gave up.',
        }),
    },
];

/**
 * classify reads one error message and returns how to lead with it. An
 * unrecognised message is not guessed at: it gets a generic headline, and the
 * raw text underneath does the work.
 */
export function classify(message: string): Explanation {
    const haystack = message.toLowerCase();

    for (const rule of RULES) {
        if (rule.match.some((needle) => haystack.includes(needle))) {
            return rule.explain(hostFrom(message));
        }
    }
    return GENERIC;
}
