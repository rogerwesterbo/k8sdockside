# Themes

Every colour k8sdockside draws comes from a theme, and a theme is a JSON file.
Thirteen ship with the app; anything else is a file you drop in a folder.

A theme is a set of colours and nothing else. It cannot ship CSS, override a
layout or run code. That is a deliberate limit rather than an unfinished one:
it means installing a theme you found on the internet is about as risky as
installing a wallpaper, and it means a theme written today still works after the
app grows a screen its author never saw.

## Installing one

Put the `.json` file in the themes folder:

| Platform | Folder |
| --- | --- |
| Linux, macOS | `$XDG_CONFIG_HOME/k8sdockside/themes/`, falling back to `~/.config/k8sdockside/themes/` |
| Windows | `%AppData%\k8sdockside\themes\` |

**Settings → Themes** shows the exact path for your machine, with a button to
open it. Themes are read at launch and whenever you press **Reload**, so an
editor open beside the app is the intended way to work on one.

Files are read from that folder and from one level inside any subfolder, so a
pack cloned or unzipped into a directory of its own is picked up as it is:

```
themes/
  my-theme.json              ← read
  acme-pack/
    neon.json                ← read
    midnight.json            ← read
    README.md                ← ignored, not a .json
  acme-pack/nested/deep.json ← NOT read; one level only
```

You can also point the app at folders elsewhere — a dotfiles repo, a shared
drive — with **Watch another folder** in the same section. Removing a folder
never deletes anything; the themes in it just stop being offered.

## Writing one

The fastest start is **Settings → Themes → Write a starter theme**, which drops
a complete file with every colour already filled in. Change one, press
**Reload**, look at it.

The minimum is an `id`, a `name` and a `base`:

```json
{
    "id": "acme-neon",
    "name": "Acme Neon",
    "tagline": "loud dark · neon pink",
    "base": "dark",
    "author": "you",
    "tokens": {
        "bg": "#0b0b12",
        "text": "#f2e9ff",
        "accent": "#ff3d9a",
        "accent-text": "#1a0010"
    }
}
```

| Field | | |
| --- | --- | --- |
| `id` | required | Lowercase letters, digits and dashes. Identifies the theme in the settings file, so changing it later reads as a different theme. |
| `name` | required | What the gallery calls it. |
| `base` | required | `dark` or `light`. Decides two things: which built-in theme fills in the colours you leave out, and what `color-scheme` the window uses — which is what colours the text caret and any control the app has not styled itself. |
| `tagline` | optional | The line under the name in the gallery. |
| `author` | optional | Yours. |
| `tokens` | optional | The colours. Every one is optional. |

**Every colour is optional.** Whatever you leave out is inherited from the
built-in theme matching your `base`, so the four-colour example above is a real,
complete theme — and it stays complete when a later version of the app adds a
colour its author never saw.

### The colours

| Token | What it is |
| --- | --- |
| `bg` | The window behind everything. |
| `bg-sidebar` | The context sidebar, the settings rail and the status bar. |
| `bg-panel` | Panels that sit on the window: the detail panel, the dock, the editor. |
| `bg-raised` | Controls that stand off their surface: buttons, pills, input wells. |
| `bg-hover` | Laid over a surface on hover. Usually translucent, so it works on all of them. |
| `bg-active` | Laid over a surface when it is selected or held. Usually translucent. |
| `border` | The structural edge around a panel, where being seen is the point. |
| `border-soft` | The line between rows in a list, where a line per item adds up. Much fainter than `border`. |
| `text` | Body text and anything that has to be read first. |
| `text-dim` | Secondary text: hints, values, the second line of a row. |
| `text-faint` | Small print: counts, section headings, file paths. Still has to clear AA at 10px. |
| `accent` | Selection, focus rings, links and primary buttons. |
| `accent-text` | Text drawn on top of `accent`. The one token that must contrast with another token rather than a surface. |
| `ok` | Healthy: Running, Ready, Bound. |
| `warn` | Not yet, or not quite: Pending, Progressing, a nearly full disk. |
| `error` | Failed: CrashLoopBackOff, Evicted, an unreachable cluster. |
| `scrollbar` | The scrollbar thumb. Translucent, so dense tables do not gain a heavy frame. |
| `scrollbar-hover` | The scrollbar thumb under the pointer. |
| `chart-1` … `chart-8` | The series colours in a metrics chart, assigned in order and never cycled. |
| `chart-grid` | Gridlines and axes in a chart. Recessive — one step off the surface. |

**About the chart colours.** The eight slots are a *validated categorical
palette*, not eight colours somebody liked: the order is what keeps neighbouring
lines apart under protanopia and deuteranopia, and the defaults clear the WCAG
and CVD gates against every built-in theme's surfaces. Restepping them for your
own palette is fine; reordering them, or picking eight by eye, will quietly make
charts unreadable for about one man in twelve. If you do change them, keep hues
in the same families and check adjacent pairs stay distinguishable.

They are deliberately separate from `ok`/`warn`/`error`. Those mean something —
a series that merely happens to be fourth must not wear the colour that means
"failing".

The same table is in the app, under **Show all colours a theme can set**, with a
swatch of what the current theme gives each one.

`accent-text` is the one people forget. Change `accent` to a pale colour and the
white text on your primary buttons disappears; the app will tell you, but it is
easier to set both at once.

### What counts as a colour

Hex (`#fff`, `#ffff`, `#4a86ff`, `#4a86ffcc`), the CSS colour functions
(`rgb()`, `rgba()`, `hsl()`, `hsla()`, `hwb()`, `lab()`, `lch()`, `oklab()`,
`oklch()`, `color-mix()`), and `transparent`.

