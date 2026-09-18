import type { ReportResponse } from "./types";
import { humanDuration } from "./format";

export interface TimeEntry {
  t?: string;
  severity?: string;
  label: string;
  scope?: string;
  tags?: string[];
}

export function sortEntries(entries: TimeEntry[]): TimeEntry[] {
  return entries.sort((a, b) => String(b.t || "").localeCompare(String(a.t || "")));
}

export function reportEntries(data: ReportResponse): TimeEntry[] {
  const def = data.definition || {};
  const milestoneTypes = new Set(def.facets?.timeline?.milestones || []);
  const tl = data.timeline || {};
  const entries: TimeEntry[] = [];

  for (const ev of tl.events || []) {
    const tags: string[] = [];
    if (ev.type && milestoneTypes.has(ev.type)) tags.push("milestone");
    entries.push({
      t: ev.t,
      severity: ev.severity,
      label: ev.label || ev.type || "event",
      scope: ev.scope || "",
      tags,
    });
  }

  for (const sp of tl.spans || []) {
    const dur = sp.end ? new Date(sp.end).getTime() - new Date(sp.start).getTime() : NaN;
    entries.push({
      t: sp.start,
      severity: "info",
      label:
        (sp.name || "span") +
        (sp.label ? " — " + sp.label : "") +
        (isFinite(dur) ? " (" + humanDuration(dur) + ")" : " (ongoing)"),
      scope: sp.scope || "",
      tags: ["span"],
    });
  }

  return sortEntries(entries);
}
