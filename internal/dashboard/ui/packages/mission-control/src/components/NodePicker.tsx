import { useMemo } from "react";
import { scopeOf, type WorkerIndex } from "../hooks/useWorkers";
import { nodeEmoji } from "../lib/vocabulary";

interface Props {
  index: WorkerIndex;
  roles: string[];
  onToggleRole: (role: string) => void;
}

export function NodePicker({ index, roles, onToggleRole }: Props) {
  const inScope = useMemo(() => index.workers.filter((w) => index.matches.has(w.id)), [index]);

  const roleCounts = useMemo(() => {
    const counts = new Map<string, number>();
    for (const w of inScope) {
      const r = w.role || "—";
      counts.set(r, (counts.get(r) || 0) + 1);
    }
    return Array.from(counts.entries()).sort((a, b) => b[1] - a[1] || a[0].localeCompare(b[0]));
  }, [inScope]);

  const shown = useMemo(() => {
    const wanted = roles.length ? new Set(roles) : null;
    return inScope.filter((w) => !wanted || wanted.has(w.role || "—")).slice(0, 200);
  }, [inScope, roles]);

  return (
    <div className="mc-node-picker">
      <div className="mc-scope-col-head">
        <span className="mc-scope-col-title">Nodes</span>
        <span className="mc-label-muted">
          {shown.length === inScope.length ? inScope.length + " in scope" : shown.length + " of " + inScope.length}
        </span>
      </div>

      <div className="mc-node-roles" role="group" aria-label="Filter by role">
        {roleCounts.map(([r, n]) => {
          const on = roles.includes(r);
          return (
            <button
              key={r}
              type="button"
              className={"mc-node-role" + (on ? " is-on" : "")}
              aria-pressed={on}
              onClick={() => onToggleRole(r)}
            >
              <span aria-hidden="true">{nodeEmoji(r)}</span>
              {r}
              <span className="mc-node-role-count">{n}</span>
            </button>
          );
        })}
      </div>

      <ul className="mc-node-list">
        {shown.map((w) => (
          <li key={w.id} className="mc-node-entry" title={scopeOf(w)}>
            <span className="mc-node-entry-emoji" aria-hidden="true">
              {nodeEmoji(w.role)}
            </span>
            <span className="mc-node-entry-id">{w.id}</span>
            <span className="mc-node-entry-role mc-label-muted">{w.role || "—"}</span>
          </li>
        ))}
        {!shown.length && <li className="mc-empty-state">No nodes match.</li>}
      </ul>
      {inScope.length > shown.length && shown.length === 200 && (
        <p className="mc-label-muted">Showing the first 200 — narrow by role or id.</p>
      )}
    </div>
  );
}
