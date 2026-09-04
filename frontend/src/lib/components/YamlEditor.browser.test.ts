import { beforeEach, expect, test, vi } from 'vitest';
import { page } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';
import YamlEditor from './YamlEditor.svelte';

// Declared through vi.hoisted because the mock factory below is lifted above
// every other statement in the file, and it needs this string.
const { LIVE } = vi.hoisted(() => ({ LIVE: 'apiVersion: v1\nkind: Pod\nmetadata:\n  name: web\n' }));

vi.mock('../../../bindings/github.com/roger/k8sdockside', () => ({
    KubeconfigService: { Sync: vi.fn().mockResolvedValue([]), Files: vi.fn().mockResolvedValue([]) },
    ResourceService: {
        Describe: vi.fn().mockResolvedValue(''),
        ResourceYAML: vi.fn().mockResolvedValue(LIVE),
        ApplyYAML: vi.fn().mockResolvedValue(LIVE),
        CheckYAML: vi.fn().mockResolvedValue({ valid: true, message: '', line: 0 }),
    },
    SettingsService: {
        Get: vi.fn().mockResolvedValue({}),
        ConfigPath: vi.fn().mockResolvedValue(''),
        SetContextPrefs: vi.fn().mockResolvedValue({}),
        SetTabOrder: vi.fn().mockResolvedValue({}),
        SetDock: vi.fn().mockResolvedValue({}),
        SetLayout: vi.fn().mockResolvedValue({}),
        SetPreferences: vi.fn().mockResolvedValue({}),
    },
}));

const { workspace } = await import('../state/workspace.svelte');
const { editors } = await import('../state/editor.svelte');
const { ResourceService } = await import('../../../bindings/github.com/roger/k8sdockside');

const PROD = '/home/u/.kube/prod::admin@prod';

/** One editor tab, as the dock hands it to the component. */
const TAB = {
    id: 'edit:prod#pods#default#web',
    view: 'edit' as const,
    contextId: PROD,
    kind: 'pods',
    namespace: 'default',
    name: 'web',
    title: 'web',
};

/** The gutter's numbers, or an empty list when it is not drawn. */
function gutter(): string[] {
    return [...document.querySelectorAll('.gutter span')].map((el) => el.textContent ?? '');
}

beforeEach(() => {
    editors.forget(TAB.id);
    workspace.settings.preferences.showLineNumbers = true;
    vi.mocked(ResourceService.ResourceYAML).mockReset().mockResolvedValue(LIVE);
    vi.mocked(ResourceService.ApplyYAML).mockReset().mockResolvedValue(LIVE);
    vi.mocked(ResourceService.CheckYAML)
        .mockReset()
        .mockResolvedValue({ valid: true, message: '', line: 0 });
});

test('opens on the object as the cluster has it', async () => {
    render(YamlEditor, { tab: TAB });

    await expect.element(page.getByRole('textbox', { name: 'web as YAML' })).toHaveValue(LIVE);
    expect(ResourceService.ResourceYAML).toHaveBeenCalledWith(PROD, 'pods', 'default', 'web');
});

test('numbers every line, and stops when the setting is turned off', async () => {
    render(YamlEditor, { tab: TAB });
    await expect.element(page.getByRole('textbox')).toHaveValue(LIVE);

    // Four lines of YAML and the empty one after the trailing newline.
    expect(gutter()).toEqual(['1', '2', '3', '4', '5']);

    workspace.settings.preferences.showLineNumbers = false;
    await expect.poll(() => gutter()).toEqual([]);
});

test('there is nothing to save until something is typed', async () => {
    render(YamlEditor, { tab: TAB });
    const save = page.getByRole('button', { name: 'Save' });

    await expect.element(save).toBeDisabled();

    await page.getByRole('textbox').fill('apiVersion: v1\nkind: Pod\n');
    await expect.element(save).toBeEnabled();
});

test('broken YAML says where it broke and holds the save back', async () => {
    vi.mocked(ResourceService.CheckYAML).mockResolvedValue({
        valid: false,
        message: 'mapping values are not allowed in this context',
        line: 2,
    });
    render(YamlEditor, { tab: TAB });
    await expect.element(page.getByRole('textbox')).toHaveValue(LIVE);

    await page.getByRole('textbox').fill('a: b\nc: d: e\n');

    await expect.element(page.getByText(/Line 2:/)).toBeVisible();
    await expect.element(page.getByRole('button', { name: 'Save' })).toBeDisabled();
    // The line is marked in the gutter as well, so the message has somewhere
    // to point rather than only naming a number.
    await expect.poll(() => document.querySelector('.gutter span.bad')?.textContent).toBe('2');
});

test('saving sends what is in the editor and takes back what was stored', async () => {
    const stored = `${LIVE}  resourceVersion: "4822"\n`;
    vi.mocked(ResourceService.ApplyYAML).mockResolvedValue(stored);
    render(YamlEditor, { tab: TAB });
    await expect.element(page.getByRole('textbox')).toHaveValue(LIVE);

    await page.getByRole('textbox').fill('apiVersion: v1\nkind: Pod\n');
    await page.getByRole('button', { name: 'Save' }).click();

    await expect.element(page.getByRole('textbox')).toHaveValue(stored);
    expect(ResourceService.ApplyYAML).toHaveBeenCalledWith(
        PROD,
        'pods',
        'default',
        'web',
        'apiVersion: v1\nkind: Pod\n',
    );
    await expect.element(page.getByText('Saved')).toBeVisible();
});

test('a refused save is reported in the API server\u2019s own words', async () => {
    vi.mocked(ResourceService.ApplyYAML).mockRejectedValue(
        new Error('Operation cannot be fulfilled on pods "web": the object has been modified'),
    );
    render(YamlEditor, { tab: TAB });
    await expect.element(page.getByRole('textbox')).toHaveValue(LIVE);

    await page.getByRole('textbox').fill('apiVersion: v1\nkind: Pod\n');
    await page.getByRole('button', { name: 'Save' }).click();

    await expect.element(page.getByText(/the object has been modified/)).toBeVisible();
    // The edit is still the only copy of what was written; it must survive.
    await expect.element(page.getByRole('textbox')).toHaveValue('apiVersion: v1\nkind: Pod\n');
});

test('a cluster that will not answer offers a retry rather than an empty editor', async () => {
    vi.mocked(ResourceService.ResourceYAML).mockRejectedValueOnce(new Error('connection refused'));
    render(YamlEditor, { tab: TAB });

    await expect.element(page.getByRole('button', { name: /Try again|Retry/ })).toBeVisible();
});
