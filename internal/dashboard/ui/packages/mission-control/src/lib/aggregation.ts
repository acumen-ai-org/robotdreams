import type { SummaryTile } from "./types";
import { LEVEL_NAMES } from "./routes";

export interface RollupLevel {
  rel: number;
  count: number;
  name: string;
}

export interface Rollup {
  leaves: number;
  levels: number;
  byLevel: RollupLevel[];
  direct: number;
  paths: string[];
}

const depthOf = (p: string) => (p ? p.split("/").filter(Boolean).length : 0);

const levelName = (depth: number) => LEVEL_NAMES[depth - 1] || "scope";

function asPaths(v: unknown): string[] {
  return Array.isArray(v) ? v.filter((x): x is string => typeof x === "string") : [];
}

function asCount(v: unknown): number | null {
  return typeof v === "number" ? v : Array.isArray(v) ? v.length : null;
}

export function rollupOf(tile: SummaryTile, scope: string): Rollup | null {
  const paths = asPaths(tile.scopes);
  const here = depthOf(tile.scope || scope);
  const leaves = paths.length || asCount(tile.instances) || 0;
  if (!leaves) return null;

  if (!paths.length) {
    return { leaves, levels: 0, byLevel: [], direct: 0, paths: [] };
  }

  const counts = new Map<number, number>();
  for (const p of paths) {
    const rel = Math.max(0, depthOf(p) - here);
    counts.set(rel, (counts.get(rel) || 0) + 1);
  }
  const byLevel = [...counts.entries()]
    .sort((a, b) => a[0] - b[0])
    .map(([rel, count]) => ({ rel, count, name: levelName(here + rel) }));

  return {
    leaves,
    levels: byLevel.length ? byLevel[byLevel.length - 1].rel : 0,
    byLevel,
    direct: counts.get(0) || 0,
    paths: [...paths].sort((a, b) => depthOf(a) - depthOf(b) || a.localeCompare(b)),
  };
}

export function rollupTitle(r: Rollup): string {
  const lead = r.leaves === 1 ? "Based on 1 report" : "Based on " + r.leaves + " reports";
  if (!r.levels) {
    return r.byLevel.length ? lead + ", written at this scope — nothing was rolled up." : lead + ".";
  }
  const rungs = r.byLevel
    .filter((l) => l.rel > 0)
    .map((l) => l.count + " " + l.name + (l.count === 1 ? "" : "s") + " down")
    .join(", ");
  return (
    lead +
    ", rolled up through " +
    r.levels +
    (r.levels === 1 ? " level" : " levels") +
    " of aggregation" +
    (rungs ? " — " + rungs : "") +
    (r.direct ? ", plus " + r.direct + " written here" : "") +
    "."
  );
}
