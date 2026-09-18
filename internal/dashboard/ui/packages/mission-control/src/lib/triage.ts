import type { KPIValue, SummaryTile } from "./types";

const RANK: Record<string, number> = { critical: 0, warn: 1, ok: 2 };

export function severityRank(status: string | undefined): number {
  return RANK[status || ""] ?? 3;
}

export function normalisedVariance(kpi: KPIValue): number {
  const target = typeof kpi.target === "number" ? kpi.target : NaN;
  const value = typeof kpi.value === "number" ? kpi.value : NaN;
  if (!isFinite(target) || !isFinite(value) || target === 0) return 0;
  const over = (value - target) / Math.abs(target);
  return kpi.direction === "below" ? -over : over;
}

export function worstVariance(tile: SummaryTile): number {
  let worst = 0;
  for (const k of tile.kpis || []) {
    const v = normalisedVariance(k);
    if (v > worst) worst = v;
  }
  return worst;
}

export interface Movement {
  from: number;
  to: number;
  abs: number;
  pct?: number;
}

export function seriesMovement(values: number[]): Movement | undefined {
  const pts = values.filter((v) => typeof v === "number" && isFinite(v));
  if (pts.length < 2) return undefined;
  const from = pts[0];
  const to = pts[pts.length - 1];
  const abs = to - from;
  return { from, to, abs, pct: from === 0 ? undefined : abs / Math.abs(from) };
}

export interface Band {
  id: "attention" | "steady" | "quiet";
  title: string;
  note: string;
  tiles: SummaryTile[];
}

export function bandTiles(tiles: SummaryTile[]): Band[] {
  const attention: SummaryTile[] = [];
  const steady: SummaryTile[] = [];
  const quiet: SummaryTile[] = [];

  for (const t of tiles) {
    const rank = severityRank(t.status);
    if (rank <= 1) attention.push(t);
    else if (rank === 2) steady.push(t);
    else quiet.push(t);
  }

  const bySeverity = (a: SummaryTile, b: SummaryTile) =>
    severityRank(a.status) - severityRank(b.status) ||
    worstVariance(b) - worstVariance(a) ||
    (a.definition || a.name || "").localeCompare(b.definition || b.name || "");

  attention.sort(bySeverity);
  steady.sort(bySeverity);
  quiet.sort(bySeverity);

  return [
    {
      id: "attention",
      title: "Needs attention",
      note: "Critical first, then warnings, each ordered by how far past its target it is.",
      tiles: attention,
    },
    { id: "steady", title: "Steady", note: "Reporting, and within tolerance.", tiles: steady },
    { id: "quiet", title: "Quiet", note: "No status reported. Not the same as healthy.", tiles: quiet },
  ];
}

export type SeverityKey = "critical" | "warn" | "ok" | "none";

export function severityKey(status: string | undefined): SeverityKey {
  switch (severityRank(status)) {
    case 0:
      return "critical";
    case 1:
      return "warn";
    case 2:
      return "ok";
    default:
      return "none";
  }
}

export function statusCounts(tiles: SummaryTile[]): Record<SeverityKey, number> {
  const out: Record<SeverityKey, number> = { critical: 0, warn: 0, ok: 0, none: 0 };
  for (const t of tiles) out[severityKey(t.status)]++;
  return out;
}
