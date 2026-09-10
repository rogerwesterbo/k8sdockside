<!--
  "Who can…?": the question RBAC is hardest to answer by reading it, because
  the answer is spread over every role and binding in the cluster. Asked here
  in the shape `kubectl auth can-i` uses, and answered with the route by which
  each subject gets there -- the binding, the role, and the rule that matched.
-->
<script lang="ts">
    import Icon from '../components/Icon.svelte';
    import {
        COMMON_QUESTIONS,
        VERB_HELP,
        bindingKey,
        describeRule,
        explainSubject,
        isMe,
        knownResources,
        roleKey,
        subjectKey,
        whoCan,
        type Access,
        type Index,
    } from './model';

    interface Props {
        access: Access;
        idx: Index;
        namespaces: string[];
        /** Show a node on the map. */
        onshow: (id: string) => void;
    }

    let { access, idx, namespaces, onshow }: Props = $props();

    const VERBS = [
        'get',
        'list',
        'watch',
        'create',
        'update',
        'patch',
        'delete',
        'deletecollection',
        'escalate',
        'bind',
        'impersonate',
    ];

    let verb = $state('get');
    let resource = $state('secrets');
    let namespace = $state('');

    let resources = $derived(knownResources(access));
    let answers = $derived(whoCan(access, idx, { verb, resource, namespace }));
    let masters = $derived(
        access.bindings.some((b) => b.subjects.some((s) => s.kind === 'Group' && s.name === 'system:masters')),
    );

    function ask(q: { verb: string; resource: string }): void {
        verb = q.verb;
        resource = q.resource;
    }
</script>

