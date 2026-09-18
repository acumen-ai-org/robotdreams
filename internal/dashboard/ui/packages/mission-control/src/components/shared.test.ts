import { describe, expect, it } from "vitest";
import { mcVariant, statusDotClass } from "./shared";

describe("statusDotClass", () => {
  it("prefixes wire statuses with mc- so a stylesheet rule can match them", () => {
    expect(statusDotClass("connected")).toBe("mc-connected");
    expect(statusDotClass("critical")).toBe("mc-critical");
  });

  it("maps anything unrecognised to mc-unknown rather than to no rule", () => {
    expect(statusDotClass("bogus")).toBe("mc-unknown");
    expect(statusDotClass(undefined)).toBe("mc-unknown");
  });
});

describe("mcVariant", () => {
  it("prefixes a wire value and is empty for none", () => {
    expect(mcVariant("info")).toBe("mc-info");
    expect(mcVariant(" warn ")).toBe("mc-warn");
    expect(mcVariant("")).toBe("");
    expect(mcVariant(undefined)).toBe("");
  });
});
