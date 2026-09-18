import { useMemo } from "react";
import { PageToolbox } from "../components/PageToolbox";
import { ViewSelector } from "../components/ViewSelector";
import { Link } from "../components/Link";
import { useConnection } from "../state/ConnectionContext";
import { useRoute } from "../state/RouterContext";
import { useShell } from "../state/ShellContext";
import { useSchedules, type Schedule } from "../hooks/useSchedules";
import { aboutVariant } from "../lib/variants";
import { crossViewRoute } from "../lib/routes";
import { fmtTime, relTime } from "../lib/format";
import { DelayedLoading, Skeleton } from "../components/Async";

interface Props {
  active?: boolean;
  workerId?: string;
}

function untilText(iso: string | undefined | null): string {
  if (!iso) return "";
  const t = new Date(iso).getTime();
  if (!isFinite(t)) return "";
  const s = Math.round((t - Date.now()) / 1000);
  if (s <= 0) return "due";
  if (s < 60) return "in " + s + "s";
  const m = Math.round(s / 60);
  if (m < 60) return "in " + m + "m";
  const h = Math.round(m / 60);
  if (h < 48) return "in " + h + "h";
  return "in " + Math.round(h / 24) + "d";
}

const SOON_WINDOW_MS = 3600_000;

function isSoon(s: Schedule): boolean {
  if (!s.enabled || !s.next_at) return false;
  const t = new Date(s.next_at).getTime();
  return isFinite(t) && t - Date.now() < SOON_WINDOW_MS;
}

interface Group {
  to: string;
  rows: Schedule[];
}

function groupByWorker(schedules: Schedule[]): Group[] {
  const by = new Map<string, Schedule[]>();
  for (const s of schedules) {
    const k = s.to || "";
    if (!by.has(k)) by.set(k, []);
    by.get(k)!.push(s);
  }
  return [...by.entries()]
    .sort(([a], [b]) => a.localeCompare(b))
    .map(([to, rows]) => ({ to, rows: rows.slice().sort((a, b) => a.id.localeCompare(b.id)) }));
}

export function SchedulesView({ active = true, workerId }: Props) {
  const route = useRoute();
  const conn = useConnection();
  const { variant: variantOf, selectVariant } = useShell();
  const variant = variantOf("schedules");
  const { schedules, pending, error, refresh } = useSchedules(workerId, active);
  const groups = useMemo(() => groupByWorker(schedules), [schedules]);

  return (
    <main id="view-schedules" className="mc-layout mc-layout-scroll mc-schedules-layout" hidden={!active}>
      <PageToolbox active={active} title="Schedules" subtitle="Schedules" info={aboutVariant("schedules", variant)}>
        <ViewSelector route={route} view="schedules" active={variant} onSelect={selectVariant} />
      </PageToolbox>

      <div className="mc-schedules-main" id={active ? "main-content" : undefined} tabIndex={-1}>
        {!conn.hasToken ? (
          <section aria-label="Schedules">
            <p className="mc-empty-state">Connect to a server to see schedules.</p>
          </section>
        ) : error ? (
          <section className="mc-panel" aria-label="Schedules">
            <p className="mc-form-error">{error}</p>
            <p>
              <button type="button" className="mc-button" onClick={refresh}>
                Try again
              </button>
            </p>
          </section>
        ) : pending && !schedules.length ? (
          <DelayedLoading>
            <Skeleton lines={5} />
          </DelayedLoading>
        ) : !groups.length ? (
          <section aria-label="Schedules">
            <p className="mc-empty-state">No schedules</p>
          </section>
        ) : (
          groups.map((g) => <WorkerSchedules key={g.to} group={g} />)
        )}
      </div>
    </main>
  );
}

function WorkerSchedules({ group }: { group: Group }) {
  const route = useRoute();
  const headingId = "schedules-" + (group.to || "unrouted").replace(/[^A-Za-z0-9_-]/g, "_");
  const n = group.rows.length;
  return (
    <section className="mc-panel mc-schedules-group" aria-labelledby={headingId}>
      <div className="mc-panel-header">
        <h2 id={headingId} className="mc-schedules-worker">
          {group.to ? (
            <Link to={{ ...crossViewRoute(route, "overview"), selected: { kind: "node", id: group.to } }}>
              {group.to}
            </Link>
          ) : (
            <span className="mc-label-muted">unrouted</span>
          )}
        </h2>
        <span className="mc-label-muted">
          {n} {n === 1 ? "schedule" : "schedules"}
        </span>
      </div>
      <div className="mc-storage-table-wrap">
        <table className="mc-storage-table mc-schedules-table">
          <thead>
            <tr>
              <th scope="col">Id</th>
              <th scope="col">Subject</th>
              <th scope="col">Cron</th>
              <th scope="col">Next fire</th>
              <th scope="col">Owner</th>
              <th scope="col">Last fired</th>
              <th scope="col">Enabled</th>
            </tr>
          </thead>
          <tbody>
            {group.rows.map((s) => (
              <tr key={s.id} className={(s.enabled ? "" : "is-off") + (isSoon(s) ? " is-soon" : "")}>
                <td className="mc-schedules-id" title={s.body || undefined}>
                  {s.id}
                </td>
                <td className="mc-schedules-subject" title={s.subject}>
                  {s.subject || <span className="mc-label-muted">—</span>}
                </td>
                <td className="mc-schedules-cron">
                  <code>{s.cron}</code>
                </td>
                <td className="mc-schedules-next">
                  {s.enabled && s.next_at ? (
                    <>
                      <span className="mc-schedules-rel">{untilText(s.next_at)}</span>
                      <span className="mc-schedules-abs">{fmtTime(s.next_at)}</span>
                    </>
                  ) : (
                    <span className="mc-label-muted">—</span>
                  )}
                </td>
                <td className="mc-schedules-owner">{s.owner || <span className="mc-label-muted">—</span>}</td>
                <td className="mc-schedules-last">
                  {s.last_at ? (
                    <>
                      <span className="mc-schedules-rel">{relTime(s.last_at)}</span>
                      <span className="mc-schedules-abs">{fmtTime(s.last_at)}</span>
                    </>
                  ) : (
                    <span className="mc-label-muted">never</span>
                  )}
                </td>
                <td>
                  <span className={"mc-schedules-enabled " + (s.enabled ? "is-on" : "is-off")}>
                    {s.enabled ? "on" : "off"}
                  </span>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </section>
  );
}
