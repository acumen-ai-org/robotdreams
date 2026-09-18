import { useMemo } from "react";
import { hierarchy, pack, type HierarchyCircularNode } from "d3-hierarchy";
import { useConnection } from "../../state/ConnectionContext";
import { placeInScope, scopeOf, type WorkerIndex } from "../../hooks/useWorkers";
import { entityCode, nodeCode, nodeEmoji } from "../../lib/vocabulary";
import { useD3Zoom, ToolboxZoom, ClearSelection } from "../../components/ZoomPan";
import { useVocabulary } from "../../state/VocabularyContext";
import type { ScopeEntry, Worker } from "../../lib/types";
import { DelayedLoading } from "../../components/Async";

const SIZE = 800;

const PLANET_BLUE = "#a9d9f2";
const PLANET_PINK = "#ebcfdc";
const INK = "#07131c";

const inscribedSide = (r: number) => r * Math.SQRT2;

const FACTORY_BODY_TOP = 0.32;
const FACTORY_PATH =
  "M0,1 L0,0.32 L0.14,0.08 L0.14,0.32 L0.31,0.08 L0.31,0.32 L0.48,0.08 L0.48,0.32 " +
  "L0.65,0.08 L0.65,0.32 L0.78,0.32 L0.78,0 L0.92,0 L0.92,0.32 L1,0.32 L1,1 Z";

const FACTORY_PATH_SMALL =
  "M0,1 L0,0.32 L0.26,0.05 L0.26,0.32 L0.52,0.05 L0.52,0.32 " + "L0.64,0.32 L0.64,0 L0.88,0 L0.88,0.32 L1,0.32 L1,1 Z";

const FACTORY_DETAIL_AT = 44;

const WORLD_BAND = 0.18;
const SHAPE_SHRINK = 0.92;
const REALM_INSET = 0.9;
const LABEL_RIM = 0.97;
const LABEL_GAP = 0.03;
const ASCENT = 0.85;

const shapeSide = (r: number) => inscribedSide(r) * SHAPE_SHRINK;

const CHAR_W = 0.55;
const MIN_LABEL = 7;
const MIN_TRUNCATED_WIDTH = 44;
const MIN_TRUNCATED_CHARS = 6;
const EMOJI_MIN_PX = 6;
const REALM_PALETTE_SIZE = 10;
const CANVAS_PADDING = 14;
const PLACE_PADDING = 10;

function fitLabel(text: string, innerWidth: number, maxSize: number): { text: string; size: number } | null {
  if (!text || maxSize < MIN_LABEL) return null;
  const size = Math.min(maxSize, innerWidth / (CHAR_W * text.length));
  if (size >= MIN_LABEL) return { text, size };
  if (innerWidth < MIN_TRUNCATED_WIDTH) return null;
  const budget = Math.floor(innerWidth / (CHAR_W * MIN_LABEL));
  if (budget < MIN_TRUNCATED_CHARS) return null;
  return { text: text.slice(0, budget - 1) + "…", size: MIN_LABEL };
}

interface TreeDatum {
  id: string;
  worker?: Worker;
  name?: string;
  path?: string;
  level: number;
  children: TreeDatum[];
}

function buildTree(workers: Worker[], scopes: ScopeEntry[]): TreeDatum {
  const root: TreeDatum = { id: "__canvas__", level: 0, children: [] };
  const containers = new Map<string, TreeDatum>();

  const containerFor = (universe: string, segs: string[]): TreeDatum => {
    let node = root;
    let rel = "";
    let full = universe;
    for (let i = 0; i < segs.length && i < 3; i++) {
      rel = rel ? rel + "/" + segs[i] : segs[i];
      full = full ? full + "/" + segs[i] : segs[i];
      let next = containers.get(rel);
      if (!next) {
        next = { id: "scope:" + full, name: segs[i], path: full, level: i + 1, children: [] };
        containers.set(rel, next);
        node.children.push(next);
      }
      node = next;
    }
    return node;
  };

  for (const s of scopes) {
    const all = s.path.split("/").filter(Boolean);
    containerFor(all[0] || "", all.slice(1));
  }

  for (const w of workers) {
    const all = scopeOf(w).split("/").filter(Boolean);
    containerFor(all[0] || "", all.slice(1)).children.push({ id: w.id, worker: w, level: 0, children: [] });
  }

  const sortRec = (n: TreeDatum) => {
    n.children.sort((a, b) => Number(!!a.worker) - Number(!!b.worker) || a.id.localeCompare(b.id));
    n.children.forEach(sortRec);
  };
  sortRec(root);
  return root;
}

