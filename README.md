# k8sdockside

A desktop workspace for the Kubernetes clusters in your local kubeconfig files,
built with [Wails v3](https://v3.wails.io/) (Go) and Svelte 5.

It finds the kubeconfigs on your machine, lists their contexts in a sidebar, and
opens each resource view as a tab. Every context carries a name and a colour you
choose, and its tabs are painted in that colour — so you always know which
cluster you are looking at.

## What it does

- **Finds kubeconfigs on disk.** `~/.kube/config`, every path in `$KUBECONFIG`,
  anything else in `~/.kube` that parses as a kubeconfig, plus files you add
  yourself. **Sync** rescans; a file that fails to parse is shown with the
  reason rather than silently dropped.
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

## Status: the cluster data is stubbed

The kubeconfig reading is real. The resource listings behind each tab are not
yet: they are fabricated in [`internal/kube/stub.go`](internal/kube/stub.go),
deterministically per context, so the whole interface can be built and used
before a live API client exists.

The fabricated data is coherent — pods belong to workloads that exist, run on
nodes that exist, and events reference real pods — and it is shaped exactly like
the real thing. Wiring in `client-go` means replacing the bodies in `stub.go`,
`tables.go` and `overview.go`. The types in `internal/kube/resources.go`, the
service signatures and the entire frontend stay as they are.

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
make dev      # hot-reloading desktop app
make build    # production build into bin/
make test     # Go unit tests with a coverage report
make lint-frontend   # svelte-check
make help     # every target
```

There is also a headless mode that runs the app as a plain HTTP server with no
native GUI dependencies, which is handy for poking at the backend:

```sh
CGO_ENABLED=0 go build -tags server -o bin/k8sdockside-server .
WAILS_SERVER_PORT=9741 ./bin/k8sdockside-server
```

## Layout

```
main.go                      window, and the three services the frontend calls
kubeconfigservice.go         discovery, add/remove, the context cache
settingsservice.go           aliases, colours, tab order, layout
resourceservice.go           dashboard, resource tables, describe
internal/kube/               kubeconfig parsing, and the stubbed cluster data
internal/appconfig/          the settings file
frontend/src/
  App.svelte                 shell: sidebar | tabs + view | detail panel
  lib/state/workspace.svelte.ts   all application state
  lib/catalogue.ts           the resource kinds the sidebar offers
  lib/colors.ts              the context palette
  lib/components/            sidebar, tab bar, tables, detail panel
```

Your settings live in `~/.config/k8sdockside/settings.json` (the path is shown
in the status bar).

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
