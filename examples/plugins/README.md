# Example plugins

Plugins that live outside the app, the way a plugin kept in its own repository
would. See [docs/plugins.md](../../docs/plugins.md) for the format.

## Trying one

**Settings → Plugins → Watch another folder**, and pick this `examples/plugins`
folder. Each plugin here is a subfolder holding a `plugin.json` and, for one with
views of its own, a `ui/` folder beside it.

- **`argocd-ui/`** — Argo CD, with a custom *Application map* view: every
  Application as a card with its health and sync state, unfolding into the
  resources it manages, with Sync and Refresh buttons. Plain HTML and script, no
  build step.
- **`kubevirt-ui/`** — KubeVirt, product-specific the way Headlamp's KubeVirt
  plugin is: Start, Pause/Resume, Restart, Reboot, Migrate and Stop on every
  VirtualMachine's action bar (declared in `plugin.json`, offered only in the
  states they make sense in), and a *Machine* panel in the VM's detail view
  with its own round icon button group. Once it is on, it takes over from the
  app's built-in VM buttons.

## A plugin in its own repository

Lay the repository out like one of these folders:

```
my-plugin/
├── plugin.json       the manifest: requires, views, cards, and "ui"
└── ui/               what the custom views open, served as-is
    ├── index.html
    └── app.js
```

Then install it with **Settings → Plugins → From a repository** (it is cloned
into the plugins folder, and its card gets an *Update from repository* button),
clone it there yourself (it is read one level deep), or add the folder that
contains it with **Watch another folder**. Press **Reload** in
Settings after editing `plugin.json`; files under `ui/` are read fresh every time
a view is opened, so reopening the tab is enough.

## Building a view with a framework

Anything that ends up as static files works — Svelte, Preact, Vue, Lit, plain
DOM. Three things differ from building for a normal web page:

1. **Include the bridge** before your own script. The app serves it:

   ```html
   <script src="/plugin-ui/_sdk/k8sdockside.js"></script>
   ```

2. **Bundle to a classic script, not an ES module.** The view runs in a
   sandboxed frame with an opaque origin, so `<script type="module">` is a
   cross-origin load, which not every webview allows from the app's own scheme.
   With Vite:

   ```js
   // vite.config.js
   export default {
       base: './',
       build: {
           outDir: 'ui',
           modulePreload: false,
           rollupOptions: {
               input: 'index.html',
               output: { format: 'iife', inlineDynamicImports: true, entryFileNames: 'app.js' },
           },
       },
   };
   ```

   and drop `type="module"` / `crossorigin` from the built `index.html` `<script>`
   if Vite leaves them in.

3. **No network.** `fetch`, XHR and websockets are refused by the page's
   Content-Security-Policy; everything about the cluster comes through
   `window.k8sdockside`. Bundle fonts and images, or inline them.

The theme comes for free: the SDK sets the app's colour tokens on your `:root`,
so `var(--bg)`, `var(--text)`, `var(--accent)`, `var(--ok)`, `var(--warn)`,
`var(--error)` and the rest match the app and follow it when the user changes
theme.
