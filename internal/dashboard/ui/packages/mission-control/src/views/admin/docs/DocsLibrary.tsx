import * as panelModule from "../../../panels/index";
import * as iconModule from "../../../components/icons";

interface Entry {
  name: string;
  what: string;
}

interface Layer {
  id: string;
  title: string;
  q: string;
  subpath: string;
  entries: Entry[];
}

const LAYERS: Layer[] = [
  {
    id: "app",
    title: "The whole app",
    q: "I want Mission Control as it ships, under my token.",
    subpath: "@robotdreams/mission-control",
    entries: [
      {
        name: "MissionControl",
        what: "Topbar, all three sections, hash routing, live feed and connection state. A token is enough; brand puts your mark in the topbar.",
      },
      {
        name: "MissionControlShell",
        what: "The three views inside the Shell, plus the two rules only the whole app knows: Admin floats over the view it was opened from, and a reader with no token is sent to the connection page. Titled Mission Control unless told otherwise.",
      },
      {
        name: "MissionControlProviders",
        what: "Every context and nothing rendered: for a host with its own routing, connection and chrome that still wants the views to work.",
      },
    ],
  },
  {
    id: "chrome",
    title: "Chrome",
    q: "I own the page; I want the toolbox slots a view expects.",
    subpath: "./components/*, ./state/PageControlsContext",
    entries: [
      {
        name: "Shell",
        what: "The basic shell: skip link, topbar with its toolbox slots, the live-status region, and your children. No product name unless you pass one, no theme switch or gear unless you put them in actions, no views, no redirect.",
      },
      {
        name: "ThemeToggle · AdminLink",
        what: "The two global actions Mission Control puts at the topbar's right edge, side by side: light/dark, and the gear whose corner dots carry the stream state and a pending rollout. They are the product's choices, not the chrome's — pass them to Shell's actions, or your own, or none.",
      },
      { name: "Topbar", what: "Brand, the one bordered box of page controls, theme toggle and the gear." },
      {
        name: "PageChrome · PageControlsProvider · usePageControls",
        what: "The slots a view fills — perspective, filters, zoom — and where they render.",
      },
      { name: "PageToolbox · ViewSelector", what: "The controls box itself, and the view switcher inside it." },
    ],
  },
  {
    id: "views",
    title: "Views",
    q: "I want one of the three pages inside my own shell.",
    subpath: "./views/*",
    entries: [
      {
        name: "OverviewView",
        what: "Control Plane: the org as tree, force graph, planet or galaxy, with the side panel for the selection.",
      },
      { name: "ReportingView", what: "Outcomes: glance board, timeline, charts, topology, narrative, plan, ask." },
      { name: "SchedulesView", what: "Schedules: every cron schedule on the control plane, by the worker it wakes." },
      {
        name: "AdminView · ConnectionPage",
        what: "This dialog, and the paste-a-token page it opens with. ADMIN_SECTIONS is the menu, exported so a host can filter it.",
      },
    ],
  },
  {
    id: "subviews",
    title: "Sub-views",
    q: "I want one rendering or one control, not the page around it.",
    subpath: "./views/overview/*, ./views/reporting/*",
    entries: [
      {
        name: "OrgTree · ForceGraph · PlanetMap · GalaxyMap",
        what: "The four renderings of the org. ForceGraph is d3, PlanetMap and GalaxyMap are three.js.",
      },
      {
        name: "NodeDetail · ScopeDetail · PanelSummary · ActivityList · StorageTable · AppBox · RolloutPanel · OverviewLegend",
        what: "What the side panel shows for a node or a scope, piece by piece.",
      },
      { name: "GlanceBoard · ReportPage", what: "The board of signal tiles, and the page one report opens into." },
      {
        name: "ReportingTimelineView · ReportingChartsView · ReportingTopologyView · ReportingNarrativeView · ReportingPlanView · ReportingAskView",
        what: "The other six perspectives, each a full view over the same reports.",
      },
      {
        name: "PulseRail · SignalTile · ScopeBar · StanceControl · StanceFilter · CategoryFilters · ReportingControls · PeriodControl",
        what: "The tiles and the controls that scope, filter, re-stance and re-period them.",
      },
    ],
  },
  {
    id: "panels",
    title: "Panels",
    q: "I have a report panel and want it drawn.",
    subpath: "./panels/*",
    entries: [
      {
        name: "PanelSection · PanelBody · panelIsWide",
        what: "The frame a panel sits in, and the registry's answer to whether it wants the full width.",
      },
      {
        name: "panels.*",
        what: "Every renderer, by name, listed below. All take { panel }; the registry picks one from data kind and primitive so a host rarely needs to.",
      },
    ],
  },
  {
    id: "primitives",
    title: "Primitives",
    q: "I am composing my own view and want the same parts.",
    subpath: "./components/*",
    entries: [
      { name: "Link", what: "A real anchor that routes plain left clicks through whichever router is active." },
      { name: "SidePanel · Tabs · TabPanel", what: "The resizable side panel with its bounds, and tabs." },
      {
        name: "Async · DelayedLoading · Skeleton · TileSkeletons · NoMatches · Onboarding",
        what: "Loading, empty and first-run states, so a host's copy of a view fails the same way.",
      },
      {
        name: "ScopeSelector · ScopeChip · SearchControl · ChipPopover · NodePicker · BarExpander",
        what: "Choosing a scope, a node, a search — the controls a page puts in its toolbox.",
      },
      {
        name: "ToolboxZoom · ZoomControls · KpiBullet · StatusBadge · SevPill · KpiStat · TickerItem · OutcomesLink",
        what: "Small parts the renderings share.",
      },
      {
        name: "TimeView · TimeModeSwitch · EntryList · TimeAxis",
        what: "Time-ordered entries: a dense list, or a zoomable time axis that runs either way — down the page or across it. Both are ours.",
      },
      { name: "icons.*", what: "The inlined icon set, listed below." },
    ],
  },
  {
    id: "state",
    title: "Contexts",
    q: "I need the views to know about my token, my URL, my theme.",
    subpath: "./state/*",
    entries: [
      {
        name: "ConnectionProvider · useConnection · ApiError",
        what: "Token, API base, and one fetchImpl every request goes through — including the event stream.",
      },
      {
        name: "HashRouterProvider · RouterProvider · useRouter · useRoute",
        what: "The route in the fragment, or supplied by the host with an onNavigate callback.",
      },
      {
        name: "ThemeProvider · useTheme · useThemeRoot",
        what: "Dark or light on <html>, or on a target element so a container carries its own. Uncontrolled it remembers the reader's choice and follows the OS until they make one. Pass theme and onThemeChange and the host owns it instead: the library stores nothing, and the toggle beside the gear becomes a request.",
      },
      {
        name: "LiveProvider · useLive · ShellProvider · useShell · VocabularyProvider · useVocabulary",
        what: "The live feed, the shell's remembered state, and the deployment's words for its levels.",
      },
    ],
  },
  {
    id: "hooks",
    title: "Hooks",
    q: "I want the data the views read, without the views.",
    subpath: "./hooks/*",
    entries: [
      {
        name: "useWorkers · useScopes · useApps · useRollout · useTraffic · useSchedules",
        what: "The org index, the scope tree, the app registry, the update rollout, message traffic, and the control plane's schedules.",
      },
      {
        name: "useEventStream · useLiveState",
        what: "The SSE feed over fetch with reconnect, and the state it keeps current.",
      },
    ],
  },
  {
    id: "lib",
    title: "Library",
    q: "I want the rules — routes, formats, triage — as plain functions.",
    subpath: "./lib/*",
    entries: [
      {
        name: "routes · variants · display",
        what: "The URL grammar and its builders, the seven perspectives, the remembered display preferences.",
      },
      {
        name: "format · types · vocabulary · triage · updates · apps",
        what: "Number and time formatting, the API types, the badges and codes per level, severity ranking, rollout phases, app URLs.",
      },
      {
        name: "storage · themeRead · sse · period",
        what: "setStoragePrefix() for more than one instance on a page, the resolved theme tokens, the SSE parser, and the reporting period (usePeriod, periodQuery).",
      },
    ],
  },
];

