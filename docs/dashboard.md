# Mission Control (the dashboard)

Mission Control is a Vite + React + TypeScript front end whose build
output is embedded into the `dream` binary via `go:embed`
(`internal/dashboard`; the output is built by `make build`, not
committed — see [Architecture & development](#architecture--development)
below). It holds no server-side state of its own — it's a pure client of
the same `/api/*` HTTP surface a worker or the CLI uses.
It is styled per the repo-root [DESIGN.md](../DESIGN.md) (light-first,
dark theme available) and is fully offline-capable: fonts and every
library are bundled and embedded, no CDN or external request at runtime.
The one exception is deliberate and operator-initiated: previewing a
node's app embeds that node's page, and nothing is fetched until you ask
for it (see [apps.md](apps.md)).

## Running it

**Mounted by default.** `dream server init` serves it at `/dashboard`
alongside the API on the same process/port — just open
`http://<addr>/dashboard/` (e.g. `http://127.0.0.1:7420/dashboard/`).
Disable it with `--no-dashboard`.

**Standalone.** If the dashboard was disabled on the control plane, or
you want it served from a separate address, run:

```sh
dream dashboard --server 127.0.0.1:7420 --addr 127.0.0.1:7421
```

This is a pure client of the same API as `server init`'s mounted mode —
there's no privileged side channel; `--server` just tells it which
control plane's `/api/*` to talk to.

## Auth: token-paste

The dashboard has no server-side identity of its own, so it uses a
pragmatic token-paste flow instead of a real browser-side keypair (see
[security-model.md](security-model.md) for why this is a deliberate
simplification, not the target design):

1. Connect a worker or mint an admin token elsewhere, e.g.:
   ```sh
   dream worker connect --server 127.0.0.1:7420 --worker-id ops --role admin
   ```
2. Open the dashboard; with no usable token it redirects to
   `#/settings/connection`, which is both the first-run login and the place
   the server URL is changed later. (The docs sections stay readable
   before you have connected to anything.)
3. Paste the token. The dashboard proves it actually authenticates
   against this server (a live API call) before committing it to
   `localStorage`; on failure it shows a connection error rather than
   silently storing a bad token.
4. Subsequent requests attach `Authorization: Bearer <token>`.

A token minted without the `admin` scope can still see the **whole org
chart** (`GET /api/workers` and `GET /api/workers/{id}` have no
per-caller authorization at all — any authenticated worker can list
every worker, its role, its `reports_to` edge, its metadata, and its
public key) and the **whole storage backend** (put/get/list/delete are
likewise unauthorized beyond "holds a valid token" — see
[security-model.md](security-model.md) for why this is a real, current
gap and not the target design). What `admin` *does*
gate: `/api/events` (the live SSE feed), reading another worker's
message inbox, and `dream server revoke`.

## Sections

The topbar carries a small section switcher with three views. Switching
is client-side (a small hash-router hook, no routing library — the
grammar is hand-rolled so scope paths and selections stay legible); the
active view — and its state — is reflected in the URL hash, so refresh,
back/forward, and deep links all work. The slug is what the interface
calls the view, so a pasted link reads as the thing it opens:

- **Control Plane** (`#/control-plane`, the default) — the original
  panels below. "Control Plane" is also the name of the wider CLI +
  Mission Control layer described in
  [vision/control-plane.md](vision/control-plane.md); this section is
  the fleet-watching part of it.
- **Outcomes** (`#/outcomes`) — the reporting surface, described in its
  own section further down.
- **Settings** (`#/settings`) — settings, the design system, and docs
  behind one left menu, reached from the gear. Drawn as a **dialog
  floating over the app**: the view you came from stays live and dimmed
  underneath, and Escape, the ✕, or the darkness puts you back.

Unknown hashes still fall back to Control Plane.

### Admin

The gear used to open a modal holding one form. That form is still there
as the **Connection** section, but it now shares a shell with everything
that is *about* the system rather than *of* it.

| Section | `#/settings/…` | What it is |
| --- | --- | --- |
| Connection | `connection` | Server URL + bearer token; also the first-run login |
| Updates | `updates` | The fleet rollout: every announced kind and what each node said back |
| Infographic | `docs/infographic` | The whole system as one clickable picture; each part expands in place |
| The spine | `docs/spine` | Two swappable primitives and a control plane |
| Hierarchy & identity | `docs/hierarchy` | The five levels, notation, iconography |
| Reporting model | `docs/reporting` | Definition vs instance; the seven facets |
| Categories | `docs/categories` | The nine subjects, their boundaries, and the non-categories |
| Modalities & views | `docs/modalities` | Seven delivery modes → seven views, two of them templated |
| Aggregation | `docs/aggregation` | How a site number becomes a universe number |
| Scope & deep links | `docs/scope` | Scope paths, prefix matching, the URL grammar |
| Updates | `docs/updates` | The update contract: announce, and what each node says back |
| Component library | `docs/library` | What the dashboard exports as `@robotdreams/mission-control`, level by level, and which hosts (Vite, Tauri, Electron) can run it |
| Colors … Voice & content | `design/…` | The design system, split into its own menu group: colors, typography, spacing & layout, iconography, motion & depth, primitive and composite components, voice |

Three things worth knowing about the shape:

**Every docs topic leads with a picture.** The spine is a CSS box
diagram, hierarchy is an indented level ladder, scope is an annotated
path. They are hand-drawn in CSS rather than generated, so they theme
with everything else and cost no diagramming dependency — mermaid stays
available for the `MermaidDiagram` panel renderer, which needs real
graph layout.

**The pages are generated from the code where possible.** Categories
renders from `lib/routes.ts`, modalities from `lib/variants.ts`, and the
design and vocabulary specimens are the app's real components. A page
cannot claim a view or a chip exists that the app does not ship.

The topbar's gear carries a second dot while an update rollout is
incomplete — someone still silent, moving, or failed — and that dot
links straight to the Updates section.

Docs sections load as lazy chunks; the shell and the connection form
stay in the main bundle, since the connection form is on the critical
path for an unauthenticated first load.

**The library's CSS cannot reach the page around it.** Every rule the
package ships is nested under `.mc-scope`, its own classes carry an
`mc-` namespace, and the palette is a separate opt-in file declaring its
tokens on `.mc-scope` rather than `:root` — so a host can drop the
dashboard into an existing page, or map the tokens to its own design
system, without either stylesheet touching the other. The app around it
(`ui/app/src/app.css`) owns the document: it is the only place `html`
and `body` are styled, and it declares the few values the frame needs
itself.

The topbar itself: a two-line brand (Robot Dreams / Mission Control),
then **one bordered box** of fixed height holding the active page's
**page-controls slot**. There is no separate view toggle any more: the
first thing in every page's toolbox is the captioned view selector, and
it *is* the navigation — a "Views" caption over one strip where
**Universe** (the Control Plane) sits as a peer of the seven Outcomes
perspectives. While Universe is active its four renderings expand in
place directly after it, in the dashed in-row group the implementation
chooser once used (Network first, the default, then Tree, Reporting,
Planets, Galaxy). Choosing how to read the system and choosing where to be are
the same gesture. At the box's far right the active view's zoom and
depth controls stack as single icons whose slider slides out on hover
or focus, and the theme toggle and gear stack the same way at the bar's
edge. On the right of the bar sit the
theme toggle, and the gear — a link to Admin, whose corner dot carries
the event-stream state (green live, amber connecting, gray offline) now
that the stream pill is gone. The side-panel toggle is no longer here:
it lives on the panel itself.

What lands in the page-controls slot depends on the page: the **scope
control** and the **perspective switcher** on both Control Plane and
Outcomes, category **filters** on Outcomes, and zoom controls from any
view showing a zoomable picture. Per-page tools belong in the strip, not
floating over the content.

The **scope control** is built on the identification vocabulary rather
than raw path text: the current path renders as the real chips for its
levels (`W01`, `R0`, `S01`), with realm colour cascading to the sites and
nodes beneath.

Its panel is deliberately large, because picking a scope is the main
navigational act in the app and the hierarchy is worth drawing at a size
you can read. Two columns, narrowing on different axes:

- **Hierarchy** (left) narrows by *place*. One row per level; worlds get
  the full circular badges at size, the way the public site draws them,
  since that is the level people navigate by. Picking something with
  levels below it is a drill-down, so the panel stays open and grows
  another row; picking a leaf closes it.
- **Nodes** (right) narrows by *kind* — role chips with counts, plus an
  id search. The list is already scoped by whatever the left column
  selected, so the two compose. Selecting a node navigates to the scope
  it reports for; Control Plane additionally passes `onSelectNode`, so the
  same click fills its detail panel.

Both columns scroll inside the panel rather than growing it. That needs
`grid-template-rows: minmax(0, 1fr)` on the panel and `min-height: 0` on
each column — an `auto` row takes its height from content, so a
`max-height` on the container alone would only clip.

### The label popover

Hovering (or focusing) any identity label opens a small panel with
**Id**, **Name**, and a count of what it contains (`#Realms`, `#Sites`,
`#Nodes`). The universe has no Id row — the vocabulary never numbers it.

This replaces the old inline reveal, where a named chip showed its code
on hover by widening in place. That moved the label under the pointer
and had room for nothing else; the popover leaves the label still and
can carry the containment count as well. The code is still in the
accessible name either way.

It is fixed-position and portalled to the body, so it escapes the
topbar's overflow and the scroll containers the chips sit in. For the
same reason it closes on scroll rather than drifting away from its
anchor. It prefers to sit below and flips above when there is no room,
clamped to the viewport on both axes. The wrapper is `display: contents`
so the chip's own layout — inline-flex inside links, table cells, flex
rows — survives being wrapped.

Scope is not Outcomes' alone — Control Plane filters its org chart by the
same scopes and carries them in its own hash (`#/control-plane/<scope>`), so
both sections are deep-linkable the same way. One consequence worth
knowing: filtering the org chart removes ancestors, so `buildIndex`
re-roots any worker whose manager is no longer in the set. Without that
the survivors would hang off a parent nobody renders and vanish.

### Perspectives

The **perspective switcher** is the permanent half of what used to be one
temporary "Option N" selector. A perspective is a consumption modality
from the reporting contract — glance, delta, storyboard, spatial, board,
narrative, conversational — and that vocabulary is not going anywhere, so
it sits in the toolbox.

Which *implementation* serves a perspective is a different question with
a different lifetime: React Chrono or TimelineJS for storyboard, d3 or
mermaid for spatial. Those stay in the brand slide-down, because one of
each is meant to be deleted. A perspective serving more than one
implementation shows the count.

Switching perspective restores the implementation you last used for it
(`dream.impl.<view>.<modality>`), so storyboard → glance → storyboard
returns you to the timeline library you were reading, not to the list's
first entry. A modality in the contract with nothing behind it would
render disabled rather than hidden; every one of them now has a view.

Control Plane's renderings declare no modality — they are four drawings
of one structure, not delivery modes — so they appear as Universe's own
in-place second level rather than as perspectives.

Everything else the topbar hides uses **one slide-down panel** under
the row, and only one can be open at a time (Escape or a view change
closes it): the **brand** opens the display options; the **search icon**
opens the worker search with its live match count; the **filter** button
opens its own panel. The **scope** control is the exception — picking a
scope is the main navigation act, so it opens as an **overlay**: a
full-width bar in the topbar's blue covers the whole topbar, with the
scope control still showing and Clear / Apply / Cancel right beside it,
then a narrower panel of the same blue hangs below with two tabs —
Places (the hierarchy) and Nodes (roles and specific nodes, deliberately
opt-in). Everything around it dims; the darkness is Cancel.

The slots are portal targets — the toolbox, the shared expander, the
actions group, and the toolbox's own tools area — so the owning view
keeps its state while the topbar decides placement (see
`state/PageControlsContext.tsx`). The tools slot is what lets a page's
*active variant* add its zoom buttons without owning the toolbox.

## What the V1/V2 comparison settled

For one round of development the dashboard shipped two implementations
side by side — every hash addressable with a `#v2/` prefix, a switch in
the topbar, and a `#v2/compare/...` form that put both in one viewport on
the same selection. They shared one app, one data layer and one hash
grammar, so the two panes were provably reading identical data at the
same instant.

That experiment is over. Each surface was judged on real fixture data and
one implementation kept; `Route.rev`, the switch, the compare pane,
`src/v2/` and `styles-v2.css` are all gone, and every `#v2/` link now
falls back to Control Plane like any other unknown hash.

What survived, and why:

| Surface | Kept | Because |
| --- | --- | --- |
| Glance board | V2 | Banded **needs attention / steady / quiet**, ordered inside a band by severity then by how far past target the worst KPI is — alphabetical order put a critical report between two healthy ones |
| Loading | V2 | Skeletons; an error card with retry; "nothing configured" and "nothing matches this filter" as different components, rather than the onboarding card doubling as the loading state |
| Report page | V2 | Scroll-spy rail that says where you are, and KPIs with targets drawn as bullet graphs instead of three spans and a subtraction |
| Control Plane tree | V2 | `role="tree"`, one tab stop, arrow-key navigation, per-row rollup and measured health |
| Search | V2 | Offered only on the tree, the one rendering that consumes it |
| Planets, Galaxy, Network | V1 | The drawings were never the thing V2 was arguing about |
| The always-visible filter bar | Neither | It restated what the toolbox controls already say, one row above them |

### Accessibility baseline

A skip link to `#main-content` (a keyboard reader would otherwise walk
the whole topbar to reach the content), and one `aria-live="polite"`
region announcing stream-state changes, which were previously visible
only as the colour of a dot.

The org chart is `role="tree"` with a single tab stop and roving focus,
rather than one `role="button"` div per row with a button nested inside
it.

## Layout: an app shell, not a document

The viewport is the frame. `html`/`body` do not scroll; `.mc-app` is exactly
viewport-height with the topbar fixed at the top, and the view below owns
the remaining space. Each of the two grid columns scrolls independently,
so the pulse rail and the main surface move separately and the topbar
never leaves. Document-shaped views (Admin) opt back into
whole-page scrolling with `.layout-scroll`; below 900px the grid stacks
and scrolls as one column again.

Because nothing grows the page any more, the big visual reports fill
their column and are navigated in place — and **zoom is always the
tool's own**, never a transform bolted over it:

- the three d3 views (planet map, force graph, scope topology) use
  **d3-zoom** via `useD3Zoom` (`components/ZoomPan.tsx`): pointer-anchored
  wheel zoom, drag-pan and scale extents in the SVG's own coordinates.
  React owns the resulting transform attribute — d3 writing it directly
  would be wiped by the very re-render each zoom event causes — and the
  behaviour attaches through a callback ref, because these views render
  their SVG only once their data arrives.
- **TimelineJS** keeps the zoom buttons in its own navigator.
- **mermaid** has no zoom of its own, so a mermaid panel offers none; it
  scrolls in its box.

The buttons render into the page toolbox's tools area.

**The right panel is part of the frame, not a card in it.** It runs from
directly under the topbar to the bottom of the viewport and hard against
the right edge, with its own left border as the only separation. A view
with a side panel therefore drops the layout's padding and lets the main
column carry it instead — otherwise the panel floats in a gutter and
reads as a floating card.

It owns its own collapse control (see `components/SidePanel.tsx`), which
used to be a topbar button — the control for a thing belongs next to the
thing. Collapsed, it becomes a 38px full-height rail carrying its name as
vertical text with the expand control at the top, so it never fully
disappears and reopens where you closed it. Open, it is resizable by
dragging its inner edge; the handle is a real `role="separator"` and
responds to arrow keys. State persists under `dream.sideCollapsed` and
`dream.sideWidth`, the latter as a custom property on the shell so no
view threads pixels through props. Sizing uses a two-track grid rather
than the 12-column one. Applies to both Control Plane (activity +
storage) and Outcomes (the pulse rail).

## The reporting views, and why they are named after modalities

The selector in the brand panel switches the active view between display
variants, persisted per view under the `dream.variant.<view>`
localStorage keys. Everything funnels through
`ui/packages/mission-control/src/lib/variants.ts`.
Non-default variants are lazy chunks, and each button's tooltip names
the modality it serves and the component it is testing.

These were called "Option 1–6" while they were being compared, which
said nothing about what any of them *is* — and made them useless as link
targets from elsewhere in the app. The names now come from the
**consumption modalities** the reporting contract already defines
(`docs/vision/reporting.md`): the human-facing delivery modes a facet
can serve. That renaming makes two facts visible that the numbering hid.

| View | Modality | Component |
| --- | --- | --- |
| Glance | glance | status badges and scorecards, one tile per report |
| Delta | delta | the movement each report made, via chart.js |
| Timeline | storyboard | the merged event feed — down the page via React Chrono, across it via TimelineJS3 |
| Spatial | spatial | the scope tree, via d3 tidy tree |
| Board | board | every plan at this scope on one aggregated board — union columns, items grouped by source report |
| Narrative | narrative | TL;DR briefings — templated rather than model-backed |
| Ask | conversational | Q&A grounded in the reports — a fixed repertoire |

There are **seven** built views, one per modality. Timeline (the
storyboard modality) is the only one with two renderers, and they are offered as a *direction* —
down or across — rather than as a choice of library, because a reader is
picking a layout, not a vendor.

Two of the seven are templated rather than model-backed, and say so on
their own pages: the contract requires that degradation to be explicit
rather than silent. Variant `id`s were deliberately left unchanged so
existing `dream.variant.*` values keep resolving; `getVariant`
additionally refuses to restore a stored id whose variant is unbuilt.
The `built: false` mechanism is kept for the next modality declared
before it is drawn — which is the state `board` was in until the plan
facet existed to feed it.

**Board and the differing-vocabularies problem.** Several definitions at
one scope expose plans whose states differ: a team's
`backlog → ready → doing → review → done` beside a department's
`proposed → committed → in_progress → delivered`. Unioning the names has
no defined ordering across vocabularies, and mapping them onto a
canonical set needs a mapping the contract does not have — which would
mean the view knowing reports by name, the one thing the reporting layer
must never do. What *is* shared is each item's fractional position in its
own flow, so the view defaults to one board per report, each in its own
columns, and offers a position-normalised merge as a deliberate choice
with the rule stated on the page. Above them sits a strip — items,
blocked, over-limit columns, past due — every number of which comes from
item fields plus declared order and never from a state name.

Control Plane has three entries, named for what they draw: **Explore**,
**Galaxy** and **Network** (d3-force).

Two of them carry two drawings. A rendering that answers the same
question a second way belongs behind one name with the choice in the
page toolbox, captioned like every other per-view tool, not as another
entry in the navigation — so **Explore**
toggles between the places and the reporting hierarchy, and **Galaxy**
between Planets (d3 circle packing) and Galaxy (react-three-fiber). The
Storyboard's Direction control is the same component, so every such
choice reads the same way. They live in the page toolbox beside the
other per-view tools, captioned above the control in the wrapper Scope
and Filters use, so every label in the bar sits on one line and every
control beneath it on another.
Outcomes does the same: **Glance** toggles between where things stand
and how they have moved, which is what Delta used to be. Every one of
those choices is a reading habit rather than a property of the page, so
each is remembered in `localStorage` through `lib/display.ts` and shared
live across whatever else is showing it.

Four of the five draw the same structure — places, with the nodes that
report for them inside. **Tree** is that structure as a collapsible
outline, which is what a reader opening an outline beside Planets and
Galaxy is asking for; it used to draw the reporting hierarchy instead,
and a fleet whose nodes sat at the universe root got a flat list of
whatever workers happened to exist. **Reporting** is the management
chain, kept as its own rendering because who-reports-to-whom is a real
question and a different shape. Network stays node-only by design: its
edges *are* reporting edges, and a place with no nodes has none.

**Explore** is the default, and it walks the structure rather than
surveying it. A breadcrumb
says where you are; the level you are on is drawn as one box per place,
and each box carries a glance at the level below it — capped, with a
"… n more …" rather than a list, because a box is read at a glance.
Nodes are not places and do not nest, so the ones reporting at the level
you are on get a table underneath. Everything there has two ways in, and
the rule is that the icon is always the other one: a box walks you into
the place and its icon shows that place in the side panel; a node row
shows the node in the side panel and its icon brings the node in here,
where the same detail panel gets the width to be read. The side panel
carries that icon too, so any node or place you have selected in another
rendering can be opened here.

**Everything is drawn in the identification vocabulary.** A box in
Explore is not a card with a title on it — it is the place's own chip
given room, the same badge its level gets everywhere else, carrying the
R0–R9 colour that cascades from the realm it sits in. The row model
(`lib/orgRows.ts`) numbers the places once they are in reading order, so
a code is a real position among siblings rather than a guess.

**The Storyboard's "across" direction is ours.** It was TimelineJS3
until it earned its way out: a classic script fetched at runtime, every
slide and navigator marker built up front (hence a 60-event cap), a
container measured once so every resize needed a nudge, an icon
`@font-face` that escaped the `.mc-scope` boundary, third-party embed
and analytics code that never ran but shipped anyway, and a `?url`
import the package README had to demand of every consumer's bundler.
`components/time/TimeAxis` keeps the one thing that mattered — an
event sits where its clock says, on an axis you can zoom, so a quiet
morning is empty space and a burst is a cluster. Position is arithmetic
on a scale in pixels per millisecond, so only the cards on screen exist
in the DOM: the cap is gone and the feed can be any length. Clusters
were what beat the old renderer, which stacked them into dozens of rows
until the navigator was taller than the panel; here cards stack into at
most five lanes and the overflow keeps its tick on the axis, so the
density is still visible and zooming in resolves it.

The view is split the way the report page splits its rail from its
sections: the axis is the index along the bottom, and the panel above
it is the page. A card on the axis has to stay small enough that a
burst still reads as a burst, which leaves no room for the event
itself — so choosing one reads it in full above. Beneath the axis, where
the scrollbar is the only other clue how far the feed runs, each end
names its own date and doubles as the way to it. The panel carries a
big arrow on either side, so a reader can walk the story without going
back to the axis for every step — left is earlier and right is later,
the same direction the axis runs, and the axis scrolls to follow
whatever the arrows choose. Dropping the
dependency took `dist/style.css` from 1585 rules to 1193 and from
448 kB to 127 kB, and left the stylesheet with no `@font-face` at all. Dropping the dependency took `dist/style.css` from
1585 rules to 1193 and from 448 kB to 127 kB, and left the stylesheet
with no `@font-face` at all.

**Both timeline drawings are ours, and they are one component.** Down
the page used to be React Chrono, which renders every card up front and
brought its own idea of what a timeline looks like. `TimeAxis` draws
either direction from the same arithmetic with the axes swapped, so the
two agree about spacing, density and colour, and a fix to one is a fix
to both. Across, time runs left to right because that is what a
timeline means; down, it runs newest first, because that is what the
list beside it does and what a feed is. Dropping `react-chrono` on top
of `@knight-lab/timelinejs` leaves the package with no rendering
dependency it does not draw itself, apart from the chart, graph and 3D
libraries.

The Storyboard now offers its three drawings as one strip of three in
the toolbox — a dense list, an axis down, an axis across — rather than
a direction control plus a list-or-timeline switch buried in a panel
header. There were only ever three drawings, and asking for them in two
questions made the reader compose an answer out of parts. The feed also
takes the page: a drawing that scrolls the column instead of itself
loses its axis and its landmarks off the bottom, so the storyboard is
the one place in Outcomes where the column does not scroll. The list
carries day separators and a rail of the days present, which is the
landmark an axis gets for free and a list otherwise has none of.

**A count that offers a filter means the tiles it counted.** Glance's
status counts each filter to their own severity, and the filter belongs
to the page rather than to either drawing, so it survives the toggle
between where things stand and how they have moved. They used to share
two bands between them — critical and warn both meant "attention" — so
pressing one lit the other.

**Sections are tabs, not scroll positions.** A report's sections are its
argument — what happened, why, what needs attention — and each is a
whole answer, so each gets the page to itself rather than a jump link
into one long column. The Narrative briefing works the same way, and
Ask is its last tab: it reads the same tiles and answers the same
questions in another voice, which is a tab rather than a perspective.

**A place exists when anything names it.** Both the maps and the Tree
build their places from the union of two sources — the report scopes
the control plane knows about, and the scope each node reports for
(`lib/orgRows.ts`, and `buildTree` in each map). So a deployment that
publishes its structure as report scopes sees that structure drawn,
whether or not a node sits in any of it, and an empty place renders as
an empty place rather than being absent. Counts are always of nodes, so
an empty place reads 0/0.

**Offline note for TimelineJS3 (the Timeline running across):** the library defaults its
`script_path` to `cdn.knightlab.com` and then fetches a font stylesheet
from there — a real external request, caught by the network audit during
development. The view pins `script_path` to our own bundled asset
directory and passes a falsy `font`, which disables that fetch entirely
(TimelineJS then inherits the page's fonts). Verified: rendering it
makes no external requests. It also ships dormant embed code for
third-party media and analytics — inert for text-only slides, but worth
knowing when comparing it against React Chrono.

## Theming

Light is the default (per DESIGN.md); dark is `[data-theme="dark"]` on
`<html>`. The toggle in the topbar persists an explicit choice under the
`dream.theme` localStorage key; with no stored choice the OS
`prefers-color-scheme` is followed live. Tokens are declared on both
`:root` and the bare `[data-theme="light"]` selector, so any nested
element can force a theme for its subtree — the design docs’
side-by-side theme previews use exactly that.

## Visualization toolset

Report panels render through a small registry
(`ui/packages/mission-control/src/panels/registry.tsx`) that maps a panel's `data_kind` +
`primitive` hint to a renderer, specific → generic:

- **d3** (eager; scale/shape/axis submodules, plus d3-hierarchy in the
  lazy map/topology chunks) — time-series charts with real axes,
  sparklines, span gantts, the Control Plane planet map, and the Outcomes
  scope topology.
- **chart.js** (lazy) — the `gauge` primitive.
- **highlight.js** (lazy) — fenced code inside narrative panels;
  `logbuffer` renders as a mono, level-colored log list.
- **marked** (lazy) — `narrative`/`markdown` panels (rendered from
  tokens with `textContent`, no raw-HTML injection).
- **mermaid** (lazy, the heaviest chunk) — `graph`/`dag`/`topology`
  panels, loaded only when a report actually emits one.
- **React Chrono** (lazy) — the Timeline panel's "timeline" display
  mode; the plain list stays the instant default.

A future media/3D renderer (react-three-fiber) plugs in as one more
registry entry — the slot is documented in the registry file.

## Panels (Control Plane)

- **Org chart** — the worker registry and report-to graph, rendered as a
  tree from whatever root you're viewing. See the depth-3 rollup rule
  below.
- **Live activity** — a live feed over the `GET /api/events`
  server-sent-events stream: worker connects, messages, storage writes as
  they happen.
- **Storage browser** — lists objects in the storage backend under a
  prefix, mirroring `dream storage ls`.

**Selection and the node detail panel.** All three Control Plane variants
share one selection: clicking a node in the tree, the planet map or the
force graph fills `views/overview/NodeDetail.tsx` at the top of the side
column, and Escape or its close button clears it. The panel is the
bridge from *who is this node* to *what has it reported*: identity and
role emoji, status, a `reports_to` link that re-selects the parent,
subtree counts, the node's scope as clickable crumbs, the reports
actually returned for that scope, and one row of links into each built
reporting view plus one of category filters. Every link is built with
`reportingHash`, so they are the same deep links Outcomes already
understands — which is what made naming the views a prerequisite.

Two things worth knowing. The scope comes from connect-time
`metadata.scope`, which is `omitempty` on the wire: a worker that
connected without one gets a plain "nothing to link to" message rather
than broken links. And because `GET /api/reports/summary` aggregates by
scope prefix, a node high in the org chart shows rolled-up tiles rather
than nothing — the empty state is reserved for a scope with genuinely no
reports beneath it.

The worker list itself is fetched once by `hooks/useWorkers.ts` and
passed to all three variants. Each used to fetch `/api/workers`
separately, which meant three copies of the list and three refetches on
every `worker_*` event.

## Outcomes

The Outcomes section (`ui/packages/mission-control/src/views/reporting/`) is a pure client of the
reporting HTTP surface documented in [reporting.md](reporting.md),
written only against the facet contracts — it knows no report by name.

**Deep links.** The path says *where* you are — the view, then the scope
one segment per level. The query says *how you are reading it*, spelled
the way the product spells the concept:

```
#/outcomes
#/outcomes/spookify/music
#/outcomes/spookify/music?category=delivery,failures
#/outcomes/spookify/music?report=incident
#/outcomes/spookify/finance/fp-and-a?report=budget-variance&stance=strategic
#/outcomes/spookify?selected=node:music-playback-01
#/control-plane/spookify/music
#/settings/docs/scope
```

| Parameter | Carries |
| --- | --- |
| `category` | comma-separated multi-select of category filters |
| `role` | comma-separated node roles; narrows the org chart, not the reports |
| `stance` | one of operational / strategic / diagnostic |
| `window` | how far back "recently" reaches — `1w`, `6h`, `3mo` |
| `report` | a definition name: present → the drill-down page, absent → the tile grid |
| `also` | the extra scopes of a multi-select, since a path can only be one place |
| `selected` | what the side panel is showing — `place:<path>`, `node:<id>`, or `report:<def>@<scope>` |

Values stay raw wherever the character is legal, so slashes in a scope
and the `:` of a selection survive intact. Everything is in the
fragment: nothing after `#` reaches the server, so deep links work from
a static file server with no rewrite rule. Unknown hashes fall back to
Control Plane; unknown category names are dropped rather than breaking
the view.

The panel's open/closed state and its width are deliberately *not* in
the URL — they are how you like to work, not what you are looking at, so
they live in localStorage (`sideCollapsed`, `sideWidth`).

**Scope bar.** A breadcrumb built from `GET /api/reports/scopes`:
root ("All") › universe › world › realm › site, with the level name (the
`level` field) as a muted label. Each crumb links to that ancestor
scope; a `▾` dropdown at each crumb switches between siblings, and a
trailing dashed dropdown descends one level. Everything below the bar
aggregates to the selected scope.

**Filters.** Category pills (`logs`, `performance`, `roadmap`,
`delivery`, `failures`, `activity`, `decisions`, `cost`, `quality` — the
standard set from `reporting/contracts/categories.yaml`) toggle a
multi-select; the
dashboard issues one `GET /api/reports/summary?scope=S&category=C`
request per selected category and merges the tiles (no `category`
parameter when none is selected). An instance count for the current
scope sits at the end of the row. The filter row applies to the tile
grid and is hidden on the report page.

**Tiles.** One clickable tile per summary view: definition name and
description, status (a dot always paired with text — `ok`/`warn`/
`critical`), the headline sentence, up to four KPIs, an inline SVG
sparkline when the summary carries one, contributing instance/scope
counts, and category tags. The minimal legal summary — just a name — is
still a perfectly clickable box, per the summary facet contract.

**Report page.** Clicking a tile (or a drilldown chip) loads
`GET /api/reports/report?definition=D&scope=S`: a back link, the
definition name and description, the summary strip (status, headline,
KPIs), then each panel rendered by its `data_kind` — `series` as a
full-width SVG line chart with a min/max/last caption, `table` as a
plain table, `events` as a ticker list, `spans` as horizontal bars on a
shared time axis (rows of positioned divs, no chart library), `kpi` as a
large stat card. Below the panels: a Timeline panel merging
`timeline.events` and `timeline.spans` into one newest-first list with
severity pills, `span` tags, and `milestone` flags (event types listed
in the definition's `facets.timeline.milestones`), and finally drilldown
chips linking to sibling report pages at the same scope.

**Pulse rail.** A compact live feed, always visible in the Outcomes
view (right rail on wide screens, stacked below on narrow), fed by the
same `GET /api/events` SSE stream as Live activity: `report_event`
prepends a severity-dotted row (definition · scope tag, relative time,
capped at 60 rows), `report_instance` adds a subtle "report updated"
row and also refreshes the currently visible summary or report view,
debounced to ~2s. Rows are filtered to the selected scope.

**Live vs. polling.** `GET /api/events` requires an **admin** token
(see the auth section above). With a non-admin token — or whenever the
stream drops — the rail's status pill honestly reads `polling` instead
of `live`, and the visible reporting view refreshes every 15 seconds
instead of reacting to pushes. `offline` means no token is connected at
all.

**Empty states.** With no scopes registered yet, the section explains
how to start: register definitions with
`dream server --reports reporting/library/`, then submit instances —
or run `dream simulate` for a populated demo. A scope whose tiles are
all filtered away says "no reports match" rather than showing nothing.

## A row budget, not a depth cap, for deep org charts

The tree opens to about **25 rows** and no further (`ROW_BUDGET` in
`ui/packages/mission-control/src/views/overview/OrgTree.tsx`), which keeps a dashboard viewing a
large organization from turning into a dense wall of nodes — the Spookify
fixture is 224 workers — per `DESIGN.md`'s "avoid dense dashboards"
principle.

A budget rather than a fixed depth, because depth is the wrong unit: the
same "depth 3" is a handful of rows in a flat org and hundreds in a wide
one. On mount the tree walks breadth-first from the root and opens whole
levels while the running row count still fits, then opens as many
individual parents of the next level as the remainder allows. Everything
else starts collapsed, with its rollup (connected of total beneath it)
and the worst measured status beneath it on the row — so a reader can see
which branch is worth opening without opening it.

The walk runs exactly once per mount, guarded by a ref: a reader's own
expansions must survive the next data refresh.

## Architecture & development

- **Source**: `internal/dashboard/ui/` is an npm workspace root
  (`package.json` with workspaces `packages/*` and `app`; the
  `package-lock.json` and `node_modules` live here too). It has two
  members:
  - `packages/mission-control/` — the dashboard as a standalone,
    unbranded React component library, published as
    `@robotdreams/mission-control` (not yet published — see
    [its README](../internal/dashboard/ui/packages/mission-control/README.md)
    for what publishing means and who consumes it). Source under
    `src/`: `views/` per section, `panels/` for the renderer registry,
    `lib/` + `hooks/` + `state/` for routing, SSE, auth, and theme, plus
    `index.ts` (the public entry) and `style.css`.
  - `app/` — the shipped dashboard: `index.html`, `src/main.tsx`,
    `src/app.css` (the document reset), `src/brand/` (the logo and
    favicon assets), and `vite.config.ts`. It does not depend on the
    library's npm package; a `resolve.alias` (and matching TS `paths`)
    points `@robotdreams/mission-control` straight at the library's
    `src/`, so one `npm install`, one bundler pass, and edits to either
    side hot-reload together, with the same chunking as before the
    split. What the workspace actually publishes is proven separately,
    by `packages/mission-control/smoke/` — a consumer with no alias,
    built against the library's compiled `dist/` and typechecked
    against its emitted `.d.ts` (`npm run smoke`).
  - Root scripts (run from `ui/`): `npm run dev` (the app, aliased to
    source), `npm run build` (typecheck, then the app build), `npm run
    typecheck`, `npm run build:lib` (the library's own build, for
    publishing), `npm run smoke`, `npm run preview`.
- **Build output**: `internal/dashboard/web/dist/` — what the binary
  embeds and serves. It is gitignored, never committed. `make ui-build`
  (also `go generate ./internal/dashboard/`, and the first half of
  `make build`) produces it; CI, the Dockerfile's node stage, and the
  release pipeline (`.goreleaser.yaml` `before.hooks`) each build it
  before compiling Go. Beside it, `web/placeholder/index.html` is the one
  committed file: it keeps the embed pattern satisfied on a fresh clone —
  a `//go:embed` pattern that matches no file is a compile error — and,
  when no build is embedded (`go install`, or `make go-build` without a
  prior UI build), it is the page served at `/dashboard`, saying the
  dashboard was not built and how to build it. `dashboard.Built()`
  reports which of the two is embedded; a build never modifies a tracked
  file. `make ui-clean` drops `dist/`.
- **API base injection**: the server replaces the `<!--DREAM_API_BASE-->`
  comment in the built `index.html` (see `internal/dashboard/server.go`);
  a Vite plugin fails the build if the marker ever goes missing, and
  `TestIndexInjectsAPIBase` guards the Go side.
- **Dev loop — one command**:

  ```sh
  go run ./cmd/dream simulate --ui-dev
  ```

  boots the simulation AND the Vite dev server wired to it: the `/api`
  proxy targets the sim, the admin token rides in as
  `VITE_DREAM_DEV_TOKEN`, and the dev UI connects itself — no dialog,
  nothing to paste. Open the URL Vite prints; edits under either
  `ui/packages/mission-control/src/` or `ui/app/src/` hot-reload
  instantly; one Ctrl-C stops everything. (Manual alternative: start any
  server, then `DREAM_API=http://127.0.0.1:<port> make ui-dev` and paste
  a token — leave the dialog's base URL empty so requests go through the
  proxy.) Before opening a PR that touches the UI, run `make ui-check`
  (eslint, prettier, tsc, the library build and its smoke consumer) — it
  is what CI runs.
- **Brand assets**: `ui/app/src/brand/logo-animated.webp` (the blinking
  logo, downscaled) and the favicon derive from the canonical logo files
  in the acumen-web repo (`src/robotdreams/assets/`) — re-copy from
  there if the mark ever changes. Under `prefers-reduced-motion` the
  topbar swaps in the static still (`logo-still.png`), since animated
  WebP ignores that media query.

### Using Mission Control in another app

Everything above this section describes `internal/dashboard/ui/app`, the
one build the `dream` binary embeds. The same UI is also
`@robotdreams/mission-control`, an installable library — see
[its README](../internal/dashboard/ui/packages/mission-control/README.md)
for the full API. A host that owns its own routing and already has a
connection token typically wants one view, not the whole shell:

```tsx
import "@robotdreams/mission-control/style.css";
import {
  MissionControlProviders,
  PageControlsProvider,
  PageChrome,
  ReportingView,
  RouterProvider,
} from "@robotdreams/mission-control";

<div className="mc-scope">
  <MissionControlProviders token={hostToken} apiBase={hostApiBase}>
    <RouterProvider route={route} onNavigate={handleNavigate}>
      <PageControlsProvider chrome={<PageChrome />}>
        <ReportingView />
      </PageControlsProvider>
    </RouterProvider>
  </MissionControlProviders>
</div>
```

The wrapping `mc-scope` element is where the library's rules and theme
tokens apply, and doubles as a natural `themeTarget` if the host wants
the view themed independently of the rest of its page.