Named colours like `red` are not accepted, and neither is anything else — no
`var()`, no `url()`, no bare values. A theme file comes from outside the app, so
this is a whitelist: the answer to "what else could go in there?" is nothing.
Write the hex.

## Shipping several at once

A file with a `themes` array is a pack, which is how a collection is
distributed as one file:

```json
{
    "name": "Acme Pack",
    "author": "acme",
    "version": "1.0.0",
    "themes": [
        { "id": "acme-neon", "name": "Acme Neon", "base": "dark", "tokens": { "accent": "#ff3d9a" } },
        { "id": "acme-day", "name": "Acme Day", "base": "light", "tokens": { "accent": "#c01f6e" } }
    ]
}
```

The pack's `name` is shown against each of its themes in the gallery, so a user
can see which ones arrived together.

## Replacing a built-in

A theme whose `id` matches a built-in one takes its place. That is how you
retheme the default without every settings file that names it having to change.
Two *user* themes claiming the same id is a mistake rather than a feature: the
first one found wins and the other is reported under **Would not load**.

## When something is wrong

Nothing about a theme is fatal. One unreadable file does not cost you the
themes either side of it, and a theme that fails is named with the reason under
**Would not load** rather than silently missing.

Two things are worth knowing:

- **Readability warnings.** The app measures every text colour against every
  surface it is drawn on, and `accent-text` against `accent`, using the WCAG
  contrast ratio. Anything below 4.5:1 is flagged on the theme's card. It is
  advice, not validation — the theme still loads. All thirteen built-ins clear
  the bar, and there is a test that keeps it that way.
- **A theme that is not installed.** If your settings name a theme that is not
  there — you deleted the file, dropped the folder, or opened the same settings
  on another machine — the app wears the default and says so in Settings. Your
  choice is *not* rewritten, so putting the theme back restores it.

## How it works, briefly

There is no rule anywhere in the app that names a theme or asks which one is on.
Every component asks for `var(--token)`; the current theme's colours are written
onto the document root as custom properties, and that is the whole mechanism.
Adding a fourteenth theme costs a file and no code, which is the point.

- `internal/themes/` — the format, the validator, the loader, and the built-in
  palettes as JSON in exactly the format described above.
- `internal/themes/builtin/*.json` — worth reading. They are the same shape your
  theme is, so any of them is a working starting point.
- `themeservice.go` — what the frontend calls.
- `frontend/src/lib/theme/apply.ts` — writing the tokens onto the document.