type PackedNode = HierarchyCircularNode<TreeDatum>;

interface Geom {
  x: number;
  y: number;
  r: number;
}

function contentRegion(level: number, r: number): { ox: number; oy: number; r: number } {
  if (level === 1) return { ox: 0, oy: r * WORLD_BAND, r: r * (1 - WORLD_BAND) };
  return { ox: 0, oy: 0, r: (shapeSide(r) / 2) * REALM_INSET };
}

function bodyRect(cx: number, cy: number, r: number) {
  const side = shapeSide(r);
  const pad = side * 0.07;
  return {
    x: cx - side / 2 + pad,
    y: cy - side / 2 + FACTORY_BODY_TOP * side + pad,
    w: side - 2 * pad,
    h: (1 - FACTORY_BODY_TOP) * side - 2 * pad,
  };
}

function gridInto(g: Geom, kids: PackedNode[], geom: Map<string, Geom>) {
  const box = bodyRect(g.x, g.y, g.r);
  const n = kids.length;
  const cols = Math.max(1, Math.min(n, Math.round(Math.sqrt((n * box.w) / Math.max(box.h, 1e-6))) || 1));
  const rows = Math.ceil(n / cols);
  const cell = Math.min(box.w / cols, box.h / rows);
  const dot = cell * 0.38;
  const ox = box.x + (box.w - cols * cell) / 2;
  const oy = box.y + (box.h - rows * cell) / 2;
  kids.forEach((c, i) => {
    const row = Math.floor(i / cols);
    const col = i % cols;
    const inRow = Math.min(cols, n - row * cols);
    const indent = ((cols - inRow) * cell) / 2;
    geom.set(c.data.id, { x: ox + indent + (col + 0.5) * cell, y: oy + (row + 0.5) * cell, r: dot });
  });
}

const LONELY_Y = 0.86;

function seatLonely(cx: number, cy: number, R: number, lonely: PackedNode[], placed: Geom[], geom: Map<string, Geom>) {
  const n = lonely.length;
  const y = cy - R * LONELY_Y;
  const half = Math.sqrt(Math.max(0, R * R - (R * LONELY_Y) ** 2)) * 0.92;
  const dot = Math.max(0.5, Math.min(R * 0.09, (2 * half) / (2.4 * n)));
  const pitch = dot * 2.4;
  const w = n * pitch;

  const clash = (x: number) => {
    let worst = 0;
    for (const g of placed) {
      const dx = Math.max(0, Math.abs(g.x - x) - (w / 2 + g.r));
      const dy = Math.max(0, Math.abs(g.y - y) - (dot + g.r));
      if (dx === 0 && dy === 0) worst += Math.min(w / 2 + g.r - Math.abs(g.x - x), dot + g.r - Math.abs(g.y - y));
    }
    return worst;
  };

  const candidates = [cx, cx - half + w / 2, cx + half - w / 2];
  let x = candidates[0];
  let best = clash(x);
  for (let i = 1; i < candidates.length && best > 0; i++) {
    const c = clash(candidates[i]);
    if (c < best * 0.85) {
      best = c;
      x = candidates[i];
    }
  }

  const left = x - w / 2 + pitch / 2;
  lonely.forEach((c, i) => geom.set(c.data.id, { x: left + i * pitch, y, r: dot }));
}

