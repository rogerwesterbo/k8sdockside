<!--
  Rules as a table: one row per resource, one column per verb, a tick where the
  rules allow it. The same rules as the YAML, read the way people ask about
  them -- "can this delete pods?" is a glance down one column.
-->
<script lang="ts">
    import { MATRIX_VERBS, VERB_HELP, describeRule, matrix, type Rule } from './model';

    interface Props {
        rules: Rule[];
        /** Also list each rule as a sentence under the table. */
        sentences?: boolean;
        empty?: string;
    }

    let { rules, sentences = false, empty = 'No rules.' }: Props = $props();

    let rows = $derived(matrix(rules));

    const SHORT: Record<string, string> = {
        get: 'get',
        list: 'list',
        watch: 'watch',
        create: 'create',
        update: 'update',
        patch: 'patch',
        delete: 'delete',
        deletecollection: 'del. all',
    };

    function groupLabel(group: string): string {
        if (group === '') return 'core';
        if (group === '*') return 'all groups';
        return group;
    }
</script>

{#if rows.length === 0}
    <p class="empty">{empty}</p>
{:else}
    <div class="frame">
        <table>
            <thead>
                <tr>
                    <th class="res">Resource</th>
                    {#each MATRIX_VERBS as verb (verb)}
                        <th class="verb" title={VERB_HELP[verb]}>{SHORT[verb]}</th>
                    {/each}
                    <th class="other">Other</th>
                </tr>
            </thead>
            <tbody>
                {#each rows as row (`${row.url}|${row.group}|${row.resource}`)}
                    <tr class:wild={row.resource === '*'}>
                        <td class="res">
                            <span class="name selectable">{row.resource === '*' ? 'everything' : row.resource}</span>
                            {#if !row.url}
                                <span class="group">{groupLabel(row.group)}</span>
                            {:else}
                                <span class="group">URL</span>
                            {/if}
                            {#if row.names.length}
                                <span class="names" title="Only these objects: {row.names.join(', ')}">
                                    only {row.names.length === 1 ? row.names[0] : `${row.names.length} named`}
                                </span>
                            {/if}
                        </td>
                        {#each MATRIX_VERBS as verb (verb)}
                            {@const cell = row.cells[verb] ?? ''}
                            <td
                                class="cell {cell}"
                                title={cell === 'yes'
                                    ? `${VERB_HELP[verb]}: allowed`
                                    : cell === 'some'
                                      ? `${VERB_HELP[verb]}: only ${row.names.join(', ')}`
                                      : `${VERB_HELP[verb]}: not allowed`}
                            >
                                {cell === 'yes' ? '✓' : cell === 'some' ? '◐' : ''}
                            </td>
                        {/each}
                        <td class="other">
                            {#each row.other as verb (verb)}
                                <span
                                    class="pill"
                                    class:danger={['escalate', 'bind', 'impersonate', '*'].includes(verb)}
                                    title={VERB_HELP[verb] ?? verb}>{verb === '*' ? 'all verbs' : verb}</span
                                >
                            {/each}
                        </td>
                    </tr>
                {/each}
            </tbody>
        </table>
    </div>
{/if}

{#if sentences && rules.length}
    <ul class="sentences">
        {#each rules as rule, i (i)}
            <li>{describeRule(rule)}</li>
        {/each}
    </ul>
{/if}

<style>
    .empty {
        color: var(--text-faint);
        font-size: 12px;
        margin: 4px 0;
    }

    .frame {
        overflow: auto;
        border: 1px solid var(--border);
        border-radius: var(--radius-sm);
    }

    table {
        border-collapse: collapse;
        width: 100%;
        font-size: 11.5px;
    }

    th {
        position: sticky;
        top: 0;
        background: var(--bg-raised);
        color: var(--text-faint);
        font-weight: 600;
        text-align: center;
        padding: 5px 4px;
        border-bottom: 1px solid var(--border);
        white-space: nowrap;
        cursor: help;
    }

    th.res {
        text-align: left;
        padding-left: 8px;
        cursor: default;
    }

    td {
        padding: 3px 4px;
        border-bottom: 1px solid var(--border-soft);
    }

    tr:last-child td {
        border-bottom: none;
    }

    td.res {
        padding-left: 8px;
        white-space: nowrap;
    }

    .name {
        font-family: var(--mono);
        color: var(--text);
    }

    tr.wild .name {
        color: var(--warn);
        font-weight: 600;
    }

    .group {
        margin-left: 6px;
        color: var(--text-faint);
        font-size: 10.5px;
    }

    .names {
        margin-left: 6px;
        color: var(--accent);
        font-size: 10.5px;
    }

    .cell {
        text-align: center;
        width: 46px;
        color: var(--text-faint);
    }

    .cell.yes {
        color: var(--ok);
        background: color-mix(in srgb, var(--ok) 10%, transparent);
        font-weight: 600;
    }

    .cell.some {
        color: var(--accent);
    }

    td.other {
        white-space: nowrap;
    }

    .pill {
        display: inline-block;
        padding: 0 5px;
        margin-right: 3px;
        border-radius: 8px;
        border: 1px solid var(--border);
        color: var(--text-dim);
        font-size: 10.5px;
    }

    .pill.danger {
        border-color: var(--error);
        color: var(--error);
    }

    .sentences {
        margin: 8px 0 0;
        padding-left: 18px;
        font-size: 12px;
        color: var(--text-dim);
        line-height: 1.6;
    }
</style>
