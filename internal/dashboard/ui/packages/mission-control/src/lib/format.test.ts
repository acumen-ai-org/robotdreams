import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cellText, fmtNum, fmtTime, humanDuration, pointValues, relTime, sevClass, unitLabel } from "./format";

describe("cellText", () => {
  it("shows primitives as themselves and nothing for null or undefined", () => {
    expect(cellText("x")).toBe("x");
    expect(cellText(3)).toBe("3");
    expect(cellText(true)).toBe("true");
    expect(cellText(null)).toBe("");
    expect(cellText(undefined)).toBe("");
  });

  it("shows structured values as JSON rather than [object Object]", () => {
    expect(cellText({ a: 1 })).toBe('{"a":1}');
    expect(cellText([1, "b"])).toBe('[1,"b"]');
  });
});

describe("fmtNum", () => {
  it("formats finite numbers to at most two decimals", () => {
    expect(fmtNum(1234.5678)).toBe(new Intl.NumberFormat(undefined, { maximumFractionDigits: 2 }).format(1234.5678));
    expect(fmtNum(2)).toBe("2");
  });

  it("uses a dash for nothing and the text of anything else", () => {
    expect(fmtNum(null)).toBe("—");
    expect(fmtNum(undefined)).toBe("—");
    expect(fmtNum("n/a")).toBe("n/a");
    expect(fmtNum(Infinity)).toBe("Infinity");
    expect(fmtNum(NaN)).toBe("NaN");
  });
});

describe("fmtTime", () => {
  it("is empty for nothing and echoes an unparseable string", () => {
    expect(fmtTime(undefined)).toBe("");
    expect(fmtTime("")).toBe("");
    expect(fmtTime("yesterday-ish")).toBe("yesterday-ish");
  });
});

describe("relTime", () => {
  beforeEach(() => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date("2026-06-01T12:00:00Z"));
  });
  afterEach(() => vi.useRealTimers());

  const ago = (ms: number) => new Date(Date.now() - ms).toISOString();

  it("scales from seconds to days", () => {
    expect(relTime(ago(0))).toBe("just now");
    expect(relTime(ago(30_000))).toBe("30s ago");
    expect(relTime(ago(5 * 60_000))).toBe("5m ago");
    expect(relTime(ago(3 * 3600_000))).toBe("3h ago");
    expect(relTime(ago(3 * 24 * 3600_000))).toBe("3d ago");
    expect(relTime("garbage")).toBe("");
  });
});

describe("humanDuration", () => {
  it("picks the unit that fits", () => {
    expect(humanDuration(5_000)).toBe("5s");
    expect(humanDuration(120_000)).toBe("2m");
    expect(humanDuration(2 * 3600_000)).toBe("2h");
    expect(humanDuration(72 * 3600_000)).toBe("3d");
    expect(humanDuration(-1)).toBe("");
    expect(humanDuration(NaN)).toBe("");
  });
});

describe("unitLabel and sevClass", () => {
  it("abbreviates the known units and passes others through", () => {
    expect(unitLabel(undefined)).toBe("");
    expect(unitLabel("count")).toBe("");
    expect(unitLabel("minutes")).toBe("min");
    expect(unitLabel("percent")).toBe("%");
    expect(unitLabel("per-hour")).toBe("/h");
    expect(unitLabel("widgets")).toBe("widgets");
  });

  it("maps severities to the shared classes with a muted fallback", () => {
    expect(sevClass("critical")).toBe("mc-sev-critical");
    expect(sevClass("error")).toBe("mc-sev-critical");
    expect(sevClass("warning")).toBe("mc-sev-warn");
    expect(sevClass("info")).toBe("mc-sev-info");
    expect(sevClass(undefined)).toBe("mc-sev-muted");
    expect(sevClass("whatever")).toBe("mc-sev-muted");
  });
});

describe("pointValues", () => {
  it("accepts numbers and value/v/y objects, dropping the rest", () => {
    expect(pointValues([1, { value: 2 }, { v: 3 }, { y: 4 }, { x: 5 }, "6", null, NaN, Infinity])).toEqual([
      1, 2, 3, 4,
    ]);
    expect(pointValues(undefined)).toEqual([]);
  });
});
