export type WindowUnit = "h" | "d" | "w" | "mo";

export interface TimeWindow {
  n: number;
  unit: WindowUnit;
}

export const WINDOW_UNITS: WindowUnit[] = ["h", "d", "w", "mo"];

export const WINDOW_DEFAULT_N = 1;

export const DEFAULT_WINDOW: TimeWindow = { n: 1, unit: "w" };

const UNIT_ONE: Record<WindowUnit, string> = { h: "hour", d: "day", w: "week", mo: "month" };
const UNIT_MANY: Record<WindowUnit, string> = { h: "hours", d: "days", w: "weeks", mo: "months" };

const HOUR = 3600_000;

const MAX_N = 999;

export function clampWindow(w: TimeWindow): TimeWindow {
  const n = Math.max(1, Math.min(MAX_N, Math.round(w.n) || 1));
  return { n, unit: WINDOW_UNITS.includes(w.unit) ? w.unit : DEFAULT_WINDOW.unit };
}

export function formatWindow(w: TimeWindow): string {
  const c = clampWindow(w);
  return c.n + c.unit;
}

export function parseWindow(raw: string | null | undefined): TimeWindow {
  const m = /^(\d{1,3})(h|d|w|mo)$/.exec((raw || "").trim());
  if (!m) return DEFAULT_WINDOW;
  return clampWindow({ n: Number(m[1]), unit: m[2] as WindowUnit });
}

export function windowSince(w: TimeWindow, now: number = Date.now()): number {
  const c = clampWindow(w);
  if (c.unit === "mo") {
    const d = new Date(now);
    d.setMonth(d.getMonth() - c.n);
    return d.getTime();
  }
  const per = c.unit === "h" ? HOUR : c.unit === "d" ? 24 * HOUR : 7 * 24 * HOUR;
  return now - c.n * per;
}

export function windowMs(w: TimeWindow, now: number = Date.now()): number {
  return now - windowSince(w, now);
}

export function windowLabel(w: TimeWindow): string {
  const c = clampWindow(w);
  return c.n === 1 ? "last " + UNIT_ONE[c.unit] : "last " + c.n + " " + UNIT_MANY[c.unit];
}

export function windowChip(w: TimeWindow): string {
  return formatWindow(w);
}

export function inWindow(t: string | number | undefined, w: TimeWindow, now: number = Date.now()): boolean {
  if (t === undefined || t === "") return false;
  const ms = typeof t === "number" ? t : Date.parse(t);
  if (!Number.isFinite(ms)) return false;
  return ms >= windowSince(w, now) && ms <= now + HOUR;
}
