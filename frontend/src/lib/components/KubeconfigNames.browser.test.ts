import { beforeEach, expect, test, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import Sidebar from './Sidebar.svelte';

vi.mock('../../../bindings/github.com/roger/k8sdockside', () => ({
    KubeconfigService: { Sync: vi.fn().mockResolvedValue([]), Files: vi.fn().mockResolvedValue([]) },
    ResourceService: { Describe: vi.fn().mockResolvedValue(''), Ping: vi.fn().mockResolvedValue(undefined) },
    LogService: {
        Containers: vi.fn().mockResolvedValue([]),
        Open: vi.fn().mockResolvedValue('logs-1'),
        Close: vi.fn(),
    },
    SettingsService: {
        Get: vi.fn().mockResolvedValue({}),
        ConfigPath: vi.fn().mockResolvedValue(''),
        SetTabOrder: vi.fn().mockResolvedValue({}),
        SetLayout: vi.fn().mockResolvedValue({}),
        SetPreferences: vi.fn().mockResolvedValue({}),
        SetContextPrefs: vi.fn().mockResolvedValue({}),
    },
}));

const { workspace } = await import('../state/workspace.svelte');

const TOKENS = `:root{--bg:#10151c;--bg-sidebar:#151b24;--bg-panel:#19202a;--bg-raised:#212b38;
--border:#46536a;--border-soft:rgba(255,255,255,.05);--bg-hover:rgba(255,255,255,.08);
--bg-active:rgba(255,255,255,.13);--text:#e8eef7;--text-dim:#a9b6c6;--text-faint:#8593a3;
--accent:#4a86ff;--ok:#5fd39b;--warn:#efb567;--error:#f4787f;--radius:6px;--radius-sm:4px;
--row-h:30px;--mono:monospace;--font:-apple-system,sans-serif;font-family:var(--font);font-size:13px}
body{color:var(--text);margin:0;width:280px}
button{font:inherit;color:inherit;background:none;border:none;padding:0;cursor:pointer}`;

function ctx(file: string, name: string) {
    return { id: `${file}::${name}`, name, cluster: name, user: 'admin',
        namespace: '', server: '', file, current: false };
}

const fileNames = () => [...document.querySelectorAll('.file-name')].map((e) => e.textContent?.trim());

beforeEach(() => {
    document.body.innerHTML = '';
    const style = document.createElement('style');
    style.textContent = TOKENS;
    document.head.appendChild(style);

    workspace.files = [
        { path: '/home/u/.kube/config', source: 'auto', error: '', contexts: [ctx('/home/u/.kube/config', 'prod')] },
        { path: '/home/u/.kube/work.yaml', source: 'manual', error: '', contexts: [ctx('/home/u/.kube/work.yaml', 'dev')] },
    ];
    workspace.settings.preferences.showKubeconfigNames = false;
});

test('the sidebar is one flat list of contexts by default', async () => {
    render(Sidebar);

    expect(fileNames()).toEqual([]);
    // The contexts themselves are untouched -- this hides the grouping, not
    // any cluster.
    expect(document.querySelectorAll('.context').length).toBe(2);
});

test('turning the setting on groups them under their kubeconfig', async () => {
    workspace.settings.preferences.showKubeconfigNames = true;
    render(Sidebar);

    expect(fileNames()).toEqual(['config', 'work.yaml']);
});

// A kubeconfig that will not parse has no contexts to show in its place. If
// hiding names hid it too, it would leave the sidebar with nothing said.
test('a file that could not be read is still named, whatever the setting', async () => {
    workspace.files = [
        ...workspace.files,
        { path: '/home/u/.kube/broken.yaml', source: 'auto', error: 'yaml: line 3: mapping values are not allowed', contexts: [] },
    ];
    render(Sidebar);

    expect(fileNames()).toEqual(['broken.yaml']);
    expect(document.querySelector('.file-error')?.textContent).toContain('mapping values');
});
