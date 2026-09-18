import { Sparkline } from "../../panels/Sparkline";
import { StatusBadge, KpiStat, tileName, contributionText } from "../../components/shared";
import { RollupMark } from "../../components/RollupMark";
import { worstVariance } from "../../lib/triage";
import type { SummaryTile, SeriesPoint, KPIValue } from "../../lib/types";
import { Link } from "../../components/Link";
import type { Route } from "../../lib/routes";

const KPIS_DENSE = 2;
const KPIS_FULL = 4;

export function SignalTile({
  tile,
  to,
  dense,
  onSelect,
  selected,
  tileKey,
}: {
  tile: SummaryTile;
  to: Route;
  dense?: boolean;
  onSelect?: () => void;
  selected?: boolean;
  tileKey?: string;
}) {
  const name = tileName(tile);
  const sparkPts: SeriesPoint[] | undefined = Array.isArray(tile.sparkline) ? tile.sparkline : tile.sparkline?.points;
  const contrib = contributionText(tile);
  const status = tile.status || "none";
  const kpis = (tile.kpis || []).slice(0, dense ? KPIS_DENSE : KPIS_FULL);
  const off = worstVariance(tile);

  const cls = "mc-signal-tile mc-status-" + status + (dense ? " is-dense" : "") + (selected ? " is-selected" : "");

  const head = (
    <div className="mc-signal-head">
      <span className="mc-signal-name mc-mono">{name}</span>
      {tile.scope && <span className="mc-signal-scope mc-mono">{tile.scope}</span>}
      <StatusBadge status={tile.status} />
      {onSelect && (
        <Link
          className="mc-signal-open"
          to={to}
          title={"Open the full " + name + " report"}
          aria-label={"Open the full " + name + " report"}
          onClick={(e) => e.stopPropagation()}
        >
          ↗
        </Link>
      )}
    </div>
  );

  if (onSelect) {
    return (
      <div
        className={cls}
        data-mc-tile={tileKey}
        role="button"
        tabIndex={0}
        aria-pressed={!!selected}
        onClick={onSelect}
        onKeyDown={(e) => {
          if (e.key === "Enter" || e.key === " ") {
            e.preventDefault();
            onSelect();
          }
        }}
      >
        {head}
        <TileBody
          tile={tile}
          dense={dense}
          contrib={contrib}
          scope={to.scope}
          sparkPts={sparkPts}
          kpis={kpis}
          off={off}
        />
      </div>
    );
  }

  return (
    <Link className={cls} to={to}>
      {head}
      <TileBody
        tile={tile}
        dense={dense}
        contrib={contrib}
        scope={to.scope}
        sparkPts={sparkPts}
        kpis={kpis}
        off={off}
      />
    </Link>
  );
}

function TileBody({
  tile,
  dense,
  contrib,
  scope,
  sparkPts,
  kpis,
  off,
}: {
  tile: SummaryTile;
  dense?: boolean;
  contrib: string;
  scope: string;
  sparkPts?: SeriesPoint[];
  kpis: KPIValue[];
  off: number;
}) {
  return (
    <div className="mc-signal-body">
      {tile.headline ? (
        <p className="mc-signal-headline">{tile.headline}</p>
      ) : (
        <p className="mc-signal-headline is-quiet">No headline reported.</p>
      )}

      {tile.headline_partial && (
        <span
          className="mc-signal-partial"
          title="This report's headline is written by its producer and cannot be rolled up"
        >
          one of {contrib || "several"} — the numbers below cover all
        </span>
      )}

      {off > 0.001 && (
        <span className="mc-signal-off" title="Worst KPI, as a share of its target">
          {Math.round(off * 100)}% past target
        </span>
      )}

      {!dense && kpis.length > 0 && (
        <div className="mc-kpi-row">
          {kpis.map((k, i) => (
            <KpiStat key={k.name || i} kpi={k} />
          ))}
        </div>
      )}

      {!dense && sparkPts && sparkPts.length > 0 && <Sparkline points={sparkPts} />}

      <div className="mc-signal-foot">
        <RollupMark tile={tile} scope={tile.scope || scope} />
        <span className="mc-signal-tags">
          {(tile.stances || []).map((s) => (
            <span key={s} className="mc-tag mc-tag-stance">
              {s}
            </span>
          ))}
          {(tile.categories || []).map((c) => (
            <span key={c} className="mc-tag">
              {c}
            </span>
          ))}
        </span>
      </div>
    </div>
  );
}
