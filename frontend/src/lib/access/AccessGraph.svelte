<!--
  The access map: subjects on the left, the bindings that name them in the
  middle, the roles those bindings grant on the right.

  Laid out in three fixed columns rather than by a force simulation, because
  RBAC always has exactly this shape, and a layout that keeps it lets a row be
  read left to right as a sentence: "this group is bound, here, to that role".
  Nodes are buttons, so the map can be walked with the keyboard; the edges are
  one SVG underneath them.

  Pointing at a node lights the chain through it; clicking pins it and opens
  its details beside the map.
-->
<script lang="ts">
    import Icon from '../components/Icon.svelte';
    import { connected, type Column, type Graph, type GraphNode } from './model';

    interface Props {
        graph: Graph;
        selected: string | null;
        onselect: (id: string | null) => void;
    }

    let { graph, selected, onselect }: Props = $props();

    const NODE_H = 28;
    const STEP = 34;
    const MIN_COL = 180;
    const MAX_COL = 300;
    const MIN_GUTTER = 56;

    let width = $state(900);
    let hovered = $state<string | null>(null);

    let colW = $derived(Math.max(MIN_COL, Math.min(MAX_COL, (width - 2 * MIN_GUTTER) / 3)));
    let gutter = $derived(Math.max(MIN_GUTTER, (width - 3 * colW) / 2));
    let total = $derived(3 * colW + 2 * gutter);

    let columns = $derived<[Column, GraphNode[]][]>([
        ['subject', graph.subjects],
        ['binding', graph.bindings],
        ['role', graph.roles],
    ]);
    let longest = $derived(Math.max(1, graph.subjects.length, graph.bindings.length, graph.roles.length));
    let height = $derived(longest * STEP);

    /** Where every node sits: shorter columns are centred against the longest. */
    let positions = $derived.by(() => {
        const out = new Map<string, { x: number; y: number }>();
        columns.forEach(([, nodes], c) => {
            const offset = ((longest - nodes.length) * STEP) / 2;
            nodes.forEach((n, i) => out.set(n.id, { x: c * (colW + gutter), y: offset + i * STEP }));
        });
        return out;
    });

    let focus = $derived(hovered ?? selected);
    let lit = $derived(focus ? connected(graph, focus) : null);

    function path(from: string, to: string): string {
        const a = positions.get(from);
        const b = positions.get(to);
        if (!a || !b) return '';
        const x1 = a.x + colW;
        const y1 = a.y + NODE_H / 2;
        const x2 = b.x;
        const y2 = b.y + NODE_H / 2;
        const mid = (x1 + x2) / 2;
        return `M${x1},${y1} C${mid},${y1} ${mid},${y2} ${x2},${y2}`;
    }

    const ICONS: Record<string, string> = {
        User: 'user',
        Group: 'users',
        ServiceAccount: 'grant',
        RoleBinding: 'link',
        ClusterRoleBinding: 'link',
        Role: 'policy',
        ClusterRole: 'policy',
    };

    function tag(node: GraphNode): string {
        if (node.missing) return node.column === 'subject' ? 'no such account' : 'role missing';
        if (node.kind === 'ClusterRoleBinding') return 'cluster-wide';
        if (node.kind === 'ClusterRole') return 'cluster role';
        if (node.kind === 'Group') return 'group';
        if (node.kind === 'User') return 'user';
        return node.namespace;
    }

    function title(node: GraphNode): string {
        const where = node.namespace ? ` in ${node.namespace}` : '';
        const risk = node.risk !== 'none' ? ` — ${node.risk} risk` : '';
        return `${node.kind} ${node.name}${where}${risk}`;
    }

    function onkey(event: KeyboardEvent): void {
        if (event.key === 'Escape') onselect(null);
    }
</script>

