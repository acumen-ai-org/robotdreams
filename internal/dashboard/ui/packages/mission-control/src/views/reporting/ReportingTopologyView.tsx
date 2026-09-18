import { useCallback, useEffect, useMemo, useState } from "react";
import { hierarchy, tree, type HierarchyPointNode } from "d3-hierarchy";
import { linkHorizontal } from "d3-shape";
import { useConnection } from "../../state/ConnectionContext";
import { useD3Zoom, ToolboxZoom } from "../../components/ZoomPan";
import { useReveal } from "../../hooks/useReveal";
import { LEVEL_NAMES, type Route } from "../../lib/routes";
import type { ScopeEntry, SummaryTile } from "../../lib/types";

const STATUS_SCOPE_CAP = 150;
const PAD = 24;
const BOX_W = 150;
const BOX_H = 26;
const CONCURRENCY = 4;

interface ScopeNode {
  name: string;
  path: string;
  entry?: ScopeEntry;
  children: ScopeNode[];
}

function buildScopeTree(scopes: ScopeEntry[]): ScopeNode {
  const synthetic: ScopeNode = { name: "All", path: "", children: [] };
  const byPath = new Map<string, ScopeNode>([["", synthetic]]);
  const ensure = (path: string): ScopeNode => {
    const existing = byPath.get(path);
    if (existing) return existing;
    const segs = path.split("/");
    const parent = ensure(segs.slice(0, -1).join("/"));
    const node: ScopeNode = { name: segs[segs.length - 1], path, children: [] };
    parent.children.push(node);
    byPath.set(path, node);
    return node;
  };
  for (const s of scopes.slice().sort((a, b) => a.path.localeCompare(b.path))) {
    ensure(s.path).entry = s;
  }
  const depth1 = synthetic.children;
  return depth1.length === 1 ? depth1[0] : synthetic;
}

type WorstStatus = "critical" | "warn" | "ok" | "unknown";

const STATUS_RANK: Record<string, number> = { critical: 3, warn: 2, ok: 1 };

function worst(tiles: SummaryTile[]): WorstStatus {
  let best: WorstStatus = "unknown";
  let rank = 0;
  for (const t of tiles) {
    const r = STATUS_RANK[t.status || ""] || 0;
    if (r > rank) {
      rank = r;
      best = t.status as WorstStatus;
    }
  }
  return best;
}

interface Props {
  route: Route;
  scopes: ScopeEntry[];
  refreshTick: number;
  active: boolean;
  picked: string | null;
  onPick: (path: string | null) => void;
  periodQuery?: string;
}

