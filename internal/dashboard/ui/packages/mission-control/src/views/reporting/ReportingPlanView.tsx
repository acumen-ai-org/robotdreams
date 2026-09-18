import { useEffect, useMemo, useState } from "react";
import { useConnection } from "../../state/ConnectionContext";
import { reportingRoute, type Route } from "../../lib/routes";
import { tileName } from "../../components/shared";
import { Link } from "../../components/Link";
import { DelayedLoading, Skeleton } from "../../components/Async";
import {
  daysLate,
  humanLabel,
  isBlocked,
  isLate,
  isTerminal,
  tallyPlans,
  tileHasPlan,
  unionStates,
  type PlanSource,
} from "../../lib/plan";
import type { PlanItem, ReportResponse, SummaryTile } from "../../lib/types";

interface Props {
  route: Route;
  tiles: SummaryTile[] | null;
  refreshTick: number;
  active: boolean;
  stuckOnly: boolean;
  periodQuery?: string;
}

const MAX_DEFINITIONS = 12;

const RANK: Record<string, number> = { critical: 0, warn: 1, ok: 2, unknown: 3 };

export default function ReportingPlanView({ route, tiles, refreshTick, active, stuckOnly, periodQuery = "" }: Props) {
  const conn = useConnection();
  const [sources, setSources] = useState<PlanSource[]>([]);
  const [error, setError] = useState("");

  const names = useMemo(() => (tiles || []).filter(tileHasPlan).map(tileName).slice(0, MAX_DEFINITIONS), [tiles]);
  const namesKey = names.join(",");
  const [loadedKey, setLoadedKey] = useState<string | null>(null);

  useEffect(() => {
    if (!active || !conn.hasToken || !names.length) {
      if (!names.length) {
        setSources([]);
        setLoadedKey("");
      }
      return;
    }
    let stale = false;
    Promise.all(
      names.map((n) =>
        conn
          .apiJSON<ReportResponse>(
            "/api/reports/report?definition=" +
              encodeURIComponent(n) +
              "&scope=" +
              encodeURIComponent(route.scope) +
              periodQuery,
          )
          .then((r) => [n, r] as const),
      ),
    )
      .then((pairs) => {
        if (stale) return;
        const out: PlanSource[] = [];
        for (const [name, r] of pairs) {
          const plan = r.plan;
          if (!plan || !(plan.states || []).length) continue;
          out.push({
            definition: name,
            scope: plan.scope || route.scope,
            status: r.summary?.status,
            plan,
            items: plan.items || [],
          });
        }
        out.sort((a, b) => {
          const d =
            (RANK[(a.status || "unknown").toLowerCase()] ?? 3) - (RANK[(b.status || "unknown").toLowerCase()] ?? 3);
          return d !== 0 ? d : a.definition.localeCompare(b.definition);
        });
        setSources(out);
        setLoadedKey(names.join(","));
        setError("");
      })
      .catch((e) => {
        if (!stale) setError("Could not load plans: " + (e instanceof Error ? e.message : String(e)));
      });
    return () => {
      stale = true;
    };
  }, [active, conn, conn.hasToken, conn.session, names, route.scope, refreshTick, periodQuery]);

  const now = Date.now();
  const tally = useMemo(() => tallyPlans(sources, now), [sources, now]);
  const union = useMemo(() => unionStates(sources), [sources]);

  const filtered = useMemo(() => {
    if (!stuckOnly) return sources;
    return sources
      .map((s) => ({
        ...s,
        items: s.items.filter((it) => (isBlocked(it) && !isTerminal(s.plan, it)) || isLate(it, now)),
      }))
      .filter((s) => s.items.length > 0);
  }, [sources, stuckOnly, now]);

  if (error) return <p className="mc-form-error">{error}</p>;
  const pending = tiles === null || (conn.hasToken && names.length > 0 && loadedKey !== namesKey);
  if (pending && !sources.length) {
    return (
      <DelayedLoading>
        <Skeleton lines={5} />
      </DelayedLoading>
    );
  }
  if (!sources.length) {
    return (
      <p className="mc-empty-state">
        No plans at this scope. A plan attaches where work is committed &mdash; try a realm or a site.
      </p>
    );
  }

  return (
    <>
      <p className="mc-plan-strip">
        <strong>{tally.items}</strong> items
        <span className="mc-plan-strip-sep">&middot;</span>
        <strong className={tally.blocked ? "is-bad" : ""}>{tally.blocked}</strong> blocked
        <span className="mc-plan-strip-sep">&middot;</span>
        <strong className={tally.overWip ? "is-bad" : ""}>{tally.overWip}</strong> column
        {tally.overWip === 1 ? "" : "s"} over the work-in-progress limit
        <span className="mc-plan-strip-sep">&middot;</span>
        <strong className={tally.late ? "is-bad" : ""}>{tally.late}</strong> past due
        <span className="mc-plan-strip-sep">&middot;</span>
        <strong>{tally.done}</strong> finished
      </p>

      {stuckOnly && !filtered.length && <p className="mc-empty-state">Nothing is blocked or past due at this scope.</p>}

      <AggregatedBoard route={route} sources={filtered} columns={union.states} forced={union.forced} />
    </>
  );
}

