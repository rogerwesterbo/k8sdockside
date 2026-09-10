<!--
  What one node on the access map means, in words.

  A subject gets the question people actually have about it -- "what can this
  do, and where?" -- answered as one table per scope, gathered from every
  binding that reaches it, including the ones reaching it through the groups
  it is in without anybody having said so. A binding says what it grants to
  whom and where. A role says what it allows and who holds it.
-->
<script lang="ts">
    import Icon from '../components/Icon.svelte';
    import { detail } from '../state/detail.svelte';
    import { workspace } from '../state/workspace.svelte';
    import RuleMatrix from './RuleMatrix.svelte';
    import {
        assessRules,
        bindingKey,
        describeScope,
        explainRole,
        explainSubject,
        grantsFor,
        impliedGroups,
        isMe,
        roleKey,
        roleKeyOf,
        subjectKey,
        type Access,
        type Binding,
        type Index,
        type Role,
        type Rule,
        type Subject,
    } from './model';

    interface Props {
        contextId: string;
        access: Access;
        idx: Index;
        /** A graph node id: "s:", "b:" or "r:" and the object's key. */
        id: string;
        onselect: (id: string | null) => void;
    }

    let { contextId, access, idx, id, onselect }: Props = $props();

    /** kind/namespace/name, where only the name may itself hold a slash. */
    function split(key: string): { kind: string; namespace: string; name: string } {
        const [kind, namespace, ...rest] = key.split('/');
        return { kind, namespace, name: rest.join('/') };
    }

    let resolved = $derived.by(() => {
        const key = id.slice(2);
        const parts = split(key);
        if (id.startsWith('s:')) return { type: 'subject' as const, subject: parts as Subject };
        if (id.startsWith('b:')) {
            const binding = access.bindings.find((b) => bindingKey(b) === key) ?? null;
            return { type: 'binding' as const, binding };
        }
        return { type: 'role' as const, role: idx.roles.get(key) ?? null, parts };
    });

    const KIND_PLURAL: Record<string, string> = {
        Role: 'roles',
        ClusterRole: 'clusterroles',
        RoleBinding: 'rolebindings',
        ClusterRoleBinding: 'clusterrolebindings',
        ServiceAccount: 'serviceaccounts',
    };

    function describe(kind: string, namespace: string, name: string): void {
        void detail.open({ contextId, kind: KIND_PLURAL[kind], namespace, name });
    }

    function edit(kind: string, namespace: string, name: string): void {
        workspace.openEditor({ contextId, kind: KIND_PLURAL[kind], namespace, name });
    }

    /** A subject's rules gathered per scope: cluster-wide first, then each namespace. */
    function byScope(subject: Subject): { scope: string; rules: Rule[]; roles: string[] }[] {
        const scopes = new Map<string, { rules: Rule[]; roles: string[] }>();
        for (const grant of grantsFor(access, idx, subject)) {
            if (!grant.role) continue;
            const entry = scopes.get(grant.scope) ?? { rules: [], roles: [] };
            entry.rules.push(...grant.role.rules);
            if (!entry.roles.includes(grant.role.name)) entry.roles.push(grant.role.name);
            scopes.set(grant.scope, entry);
        }
        return [...scopes.entries()]
            .map(([scope, v]) => ({ scope, ...v }))
            .sort((a, b) => (a.scope === '' ? -1 : b.scope === '' ? 1 : a.scope.localeCompare(b.scope)));
    }

    function riskText(level: string): string {
        return (
            { critical: 'Critical', high: 'High', low: 'Writes', none: 'Read-only' } as Record<string, string>
        )[level];
    }

    function roleOf(binding: Binding): Role | null {
        return idx.roles.get(roleKeyOf(binding)) ?? null;
    }
</script>

