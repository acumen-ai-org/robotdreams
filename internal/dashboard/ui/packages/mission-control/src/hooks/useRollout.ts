import { useEffect, useMemo, useState } from "react";
import { ApiError, useConnection } from "../state/ConnectionContext";
import { type Announcement, type Rollout, type RolloutNode, latestByKind } from "../lib/updates";

export interface RolloutState {
  kinds: string[];
  latest: Map<string, Announcement>;
  rollout: Rollout | null;
  updateOf: Map<string, RolloutNode>;
  adminOnly: boolean;
  error: string;
}

const EMPTY: Map<string, RolloutNode> = new Map();

export function useRollout(kind: string, version: number, active: boolean): RolloutState {
  const conn = useConnection();
  const [announcements, setAnnouncements] = useState<Announcement[]>([]);
  const [rollout, setRollout] = useState<Rollout | null>(null);
  const [adminOnly, setAdminOnly] = useState(false);
  const [error, setError] = useState("");

  useEffect(() => {
    if (!active || !conn.hasToken) return;
    let stale = false;
    conn
      .apiJSON<{ announcements?: Announcement[] }>("/api/updates")
      .then((d) => {
        if (stale) return;
        setAnnouncements(d.announcements || []);
        setError("");
      })
      .catch((e) => {
        if (stale) return;
        setError("Could not load updates: " + (e instanceof Error ? e.message : String(e)));
      });
    return () => {
      stale = true;
    };
  }, [conn, conn.hasToken, conn.session, version, active]);

  useEffect(() => {
    if (!active || !conn.hasToken || !kind) {
      setRollout(null);
      return;
    }
    let stale = false;
    conn
      .apiJSON<Rollout>("/api/updates/rollout?kind=" + encodeURIComponent(kind))
      .then((d) => {
        if (stale) return;
        setRollout(d);
        setAdminOnly(false);
        setError("");
      })
      .catch((e) => {
        if (stale) return;
        if (e instanceof ApiError && e.status === 403) {
          setRollout(null);
          setAdminOnly(true);
          return;
        }
        setError("Could not load the rollout: " + (e instanceof Error ? e.message : String(e)));
      });
    return () => {
      stale = true;
    };
  }, [conn, conn.hasToken, conn.session, version, active, kind]);

  const latest = useMemo(() => latestByKind(announcements), [announcements]);
  const kinds = useMemo(() => Array.from(latest.keys()), [latest]);

  const updateOf = useMemo(() => {
    if (!rollout) return EMPTY;
    const m = new Map<string, RolloutNode>();
    for (const n of rollout.nodes) m.set(n.worker_id, n);
    return m;
  }, [rollout]);

  return { kinds, latest, rollout, updateOf, adminOnly, error };
}
