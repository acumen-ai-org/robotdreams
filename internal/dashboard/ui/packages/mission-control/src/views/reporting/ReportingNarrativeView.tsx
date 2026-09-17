import { Suspense, lazy, type ReactNode, useEffect, useMemo, useState } from "react";
import { useConnection } from "../../state/ConnectionContext";
import { reportingRoute, type Route } from "../../lib/routes";
import { StatusBadge, tileName } from "../../components/shared";
import { Link } from "../../components/Link";
import type { ReportEvent, SummaryTile } from "../../lib/types";
import { Tabs, TabPanel, type TabDef } from "../../components/Tabs";

const AskView = lazy(() => import("./ReportingAskView"));

interface Props {
  route: Route;
  tiles: SummaryTile[];
  refreshTick: number;
  active: boolean;
  periodQuery?: string;
}

const RANK: Record<string, number> = { critical: 0, warn: 1, ok: 2, unknown: 3 };
const EVENT_CAP = 40;
const LIST_CAP = 6;

function severityOf(t: SummaryTile): string {
  return (t.status || "unknown").toLowerCase();
}

function CappedList<T>({
  items,
  renderItem,
  keyOf,
}: {
  items: T[];
  renderItem: (item: T) => ReactNode;
  keyOf: (item: T) => string;
}) {
  const [expanded, setExpanded] = useState(false);
  const shown = expanded ? items : items.slice(0, LIST_CAP);
  const hidden = items.length - LIST_CAP;
  return (
    <>
      <ul>
        {shown.map((item) => (
          <li key={keyOf(item)}>{renderItem(item)}</li>
        ))}
      </ul>
      {hidden > 0 && (
        <button
          type="button"
          className="mc-button mc-button-tertiary"
          aria-expanded={expanded}
          onClick={() => setExpanded((e) => !e)}
        >
          {expanded ? "Show less" : "Show " + hidden + " more"}
        </button>
      )}
    </>
  );
}

function tally(tiles: SummaryTile[]): string {
  const counts = new Map<string, number>();
  for (const t of tiles) counts.set(severityOf(t), (counts.get(severityOf(t)) || 0) + 1);
  const parts: string[] = [];
  for (const [k, label] of [
    ["critical", "critical"],
    ["warn", "needing attention"],
    ["ok", "healthy"],
  ] as const) {
    const n = counts.get(k);
    if (n) parts.push(n + " " + label);
  }
  return parts.join(", ") || "nothing reporting";
}

