// What can be done to an object, by kind.
//
// One table rather than a chain of conditions in the panel, so that the answer
// to "what can I do to a StatefulSet" is in one place and adding a kind is a
// line rather than a branch. Nothing here calls a cluster: it decides which
// buttons are drawn, and the store beside it does the work.

import { DASHBOARD, HELM_RELEASES, PORT_FORWARDS, SETTINGS } from './catalogue';

export type ActionId =
    | 'edit'
    | 'logs'
    | 'shell'
    | 'forward'
    | 'scale'
    | 'restart'
    | 'cordon'
    | 'drain'
    | 'delete'
    | 'values'
    | 'rollback'
    | 'uninstall';

/**
 * How choosing an action behaves.
 *
 * `immediate` runs at once, `confirm` replaces the bar with a question naming
 * the object, and `number` asks for a replica count first.
 */
export type ActionForm = 'immediate' | 'confirm' | 'number' | 'ports' | 'revision';

export interface Action {
    id: ActionId;
    label: string;
    icon: string;
    form: ActionForm;
    /** Marks the action that cannot be undone, so the bar can colour it apart. */
    tone?: 'danger';
    /**
     * Marks an action that runs helm rather than talking to the API server.
     *
     * The bar disables these, with the reason, on a machine where helm was not
     * found. Reading a release needs nothing installed -- the record is a
     * Secret the app decodes -- but changing one is Helm's own operation. See
     * internal/helmcli.
     */
    needsHelm?: boolean;
}

const EDIT: Action = { id: 'edit', label: 'Edit', icon: 'edit', form: 'immediate' };
const LOGS: Action = { id: 'logs', label: 'Logs', icon: 'rows', form: 'immediate' };
/**
 * A terminal in the object. Immediate: which shell, and which of the settings'
 * terminals opens it, are answered from the preferences rather than asked here
 * -- a shell you have to fill in a form for is not a shell you would use.
 */
const SHELL: Action = { id: 'shell', label: 'Shell', icon: 'terminal', form: 'immediate' };
/**
 * A local port into the object. It asks first, because there are three answers
 * it cannot guess: which port, on which local port, and whether to open a
 * browser on it.
 */
const FORWARD: Action = { id: 'forward', label: 'Forward', icon: 'forward', form: 'ports' };
const SCALE: Action = { id: 'scale', label: 'Scale', icon: 'scale', form: 'number' };
const RESTART: Action = { id: 'restart', label: 'Restart', icon: 'repeat', form: 'immediate' };
/** Its label follows the node: a cordoned one offers to be uncordoned instead. */
const CORDON: Action = { id: 'cordon', label: 'Cordon', icon: 'shield', form: 'immediate' };
// Draining moves every workload off a node. Routine, and never a single click.
const DRAIN: Action = { id: 'drain', label: 'Drain', icon: 'server', form: 'confirm' };
const DELETE: Action = { id: 'delete', label: 'Delete', icon: 'trash', form: 'confirm', tone: 'danger' };

/**
 * The three things that can be done to a Helm release.
 *
 * Values opens the release's user-supplied values in the dock, exactly as Edit
 * opens an object's YAML: the same gesture on the same editor. There is no
 * separate Upgrade button, and that is deliberate rather than an omission --
 * an upgrade is a chart, a version and a set of values applied together, so
 * splitting it into a button here and a document there would mean choosing a
 * version without seeing the values it applies to. The editor carries all
 * three and its save button is the upgrade.
 *
 * It needs no helm to open: reading the values is reading the release, which
 * the app does itself. Upgrading from it does, and the editor says so there.
 */
const VALUES: Action = { id: 'values', label: 'Values', icon: 'edit', form: 'immediate' };
/**
 * Back to an earlier revision, which asks which one.
 *
 * Unlike an upgrade it needs no chart: the revision being returned to stored
 * its own rendered manifest, so a release whose chart nobody can find any more
 * can still be rolled back.
 */
const ROLLBACK: Action = { id: 'rollback', label: 'Rollback', icon: 'undo', form: 'revision', needsHelm: true };
/**
 * Removing the release and everything it installed. Danger-toned and confirmed,
 * like Delete, and for more reason than Delete has: one release is many objects.
 */
const UNINSTALL: Action = {
    id: 'uninstall',
    label: 'Uninstall',
    icon: 'trash',
    form: 'confirm',
    tone: 'danger',
    needsHelm: true,
};

/**
 * The kinds that have logs: a pod, and the workloads whose own pods can be
 * found from a selector. Everything else either runs no containers or has no
 * way to say which pods are its.
 */
const LOGGABLE = [
    'pods',
    'deployments',
    'statefulsets',
    'daemonsets',
    'replicasets',
    'replicationcontrollers',
    'jobs',
    'cronjobs',
];

/**
 * The kinds a shell can be opened in.
 *
 * A pod runs containers, and a workload resolves to one of its pods -- the same
 * reach Logs has, and for the same reason. A node is here too and is a
 * different thing entirely: there is no exec against a machine, so it is a
 * privileged pod created on it and chrooted into its filesystem. See
 * internal/kube/nodeshell.go.
 */
const SHELLABLE = [
    'pods',
    'deployments',
    'statefulsets',
    'daemonsets',
    'replicasets',
    'replicationcontrollers',
    'jobs',
    'nodes',
];

/**
 * The kinds a port can be forwarded from.
 *
 * A Service is the common case and the one that needs the most work behind it:
 * its ports exist only in the cluster's routing, so forwarding one means
 * finding a pod behind it and the port on that pod the service port lands on.
 * A workload forwards through one of its pods, exactly as a shell does.
 */
const FORWARDABLE = [
    'pods',
    'services',
    'deployments',
    'statefulsets',
    'daemonsets',
    'replicasets',
    'replicationcontrollers',
];

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
 * The views that are not objects at all: places in the app rather than things
 * in a cluster.
 *
 * Helm releases used to be here, and are not any more. A release is still not a
 * Kubernetes object -- it is a Secret the backend decodes -- which is why it
 * gets its own four actions below rather than Edit and Delete: deleting the
 * Secret would leave every object the release installed running, with nothing
 * left that knows they belong together.
 */
const NOT_AN_OBJECT = [DASHBOARD, SETTINGS, PORT_FORWARDS];

/** The actions offered for one kind, in the order the bar draws them. */
export function actionsFor(kind: string): Action[] {
    if (NOT_AN_OBJECT.includes(kind)) return [];
    if (kind === HELM_RELEASES) return [VALUES, ROLLBACK, UNINSTALL];

    const out: Action[] = [EDIT];
    if (LOGGABLE.includes(kind)) out.push(LOGS);
    if (SHELLABLE.includes(kind)) out.push(SHELL);
    if (FORWARDABLE.includes(kind)) out.push(FORWARD);
    if (SCALABLE.includes(kind)) out.push(SCALE);
    if (ROLLABLE.includes(kind)) out.push(RESTART);
    if (kind === 'nodes') out.push(CORDON, DRAIN);
    out.push(DELETE);
    return out;
}
