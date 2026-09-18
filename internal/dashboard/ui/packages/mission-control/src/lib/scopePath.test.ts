import { describe, expect, it } from "vitest";
import { scopeAncestors, scopeDepth, scopeLeaf, scopeParent } from "./scopePath";

describe("scopePath", () => {
  it("scopeParent walks one level up and stops at the universe", () => {
    expect(scopeParent("spookify/content/editorial")).toBe("spookify/content");
    expect(scopeParent("spookify")).toBe("");
    expect(scopeParent("")).toBe("");
  });

  it("scopeAncestors lists the prefixes root first, without the path itself", () => {
    expect(scopeAncestors("spookify/content/editorial")).toEqual(["spookify", "spookify/content"]);
    expect(scopeAncestors("spookify")).toEqual([]);
    expect(scopeAncestors("")).toEqual([]);
  });

  it("ignores stray slashes", () => {
    expect(scopeAncestors("/spookify//content/")).toEqual(["spookify"]);
    expect(scopeDepth("/spookify//content/")).toBe(2);
    expect(scopeLeaf("spookify/content/")).toBe("content");
  });

  it("scopeLeaf and scopeDepth describe the place", () => {
    expect(scopeLeaf("spookify/content/editorial")).toBe("editorial");
    expect(scopeLeaf("")).toBe("");
    expect(scopeDepth("")).toBe(0);
    expect(scopeDepth("spookify")).toBe(1);
    expect(scopeDepth("spookify/a/b/c")).toBe(4);
  });
});
