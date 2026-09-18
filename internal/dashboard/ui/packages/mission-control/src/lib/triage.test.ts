import { describe, expect, it } from "vitest";
import {
  bandTiles,
  normalisedVariance,
  seriesMovement,
  severityKey,
  severityRank,
  statusCounts,
  worstVariance,
} from "./triage";
import type { SummaryTile } from "./types";

describe("severityRank / severityKey", () => {
  it("ranks worst first and files the unknown as no data, not ok", () => {
    expect(severityRank("critical")).toBe(0);
    expect(severityRank("warn")).toBe(1);
    expect(severityRank("ok")).toBe(2);
    expect(severityRank(undefined)).toBe(3);
    expect(severityRank("fine")).toBe(3);
    expect(severityKey("critical")).toBe("critical");
    expect(severityKey("ok")).toBe("ok");
    expect(severityKey("")).toBe("none");
  });
});

describe("normalisedVariance", () => {
  it("is the signed fraction over target, positive in the bad direction", () => {
    expect(normalisedVariance({ value: 400, target: 300 })).toBeCloseTo(1 / 3);
    expect(normalisedVariance({ value: 400, target: 300, direction: "above" })).toBeCloseTo(1 / 3);
    expect(normalisedVariance({ value: 200, target: 300, direction: "below" })).toBeCloseTo(1 / 3);
    expect(normalisedVariance({ value: 400, target: 300, direction: "below" })).toBeCloseTo(-1 / 3);
  });

  it("is zero without a usable target", () => {
    expect(normalisedVariance({ value: 5 })).toBe(0);
    expect(normalisedVariance({ value: 5, target: 0 })).toBe(0);
    expect(normalisedVariance({ target: 5 })).toBe(0);
  });

  it("worstVariance takes the largest over a tile, never below zero", () => {
    expect(
      worstVariance({
        kpis: [
          { value: 1, target: 2 },
          { value: 9, target: 3 },
        ],
      }),
    ).toBe(2);
    expect(worstVariance({ kpis: [{ value: 1, target: 2 }] })).toBe(0);
    expect(worstVariance({})).toBe(0);
  });
});

describe("seriesMovement", () => {
  it("is first to last, with no percentage from zero", () => {
    expect(seriesMovement([10, 12, 15])).toEqual({ from: 10, to: 15, abs: 5, pct: 0.5 });
    expect(seriesMovement([0, 5])).toEqual({ from: 0, to: 5, abs: 5, pct: undefined });
    expect(seriesMovement([-4, -2])).toEqual({ from: -4, to: -2, abs: 2, pct: 0.5 });
  });

  it("needs two finite points", () => {
    expect(seriesMovement([])).toBeUndefined();
    expect(seriesMovement([1])).toBeUndefined();
    expect(seriesMovement([NaN, 1])).toBeUndefined();
    expect(seriesMovement([NaN, 1, 3])).toEqual({ from: 1, to: 3, abs: 2, pct: 2 });
  });
});

describe("bandTiles", () => {
  const tiles: SummaryTile[] = [
    { name: "zebra", status: "ok" },
    { name: "apple", status: "ok" },
    { name: "mild", status: "warn", kpis: [{ value: 11, target: 10 }] },
    { name: "wild", status: "warn", kpis: [{ value: 20, target: 10 }] },
    { name: "boom", status: "critical" },
    { name: "mute" },
    { definition: "def-only", status: "bogus" },
  ];

  it("puts attention first, then steady, then quiet, worst first within each", () => {
    const bands = bandTiles(tiles);
    expect(bands.map((b) => b.id)).toEqual(["attention", "steady", "quiet"]);
    expect(bands[0].tiles.map((t) => t.name)).toEqual(["boom", "wild", "mild"]);
    expect(bands[1].tiles.map((t) => t.name)).toEqual(["apple", "zebra"]);
    expect(bands[2].tiles.map((t) => t.definition || t.name)).toEqual(["def-only", "mute"]);
  });

  it("returns all three bands even when empty", () => {
    const bands = bandTiles([]);
    expect(bands).toHaveLength(3);
    expect(bands.every((b) => b.tiles.length === 0)).toBe(true);
  });

  it("statusCounts agrees with the bands", () => {
    expect(statusCounts(tiles)).toEqual({ critical: 1, warn: 2, ok: 2, none: 2 });
    expect(statusCounts([])).toEqual({ critical: 0, warn: 0, ok: 0, none: 0 });
  });
});
