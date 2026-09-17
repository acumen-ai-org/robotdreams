import { useCallback, useEffect, useState } from "react";
import { useConnection, ApiError } from "../state/ConnectionContext";
import { useLive } from "../state/LiveContext";

const POLL_MS = 15000;

export interface Schedule {
  id: string;
  owner: string;
  to: string;
  cron: string;
  subject: string;
  body: string;
  enabled: boolean;
  next_at: string;
  last_at: string | null;
  created_at: string;
}

export interface SchedulesState {
  schedules: Schedule[];
  pending: boolean;
  error: string;
  refresh: () => void;
}

export function useSchedules(workerId?: string, active = true): SchedulesState {
  const conn = useConnection();
  const { stream, scheduleVersion } = useLive();
  const [schedules, setSchedules] = useState<Schedule[]>([]);
  const [error, setError] = useState("");
  const [pending, setPending] = useState(() => active && conn.hasToken);
  const [tick, setTick] = useState(0);
  const refresh = useCallback(() => setTick((n) => n + 1), []);

  useEffect(() => {
    if (!active || !conn.hasToken) {
      setPending(false);
      return;
    }
    let stale = false;
    setPending(true);
    conn
      .apiJSON<{ schedules?: Schedule[] }>(
        "/api/schedules" + (workerId ? "?worker_id=" + encodeURIComponent(workerId) : ""),
      )
      .then((data) => {
        if (stale) return;
        setSchedules(data.schedules || []);
        setError("");
      })
      .catch((e) => {
        if (stale) return;
        if (e instanceof ApiError && e.status === 401) {
          conn.logout();
          return;
        }
        setError("Could not load schedules: " + (e instanceof Error ? e.message : String(e)));
      })
      .finally(() => {
        if (!stale) setPending(false);
      });
    return () => {
      stale = true;
    };
  }, [conn, conn.hasToken, conn.session, workerId, active, scheduleVersion, tick]);

  useEffect(() => {
    if (!(active && stream !== "live" && conn.hasToken)) return;
    const t = setInterval(refresh, POLL_MS);
    return () => clearInterval(t);
  }, [active, stream, conn.hasToken, refresh]);

  return { schedules, pending, error, refresh };
}