<!-- svelte-ignore a11y_no_static_element_interactions -->
<div class="graph" bind:clientWidth={width} onkeydown={onkey}>
    <div class="heads" style:width="{total}px">
        <div style:width="{colW}px">
            <strong>Who</strong>
            <span>{graph.subjects.length} users, groups & service accounts</span>
        </div>
        <div style:width="{colW}px" style:left="{colW + gutter}px">
            <strong>Is granted, where</strong>
            <span>{graph.bindings.length} bindings</span>
        </div>
        <div style:width="{colW}px" style:left="{2 * (colW + gutter)}px">
            <strong>What</strong>
            <span>{graph.roles.length} roles</span>
        </div>
    </div>

    <!-- A click on the empty canvas lets go of the pinned node. -->
    <!-- svelte-ignore a11y_click_events_have_key_events -->
    <div
        class="canvas"
        style:width="{total}px"
        style:height="{height}px"
        onclick={(e) => {
            if (e.target === e.currentTarget) onselect(null);
        }}
    >
        <svg width={total} height={height} aria-hidden="true">
            {#each graph.edges as edge (`${edge.from}>${edge.to}`)}
                {@const on = lit !== null && lit.has(edge.from) && lit.has(edge.to)}
                <path d={path(edge.from, edge.to)} class:on class:off={lit !== null && !on} />
            {/each}
        </svg>

        {#each columns as [column, nodes] (column)}
            {#each nodes as node (node.id)}
                {@const at = positions.get(node.id)}
                {#if at}
                    <button
                        class="node {node.column} risk-{node.risk}"
                        class:selected={selected === node.id}
                        class:dim={lit !== null && !lit.has(node.id)}
                        class:me={node.me}
                        class:missing={node.missing}
                        class:match={node.match}
                        class:system={node.system}
                        style:left="{at.x}px"
                        style:top="{at.y}px"
                        style:width="{colW}px"
                        style:height="{NODE_H}px"
                        title={title(node)}
                        onmouseenter={() => (hovered = node.id)}
                        onmouseleave={() => (hovered = null)}
                        onfocus={() => (hovered = node.id)}
                        onblur={() => (hovered = null)}
                        onclick={() => onselect(selected === node.id ? null : node.id)}
                    >
                        <Icon name={ICONS[node.kind] ?? 'box'} size={13} />
                        <span class="label">{node.name}</span>
                        {#if node.me}<span class="you">you</span>{/if}
                        <span class="tag">{tag(node)}</span>
                    </button>
                {/if}
            {/each}
        {/each}
    </div>
</div>

<style>
    .graph {
        position: relative;
        min-width: 0;
        overflow-x: auto;
        padding-bottom: 12px;
    }

    .heads {
        position: relative;
        height: 34px;
        margin-bottom: 6px;
    }

    .heads > div {
        position: absolute;
        top: 0;
        display: flex;
        flex-direction: column;
        gap: 1px;
        font-size: 11px;
    }

    .heads strong {
        font-size: 11px;
        letter-spacing: 0.08em;
        text-transform: uppercase;
        color: var(--text-dim);
    }

    .heads span {
        color: var(--text-faint);
    }

    .canvas {
        position: relative;
    }

    svg {
        position: absolute;
        inset: 0;
        pointer-events: none;
    }

    path {
        fill: none;
        stroke: var(--border);
        stroke-width: 1.2;
        transition: opacity 120ms ease;
    }

    path.on {
        stroke: var(--accent);
        stroke-width: 1.8;
    }

    path.off {
        opacity: 0.15;
    }

    .node {
        position: absolute;
        display: flex;
        align-items: center;
        gap: 6px;
        padding: 0 8px;
        border: 1px solid var(--border);
        border-left: 3px solid var(--border);
        border-radius: var(--radius-sm);
        background: var(--bg-panel);
        font-size: 12px;
        text-align: left;
        color: var(--text);
        transition:
            opacity 120ms ease,
            border-color 120ms ease;
    }

    .node:hover {
        background: var(--bg-hover);
    }

    .node.selected {
        border-color: var(--accent);
        box-shadow: 0 0 0 1px var(--accent);
    }

    .node.dim {
        opacity: 0.3;
    }

    .node.system .label {
        color: var(--text-dim);
    }

    .node.match {
        background: color-mix(in srgb, var(--accent) 14%, var(--bg-panel));
    }

    /* How much a role hands over, carried back along the chain to whoever holds it. */
    .node.risk-low {
        border-left-color: var(--accent);
    }

    .node.risk-high {
        border-left-color: var(--warn);
    }

    .node.risk-critical {
        border-left-color: var(--error);
    }

    .node.missing {
        border-style: dashed;
        color: var(--text-faint);
    }

    .node.me {
        background: color-mix(in srgb, var(--ok) 12%, var(--bg-panel));
    }

    .label {
        flex: 1 1 auto;
        min-width: 0;
        overflow: hidden;
        text-overflow: ellipsis;
        white-space: nowrap;
    }

    .tag {
        flex: 0 1 auto;
        max-width: 45%;
        overflow: hidden;
        text-overflow: ellipsis;
        white-space: nowrap;
        font-size: 10.5px;
        color: var(--text-faint);
    }

    .node.missing .tag {
        color: var(--warn);
    }

    .you {
        flex: none;
        padding: 0 5px;
        border-radius: 8px;
        background: var(--ok);
        color: var(--bg);
        font-size: 10px;
        font-weight: 600;
    }
</style>