<aside class="detail">
    <button class="close" onclick={() => onselect(null)} title="Close" aria-label="Close details">
        <Icon name="close" size={14} />
    </button>

    {#if resolved.type === 'subject'}
        {@const s = resolved.subject}
        {@const grants = grantsFor(access, idx, s)}
        {@const implied = impliedGroups(s, access.me)}
        {@const missing =
            s.kind === 'ServiceAccount' &&
            access.serviceAccounts.length > 0 &&
            !idx.accounts.has(`${s.namespace}/${s.name}`)}
        <p class="kind">
            <Icon name={s.kind === 'Group' ? 'users' : s.kind === 'User' ? 'user' : 'grant'} size={13} />
            {s.kind}{s.namespace ? ` in ${s.namespace}` : ''}
        </p>
        <h3 class="selectable">{s.name}</h3>
        {#if isMe(s, access.me)}
            <p class="you">{s.kind === 'Group' ? 'You are in this group.' : 'This is you.'}</p>
        {/if}
        <p class="explain">{explainSubject(s)}</p>
        {#if missing}
            <p class="warn">
                No service account by this name exists. The binding is waiting for one — anybody who creates it
                gets these permissions.
            </p>
        {/if}
        {#if implied.length}
            <p class="sub">Also a member of</p>
            <div class="chips">
                {#each implied as g (g)}
                    <button class="chip" onclick={() => onselect('s:' + subjectKey({ kind: 'Group', name: g, namespace: '' }))}>
                        {g}
                    </button>
                {/each}
            </div>
        {/if}

        <h4>Granted through</h4>
        {#if grants.length === 0}
            <p class="muted">No binding names this subject or its groups. It can do nothing RBAC controls.</p>
        {:else}
            <ul class="routes">
                {#each grants as g (bindingKey(g.binding))}
                    {@const risk = g.role ? assessRules(g.role.rules).level : 'none'}
                    <li>
                        <span class="dot risk-{risk}" title={riskText(risk)}></span>
                        <button class="link" onclick={() => onselect('r:' + roleKeyOf(g.binding))}>
                            {g.binding.roleName}
                        </button>
                        <span class="muted">{g.scope ? `in ${g.scope}` : 'cluster-wide'}</span>
                        <span class="via">
                            via
                            <button class="link" onclick={() => onselect('b:' + bindingKey(g.binding))}>
                                {g.binding.name}
                            </button>
                            {#if g.viaGroup}(as member of {g.viaGroup}){/if}
                        </span>
                    </li>
                {/each}
            </ul>
        {/if}

        {#each byScope(s) as scope (scope.scope)}
            <h4>{scope.scope ? `In namespace ${scope.scope}` : 'Everywhere (cluster-wide)'}</h4>
            <p class="muted small">From {scope.roles.join(', ')}</p>
            <RuleMatrix rules={scope.rules} />
        {/each}

        {#if s.kind === 'ServiceAccount' && !missing}
            <div class="actions">
                <button class="btn" onclick={() => describe('ServiceAccount', s.namespace, s.name)}>
                    Open service account
                </button>
            </div>
        {/if}
    {:else if resolved.type === 'binding'}
        {@const b = resolved.binding}
        {#if !b}
            <p class="muted">This binding is no longer there.</p>
        {:else}
            {@const role = roleOf(b)}
            <p class="kind"><Icon name="link" size={13} /> {b.kind}{b.namespace ? ` in ${b.namespace}` : ''}</p>
            <h3 class="selectable">{b.name}</h3>
            <p class="explain">
                Grants the {b.roleKind}
                <button class="link" onclick={() => onselect('r:' + roleKeyOf(b))}>{b.roleName}</button>
                to {b.subjects.length}
                {b.subjects.length === 1 ? 'subject' : 'subjects'}
                <strong>{describeScope(b)}</strong>.
            </p>
            {#if b.kind === 'RoleBinding' && b.roleKind === 'ClusterRole'}
                <p class="note">
                    A RoleBinding to a ClusterRole borrows the cluster role's rules, but they apply only inside
                    {b.namespace}. This is the usual way to reuse admin, edit or view per namespace.
                </p>
            {/if}
            {#if !role}
                <p class="warn">
                    The {b.roleKind} {b.roleName} does not exist{access.unreadable.length ? ', or could not be read' : ''}.
                    This binding grants nothing until it is created.
                </p>
            {/if}

            <h4>Subjects</h4>
            {#if b.subjects.length === 0}
                <p class="muted">Nobody. The binding exists but grants to no one.</p>
            {:else}
                <ul class="routes">
                    {#each b.subjects as s, i (`${subjectKey(s)}|${i}`)}
                        <li>
                            <Icon name={s.kind === 'Group' ? 'users' : s.kind === 'User' ? 'user' : 'grant'} size={12} />
                            <button class="link" onclick={() => onselect('s:' + subjectKey(s))}>{s.name}</button>
                            <span class="muted">{s.kind}{s.namespace ? ` in ${s.namespace}` : ''}</span>
                            {#if isMe(s, access.me)}<span class="pill-you">you</span>{/if}
                        </li>
                    {/each}
                </ul>
            {/if}

            {#if role}
                <h4>What it allows</h4>
                <RuleMatrix rules={role.rules} />
            {/if}

            <div class="actions">
                <button class="btn" onclick={() => describe(b.kind, b.namespace, b.name)}>Describe</button>
                <button class="btn" onclick={() => edit(b.kind, b.namespace, b.name)}>Edit YAML</button>
            </div>
        {/if}
    {:else}
        {@const role = resolved.role}
        {@const parts = resolved.parts}
        <p class="kind"><Icon name="policy" size={13} /> {parts.kind}{parts.namespace ? ` in ${parts.namespace}` : ''}</p>
        <h3 class="selectable">{parts.name}</h3>
        {#if !role}
            <p class="warn">
                This role does not exist{access.unreadable.length ? ', or could not be read' : ''}, but a binding
                names it.
            </p>
        {:else}
            {@const risk = assessRules(role.rules)}
            {@const holders = idx.bindingsByRole.get(roleKey(role)) ?? []}
            {#if explainRole(role)}<p class="explain">{explainRole(role)}</p>{/if}
            <p class="risk risk-{risk.level}">
                <span class="dot risk-{risk.level}"></span>
                {riskText(risk.level)}
            </p>
            {#if risk.reasons.length}
                <ul class="reasons">
                    {#each risk.reasons as r (r)}<li>{r}</li>{/each}
                </ul>
            {/if}
            {#if role.aggregated}
                <p class="note">
                    Aggregated: a controller builds this role's rules from every ClusterRole matching its selector.
                    Installing an operator can quietly add to it.
                </p>
            {/if}

            <h4>Held through</h4>
            {#if holders.length === 0}
                <p class="muted">No binding names this role. It grants nothing to anyone.</p>
            {:else}
                <ul class="routes">
                    {#each holders as b (bindingKey(b))}
                        <li>
                            <button class="link" onclick={() => onselect('b:' + bindingKey(b))}>{b.name}</button>
                            <span class="muted">
                                {b.kind === 'ClusterRoleBinding' ? 'cluster-wide' : `in ${b.namespace}`} ·
                                {b.subjects.length}
                                {b.subjects.length === 1 ? 'subject' : 'subjects'}
                            </span>
                        </li>
                    {/each}
                </ul>
            {/if}

            <h4>What it allows</h4>
            <RuleMatrix rules={role.rules} sentences empty="No rules: this role allows nothing." />

            <div class="actions">
                <button class="btn" onclick={() => describe(role.kind, role.namespace, role.name)}>Describe</button>
                <button class="btn" onclick={() => edit(role.kind, role.namespace, role.name)}>Edit YAML</button>
            </div>
        {/if}
    {/if}
</aside>

<style>
    .detail {
        position: relative;
        padding: 14px 16px 20px;
        border: 1px solid var(--border);
        border-radius: var(--radius);
        background: var(--bg-panel);
        font-size: 12.5px;
        overflow: auto;
    }

    .close {
        position: absolute;
        top: 10px;
        right: 10px;
        display: grid;
        place-items: center;
        width: 24px;
        height: 24px;
        border-radius: var(--radius-sm);
        color: var(--text-dim);
    }

    .close:hover {
        background: var(--bg-hover);
        color: var(--text);
    }

    .kind {
        display: flex;
        align-items: center;
        gap: 6px;
        margin: 0;
        color: var(--text-faint);
        font-size: 11px;
        text-transform: uppercase;
        letter-spacing: 0.06em;
    }

    h3 {
        margin: 4px 28px 8px 0;
        font-size: 15px;
        font-weight: 600;
        word-break: break-all;
    }

    h4 {
        margin: 16px 0 6px;
        font-size: 11px;
        letter-spacing: 0.08em;
        text-transform: uppercase;
        color: var(--text-faint);
        font-weight: 600;
    }

    .explain {
        margin: 0 0 8px;
        color: var(--text-dim);
        line-height: 1.5;
    }

    .note,
    .warn,
    .you {
        margin: 8px 0;
        padding: 7px 10px;
        border-radius: var(--radius-sm);
        line-height: 1.5;
    }

    .note {
        background: color-mix(in srgb, var(--accent) 10%, transparent);
        color: var(--text-dim);
    }

    .warn {
        background: color-mix(in srgb, var(--warn) 14%, transparent);
        color: var(--text);
    }

    .you {
        background: color-mix(in srgb, var(--ok) 14%, transparent);
        color: var(--text);
        font-weight: 600;
    }

    .sub {
        margin: 10px 0 4px;
        color: var(--text-faint);
        font-size: 11.5px;
    }

    .chips {
        display: flex;
        flex-wrap: wrap;
        gap: 4px;
    }

    .chip {
        padding: 1px 8px;
        border: 1px solid var(--border);
        border-radius: 10px;
        font-size: 11px;
        font-family: var(--mono);
        color: var(--text-dim);
    }

    .chip:hover {
        border-color: var(--accent);
        color: var(--text);
    }

    .routes {
        list-style: none;
        margin: 0;
        padding: 0;
        display: flex;
        flex-direction: column;
        gap: 5px;
    }

    .routes li {
        display: flex;
        align-items: center;
        flex-wrap: wrap;
        gap: 6px;
    }

    .via {
        color: var(--text-faint);
        font-size: 11.5px;
    }

    .link {
        color: var(--accent);
        font-family: var(--mono);
        font-size: 12px;
    }

    .link:hover {
        text-decoration: underline;
    }

    .muted {
        color: var(--text-faint);
    }

    .small {
        margin: -2px 0 6px;
        font-size: 11px;
    }

    .dot {
        display: inline-block;
        width: 8px;
        height: 8px;
        border-radius: 50%;
        background: var(--text-faint);
        flex: none;
    }

    .dot.risk-low {
        background: var(--accent);
    }

    .dot.risk-high {
        background: var(--warn);
    }

    .dot.risk-critical {
        background: var(--error);
    }

    .risk {
        display: flex;
        align-items: center;
        gap: 6px;
        margin: 6px 0 2px;
        font-weight: 600;
    }

    .risk.risk-critical {
        color: var(--error);
    }

    .risk.risk-high {
        color: var(--warn);
    }

    .reasons {
        margin: 2px 0 0;
        padding-left: 18px;
        color: var(--text-dim);
        line-height: 1.6;
    }

    .pill-you {
        padding: 0 5px;
        border-radius: 8px;
        background: var(--ok);
        color: var(--bg);
        font-size: 10px;
        font-weight: 600;
    }

    .actions {
        display: flex;
        gap: 8px;
        margin-top: 16px;
    }

    .btn {
        padding: 5px 12px;
        border: 1px solid var(--border);
        border-radius: var(--radius-sm);
        background: var(--bg-raised);
        font-size: 12px;
    }

    .btn:hover {
        border-color: var(--accent);
        background: var(--bg-hover);
    }
</style>
