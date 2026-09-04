import { beforeEach, expect, test, vi } from 'vitest';
import { page } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';
import SettingsView from './SettingsView.svelte';

// The About section asks the backend what this build is the moment it mounts,
// and the sources section reads the file list; neither is what is under test.
vi.mock('../../../../bindings/github.com/roger/k8sdockside', () => ({
    KubeconfigService: { Sync: vi.fn().mockResolvedValue([]), Files: vi.fn().mockResolvedValue([]) },
    ResourceService: { Describe: vi.fn().mockResolvedValue('') },
    LogService: {
        Containers: vi.fn().mockResolvedValue([]),
        Open: vi.fn().mockResolvedValue('logs-1'),
        Close: vi.fn(),
    },
    SettingsService: {
        Get: vi.fn().mockResolvedValue({}),
        ConfigPath: vi.fn().mockResolvedValue('/home/u/.config/k8sdockside/settings.json'),
        About: vi.fn().mockResolvedValue({ version: 'test', wails: 'test', go: 'test', platform: 'test' }),
        SetTabOrder: vi.fn().mockResolvedValue({}),
        SetLayout: vi.fn().mockResolvedValue({}),
        SetPreferences: vi.fn().mockResolvedValue({}),
        SetContextPrefs: vi.fn().mockResolvedValue({}),
    },
}));

const TOKENS = `:root{--bg:#10151c;--bg-sidebar:#151b24;--bg-panel:#19202a;--bg-raised:#212b38;
--border:#46536a;--border-soft:rgba(255,255,255,.05);--bg-hover:rgba(255,255,255,.08);
--bg-active:rgba(255,255,255,.13);--text:#e8eef7;--text-dim:#a9b6c6;--text-faint:#8593a3;
--accent:#4a86ff;--ok:#5fd39b;--warn:#efb567;--error:#f4787f;--radius:6px;--radius-sm:4px;
--mono:monospace;--font:-apple-system,sans-serif;font-family:var(--font);font-size:13px}
body{color:var(--text);margin:0;width:900px}
button{font:inherit;color:inherit;background:none;border:none;padding:0;cursor:pointer}`;

const rail = () => page.getByRole('tab');

beforeEach(() => {
    document.body.innerHTML = '';
    const style = document.createElement('style');
    style.textContent = TOKENS;
    document.head.appendChild(style);
});

test('the sections are ordered with the everyday ones first and About last', async () => {
    render(SettingsView);

    const labels = (await rail().all()).map((item) => item.element().textContent?.trim());
    expect(labels).toEqual(['Appearance', 'Behaviour', 'Kubeconfig sources', 'About']);
});

test('it opens on Appearance', async () => {
    render(SettingsView);

    await expect.element(page.getByRole('tab', { name: 'Appearance' })).toHaveAttribute('aria-selected', 'true');
});

// These two run in order on purpose: together they are one scenario that
// cannot be written as a single test, because what is being checked is that
// the choice outlives the component instance that made it.
//
// The shell wraps the active view in {#key activeTab.id}, so switching to a
// cluster tab destroys this component and returning builds a new one. The
// section therefore cannot live in component state -- it used to, and every
// visit landed back on the first section.
test('choosing a section selects it', async () => {
    render(SettingsView);

    await page.getByRole('tab', { name: 'Behaviour' }).click();

    await expect.element(page.getByRole('tab', { name: 'Behaviour' })).toHaveAttribute('aria-selected', 'true');
});

test('...and a freshly mounted view comes back to it, not to the first section', async () => {
    render(SettingsView);

    await expect.element(page.getByRole('tab', { name: 'Behaviour' })).toHaveAttribute('aria-selected', 'true');
    await expect.element(page.getByRole('tab', { name: 'Appearance' })).toHaveAttribute('aria-selected', 'false');
});
