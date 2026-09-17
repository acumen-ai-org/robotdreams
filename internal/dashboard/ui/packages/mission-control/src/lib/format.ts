const numFmt = new Intl.NumberFormat(undefined, { maximumFractionDigits: 2 });

export function cellText(v: unknown): string {
  switch (typeof v) {
    case "string":
      return v;
    case "undefined":
      return "";
    case "object":
      return v === null ? "" : JSON.stringify(v);
    default:
      return String(v);
  }
}

export function fmtNum(v: unknown): string {
  if (typeof v !== "number" || !isFinite(v)) return v == null ? "—" : cellText(v);
  return numFmt.format(v);
}

export function fmtTime(iso: string | undefined | null): string {
  if (!iso) return "";
  const d = new Date(iso);
  return isNaN(d.getTime()) ? String(iso) : d.toLocaleString();
}

export function pickTimeFormat(isos: Array<string | undefined>): (iso: string | undefined) => string {
  const times = isos
    .map((s) => (s ? new Date(s).getTime() : NaN))
    .filter((t) => isFinite(t))
    .sort((a, b) => a - b);
  if (!times.length) return fmtTime;

  const first = new Date(times[0]);
  const last = new Date(times[times.length - 1]);
  const sameDay =
    first.getFullYear() === last.getFullYear() &&
    first.getMonth() === last.getMonth() &&
    first.getDate() === last.getDate();
  const sameYear = first.getFullYear() === last.getFullYear();

  const opts: Intl.DateTimeFormatOptions = sameDay
    ? { hour: "2-digit", minute: "2-digit", second: "2-digit" }
    : sameYear
      ? { month: "short", day: "numeric", hour: "2-digit", minute: "2-digit" }
      : { year: "numeric", month: "short", day: "numeric", hour: "2-digit", minute: "2-digit" };

  const fmt = new Intl.DateTimeFormat(undefined, opts);
  return (iso) => {
    if (!iso) return "";
    const d = new Date(iso);
    return isNaN(d.getTime()) ? String(iso) : fmt.format(d);
  };
}

export function dayLabel(isos: Array<string | undefined>): string {
  const times = isos.map((s) => (s ? new Date(s).getTime() : NaN)).filter((t) => isFinite(t));
  if (!times.length) return "";
  const first = new Date(Math.min(...times));
  const last = new Date(Math.max(...times));
  const sameDay =
    first.getFullYear() === last.getFullYear() &&
    first.getMonth() === last.getMonth() &&
    first.getDate() === last.getDate();
  if (!sameDay) return "";
  const fmt = new Intl.DateTimeFormat(undefined, { weekday: "short", year: "numeric", month: "short", day: "numeric" });
  return fmt.format(first);
}

export function relTime(iso: string | undefined): string {
  const t = new Date(iso || Date.now()).getTime();
  if (!isFinite(t)) return "";
  const s = Math.max(0, Math.round((Date.now() - t) / 1000));
  if (s < 10) return "just now";
  if (s < 60) return s + "s ago";
  const m = Math.round(s / 60);
  if (m < 60) return m + "m ago";
  const h = Math.round(m / 60);
  if (h < 48) return h + "h ago";
  return Math.round(h / 24) + "d ago";
}

export function humanDuration(ms: number): string {
  if (!isFinite(ms) || ms < 0) return "";
  const s = Math.round(ms / 1000);
  if (s < 90) return s + "s";
  const m = Math.round(s / 60);
  if (m < 90) return m + "m";
  const h = Math.round(m / 60);
  if (h < 48) return h + "h";
  return Math.round(h / 24) + "d";
}

export function unitLabel(unit: string | undefined): string {
  if (!unit || unit === "count") return "";
  if (unit === "minutes") return "min";
  if (unit === "seconds") return "s";
  if (unit === "per-hour") return "/h";
  if (unit === "per-minute") return "/min";
  if (unit === "percent") return "%";
  return unit;
}

export function sevClass(sev: string | undefined): string {
  if (sev === "critical" || sev === "error") return "mc-sev-critical";
  if (sev === "warn" || sev === "warning") return "mc-sev-warn";
  if (sev === "info") return "mc-sev-info";
  return "mc-sev-muted";
}

export function pointValues(points: unknown[] | undefined): number[] {
  return (points || [])
    .map((p) => {
      if (typeof p === "number") return p;
      if (p && typeof p === "object") {
        const o = p as Record<string, unknown>;
        if (typeof o.value === "number") return o.value;
        if (typeof o.v === "number") return o.v;
        if (typeof o.y === "number") return o.y;
      }
      return NaN;
    })
    .filter((v) => typeof v === "number" && isFinite(v));
}
