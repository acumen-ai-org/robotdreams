import { useEffect, useState } from "react";
import { useConnection } from "../../state/ConnectionContext";
import { useVocabulary } from "../../state/VocabularyContext";
import { reportingRoute, underScope, type Route } from "../../lib/routes";
import { scopeAncestors, scopeDepth, scopeLeaf, scopeParent } from "../../lib/scopePath";
import { scopeChildren } from "../../lib/scopeTree";
import { severityRank } from "../../lib/triage";
import type { ScopeEntry, SummaryTile } from "../../lib/types";
import type { WorkerIndex } from "../../hooks/useWorkers";
import { scopeOf } from "../../hooks/useWorkers";
import { SignalTile } from "../../views/reporting/SignalTile";
import { FactsBox } from "./FactsBox";
import { HierarchyBox, type HierarchyEntry } from "./HierarchyBox";
import { MapIcon } from "../icons";
import { statusDotClass } from "../shared";

export function useScopeTiles(scope: string | null): SummaryTile[] | null {
  const conn = useConnection();
  const [tiles, setTiles] = useState<SummaryTile[] | null>(null);
  useEffect(() => {
    if (scope === null || !conn.hasToken) {
      setTiles(null);
      return;
    }
    let stale = false;
    setTiles(null);
    conn
      .apiJSON<{ tiles?: SummaryTile[] }>("/api/reports/summary?scope=" + encodeURIComponent(scope))
      .then((d) => {
        if (!stale) setTiles(d.tiles || []);
      })
      .catch(() => {
        if (!stale) setTiles([]);
      });
    return () => {
      stale = true;
    };
  }, [conn, conn.hasToken, conn.session, scope]);
  return tiles;
}

export function placeFacts(path: string, index: WorkerIndex | undefined, tiles: SummaryTile[] | null) {
  const within = index ? index.workers.filter((w) => scopeOf(w) && underScope(scopeOf(w), path)) : [];
  const active = within.filter((w) => w.status === "connected").length;
  let worst = "";
  for (const t of tiles || []) {
    if (severityRank(t.status) > severityRank(worst)) worst = t.status || "";
  }
  return { within: within.length, active, worst };
}

export function PlaceFacts({ path, index, tiles }: { path: string; index?: WorkerIndex; tiles: SummaryTile[] | null }) {
  const vocab = useVocabulary();
  const { within, active, worst } = placeFacts(path, index, tiles);
  const lines: React.ReactNode[] = [
    <span key="p" className="mc-mono">
      {path}
    </span>,
  ];
  if (worst && worst !== "ok") lines.push(<strong key="w">{worst}</strong>);
  return (
    <FactsBox
      caption={vocab.one(Math.max(0, scopeDepth(path) - 1))}
      icon={<MapIcon size={14} />}
      figure={active}
      of={within}
      tone={worst === "critical" || worst === "warn" ? "attention" : undefined}
      lines={lines}
    />
  );
}

export function PlaceHierarchy({
  path,
  scopes,
  onPickPlace,
}: {
  path: string;
  scopes: ScopeEntry[];
  onPickPlace: (p: string) => void;
}) {
  const vocab = useVocabulary();
  const parent = scopeParent(path);
  const up: HierarchyEntry[] = scopeAncestors(path).map((p) => ({
    key: p,
    label: scopeLeaf(p),
    hint: p,
    onPick: () => onPickPlace(p),
  }));
  const kids = scopeChildren(scopes, path);
  const down: HierarchyEntry[] = kids.map((k) => ({
    key: k.path,
    label: k.name,
    hint: k.path,
    onPick: () => onPickPlace(k.path),
  }));
  return (
    <HierarchyBox
      upCaption={parent ? "Sits in" : "Sits in"}
      up={up}
      downCaption="Contains"
      down={down}
      emptyDown={"no " + vocab.many(Math.min(4, scopeDepth(path))).toLowerCase()}
    />
  );
}

export function PlaceReports({ path, route, tiles }: { path: string; route: Route; tiles: SummaryTile[] | null }) {
  if (tiles === null) return <p className="mc-body-muted">Loading…</p>;
  if (!tiles.length) return <p className="mc-body-muted">Nothing is reporting at or beneath this scope yet.</p>;
  return (
    <div className="mc-side-tiles">
      {tiles.map((t) => (
        <SignalTile
          key={t.definition || t.name}
          tile={t}
          dense
          to={reportingRoute(route, { scope: path, report: t.definition || t.name || "" })}
        />
      ))}
    </div>
  );
}

export function PlaceNodes({
  path,
  index,
  onPickNode,
}: {
  path: string;
  index?: WorkerIndex;
  onPickNode: (id: string) => void;
}) {
  const vocab = useVocabulary();
  if (!index) return <p className="mc-body-muted">The fleet is still loading.</p>;
  const within = index.workers.filter((w) => scopeOf(w) && underScope(scopeOf(w), path));
  if (!within.length) {
    return (
      <p className="mc-body-muted">
        No {vocab.many(4).toLowerCase()} report for this place. It exists because something publishes here, or because a
        place below it does.
      </p>
    );
  }
  const here = within.filter((w) => scopeOf(w) === path);
  const below = within.filter((w) => scopeOf(w) !== path);
  return (
    <div className="mc-place-nodes">
      {here.length > 0 && (
        <NodeRows label={"At this " + vocab.lower(scopeDepth(path) - 1)} rows={here} onPick={onPickNode} />
      )}
      {below.length > 0 && <NodeRows label="Below it" rows={below} onPick={onPickNode} />}
    </div>
  );
}

function NodeRows({
  label,
  rows,
  onPick,
}: {
  label: string;
  rows: { id: string; role?: string; status?: string }[];
  onPick: (id: string) => void;
}) {
  return (
    <div className="mc-detail-section">
      <span className="mc-label-muted">
        {label} · {rows.length}
      </span>
      <ul className="mc-node-rows">
        {rows.map((w) => (
          <li key={w.id}>
            <button type="button" className="mc-node-row" onClick={() => onPick(w.id)}>
              <span className={"mc-status-dot " + statusDotClass(w.status || "disconnected")} aria-hidden="true" />
              <span className="mc-mono mc-node-row-id">{w.id}</span>
              <span className="mc-label-muted">{w.role || "—"}</span>
            </button>
          </li>
        ))}
      </ul>
    </div>
  );
}