function AggregatedBoard({
  route,
  sources,
  columns,
  forced,
}: {
  route: Route;
  sources: PlanSource[];
  columns: string[];
  forced: boolean;
}) {
  const now = Date.now();
  const [openGroups, setOpenGroups] = useState<Set<string>>(new Set());
  const toggleGroup = (key: string) =>
    setOpenGroups((cur) => {
      const next = new Set(cur);
      if (next.has(key)) next.delete(key);
      else next.add(key);
      return next;
    });

  const grid = useMemo(() => {
    const byCol = new Map<string, Map<string, { source: PlanSource; items: PlanItem[] }>>();
    for (const c of columns) byCol.set(c, new Map());
    for (const src of sources) {
      for (const it of src.items) {
        const col = byCol.get(it.state || "");
        if (!col) continue;
        let g = col.get(src.definition);
        if (!g) {
          g = { source: src, items: [] };
          col.set(src.definition, g);
        }
        g.items.push(it);
      }
    }
    return columns.map((c) => {
      const groups = [...byCol.get(c)!.entries()]
        .map(([definition, g]) => ({
          definition,
          source: g.source,
          items: g.items,
          blocked: g.items.filter((it) => isBlocked(it) && !isTerminal(g.source.plan, it)).length,
          late: g.items.filter((it) => isLate(it, now)).length,
        }))
        .sort((a, b) => b.items.length - a.items.length || a.definition.localeCompare(b.definition));
      return { state: c, total: groups.reduce((n, g) => n + g.items.length, 0), groups };
    });
  }, [sources, columns, now]);

  const busiest = Math.max(1, ...grid.map((c) => c.total));

  return (
    <section className="mc-panel-card">
      <header className="mc-panel-card-head">
        <h3>Every plan, one board</h3>
      </header>
      <p className="mc-plan-note">
        {forced
          ? "These plans' declared column orders disagree, so this is a forced merge: every distinct column shows, " +
            "ordered by where it sits on average in the flows that declare it. The order is a reading, not a fact."
          : "Columns are the union of every plan's declared columns — they merge cleanly because the library shares " +
            "one flow vocabulary."}{" "}
        Items are grouped by the report they come from; a count expands to the cards.
      </p>
      <div className="mc-plan-board">
        <div className="mc-plan-columns" style={{ gridTemplateColumns: `repeat(${grid.length}, minmax(12rem, 1fr))` }}>
          {grid.map((col) => (
            <section
              className="mc-plan-column"
              key={col.state}
              aria-label={humanLabel(col.state) + " — " + col.total + " items across " + col.groups.length + " reports"}
            >
              <header className="mc-plan-column-head">
                <h4 className="mc-plan-column-title">{humanLabel(col.state)}</h4>
                <p className="mc-plan-column-count">
                  <strong>{col.total}</strong>
                  <span className="mc-plan-limit">
                    {" "}
                    in {col.groups.length} {col.groups.length === 1 ? "report" : "reports"}
                  </span>
                </p>
                <div className="mc-plan-fill" aria-hidden="true">
                  <span style={{ width: `${Math.round((col.total / busiest) * 100)}%` }} />
                </div>
              </header>
              <ul className="mc-agg-groups" role="list">
                {col.groups.map((g) => {
                  const key = col.state + "//" + g.definition;
                  const open = openGroups.has(key);
                  return (
                    <li className={"mc-agg-group" + (open ? " is-open" : "")} key={key}>
                      <button
                        type="button"
                        className="mc-agg-group-head"
                        aria-expanded={open}
                        onClick={() => toggleGroup(key)}
                      >
                        <span className="mc-agg-group-name">{g.definition}</span>
                        <span className="mc-agg-group-count mc-mono">{g.items.length}</span>
                        {g.blocked > 0 && <span className="mc-agg-group-flag is-blocked">{g.blocked} blocked</span>}
                        {g.late > 0 && <span className="mc-agg-group-flag is-late">{g.late} late</span>}
                        <span className="mc-agg-group-caret" aria-hidden="true">
                          {open ? "▾" : "▸"}
                        </span>
                      </button>
                      {open && (
                        <ul className="mc-plan-cards" role="list">
                          {g.items.map((it) => (
                            <AggCard
                              key={(it.scope || g.source.scope) + "/" + g.definition + "/" + it.id}
                              item={it}
                              source={g.source}
                              now={now}
                              route={route}
                            />
                          ))}
                        </ul>
                      )}
                    </li>
                  );
                })}
                {!col.groups.length && <li className="mc-agg-empty mc-body-muted">nothing here</li>}
              </ul>
            </section>
          ))}
        </div>
      </div>
    </section>
  );
}

function AggCard({ item, source, now, route }: { item: PlanItem; source: PlanSource; now: number; route: Route }) {
  const blocked = isBlocked(item) && !isTerminal(source.plan, item);
  const late = daysLate(item, now);
  const overdue = !Number.isNaN(late) && late > 0;
  const unit = source.plan.size_unit ? ` ${source.plan.size_unit}` : "";
  const scope = item.scope || source.scope;
  return (
    <li className={"mc-plan-card" + (blocked ? " is-blocked" : "") + (overdue ? " is-late" : "")}>
      <p className="mc-plan-card-title">{item.title || item.id}</p>
      <p className="mc-plan-card-meta">
        <Link
          className="mc-plan-origin"
          to={reportingRoute(route, { report: source.definition })}
          title={"From " + source.definition + " at " + scope}
        >
          {source.definition}
        </Link>
        {item.owner && <span className="mc-plan-owner">{item.owner}</span>}
        {item.size ? (
          <span className="mc-plan-size">
            {item.size}
            {unit}
          </span>
        ) : null}
        {scope && <span className="mc-plan-scope">{scope.split("/").pop()}</span>}
      </p>
      {(blocked || overdue) && (
        <p className="mc-plan-card-flags">
          {blocked && (
            <span className="mc-plan-flag is-blocked" title={`blocked by ${(item.blocked_by || []).join(", ")}`}>
              blocked
            </span>
          )}
          {overdue && <span className="mc-plan-flag is-late">{Math.round(late)}d late</span>}
        </p>
      )}
    </li>
  );
}
