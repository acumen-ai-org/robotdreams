import { Fragment, useMemo } from "react";
import { SevPill } from "../shared";
import { dayLabel, pickTimeFormat } from "../../lib/format";
import type { TimeEntry } from "../../lib/time";

interface Props {
  entries: TimeEntry[];
  empty?: string;
  anchorPrefix?: string;
}

export function dayAnchor(prefix: string, iso: string | undefined): string {
  const d = iso ? new Date(iso) : null;
  if (!d || isNaN(d.getTime())) return prefix + "unknown";
  return prefix + d.getFullYear() + "-" + (d.getMonth() + 1) + "-" + d.getDate();
}

export function EntryList({ entries, empty = "Nothing here yet.", anchorPrefix }: Props) {
  const fmt = useMemo(() => pickTimeFormat(entries.map((e) => e.t)), [entries]);
  const caption = useMemo(() => dayLabel(entries.map((e) => e.t)), [entries]);

  if (!entries.length) {
    return (
      <ul className="mc-ticker-list">
        <li className="mc-empty-state">{empty}</li>
      </ul>
    );
  }

  return (
    <ul className="mc-ticker-list">
      {caption && (
        <li className="mc-timeline-daylabel mc-label-muted" aria-hidden="true">
          {caption}
        </li>
      )}
      {entries.map((e, i) => {
        const dayTurns = !caption && (i === 0 || dayAnchor("", e.t) !== dayAnchor("", entries[i - 1].t));
        return (
          <Fragment key={i}>
            {dayTurns && (
              <li className="mc-ticker-day mc-label-muted" id={anchorPrefix ? dayAnchor(anchorPrefix, e.t) : undefined}>
                {e.t
                  ? new Date(e.t).toLocaleDateString(undefined, { weekday: "short", month: "short", day: "numeric" })
                  : "undated"}
              </li>
            )}
            <li className="mc-ticker-item">
              <span className="mc-ticker-time">{fmt(e.t)}</span>
              <SevPill sev={e.severity} />
              <span className="mc-ticker-label">{e.label}</span>
              <span className="mc-ticker-extras">
                {(e.tags || []).map((tag) => (
                  <span className={"mc-tag" + (tag === "milestone" ? " mc-tag-milestone" : "")} key={tag}>
                    {tag}
                  </span>
                ))}
                {e.scope && <span className="mc-label-muted mc-ticker-scope">{e.scope}</span>}
              </span>
            </li>
          </Fragment>
        );
      })}
    </ul>
  );
}
