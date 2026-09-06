# Plugins

A solution plugin gives something installed *in your clusters* — Argo CD, Flux,
Prometheus — a place of its own in the sidebar, instead of leaving its custom
resources scattered through the Custom Resource Definitions tree under group
names like `kustomize.toolkit.fluxcd.io`.

Three ship with the app. Anything else is a JSON file you drop in a folder.

Like a theme, a plugin is data and nothing else: it names resource kinds the app
already knows how to list and says how to arrange and summarise them. It cannot
ship code, CSS or queries. That is the limit that makes installing a stranger's
plugin about as risky as installing their wallpaper, and it is why a plugin
written today keeps working as the app grows.

## The distinction everything here turns on

**A plugin is installed on your machine. The solution it describes is installed
in a cluster.** Those come apart constantly — you keep the Argo CD plugin and
open a cluster that has never heard of it — so the app never conflates them:

- The sidebar shows every installed plugin under **Plugins**, for every
  cluster, and marks the ones this cluster does not appear to have as *not
  installed*. It does not hide them: you installed the plugin, and a row
  vanishing without explanation is worse than a row that says why it is quiet.
- Each plugin's **Overview** is where that gets explained properly — which of
  the kinds it needs this cluster serves, and which it does not.
- Its other views still open. They report `this cluster does not serve
  applications.argoproj.io — the argoproj.io API is not installed`, which is the
  ordinary answer for an optional API, not a failure.

## Installing one

Put the `.json` file in the plugins folder:

