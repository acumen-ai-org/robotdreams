import { describe, expect, it } from "vitest";
import {
  formatSelection,
  parseSelection,
  placeID,
  sameSelection,
  selectedID,
  selectionFromID,
  type Selection,
} from "./selection";

describe("selection written form", () => {
  const cases: Array<[Selection, string]> = [
    [{ kind: "place", path: "spookify/advertising" }, "place:spookify/advertising"],
    [{ kind: "node", id: "advertising-ad-brand-01" }, "node:advertising-ad-brand-01"],
    [{ kind: "report", def: "task-board", scope: "spookify/content" }, "report:task-board@spookify/content"],
    [{ kind: "report", def: "task-board", scope: "" }, "report:task-board@"],
  ];
  for (const [sel, text] of cases) {
    it(`round-trips ${text}`, () => {
      expect(formatSelection(sel)).toBe(text);
      expect(parseSelection(text)).toEqual(sel);
    });
  }

  it("formats nothing for an empty selection", () => {
    expect(formatSelection(null)).toBe("");
    expect(formatSelection({ kind: "place", path: "" })).toBe("");
    expect(formatSelection({ kind: "node", id: "" })).toBe("");
    expect(formatSelection({ kind: "report", def: "", scope: "x" })).toBe("");
  });

  it("parses anything unrecognised as no selection", () => {
    expect(parseSelection(null)).toBeNull();
    expect(parseSelection(undefined)).toBeNull();
    expect(parseSelection("   ")).toBeNull();
    expect(parseSelection("nocolon")).toBeNull();
    expect(parseSelection("place:")).toBeNull();
    expect(parseSelection("galaxy:andromeda")).toBeNull();
    expect(parseSelection("report:no-at-sign")).toBeNull();
    expect(parseSelection("report:@spookify")).toBeNull();
  });

  it("splits a report on the last @ so an @ in a scope is not fatal", () => {
    expect(parseSelection("report:a@b@c")).toEqual({ kind: "report", def: "a@b", scope: "c" });
  });

  it("trims surrounding whitespace", () => {
    expect(parseSelection("  node:x ")).toEqual({ kind: "node", id: "x" });
  });
});

describe("sameSelection", () => {
  it("compares by kind and identity", () => {
    expect(sameSelection(null, null)).toBe(true);
    expect(sameSelection(null, { kind: "node", id: "x" })).toBe(false);
    expect(sameSelection({ kind: "node", id: "x" }, { kind: "node", id: "x" })).toBe(true);
    expect(sameSelection({ kind: "node", id: "x" }, { kind: "place", path: "x" })).toBe(false);
    expect(sameSelection({ kind: "report", def: "d", scope: "a" }, { kind: "report", def: "d", scope: "b" })).toBe(
      false,
    );
  });
});

describe("drawing ids", () => {
  it("translates places to scope: ids and back", () => {
    expect(placeID("spookify/x")).toBe("scope:spookify/x");
    expect(selectedID({ kind: "place", path: "spookify/x" })).toBe("scope:spookify/x");
    expect(selectionFromID("scope:spookify/x")).toEqual({ kind: "place", path: "spookify/x" });
  });

  it("passes node ids through and has no id for a report", () => {
    expect(selectedID({ kind: "node", id: "n1" })).toBe("n1");
    expect(selectionFromID("n1")).toEqual({ kind: "node", id: "n1" });
    expect(selectedID({ kind: "report", def: "d", scope: "s" })).toBeNull();
    expect(selectedID(null)).toBeNull();
    expect(selectionFromID(null)).toBeNull();
    expect(selectionFromID("")).toBeNull();
  });
});
