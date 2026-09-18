import { useEffect, useState } from "react";
import { useConnection } from "../../state/ConnectionContext";
import { blankRoute, reportingRoute, type Route } from "../../lib/routes";
import { OutcomesLink, StatusBadge, tileName } from "../../components/shared";
import { Link } from "../../components/Link";
import { countSubtree, scopeOf, type WorkerIndex } from "../../hooks/useWorkers";
import type { SummaryTile, Worker } from "../../lib/types";
import { phaseClass, phaseOf, type RolloutNode } from "../../lib/updates";
import { fmtTime } from "../../lib/format";

const BLANK: Route = blankRoute("reporting");

interface Props {
  worker: Worker;
  index: WorkerIndex;
  update?: RolloutNode;
  onSelect: (id: string | null) => void;
}

export function NodeDetail({ worker, index, update, onSelect }: Props) {
  const conn = useConnection();
  const scope = scopeOf(worker);
  const { total, active } = countSubtree(index.childrenOf, worker.id);
  const [tiles, setTiles] = useState<SummaryTile[] | null>(null);

  useEffect(() => {
    if (!scope || !conn.hasToken) {
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

  const segs = scope ? scope.split("/") : [];

  return (
    <section className="mc-panel mc-node-detail" aria-label="Node detail">
      <dl className="mc-detail-list">
        <dt>Status</dt>
        <dd>
          <StatusBadge status={worker.status === "connected" ? "ok" : "unknown"} />
          <span className="mc-body-muted"> {worker.status || "unknown"}</span>
        </dd>
        <dt>Role</dt>
        <dd>{worker.role || "—"}</dd>
        <dt>Reports to</dt>
        <dd>
          {worker.reports_to ? (
            <button className="mc-link-button" type="button" onClick={() => onSelect(worker.reports_to!)}>
              {worker.reports_to}
            </button>
          ) : (
            <span className="mc-body-muted">nobody — a root of the org chart</span>
          )}
        </dd>
        {total > 0 && (
          <>
            <dt>Subtree</dt>
            <dd className="mc-mono">
              {total} reports, {active} active
            </dd>
          </>
        )}
      </dl>

      {update && (
        <div className="mc-detail-section">
          <span className="mc-detail-label">Update</span>
          <p className="mc-detail-update">
            <span className={"mc-rollout-phase " + phaseClass(update.status)} title={phaseOf(update.status).hint}>
              {phaseOf(update.status).label}
            </span>
            <span className="mc-mono">
              {update.current_version || "version not reported"}
              {update.target_version && update.target_version !== update.current_version
                ? " \u2192 " + update.target_version
                : ""}
            </span>
          </p>
          <p className="mc-body-muted mc-detail-update-kind">
            <span className="mc-mono">{update.kind}</span>
            {update.reported_at ? " \u00b7 reported " + fmtTime(update.reported_at) : " \u00b7 never reported"}
          </p>
          {update.detail && <p className="mc-body-muted">{update.detail}</p>}
        </div>
      )}

      {!scope ? (
        <p className="mc-empty-state">This node connected without a reporting scope, so there is nothing to link to.</p>
      ) : (
        <>
          <div className="mc-detail-section">
            <span className="mc-detail-label-row">
              <span className="mc-detail-label">Scope</span>
              <OutcomesLink scope={scope} />
            </span>
            <span className="mc-scope-crumbs">
              {segs.map((seg, i) => {
                const path = segs.slice(0, i + 1).join("/");
                return (
                  <Link key={path} className="mc-scope-crumb" to={reportingRoute(BLANK, { scope: path })}>
                    {seg}
                  </Link>
                );
              })}
            </span>
          </div>

          <div className="mc-detail-section">
            <span className="mc-detail-label">Reports here</span>
            {tiles === null ? (
              <p className="mc-empty-state">Loading…</p>
            ) : tiles.length === 0 ? (
              <p className="mc-empty-state">
                An aggregation scope: reports roll up to here, but none are authored at this level.
              </p>
            ) : (
              <ul className="mc-detail-reports">
                {tiles.map((t) => {
                  const name = tileName(t);
                  return (
                    <li key={name}>
                      <Link to={reportingRoute(BLANK, { scope, report: name })}>
                        <span className="mc-detail-report-name">{name}</span>
                        <StatusBadge status={t.status} />
                        <span className="mc-report-goto" aria-hidden="true">
                          ↗
                        </span>
                      </Link>
                      {t.headline && <span className="mc-detail-report-headline mc-body-muted">{t.headline}</span>}
                    </li>
                  );
                })}
              </ul>
            )}
          </div>
        </>
      )}
    </section>
  );
}