const BRING = [
  [
    "React 19",
    "react and react-dom are peer dependencies. Everything else — chart.js, d3, three, mermaid, marked and highlight.js — the package brings.",
  ],
  [
    "The stylesheet, once",
    'import "@robotdreams/mission-control/style.css" — structure only. Every rule in it is nested under .mc-scope, every class the library owns carries the mc- namespace, and there is no :root, html or body rule in the file. Wrap a view or panel in mc-scope so the rules apply.',
  ],
  [
    "A palette, or your own tokens",
    'import "@robotdreams/mission-control/theme.css" for Mission Control\'s own colours, light and dark. It declares them on .mc-scope, not :root. Leave it out and declare the same --tokens on your scope element instead, mapped to your design system — the structural sheet reads them and defines none.',
  ],
  [
    "Fonts",
    "The stylesheet only names --font-heading, --font-body and --font-mono, with system fallbacks. The shipped app installs Inter, Space Grotesk and JetBrains Mono.",
  ],
  [
    "A bundler that resolves an exports map",
    'moduleResolution: "bundler" (or node16) in tsconfig; Vite does the rest.',
  ],
  [
    "A way to reach the control plane",
    "Same-origin, a proxy, or fetchImpl. The server sends no CORS headers, so a host whose page origin is not the control plane's must pick one of the other two.",
  ],
];

