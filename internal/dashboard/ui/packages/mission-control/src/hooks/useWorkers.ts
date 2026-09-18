import { useEffect, useMemo, useState } from "react";
import { useConnection, ApiError } from "../state/ConnectionContext";
import { underAnyScope } from "../lib/routes";
import type { Worker } from "../lib/types";

export interface WorkerIndex {
  workers: Worker[];
  byId: Map<string, Worker>;
  childrenOf: Map<string, Worker[]>;
  matches: Set<string>;
  matchAll: boolean;
  pending: boolean;
  scopeSel: string[];
  nodesUnder: Map<string, number>;
  error: string;
}

function buildIndex(workers: Worker[]): Pick<WorkerIndex, "workers" | "byId" | "childrenOf"> {
  const byId = new Map<string, Worker>();
  const childrenOf = new Map<string, Worker[]>();
  for (const w of workers) byId.set(w.id, w);
  for (const w of workers) {
    const parent = w.reports_to && byId.has(w.reports_to) ? w.reports_to : "";
    if (!childrenOf.has(parent)) childrenOf.set(parent, []);
    childrenOf.get(parent)!.push(w);
  }
  for (const kids of childrenOf.values()) kids.sort((a, b) => a.id.localeCompare(b.id));
  return { workers, byId, childrenOf };
}

export function useWorkers(version: number, active: boolean, scopes: string[] = [], roles: string[] = []): WorkerIndex {
  const conn = useConnection();
  const [workers, setWorkers] = useState<Worker[]>([]);
  const [error, setError] = useState("");
  const [pending, setPending] = useState(() => active && conn.hasToken);

  useEffect(() => {
    if (!active || !conn.hasToken) {
      setPending(false);
      return;
    }
    let stale = false;
    setPending(true);
    conn
      .apiJSON<{ workers?: Worker[] }>("/api/workers")
      .then((data) => {
        if (!stale) {
          setWorkers(data.workers || []);
          setError("");
        }
      })
      .catch((e) => {
        if (stale) return;
        if (e instanceof ApiError && (e.status === 401 || e.status === 403)) {
          conn.logout();
          return;
        }
        setError("Could not load workers: " + (e instanceof Error ? e.message : String(e)));
      })
      .finally(() => {
        if (!stale) setPending(false);
      });
    return () => {
      stale = true;
    };
  }, [conn, conn.hasToken, conn.session, version, active]);

  const key = scopes.join(",");
  const roleKey = roles.join(",");
  const idx = useMemo(() => {
    const sel = key ? key.split(",") : [];
    const wanted = roleKey ? new Set(roleKey.split(",")) : null;
    const matchAll = !sel.length && !wanted;
    const matches = new Set<string>();
    for (const w of workers) {
      const okScope = !sel.length || (!!scopeOf(w) && underAnyScope(scopeOf(w), sel));
      const okRole = !wanted || wanted.has(w.role || "—");
      if (okScope && okRole) matches.add(w.id);
    }
    const nodesUnder = new Map<string, number>();
    for (const w of workers) {
      const segs = scopeOf(w).split("/").filter(Boolean);
      let path = "";
      for (const seg of segs) {
        path = path ? path + "/" + seg : seg;
        nodesUnder.set(path, (nodesUnder.get(path) || 0) + 1);
      }
    }
    return { ...buildIndex(workers), matches, matchAll, scopeSel: sel, nodesUnder };
  }, [workers, key, roleKey]);
  return useMemo(() => ({ ...idx, error, pending }), [idx, error, pending]);
}

export function countSubtree(childrenOf: Map<string, Worker[]>, workerID: string): { total: number; active: number } {
  let total = 0;
  let active = 0;
  const stack = (childrenOf.get(workerID) || []).slice();
  while (stack.length) {
    const w = stack.pop()!;
    total++;
    if (w.status === "connected") active++;
    const kids = childrenOf.get(w.id);
    if (kids) stack.push(...kids);
  }
  return { total, active };
}

export function placeInScope(index: Pick<WorkerIndex, "scopeSel">, path: string): boolean {
  if (!index.scopeSel.length) return true;
  return underAnyScope(path, index.scopeSel);
}

export function scopeOf(w: Worker | undefined): string {
  return w?.metadata?.scope || "";
}
