import { useEffect, useMemo, useState } from "react";
import { useConnection } from "../state/ConnectionContext";
import { type NodeApp, appsByWorker } from "../lib/apps";

export interface AppsState {
  byWorker: Map<string, NodeApp>;
  error: string;
}

const EMPTY: Map<string, NodeApp> = new Map();

export function useApps(version: number, active: boolean): AppsState {
  const conn = useConnection();
  const [apps, setApps] = useState<NodeApp[]>([]);
  const [error, setError] = useState("");

  useEffect(() => {
    if (!active || !conn.hasToken) return;
    let stale = false;
    conn
      .apiJSON<{ apps?: NodeApp[] }>("/api/apps")
      .then((d) => {
        if (stale) return;
        setApps(d.apps || []);
        setError("");
      })
      .catch((e) => {
        if (stale) return;
        setError("Could not load apps: " + (e instanceof Error ? e.message : String(e)));
      });
    return () => {
      stale = true;
    };
  }, [conn, conn.hasToken, conn.session, version, active]);

  const byWorker = useMemo(() => (apps.length ? appsByWorker(apps) : EMPTY), [apps]);
  return { byWorker, error };
}
