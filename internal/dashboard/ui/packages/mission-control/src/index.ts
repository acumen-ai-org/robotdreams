import "./style.css";

export { MissionControl, type MissionControlProps } from "./MissionControl";
export { MissionControlShell, type MissionControlShellProps } from "./MissionControlShell";
export { MissionControlProviders, type MissionControlProvidersProps } from "./MissionControlProviders";

export { Shell, type ShellProps } from "./components/Shell";
export { Topbar } from "./components/Topbar";
export { ThemeToggle, AdminLink } from "./components/TopbarActions";
export { PageChrome, PageControlsProvider, usePageControls, type PanelId } from "./state/PageControlsContext";
export { PageToolbox } from "./components/PageToolbox";
export { ViewSelector } from "./components/ViewSelector";

export { OverviewView } from "./views/overview/OverviewView";
export { ReportingView } from "./views/reporting/ReportingView";
export { SchedulesView } from "./views/SchedulesView";
export { AdminView } from "./views/admin/AdminView";
export { ConnectionPage } from "./views/admin/ConnectionPage";
export { ADMIN_SECTIONS, GROUP_LABELS, findSection, type AdminSection } from "./views/admin/sections";

export { OrgTree, type OrgTreeMode } from "./views/overview/OrgTree";
export { ExploreView } from "./views/overview/ExploreView";
export { RenderingToggle, type RenderingOption } from "./components/RenderingToggle";
export { default as ForceGraph } from "./views/overview/ForceGraph";
export { default as PlanetMap } from "./views/overview/PlanetMap";
export { default as GalaxyMap } from "./views/overview/GalaxyMap";
export { NodeDetail } from "./views/overview/NodeDetail";
export { ActivityList } from "./views/overview/ActivityList";
export { StorageTable } from "./views/overview/StorageTable";
export { AppBox } from "./views/overview/AppBox";
export { RolloutPanel } from "./views/overview/RolloutPanel";
export { OverviewLegend } from "./views/overview/Legend";

export { GlanceBoard } from "./views/reporting/GlanceBoard";
export { ReportPage } from "./views/reporting/ReportPage";
export { default as ReportingTimelineView } from "./views/reporting/ReportingTimelineView";
export { default as ReportingChartsView } from "./views/reporting/ReportingChartsView";
export { default as ReportingTopologyView } from "./views/reporting/ReportingTopologyView";
export { default as ReportingNarrativeView } from "./views/reporting/ReportingNarrativeView";
export { default as ReportingPlanView } from "./views/reporting/ReportingPlanView";
export { default as ReportingAskView } from "./views/reporting/ReportingAskView";
export { PulseRail, type PulseMode } from "./views/reporting/PulseRail";
export { SignalTile } from "./views/reporting/SignalTile";
export { ScopeBar } from "./views/reporting/ScopeBar";
export { StanceControl } from "./views/reporting/StanceControl";
export { StanceFilter } from "./views/reporting/StanceFilter";
export { CategoryFilters } from "./views/reporting/CategoryFilters";
export { ReportingControls } from "./views/reporting/ReportingControls";
export { PeriodControl } from "./views/reporting/PeriodControl";

export { PanelBody, PanelSection, panelIsWide, type PanelProps } from "./panels/registry";
export * as panels from "./panels/index";

export { Link, type LinkProps } from "./components/Link";
export { SidePanel, SIDE_MIN, SIDE_MAX, clampSideWidth } from "./components/SidePanel";
export { DetailPanel } from "./components/DetailPanel";
export { useSelection } from "./state/SelectionContext";
export {
  formatSelection,
  parseSelection,
  sameSelection,
  selectedID,
  selectionFromID,
  placeID,
  type Selection,
} from "./lib/selection";
export { Tabs, TabPanel, type TabDef } from "./components/Tabs";
export { Async, DelayedLoading, NoMatches, Onboarding, Skeleton, TileSkeletons } from "./components/Async";
export { ScopeSelector, ScopeChip, describePath, type ScopeSeg } from "./components/ScopeSelector";
export { SearchControl } from "./components/SearchControl";
export { ChipPopover, type ChipInfo } from "./components/ChipPopover";
export { BarExpander } from "./components/BarExpander";
export { ToolboxZoom, ZoomControls, type ZoomApi } from "./components/ZoomPan";
export { KpiBullet } from "./components/KpiBullet";
export { NodePicker } from "./components/NodePicker";
export {
  OutcomesLink,
  StatusBadge,
  KpiStat,
  SevPill,
  TickerItem,
  tileName,
  contributionText,
} from "./components/shared";
export * as icons from "./components/icons";
export { TimeView, TimeModeSwitch } from "./components/time/TimeView";
export { EntryList, dayAnchor } from "./components/time/EntryList";
export { TimeAxis, type TimeAxisItem, type TimeAxisDirection } from "./components/time/TimeAxis";

