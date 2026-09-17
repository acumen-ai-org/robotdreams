import type { PlanItem, PlanView, SummaryTile } from "./types";

export interface PlanSource {
  definition: string;
  scope: string;
  status?: string;
  plan: PlanView;
  items: PlanItem[];
}

export const MS_PER_DAY = 86_400_000;

export interface StateUnion {
  states: string[];
  forced: boolean;
}

export function unionStates(sources: PlanSource[]): StateUnion {
  const order: string[] = [];
  const seen = new Set<string>();
  const after = new Map<string, Set<string>>();
  const indeg = new Map<string, number>();

  for (const s of sources) {
    const st = states(s.plan);
    for (const w of st) {
      if (!seen.has(w)) {
        seen.add(w);
        order.push(w);
        after.set(w, new Set());
        indeg.set(w, 0);
      }
    }
    for (let i = 0; i + 1 < st.length; i++) {
      const a = st[i];
      const b = st[i + 1];
      if (!after.get(a)!.has(b)) {
        after.get(a)!.add(b);
        indeg.set(b, (indeg.get(b) || 0) + 1);
      }
    }
  }

  const out: string[] = [];
  const free = order.filter((w) => (indeg.get(w) || 0) === 0);
  while (free.length) {
    free.sort((a, b) => order.indexOf(a) - order.indexOf(b));
    const w = free.shift()!;
    out.push(w);
    for (const nxt of after.get(w)!) {
      const d = (indeg.get(nxt) || 0) - 1;
      indeg.set(nxt, d);
      if (d === 0) free.push(nxt);
    }
  }
  if (out.length === order.length) return { states: out, forced: false };

  const position = new Map<string, { sum: number; n: number }>();
  for (const s of sources) {
    const st = states(s.plan);
    if (st.length < 2) {
      for (const w of st) {
        const e = position.get(w) || { sum: 0, n: 0 };
        e.sum += 1;
        e.n += 1;
        position.set(w, e);
      }
      continue;
    }
    st.forEach((w, i) => {
      const e = position.get(w) || { sum: 0, n: 0 };
      e.sum += i / (st.length - 1);
      e.n += 1;
      position.set(w, e);
    });
  }
  const avg = (w: string) => {
    const e = position.get(w);
    return e && e.n ? e.sum / e.n : 0.5;
  };
  const forcedOut = [...order].sort((a, b) => avg(a) - avg(b) || order.indexOf(a) - order.indexOf(b));
  return { states: forcedOut, forced: true };
}

function states(plan: PlanView): string[] {
  return (plan.states || []).map((c) => c.state);
}

export function flowPosition(plan: PlanView, item: PlanItem): number {
  const st = states(plan);
  const i = st.indexOf(item.state || "");
  if (i < 0) return -1;
  if (st.length <= 1) return i === 0 ? 1 : -1;
  return i / (st.length - 1);
}

export function isTerminal(plan: PlanView, item: PlanItem): boolean {
  return flowPosition(plan, item) === 1;
}

export function isBlocked(item: PlanItem): boolean {
  return (item.blocked_by || []).length > 0;
}

export function daysLate(item: PlanItem, now: number): number {
  if (!item.due) return NaN;
  const due = Date.parse(item.due);
  if (Number.isNaN(due)) return NaN;
  return (now - due) / MS_PER_DAY;
}

export function isLate(item: PlanItem, now: number): boolean {
  const d = daysLate(item, now);
  return !Number.isNaN(d) && d > 0;
}

export interface OverWip {
  state: string;
  count: number;
  limit: number;
}

export function overWip(plan: PlanView): OverWip[] {
  return (plan.states || [])
    .filter((c) => (c.limit || 0) > 0 && c.items > (c.limit || 0))
    .map((c) => ({ state: c.state, count: c.items, limit: c.limit || 0 }));
}

export function triageItems(items: PlanItem[], now: number): PlanItem[] {
  return [...items].sort((a, b) => {
    const ab = isBlocked(a),
      bb = isBlocked(b);
    if (ab !== bb) return ab ? -1 : 1;
    const al = daysLate(a, now),
      bl = daysLate(b, now);
    const aLate = !Number.isNaN(al) && al > 0,
      bLate = !Number.isNaN(bl) && bl > 0;
    if (aLate !== bLate) return aLate ? -1 : 1;
    if (aLate && bLate && al !== bl) return bl - al;
    const ad = a.due ? Date.parse(a.due) : Infinity;
    const bd = b.due ? Date.parse(b.due) : Infinity;
    if (ad !== bd) return ad - bd;
    return (a.title || a.id).localeCompare(b.title || b.id);
  });
}

export function groupByState(plan: PlanView, items: PlanItem[]): Map<string, PlanItem[]> {
  const out = new Map<string, PlanItem[]>();
  for (const c of plan.states || []) out.set(c.state, []);
  const known = new Set(out.keys());
  for (const it of items) {
    const key = it.state && known.has(it.state) ? it.state : "";
    if (!out.has(key)) out.set(key, []);
    out.get(key)!.push(it);
  }
  return out;
}

export function sharedHorizons(sources: PlanSource[]): string[] | null {
  const withHorizons = sources.filter((s) => (s.plan.horizons || []).length > 0);
  if (!withHorizons.length) return null;
  const first = (withHorizons[0].plan.horizons || []).map((h) => h.name);
  for (const s of withHorizons) {
    const names = (s.plan.horizons || []).map((h) => h.name);
    if (names.length !== first.length || names.some((n, i) => n !== first[i])) return null;
  }
  return first;
}

export interface PlanTally {
  items: number;
  blocked: number;
  overWip: number;
  late: number;
  done: number;
  truncated: boolean;
}

export function tallyPlans(sources: PlanSource[], now: number): PlanTally {
  const t: PlanTally = { items: 0, blocked: 0, overWip: 0, late: 0, done: 0, truncated: false };
  for (const s of sources) {
    t.items += s.plan.total ?? s.items.length;
    t.blocked += s.plan.blocked ?? s.items.filter(isBlocked).length;
    t.overWip += overWip(s.plan).length;
    t.truncated = t.truncated || !!s.plan.truncated;
    for (const c of s.plan.states || []) {
      if (c.state === (s.plan.states || []).at(-1)?.state) t.done += c.items;
    }
    for (const it of s.items) if (isLate(it, now)) t.late++;
  }
  return t;
}

export function tileHasPlan(tile: SummaryTile): boolean {
  return (tile.facets || []).includes("plan");
}

export function humanLabel(name: string): string {
  return name.replace(/[_-]+/g, " ");
}
