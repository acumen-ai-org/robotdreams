import { placeInScope, scopeOf, type WorkerIndex } from "../hooks/useWorkers";
import type { ScopeEntry, Worker } from "./types";

export const MAX_PLACE_DEPTH = 3;

export type RowKind = "place" | "node";

export interface Row {
  id: string;
  kind: RowKind;
  label: string;
  worker?: Worker;
  path?: string;
  level: number;
  index: number;
  siblings: number;
  realm?: number;
}

export interface RowTree {
  childrenOf: Map<string, Row[]>;
  byId: Map<string, Row>;
  parentOf: Map<string, string>;
  roll: Map<string, { active: number; total: number }>;
  reports: Map<string, { here: number; under: number }>;
}

const isConnected = (w: Worker) => w.status === "connected";

function emptyTree(): RowTree {
  return { childrenOf: new Map([["", []]]), byId: new Map(), parentOf: new Map(), roll: new Map(), reports: new Map() };
}

function push(t: RowTree, parent: string, row: Row) {
  if (!t.childrenOf.has(parent)) t.childrenOf.set(parent, []);
  t.childrenOf.get(parent)!.push(row);
  t.byId.set(row.id, row);
  t.parentOf.set(row.id, parent);
}

function rollUp(t: RowTree) {
  const visit = (id: string): { active: number; total: number } => {
    let total = 0;
    let active = 0;
    for (const c of t.childrenOf.get(id) || []) {
      if (c.kind === "node" && c.worker) {
        total++;
        if (isConnected(c.worker)) active++;
      }
      const sub = visit(c.id);
      total += sub.total;
      active += sub.active;
    }
    t.roll.set(id, { active, total });
    return { active, total };
  };
  visit("");
}

function sortRows(t: RowTree) {
  for (const list of t.childrenOf.values()) {
    list.sort((a, b) => Number(a.kind === "node") - Number(b.kind === "node") || a.label.localeCompare(b.label));
  }
}

function numberPlaces(t: RowTree) {
  const walk = (parent: string, realm: number | undefined) => {
    const places = (t.childrenOf.get(parent) || []).filter((r) => r.kind === "place");
    places.forEach((r, i) => {
      const isRealm = r.level === 2;
      r.index = isRealm ? i % 10 : i + 1;
      r.siblings = places.length;
      r.realm = isRealm ? i % 10 : realm;
      walk(r.id, r.realm);
    });
    const nodes = (t.childrenOf.get(parent) || []).filter((r) => r.kind === "node");
    nodes.forEach((n, i) => {
      n.index = i + 1;
      n.siblings = nodes.length;
      n.realm = realm;
    });
  };
  walk("", undefined);
}

function rollReports(t: RowTree, here: Map<string, number>) {
  const visit = (id: string): number => {
    const row = t.byId.get(id);
    const own = (row?.path && here.get(row.path)) || 0;
    let under = own;
    for (const c of t.childrenOf.get(id) || []) under += visit(c.id);
    t.reports.set(id, { here: own, under });
    return under;
  };
  for (const c of t.childrenOf.get("") || []) visit(c.id);
}

export function placeRows(index: Pick<WorkerIndex, "workers">, scopes: ScopeEntry[]): RowTree {
  const t = emptyTree();
  const byRel = new Map<string, Row>();

  const place = (all: string[]): Row | undefined => {
    let rel = "";
    let full = all[0] || "";
    let parent = "";
    let row: Row | undefined;
    for (let i = 1; i < all.length && i <= MAX_PLACE_DEPTH; i++) {
      const seg = all[i];
      rel = rel ? rel + "/" + seg : seg;
      full = full ? full + "/" + seg : seg;
      let next = byRel.get(rel);
      if (!next) {
        next = { id: "scope:" + full, kind: "place", label: seg, path: full, level: i, index: 0, siblings: 0 };
        byRel.set(rel, next);
        push(t, parent, next);
      }
      row = next;
      parent = next.id;
    }
    return row;
  };

  for (const s of scopes) place(s.path.split("/").filter(Boolean));

  for (const w of index.workers) {
    const home = place(scopeOf(w).split("/").filter(Boolean));
    push(t, home?.id ?? "", { id: w.id, kind: "node", label: w.id, worker: w, level: 0, index: 0, siblings: 0 });
  }

  sortRows(t);
  numberPlaces(t);
  rollUp(t);
  const recent = new Map<string, number>();
  for (const s of scopes) if (typeof s.recent === "number") recent.set(s.path, s.recent);
  if (recent.size) rollReports(t, recent);
  return t;
}

export function reportRows(index: Pick<WorkerIndex, "childrenOf">): RowTree {
  const t = emptyTree();
  const walk = (parent: string) => {
    for (const w of index.childrenOf.get(parent === "" ? "" : parent) || []) {
      push(t, parent, { id: w.id, kind: "node", label: w.id, worker: w, level: 0, index: 0, siblings: 0 });
      walk(w.id);
    }
  };
  walk("");
  rollUp(t);
  return t;
}

export function rowInSelection(index: Pick<WorkerIndex, "matchAll" | "matches" | "scopeSel">, row: Row): boolean {
  if (row.kind === "node") return index.matchAll || index.matches.has(row.id);
  return placeInScope(index, row.path || "");
}
