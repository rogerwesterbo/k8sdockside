// The documents open in the dock's YAML editor.
//
// One per dock tab, keyed by the tab's id and outliving the component that
// renders it: switching dock tabs, or away to another cluster and back,
// destroys the editor component, and an edit half made must survive that. It is
// the tab closing that discards a draft, because that is the only action that
// says so.
//
// Nothing here knows about the workspace store. A document needs the four
// fields that name an object and nothing else, which is what keeps this
// testable without a window, a tab strip or a cluster. The change signal is no
// exception: it is a leaf that names objects the same way, and saying an object
// changed is not the same as knowing who is looking at it.

import { ResourceService } from '../../../bindings/github.com/roger/k8sdockside';
import { changes } from './changes.svelte';

/** What an editor is open on: enough to read one object and write it back. */
export interface EditTarget {
    contextId: string;
    kind: string;
    namespace: string;
    name: string;
}

/** The backend's answer to "is this still YAML?". Line is 1-based, 0 for none. */
export interface YamlCheck {
    valid: boolean;
    message: string;
    line: number;
}

const VALID: YamlCheck = { valid: true, message: '', line: 0 };

/** One open document. */
export interface EditorDoc {
    status: 'loading' | 'ready' | 'error';
    /** The YAML as the cluster last gave it: what "changed" is measured against. */
    original: string;
    text: string;
    /** Why the document could not be read, or why the last save was refused. */
    error: string;
    check: YamlCheck;
    saving: boolean;
    /** True from a successful save until the next keystroke. */
    saved: boolean;
}

const BLANK: EditorDoc = {
    status: 'loading',
    original: '',
    text: '',
    error: '',
    check: VALID,
    saving: false,
    saved: false,
};

/**
 * How long after the last keystroke the document is checked.
 *
 * The check is a call into Go rather than a parser in the frontend, so that the
 * editor and the save path agree about what YAML is -- there is one parser, and
 * it is the one that will actually be asked to read the document. A quarter of
 * a second is long enough that typing a word costs one call.
 */
const CHECK_DELAY = 250;

function message(err: unknown): string {
    if (err instanceof Error) return err.message;
    return String(err);
}

class Editors {
    private docs = $state<Record<string, EditorDoc>>({});
    private timers = new Map<string, ReturnType<typeof setTimeout>>();
    /**
     * The load each document is on. A reload issued while one is still in
     * flight would otherwise be able to lose the race and overwrite the newer
     * answer with the older one.
     */
    private loads = new Map<string, number>();

    /** The document for a tab. Never null: an unopened tab reads as loading. */
    doc(id: string): EditorDoc {
        return this.docs[id] ?? BLANK;
    }

    /** Whether a document has edits that are not in the cluster. */
    isDirty(id: string): boolean {
        const doc = this.docs[id];
        return doc !== undefined && doc.status === 'ready' && doc.text !== doc.original;
    }

    /**
     * Reads an object into the editor. Called every time the component mounts,
     * so an already-open document is left exactly as the user left it; `force`
     * is the Reload button, which deliberately throws the draft away.
     */
    async load(id: string, target: EditTarget, { force = false } = {}): Promise<void> {
        if (this.docs[id] && !force) return;

        const attempt = (this.loads.get(id) ?? 0) + 1;
        this.loads.set(id, attempt);
        this.docs[id] = { ...BLANK, status: 'loading' };

        try {
            const text = await ResourceService.ResourceYAML(
                target.contextId,
                target.kind,
                target.namespace,
                target.name,
            );
            if (this.loads.get(id) !== attempt) return;
            this.docs[id] = { ...BLANK, status: 'ready', original: text, text };
        } catch (err) {
            if (this.loads.get(id) !== attempt) return;
            this.docs[id] = { ...BLANK, status: 'error', error: message(err) };
        }
    }

    /** Records a keystroke and schedules the check that follows it. */
    edit(id: string, text: string): void {
        const doc = this.docs[id];
        if (!doc) return;
        doc.text = text;
        doc.saved = false;
        this.scheduleCheck(id);
    }

    /** Throws away the edits and goes back to what the cluster gave. */
    revert(id: string): void {
        const doc = this.docs[id];
        if (!doc) return;
        clearTimeout(this.timers.get(id));
        this.timers.delete(id);
        doc.text = doc.original;
        doc.check = VALID;
        doc.error = '';
        doc.saved = false;
    }

    /**
     * Writes the document to the cluster.
     *
     * On success the editor takes what came back rather than keeping what was
     * sent: the object now carries a new resourceVersion, and whatever
     * defaulting and admission control did to it on the way in. Keeping the
     * sent text would leave the next save arguing with a version of the object
     * that no longer exists.
     *
     * A save is also the one moment this app makes the same object wrong
     * wherever else it is on screen, so it says the object changed. On success
     * only: a refused save left the cluster holding what it already held.
     */
    async save(id: string, target: EditTarget): Promise<boolean> {
        const doc = this.docs[id];
        if (!doc || doc.saving || doc.status !== 'ready') return false;

        doc.saving = true;
        doc.error = '';
        try {
            const saved = await ResourceService.ApplyYAML(
                target.contextId,
                target.kind,
                target.namespace,
                target.name,
                doc.text,
            );
            doc.text = saved;
            doc.original = saved;
            doc.check = VALID;
            doc.saved = true;
            changes.changed(target);
            return true;
        } catch (err) {
            doc.error = message(err);
            return false;
        } finally {
            doc.saving = false;
        }
    }

    /** Drops a document, with the tab it belonged to. */
    forget(id: string): void {
        clearTimeout(this.timers.get(id));
        this.timers.delete(id);
        this.loads.delete(id);
        delete this.docs[id];
    }

    private scheduleCheck(id: string): void {
        clearTimeout(this.timers.get(id));
        this.timers.set(
            id,
            setTimeout(() => {
                this.timers.delete(id);
                void this.check(id);
            }, CHECK_DELAY),
        );
    }

    /**
     * Asks the backend whether the document parses.
     *
     * A failed call leaves the last answer standing rather than blocking the
     * save: this is an aid to the person typing, not the gate on what reaches
     * the cluster. The API server is that gate, and it is not optional.
     */
    private async check(id: string): Promise<void> {
        const doc = this.docs[id];
        if (!doc) return;

        const text = doc.text;
        try {
            const result = await ResourceService.CheckYAML(text);
            const current = this.docs[id];
            // Another keystroke landed while this was in flight; its own check
            // is already scheduled and will be the one that matters.
            if (!current || current.text !== text) return;
            current.check = { valid: result.valid, message: result.message, line: result.line };
        } catch {
            // Deliberately silent -- see above.
        }
    }
}

export const editors = new Editors();
