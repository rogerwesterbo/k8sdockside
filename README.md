# k8sdockside

A desktop workspace for the Kubernetes clusters in your local kubeconfig files,
built with [Wails v3](https://v3.wails.io/) (Go) and Svelte 5.

It finds the kubeconfigs on your machine, lists their contexts in a sidebar, and
opens each resource view as a tab. Every context carries a name and a colour you
choose, and its tabs are painted in that colour — so you always know which
cluster you are looking at.

## What it does

- **Finds kubeconfigs on disk.** `~/.kube/config`, every path in `$KUBECONFIG`,
  anything else in `~/.kube` that parses as a kubeconfig, plus files and folders
  you add yourself. **Sync** rescans; a file that fails to parse is shown with
  the reason rather than silently dropped.
- **Point it at a folder.** Watched folders are scanned one level deep and the
  name is ignored entirely: every regular file is opened, and whatever turns out
  to be text, then YAML, then a kubeconfig with contexts is taken. That is how a
  directory of `cluster-a`, `kubeconfig-prod` and `staging.config` loads without
  any of them being named the way a glob would expect. The folder is remembered,
  so a config dropped in later appears on the next sync.
- **Remove any of them.** The × on a file means "I do not want this", whatever
  put it there. One you added by hand is forgotten; one discovery found is
  recorded as hidden instead, because forgetting it would only mean finding it
  again on the next sync. Hidden files are listed at the foot of the sidebar
  with a button to bring each one back.
- **Names and colours each context.** Select a context and the panel at the
  bottom of the sidebar renames it and picks its colour. Both are yours alone —
  the kubeconfig file is never modified.
- **Tabs, coloured by context and sortable.** Choose a view under a context and
  it opens as a tab: filled with that context's colour when active, tinted when
  not. Drag tabs to reorder them (or Alt+Left/Right from the keyboard). The
  order is remembered and the tabs reopen next launch.
- **A describe panel that slides in.** Click a row and its `describe` report
  slides in from the edge. Dock it right, bottom or left, and drag its edge to
  resize; both are remembered.
- **Edit an object as YAML.** The describe panel has an **Edit** button, which
  opens that object in the dock at the foot of the window: the live YAML, with
  line numbers (a setting, on by default), checked as you type, and a **Save**
  that writes it back with `⌘S`. A save that the cluster refuses -- a conflict,
  an admission webhook, a missing permission -- is reported in the API server's
  own words, and what you typed stays where it is.
- **A shell in a container, or on a node.** The detail panel's **Shell** opens
  a real terminal in the dock beside the logs and the editor -- resizable, with
  a container picker and its own scrollback -- or, if you would rather, in the
  terminal emulator you already use, with your font and your tmux. Which shell
  it tries is a list rather than a guess (`bash`, then `sh`), because a
  container image is free to have either, both or neither. A node has no exec
  to open, so **Shell** on one creates the same privileged pod `kubectl debug`
  would, chrooted into the machine, and deletes it when the terminal closes.
  See **Settings → Terminal**.
- **Port forwarding, listed where it can be stopped.** **Forward** on a pod or
  a service asks three things a button cannot guess: which port, which local
  port -- blank means any free one -- and whether to open a browser on it. A
  Service resolves through to a pod behind it and to the port on that pod its
  service port lands on, which is rarely the same number. Every forward appears
  under **Network** in the sidebar with the port it is listening on and a button
  that disconnects it, and in a **Port Forwards** tab that reconnects and
  removes them. They are remembered between sessions as requests rather than as
  connections: nothing dials a cluster at launch.
- **Themes, and themes you add yourself.** Thirteen palettes ship with the app —
  from `K8s Dockside Dark` through `Deep Sea`, `Driftwood` and `Lighthouse` to
  ports of Nord and Catppuccin Mocha — chosen from a gallery in
  **Settings → Themes** that draws each one as a miniature of the app rather
  than a row of swatches. A theme is a JSON file of colours and nothing else:
  drop one in the themes folder, or point the app at a folder of them, and it
  appears alongside the built-ins. It cannot ship CSS or run code, and anything
  it leaves out is inherited, so a four-colour theme is a real one. See
  [docs/themes.md](docs/themes.md).
- **Plugins for what is installed in the cluster.** Argo CD, Flux and Prometheus
  ship with the app and appear under a **Solutions** section in the sidebar, each
  unfolding into its own views — Applications, Kustomizations, Service Monitors —
  instead of leaving those custom resources scattered through the definitions
  tree under group names. Each has an overview saying whether this cluster
  actually has it, which of the kinds it needs are served, and a live count of
  what it manages, worst first. A plugin is a JSON file naming kinds the app
  already knows how to show, so adding one for your own operator is a file rather
  than a fork. See [docs/plugins.md](docs/plugins.md).
- **Graphs, where the cluster can answer for them.** With Prometheus in the
  cluster, a pod's detail panel gains CPU, memory and network over time, a node
  gains its own, and the dashboard gains cluster CPU, memory, pods by phase and
  API server request rate. It is found automatically and reached **through the
  API server** — no port-forward, no second credential — with a per-cluster
  override for Thanos or anything outside. The queries are part of the plugin
  file, so charting your own operator is a few lines of PromQL.
- **A dock that stays put.** The dock's tabs are always on screen and behave
  like the strip above them: coloured by their cluster, dragged into order,
  scrolled when they no longer fit, right-clicked for what to close, and
  reopened next launch. What is open in it survives switching context, opening
  and closing tabs, and closing every tab there is -- an edit you are part way
  through is the one thing that must not disappear because you looked at
  something else. A tab with unsaved changes wears a dot instead of its cross.

## How the cluster data gets here

Resource listings are **live and watched**, not polled. Opening a tab starts a
`client-go` informer against the cluster; the backend pushes the whole current
table to the frontend whenever anything changes, so a rollout repaints as it
happens and nothing has a refresh button.

Some details worth knowing:

- **Everything goes through the dynamic client.** Built-in kinds, the Gateway
  API and arbitrary CRDs are all `unstructured` objects on one code path, which
  is what lets a custom resource open in a tab without anything being compiled
  in for it.
- **One watch per kind per context**, shared by every tab looking at it, closed
  when the last of them does. The namespace filter is applied to the informer's
  cache, so changing it repaints without reopening anything.
- **Columns are data.** Most are JSONPath expressions; the few that are computed
  rather than read (a pod's ready count, a workload's condition) have a Go
  function instead. Custom resources supply their own columns from the CRD's
  `additionalPrinterColumns` -- the same source `kubectl get` uses, so the
  tables match the command line.
- **Secrets are redacted before they are cached.** An informer holds the whole
  collection in memory; the tables only ever show how many keys a secret has, so
  the values are dropped on the way in. The editor is the exception, and has to
  be: it reads the object live rather than from the cache, because an editor
  that opened on a redacted copy would save the redaction back over the secret.
- **Editing is a read, a `PUT` and nothing clever.** The document keeps its
  `resourceVersion`, so a save while a controller (or another person) has moved
  the object on is refused as a conflict rather than silently winning it. What
  comes back from the save is what the editor then holds -- with the new
  version, and whatever defaulting and admission control did on the way in --
  so a second save is not arguing with an object that no longer exists.
  Changing `apiVersion`, `kind`, `metadata.name` or `metadata.namespace` is
  refused: the write goes to the URL of the object that was opened, so a rename
  there cannot mean what it looks like it means.
- **A shell and a forward are streams, not requests.** Both upgrade one HTTP
  request into a connection that stays open -- over websockets where the API
  server speaks them, over SPDY where it or something in between does not, which
  is the same pair `kubectl` falls back through. Both ride the connection a tab
  already has. A shell tries each configured shell in turn and takes "no such
  executable" as "try the next one"; while it is doing that, what you type is
  held for whichever attempt is live, because a failed attempt's copier would
  otherwise still be holding the keyboard.
- **Node and dashboard figures are capacity and requests, not usage.** Live
  usage comes from the metrics API, a separate server that is not always
  installed.

Credentials come from `clientcmd`, so client certificates, tokens and `exec`
plugins all work. Note that the kubeconfig is read twice by design: the parser
in [`internal/kube/config.go`](internal/kube/config.go) reads the little needed
to *list* contexts and is forgiving of files it cannot fully understand, while
`clientcmd` reads what is needed to *connect*.

### The frontend tests

`make test-frontend` runs two vitest projects. Application state (`*.test.ts`)
runs in jsdom with the Wails bindings mocked -- it is plain logic over runes and
does not need a browser. Components (`*.browser.test.ts`) run in headless
Chromium through Playwright, because `$effect`, focus and pointer handling only
behave as they do in the app when there is a real browser under them.

The browser project needs Chromium once: `cd frontend && npx playwright install
chromium`.

### Running the Go tests against a real cluster

Most of the package is tested against literals. The live tests are opt-in,
because they need a cluster:

```sh
K8SDOCKSIDE_TEST_KUBECONFIG=~/kubeconfig/example.config \
K8SDOCKSIDE_TEST_CONTEXT=admin@example \
  go test ./internal/kube/ -run Live -v
```

The exec and port-forward checks need something to open, and skip without it.
Both go through a protocol upgrade that no unit test can stand in for:

```sh
K8SDOCKSIDE_TEST_KUBECONFIG=~/.kube/config \
K8SDOCKSIDE_TEST_CONTEXT=admin@example \
K8SDOCKSIDE_TEST_POD=argocd/argocd-redis-5b965dbf67-wnk4x \
K8SDOCKSIDE_TEST_SERVICE=argocd/argocd-server \
  go test ./internal/kube/ -run Live -v
```

## Requirements

- Go 1.27+ and Node 20+
- The [Wails v3 CLI](https://v3.wails.io/getting-started/installation/):
  `go install github.com/wailsapp/wails/v3/cmd/wails3@latest`
- **Linux:** GTK 4 and WebKitGTK 6 development packages. Without them neither
  the app nor the `wails3` CLI will compile:
  - Arch: `sudo pacman -S gtk4 webkitgtk-6.0`
  - Debian/Ubuntu: `sudo apt install libgtk-4-dev libwebkitgtk-6.0-dev`

## Running

```sh
make dev             # hot-reloading desktop app
make build           # production build into bin/
make test            # Go unit tests with a coverage report
make test-frontend   # Svelte tests (vitest)
make lint-frontend   # svelte-check
make help            # every target
```

There is also a headless mode that runs the app as a plain HTTP server with no
native GUI dependencies, which is handy for poking at the backend:

```sh
CGO_ENABLED=0 go build -tags server -o bin/k8sdockside-server .
WAILS_SERVER_PORT=9741 ./bin/k8sdockside-server
```

## Layout

```
main.go                      window, and the services the frontend calls
kubeconfigservice.go         discovery, add/remove, the context cache
settingsservice.go           aliases, colours, tab order, layout
resourceservice.go           dashboard, resource tables, describe, editing
themeservice.go              the theme catalogue, and the folders it is read from
pluginservice.go             the solution plugins, and each one's per-cluster overview
metricsservice.go            finding a cluster's Prometheus, and drawing plugin charts
terminalservice.go           shells: the sessions open, and the external terminals
portforwardservice.go        the tunnels open, and the ones remembered from last time
internal/kube/               kubeconfig parsing, and the stubbed cluster data
internal/appconfig/          the settings file
internal/addons/             finding and merging add-on files, shared by the two below
internal/themes/             the theme format, loader and built-in palettes
internal/themes/builtin/     the thirteen themes, as JSON in the public format
internal/plugins/            the plugin format, loader and overview builder
internal/plugins/builtin/    Argo CD, Flux and Prometheus, in the public format
internal/metrics/            PromQL, Prometheus discovery, and reading its answers
internal/termapp/            the terminal emulators on this machine, and how to run one in it
frontend/src/
  App.svelte                 shell: sidebar | tabs + view | detail panel | dock
  lib/state/workspace.svelte.ts   all application state
  lib/state/editor.svelte.ts      the documents open in the dock's editor
  lib/state/terminals.svelte.ts   the shells open in the dock, and their xterm instances
  lib/state/forwards.svelte.ts    the port forwards, live and remembered
  lib/theme/apply.ts         writing a theme's colours onto the document
  lib/plugins/               the plugin catalogue as the sidebar and overview see it
  lib/charts/                the SVG line chart, and the panel that hosts them
  lib/catalogue.ts           the resource kinds the sidebar offers
  lib/colors.ts              the context palette
  lib/components/            sidebar, tab bar, tables, detail panel, dock
```

Your settings live in `$XDG_CONFIG_HOME/k8sdockside/settings.json`, falling
back to `~/.config/k8sdockside/settings.json`, on both macOS and Linux; Windows
uses `%AppData%`. The path is shown in the status bar. Themes you install go in
`themes/` and `plugins/` folders beside it — see [docs/themes.md](docs/themes.md)
and [docs/plugins.md](docs/plugins.md).

Releases up to and including the ones that used `os.UserConfigDir` wrote to
`~/Library/Application Support/k8sdockside/` on macOS. That file is moved to
the new location automatically on first launch, unless one is already there.

### A note on the generated bindings

`frontend/bindings/` is produced by `wails3 generate bindings` (`make
generate`). Rerun it after changing a service's methods or any type they
mention.

Two things about the generated types are worth knowing, and both are handled in
one place — `frontend/src/lib/state/adopt.ts`, which every service result passes
through on its way into the app:

- Every Go slice and map is typed as nullable, because a nil one serialises to
  JSON `null`.
- Depending on the generator flags, models arrive as *classes* rather than plain
  objects, and Svelte's `$state` does not deep-proxy class instances — so a
  nested write to one would never reach the UI.

`make generate` and the binding step inside `wails3 build` must be given the
same flags, or each will quietly undo the other's output.
