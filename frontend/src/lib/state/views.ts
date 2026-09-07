// What each resource tab was showing when it was last on screen: its sort, its
// namespace filter and its search.
//
// The pane wraps the active view in {#key tab.id}, so bringing another tab
// forward destroys the table and coming back builds a new one, which would
// otherwise land on the default order, every namespace and an empty search --
// and pods sorted by age is exactly the view somebody leaves for a moment and
// expects to find again. So a table writes what it is showing here as it
// changes, and reads it back as it mounts.
//
// Kept for the session and no longer. A closed tab takes its view with it, the
// way it takes an editor's buffer: a search left in a tab closed this morning
// is not the search wanted from the one opened this afternoon, and hiding rows
// behind a filter nobody remembers typing would be the worse surprise. Not
// written to disk either: a sort is a column index, and a settings file older
// than a change to a kind's columns would sort by the wrong one.

/** How one tab was left. */
export interface TabView {
    sortColumn: number | null;
    sortDescending: boolean;
    namespaces: string[];
    query: string;
}

const remembered = new Map<string, TabView>();

export const views = {
    /** How a tab was left, or null for one that has not been seen. */
    recall(tabId: string): TabView | null {
        const view = remembered.get(tabId);
        return view ? { ...view, namespaces: [...view.namespaces] } : null;
    },

    remember(tabId: string, view: TabView): void {
        remembered.set(tabId, { ...view, namespaces: [...view.namespaces] });
    },

    /** Drops a tab's view, as closing it does. */
    forget(tabId: string): void {
        remembered.delete(tabId);
    },

    /** Drops every tab's view. For tests, which share this module across cases. */
    forgetAll(): void {
        remembered.clear();
    },
};