export { ThemeProvider, useTheme, useThemeRoot, type Theme, type ThemeProviderProps } from "./state/ThemeContext";
export { VocabularyProvider, useVocabulary, DEFAULT_LEVELS, type LevelWord } from "./state/VocabularyContext";
export {
  ConnectionProvider,
  useConnection,
  ApiError,
  type ConnectionProviderProps,
  type ConnectionValue,
} from "./state/ConnectionContext";
export {
  HashRouterProvider,
  RouterProvider,
  useRouter,
  useRoute,
  type RouterValue,
  type RouterProviderProps,
  type NavigateOptions,
} from "./state/RouterContext";
export { LiveProvider, useLive } from "./state/LiveContext";
export { ShellProvider, useShell, type ShellState } from "./state/ShellContext";

export { useWorkers, countSubtree, placeInScope, scopeOf, type WorkerIndex } from "./hooks/useWorkers";
export { useScopes } from "./hooks/useScopes";
export { useApps, type AppsState } from "./hooks/useApps";
export { useRollout, type RolloutState } from "./hooks/useRollout";
export { useTraffic, edgeKey, type Traffic, type TrafficApi } from "./hooks/useTraffic";
export { useEventStream, type StreamState } from "./hooks/useEventStream";
export { useLiveState, type LiveState } from "./hooks/useLiveState";
export { useSchedules, type Schedule, type SchedulesState } from "./hooks/useSchedules";

export {
  parseHash,
  buildHash,
  blankRoute,
  adminHash,
  crossViewHash,
  viewHash,
  reportingHash,
  adminRoute,
  crossViewRoute,
  viewRoute,
  reportingRoute,
  underScope,
  underAnyScope,
  CATEGORIES,
  STANCES,
  STANCE_QUESTIONS,
  LEVEL_NAMES,
  ADMIN_DEFAULT,
  type Route,
  type ViewName,
} from "./lib/routes";
export {
  VIEW_VARIANTS,
  getVariant,
  setVariant,
  perspectivesFor,
  perspectiveOf,
  variantForPerspective,
  aboutVariant,
  type VariantDef,
  type Perspective,
} from "./lib/variants";
export {
  useTimeMode,
  useTimeOrientation,
  useDisplayPref,
  useExploreMode,
  useSpaceMode,
  useNodeListMode,
  DISPLAY_KEYS,
  type TimeMode,
  type TimeOrientation,
  type ExploreMode,
  type SpaceMode,
  type NodeListMode,
} from "./lib/display";
export {
  placeRows,
  reportRows,
  rowInSelection,
  MAX_PLACE_DEPTH,
  type Row,
  type RowKind,
  type RowTree,
} from "./lib/orgRows";
export {
  usePeriod,
  periodQuery,
  periodCaption,
  stepAt,
  PERIOD_KINDS,
  PERIOD_LABELS,
  type PeriodKind,
  type PeriodSelection,
  type PeriodApi,
} from "./lib/period";
export { setStoragePrefix, storageKey } from "./lib/storage";
export { resolveTheme, cssVar } from "./lib/themeRead";
export * from "./lib/format";
export * from "./lib/types";
export {
  entityCode,
  nodeCode,
  nodeEmoji,
  UniverseIcon,
  SiteIcon,
  WorldBadge,
  NodeBadge,
  SiteBadge,
  RealmChip,
  REALM_COLORS,
  NODE_ROLE_EMOJI,
  NODE_DEFAULT_EMOJI,
} from "./lib/vocabulary";
export { readSSE, type SSEHandler } from "./lib/sse";
export { appsByWorker, safeAppURL, appHost, type NodeApp } from "./lib/apps";
export { worstVariance, severityRank, normalisedVariance, seriesMovement, bandTiles, statusCounts } from "./lib/triage";
export {
  latestByKind,
  phaseOf,
  phaseClass,
  UPDATE_PHASES,
  type Announcement,
  type Rollout,
  type RolloutNode,
  type UpdatePhase,
} from "./lib/updates";
