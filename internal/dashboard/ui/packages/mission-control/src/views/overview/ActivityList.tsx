import { fmtTime } from "../../lib/format";
import { mcVariant } from "../../components/shared";
import type { ActivityEntry } from "../../lib/types";

export function ActivityList({ entries }: { entries: ActivityEntry[] }) {
  return (
    <section className="mc-panel mc-panel-activity" aria-labelledby="activity-heading">
      <div className="mc-panel-header">
        <h2 id="activity-heading">Live activity</h2>
        <span className="mc-label-muted">{entries.length ? entries.length + " recent" : ""}</span>
      </div>
      <ul className="mc-activity-list">
        {!entries.length && <li className="mc-empty-state">No activity yet.</li>}
        {entries.map((a) => (
          <li key={a.id} className="mc-activity-item">
            <div className="mc-activity-top">
              <span className={"mc-activity-type " + mcVariant(a.typeClass)}>{a.label}</span>
              <span className="mc-activity-time">{fmtTime(a.at)}</span>
            </div>
            <div>{a.detail}</div>
          </li>
        ))}
      </ul>
    </section>
  );
}
