import { useEffect, useState } from "react";
import { useConnection } from "../state/ConnectionContext";
import type { StorageObject } from "../lib/types";

export interface StorageState {
  objects: StorageObject[];
  error: string;
  loading: boolean;
}

export function useStorage(version: number): StorageState {
  const conn = useConnection();
  const [state, setState] = useState<StorageState>({ objects: [], error: "", loading: true });

  useEffect(() => {
    if (!conn.hasToken) {
      setState({ objects: [], error: "", loading: false });
      return;
    }
    let stale = false;
    conn
      .apiJSON<{ objects?: StorageObject[] }>("/api/storage/objects?prefix=")
      .then((data) => {
        if (stale) return;
        const objects = (data.objects || [])
          .slice()
          .sort((a, b) => ((a.updated_at || "") < (b.updated_at || "") ? 1 : -1));
        setState({ objects, error: "", loading: false });
      })
      .catch((e) => {
        if (!stale) {
          setState({
            objects: [],
            error: "Could not load storage: " + (e instanceof Error ? e.message : String(e)),
            loading: false,
          });
        }
      });
    return () => {
      stale = true;
    };
  }, [conn, conn.hasToken, conn.session, version]);

  return state;
}

export function narrowStorage(
  objects: StorageObject[],
  opts: { updatedBy?: string; underScope?: string; scopeOfWorker?: (id: string) => string },
): StorageObject[] {
  if (opts.updatedBy) return objects.filter((o) => o.updated_by === opts.updatedBy);
  if (opts.underScope !== undefined && opts.scopeOfWorker) {
    const under = opts.underScope;
    return objects.filter((o) => {
      const sc = o.updated_by ? opts.scopeOfWorker!(o.updated_by) : "";
      return !!sc && (under === "" || sc === under || sc.startsWith(under + "/"));
    });
  }
  return objects;
}