const HOSTS = [
  {
    host: "A Vite or Next.js app, same origin as the control plane",
    how: "The plain case. MissionControl with a token, or the views inside your shell. The shipped dashboard is exactly this.",
  },
  {
    host: "A page on another origin",
    how: "Put the control plane behind a proxy that also serves the page, or one that adds CORS. apiBase then points at it.",
  },
  {
    host: "Tauri",
    how: "Yes: a Tauri window is a system webview running your React bundle, and nothing here needs Node or a DOM the webview lacks. The window's origin (tauri://localhost) is not the control plane's, so pass the Tauri HTTP plugin's fetch as fetchImpl — it skips CORS and streams, so the event feed works too. Keep apiBase absolute.",
  },
  {
    host: "Electron",
    how: "Same as Tauri: the renderer is Chromium, so either relax CORS in the session's webRequest or pass a fetchImpl that goes through the main process.",
  },
  {
    host: "A Vue, Svelte or plain page",
    how: "Mount React into one element with createRoot and render MissionControl or a view there. The library has no framework-neutral build.",
  },
];

function names(mod: object): string[] {
  return Object.keys(mod)
    .filter((k) => k !== "default")
    .sort((a, b) => a.localeCompare(b));
}

const PANEL_NAMES = names(panelModule);
const ICON_NAMES = names(iconModule);

