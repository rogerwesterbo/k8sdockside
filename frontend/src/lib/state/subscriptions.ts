// The live side of a resource tab.
//
// A tab does not fetch its rows, it subscribes: the backend opens a watch
// against the cluster and pushes the whole current table whenever anything
// changes. One listener serves every tab -- Wails delivers each event to every
// registered handler, so routing by subscription ID here is cheaper than making
// each tab filter the traffic of all the others.

import { Events } from '@wailsio/runtime';
import { ResourceService } from '../../../bindings/github.com/roger/k8sdockside';
import { adoptTable, type Table } from './adopt';

type Listener = (table: Table) => void;

const listeners = new Map<string, Listener>();

// A snapshot can arrive before Subscribe's promise has resolved: the backend
// starts pushing the moment the watch is open, which may be before the reply
// carrying its ID has crossed back. Holding the most recent snapshot per
// subscription means the first payload -- the one that clears the tab's loading
// state -- is never the one that gets dropped.
const pending = new Map<string, Table>();

Events.On('resource:snapshot', (event) => {
    const snapshot = event.data;
    if (!snapshot?.subscriptionId) return;

    const table = adoptTable(snapshot.table);
    const listener = listeners.get(snapshot.subscriptionId);
    if (listener) {
        listener(table);
    } else {
        pending.set(snapshot.subscriptionId, table);
    }
});

/** One tab's live view. Closing it stops the watch if no other tab shares it. */
export interface Subscription {
    setNamespace(namespace: string): void;
    close(): void;
}

/**
 * Opens a live view of one resource kind. Rows arrive through onTable, starting
 * with the cluster's current contents once the watch has synced; a cluster that
 * cannot be reached reports through onError instead.
 */
export function subscribe(
    contextId: string,
    kind: string,
    namespace: string,
    onTable: Listener,
    onError: (message: string) => void,
): Subscription {
    let id: string | null = null;
    let closed = false;
    // Where the namespace filter has been moved to while we were still waiting
    // for the subscription to open, so an impatient click is not lost.
    let wanted = namespace;

    ResourceService.Subscribe(contextId, kind, namespace)
        .then((subscriptionId) => {
            if (closed) {
                // A snapshot may already have been buffered under this ID by
                // the time we learn the tab has gone.
                pending.delete(subscriptionId);
                void ResourceService.Unsubscribe(subscriptionId);
                return;
            }
            id = subscriptionId;
            listeners.set(subscriptionId, onTable);

            const buffered = pending.get(subscriptionId);
            if (buffered) {
                pending.delete(subscriptionId);
                onTable(buffered);
            }
            if (wanted !== namespace) {
                void ResourceService.SetNamespace(subscriptionId, wanted);
            }
        })
        .catch((err: unknown) => {
            if (!closed) onError(err instanceof Error ? err.message : String(err));
        });

    return {
        setNamespace(next: string): void {
            wanted = next;
            if (id) void ResourceService.SetNamespace(id, next);
        },
        close(): void {
            closed = true;
            if (id === null) return;
            listeners.delete(id);
            pending.delete(id);
            void ResourceService.Unsubscribe(id);
            id = null;
        },
    };
}
