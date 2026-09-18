# @robotdreams/mission-control

Mission Control — the Robot Dreams dashboard — as an unbranded React
component library. It renders the same UI shipped inside the `dream`
binary (see [docs/dashboard.md](../../../../docs/dashboard.md)), usable
at four levels: the whole app, its chrome around your own views, a
single view, or a single panel.

Not yet published. `internal/dashboard/ui/app`, the app the `dream`
binary embeds, does not depend on the published package either — it
consumes this library's source directly through a Vite alias (see
"Developing it" below). Publishing is manual:

```sh
npm run build:lib
npm publish -w @robotdreams/mission-control
```

## Install

```sh
npm install @robotdreams/mission-control react react-dom
```

`react` and `react-dom` are peer dependencies (`^19.0.0`); the library
brings its own chart, d3, three, mermaid, marked and highlight.js
dependencies.

Import the two stylesheets once, anywhere in your app:

```ts
import "@robotdreams/mission-control/style.css"; // structure — required
import "@robotdreams/mission-control/theme.css"; // palette — opt-in
```

`style.css` is the layout and the components; it reads its colours from
custom properties and declares none of them. `theme.css` is Mission
Control's own palette, light and dark. Skip it and declare the same
`--tokens` on your `.mc-scope` element yourself, mapped to your design
system — every token the structural sheet reads is named in that file.

The dashboard documents itself: Admin → Docs → **Component library**
(`#/settings/docs/library`) lists every export by level, with the panel and
icon rosters read from the modules, and the hosts it runs in.

Fonts are yours to bring. The stylesheet only names `--font-heading`,
`--font-body`, and `--font-mono`, with system fallbacks — the shipped
app installs `@fontsource-variable/{inter,space-grotesk,jetbrains-mono}`.

## Requirements

- React 19.
- `moduleResolution: "bundler"` (or `node16`/`nodenext`) in
  `tsconfig.json` — the package exports map relies on it.

## The four levels

### Whole app

`MissionControl` renders the full shell — topbar, all four sections
(Control Plane, Outcomes, Schedules, Settings), its own routing and
connection state. A token is enough:

```tsx
import { MissionControl } from "@robotdreams/mission-control";
import "@robotdreams/mission-control/style.css";

<MissionControl connection={{ token: myToken, apiBase: "https://control-plane.example.com" }} />;
```

With no `brand` prop the topbar shows the product name alone; pass a
`ReactNode` to put your own mark there.

### The chrome around your own content

`Shell` is the basic shell with nothing Robot Dreams in it: the skip
link, the topbar with the page-control slots a view fills, the live
status region, and your children. It shows no product name unless you
pass `title` or `brand`, and no theme switch or gear unless you put
them in `actions` — `ThemeToggle` and `AdminLink` are exported for
that, and sit side by side at the topbar's right edge (link to Admin
only if you also render `AdminView`). A host that would rather drive
light/dark from its own settings can skip the toggle entirely and pass
`theme`/`onThemeChange` instead; see Theming below.
`MissionControlShell` is this plus those two actions, the four views,
and the two rules only the whole app knows.

```tsx
import { MissionControlProviders, Shell, ThemeToggle, ReportingView } from "@robotdreams/mission-control";

<MissionControlProviders connection={{ token, apiBase }} router={{ route, onNavigate }}>
  <Shell title="Ops console" actions={<ThemeToggle />}>
    {route.view === "reporting" ? <ReportingView /> : <MyOwnPage />}
  </Shell>
</MissionControlProviders>;
```

Give your own content `id="main-content"` so the skip link lands on it;
the library's views do this themselves.

### One view inside your own shell

For a host with its own routing, connection, and chrome, use
`MissionControlProviders` (context only, nothing rendered) plus
`PageControlsProvider`/`PageChrome` for the toolbox slots a view
expects:

```tsx
import {
  MissionControlProviders,
  PageControlsProvider,
  PageChrome,
  ReportingView,
  RouterProvider,
} from "@robotdreams/mission-control";
import "@robotdreams/mission-control/style.css";

// `route` is a Route the host keeps in its own state (parseHash() makes
// one from a hash string); onNavigate receives the next Route and the
// host updates its URL and state. The container carries the theme.
const [el, setEl] = useState<HTMLElement | null>(null);

<div className="mc-scope" ref={setEl}>
  <MissionControlProviders
    connection={{ token: hostToken, apiBase: hostApiBase }}
    router={{ route, onNavigate }}
    themeTarget={el}
  >
    <PageControlsProvider view={route.view} chrome>
      <ReportingView />
    </PageControlsProvider>
  </MissionControlProviders>
</div>;
```

The other top-level views are `OverviewView`, `SchedulesView` and
`AdminView` (takes `onClose`). `AdminView`'s Connection section hides
itself when the host supplies a token directly — there's nothing for an
operator to paste.

The view selector in every toolbox lists Universe, the Outcomes
perspectives and Schedules; a host that routes on the view name sees
`route.view` as `"overview"`, `"reporting"` or `"schedules"` (`"admin"`
for Settings).

### Schedules

