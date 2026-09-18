import { describe, expect, it } from "vitest";
import {
  DEFAULT_WINDOW,
  clampWindow,
  formatWindow,
  inWindow,
  parseWindow,
  windowLabel,
  windowMs,
  windowSince,
} from "./window";

const HOUR = 3600_000;

describe("parseWindow / formatWindow", () => {
  it("round-trips every unit", () => {
    for (const text of ["1h", "12h", "3d", "1w", "6mo", "999d"]) {
      expect(formatWindow(parseWindow(text))).toBe(text);
    }
  });

  it("falls back to the default for anything malformed", () => {
    for (const raw of [null, undefined, "", "0w", "w", "1y", "1000d", "1 w", "-1d", "1.5d"]) {
      expect(parseWindow(raw)).toEqual(DEFAULT_WINDOW);
    }
  });

  it("tolerates surrounding whitespace", () => {
    expect(parseWindow("  2d ")).toEqual({ n: 2, unit: "d" });
  });
});

describe("clampWindow", () => {
  it("keeps n within 1..999 and the unit known", () => {
    expect(clampWindow({ n: 0, unit: "d" })).toEqual({ n: 1, unit: "d" });
    expect(clampWindow({ n: 5000, unit: "h" })).toEqual({ n: 999, unit: "h" });
    expect(clampWindow({ n: 2.4, unit: "w" })).toEqual({ n: 2, unit: "w" });
    expect(clampWindow({ n: NaN, unit: "w" })).toEqual({ n: 1, unit: "w" });
    expect(clampWindow({ n: 3, unit: "years" as never })).toEqual({ n: 3, unit: DEFAULT_WINDOW.unit });
  });
});

describe("windowSince", () => {
  const now = Date.UTC(2026, 2, 31, 12, 0, 0);

  it("is arithmetic for hours, days and weeks", () => {
    expect(windowSince({ n: 2, unit: "h" }, now)).toBe(now - 2 * HOUR);
    expect(windowSince({ n: 3, unit: "d" }, now)).toBe(now - 3 * 24 * HOUR);
    expect(windowSince({ n: 1, unit: "w" }, now)).toBe(now - 7 * 24 * HOUR);
    expect(windowMs({ n: 1, unit: "w" }, now)).toBe(7 * 24 * HOUR);
  });

  it("walks the calendar for months", () => {
    const since = new Date(windowSince({ n: 1, unit: "mo" }, now));
    const from = new Date(now);
    expect(since.getTime()).toBeLessThan(now);
    expect(since.getTime()).toBeGreaterThan(now - 32 * 24 * HOUR);
    expect(since.getFullYear()).toBe(from.getFullYear());
  });
});

describe("windowLabel", () => {
  it("reads as a sentence, singular and plural", () => {
    expect(windowLabel({ n: 1, unit: "w" })).toBe("last week");
    expect(windowLabel({ n: 3, unit: "d" })).toBe("last 3 days");
    expect(windowLabel({ n: 1, unit: "mo" })).toBe("last month");
    expect(windowLabel({ n: 12, unit: "h" })).toBe("last 12 hours");
  });
});

describe("inWindow", () => {
  const now = Date.UTC(2026, 0, 10);
  const w = { n: 1, unit: "d" as const };

  it("accepts instants inside the window and a little clock skew ahead", () => {
    expect(inWindow(now - HOUR, w, now)).toBe(true);
    expect(inWindow(new Date(now - 23 * HOUR).toISOString(), w, now)).toBe(true);
    expect(inWindow(now + 30 * 60_000, w, now)).toBe(true);
  });

  it("rejects instants before the window, far ahead, or unparseable", () => {
    expect(inWindow(now - 25 * HOUR, w, now)).toBe(false);
    expect(inWindow(now + 2 * HOUR, w, now)).toBe(false);
    expect(inWindow(undefined, w, now)).toBe(false);
    expect(inWindow("", w, now)).toBe(false);
    expect(inWindow("not a date", w, now)).toBe(false);
  });
});
