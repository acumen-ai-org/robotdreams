# Mission Control library — design notes

## routes and state

- `VIEW_SLUGS` maps the URL slug (`control-plane`) to the internal view name (`overview`) because `dream.variant.<view>` storage keys use the internal names; renaming them would silently reset every reader's chosen rendering.
- `crossViewRoute` carries scope, category, roles, stance and window across a view switch (they describe what you are reading), but drops `report` and `section` (they are pages).
- `Route.scopes` is a union, not an aggregate: the API aggregates within one scope, so sibling scopes are fetched separately and shown side by side.
- `TimeWindow` is a count plus a unit rather than two instants so a shared link stays true as the page ages; `windowSince` walks the calendar for months and does arithmetic for the rest.
- `Selection` lives in `lib/selection.ts`, not the React context, because `lib/routes` must parse and format it without importing React.
- `readSSE` streams over `fetch` + `ReadableStream` because `EventSource` cannot send the `Authorization` header the API requires.
- `setStoragePrefix` lets a host change the `dream.` localStorage prefix once before first render; `readStored`/`writeStored` never throw so blocked storage degrades to defaults.
- `useDisplayPref` shares a preference between every mounted reader via a window event, because the `storage` event only fires in other tabs; `allowed` guards against a stored mode that no longer exists.
- `usePeriod` is local React state remembered in storage, not a route parameter: a period is a way of reading whatever the route points at, and a deep link should open Live.
- `periodQuery` always starts with "&" because every reporting URL already carries `scope=`.
- `lib/plan.ts` never matches a state, lane or level name against a literal; `flowPosition` (an item's fractional position in its own declared order) is the only thing comparable across plans with different vocabularies.
- `unionStates` merges declared orders topologically; on a cycle it falls back to average `flowPosition` and sets `forced` so the page can say the order is a reading, not a fact.
- `daysLate` is computed client-side because `pkg/reporting` has no clock.
- `triageItems` orders blocked, most-late, soonest-due, title — deliberately unlike the server's stable keep-order — so a per-column cap keeps the cards a reader would look at first.
- `sharedHorizons` requires identical horizon vocabularies, not an intersection: a partial overlap is not a shared axis.
- `bandTiles` groups critical and warn into one band (same response: look now) and keeps no-data tiles out of Steady; `RANK` sorts unrecognised statuses as no-data, not ok.
- `normalisedVariance` divides by the target and flips sign by `direction` so tiles with different units rank on the same scale.
- `Movement.pct` is undefined when the opening value is zero: "up from nothing" has no percentage.
- `rollupOf` returns null when the summary carries no contributor data, and reports only a leaf count (no levels) when it carries only a count.
- `safeAppURL` re-checks `SAFE_SCHEMES` next to the click even though the server already refuses other schemes: an href is the one place worker text is actuated, and the server may be older.
- `UPDATE_PHASES` puts `unknown` first (told, not yet answered) and keeps `declined` out of the danger tone: refusing an update is a legitimate answer under the contract; `current` folds into `unknown`.
- `placeRows` creates a place when anything names it (a report scope or a worker) so a fleet without scope entries still has structure; `MAX_PLACE_DEPTH` stops at site, the universe is not a row.
- `numberPlaces` assigns codes after `sortRows` because a code is a position among siblings; the realm index (0-based, `R0`–`R9`) is copied down so its colour cascades.
- `pointValues` accepts raw numbers or `value`/`v`/`y` keys because the `SeriesPoint` wire shape is not pinned down in docs/reporting.md.
- `LEVEL_NAMES` is indexed from the universe at 0 while a path's depth counts the universe as 1; `levelName` in `aggregation.ts` carries the offset and says "scope" past the named rungs.
- `PlanItem.state` outside the declared order is drawn in a trailing unplaced column rather than dropped; the server rejects it at ingest, so this is belt and braces.
- `Summary.recent` distinguishes absent (no `?since=` asked) from zero (nothing lately).
- `NODE_ROLE_EMOJI` falls back to ⬡ so an unmapped role is never invisible; `WorldBadge` fill alternates by parity as rhythm, never meaning.
- `ChipText` shows the name when there is one and the code otherwise; the code stays in the accessible name and the popover, never inline, because revealing it resized the chip under the pointer.
- `ConnectionProvider` keeps credentials in a ref so `apiFetch` stays referentially stable; `session` is the render-visible signal that they changed.
- `initialCreds` precedence: stored paste, then the host's `token`, then `fallbackToken` (which always talks to the same origin, i.e. the dev proxy).
- `ConnectionProvider.persist` defaults to false so the library stores nothing unless the shipped app turns it on; a host with its own token handles 401 via `onUnauthorized`.
- `HashRouterProvider` and `RouterProvider` are the two implementations of one contract so views never touch `window.location`; `navigate({replace})` is for live previews, and `replaceState` fires no `hashchange`, so the listener is called by hand.
- `useSelection` is a hook over the router, not a provider: selection is part of what you look at, and selecting navigates with `replace` so map clicks do not fill the back button.
- `LiveProvider` opens the stream once; without one a view reads the frozen `INERT` feed and shows no live decoration rather than an error.
- `PageControlsProvider` closes its one slide-down panel on route change, Escape and pointer-down outside; closing is cancelling, and the trigger marker keeps the opener's own toggle from reopening it.
- `registerSlot` throws when two slot hosts (a `Topbar` and a `PageChrome`) register under one provider, because they would silently steal each other's portal target.
- `ThemeProvider` with a `target` always writes `data-theme` (a dark host page must not be inherited); on the document, dark sets the attribute and light removes it.
- `ThemeProvider` follows OS preference changes only while uncontrolled and unchosen; a controlled theme is never written locally so it cannot fight the prop.
- `useThemeRoot` never throws without a provider because panels render standalone in tests and docs specimens.
- `VocabularyProvider` merges stored levels over `DEFAULT_LEVELS` so an older stored file cannot shorten the vocabulary; codes (W01, R4, S017) do not follow a rename because they are identity, the word is presentation.
- `useShell` falls back to local state without a provider so a standalone view's collapse and perspective controls still work.
- `MissionControlShell.lastMain` is updated during render, not in an effect, because an effect would run after the connection redirect and remember the wrong view.

## theme and CSS scoping

- Every `style.css` rule carries `.mc-scope`; base element rules double it (`.mc-scope.mc-scope h2`) to outrank component classes, so a class rule on an h2 must double it too or silently lose.
- `theme.css` is the opt-in palette: every token `style.css` reads is declared on `.mc-scope`, never `:root`; a host leaves it out and maps the same tokens to its own system.
- Dark is `[data-theme="dark"]` on the scope, an ancestor or a nested override; the dark rules come first so an explicit light on the scope outranks an inherited dark by source order.
- `postcss-mc-scope.js` confines vendor sheets (highlight.js) to `.mc-scope`, rewrites `:root`/`html`/`body` to the scope, namespaces `@keyframes` (global by nature), and is idempotent over already-scoped rules.
- The library build emits one module per source file (a consumer re-splits the lazy views), inlines vendor CSS into `style.css`, and copies `theme.css` out via `emitTheme` so the palette stays opt-in.
- The app applies the same scope plugin to everything except `app/src/`, because `app.css` is deliberately the document around the scope; `app/vite.config.ts` fails the build if `<!--DREAM_API_BASE-->` goes missing.
- `--surface-head` and `--selected` are their own tokens: "darker" points opposite ways per theme, and selection must read differently from links (`--brand-strong`), focus (`--focus`) and status.
- `--site-bg` is grey rather than a realm tint because a site sits inside its realm and borrowing the colour left them distinguishable only by silhouette.
- Colour never carries meaning alone: status bars carry a word badge, commitment is border weight, `is-declined` is never the danger tone; check boards in greyscale.
- `.mc-status-dot` is blockified because it is also drawn in table cells and inline text, where width and height on an inline box paint nothing but a sliver.
- Drawings (`.mc-panel-fill`, the storyboard) take the column's height and pan inside it, and height is passed down every link of the chain explicitly, so a four-thousand-pixel SVG never scrolls the page.
- Fonts are the host's to bring: `style.css` names only `--font-*` tokens with a system fallback; the shipped app self-hosts them to stay offline-capable.
- `eslint.config.js` turns the React Compiler rules off (no compiler in this build) and `react-refresh/only-export-components` off because each `./state/*` module co-locates context, provider and hook.
- `smoke/` consumes the built package through the exports map from `dist/` with no alias, under a controlled theme and its own router, so `npm run smoke` proves what is published.

## panels and primitives

- `Async` keeps pending (null), error, `NoMatches` (a filter emptied it; `clearTo` is a real link) and `Onboarding` (nothing configured) apart, so a healthy fleet's first round-trip never shows onboarding.
- `DelayedLoading` paints nothing for `delayMs` (500 ms) so a fast fetch never flashes a skeleton; pair it with a null-vs-empty pending flag, never `length === 0`.
- `BarExpander` measures every distance against its own element and the chrome top, never the viewport, so a host with chrome of its own still gets the overlay in place.
- `ChipPopover` is fixed-position, portalled to the body, closes on scroll, measures the chip (its wrapper is `display: contents`), and waits `HOVER_DELAY_MS` unless Ctrl is held.
- `DetailPanel` is one component for both views; `active` gates it because both views stay mounted; the opening tab is the view's question and resets on selection change; chrome above the tabs never scrolls.
- `KpiBullet` is inline SVG, not chart.js, and scales so the target sits at two thirds (`TARGET_AT_TWO_THIRDS`) to leave room for an overshoot; with no target it degrades to the plain stat.
- `Link` renders a real anchor with a real href and hands a plain left click to the router, which is what lets a host own the URL.
- `NodePicker` is deliberately not clickable: the scope panel edits a draft that only Apply commits, so a link out of it would skip the decision.
- `PageToolbox` draws no page title (the view selector says it) and renders `WindowControl` itself so no view can forget it; `info` visibility is stored so explanations are dismissed once.
- `ToolboxRenderingToggle` and `ToolboxZoom` gate on `active` because views are hidden-toggled, not unmounted, and a hidden view would otherwise portal into whichever toolbox is on screen.
- `ScopeSelector` edits the live selection with `replace`; Cancel/Escape restore what was selected at open; dropping a place drops everything picked inside it; `MAX_SCOPE_ITEMS` caps the rows drawn.
- `Shell` registers its `.mc-scope` element with the theme so mermaid and chart.js resolve tokens inside the scope; `STREAM_SAID` spells out stream states for the live region.
- `SidePanel` collapses to a named rail; its resize handle is an ARIA window-splitter and publishes the width as a custom property on the view grid, so nothing reaches the document.
- `Tabs` is every strip: `to` makes a tab a link and `variant="switch"` gives links their own semantics instead of the ARIA tab pattern; the marker is one sliding element measured from the tab.
- `ViewSelector` is both the navigation and the perspective strip; every view's toolbox renders it (Admin included) so navigation goes through the route, and choosing a perspective from the bar drops the selection.
- `WindowControl` holds its number as a draft and commits on blur, Enter or a valid change, with `replace`, so typing does not renavigate mid-keystroke or fill the back button.
- `useD3Zoom` uses d3-zoom in SVG space, React owns the `transform` attribute, dblclick zoom is removed (it fights click-to-pin), and `zoomTo` returns false until the canvas attaches so `useReveal` can retry.
- `statusDotClass` and `mcVariant` add the `mc-` prefix because the stylesheet's rules are prefixed and wire values are not; `components/shared.test.ts` fails if the prefix is dropped.
- `icons.tsx` and `lib/vocabulary.tsx` inline lucide paths (24x24, currentColor, stroke 2) to keep the library dependency-light.
- `EntryList` and `TimeAxis` are the only two time drawings and both read `TimeEntry`; the list is the default because it is instant, the timeline a lazy chunk.
- `EntryList` sizes the time format to the whole list and captions a single day once; `dayAnchor` ids make "scroll to this day" an anchor lookup.
- `TimeAxis` draws both directions with one `SHAPE`, places events where their clock says, stacks cards greedily into `lanes`, and keeps overflow as ticks so a burst never grows the panel.
- `TimeAxis` opens at `fitZoom` (`OPENING_ROOM_PER_EVENT` of a card per event, capped by `OPENING_ZOOM_CAP`): "everything fits" piles a burst, fitting the closest events stretches the track.
- `TimeAxis` runs left-to-right across the page and newest-first down it, matching the list beside it; the axis, not the card, is the single tab stop and arrow keys walk events.
- `TimeView` always runs its timeline down the page; the across-the-page storyboard is one page's surface choice, offered only in the Storyboard's own control.
- `FactsBox`, `HierarchyBox` and `TypeIcon` give every selectable kind one header shape; hierarchy entries select rather than navigate, and `PEEK` bounds the children listed.
- `NodePanel` reaches Outcomes through what the node wrote (tiles whose contributors name it); tiles without contributors are left out rather than guessed.
- `PlacePanel` is the one place implementation; the view's question picks the opening tab, not the component; `PlaceNodes` rows select the node so walking never leaves the panel.
- `ReportPanel` distinguishes the level a report is read at (aggregate vs. the instance one node wrote) and lists `reportLevels` as its views.
- `ViewBar` sets the variant before navigating (a view reads its rendering as it mounts) and offers only renderings `targets.ts` says can show the kind: Network holds no place, Outcomes draws no worker.
- `PANEL_RENDERERS` resolves specific to generic so an unknown planning primitive still draws as a `Board`; `Burndown` renders a series because a plan has no item identity between instances.
- `panelIsWide` lives in the registry, not the page, so every layout surface agrees: board, lanes, burndown, time axes, tables and logs take full width; `checklist` is deliberately narrow.
- `Board` draws every declared column (an empty column is a fact), caps cards at `CARDS_PER_COLUMN` after `triageItems`, folds lanes into chips past `MAX_LANES`, and keys cards on (scope, id).
- `Checklist` uses a glyph plus sr-only word, not a checkbox, because the reporting layer never authors plans; terminal is the last declared state, never a state named "done".
- `Funnel`, `WaterfallChart` and `RankedBarsChart` read tables through `tableData.ts`: the `scope` column the aggregator appends is never a label or value, rows are summed per label in first-appearance order.
- `Heatmap` serves `status_matrix` too: axes are the first two non-numeric columns, the value the first numeric; roll-up duplicates sum numbers and let the worst status win.
- `RoadmapLanes` draws commitment as border weight (solid/outlined/dashed) read off the plan's declared order, because colour is reserved for status; the unscheduled column and unassigned row appear only when occupied.
- `MermaidDiagram` draws a dag from a plan's `blocked_by` edges, blocker to blocked, and drops edges whose target is outside the instance rather than inventing a node.
- `Narrative` uses type predicates over marked tokens because `Tokens.Generic` never narrows on `type`.
- `ChartCanvas` owns every chart.js instance: registers only the pieces used, destroys on unmount, and rebuilds on theme change by watching the theme root.
- `chartColors` reads CSS custom properties inside render, never at module scope (the stylesheet may not have applied), so charts follow light/dark without a palette of their own.
- `LineChart` averages a merged series per instant and says so; its reference line is the target or else the mean, because a trend without a reference cannot be read.
- `Burndown` draws the ideal line from the first observation to zero at window end and projects from the slope over the whole span, not the last two points.
- `WaterfallChart` leaves bars as reported and names the gap when steps do not reach the closing total; rescaling would draw a number nobody measured.
- `RankedBarsChart` uses one colour with the leader emphasised, labels every bar (`autoSkip: false`) and names how many rows it truncated.
- `Ticker` converts to `TimeEntry` and hands off to `EntryList` so an events panel and a report timeline never look like two different lists.

## hooks

- `useEventStream` holds `onEvent` in a ref, and `useLiveState` puts only the stable `traffic.bump` in its handler deps, so a handler change never tears the SSE connection down.
- `useLiveState` records one activity entry per announcement (not per recipient) and puts worker ids on each entry so the side panel can narrow the feed, since `detail` is a rendered sentence.
- `useLiveState` treats `worker_role_changed` like any other announcement (an entry plus an `orgVersion` bump) and offers no way to cause one: a role is changed with `dream worker edit`, and the dashboard only ever GETs, so reflecting the change is the whole contract.
- `useReveal` moves the view to the selection only on arrival (view became visible, or selection changed under it); a view that made the change bumps `quiet`; `reveal` returning false is retried rather than recorded.
- `useRollout` does not `logout` on 403: the fleet view is admin-only while the rest of the dashboard is not, so it sets `adminOnly` and renders that as a normal state.
- `useScopes` keys its fetch on the formatted window string, not the object, so a re-render does not refetch.
- `useStorage` sits above `StorageTable` so the side panel's tab label can count objects without a second request.
- `useTraffic` keys edges unordered (`edgeKey`) because messages go up and org edges draw down; weights decay each `DECAY_MS` tick and drop below `FLOOR`, so width is a rate.
- `useWorkers` seeds `pending` from whether it will fetch so first paint is loading, not "no workers"; a worker whose manager is filtered out becomes a root so it stays visible.
- `useWorkers.matches` dims rather than drops non-matching workers (a hierarchy with its middle removed stops being one); `placeInScope` dims places by scope only, never by role.
- `useWorkers` memoises one index per set of facts; a fresh object per render re-ran the outline's fold effect and shut expanded branches.