<section class="whocan">
    <div class="question">
        <span class="lead">Who can</span>
        <select bind:value={verb} title={VERB_HELP[verb]}>
            {#each VERBS as v (v)}
                <option value={v}>{v}</option>
            {/each}
        </select>
        <input
            type="text"
            list="access-resources"
            bind:value={resource}
            placeholder="pods, deployments.apps, pods/exec…"
            spellcheck="false"
            aria-label="Resource"
        />
        <datalist id="access-resources">
            {#each resources as r (r)}<option value={r}></option>{/each}
        </datalist>
        <span class="lead">in</span>
        <select bind:value={namespace}>
            <option value="">any namespace</option>
            {#each namespaces as ns (ns)}<option value={ns}>{ns}</option>{/each}
        </select>
        <span class="lead">?</span>
    </div>

    <div class="quick">
        <span class="muted">Common questions:</span>
        {#each COMMON_QUESTIONS as q (q.label)}
            <button class="chip" class:on={q.verb === verb && q.resource === resource} onclick={() => ask(q)}>
                {q.label}
            </button>
        {/each}
    </div>

    <p class="hint">
        {VERB_HELP[verb] ?? verb}. Written the way <code>kubectl auth can-i</code> takes it:
        <code>resource.group/subresource</code>, with the group left off for core kinds.
    </p>

    {#if answers.length === 0}
        <p class="none">
            <Icon name="shield" size={14} /> No RBAC binding allows this{namespace ? ` in ${namespace}` : ''}.
        </p>
    {:else}
        <p class="count">
            {answers.length}
            {answers.length === 1 ? 'subject' : 'subjects'} can {verb}
            <code>{resource}</code>{namespace ? ` in ${namespace}` : ''}:
        </p>
        <ul class="answers">
            {#each answers as a (subjectKey(a.subject))}
                <li>
                    <div class="who">
                        <Icon
                            name={a.subject.kind === 'Group' ? 'users' : a.subject.kind === 'User' ? 'user' : 'grant'}
                            size={13}
                        />
                        <button class="link" onclick={() => onshow('s:' + subjectKey(a.subject))}>
                            {a.subject.name}
                        </button>
                        <span class="muted">
                            {a.subject.kind}{a.subject.namespace ? ` in ${a.subject.namespace}` : ''}
                        </span>
                        {#if isMe(a.subject, access.me)}<span class="you">you</span>{/if}
                        {#if a.onlyNamed}<span class="limited">only named objects</span>{/if}
                    </div>
                    {#if a.subject.kind === 'Group'}
                        <p class="explain">{explainSubject(a.subject)}</p>
                    {/if}
                    <ul class="routes">
                        {#each a.routes as r, i (`${bindingKey(r.binding)}|${i}`)}
                            <li>
                                via
                                <button class="link" onclick={() => onshow('b:' + bindingKey(r.binding))}>
                                    {r.binding.name}
                                </button>
                                →
                                <button class="link" onclick={() => onshow('r:' + roleKey(r.role))}>
                                    {r.role.name}
                                </button>
                                <span class="scope">{r.scope ? `in ${r.scope}` : 'cluster-wide'}</span>
                                <span class="rule">{describeRule(r.rule)}</span>
                            </li>
                        {/each}
                    </ul>
                </li>
            {/each}
        </ul>
    {/if}

    <p class="caveat">
        Worked out from the roles and bindings this app could read{access.unreadable.length
            ? ' — some could not be read, so the list may be short'
            : ''}. Members of the group system:masters can do everything regardless of RBAC{masters
            ? ', and this cluster binds it'
            : ''}. Other authorizers (the Node authorizer, webhooks) are not visible from here.
    </p>
</section>

<style>
    .whocan {
        max-width: 980px;
    }

    .question {
        display: flex;
        flex-wrap: wrap;
        align-items: center;
        gap: 8px;
        font-size: 15px;
    }

    .lead {
        font-weight: 600;
    }

    select {
        font: inherit;
        font-size: 13px;
        color: var(--text);
        background: var(--bg);
        border: 1px solid var(--border);
        border-radius: var(--radius-sm);
        padding: 4px 6px;
    }

    input[type='text'] {
        width: 280px;
        font-family: var(--mono);
        font-size: 12.5px;
    }

    .quick {
        display: flex;
        flex-wrap: wrap;
        align-items: center;
        gap: 6px;
        margin: 12px 0 6px;
        font-size: 12px;
    }

    .chip {
        padding: 2px 9px;
        border: 1px solid var(--border);
        border-radius: 11px;
        color: var(--text-dim);
        font-size: 11.5px;
    }

    .chip:hover,
    .chip.on {
        border-color: var(--accent);
        color: var(--text);
    }

    .hint {
        margin: 4px 0 14px;
        color: var(--text-faint);
        font-size: 11.5px;
    }

    code {
        font-family: var(--mono);
        font-size: 11.5px;
    }

    .none {
        display: flex;
        align-items: center;
        gap: 8px;
        padding: 12px 14px;
        border: 1px dashed var(--border);
        border-radius: var(--radius);
        color: var(--text-dim);
    }

    .count {
        margin: 0 0 8px;
        color: var(--text-dim);
    }

    .answers {
        list-style: none;
        margin: 0;
        padding: 0;
        display: flex;
        flex-direction: column;
        gap: 8px;
    }

    .answers > li {
        padding: 9px 12px;
        border: 1px solid var(--border);
        border-radius: var(--radius);
        background: var(--bg-panel);
    }

    .who {
        display: flex;
        align-items: center;
        gap: 7px;
        font-size: 13px;
    }

    .explain {
        margin: 3px 0 0 20px;
        font-size: 11.5px;
        color: var(--text-faint);
    }

    .routes {
        list-style: none;
        margin: 6px 0 0 20px;
        padding: 0;
        font-size: 11.5px;
        color: var(--text-faint);
        display: flex;
        flex-direction: column;
        gap: 3px;
    }

    .routes li {
        display: flex;
        flex-wrap: wrap;
        align-items: baseline;
        gap: 5px;
    }

    .scope {
        color: var(--text-dim);
    }

    .rule {
        color: var(--text-faint);
        font-style: italic;
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
        font-size: 11.5px;
    }

    .you {
        padding: 0 5px;
        border-radius: 8px;
        background: var(--ok);
        color: var(--bg);
        font-size: 10px;
        font-weight: 600;
    }

    .limited {
        padding: 0 6px;
        border-radius: 8px;
        border: 1px solid var(--accent);
        color: var(--accent);
        font-size: 10.5px;
    }

    .caveat {
        margin: 16px 0 0;
        font-size: 11.5px;
        color: var(--text-faint);
        line-height: 1.5;
    }
</style>
