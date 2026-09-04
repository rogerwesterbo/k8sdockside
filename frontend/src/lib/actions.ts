// What can be done to an object, by kind.
//
// One table rather than a chain of conditions in the panel, so that the answer
// to "what can I do to a StatefulSet" is in one place and adding a kind is a
// line rather than a branch. Nothing here calls a cluster: it decides which
// buttons are drawn, and the store beside it does the work.

import { DASHBOARD, HELM_RELEASES, SETTINGS } from './catalogue';

export type ActionId = 'edit' | 'scale' | 'restart' | 'cordon' | 'drain' | 'delete';

/**
 * How choosing an action behaves.
 *
 * `immediate` runs at once, `confirm` replaces the bar with a question naming
 * the object, and `number` asks for a replica count first.
 */
export type ActionForm = 'immediate' | 'confirm' | 'number';

export interface Action {
    id: ActionId;
    label: string;
    icon: string;
    form: ActionForm;
    /** Marks the action that cannot be undone, so the bar can colour it apart. */
    tone?: 'danger';
}

const EDIT: Action = { id: 'edit', label: 'Edit', icon: 'edit', form: 'immediate' };
const SCALE: Action = { id: 'scale', label: 'Scale', icon: 'scale', form: 'number' };
const RESTART: Action = { id: 'restart', label: 'Restart', icon: 'repeat', form: 'immediate' };
/** Its label follows the node: a cordoned one offers to be uncordoned instead. */
const CORDON: Action = { id: 'cordon', label: 'Cordon', icon: 'shield', form: 'immediate' };
// Draining moves every workload off a node. Routine, and never a single click.
const DRAIN: Action = { id: 'drain', label: 'Drain', icon: 'server', form: 'confirm' };
const DELETE: Action = { id: 'delete', label: 'Delete', icon: 'trash', form: 'confirm', tone: 'danger' };

/** The kinds that carry a replica count, which is what Scale sets. */
const SCALABLE = ['deployments', 'statefulsets', 'replicasets', 'replicationcontrollers'];

/**
 * The kinds that own a pod template, which is what Restart stamps.
 *
 * ReplicaSets are deliberately absent although they have a template: the
 * Deployment above one owns it, and a restart stamped here would be undone by
 * the next reconcile.
 */
const ROLLABLE = ['deployments', 'statefulsets', 'daemonsets'];

/**
 * The views that are not objects at all. The dashboard and settings are places
 * in the app; a Helm release is a Secret the backend decodes, so deleting "it"
 * would not mean what it appears to.
 */
const NOT_AN_OBJECT = [DASHBOARD, SETTINGS, HELM_RELEASES];

/** The actions offered for one kind, in the order the bar draws them. */
export function actionsFor(kind: string): Action[] {
    if (NOT_AN_OBJECT.includes(kind)) return [];

    const out: Action[] = [EDIT];
    if (SCALABLE.includes(kind)) out.push(SCALE);
    if (ROLLABLE.includes(kind)) out.push(RESTART);
    if (kind === 'nodes') out.push(CORDON, DRAIN);
    out.push(DELETE);
    return out;
}
