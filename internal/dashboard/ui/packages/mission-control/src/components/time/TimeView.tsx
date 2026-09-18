import { Suspense, lazy } from "react";
import { Tabs } from "../Tabs";
import { EntryList } from "./EntryList";
import { useTimeMode } from "../../lib/display";
import type { TimeEntry } from "../../lib/time";

const TimeStory = lazy(() => import("./TimeStory"));

const toAxisItem = (e: TimeEntry, i: number) => ({
  id: String(i) + ":" + (e.t || ""),
  t: e.t,
  headline: e.label,
  severity: e.severity,
  scope: e.scope,
});

interface Props {
  entries: TimeEntry[];
  timelineCap?: number;
  empty?: string;
  anchorPrefix?: string;
}

export function TimeModeSwitch({ count }: { count?: number }) {
  const [mode, setMode] = useTimeMode();
  return (
    <span className="mc-timeline-modes">
      <Tabs
        tabs={[
          { id: "list", label: "list" },
          { id: "timeline", label: "timeline" },
        ]}
        active={mode}
        onSelect={(id) => setMode(id as typeof mode)}
        label="How to draw these entries"
        variant="switch"
      />
      {count != null && count > 0 && <span className="mc-label-muted">{count} entries</span>}
    </span>
  );
}

export function TimeView({ entries, timelineCap = 500, empty, anchorPrefix }: Props) {
  const [mode] = useTimeMode();

  if (mode === "list") return <EntryList entries={entries} empty={empty} anchorPrefix={anchorPrefix} />;
  return (
    <Suspense fallback={<p className="mc-empty-state">Loading timeline…</p>}>
      <TimeStory direction="y" items={entries.slice(0, timelineCap).map(toAxisItem)} empty={empty} />
    </Suspense>
  );
}
