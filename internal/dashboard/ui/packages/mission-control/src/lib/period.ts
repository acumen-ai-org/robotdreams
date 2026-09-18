import { useCallback, useState } from "react";
import { readStored, writeStored } from "./storage";

export type PeriodKind = "day" | "week" | "month" | "quarter" | "year";

export const PERIOD_KINDS: readonly PeriodKind[] = ["day", "week", "month", "quarter", "year"];

export const PERIOD_LABELS: Record<PeriodKind, string> = {
  day: "Day",
  week: "Week",
  month: "Month",
  quarter: "Quarter",
  year: "Year",
};

export interface PeriodSelection {
  kind: PeriodKind | "";
  at: string;
}

export const LIVE: PeriodSelection = { kind: "", at: "" };

const KEY = "reportingPeriod";

function isKind(v: string): v is PeriodKind {
  return (PERIOD_KINDS as readonly string[]).includes(v);
}

function readPeriod(): PeriodSelection {
  const raw = readStored(KEY);
  if (!raw) return LIVE;
  try {
    const p = JSON.parse(raw) as Partial<PeriodSelection>;
    if (!p || typeof p.kind !== "string" || !isKind(p.kind)) return LIVE;
    const at = typeof p.at === "string" && isFinite(new Date(p.at).getTime()) ? p.at : "";
    return { kind: p.kind, at };
  } catch {
    return LIVE;
  }
}

export function periodQuery(p: PeriodSelection): string {
  if (!p.kind) return "";
  return "&period=" + p.kind + (p.at ? "&at=" + encodeURIComponent(p.at) : "");
}

export function stepAt(kind: PeriodKind, at: string, dir: 1 | -1): string {
  const d = at ? new Date(at) : new Date();
  if (!isFinite(d.getTime())) return new Date().toISOString();
  switch (kind) {
    case "day":
      d.setDate(d.getDate() + dir);
      break;
    case "week":
      d.setDate(d.getDate() + 7 * dir);
      break;
    case "month":
      d.setMonth(d.getMonth() + dir);
      break;
    case "quarter":
      d.setMonth(d.getMonth() + 3 * dir);
      break;
    case "year":
      d.setFullYear(d.getFullYear() + dir);
      break;
  }
  return d.toISOString();
}

export interface PeriodApi {
  period: PeriodSelection;
  setKind: (kind: PeriodKind | "") => void;
  step: (dir: 1 | -1) => void;
  reset: () => void;
}

export function usePeriod(): PeriodApi {
  const [period, setPeriod] = useState<PeriodSelection>(readPeriod);
  const update = useCallback((next: PeriodSelection) => {
    setPeriod(next);
    writeStored(KEY, JSON.stringify(next));
  }, []);
  const setKind = useCallback(
    (kind: PeriodKind | "") => update(kind ? { kind, at: period.at || new Date().toISOString() } : LIVE),
    [update, period.at],
  );
  const step = useCallback(
    (dir: 1 | -1) => {
      if (!period.kind) return;
      update({ kind: period.kind, at: stepAt(period.kind, period.at, dir) });
    },
    [update, period],
  );
  const reset = useCallback(() => {
    if (!period.kind) return;
    update({ kind: period.kind, at: new Date().toISOString() });
  }, [update, period.kind]);
  return { period, setKind, step, reset };
}

export function periodCaption(p: { kind: string; start: string; end: string }): string {
  const start = new Date(p.start);
  const end = new Date(new Date(p.end).getTime() - 1);
  if (!isFinite(start.getTime()) || !isFinite(end.getTime())) return p.kind;
  const label = isKind(p.kind) ? PERIOD_LABELS[p.kind] : p.kind;
  const sameYear = start.getFullYear() === end.getFullYear();
  const sameDay = sameYear && start.getMonth() === end.getMonth() && start.getDate() === end.getDate();
  const full: Intl.DateTimeFormatOptions = { weekday: "short", day: "numeric", month: "short", year: "numeric" };
  const short: Intl.DateTimeFormatOptions = { weekday: "short", day: "numeric", month: "short" };
  if (sameDay) return label + " · " + start.toLocaleDateString(undefined, full);
  return (
    label +
    " · " +
    start.toLocaleDateString(undefined, sameYear ? short : full) +
    " – " +
    end.toLocaleDateString(undefined, full)
  );
}