`SchedulesView({ active?, workerId? })` lists every cron schedule the
control plane holds, grouped by the worker each one wakes (`to`): id,
subject, the cron expression with the next fire rendered relative and
absolute, owner, last fired ("never" until it has), and whether it is
enabled. It reads `GET /api/schedules` (with `?worker_id=<id>` when
`workerId` is given) through the connection's `apiJSON`, and refetches
on every `schedule_fired` event from the live feed — or every 15 s
while the stream is not live. No admin credential is needed: the server
answers with whatever the identity may see, and the view says "No
schedules" when that is nothing.

`useSchedules(workerId?, active?)` is the hook behind it, exported for
a host that wants the rows without the table: `{ schedules, pending,
error, refresh }`, `Schedule` being the API's shape (`id`, `owner`,
`to`, `cron`, `subject`, `body`, `enabled`, `next_at`, `last_at`
nullable, `created_at`; RFC3339 times).

### The reporting period

The Outcomes toolbox carries a period control — Live · Day · Week ·
Month · Quarter · Year with ‹ › to step and • to return to the current
stretch. Live sends nothing and is today's behaviour (the newest report
at every scope). A period is appended to every perspective's fetches
and to the drill-down page as `period=<kind>&at=<RFC3339>`, so
`/api/reports/summary`, `/report` and `/timeline` fold each scope's
instances over that stretch server-side; the Timeline perspective uses
it as its bounds. The period the server actually used — echoed as
`{"period":{"kind","start","end"}}` — is shown as a caption under the
toolbox. The choice is remembered under the storage prefix
(`<prefix>reportingPeriod`), not carried in the route: a deep link opens
live.

`usePeriod()`, `periodQuery()`, `periodCaption()`, `stepAt()` and
`PeriodControl` are exported for a host composing its own toolbox.

### A single panel

Panels compose without any provider beyond what they read from context:

```tsx
import { PanelSection, PanelBody, panels } from "@robotdreams/mission-control";

<PanelSection title="Recent activity">
  <PanelBody>
    <panels.EventsPanel {...panelProps} />
  </PanelBody>
</PanelSection>;
```

Primitives (`Link`, `SidePanel`, `Tabs`, `ScopeSelector`, and more),
contexts, hooks, and `lib` helpers are all exported from the root entry
too — see `src/index.ts` for exact names, or import a subpath
(`@robotdreams/mission-control/views/*`, `./panels/*`, `./components/*`,
`./state/*`, `./hooks/*`, `./lib/*`) to pull in less.

## Routing contract

- `HashRouterProvider` (what the whole-app entry points use) — the route
  lives in the URL fragment, as in the shipped app.
- `RouterProvider({ route, onNavigate(next, { replace }), href? })` — for
  a host that owns its own URLs: you supply the current route and a
  callback for when the library wants to navigate; `href` controls what
  `Link` renders as `href` for a non-hash-based host.

`Link` renders a real anchor and routes plain left clicks through
whichever router is active; modified clicks fall through to the browser.
Route builders (`adminRoute`, `crossViewRoute`, `viewRoute`,
`reportingRoute`) sit beside the hash helpers (`parseHash`, `buildHash`).

## Connection contract

`ConnectionProvider({ token?, apiBase?, fetchImpl?, persist?, fallbackToken?, onUnauthorized? })`
(also the `connection` prop on `MissionControl`):

- Pass `token` (and `apiBase`) when the host already has one — no
  connection page, no redirect, `AdminView` hides its Connection section.
- Omit `token` for the paste-a-token flow: the app used by `dream` passes
  `persist: true` (stores the pasted token under the storage prefix, see
  below), plus `apiBase` and a dev token. `persist` defaults to `false`.
- `onUnauthorized` fires on a 401, so a host managing its own token can
  react.

## Hosting in Tauri or Electron

The library needs nothing beyond a DOM and React 19, so a Tauri window
(a system webview) or an Electron renderer runs it as is. What changes
is the origin: the window is not served from the control plane, and the
control plane sends no CORS headers, so the browser `fetch` is refused.
Every request the library makes — including the `/api/events` stream —
goes through the one `fetchImpl` the connection was given, so supply
one that is not subject to CORS:

```tsx
import { fetch as tauriFetch } from "@tauri-apps/plugin-http";

<MissionControl connection={{ token, apiBase: "https://control-plane.example.com", fetchImpl: tauriFetch }} />;
```

The Tauri HTTP plugin's `fetch` streams response bodies, which the SSE
reader relies on. Keep `apiBase` absolute. In Electron, either pass a
`fetchImpl` that hops to the main process, or relax CORS for the
control plane's origin in the session's `webRequest` handlers.

## Theming

`ThemeProvider({ target? })`, used automatically by the whole-app entry
points. With no `target` it sets `data-theme` on `<html>` (dark only;
light removes it, following `prefers-color-scheme` when nothing is
stored). With a `target` element it always sets `data-theme` there
instead, so a container can carry its own theme — pass the element
wrapping your content (the same one carrying `mc-scope`, below) as
`themeTarget`.

