import { useEffect, useMemo, useRef, useState } from "react";
import {
  forceCenter,
  forceCollide,
  forceLink,
  forceManyBody,
  forceSimulation,
  type Simulation,
  type SimulationLinkDatum,
  type SimulationNodeDatum,
} from "d3-force";
import { useConnection } from "../../state/ConnectionContext";
import type { WorkerIndex } from "../../hooks/useWorkers";
import { nodeCode, nodeEmoji } from "../../lib/vocabulary";
import { createPortal } from "react-dom";
import { LayersIcon } from "../../components/icons";
import { usePageControls } from "../../state/PageControlsContext";
import { useD3Zoom, ToolboxZoom } from "../../components/ZoomPan";
import type { Worker } from "../../lib/types";
import { edgeKey, type Traffic } from "../../hooks/useTraffic";
import { phaseClass, phaseOf, type RolloutNode } from "../../lib/updates";
import { type NodeApp } from "../../lib/apps";
import { DelayedLoading } from "../../components/Async";

const WIDTH = 900;
const HEIGHT = 600;
const TICKS = 220;
const TICKS_PER_FRAME = 6;

interface GraphNode extends SimulationNodeDatum {
  id: string;
  role: string;
  status: string;
  depth: number;
  code: string;
}

type GraphLink = SimulationLinkDatum<GraphNode>;

function radiusFor(depth: number): number {
  if (depth === 0) return 16;
  if (depth === 1) return 11;
  if (depth === 2) return 8;
  return 6;
}

function depthOf(w: Worker, byId: Map<string, Worker>): number {
  let depth = 0;
  let cur = w;
  const seen = new Set<string>([w.id]);
  while (cur.reports_to) {
    const parent = byId.get(cur.reports_to);
    if (!parent || seen.has(parent.id)) break;
    seen.add(parent.id);
    cur = parent;
    depth++;
  }
  return depth;
}

function endId(end: GraphLink["source"]): string {
  if (typeof end === "string") return end;
  if (typeof end === "number") return String(end);
  return end.id;
}

const MAX_LEVELS = 9;

function ToolboxLevels({
  active,
  levels,
  max,
  hidden,
  onChange,
}: {
  active: boolean;
  levels: number;
  max: number;
  hidden: number;
  onChange: (n: number) => void;
}) {
  const controls = usePageControls();
  if (!active || !controls.railSlot || max <= 1) return null;
  const shown = Math.min(levels, max);
  return createPortal(
    <span className="mc-tool-flyout" role="group" aria-label="Levels shown">
      <button
        type="button"
        className="mc-tool-flyout-btn"
        aria-label={"Levels shown: " + shown + " of " + max}
        title="Levels shown"
      >
        <LayersIcon size={14} />
      </button>
      <span className="mc-tool-flyout-pop">
        <input
          type="range"
          min={1}
          max={max}
          value={shown}
          onChange={(e) => onChange(Number(e.target.value))}
          aria-label={"Levels shown: " + shown + " of " + max}
        />
        <span className="mc-level-control-value mc-mono">{shown}</span>
        {hidden > 0 && <span className="mc-level-control-hidden">−{hidden}</span>}
      </span>
    </span>,
    controls.railSlot,
  );
}

interface Props {
  index: WorkerIndex;
  active: boolean;
  selected: string | null;
  onSelect: (id: string | null) => void;
  traffic?: Traffic;
  updateOf?: Map<string, RolloutNode>;
  appOf?: Map<string, NodeApp>;
}

function updateLabel(upd: RolloutNode | undefined): string {
  if (!upd) return "";
  return " \u00b7 update " + phaseOf(upd.status).label;
}

const LINK_MIN = 1.2;
const LINK_MAX = 7;
const PATH_LINK_MIN = 2.5;

function linkWidth(weight: number, peak: number): number {
  if (!peak || weight <= 0) return LINK_MIN;
  return LINK_MIN + (LINK_MAX - LINK_MIN) * Math.sqrt(Math.min(1, weight / peak));
}

