import { useVocabulary } from "../../state/VocabularyContext";
import { reportingRoute, type Route } from "../../lib/routes";
import { rollupOf, rollupTitle } from "../../lib/aggregation";
import { scopeAncestors, scopeLeaf } from "../../lib/scopePath";
import type { Contributor, SummaryTile } from "../../lib/types";
import { StatusBadge } from "../shared";
import { RollupMark } from "../RollupMark";
import { FactsBox } from "./FactsBox";
import { HierarchyBox, type HierarchyEntry } from "./HierarchyBox";
import { ReportIcon } from "../icons";
import { Link } from "../Link";

export function reportLevels(tile: SummaryTile | null, scope: string): { path: string; leaf: boolean }[] {
  const roll = tile ? rollupOf(tile, scope) : null;
  const below = (roll?.paths || []).filter((p) => p !== scope);
  return [
    ...scopeAncestors(scope).map((p) => ({ path: p, leaf: false })),
    { path: scope, leaf: !below.length },
    ...below.map((p) => ({ path: p, leaf: true })),
  ];
}

export function ReportFacts({ tile, scope }: { tile: SummaryTile | null; scope: string }) {
  const roll = tile ? rollupOf(tile, scope) : null;
  const aggregated = !!roll && roll.levels > 0;
  const lines: React.ReactNode[] = [];
  if (tile?.headline) lines.push(tile.headline);
  lines.push(aggregated ? "rolled up from " + roll.leaves + " below" : "written at this scope");
  return (
    <FactsBox
      caption={aggregated ? "Aggregated" : "Written here"}
      icon={<ReportIcon size={14} />}
      figure={tile?.status || "unknown"}
      tone={
        tile?.status === "critical" || tile?.status === "warn" ? "attention" : tile?.status === "ok" ? "ok" : undefined
      }
      lines={lines}
    />
  );
}

export function ReportHierarchy({
  tile,
  scope,
  onPickReport,
}: {
  tile: SummaryTile | null;
  scope: string;
  onPickReport: (scope: string) => void;
}) {
  const roll = tile ? rollupOf(tile, scope) : null;
  const up: HierarchyEntry[] = scopeAncestors(scope).map((p) => ({
    key: p,
    label: scopeLeaf(p),
    hint: "The same report rolled up to " + p,
    onPick: () => onPickReport(p),
  }));
  const down: HierarchyEntry[] = (roll?.paths || [])
    .filter((p) => p !== scope)
    .map((p) => ({
      key: p,
      label: scopeLeaf(p),
      hint: "The instance written at " + p,
      onPick: () => onPickReport(p),
    }));
  return (
    <HierarchyBox
      upCaption="Rolls up to"
      up={up}
      downCaption="Built from"
      down={down}
      emptyDown="nothing — written here"
    />
  );
}

export function ReportContributors({
  tile,
  scope,
  route,
  onPickNode,
  onPickReport,
}: {
  tile: SummaryTile | null;
  scope: string;
  route: Route;
  onPickNode: (id: string) => void;
  onPickReport: (scope: string) => void;
}) {
  const vocab = useVocabulary();
  const roll = tile ? rollupOf(tile, scope) : null;
  const contributors: Contributor[] = tile?.contributors || [];

  if (!tile) return <p className="mc-body-muted">Loading…</p>;

  return (
    <div className="mc-report-sources-body">
      {roll && (
        <p className="mc-body-muted" title={rollupTitle(roll)}>
          <RollupMark tile={tile} scope={scope} />{" "}
          {roll.levels > 0
            ? "from " +
              roll.leaves +
              (roll.leaves === 1 ? " report" : " reports") +
              " below this " +
              vocab.lower(Math.max(0, scope.split("/").filter(Boolean).length - 1))
            : "written at this scope — nothing rolled up"}
        </p>
      )}

      {contributors.length === 0 ? (
        <p className="mc-body-muted">This control plane does not report who wrote the contributing instances.</p>
      ) : (
        <ul className="mc-contributors">
          {contributors.map((c) => (
            <li key={c.scope} className="mc-contributor">
              <button type="button" className="mc-contributor-scope" onClick={() => onPickReport(c.scope)}>
                <span className="mc-mono">{scopeLeaf(c.scope) || c.scope}</span>
              </button>
              {c.producer ? (
                <button
                  type="button"
                  className="mc-contributor-by"
                  title={"Written by " + c.producer}
                  onClick={() => onPickNode(c.producer as string)}
                >
                  {c.producer}
                </button>
              ) : (
                <span className="mc-contributor-by is-unknown">producer not recorded</span>
              )}
            </li>
          ))}
        </ul>
      )}

      <Link
        className="mc-toolbox-button"
        to={reportingRoute(route, { scope, report: tile.definition || tile.name || "" })}
      >
        Open the full report ↗
      </Link>
    </div>
  );
}

export function ReportHeadline({ tile }: { tile: SummaryTile | null }) {
  if (!tile) return <p className="mc-body-muted">Loading…</p>;
  return (
    <div className="mc-report-read">
      <div className="mc-report-read-head">
        <StatusBadge status={tile.status} />
        {tile.description && <span className="mc-label-muted">{tile.description}</span>}
      </div>
      {tile.headline ? (
        <p className="mc-report-read-line">{tile.headline}</p>
      ) : (
        <p className="mc-body-muted">No headline reported.</p>
      )}
      {tile.headline_partial && (
        <span className="mc-label-muted">one contributor's words — the numbers cover all of them</span>
      )}
    </div>
  );
}