| Platform | Folder |
| --- | --- |
| Linux, macOS | `$XDG_CONFIG_HOME/k8sdockside/plugins/`, falling back to `~/.config/k8sdockside/plugins/` |
| Windows | `%AppData%\k8sdockside\plugins\` |

**Settings → Plugins** shows the exact path, with a button to open it. Files are
read from that folder and one level into any subfolder, so a pack cloned or
unzipped into a directory of its own works as it is. You can also point the app
at folders elsewhere with **Watch another folder**. Plugins are read at launch
and whenever you press **Reload**.

This is the same folder layout, the same starter-file button and the same
"would not load" reporting as [themes](themes.md), on purpose.

## Writing one

Start with **Settings → Plugins → Write a starter plugin**, which drops a
working file you can edit a line at a time.

```json
{
    "id": "acme",
    "name": "Acme Mesh",
    "tagline": "service mesh",
    "icon": "share",
    "docs": "https://example.com/acme",
    "description": "One or two sentences on what this is and what to look at first.",
    "requires": [
        { "kind": "crd:meshes.acme.io", "label": "Meshes" },
        { "kind": "crd:sidecars.acme.io", "label": "Sidecars", "optional": true }
    ],
    "views": [
        { "id": "meshes", "label": "Meshes", "icon": "share", "kind": "crd:meshes.acme.io" },
        {
            "id": "control-plane",
            "label": "Control plane",
            "icon": "server",
            "kind": "deployments",
            "namespace": "acme-system",
            "selector": "app.kubernetes.io/part-of=acme"
        }
    ],
    "cards": [
        {
            "label": "Meshes",
            "kind": "crd:meshes.acme.io",
            "groupBy": "status.conditions[Ready]",
            "tones": { "True": "ok", "False": "error", "Unknown": "warn" }
        }
    ]
}
```

| Field | | |
| --- | --- | --- |
| `id` | required | Lowercase letters, digits and dashes. It appears in every tab's identity, so changing it later loses those tabs from a saved session. |
| `name` | required | What the sidebar calls it. |
| `tagline` | optional | One short line, shown under the name on the overview. |
| `description` | optional | A paragraph on the overview. Worth writing: it is where you say what to look at first. |
| `icon` | optional | See [icons](#icons). Defaults to `puzzle`. |
| `docs` | optional | A link on the overview. `http(s)` only. |
| `author` | optional | Yours. |
| `requires` | optional | The kinds the overview checks this cluster for. |
| `views` | required¹ | The rows under the plugin in the sidebar. |
| `cards` | optional¹ | The live counts on the overview. |
| `charts` | optional¹ | Time-series graphs from the cluster's Prometheus. See [Charts](#charts). |

¹ A plugin needs at least one of `views`, `cards` or `charts` — otherwise there
would be nothing to show.

### Kinds

Everywhere a `kind` appears it names something the app can already open:

- a built-in name — `pods`, `deployments`, `services`, `configmaps`, … (the
  sidebar's own kinds, listed in `frontend/src/lib/catalogue.ts`)
- `crd:<plural>.<group>` for a custom resource — `crd:applications.argoproj.io`

A kind that is neither is refused when the file is read, and named under **would
not load**, rather than becoming a sidebar row that opens onto an error.

### Views

A view is one row under the plugin and one tab when clicked.

| Field | | |
| --- | --- | --- |
| `id` | required | Lowercase letters, digits and dashes, unique within the plugin. It is what a restored tab is found by, so renaming one loses its place in a saved session. `overview` is reserved. |
| `label` | required | The row's text. |
| `kind` | required | What it lists. |
| `icon` | optional | Defaults to `puzzle`. |
| `namespace` | optional | Pins the view to one namespace. The tab's namespace picker is replaced with a note saying where you are — a view called "Argo CD's own workloads" is not answering a question about `kube-system`. |
| `selector` | optional | A label selector in the usual `a=b,c in (d,e)` syntax. It is what lets a plugin offer a view of a *built-in* kind — the Deployments that are Argo CD's — rather than only of custom resources nothing else owns. |

A malformed selector or namespace is caught when the file is read, not when the
tab is opened.

**Every plugin gets an Overview view whether or not it declares one**, always
first. It is where "is this even in this cluster?" is answered, which matters
most exactly when the CRDs are missing and every other row would open onto the
same error.

### Requirements

`requires` is what the overview checks the cluster for, and what decides whether
the sidebar row reads as installed.

A plugin counts as installed when **every non-optional** requirement is served.
That is deliberately not "any of them": a cluster serving one Argo CD CRD out of
three is a broken install, not a working one, and saying "installed" would send
the reader looking in the wrong place.

Mark the extras `"optional": true`. Argo CD without ApplicationSets is still
Argo CD, and an absent optional requirement is shown greyed rather than as a red
cross.

### Cards

A card is one tile on the overview: how many of a kind there are, optionally
divided by one of their own fields. A divided count is drawn as a ring with the
total in the middle and one slice per value, coloured by its tone, with the
values listed beside it. A card's title opens the view that lists the same
kind, when the plugin has one.

| Field | | |
| --- | --- | --- |
| `label` | required | The tile's heading. |
| `kind` | required | What to count. |
| `groupBy` | optional | The field to divide by. Omit for a plain count. |
| `tones` | optional | Maps a field value to `ok`, `warn`, `error` or `info`. A value with no entry is drawn plainly, so only name the ones that mean something. |
| `namespace`, `selector` | optional | Narrow what is counted, exactly as on a view. |

**Field paths** come in two shapes, and only two:

```
status.health.status        a plain dotted path
status.conditions[Ready]    the status of the condition of that type
```

The second exists because conditions are the near-universal Kubernetes idiom and
a dotted path cannot reach into a list. It is not a query language: a plugin file
comes from outside the app, and "an address" is a much smaller thing to accept
than "an expression". A path that is not one of these two shapes is refused when
the file is read.

Objects whose field is absent are counted in their own bucket, shown as *no
status yet* — a resource that has not been reconciled has no status at all, and
that is worth seeing rather than rounding to zero.

Buckets are ordered **worst first**, not by count: one Degraded application among
forty Healthy ones is the whole reason the tile is on screen, and sorting by size
would bury it.

## Charts

A card counts what the API server can tell you. A **chart** draws what it cannot:
CPU over the last hour, memory climbing towards a limit, requests by response
code. Those come from the cluster's Prometheus.

```json
"charts": [
    {
        "id": "cpu",
        "label": "CPU",
        "attach": "pods",
        "unit": "cores",
        "legend": "container",
        "description": "Cores used per container, averaged over five minutes.",
        "query": "sum by (container) (rate(container_cpu_usage_seconds_total{namespace=\"$namespace\",pod=\"$name\",container!=\"\"}[5m]))"
    }
]
```

| Field | | |
| --- | --- | --- |
| `id` | required | Lowercase letters, digits and dashes, unique within the plugin. |
| `label` | required | The chart's title. |
| `attach` | required | Where it is drawn — see below. |
| `query` | required | PromQL. |
| `unit` | optional | `cores`, `bytes`, `bytes/s`, `percent`, `ops/s`, `seconds`, `count`, or omitted for a plain number. It decides only how values are written — 512 MiB rather than 536870912. |
| `legend` | optional | The Prometheus label each series is named by. Omitted, a query returning several series names them by their whole label set. |
| `description` | optional | A sentence behind the ⓘ next to the title. Worth writing: what a query actually measures is rarely obvious from a word like "CPU". |

### Where a chart is drawn

`attach` takes one of:

| Value | Where it appears | Variables |
| --- | --- | --- |
| a kind — `pods`, `nodes`, `crd:…` | the detail panel of any object of that kind | `$namespace`, `$name`, `$node` |
| `dashboard` | the cluster's own dashboard tab | none |
| `overview` | the plugin's own overview | none |

The three variables are the only things interpolable into a query, and a chart
attached to `dashboard` or `overview` may not use them — there is no object to
name, so the query would always come out empty. Both rules are checked when the
file is read.

`$node` and `$name` are the same value on a node's own charts, so either reads
correctly there.

### What is and is not allowed in a query

The query goes to Prometheus **as written**. That is a different thing from the
field paths a card uses: a field path is an expression *this app* evaluates, so
it is kept to two shapes it can evaluate safely, while PromQL is passed through
untouched to a server that exists to answer it. There is nothing to sandbox.

What *is* policed is the substitution. A value lands inside a label matcher, so
it is checked against what a Kubernetes name can contain before it goes in —
anything with a quote, a brace or a newline in it is refused rather than escaped.
A query referring to a variable that does not exist is refused when the file is
read, rather than left in for Prometheus to read the `$` as an operator.

A plugin also cannot choose *where* a query goes. The endpoint is always the one
resolved for the current context, so a plugin cannot point queries at somewhere
else.

### Where the data comes from

The app looks for a Prometheus in the cluster and reaches it **through the API
server's service proxy** — the same API server the kubeconfig already
authenticates against. That means no port-forward, no second credential, and it
works wherever the kubeconfig works, including through a bastion that only
exposes the API server. It needs the `services/proxy` permission, which most
read-only roles include.

Discovery looks for a Service labelled `app.kubernetes.io/name=prometheus` first,
then `app=prometheus`, then a short list of well-known names — and within each,
for a port named `web`, `http-web`, `http` or `api`, or any port numbered 9090.
The label is a much stronger signal than a name, which is why it is checked
first: a cluster easily has several things called prometheus-something.

When that finds the wrong thing or nothing, set the address on the context
itself, in the sidebar's cluster settings panel:

```
monitoring/prometheus-operated:9090     through the API server's service proxy
https://thanos.example.com              straight at the address
```

The second form sends **no credentials** — it is an address somebody typed into a
settings field, and quietly presenting the cluster's credentials to it would be a
way to leak them somewhere the kubeconfig never pointed. A Prometheus behind auth
wants a proxy in front of it, or the service form.

### Reading a chart

- Every chart on a page shares one time-range control. That is deliberate: charts
  with different windows cannot be compared, and comparing them is why they are
  next to each other.
- Series colours are `--chart-1` … `--chart-8`, assigned in order and never
  cycled. The order is what keeps neighbouring lines apart under protanopia and
  deuteranopia, so a theme may restep those tokens but should not reorder them. A
  ninth series folds into the eighth rather than being given an invented hue.
- Values are written out — in the legend, at the line's end, and in the crosshair
  readout — rather than left to be read off the axis. Hover or focus the chart and
  arrow along it for the numbers at a moment.
- The y-axis always starts at zero. These are rates and sizes, where the distance
  from nothing is the thing being read.
- A sample Prometheus could not compute leaves a gap rather than a zero — those
  mean opposite things on a chart.

### Icons

Any of: `alert`, `bell`, `box`, `check`, `chip`, `clock`, `copies`, `dashboard`,
`database`, `display`, `drive`, `edit`, `file`, `folder`, `gateway`, `gauge`,
`globe`, `grant`, `helm`, `info`, `layers`, `link`, `lock`, `monitor`, `policy`,
`priority`, `puzzle`, `refresh`, `repeat`, `rocket`, `route`, `rows`, `scale`,
`search`, `server`, `settings`, `share`, `shield`, `sliders`, `type`, `webhook`.
An unrecognised name draws a plain box.

## Shipping several at once

A file with a `plugins` array is a pack, which is how a collection is
distributed as one file:

```json
{
    "name": "Acme Pack",
    "author": "acme",
    "version": "1.0.0",
    "plugins": [ { "id": "acme", "...": "..." }, { "id": "acme-edge", "...": "..." } ]
}
```

## Replacing a built-in

A plugin whose `id` matches a built-in one takes its place — that is how you
retune the Flux plugin's view list without renaming it everywhere. Two *user*
plugins claiming the same id is a mistake: the first found wins, the other is
reported under **would not load**.

## When something is wrong

Nothing about a plugin is fatal. One unreadable file does not cost you the
plugins either side of it; one bad plugin inside a pack does not cost you the
rest of the pack; and anything refused is named with a reason under **would not
load** rather than silently missing.

A tab left open on a plugin that has since been uninstalled says so, and keeps
your tab, rather than showing an empty table.

## The built-in three

`internal/plugins/builtin/*.json` are in exactly the format above and are worth
reading as worked examples:

- **`argocd.json`** — health and sync as two separate cards over the same kind,
  and a view of a *built-in* kind narrowed by a label selector.
- **`flux.json`** — thirteen views over five API groups, all summarised through
  the one `status.conditions[Ready]` path every Flux controller answers on.
- **`prometheus.json`** — the Prometheus Operator's CRDs, plus every chart in the
  app: CPU, memory and network per pod; CPU, memory and pod count per node; and
  cluster CPU, memory, pods by phase and API server request rate on the
  dashboard. Its queries assume the metric and label names the
  kube-prometheus-stack sets up (cAdvisor with a `node` label,
  kube-state-metrics). If your monitoring is set up differently, copy the file
  into your own plugins folder under the same id and change the queries — a
  user plugin replaces a built-in of the same id.

None of them pins a namespace, on purpose: the official manifests set
`app.kubernetes.io/part-of` wherever they are installed, so a selector is robust
where a hardcoded `argocd` or `flux-system` would quietly show nothing for
anyone who installed elsewhere.

## How it works, briefly

A tab opened on a plugin view carries the kind `plugin:<pluginId>/<viewId>` —
the same trick `crd:<plural>.<group>` plays. A tab's kind is persisted,
reordered, restored and titled by machinery that never looks inside it, so a
plugin view becomes a tab without any of that learning a second shape. It is
resolved back into a real kind and its filters at the last moment, when the watch
is opened.

- `internal/plugins/` — the format, the validator, the loader, the overview
  builder, and the three built-ins.
- `internal/addons/` — the file discovery both plugins and themes share.
- `internal/kube/tally.go` — counting objects by a field path.
- `pluginservice.go` — what the frontend calls.