The theme is uncontrolled by default: it remembers the reader's choice
and follows the OS preference until they make one. Pass `theme` and
`onThemeChange` to own it from outside instead — the library then stores
nothing and follows nothing, and the toggle beside the gear asks you for
the change rather than making it:

```tsx
const [theme, setTheme] = useState<Theme>("dark");

<MissionControl connection={{ token }} theme={theme} onThemeChange={setTheme} />;
```

`onThemeChange` fires either way, so an uncontrolled host can still
observe the choice, and `defaultTheme` sets the uncontrolled starting
point when nothing is stored. `useTheme()` gives any component
`theme`, `toggle()`, `setTheme()` and `controlled`.

`theme.css` declares the palette on `.mc-scope` rather than `:root`,
light by default, with `[data-theme="dark"]` on the scope element, on
any ancestor, or on any nested element that wants to force a theme for
its subtree. Dark is declared first so an explicit light on the scope
element still wins.

`useThemeRoot()` returns the element to read resolved tokens from, for
the consumers that need a colour rather than a class — mermaid,
chart.js. Because the palette lives on `.mc-scope`, that
element has to be inside the scope: it is your `themeTarget` when you
pass one, otherwise the scope element the `Shell` registers.

## Storage prefix

Every remembered preference (theme, side panel width, per-view variant)
lives under one localStorage prefix, `dream.` by default. Call
`setStoragePrefix()` before first render to change it — needed if you
embed more than one instance on a page:

```ts
import { setStoragePrefix } from "@robotdreams/mission-control";
setStoragePrefix("myhost.");
```

## CSS scoping

The stylesheet cannot touch anything the library did not render. Every
rule in `dist/style.css` is nested under `.mc-scope` — the reset and the
typography base rules included — so there is no `:root`, no `html` or
`body` rule, and no bare element selector anywhere in the file. Every
class the library owns carries the `mc-` namespace, and every
`@keyframes` name does too, since animation names are global wherever
the rule sits. State modifiers (`is-`, `has-`) are unprefixed by
design: they only ever appear alongside an `mc-` class.

The third-party sheets the components pull in (highlight.js, TimelineJS)
are scoped the same way at build time by `postcss-mc-scope.js`, but
their class names are left alone — their own JS writes those. The one
thing that is still global is TimelineJS's `@font-face` for its icon
font, which has to keep its family name to work at all.

`MissionControl`'s root is `<div class="mc-app mc-scope">`; embedding at
the view or panel level, wrap your own container in `mc-scope` so the
rules apply — that element is also a good `themeTarget`. `PageChrome`
renders under `.mc-chrome`.

If you render markup into one of the library's slots — a `brand` node in
the topbar, say — use its `mc-` classes: that markup is inside its
subtree, so its classes are what style it.

## Live feed and shell state

- `LiveProvider`/`useLive()` — without one, a view gets an inert feed and
  falls back to polling (the reporting and schedules pages poll every
  15s). The whole-app entry points include it, wired to `/api/events`;
  `schedule_fired` bumps `scheduleVersion` and lands in the activity
  feed beside the messages.
- `ShellProvider`/`useShell()` — side panel state (collapsed, width) and
  chosen renderings, shared across views. Without one, a view keeps this
  state locally.

## Using it before it is published

The package is not on npm yet. From a checkout:

```sh
cd internal/dashboard/ui
npm ci
npm run build:lib                                  # -> packages/mission-control/dist
npm pack -w @robotdreams/mission-control           # -> robotdreams-mission-control-<version>.tgz
```

Then, in the consuming project, either install the tarball
(`npm install /path/to/robotdreams-mission-control-<version>.tgz`) or
depend on the directory (`"@robotdreams/mission-control": "file:../robotdreams/internal/dashboard/ui/packages/mission-control"`).
Both resolve through the same `exports` map a registry install would, so
`dist/` must exist — rebuild it after changing library source.

## Publishing

`.github/workflows/publish-mission-control.yml` publishes to npm, but
never on its own: dispatch it by hand (a dry run by default) or push a
tag `mission-control-v<version>` matching `package.json`. A real publish
needs the `NPM_TOKEN` repository secret. Bump the version in
`packages/mission-control/package.json` first.

## Developing it

From `internal/dashboard/ui/` (the workspace root, shared with `app/`):

- `npm run build:lib` — the real build: Vite library mode into `dist/`
  (ES modules, one file per source module via `preserveModules` so a
  consumer's bundler can still code-split the lazy views, plus
  `dist/style.css`), then `tsc -p tsconfig.build.json` for the `.d.ts`
  files beside each module.
- `npm run smoke` — builds `smoke/` (a plain consumer, no source alias)
  against `dist/` and typechecks it against the emitted `.d.ts`. This is
  what proves the _published_ package works — `app/` never touches
  `dist/`.

`app/`, the app embedded in the `dream` binary, does not install this
package from npm: its `vite.config.ts` aliases
`@robotdreams/mission-control` straight at this package's `src/`, so
changes here hot-reload into it immediately, with one install and one
bundler pass shared across both. Run `npm run smoke` before publishing —
the alias path can't catch a break in the built output.
