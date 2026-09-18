# Mission Control views — design notes

## Control Plane / Explore

- `OverviewView` owns Explore's focus (`exploreFocus`) rather than `ExploreView`, because the side panel and an arriving selection can point Explore at a place; clicks inside the view do not.
- `OverviewView.revealInExplore` focuses a place directly and, for a node, focuses the place it sits in and highlights its row, since a node cannot be a room.
- `OverviewView` fetches report health per world plus the root (a bounded handful), never once for the fleet: a tile's status aggregates every contributor, so one bad squad would paint ninety rows critical.
- `OverviewView` fetches `tilesByScope` only when the reporting outline is showing; no other rendering reads it.
- `OverviewView.onSelect` opens the side panel on a pick and leaves it as the reader left it on a clear, so a selection is never invisible.
- `OrgTree.worstByScope` shows a status badge only for a scope that was queried at exactly that scope; rows below inherit nothing.
- `OrgTree` expands breadth-first and greedily within a level under the row budget, opening as many parents as still fit rather than closing a whole level.
- `OrgTree` runs the initial expansion once, via a ref, when `childrenOf` arrives; a dependency list would refold the reader's own expansions on every refresh.
- `OrgTree` under a scope/role filter shuts branches with no match, opens branches holding one under the same row budget, and restores the pre-filter collapse state when the filter clears.
- `OrgTree` keeps an empty place open under a scope filter (it is what the filter pointed at) but shuts a populated place whose nodes all fail a role filter.
- `OrgTree` searches place names as well as node ids, so "playback" finds the place as readily as its nodes.
- `OrgTree.visible` is derived from the collapse state the rendering uses, because it is the list the arrow keys walk.
- `OrgTree` draws a place mark instead of a status dot for a place: the dot means a node's connection, and a place has none.
- `ExploreView` shows one level big, everything above it in a breadcrumb, and a capped glance (`PEEK`) of the level below inside each box.
- `ExploreView` treats a focus that no longer exists as the universe, not an error: the fleet moved under the reader.
- `ExploreView` draws the breadcrumb separator as its own element; as part of the button it took the hover and rounded ground and read as chips.
- `ExploreView`: the obvious action follows the shape (a box is walked into, a node is read about); the icon beside it is always the other way.
- `NodeDetail` builds Outcomes links from `blankRoute("reporting")` plus the current revision: the links mean "this scope, nothing else" but must stay in the reader's revision.
- `NodeDetail` draws no header; the side panel above it already names the node and carries the clear control.
- `AppBox` sits above the tabs because it is the one thing in the panel acted on rather than read, and folds its preview away when the selection moves.
- `RolloutPanel` counts silence as a first-class phase and makes the counts strip the filter, so "failed 2" is the way to those two.
- `RolloutPanel` shows `PAGE_SIZE` rows and lets the counts, which are also the filter, reach the rest; the side panel holds three things.
- `StorageTable` narrows on `updated_by` for both a node and a place (through the worker's scope), because every write is attributed.
- `Legend` generates its update-phase entries from `UPDATE_PHASES`, so it cannot claim a phase the app does not ship.

## Galaxy

- `PlanetMap` packs by scope path, not `reports_to`: containment comes from scope, reporting is an edge, and a CEO is a node, not a place.
- `PlanetMap` makes the universe the canvas rather than a circle; worlds are the outermost circles and universe-scoped workers sit among them.
- `PlanetMap` tells levels apart by shape (world circle, realm square, site factory) so a plain dot is always a worker.
- `PlanetMap.buildTree` creates containers from report scopes and worker scopes through one `containerFor`, so an empty place is still drawn.
- `PlanetMap` draws non-circular levels in the square inscribed in the packed circle (`inscribedSide`), which inherits the packing's no-overlap guarantee.
- `PlanetMap.contentRegion` declares the circle each shape can host, and `fitToShapes` maps each packing into it with one translation and scale, keeping siblings apart.
- `PlanetMap.gridInto` lays a site's workers out row-major on the factory floor rather than packing them, so rosters compare across sites.
- `PlanetMap.seatLonely` lifts a worker whose scope is a container of places (CEO, VP, lead) into a row at the top, trying top-centre, top-left, top-right in fixed order.
- `PlanetMap` reserves each place's label band inside its own packed circle (`WORLD_BAND`, `SHAPE_SHRINK`), so a name never runs onto a neighbour.
- `PlanetMap.fitLabel` estimates text width at `CHAR_W` per glyph instead of measuring the DOM, which would cost a layout pass per shape per zoom.
- `PlanetMap` decides label legibility in screen pixels (`labelBand` takes the zoom scale), so names appear as the reader zooms in.
- `PlanetMap` label row degrades name → truncated name → code only → `<title>`, never a smudge below `MIN_LABEL`.
- `PlanetMap` paints a site grey (`--site-bg`) so a factory reads as a factory against any realm tint; `INK` is fixed because planets are light in both themes.
- `PlanetMap` switches to `FACTORY_PATH_SMALL` under `FACTORY_DETAIL_AT` screen pixels, where four roof teeth become a grey fringe.
- `PlanetMap` draws shapes and dots in one pass and all labels in a second, so no child can cover its parent's name and no name covers an emoji.
- `PlanetMap` labels carry no pointer events or accessible name; the shape below is already the button with the same `<title>`.
- `PlanetMap` draws the selected node's name on a `rect` plate sized by `CHAR_W`, not `paint-order: stroke`, which traced the letters and grew with zoom.
- `GalaxyMap` shows containment, not orbits: a realm is in its world, and motion made things impossible to compare.
- `GalaxyMap` packs realms on one floor with the same d3 packing as `PlanetMap`, so the two views agree on which realm is big.
- `GalaxyMap.FACTORY_GEO` extrudes `PlanetMap`'s factory outline, so a shape learned in one view is known in the other.
- `GalaxyMap.layout` computes absolute world positions once, because meshes, click-to-fly and the distance test all want them.
- `GalaxyMap.looseRow` floats a container's own workers above its platforms, the same "presiding" placement `PlanetMap.seatLonely` makes.
- `GalaxyMap` keeps realm platforms inside `DISC` and everything under `DOME` of the sphere, so the glass stays glass.
- `GalaxyMap.useNear` gates a factory's floor on camera distance with `NEAR_HYSTERESIS`, so a camera on the threshold does not flicker it.
- `GalaxyMap` opens a world or factory only when chosen (or, for a world, entered), never on nearness: walls dissolving uninvited read as a rendering fault.
- `GalaxyMap.Site` keeps its factory open while the selection is inside it, so picking one of its nodes does not close the walls on that node.
- `GalaxyMap` glass writes no depth and renders last; an open factory's walls stop taking clicks (`IGNORE` raycast).
- `GalaxyMap.Controls` flies by keeping the camera's direction and changes only distance, so the clicked thing does not arrive from a new side.
- `GalaxyMap.SceneLabel` draws names as canvas textures on planes in the scene (far-inside wall for worlds and open factories, near face otherwise), avoiding drei and a font atlas.
- `GalaxyMap.SceneLabel` computes in world space and hands back a local offset; mixing frames put signs at the wrong distance.
- `GalaxyMap` sets `frameloop` to "always" only while active: views are hidden-toggled, and damping, flight and distance tests animate.
- `GalaxyMap` resets the document cursor on unmount, because a pointer-out never arrives for a view swapped mid-hover.
- `GalaxyMap.World` is its own component so the per-frame distance test re-renders one sphere, not the galaxy.
- `GalaxyMap`'s `ClearSelection` is the only way out of a world: once open, a second click goes through the glass.

## Network

- `ForceGraph` draws `reports_to` edges — what the nested views throw away — and stops after `TICKS`, so a settled layout burns no core.
- `ForceGraph.linkWidth` scales edge weight by square root with a `LINK_MIN` floor: a busy edge must not be a black band, a quiet edge must still exist.
- `ForceGraph` caps depth with `levels` because a full fleet is a hairball; an edge is kept only when both ends are.
- `ForceGraph.endId` copes with both id and object link ends, since d3-force rewrites them once the simulation starts.
- `ForceGraph.ancestors` walks the selected worker's chain of command and bails on a cycle; `pathLinks` are the edges of that walk.
- `ForceGraph` draws the chain of command in a second pass behind the nodes, thicker but still scaled by traffic (`PATH_LINK_MIN` floor).
- `ForceGraph` draws update phase as an outer ring, and silence as a dashed ring rather than nothing.
- `ForceGraph` greys an edge when either end is outside the filter, and names every ancestor on a lit chain however deep.

## Outcomes perspectives

- `ReportingView` keeps `scopes` as `null` until fetched; an empty array is a different statement and showed the onboarding card mid-round-trip.
- `ReportingView` requests the product of selected scopes and categories, since the API aggregates within one scope and one category.
- `ReportingView` keys tiles by definition and scope; the same definition at two scopes is two numbers.
- `ReportingView` owns `only`, `stuckOnly` and the period, because more than one drawing or control reads each.
- `ReportingView` fetches the worker index too, so the shared side panel counts a place's nodes the same way it does on the Control Plane.
- `ReportingControls.useBarHasRoom` measures whether the tools area is clipped (open) or the toolbox has spare width (closed) on every resize, with `HYSTERESIS` against flapping.
- `ReportingControls` starts open and records what the open form cost, since there is nothing to compare against before a first draw.
- `ReportingControls` lists `MAX_LISTED` category names and then shows a count; the stance rides in the same control.
- `ReportingControls` applies each toggle immediately and restores `original` on Cancel or Escape, with replace navigation.
- `GlanceRibbon` counts from the whole set and filters to exactly the tiles each count names; hidden boards do not portal counts (`active`).
- `GlanceBoard` scrolls an arriving selected tile into view rather than leaving it ringed below the fold.
- `ReportingChartsView` states movement outright (`Movement` from first to last point when no `delta` is sent), draws a baseline at the window's opening value, and sorts biggest mover first.
- `ReportingChartsView.MovementTag` shows "flat" for zero and drops the percentage past `MAX_SHOWN_PCT`; a series opening near zero produces a meaningless 8800%.
- `ReportingChartsView` cards have fixed chart and KPI heights so a grid of unequal reports still scans.
- `ReportingPlanView` draws one aggregated board whose columns are `unionStates`' merge (topological when vocabularies agree, forced otherwise) and says which one the reader is getting.
- `ReportingPlanView` never maps state names onto a canonical flow; that would require knowing reports by name.
- `ReportingPlanView` fetches only plan-bearing definitions from the tile's facet list, capped at `MAX_DEFINITIONS`, and treats terminal items as unblocked whatever edges they keep.
- `ReportingPlanView` shows pending until `loadedKey` matches `namesKey`, so the empty state appears only after an answer.
- `ReportingTimelineView` owns the one feed fetch and hands it to whichever direction shows; it fetches per category and dedupes on (t, type, scope, label).
- `ReportingTimelineView` tags criticals as milestones because milestone flags are per-definition config the merged feed does not carry.
- `ReportingTimelineView` sends no `since` when a period is set; the period is the feed's bounds.
- `ReportingTimeline2View` receives events as a prop and reverses to oldest first; a story runs forwards.
- `ReportingTopologyView` picks the single real depth-1 scope as root and falls back to the synthetic "All" only when several exist.
- `ReportingTopologyView` skips status dots above `STATUS_SCOPE_CAP` scopes; the fan-out is one summary request per scope, and a server-side group-by is the follow-up.
- `ReportingTopologyView` picks on click and travels from the panel, so the tree stays a map rather than a menu.
- `ReportPage` shows one section at a time, defaults to the first when a stance drops the open one, and orders `next` ahead of Detail.
- `ReportPage` says out loud that stances describe and categories navigate; only the chips that navigate look like links.
- `ReportSources` rebuilds the roll-up tree from contributing scope paths; every rung opens as the same report at that scope, every leaf as its producer's instance.
- `ReportingNarrativeView` and `ReportingAskView` are model-free templates over the same tiles; Ask answers only its fixed repertoire and cites what it read.
- `ReportingNarrativeView.CappedList` is shared by both lists so they cannot drift into different truncation rules.
- `SignalTile` puts the headline largest and the name as a caption; status is a top bar plus a badge, solid only for critical and warn.
- `SignalTile` becomes a button when given `onSelect`, keeping the corner arrow as the link to the whole page.
- `StanceFilter` is single-select with an explicit "Any": a stance is a footing, and two stances argue neither.
- `StanceControl` is a row of links (`switch`, not `tabs`) because it changes the page rather than a panel.
- `PeriodControl` makes Live the first segment, since it answers the same "over what stretch?" question and clears stepping.
- `CategoryFilters` in `compact` never wraps; the bar is a fixed height, and a wrapped row is a vanished row.
- `ScopeBar` uses the server's own level name when given, otherwise the deployment's word for that depth.

## Settings

- `AdminView` is a dialog over the live, dimmed view; Escape, ✕ and the backdrop return to it, and docs load lazily.
- `sections` is one flat list with docs topics inline, so the whole surface is visible without clicking; `id` is the hash.
- `ConnectionPage` is both "sign in" and "change server": the app sends you here when there is no usable token.
- `UpdatesPage` holds the fleet-level rollout; announcing and auditing is administration, not org-chart reading.
- `VocabularyPage` names the levels because they are a labelling convention, not something the control plane enforces.
- `admin/docs/*` and `admin/design/*` render the app's real components and generated tables (`UPDATE_PHASES`, `lib/variants.ts`), so a page cannot claim what does not ship.

## Schedules

- `SchedulesView` is a view, not a reporting perspective: a schedule is what the control plane will do next, grouped by the worker it wakes.
- `SchedulesView` needs no admin credential; `GET /api/schedules` answers with whatever the identity may see.
- `SchedulesView.untilText` says "due" for a passed instant, which happens between a fire and the refetch that moves `next_at`.
