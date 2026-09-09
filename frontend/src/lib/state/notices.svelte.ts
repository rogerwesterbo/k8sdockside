// The one-line message the status bar shows: something failed, or something
// worked and said so.
//
// Its own store because nothing about it belongs to a workspace. Every layer
// reports through it -- an action refused by the API server, a theme that would
// not load, a forward that dropped -- and none of those should have to reach
// for the thing that owns tabs and panes to say one sentence.

/** A transient message shown in the status bar. */
export interface Notice {
    text: string;
    tone: 'info' | 'error';
}

class Notices {
    /** What the status bar is showing, or nothing. */
    current = $state<Notice | null>(null);

    /**
     * Reports something that went wrong, in the words of whatever refused it.
     * Public because an action's refusal -- an API server saying which verb on
     * which resource was denied -- is reported by the component that asked.
     */
    fail(text: string): void {
        this.current = { text, tone: 'error' };
    }

    inform(text: string): void {
        this.current = { text, tone: 'info' };
    }

    dismiss(): void {
        this.current = null;
    }
}

export const notices = new Notices();
