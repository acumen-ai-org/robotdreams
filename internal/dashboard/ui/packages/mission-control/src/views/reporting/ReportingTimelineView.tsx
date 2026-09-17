import { Suspense, lazy, useEffect, useMemo, useRef, useState } from "react";
import { useConnection } from "../../state/ConnectionContext";
import { type Route } from "../../lib/routes";
import { useTimeMode, useTimeOrientation } from "../../lib/display";
import { ToolboxRenderingToggle } from "../../components/RenderingToggle";
import { ColumnsIcon, ListIcon, RowsIcon } from "../../components/icons";
import type { ReportEvent } from "../../lib/types";
import { TimeView } from "../../components/time/TimeView";
import { dayAnchor } from "../../components/time/EntryList";
import { sortEntries, type TimeEntry } from "../../lib/time";

const DAY = "sb-day-";

const Timeline2View = lazy(() => import("./ReportingTimeline2View"));

const TIMELINE_CAP = 200;

interface Props {
  route: Route;
  refreshTick: number;
  pollTick: number;
  active: boolean;
  periodQuery?: string;
}

export default function ReportingTimelineView({ route, refreshTick, pollTick, active, periodQuery = "" }: Props) {
  const conn = useConnection();
  const [direction, setDirection] = useTimeOrientation();
  const [mode, setMode] = useTimeMode();
  const bodyRef = useRef<HTMLDivElement>(null);
  const drawing: Drawing = direction === "horizontal" ? "across" : mode === "timeline" ? "down" : "list";
  const setDrawing = (d: Drawing) => {
    if (d === "across") return setDirection("horizontal");
    setDirection("vertical");
    setMode(d === "down" ? "timeline" : "list");
  };
  const [events, setEvents] = useState<ReportEvent[]>([]);
  const [error, setError] = useState("");

  useEffect(() => {
    if (!active || !conn.hasToken) return;
    let stale = false;
    const scopeQ = "scope=" + encodeURIComponent(route.scope) + periodQuery;
    const urls = route.cats.length
      ? route.cats.map((c) => "/api/reports/timeline?" + scopeQ + "&category=" + encodeURIComponent(c))
      : ["/api/reports/timeline?" + scopeQ];
    Promise.all(urls.map((u) => conn.apiJSON<{ events?: ReportEvent[] }>(u)))
      .then((results) => {
        if (stale) return;
        const seen = new Set<string>();
        const merged: ReportEvent[] = [];
        for (const r of results) {
          for (const ev of r.events || []) {
            const key = [ev.t, ev.type, ev.scope, ev.label].join("|");
            if (seen.has(key)) continue;
            seen.add(key);
            merged.push(ev);
          }
        }
        merged.sort((a, b) => String(b.t || "").localeCompare(String(a.t || "")));
        setEvents(merged.slice(0, TIMELINE_CAP));
        setError("");
      })
      .catch((e) => {
        if (!stale) setError("Could not load timeline: " + (e instanceof Error ? e.message : String(e)));
      });
    return () => {
      stale = true;
    };
  }, [active, conn, conn.hasToken, conn.session, route.scope, route.cats, refreshTick, pollTick, periodQuery]);

  const entries = useMemo<TimeEntry[]>(
    () =>
      sortEntries(
        events.map((ev) => ({
          t: ev.t,
          severity: ev.severity,
          label: ev.label || ev.type || "event",
          scope: ev.scope || "",
          tags: ev.severity === "critical" ? ["milestone"] : [],
        })),
      ),
    [events],
  );

  const days = useMemo(() => {
    const seen = new Map<string, string>();
    for (const e of entries) {
      const d = e.t ? new Date(e.t) : null;
      if (!d || isNaN(d.getTime())) continue;
      const key = dayAnchor("", e.t);
      if (!seen.has(key))
        seen.set(key, d.toLocaleDateString(undefined, { weekday: "short", month: "short", day: "numeric" }));
    }
    return [...seen].map(([key, label]) => ({ key, label }));
  }, [entries]);

  return (
    <div className="mc-reporting-main mc-storyboard">
      <ToolboxDrawing active={active} drawing={drawing} onChange={setDrawing} />
      {error ? (
        <section className="mc-panel mc-report-panel" aria-label="Merged timeline">
          <p className="mc-form-error">{error}</p>
        </section>
      ) : direction === "horizontal" ? (
        <Suspense fallback={<p className="mc-empty-state">Loading storyboard…</p>}>
          <Timeline2View events={events} />
        </Suspense>
      ) : (
        <section className="mc-panel mc-panel-fill mc-tdown" aria-label="Merged timeline">
          {drawing === "list" && days.length > 1 && (
            <nav className="mc-tdown-days" aria-label="Jump to a day">
              {days.map((d) => (
                <button
                  key={d.key}
                  className="mc-tdown-day"
                  type="button"
                  onClick={() =>
                    bodyRef.current
                      ?.querySelector("#" + DAY + d.key)
                      ?.scrollIntoView({ behavior: "smooth", block: "start" })
                  }
                >
                  {d.label}
                </button>
              ))}
            </nav>
          )}
          <div className="mc-tdown-body" ref={bodyRef}>
            <TimeView entries={entries} empty="No timeline events at this scope yet." anchorPrefix={DAY} />
          </div>
        </section>
      )}
    </div>
  );
}

type Drawing = "list" | "down" | "across";

function ToolboxDrawing({
  active,
  drawing,
  onChange,
}: {
  active: boolean;
  drawing: Drawing;
  onChange: (d: Drawing) => void;
}) {
  return (
    <ToolboxRenderingToggle
      active={active}
      label="Storyboard"
      value={drawing}
      onChange={onChange}
      options={[
        { id: "list", label: "A dense list, newest first", icon: <ListIcon size={15} /> },
        { id: "down", label: "A time axis running down the page", icon: <RowsIcon size={15} /> },
        { id: "across", label: "A time axis running across it", icon: <ColumnsIcon size={15} /> },
      ]}
    />
  );
}