export default function ReportingNarrativeView({ route, tiles, refreshTick, active, periodQuery = "" }: Props) {
  const [tab, setTab] = useState("tldr");
  const conn = useConnection();
  const [events, setEvents] = useState<ReportEvent[]>([]);

  useEffect(() => {
    if (!active || !conn.hasToken) return;
    let stale = false;
    const scopeQ = "scope=" + encodeURIComponent(route.scope) + periodQuery;
    conn
      .apiJSON<{ events?: ReportEvent[] }>("/api/reports/timeline?" + scopeQ)
      .then((d) => {
        if (!stale) setEvents((d.events || []).slice(0, EVENT_CAP));
      })
      .catch(() => {
        if (!stale) setEvents([]);
      });
    return () => {
      stale = true;
    };
  }, [conn, conn.hasToken, conn.session, route.scope, refreshTick, active, periodQuery]);

  const sorted = useMemo(
    () =>
      [...tiles].sort(
        (a, b) => (RANK[severityOf(a)] ?? 9) - (RANK[severityOf(b)] ?? 9) || tileName(a).localeCompare(tileName(b)),
      ),
    [tiles],
  );
  const attention = sorted.filter((t) => severityOf(t) === "critical" || severityOf(t) === "warn");
  const notable = useMemo(
    () =>
      events
        .filter((e) => (e.severity || "").toLowerCase() === "critical" || (e.severity || "").toLowerCase() === "warn")
        .slice(0, 8),
    [events],
  );
  const where = route.scopes.length ? route.scopes.join(", ") : "everything";

  const elsewhere = sorted.filter((t) => !attention.includes(t));

  const tabs: TabDef[] = [
    { id: "tldr", label: "TL;DR", hint: "The whole answer, in a sentence" },
    {
      id: "attention",
      label: "What needs attention",
      badge: attention.length || undefined,
      hint: "The reports asking for a decision",
    },
    { id: "else", label: "Everything else", badge: elsewhere.length || undefined, hint: "The steady ones" },
    { id: "recently", label: "Recently", badge: notable.length || undefined, hint: "Notable events that landed" },
    { id: "ask", label: "Ask", hint: "Grounded questions over these same reports" },
  ];
  const at = tabs.some((t) => t.id === tab) ? tab : "tldr";

  return (
    <div className="mc-narrative">
      <p className="mc-proposed-note">
        <strong>Proposed.</strong> Sentences here are templated from the reports, not written by a model — the
        contract's stated fallback when no model is configured. What a model would change is the wording, not the data.
      </p>

      <Tabs tabs={tabs} active={at} onSelect={setTab} label="Briefing sections" />
      <TabPanel id={at}>
        {at === "tldr" && (
          <section className="mc-narrative-lede">
            <p>
              Across <strong>{where}</strong>, {tiles.length} {tiles.length === 1 ? "report" : "reports"}:{" "}
              {tally(tiles)}.
              {attention.length === 0
                ? " Nothing is asking for attention right now."
                : " " +
                  attention.length +
                  (attention.length === 1 ? " report wants" : " reports want") +
                  " attention" +
                  (notable.length ? ", and " + notable.length + " notable events landed recently." : ".")}
            </p>
          </section>
        )}

        {at === "attention" && (
          <section className="mc-narrative-block">
            {attention.length === 0 ? (
              <p className="mc-empty-state">Nothing is asking for attention at this scope.</p>
            ) : (
              <CappedList
                items={attention}
                keyOf={(t) => (t.scope || "") + tileName(t)}
                renderItem={(t) => (
                  <>
                    <StatusBadge status={t.status} />{" "}
                    <Link to={reportingRoute(route, { report: tileName(t), ...(t.scope ? { scope: t.scope } : {}) })}>
                      {tileName(t)}
                    </Link>
                    {t.scope && <span className="mc-body-muted mc-mono"> · {t.scope}</span>}
                    {t.headline && <> — {t.headline}</>}
                  </>
                )}
              />
            )}
          </section>
        )}

        {at === "else" && (
          <section className="mc-narrative-block">
            {elsewhere.length === 0 ? (
              <p className="mc-empty-state">Nothing else is reporting at this scope.</p>
            ) : (
              <CappedList
                items={elsewhere}
                keyOf={(t) => (t.scope || "") + tileName(t)}
                renderItem={(t) => (
                  <>
                    <Link to={reportingRoute(route, { report: tileName(t), ...(t.scope ? { scope: t.scope } : {}) })}>
                      {tileName(t)}
                    </Link>
                    {t.headline && <> — {t.headline}</>}
                  </>
                )}
              />
            )}
          </section>
        )}

        {at === "recently" && (
          <section className="mc-narrative-block">
            {notable.length === 0 ? (
              <p className="mc-empty-state">Nothing notable has landed recently.</p>
            ) : (
              <ul>
                {notable.map((e, i) => (
                  <li key={i}>
                    <span className={"mc-sev mc-sev-" + (e.severity || "info").toLowerCase()}>
                      {e.severity || "info"}
                    </span>{" "}
                    {e.label || e.type}
                    {e.scope && <span className="mc-body-muted mc-mono"> · {e.scope}</span>}
                  </li>
                ))}
              </ul>
            )}
          </section>
        )}

        {at === "ask" && (
          <Suspense fallback={<p className="mc-empty-state">Loading…</p>}>
            <AskView route={route} tiles={tiles} />
          </Suspense>
        )}
      </TabPanel>
    </div>
  );
}
