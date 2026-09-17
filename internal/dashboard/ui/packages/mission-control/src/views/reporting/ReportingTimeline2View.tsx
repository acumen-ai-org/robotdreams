import { useMemo } from "react";
import { TimeStory } from "../../components/time/TimeStory";
import type { TimeAxisItem } from "../../components/time/TimeAxis";
import type { ReportEvent } from "../../lib/types";

interface Props {
  events: ReportEvent[];
}

export default function ReportingTimeline2View({ events: feed }: Props) {
  const items = useMemo<TimeAxisItem[]>(
    () =>
      feed
        .slice()
        .reverse()
        .map((ev, i) => ({
          id: (ev.t || "") + ":" + (ev.type || "") + ":" + i,
          t: ev.t,
          headline: ev.label || ev.type || "event",
          severity: ev.severity,
          type: ev.type,
          scope: ev.scope,
        })),
    [feed],
  );

  return (
    <section className="mc-panel mc-panel-fill mc-tline" aria-label="Merged timeline, running across">
      <TimeStory items={items} empty="No timeline events at this scope yet." />
    </section>
  );
}