export default function ReportingTopologyView({
  route,
  scopes,
  refreshTick,
  active,
  picked,
  onPick,
  periodQuery = "",
}: Props) {
  const conn = useConnection();
  const [statuses, setStatuses] = useState<Map<string, WorstStatus>>(new Map());
  const zoom = useD3Zoom();
  const [mine, setMine] = useState(0);

  const layout = useMemo(() => {
    if (!scopes.length) return null;
    const root = hierarchy(buildScopeTree(scopes), (d) => d.children);
    const realmOf = new Map<string, number>();
    let realms = 0;
    root.eachBefore((d) => {
      if (d.depth === 2) realmOf.set(d.data.path, realms++ % 10);
      else if (d.depth > 2 && d.parent) {
        const inherited = realmOf.get(d.parent.data.path);
        if (inherited != null) realmOf.set(d.data.path, inherited);
      }
    });
    const laid = tree<ScopeNode>().nodeSize([34, 190])(root);
    let minX = Infinity;
    let maxX = -Infinity;
    let maxY = 0;
    laid.each((d) => {
      minX = Math.min(minX, d.x);
      maxX = Math.max(maxX, d.x);
      maxY = Math.max(maxY, d.y);
    });
    const byPath = new Map<string, HierarchyPointNode<ScopeNode>>();
    laid.each((d) => byPath.set(d.data.path, d));
    return { laid, realmOf, byPath, minX, maxX, maxY };
  }, [scopes]);

  useEffect(() => {
    if (!active || !conn.hasToken || !scopes.length || scopes.length > STATUS_SCOPE_CAP) return;
    let stale = false;
    const paths = scopes.map((s) => s.path);
    const results = new Map<string, WorstStatus>();
    let i = 0;
    const workOne = async (): Promise<void> => {
      for (;;) {
        const idx = i++;
        if (idx >= paths.length || stale) return;
        const data = await conn
          .apiJSON<{ tiles?: SummaryTile[] }>(
            "/api/reports/summary?scope=" + encodeURIComponent(paths[idx]) + periodQuery,
          )
          .catch(() => null);
        if (data) results.set(paths[idx], worst(data.tiles || []));
      }
    };
    void Promise.all(Array.from({ length: CONCURRENCY }, workOne)).then(() => {
      if (!stale) setStatuses(new Map(results));
    });
    return () => {
      stale = true;
    };
  }, [active, conn, conn.hasToken, conn.session, scopes, refreshTick, periodQuery]);

  const flyTo = useCallback(
    (path: string) => {
      const node = layout ? layout.byPath.get(path) : undefined;
      if (!layout || !node) return false;
      return zoom.zoomTo(node.y + PAD + BOX_W / 2, node.x - layout.minX + PAD + BOX_H / 2, BOX_W);
    },
    [layout, zoom],
  );
  useReveal(picked, active, flyTo, mine);

  if (!layout) return <p className="mc-empty-state">No report scopes yet.</p>;

  const width = layout.maxY + BOX_W + PAD * 2;
  const height = layout.maxX - layout.minX + BOX_H + PAD * 2;
  const link = linkHorizontal<unknown, HierarchyPointNode<ScopeNode>>()
    .x((d) => d.y + PAD)
    .y((d) => d.x - layout.minX + PAD + BOX_H / 2);

  return (
    <section className="mc-panel mc-panel-fill" aria-label="Scope topology">
      <ToolboxZoom zoom={zoom} label="Scope topology" active={active} />
      <div className="mc-zoom-viewport">
        <svg
          ref={zoom.svgRef}
          className="mc-topology-svg"
          viewBox={`0 0 ${width} ${height}`}
          role="img"
          aria-label="Scope tree; click a scope to open it beside the drawing"
        >
          <g transform={zoom.transform}>
            {layout.laid.links().map((l, i) => (
              <path key={i} className="mc-topo-link" d={link(l) || undefined} />
            ))}
            {layout.laid.descendants().map((d) => {
              const path = d.data.path;
              const realm = layout.realmOf.get(path);
              const status = statuses.get(path);
              const level = d.data.entry?.level || LEVEL_NAMES[d.depth] || "";
              const instances = d.data.entry?.instances || 0;
              const label = d.data.name + (instances ? " · " + instances : "");
              const boxFill = realm != null ? `var(--realm-${realm}-bg)` : "var(--surface)";
              const textFill = realm != null ? `var(--realm-${realm}-fg)` : "var(--text)";
              const isCurrent = path === route.scope;
              const isPicked = path === picked;
              return (
                <g
                  key={path || "__root__"}
                  className={"mc-topo-node" + (isPicked ? " is-picked" : "")}
                  transform={`translate(${d.y + PAD},${d.x - layout.minX + PAD})`}
                  role="link"
                  aria-label={`Scope ${path || "All"}${level ? " (" + level + ")" : ""}${status ? ", status " + status : ""}`}
                  onClick={() => {
                    setMine((n) => n + 1);
                    onPick(picked === path ? null : path);
                  }}
                >
                  <title>{(path || "All") + (status ? " — " + status : "")}</title>
                  <rect
                    className="mc-topo-box"
                    width={BOX_W}
                    height={BOX_H}
                    rx={6}
                    fill={boxFill}
                    stroke={isPicked || isCurrent ? "var(--brand-strong)" : realm != null ? textFill : "var(--border)"}
                    strokeWidth={isPicked ? 3 : isCurrent ? 2 : 1}
                  />
                  <text className="mc-topo-label" x={10} y={BOX_H / 2 + 4} fill={textFill}>
                    {label.length > 18 ? label.slice(0, 17) + "…" : label}
                  </text>
                  {level && (
                    <text className="mc-topo-level" x={10} y={-4}>
                      {level}
                    </text>
                  )}
                  {status && status !== "unknown" && (
                    <circle
                      cx={BOX_W - 10}
                      cy={BOX_H / 2}
                      r={4}
                      fill={
                        status === "critical" ? "var(--danger)" : status === "warn" ? "var(--warm)" : "var(--success)"
                      }
                    />
                  )}
                </g>
              );
            })}
          </g>
        </svg>
      </div>
    </section>
  );
}
