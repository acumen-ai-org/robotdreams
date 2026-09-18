import { describe, expect, it } from "vitest";
import { rollupOf, rollupTitle } from "./aggregation";

describe("rollupOf", () => {
  it("is null when the summary says nothing about contributors", () => {
    expect(rollupOf({}, "spookify")).toBeNull();
    expect(rollupOf({ scopes: [], instances: 0 }, "spookify")).toBeNull();
  });

  it("reports a bare count without guessing at levels", () => {
    expect(rollupOf({ instances: 4 }, "spookify")).toEqual({ leaves: 4, levels: 0, byLevel: [], direct: 0, paths: [] });
    expect(rollupOf({ instances: [1, 2, 3] }, "spookify")?.leaves).toBe(3);
  });

  it("counts contributors by depth below the scope being read", () => {
    const r = rollupOf(
      { scopes: ["spookify/music/platform/deploy", "spookify/music/platform", "spookify/music", "spookify/music/ads"] },
      "spookify/music",
    );
    expect(r).not.toBeNull();
    expect(r!.leaves).toBe(4);
    expect(r!.direct).toBe(1);
    expect(r!.levels).toBe(2);
    expect(r!.byLevel).toEqual([
      { rel: 0, count: 1, name: "world" },
      { rel: 1, count: 2, name: "realm" },
      { rel: 2, count: 1, name: "site" },
    ]);
    expect(r!.paths).toEqual([
      "spookify/music",
      "spookify/music/ads",
      "spookify/music/platform",
      "spookify/music/platform/deploy",
    ]);
  });

  it("prefers the tile's own scope, ignores non-string entries, and names deep rungs neutrally", () => {
    const r = rollupOf({ scope: "spookify", scopes: ["spookify/a/b/c/d", 7, null] }, "ignored/scope");
    expect(r!.leaves).toBe(1);
    expect(r!.byLevel).toEqual([{ rel: 4, count: 1, name: "scope" }]);
  });
});

describe("rollupTitle", () => {
  it("says how many and how far down", () => {
    expect(rollupTitle({ leaves: 1, levels: 0, byLevel: [], direct: 0, paths: [] })).toBe("Based on 1 report.");
    expect(
      rollupTitle({ leaves: 3, levels: 0, byLevel: [{ rel: 0, count: 3, name: "site" }], direct: 3, paths: [] }),
    ).toBe("Based on 3 reports, written at this scope — nothing was rolled up.");
    const r = rollupOf({ scopes: ["spookify/music", "spookify/music/a", "spookify/music/b/c"] }, "spookify/music")!;
    expect(rollupTitle(r)).toBe(
      "Based on 3 reports, rolled up through 2 levels of aggregation — 1 realm down, 1 site down, plus 1 written here.",
    );
  });
});