function fitToShapes(root: PackedNode): Map<string, Geom> {
  const geom = new Map<string, Geom>([[root.data.id, { x: root.x, y: root.y, r: root.r }]]);
  root.eachBefore((d) => {
    const kids = d.children;
    if (!kids?.length) return;
    const g = geom.get(d.data.id)!;
    if (d.data.level === 3) return gridInto(g, kids, geom);
    const region = d.depth === 0 ? { ox: 0, oy: 0, r: g.r } : contentRegion(d.data.level, g.r);
    const f = region.r / d.r;
    const seat = (c: PackedNode): Geom => ({
      x: g.x + region.ox + (c.x - d.x) * f,
      y: g.y + region.oy + (c.y - d.y) * f,
      r: c.r * f,
    });
    const places = kids.filter((c) => !c.data.worker);
    const lonely = kids.filter((c) => c.data.worker);
    const placed = places.map((c) => {
      const gc = seat(c);
      geom.set(c.data.id, gc);
      return gc;
    });
    if (!lonely.length) return;
    if (!places.length) {
      for (const c of lonely) geom.set(c.data.id, seat(c));
      return;
    }
    seatLonely(g.x + region.ox, g.y + region.oy, region.r, lonely, placed, geom);
  });
  return geom;
}

function labelBand(lvl: number, r: number, k: number) {
  const shapeTop = lvl === 1 ? r * (2 * WORLD_BAND - 1) : -shapeSide(r) / 2;
  const baselineY = shapeTop - LABEL_GAP * r;
  const size = Math.min(13 / k, Math.max(0, (LABEL_RIM * r - Math.abs(baselineY)) / ASCENT));
  const top = baselineY - ASCENT * size;
  const width = 2 * Math.sqrt(Math.max(0, r * r - top * top)) * 0.94;
  return { baselineY, size, width };
}

interface Annotations {
  worldIndex: Map<string, number>;
  realmIndex: Map<string, number>;
  siteIndex: Map<string, number>;
  codes: Map<string, string>;
}

function annotate(root: PackedNode): Annotations {
  const worldIndex = new Map<string, number>();
  const realmIndex = new Map<string, number>();
  const siteIndex = new Map<string, number>();
  const codes = new Map<string, string>();
  let worlds = 0;
  let realms = 0;
  let sites = 0;
  let workers = 0;
  const total = root.descendants().filter((d) => d.data.worker).length;
  root.eachBefore((d) => {
    if (d.data.worker) {
      codes.set(d.data.id, nodeCode(++workers, total));
      return;
    }
    if (d.data.level === 1) worldIndex.set(d.data.id, ++worlds);
    else if (d.data.level === 2) realmIndex.set(d.data.id, realms++ % REALM_PALETTE_SIZE);
    else if (d.data.level === 3) siteIndex.set(d.data.id, ++sites);
  });
  return { worldIndex, realmIndex, siteIndex, codes };
}

function realmOf(d: PackedNode, ann: Annotations): number | undefined {
  for (let n: PackedNode | null = d; n; n = n.parent) {
    const r = ann.realmIndex.get(n.data.id);
    if (r != null) return r;
  }
  return undefined;
}

interface PlaceVisual {
  lvl: number;
  code: string;
  fill: string;
  rowFill: string;
  title: string;
  side: number;
  row: { name: string | null; size: number; y: number } | null;
}

function placeVisual(
  d: PackedNode,
  r: number,
  ann: Annotations,
  lower: (level: number) => string,
  k: number,
): PlaceVisual {
  const lvl = d.data.level;
  const worldN = ann.worldIndex.get(d.data.id);
  const realmN = ann.realmIndex.get(d.data.id);
  const siteN = ann.siteIndex.get(d.data.id);
  const inRealm = realmOf(d, ann);
  const code =
    lvl === 1
      ? entityCode("W", worldN || 1, ann.worldIndex.size)
      : lvl === 2
        ? "R" + (realmN ?? 0)
        : entityCode("S", siteN || 1, ann.siteIndex.size);
  const fill =
    lvl === 1
      ? (worldN || 1) % 2 === 1
        ? PLANET_BLUE
        : PLANET_PINK
      : lvl === 3
        ? "var(--site-bg)"
        : inRealm != null
          ? `var(--realm-${inRealm}-bg)`
          : "var(--surface-soft)";
  const rowFill = lvl <= 2 ? INK : inRealm != null ? `var(--realm-${inRealm}-fg)` : "var(--text-muted)";
  const title = `${code} · ${d.data.name} (${lower(lvl)})`;

  const side = shapeSide(r);

  const band = labelBand(lvl, r, k);
  const fitted = fitLabel(d.data.name || "", band.width * k, band.size * k);
  const codeOnly = Math.min(band.size * k, 11, (band.width * k) / (CHAR_W * (code.length + 0.4)));
  const row = fitted
    ? { name: fitted.text, size: fitted.size / k, y: band.baselineY }
    : codeOnly >= MIN_LABEL
      ? { name: null, size: codeOnly / k, y: band.baselineY }
      : null;
  return { lvl, code, fill, rowFill, title, side, row };
}

