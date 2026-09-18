import { useVocabulary } from "../../state/VocabularyContext";
import { reportingRoute, type Route } from "../../lib/routes";
import { nodeEmoji } from "../../lib/vocabulary";
import { scopeAncestors, scopeLeaf } from "../../lib/scopePath";
import type { SummaryTile, Worker } from "../../lib/types";
import { countSubtree, scopeOf, type WorkerIndex } from "../../hooks/useWorkers";
import { SignalTile } from "../../views/reporting/SignalTile";
import { FactsBox } from "./FactsBox";
import { HierarchyBox, type HierarchyEntry } from "./HierarchyBox";

export function NodeFacts({ worker, index }: { worker: Worker; index?: WorkerIndex }) {
  const sub = index ? countSubtree(index.childrenOf, worker.id) : { total: 0, active: 0 };
  const connected = worker.status === "connected";
  const lines: React.ReactNode[] = [worker.role || "no role given"];
  if (sub.total > 0) {
    lines.push(
      <>
        <strong>{sub.active}</strong> of <strong>{sub.total}</strong> beneath it active
      </>,
    );
  }
  return (
    <FactsBox
      caption={worker.role || "Node"}
      icon={<span className="mc-node-emoji">{nodeEmoji(worker.role)}</span>}
      figure={worker.status || "unknown"}
      tone={connected ? "ok" : "attention"}
      lines={lines}
    />
  );
}

export function NodeHierarchy({
  worker,
  index,
  onPickNode,
  onPickPlace,
}: {
  worker: Worker;
  index?: WorkerIndex;
  onPickNode: (id: string) => void;
  onPickPlace: (path: string) => void;
}) {
  const vocab = useVocabulary();
  const scope = scopeOf(worker);

  const up: HierarchyEntry[] = [];
  if (worker.reports_to) {
    up.push({
      key: worker.reports_to,
      label: worker.reports_to,
      hint: "Reports to " + worker.reports_to,
      onPick: () => onPickNode(worker.reports_to as string),
    });
  }
  if (scope) {
    up.push({
      key: "scope:" + scope,
      label: scopeLeaf(scope),
      hint: "Sits in " + scope,
      onPick: () => onPickPlace(scope),
    });
    for (const p of scopeAncestors(scope).reverse()) {
      up.push({ key: "scope:" + p, label: scopeLeaf(p), hint: p, onPick: () => onPickPlace(p) });
    }
  }

  const kids = index ? index.childrenOf.get(worker.id) || [] : [];
  const down: HierarchyEntry[] = kids.map((w) => ({
    key: w.id,
    label: w.id,
    hint: (w.role || "node") + " — " + w.id,
    onPick: () => onPickNode(w.id),
  }));

  return (
    <HierarchyBox
      upCaption="Answers to"
      up={up}
      downCaption={vocab.many(4)}
      down={down}
      emptyDown="nobody reports to it"
    />
  );
}

export function NodeReports({ worker, route, tiles }: { worker: Worker; route: Route; tiles: SummaryTile[] | null }) {
  const scope = scopeOf(worker);
  if (!scope) {
    return <p className="mc-body-muted">This node connected without a reporting scope, so it writes no reports.</p>;
  }
  if (tiles === null) return <p className="mc-body-muted">Loading…</p>;
  const mine = tiles.filter((t) => (t.contributors || []).some((c) => c.producer === worker.id));
  if (!mine.length) {
    return (
      <p className="mc-body-muted">
        Nothing at this scope names this node as its producer. Reports here may be written by a sibling, or rolled up
        from below.
      </p>
    );
  }
  return (
    <div className="mc-side-tiles">
      {mine.map((t) => {
        const at = (t.contributors || []).find((c) => c.producer === worker.id);
        return (
          <SignalTile
            key={t.definition || t.name}
            tile={t}
            dense
            to={reportingRoute(route, { scope: at?.scope || scope, report: t.definition || t.name || "" })}
          />
        );
      })}
    </div>
  );
}