export default function DocsLibrary() {
  return (
    <>
      <section className="mc-admin-section">
        <div
          className="mc-lib-diagram"
          role="img"
          aria-label="Nested boxes: the whole app contains Mission Control's shell, which contains the unbranded Shell, which contains views, which contain sub-views, which contain panels and primitives; contexts, hooks and the library sit beside every level."
        >
          <div className="mc-lib-nest">
            <span className="mc-lib-nest-title">MissionControl</span>
            <span className="mc-lib-nest-sub">the whole app · one token</span>
            <div className="mc-lib-nest">
              <span className="mc-lib-nest-title">MissionControlShell</span>
              <span className="mc-lib-nest-sub">the three views · Admin over the view · connection redirect</span>
              <div className="mc-lib-nest">
                <span className="mc-lib-nest-title">Shell</span>
                <span className="mc-lib-nest-sub">
                  unbranded chrome: skip link · topbar · toolbox slots · live status
                </span>
                <div className="mc-lib-nest">
                  <span className="mc-lib-nest-title">OverviewView · ReportingView · SchedulesView · AdminView</span>
                  <span className="mc-lib-nest-sub">a view inside your own page</span>
                  <div className="mc-lib-nest">
                    <span className="mc-lib-nest-title">OrgTree · GlanceBoard · ReportPage · …</span>
                    <span className="mc-lib-nest-sub">a rendering or a control</span>
                    <div className="mc-lib-nest mc-lib-nest-leaf">
                      <span className="mc-lib-nest-title">panels.* · Link · Tabs · icons.*</span>
                      <span className="mc-lib-nest-sub">panels and primitives</span>
                    </div>
                  </div>
                </div>
              </div>
            </div>
          </div>
          <div className="mc-lib-side">
            <span className="mc-lib-side-title">beside every level</span>
            <span className="mc-lib-side-item">contexts</span>
            <span className="mc-lib-side-item">hooks</span>
            <span className="mc-lib-side-item">lib</span>
          </div>
        </div>
        <span className="mc-provenance">source: internal/dashboard/ui/packages/mission-control · src/index.ts</span>
      </section>

      <section className="mc-admin-section">
        <h2>One package, ten levels</h2>
        <p className="mc-body-muted">
          This dashboard is <code className="mc-mono">@robotdreams/mission-control</code>, an unbranded React library
          the <code className="mc-mono">dream</code> binary happens to embed. A host takes the outermost box it wants to
          own and leaves the rest to the library: the whole app under its own token, Mission Control's shell with its
          three views, the bare unbranded Shell around views of its own choosing, one view in its own shell, or one
          panel in its own page. Everything is exported from the root entry; the subpaths exist so a bundler can take
          less.
        </p>
      </section>

      {LAYERS.map((l) => (
        <section className="mc-admin-section" key={l.id}>
          <h2>{l.title}</h2>
          <p className="mc-body-muted">
            <em>{l.q}</em> — <code className="mc-mono">{l.subpath}</code>
          </p>
          <div className="mc-storage-table-wrap">
            <table className="mc-storage-table mc-lib-table">
              <thead>
                <tr>
                  <th scope="col">Export</th>
                  <th scope="col">What it is</th>
                </tr>
              </thead>
              <tbody>
                {l.entries.map((e) => (
                  <tr key={e.name}>
                    <td className="mc-mono mc-lib-name">{e.name}</td>
                    <td className="mc-body-muted">{e.what}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          {l.id === "panels" && (
            <p className="mc-lib-roster">
              <span className="mc-lib-roster-label">{PANEL_NAMES.length} renderers</span>
              {PANEL_NAMES.map((n) => (
                <code className="mc-mono mc-lib-roster-item" key={n}>
                  {n}
                </code>
              ))}
            </p>
          )}
          {l.id === "primitives" && (
            <p className="mc-lib-roster">
              <span className="mc-lib-roster-label">{ICON_NAMES.length} icons</span>
              {ICON_NAMES.map((n) => (
                <code className="mc-mono mc-lib-roster-item" key={n}>
                  {n}
                </code>
              ))}
            </p>
          )}
        </section>
      ))}

      <section className="mc-admin-section">
        <h2>What a host brings</h2>
        <ul className="mc-doc-list">
          {BRING.map(([what, why]) => (
            <li key={what}>
              <strong>{what}</strong>
              <p className="mc-body-muted">{why}</p>
            </li>
          ))}
        </ul>
      </section>

      <section className="mc-admin-section">
        <h2>Where it runs</h2>
        <p className="mc-body-muted">
          Anywhere React 19 renders into a DOM. The one thing that changes between hosts is how requests reach the
          control plane, because the library talks to it with <code className="mc-mono">fetch</code> — every call,
          including the event stream, goes through the one <code className="mc-mono">fetchImpl</code> the connection was
          given.
        </p>
        <div className="mc-storage-table-wrap">
          <table className="mc-storage-table mc-lib-table">
            <thead>
              <tr>
                <th scope="col">Host</th>
                <th scope="col">How</th>
              </tr>
            </thead>
            <tbody>
              {HOSTS.map((h) => (
                <tr key={h.host}>
                  <td>{h.host}</td>
                  <td className="mc-body-muted">{h.how}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
        <p className="mc-body-muted">
          Not yet on npm. From a checkout, <code className="mc-mono">npm run build:lib</code> then{" "}
          <code className="mc-mono">npm pack -w @robotdreams/mission-control</code> gives a tarball that installs
          through the same exports map a registry install would; the package README has the rest.
        </p>
      </section>
    </>
  );
}
