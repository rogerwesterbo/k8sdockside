// The tab being dragged, for the moment it is in the air.
//
// Held here rather than in the drag's own dataTransfer because a dragover
// handler is only allowed to see the *types* a drag carries, never its values --
// and which pane a tab came from is exactly what the pane under the pointer
// needs to know to decide whether this is a reorder or a move. The drag never
// leaves the window, so a module-level note of it is the whole of the problem.
//
// It is reactive, and that is why it is a file of its own rather than a
// variable in panes.ts: every pane draws a drop target while a tab is in the
// air, and all of them have to stop the moment it lands, wherever it lands.
// The DOM cannot be relied on to say when that is. A tab dropped into another
// pane is taken out of the strip it came from, and a drag source removed from
// the document fires no dragend -- so the panes that were not dropped on would
// go on showing a target for a drag that had already ended. Ending it here,
// once, clears every one of them together.

import type { PaneId } from './panes';

export interface TabDrag {
    id: string;
    from: PaneId;
}

let inFlight = $state<TabDrag | null>(null);

export function beginTabDrag(drag: TabDrag): void {
    inFlight = drag;
}

export function currentTabDrag(): TabDrag | null {
    return inFlight;
}

export function endTabDrag(): void {
    inFlight = null;
}
