import { describe, expect, it } from "vitest";
import {
  ADMIN_DEFAULT,
  adminHash,
  blankRoute,
  buildHash,
  crossViewRoute,
  parseHash,
  reportingHash,
  underAnyScope,
  underScope,
  viewHash,
} from "./routes";
import { DEFAULT_WINDOW } from "./window";

describe("parseHash", () => {
  it("reads the view from the slug and the scope from the path", () => {
    const r = parseHash("#/control-plane/spookify/music");
    expect(r.view).toBe("overview");
    expect(r.scope).toBe("spookify/music");
    expect(r.scopes).toEqual(["spookify/music"]);
  });

  it("lands an unknown or empty hash on the control plane", () => {
    expect(parseHash("").view).toBe("overview");
    expect(parseHash("#/nowhere/at/all")).toEqual(blankRoute("overview"));
    expect(parseHash("#/")).toEqual(blankRoute("overview"));
  });

  it("keeps only known categories and stances", () => {
    const r = parseHash("#/outcomes/spookify?category=delivery,bogus,failures&stance=strategic");
    expect(r.cats).toEqual(["delivery", "failures"]);
    expect(r.stance).toBe("strategic");
    expect(parseHash("#/outcomes?stance=wishful").stance).toBe("");
  });

  it("reads report only on the outcomes view", () => {
    expect(parseHash("#/outcomes/spookify/music?report=incident").report).toBe("incident");
    expect(parseHash("#/control-plane/spookify?report=incident").report).toBe("");
  });

  it("collects the multi-select from `also`, trimming slashes", () => {
    const r = parseHash("#/outcomes/spookify/music?also=/spookify/finance/,spookify/ads");
    expect(r.scopes).toEqual(["spookify/music", "spookify/finance", "spookify/ads"]);
    expect(r.scope).toBe("spookify/music");
  });

  it("carries the window and the selection", () => {
    const r = parseHash("#/outcomes?window=3d&selected=node:music-playback-01");
    expect(r.win).toEqual({ n: 3, unit: "d" });
    expect(r.selected).toEqual({ kind: "node", id: "music-playback-01" });
    expect(parseHash("#/outcomes?window=never").win).toEqual(DEFAULT_WINDOW);
  });

  it("gives settings a section, defaulting to the connection page", () => {
    expect(parseHash("#/settings").section).toBe(ADMIN_DEFAULT);
    expect(parseHash("#/settings/docs/hierarchy").section).toBe("docs/hierarchy");
    expect(parseHash("#/settings/docs?category=logs").cats).toEqual([]);
  });

  it("survives a malformed escape in a segment", () => {
    expect(parseHash("#/control-plane/100%").scope).toBe("100%");
  });
});

describe("buildHash", () => {
  it("writes the shortest hash for a blank route", () => {
    expect(buildHash(blankRoute("overview"))).toBe("#/control-plane");
    expect(buildHash(blankRoute("reporting"))).toBe("#/outcomes");
    expect(buildHash(blankRoute("admin"))).toBe("#/settings/" + ADMIN_DEFAULT);
  });

  it("omits the default window and includes any other", () => {
    expect(buildHash({ ...blankRoute("reporting"), win: { n: 1, unit: "w" } })).toBe("#/outcomes");
    expect(buildHash({ ...blankRoute("reporting"), win: { n: 12, unit: "h" } })).toBe("#/outcomes?window=12h");
  });

  it("keeps slashes, colons and at-signs legible in the query", () => {
    const h = buildHash({
      ...blankRoute("reporting"),
      scope: "spookify/music",
      scopes: ["spookify/music", "spookify/finance"],
      selected: { kind: "report", def: "task-board", scope: "spookify/content" },
    });
    expect(h).toBe("#/outcomes/spookify/music?also=spookify/finance&selected=report:task-board@spookify/content");
  });

  it("escapes only the structural characters in a value", () => {
    const h = buildHash({ ...blankRoute("reporting"), report: "a b&c=d?e#f%" });
    expect(h).toBe("#/outcomes?report=a%20b%26c%3Dd%3Fe%23f%25");
    expect(parseHash(h).report).toBe("a b&c=d?e#f%");
  });
});

describe("round trips", () => {
  const hashes = [
    "#/control-plane",
    "#/control-plane/spookify/music",
    "#/outcomes/spookify/music?category=delivery,failures",
    "#/outcomes/spookify/music?report=incident",
    "#/outcomes/spookify/finance/fp-and-a?report=budget-variance&stance=strategic",
    "#/outcomes?selected=node:music-playback-01",
    "#/outcomes/spookify/music?also=spookify/finance&role=builder,ops&window=3mo",
    "#/schedules",
    "#/settings/connection",
    "#/settings/docs/hierarchy",
  ];
  for (const h of hashes) {
    it(`parse then build keeps ${h}`, () => {
      expect(buildHash(parseHash(h))).toBe(h);
    });
  }

  it("keeps a scope segment with a space through encoding", () => {
    const r = { ...blankRoute("overview"), scope: "spookify/two words" };
    const h = buildHash(r);
    expect(h).toBe("#/control-plane/spookify/two%20words");
    expect(parseHash(h).scope).toBe("spookify/two words");
  });
});

describe("view builders", () => {
  it("carries scope, category and window across views but not the report or section", () => {
    const from = parseHash("#/outcomes/spookify/music?report=incident&category=logs&window=2d");
    const to = crossViewRoute(from, "overview");
    expect(to.view).toBe("overview");
    expect(to.scope).toBe("spookify/music");
    expect(to.cats).toEqual(["logs"]);
    expect(to.win).toEqual({ n: 2, unit: "d" });
    expect(to.report).toBe("");
    expect(to.section).toBe("");
  });

  it("a scope override alone selects just that scope", () => {
    const cur = parseHash("#/outcomes/spookify/music?also=spookify/finance");
    expect(viewHash(cur, { scope: "spookify/ads" })).toBe("#/outcomes/spookify/ads");
    expect(viewHash(cur, { scope: "" })).toBe("#/outcomes");
  });

  it("reportingHash switches to outcomes and drops the settings section", () => {
    const cur = parseHash("#/settings/docs/hierarchy");
    expect(reportingHash(cur, { report: "incident" })).toBe("#/outcomes?report=incident");
  });

  it("adminHash falls back to the default section", () => {
    expect(adminHash("")).toBe("#/settings/" + ADMIN_DEFAULT);
    expect(adminHash("updates")).toBe("#/settings/updates");
  });
});

describe("underScope", () => {
  it("matches on segment boundaries, not string prefixes", () => {
    expect(underScope("spookify/advertising/adtech", "spookify/advertising")).toBe(true);
    expect(underScope("spookify/advertising", "spookify/advertising")).toBe(true);
    expect(underScope("spookify/advertising-labs", "spookify/advertising")).toBe(false);
    expect(underScope("anything", "")).toBe(true);
  });

  it("underAnyScope treats an empty selection as everything", () => {
    expect(underAnyScope("spookify/x", [])).toBe(true);
    expect(underAnyScope("spookify/x", ["spookify/y", "spookify/x"])).toBe(true);
    expect(underAnyScope("spookify/x", ["spookify/y"])).toBe(false);
  });
});
