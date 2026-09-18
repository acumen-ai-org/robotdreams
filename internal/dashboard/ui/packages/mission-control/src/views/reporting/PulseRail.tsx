import { relTime, sevClass } from "../../lib/format";
import { underScope } from "../../lib/routes";
import type { PulseItem } from "../../lib/types";

const PULSE_CAP = 60;

export type PulseMode = "live" | "polling" | "offline";

export function PulseRail({ items, scope, mode }: { items: PulseItem[]; scope: string; mode: PulseMode }) {
  const visible = items.filter((p) => underScope(p.scope, scope)).slice(0, PULSE_CAP);
  const pillCls = mode === "live" ? "mc-pill-live" : "mc-pill-muted";

  return (
    <aside className="mc-pulse-rail mc-panel" aria-labelledby="pulse-heading">
      <div className="mc-panel-header">
        <h2 id="pulse-heading">Pulse</h2>
        <span className={"mc-pill " + pillCls} title="Live report stream status">
          <span className="mc-dot" aria-hidden="true"></span>
          <span>{mode}</span>
        </span>
      </div>
      <ul className="mc-pulse-list">
        {!visible.length && <li className="mc-empty-state">No report activity at this scope yet.</li>}
        {visible.map((p, i) => (
          <li key={i} className={"mc-pulse-item" + (p.kind === "instance" ? " mc-pulse-instance" : "")}>
            <span className={"mc-sev-dot " + sevClass(p.severity)} aria-hidden="true"></span>
            <span className="mc-pulse-body">
              <span className="mc-pulse-label">
                <span className="mc-sr-only">{(p.severity || "info") + ": "}</span>
                {p.label}
              </span>
              <span className="mc-pulse-tag">{p.definition + " · " + (p.scope || "root")}</span>
            </span>
            <span className="mc-pulse-time">{relTime(p.t)}</span>
          </li>
        ))}
      </ul>
    </aside>
  );
}