interface Props {
  index: WorkerIndex;
  scopes: ScopeEntry[];
  active: boolean;
  selected: string | null;
  onSelect: (id: string | null) => void;
}

export default function PlanetMap({ index, scopes, active, selected, onSelect }: Props) {
  const conn = useConnection();
  const vocab = useVocabulary();
  const { workers, error } = index;
  const zoom = useD3Zoom();

  const layout = useMemo(() => {
    const tree = buildTree(workers, scopes);
    if (!tree.children.length) return null;
    const root = hierarchy(tree, (d) => d.children)
      .sum((d) => (d.children.length ? 0 : 1))
      .sort((a, b) => (b.value || 0) - (a.value || 0));
    const packed = pack<TreeDatum>()
      .size([SIZE, SIZE])
      .padding((d) => (d.depth === 0 ? CANVAS_PADDING : PLACE_PADDING))(root);
    return { packed, ann: annotate(packed), geom: fitToShapes(packed) };
  }, [workers, scopes]);

  return (
    <section className="mc-panel mc-panel-fill" aria-label="Scope map">
      <ToolboxZoom zoom={zoom} label="Scope map" active={active} />
      <ClearSelection
        show={!!selected}
        onClear={() => {
          onSelect(null);
          zoom.reset();
        }}
      />
      {!conn.hasToken ? (
        <p className="mc-empty-state">Connect to a server to see the map.</p>
      ) : error ? (
        <p className="mc-empty-state">{error}</p>
      ) : index.pending ? (
        <DelayedLoading />
      ) : !layout ? (
        <p className="mc-empty-state">No workers connected yet.</p>
      ) : (
        <div className="mc-zoom-viewport">
          <svg
            ref={zoom.svgRef}
            className="mc-planet-map"
            viewBox={`0 0 ${SIZE} ${SIZE}`}
            role="img"
            aria-label="Scope map: workers packed by the scope they report for"
          >
            <g transform={zoom.transform}>
              <defs>
                <radialGradient id="planet-hi" cx="35%" cy="30%" r="75%">
                  <stop offset="0%" stopColor="#ffffff" stopOpacity="0.4" />
                  <stop offset="55%" stopColor="#ffffff" stopOpacity="0.08" />
                  <stop offset="100%" stopColor="#ffffff" stopOpacity="0" />
                </radialGradient>
              </defs>

              <g className="mc-map-shapes">
                {layout.packed.descendants().map((d) => {
                  if (d.depth === 0) return null;
                  const g = layout.geom.get(d.data.id)!;

                  if (!d.data.worker) {
                    const v = placeVisual(d, g.r, layout.ann, vocab.lower, zoom.scale);
                    return (
                      <g
                        key={d.data.id}
                        transform={`translate(${g.x},${g.y})`}
                        role="button"
                        tabIndex={0}
                        aria-label={"Zoom to " + v.title}
                        className={
                          "mc-map-place" +
                          (selected === d.data.id ? " is-selected" : "") +
                          (placeInScope(index, d.data.path || "") ? "" : " is-outside")
                        }
                        onClick={() => {
                          onSelect(selected === d.data.id ? null : d.data.id);
                          zoom.zoomTo(g.x, g.y, g.r);
                        }}
                        onKeyDown={(e) => {
                          if (e.key === "Enter" || e.key === " ") {
                            e.preventDefault();
                            onSelect(selected === d.data.id ? null : d.data.id);
                            zoom.zoomTo(g.x, g.y, g.r);
                          }
                        }}
                      >
                        <title>{v.title}</title>
                        {v.lvl === 1 ? (
                          <circle r={g.r} fill={v.fill} className="mc-map-shape mc-planet-circle" />
                        ) : v.lvl === 2 ? (
                          <rect
                            x={-v.side / 2}
                            y={-v.side / 2}
                            width={v.side}
                            height={v.side}
                            rx={Math.min(6, v.side * 0.12)}
                            fill={v.fill}
                            className="mc-map-shape mc-realm-square"
                          />
                        ) : (
                          <path
                            d={v.side * zoom.scale >= FACTORY_DETAIL_AT ? FACTORY_PATH : FACTORY_PATH_SMALL}
                            transform={`translate(${-v.side / 2},${-v.side / 2}) scale(${v.side})`}
                            vectorEffect="non-scaling-stroke"
                            fill={v.fill}
                            className="mc-map-shape mc-site-factory"
                          />
                        )}
                        {v.lvl === 1 && <circle r={g.r} fill="url(#planet-hi)" pointerEvents="none" />}
                      </g>
                    );
                  }

                  const w = d.data.worker;
                  const code = layout.ann.codes.get(w.id) || "";
                  const connected = w.status === "connected";
                  const isSelected = selected === w.id;
                  const title = `${code} · ${w.id}${w.role ? " (" + w.role + ")" : ""} — ${w.status || "unknown"}`;
                  return (
                    <g
                      key={w.id}
                      transform={`translate(${g.x},${g.y})`}
                      role="img"
                      aria-label={title}
                      className={"mc-map-node" + (index.matchAll || index.matches.has(w.id) ? "" : " is-outside")}
                      onClick={() => {
                        onSelect(selected === w.id ? null : w.id);
                        zoom.zoomTo(g.x, g.y, g.r);
                      }}
                    >
                      <title>{title}</title>
                      <circle
                        r={g.r}
                        fill="var(--surface)"
                        stroke={isSelected ? "var(--selected)" : connected ? "var(--success)" : "var(--text-muted)"}
                        strokeWidth={isSelected ? "2" : "1"}
                        vectorEffect="non-scaling-stroke"
                      />
                      {g.r * zoom.scale >= EMOJI_MIN_PX && (
                        <text textAnchor="middle" dy="0.35em" fontSize={Math.min(g.r * 1.2, 15 / zoom.scale)}>
                          {nodeEmoji(w.role)}
                        </text>
                      )}
                    </g>
                  );
                })}
              </g>

              <g className="mc-map-labels" pointerEvents="none" aria-hidden="true">
                {layout.packed.descendants().map((d) => {
                  if (d.depth === 0) return null;
                  const g = layout.geom.get(d.data.id)!;
                  if (d.data.worker) {
                    if (selected !== d.data.worker.id) return null;
                    const text = d.data.worker.id;
                    const fs = 10 / zoom.scale;
                    const w = text.length * CHAR_W * fs + fs * 1.1;
                    const h = fs * 1.7;
                    const y = g.y + g.r + 5 / zoom.scale;
                    return (
                      <g key={d.data.id}>
                        <rect x={g.x - w / 2} y={y} width={w} height={h} rx={h * 0.3} className="mc-map-node-plate" />
                        <text
                          x={g.x}
                          y={y + h * 0.72}
                          textAnchor="middle"
                          className="mc-map-code mc-map-node-label"
                          fontSize={fs}
                        >
                          {text}
                        </text>
                      </g>
                    );
                  }
                  const v = placeVisual(d, g.r, layout.ann, vocab.lower, zoom.scale);
                  if (!v.row) return null;
                  return (
                    <g
                      key={d.data.id}
                      transform={`translate(${g.x},${g.y})`}
                      className={placeInScope(index, d.data.path || "") ? undefined : "is-outside"}
                    >
                      <text
                        y={v.row.y}
                        textAnchor="middle"
                        className="mc-map-name"
                        fill={v.rowFill}
                        fontSize={v.row.size}
                      >
                        <tspan className="mc-map-code">{v.code}</tspan>
                        {v.row.name ? <tspan>{" " + v.row.name}</tspan> : null}
                      </text>
                    </g>
                  );
                })}
              </g>
            </g>
          </svg>
        </div>
      )}
    </section>
  );
}
