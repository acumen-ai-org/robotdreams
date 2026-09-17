import { useEffect, useMemo, useState } from "react";
import { useStorage } from "../../hooks/useStorage";
import { fmtTime } from "../../lib/format";
import { underScope } from "../../lib/routes";
import type { StorageObject } from "../../lib/types";
import { scopeOf, type WorkerIndex } from "../../hooks/useWorkers";

const PAGE_SIZE = 25;

interface Props {
  version: number;
  updatedBy?: string;
  underScope?: string;
  index?: WorkerIndex;
  objects?: StorageObject[];
}

export function StorageTable({ version, updatedBy, underScope: scopeFilter, index, objects: given }: Props) {
  const own = useStorage(given ? -1 : version);
  const objects = given ?? own.objects;
  const error = given ? "" : own.error;
  const [shown, setShown] = useState(PAGE_SIZE);
  useEffect(() => {
    setShown(PAGE_SIZE);
  }, [objects]);

  const filtered = useMemo(() => {
    if (updatedBy) return objects.filter((o) => o.updated_by === updatedBy);
    if (scopeFilter !== undefined && index) {
      return objects.filter((o) => {
        const w = o.updated_by ? index.byId.get(o.updated_by) : undefined;
        const sc = w ? scopeOf(w) : "";
        return !!sc && underScope(sc, scopeFilter);
      });
    }
    return objects;
  }, [objects, updatedBy, scopeFilter, index]);

  const visible = filtered.slice(0, shown);

  return (
    <section className="mc-panel mc-panel-storage" aria-labelledby="storage-heading">
      <div className="mc-panel-header">
        <h2 id="storage-heading">Storage browser</h2>
        <span className="mc-label-muted">
          {filtered.length ? Math.min(shown, filtered.length) + " / " + filtered.length + " objects" : ""}
        </span>
      </div>
      <div className="mc-storage-table-wrap">
        <table className="mc-storage-table">
          <thead>
            <tr>
              <th scope="col">Path</th>
              <th scope="col">Revision</th>
              <th scope="col">Updated by</th>
              <th scope="col">Updated at</th>
            </tr>
          </thead>
          <tbody>
            {error ? (
              <tr>
                <td colSpan={4} className="mc-empty-state">
                  {error}
                </td>
              </tr>
            ) : !visible.length ? (
              <tr>
                <td colSpan={4} className="mc-empty-state">
                  No objects yet.
                </td>
              </tr>
            ) : (
              visible.map((o) => (
                <tr key={o.path}>
                  <td className="mc-mono" title={o.path}>
                    {o.path}
                  </td>
                  <td className="mc-mono">{o.revision}</td>
                  <td>{o.updated_by || "—"}</td>
                  <td className="mc-mono">{fmtTime(o.updated_at)}</td>
                </tr>
              ))
            )}
          </tbody>
        </table>
      </div>
      {shown < filtered.length && (
        <div className="mc-storage-pager">
          <button
            className="mc-button mc-button-secondary"
            type="button"
            onClick={() => setShown((s) => s + PAGE_SIZE)}
          >
            Load more
          </button>
        </div>
      )}
    </section>
  );
}
