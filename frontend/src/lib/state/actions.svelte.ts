// Doing things to objects: deleting, scaling, restarting, cordoning, draining.
//
// The catalogue in ../actions.ts decides which buttons a kind gets; this runs
// what they do. Split that way because the two change for different reasons --
// one when a kind gains an action, the other when an action gains a step -- and
// because the catalogue is then a table anyone can read without a cluster.
//
// Like the editors store, nothing here knows about the workspace. An action
// needs the four fields that name an object; whoever pressed the button owns
// telling the user how it went.

import { Events } from '@wailsio/runtime';
import { ActionService } from '../../../bindings/github.com/rogerwesterbo/k8sdockside';
import type * as kube from '../../../bindings/github.com/rogerwesterbo/k8sdockside/internal/kube/models.js';
import { changes, type ObjectRef } from './changes.svelte';

/** What the bar knows about an object beyond its name. */
export interface ObjectState {
    scalable: boolean;
    replicas: number;
    cordoned: boolean;
    /** A pod's containers, as the same squares the table draws. */
    containers: kube.Pill[];
}

/** A pod a drain would not move, and why. */
export interface Refusal {
    pod: { namespace: string; name: string };
    reason: string;
}

/** A drain in flight, or the last one this node had. */
export interface DrainState {
    id: string;
    /** "cordoning", "planning", "evicting", "done" or "failed". */
    phase: string;
    evicted: number;
    total: number;
    refused: Refusal[];
    error: string;
    done: boolean;
}

const UNKNOWN: ObjectState = { scalable: false, replicas: 0, cordoned: false, containers: [] };

function key(ref: ObjectRef): string {
    return `${ref.contextId}#${ref.kind}#${ref.namespace}#${ref.name}`;
}

function message(err: unknown): Error {
    return err instanceof Error ? err : new Error(String(err));
}

/** A drain that has only just been asked for, before the first report lands. */
function starting(id: string): DrainState {
    return { id, phase: 'cordoning', evicted: 0, total: 0, refused: [], error: '', done: false };
}

class Actions {
    private states = $state<Record<string, ObjectState>>({});
    private drains = $state<Record<string, DrainState>>({});
    /**
     * Which node each drain ID belongs to. Reports are routed by the ID the
     * backend handed back rather than by the node name in the payload, so two
     * nodes draining at once cannot be confused for each other.
     */
    private draining = new Map<string, string>();

    constructor() {
        Events.On('node:drain', (event: { data: kube.DrainProgress }) => {
            const progress = event.data;
            const at = progress?.drainId ? this.draining.get(progress.drainId) : undefined;
            if (!at) return;

            this.drains[at] = {
                id: progress.drainId,
                phase: progress.phase,
                evicted: progress.evicted,
                total: progress.total,
                refused: progress.refused ?? [],
                error: progress.error,
                done: progress.done,
            };
            if (progress.done) this.draining.delete(progress.drainId);
        });
    }

    /** What is known about an object. Never null: an unread one reads as plain. */
    stateOf(ref: ObjectRef): ObjectState {
        return this.states[key(ref)] ?? UNKNOWN;
    }

    /**
     * Reads the object for what its buttons need to say.
     *
     * A failure is swallowed rather than raised: Edit and Delete need nothing
     * from this call, and a cluster that will not answer must still leave a
     * usable bar rather than an error where the buttons were.
     */
    async load(ref: ObjectRef): Promise<void> {
        try {
            const state = await ActionService.ObjectState(ref.contextId, ref.kind, ref.namespace, ref.name);
            this.states[key(ref)] = {
                scalable: state.scalable,
                replicas: state.replicas,
                cordoned: state.cordoned,
                // Null rather than empty is what Go sends for a kind with none.
                containers: state.containers ?? [],
            };
        } catch {
            // Deliberately silent -- see above.
        }
    }

    /** Drops what is held about an object, with the panel that showed it. */
    forget(ref: ObjectRef): void {
        const at = key(ref);
        delete this.states[at];
        const drain = this.drains[at];
        if (drain) this.draining.delete(drain.id);
        delete this.drains[at];
    }

    /**
     * Deletes the object.
     *
     * Named `remove` because `delete` is a reserved word, and the alternative
     * -- a method you can only call through a string index -- reads worse than
     * the rename.
     *
     * It does not signal a change, unlike every other action here: there is no
     * object left to re-read, and asking the describe panel to try would show
     * the user a 404 where their object used to be.
     */
    async remove(ref: ObjectRef): Promise<void> {
        try {
            await ActionService.Delete(ref.contextId, ref.kind, ref.namespace, ref.name);
        } catch (err) {
            throw message(err);
        }
    }

    async scale(ref: ObjectRef, replicas: number): Promise<void> {
        await this.run(ref, () =>
            ActionService.Scale(ref.contextId, ref.kind, ref.namespace, ref.name, replicas),
        );
    }

    async restart(ref: ObjectRef): Promise<void> {
        await this.run(ref, () => ActionService.Restart(ref.contextId, ref.kind, ref.namespace, ref.name));
    }

    async cordon(ref: ObjectRef, on: boolean): Promise<void> {
        await this.run(ref, () => ActionService.Cordon(ref.contextId, ref.name, on));
    }

    /** The drain on this node, or null if it has never had one. */
    drainOf(ref: ObjectRef): DrainState | null {
        return this.drains[key(ref)] ?? null;
    }

    /**
     * Starts moving everything off a node.
     *
     * It resolves once the drain is under way, not once it is finished: a drain
     * waits on disruption budgets and can take minutes, and its progress
     * arrives as events.
     */
    async drain(ref: ObjectRef): Promise<void> {
        let id: string;
        try {
            id = await ActionService.Drain(ref.contextId, ref.name);
        } catch (err) {
            throw message(err);
        }
        const at = key(ref);
        this.draining.set(id, at);
        // Only if nothing has already reported: the first event can arrive
        // before the call carrying the ID has come back, exactly as a
        // subscription's first snapshot can.
        this.drains[at] ??= starting(id);
    }

    /**
     * Calls off a drain in flight. The node stays cordoned -- it is half
     * emptied, and letting work back onto it is not what stopping meant.
     */
    cancelDrain(ref: ObjectRef): void {
        const drain = this.drains[key(ref)];
        if (!drain || drain.done) return;
        ActionService.CancelDrain(drain.id);
    }

    /**
     * Runs one action and, if it worked, says the object changed -- so the
     * describe panel beside the button re-reads rather than going on showing
     * what the cluster held a moment ago.
     */
    private async run(ref: ObjectRef, call: () => Promise<unknown>): Promise<void> {
        try {
            await call();
        } catch (err) {
            throw message(err);
        }
        changes.changed(ref);
        await this.load(ref);
    }
}

export const actions = new Actions();
