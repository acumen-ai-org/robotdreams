import { Component, Suspense, lazy, type ComponentType, type ReactNode } from "react";
import type { Panel } from "../lib/types";
import { SpansGantt } from "./SpansGantt";
import { DataGrid } from "./DataGrid";
import { Ticker } from "./Ticker";
import { KpiCard } from "./KpiCard";
import { Funnel } from "./Funnel";
import { Heatmap } from "./Heatmap";
import { Board } from "./Board";
import { RoadmapLanes } from "./RoadmapLanes";
import { Checklist } from "./Checklist";

export interface PanelProps {
  panel: Panel;
}

interface RendererEntry {
  match: (p: Panel) => boolean;
  component: ComponentType<PanelProps>;
}

const LineChart = lazy(() => import("./charts/LineChart"));
const RankedBarsChart = lazy(() => import("./charts/RankedBarsChart"));
const WaterfallChart = lazy(() => import("./charts/WaterfallChart"));
const QuickChart = lazy(() => import("./QuickChart"));
const LogBuffer = lazy(() => import("./LogBuffer"));
const Narrative = lazy(() => import("./Narrative"));
const MermaidDiagram = lazy(() => import("./MermaidDiagram"));
const Burndown = lazy(() => import("./charts/Burndown"));

const REGISTRY: RendererEntry[] = [
  { match: (p) => p.primitive === "logbuffer", component: LogBuffer },
  { match: (p) => p.primitive === "funnel" && p.data_kind === "table", component: Funnel },
  { match: (p) => p.primitive === "waterfall" && p.data_kind === "table", component: WaterfallChart },
  { match: (p) => p.primitive === "bar_ranked" && p.data_kind === "table", component: RankedBarsChart },
  {
    match: (p) => (p.primitive === "heatmap" || p.primitive === "status_matrix") && p.data_kind === "table",
    component: Heatmap,
  },
  { match: (p) => p.primitive === "gauge" && p.data_kind === "kpi", component: QuickChart },
  { match: (p) => p.primitive === "narrative" || p.primitive === "markdown", component: Narrative },
  { match: (p) => p.primitive === "board" && p.data_kind === "items", component: Board },
  { match: (p) => p.primitive === "roadmap_lanes" && p.data_kind === "items", component: RoadmapLanes },
  { match: (p) => p.primitive === "checklist" && p.data_kind === "items", component: Checklist },
  { match: (p) => p.primitive === "burndown", component: Burndown },
  {
    match: (p) => p.data_kind === "graph" || p.primitive === "dag" || p.primitive === "topology",
    component: MermaidDiagram,
  },
  { match: (p) => p.data_kind === "series", component: LineChart },
  { match: (p) => p.data_kind === "spans", component: SpansGantt },
  { match: (p) => p.data_kind === "table", component: DataGrid },
  { match: (p) => p.data_kind === "events", component: Ticker },
  { match: (p) => p.data_kind === "kpi", component: KpiCard },
  { match: (p) => p.data_kind === "items", component: Board },
];

class PanelErrorBoundary extends Component<{ children: ReactNode }, { failed: boolean }> {
  state = { failed: false };
  static getDerivedStateFromError() {
    return { failed: true };
  }
  render() {
    if (this.state.failed) {
      return <p className="mc-empty-state">Could not render this panel.</p>;
    }
    return this.props.children;
  }
}

export function PanelBody({ panel }: PanelProps) {
  const entry = REGISTRY.find((r) => r.match(panel));
  if (!entry) {
    return <p className="mc-empty-state">Unsupported panel kind: {panel.data_kind || "unknown"}</p>;
  }
  const C = entry.component;
  return (
    <PanelErrorBoundary>
      <Suspense fallback={<p className="mc-empty-state">Loading…</p>}>
        <C panel={panel} />
      </Suspense>
    </PanelErrorBoundary>
  );
}

const WIDE_PRIMITIVES = new Set([
  "timeseries",
  "board",
  "roadmap_lanes",
  "burndown",
  "small_multiples",
  "gantt",
  "ticker",
  "logbuffer",
  "datagrid",
  "heatmap",
  "status_matrix",
  "dag",
  "topology",
  "flamegraph",
  "diff",
  "narrative",
  "markdown",
]);
const WIDE_KINDS = new Set(["series", "spans", "events", "table", "graph", "items"]);

export function panelIsWide(panel: Panel): boolean {
  if (panel.primitive && WIDE_PRIMITIVES.has(panel.primitive)) return true;
  if (panel.primitive) return false;
  return !!panel.data_kind && WIDE_KINDS.has(panel.data_kind);
}

export function PanelSection({ panel, wide }: PanelProps & { wide?: boolean }) {
  return (
    <section className={"mc-panel mc-report-panel" + (wide ? " is-wide" : "")}>
      <div className="mc-panel-header">
        <h3>{panel.title || panel.data_kind || "Panel"}</h3>
        {panel.primitive && <span className="mc-label-muted">{panel.primitive}</span>}
      </div>
      <PanelBody panel={panel} />
    </section>
  );
}
