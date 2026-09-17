import { useEffect, useState } from "react";
import { useConnection } from "../state/ConnectionContext";
import type { ScopeEntry } from "../lib/types";
import { formatWindow, windowSince, type TimeWindow } from "../lib/window";

export function useScopes(active: boolean, win?: TimeWindow): ScopeEntry[] {
  const conn = useConnection();
  const [scopes, setScopes] = useState<ScopeEntry[]>([]);
  const winKey = win ? formatWindow(win) : "";

  useEffect(() => {
    if (!active || !conn.hasToken) return;
    let stale = false;
    const q = win ? "?since=" + encodeURIComponent(new Date(windowSince(win)).toISOString()) : "";
    conn
      .apiJSON<{ scopes?: ScopeEntry[] }>("/api/reports/scopes" + q)
      .then((d) => {
        if (!stale) setScopes(d.scopes || []);
      })
      .catch(() => {
        if (!stale) setScopes([]);
      });
    return () => {
      stale = true;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [conn, conn.hasToken, conn.session, active, winKey]);

  return scopes;
}