export default function ForceGraph({ index, active, selected, onSelect, traffic, updateOf, appOf }: Props) {
  const conn = useConnection();
  const { workers, error } = index;
  const [, setTick] = useState(0);
  const [hover, setHover] = useState<string | null>(null);
  const [levels, setLevels] = useState(MAX_LEVELS);
  const simRef = useRef<Simulation<GraphNode, GraphLink> | null>(null);
  const zoom = useD3Zoom();

  const graph = useMemo(() => {
    if (!workers.length) return null;
    const byId = new Map(workers.map((w) => [w.id, w]));
    const withDepth = workers.map((w, i) => ({ w, depth: depthOf(w, byId), i }));
    const deepest = withDepth.reduce((m, x) => Math.max(m, x.depth), 0);
    const kept = withDepth.filter((x) => x.depth < levels);
    const keptIds = new Set(kept.map((x) => x.w.id));
    const nodes: GraphNode[] = kept.map((x) => ({
      id: x.w.id,
      role: x.w.role || "",
      status: x.w.status || "disconnected",
      depth: x.depth,
      code: nodeCode(x.i + 1, workers.length),
    }));
    const links: GraphLink[] = kept
      .filter((x) => x.w.reports_to && keptIds.has(x.w.reports_to))
      .map((x) => ({ source: x.w.reports_to as string, target: x.w.id }));
    return { nodes, links, deepest: deepest + 1, hidden: workers.length - kept.length };
  }, [workers, levels]);

  const ancestors = useMemo(() => {
    const path = new Set<string>();
    let cur = selected ? index.byId.get(selected) : undefined;
    if (!cur) return path;
    const seen = new Set<string>([cur.id]);
    while (cur.reports_to) {
      const parent = index.byId.get(cur.reports_to);
      if (!parent || seen.has(parent.id)) break;
      seen.add(parent.id);
      path.add(parent.id);
      cur = parent;
    }
    return path;
  }, [selected, index.byId]);

  const pathLinks = useMemo(() => {
    const on = new Set<GraphLink>();
    if (!selected || !graph) return on;
    for (const l of graph.links) {
      const t = endId(l.target);
      if (t !== selected && !ancestors.has(t)) continue;
      if (ancestors.has(endId(l.source))) on.add(l);
    }
    return on;
  }, [graph, selected, ancestors]);

  useEffect(() => {
    simRef.current?.stop();
    if (!graph) return;
    const sim = forceSimulation<GraphNode, GraphLink>(graph.nodes)
      .force(
        "link",
        forceLink<GraphNode, GraphLink>(graph.links)
          .id((d) => d.id)
          .distance(46)
          .strength(0.65),
      )
      .force("charge", forceManyBody<GraphNode>().strength(-140))
      .force("center", forceCenter(WIDTH / 2, HEIGHT / 2))
      .force(
        "collide",
        forceCollide<GraphNode>().radius((d) => radiusFor(d.depth) + 3),
      )
      .stop();

    simRef.current = sim;
    let frame = 0;
    let raf = 0;
    const step = () => {
      for (let i = 0; i < TICKS_PER_FRAME && frame < TICKS; i++, frame++) sim.tick();
      setTick((t) => t + 1);
      if (frame < TICKS) raf = requestAnimationFrame(step);
    };
    raf = requestAnimationFrame(step);
    return () => {
      cancelAnimationFrame(raf);
      sim.stop();
    };
  }, [graph]);

  return (
    <section className="mc-panel mc-panel-fill" aria-label="Reporting network">
      <ToolboxZoom zoom={zoom} label="Reporting network" active={active} />
      <ToolboxLevels
        active={active}
        levels={levels}
        max={graph?.deepest ?? MAX_LEVELS}
        hidden={graph?.hidden ?? 0}
        onChange={setLevels}
      />
      <>
        {!conn.hasToken ? (
          <p className="mc-empty-state">Connect to a server to see the network.</p>
        ) : error ? (
          <p className="mc-empty-state">{error}</p>
        ) : index.pending ? (
          <DelayedLoading />
        ) : !graph ? (
          <p className="mc-empty-state">No workers connected yet.</p>
        ) : (
          <div className="mc-zoom-viewport">
            <svg
              ref={zoom.svgRef}
              className="mc-force-graph"
              viewBox={`0 0 ${WIDTH} ${HEIGHT}`}
              role="img"
              aria-label="Reporting network: workers connected by their reports-to edges"
            >
              <g transform={zoom.transform}>
                <g className="mc-force-links">
                  {graph.links.map((l) => {
                    const s = l.source as GraphNode;
                    const t = l.target as GraphNode;
                    if (typeof s !== "object" || typeof t !== "object") return null;
                    if (pathLinks.has(l)) return null;
                    const inSel = index.matchAll || (index.matches.has(s.id) && index.matches.has(t.id));
                    const weight = traffic?.weights.get(edgeKey(s.id, t.id)) || 0;
                    return (
                      <line
                        key={s.id + ">" + t.id}
                        className={inSel ? undefined : "is-outside"}
                        strokeWidth={linkWidth(weight, traffic?.peak || 0)}
                        x1={s.x}
                        y1={s.y}
                        x2={t.x}
                        y2={t.y}
                      >
                        {weight > 0 && <title>{`${s.id} ↔ ${t.id}: busy line`}</title>}
                      </line>
                    );
                  })}
                </g>
                <g className="mc-force-links-path">
                  {graph.links.map((l) => {
                    if (!pathLinks.has(l)) return null;
                    const s = l.source as GraphNode;
                    const t = l.target as GraphNode;
                    if (typeof s !== "object" || typeof t !== "object") return null;
                    const weight = traffic?.weights.get(edgeKey(s.id, t.id)) || 0;
                    return (
                      <line
                        key={s.id + ">" + t.id}
                        stroke="var(--selected)"
                        strokeWidth={Math.max(PATH_LINK_MIN, linkWidth(weight, traffic?.peak || 0))}
                        opacity={0.7}
                        x1={s.x}
                        y1={s.y}
                        x2={t.x}
                        y2={t.y}
                      />
                    );
                  })}
                </g>
                {graph.nodes.map((n) => {
                  const r = radiusFor(n.depth);
                  const connected = n.status === "connected";
                  const isHover = hover === n.id;
                  const isSelected = selected === n.id;
                  const onPath = ancestors.has(n.id);
                  const upd = updateOf?.get(n.id);
                  const app = appOf?.get(n.id);
                  return (
                    <g
                      key={n.id}
                      transform={`translate(${n.x || 0},${n.y || 0})`}
                      className={"mc-force-node" + (index.matchAll || index.matches.has(n.id) ? "" : " is-outside")}
                      onClick={() => onSelect(selected === n.id ? null : n.id)}
                      onMouseEnter={() => setHover(n.id)}
                      onMouseLeave={() => setHover((h) => (h === n.id ? null : h))}
                      role="img"
                      aria-label={`${n.code} ${n.id}${n.role ? " (" + n.role + ")" : ""} — ${n.status}${updateLabel(upd)}${app ? " · serves an app" : ""}`}
                    >
                      <title>{`${n.code} · ${n.id}${n.role ? " (" + n.role + ")" : ""} — ${n.status}${updateLabel(upd)}${app ? " · serves an app" : ""}`}</title>
                      <circle
                        r={r}
                        fill="var(--surface)"
                        stroke={
                          isSelected || onPath ? "var(--selected)" : connected ? "var(--success)" : "var(--text-muted)"
                        }
                        strokeWidth={isSelected ? 2 : onPath ? 1.5 : 2}
                        opacity={onPath && !isSelected ? 0.6 : undefined}
                      />
                      {upd && (
                        <circle
                          r={r + 3}
                          fill="none"
                          className={"mc-force-update " + phaseClass(upd.status)}
                          strokeDasharray={phaseOf(upd.status).tone === "silent" ? "2 3" : undefined}
                        />
                      )}
                      <text textAnchor="middle" dy="0.35em" fontSize={r}>
                        {nodeEmoji(n.role)}
                      </text>
                      {app && (
                        <text
                          className="mc-force-app"
                          x={r + 2}
                          y={-r}
                          fontSize={Math.max(8, r * 0.8)}
                          aria-hidden="true"
                        >
                          ↗
                        </text>
                      )}
                      {(isHover || isSelected || onPath || n.depth <= 1) && (
                        <text className="mc-force-label" y={r + 11} textAnchor="middle">
                          {n.id}
                        </text>
                      )}
                    </g>
                  );
                })}
              </g>
            </svg>
          </div>
        )}
      </>
    </section>
  );
}
