import { describe, expect, it } from "vitest";
import { MAX_PLACE_DEPTH, placeRows, reportRows, rowInSelection, type Row } from "./orgRows";
import type { ScopeEntry, Worker } from "./types";

const worker = (id: string, scope: string, extra: Partial<Worker> = {}): Worker => ({
  id,
  status: "connected",
  metadata: { scope },
  ...extra,
});

const labels = (rows: Row[] | undefined) => (rows || []).map((r) => r.label);

describe("placeRows", () => {
  const scopes: ScopeEntry[] = [
    { path: "spookify/music", recent: 2 },
    { path: "spookify/music/platform", recent: 3 },
    { path: "spookify/ads" },
  ];
  const workers: Worker[] = [
    worker("n-plat-1", "spookify/music/platform"),
    worker("n-plat-2", "spookify/music/platform", { status: "disconnected" }),
    worker("n-music", "spookify/music"),
    worker("n-root", "spookify"),
    worker("n-loose", ""),
    worker("n-deep", "spookify/ads/sales/emea/paris"),
  ];

  it("builds every place named by a scope or a worker, with nodes under their place", () => {
    const t = placeRows({ workers }, scopes);
    expect(labels(t.childrenOf.get(""))).toEqual(["ads", "music", "n-loose", "n-root"]);
    expect(labels(t.childrenOf.get("scope:spookify/music"))).toEqual(["platform", "n-music"]);
    expect(labels(t.childrenOf.get("scope:spookify/music/platform"))).toEqual(["n-plat-1", "n-plat-2"]);
  });

  it("stops creating places at MAX_PLACE_DEPTH and files deeper nodes at the deepest place", () => {
    const t = placeRows({ workers }, scopes);
    expect(MAX_PLACE_DEPTH).toBe(3);
    expect(t.byId.has("scope:spookify/ads/sales/emea")).toBe(true);
    expect(t.byId.has("scope:spookify/ads/sales/emea/paris")).toBe(false);
    expect(t.parentOf.get("n-deep")).toBe("scope:spookify/ads/sales/emea");
    expect(t.byId.get("scope:spookify/ads/sales/emea")?.level).toBe(3);
  });

  it("numbers places among their siblings and cascades the realm to sites and nodes", () => {
    const t = placeRows({ workers }, scopes);
    const ads = t.byId.get("scope:spookify/ads")!;
    const music = t.byId.get("scope:spookify/music")!;
    expect([ads.index, ads.siblings]).toEqual([1, 2]);
    expect([music.index, music.siblings]).toEqual([2, 2]);
    expect(ads.realm).toBeUndefined();
    const platform = t.byId.get("scope:spookify/music/platform")!;
    expect(platform.level).toBe(2);
    expect(platform.index).toBe(0);
    expect(platform.realm).toBe(0);
    expect(t.byId.get("n-plat-1")?.realm).toBe(0);
    expect(t.byId.get("scope:spookify/ads/sales/emea")?.realm).toBe(0);
  });

  it("rolls worker counts up, counting each node once", () => {
    const t = placeRows({ workers }, scopes);
    expect(t.roll.get("scope:spookify/music/platform")).toEqual({ active: 1, total: 2 });
    expect(t.roll.get("scope:spookify/music")).toEqual({ active: 2, total: 3 });
    expect(t.roll.get("scope:spookify/ads")).toEqual({ active: 1, total: 1 });
    expect(t.roll.get("")).toEqual({ active: 5, total: 6 });
  });

  it("rolls recent reports: own count here, subtree count under", () => {
    const t = placeRows({ workers }, scopes);
    expect(t.reports.get("scope:spookify/music")).toEqual({ here: 2, under: 5 });
    expect(t.reports.get("scope:spookify/music/platform")).toEqual({ here: 3, under: 3 });
    expect(t.reports.get("scope:spookify/ads")).toEqual({ here: 0, under: 0 });
  });

  it("leaves the report map empty when no scope carried a window count", () => {
    const t = placeRows({ workers: [] }, [{ path: "spookify/x" }]);
    expect(t.reports.size).toBe(0);
    expect(t.roll.get("scope:spookify/x")).toEqual({ active: 0, total: 0 });
  });

  it("is empty for nothing", () => {
    const t = placeRows({ workers: [] }, []);
    expect(t.childrenOf.get("")).toEqual([]);
    expect(t.byId.size).toBe(0);
  });
});

describe("reportRows", () => {
  it("nests nodes under whoever they report to", () => {
    const ceo = worker("ceo", "spookify");
    const lead = worker("lead", "spookify/music", { reports_to: "ceo" });
    const dev = worker("dev", "spookify/music", { reports_to: "lead", status: "idle" });
    const childrenOf = new Map<string, Worker[]>([
      ["", [ceo]],
      ["ceo", [lead]],
      ["lead", [dev]],
    ]);
    const t = reportRows({ childrenOf });
    expect(labels(t.childrenOf.get(""))).toEqual(["ceo"]);
    expect(t.parentOf.get("dev")).toBe("lead");
    expect(t.roll.get("ceo")).toEqual({ active: 1, total: 2 });
    expect(t.roll.get("dev")).toEqual({ active: 0, total: 0 });
    expect(t.byId.get("dev")?.kind).toBe("node");
  });
});

describe("rowInSelection", () => {
  const place: Row = {
    id: "scope:spookify/music",
    kind: "place",
    label: "music",
    path: "spookify/music",
    level: 1,
    index: 1,
    siblings: 1,
  };
  const node: Row = { id: "n1", kind: "node", label: "n1", level: 0, index: 1, siblings: 1 };

  it("a node answers to the match set, a place to the scope selection", () => {
    const all = { matchAll: true, matches: new Set<string>(), scopeSel: [] as string[] };
    expect(rowInSelection(all, node)).toBe(true);
    expect(rowInSelection(all, place)).toBe(true);

    const narrowed = { matchAll: false, matches: new Set(["n2"]), scopeSel: ["spookify/ads"] };
    expect(rowInSelection(narrowed, node)).toBe(false);
    expect(rowInSelection(narrowed, place)).toBe(false);
    expect(rowInSelection({ ...narrowed, scopeSel: ["spookify/music"] }, place)).toBe(true);
    expect(rowInSelection({ ...narrowed, matches: new Set(["n1"]) }, node)).toBe(true);
  });
});
