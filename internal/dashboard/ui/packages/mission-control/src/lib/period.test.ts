import { describe, expect, it } from "vitest";
import { LIVE, PERIOD_LABELS, periodCaption, periodQuery, stepAt } from "./period";

describe("periodQuery", () => {
  it("is empty for Live", () => {
    expect(periodQuery(LIVE)).toBe("");
    expect(periodQuery({ kind: "", at: "2026-01-01T00:00:00Z" })).toBe("");
  });

  it("starts with & and encodes the instant", () => {
    expect(periodQuery({ kind: "week", at: "" })).toBe("&period=week");
    expect(periodQuery({ kind: "month", at: "2026-01-01T00:00:00+01:00" })).toBe(
      "&period=month&at=2026-01-01T00%3A00%3A00%2B01%3A00",
    );
  });
});

describe("stepAt", () => {
  const at = "2026-01-31T12:00:00.000Z";

  it("moves by one unit in each direction", () => {
    const day = new Date(stepAt("day", at, 1));
    expect(day.getTime() - Date.parse(at)).toBe(24 * 3600_000);
    const week = new Date(stepAt("week", at, -1));
    expect(Date.parse(at) - week.getTime()).toBe(7 * 24 * 3600_000);
    const year = new Date(stepAt("year", at, 1));
    expect(year.getFullYear()).toBe(2027);
  });

  it("steps months and quarters on the calendar, landing in the next month", () => {
    const m = new Date(stepAt("month", "2026-01-15T12:00:00.000Z", 1));
    expect(m.getMonth() - new Date("2026-01-15T12:00:00.000Z").getMonth()).toBe(1);
    const q = new Date(stepAt("quarter", "2026-01-15T12:00:00.000Z", 1));
    expect(q.getMonth() - new Date("2026-01-15T12:00:00.000Z").getMonth()).toBe(3);
  });

  it("falls back to now when the instant is missing or unparseable", () => {
    const before = Date.now();
    const fromEmpty = Date.parse(stepAt("day", "", 1));
    expect(fromEmpty).toBeGreaterThanOrEqual(before + 24 * 3600_000 - 1000);
    const fromJunk = Date.parse(stepAt("day", "not a date", 1));
    expect(fromJunk).toBeGreaterThanOrEqual(before - 1000);
    expect(fromJunk).toBeLessThanOrEqual(Date.now() + 1000);
  });
});

describe("periodCaption", () => {
  it("names the kind and the span; a single day reads as one date", () => {
    const week = periodCaption({ kind: "week", start: "2025-09-08T00:00:00", end: "2025-09-15T00:00:00" });
    expect(week.startsWith(PERIOD_LABELS.week + " · ")).toBe(true);
    expect(week).toContain(" – ");
    const day = periodCaption({ kind: "day", start: "2025-09-08T00:00:00", end: "2025-09-09T00:00:00" });
    expect(day.startsWith(PERIOD_LABELS.day + " · ")).toBe(true);
    expect(day).not.toContain(" – ");
  });

  it("keeps an unknown kind's name and survives bad dates", () => {
    expect(periodCaption({ kind: "fortnight", start: "x", end: "y" })).toBe("fortnight");
    const c = periodCaption({ kind: "sprint", start: "2025-09-08T00:00:00Z", end: "2025-09-22T00:00:00Z" });
    expect(c.startsWith("sprint · ")).toBe(true);
  });
});
