import { EntryList } from "../components/time/EntryList";
import { sortEntries, type TimeEntry } from "../lib/time";
import type { Panel } from "../lib/types";

export function Ticker({ panel }: { panel: Panel }) {
  const entries: TimeEntry[] = sortEntries(
    (panel.events || []).map((ev) => ({
      t: ev.t,
      severity: ev.severity,
      label: ev.label || ev.type || "event",
      scope: ev.scope || "",
    })),
  );
  return <EntryList entries={entries} empty="No events." />;
}
