import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import {
  NODE_DEFAULT_EMOJI,
  NODE_ROLE_EMOJI,
  NodeBadge,
  REALM_COLORS,
  RealmChip,
  SiteBadge,
  entityCode,
  nodeCode,
  nodeEmoji,
} from "./vocabulary";

describe("codes", () => {
  it("pads to the width of the largest count with a per-level floor", () => {
    expect(entityCode("W", 3, 12)).toBe("W03");
    expect(entityCode("S", 7, 120)).toBe("S007");
    expect(entityCode("W", 1, 4)).toBe("W01");
    expect(entityCode("W", 1, 4, 1)).toBe("W1");
    expect(entityCode("S", 1234, 5000, 2)).toBe("S1234");
  });

  it("nodes have a six-digit floor", () => {
    expect(nodeCode(1, 40)).toBe("N000001");
    expect(nodeCode(42, 40)).toBe("N000042");
    expect(nodeCode(1, 12_000_000)).toBe("N00000001");
  });
});

describe("nodeEmoji", () => {
  it("maps every documented role and is case-insensitive", () => {
    for (const [role, emoji] of Object.entries(NODE_ROLE_EMOJI)) {
      expect(nodeEmoji(role)).toBe(emoji);
      expect(nodeEmoji(role.toUpperCase())).toBe(emoji);
    }
  });

  it("falls back to ⬡ for an unknown or missing role", () => {
    expect(NODE_DEFAULT_EMOJI).toBe("⬡");
    expect(nodeEmoji("astronaut")).toBe("⬡");
    expect(nodeEmoji(undefined)).toBe("⬡");
    expect(nodeEmoji("")).toBe("⬡");
  });
});

describe("REALM_COLORS", () => {
  it("has the ten realm hues with hex pairs", () => {
    expect(REALM_COLORS).toHaveLength(10);
    for (const c of REALM_COLORS) {
      expect(c.tint).toMatch(/^#[0-9A-F]{6}$/i);
      expect(c.deep).toMatch(/^#[0-9A-F]{6}$/i);
    }
  });
});

describe("chips", () => {
  it("NodeBadge shows the role emoji as decoration and the name as the label", () => {
    render(<NodeBadge n={1} max={40} realm={2} role="builder" name="playback" />);
    const chip = screen.getByLabelText("N000001 playback");
    expect(chip).toHaveClass("mc-id-chip", "mc-r2");
    expect(chip).toHaveTextContent("playback");
    expect(chip).not.toHaveTextContent("N000001");
    const icon = chip.querySelector(".mc-chip-icon");
    expect(icon).toHaveAttribute("aria-hidden", "true");
    expect(icon).toHaveTextContent("🔧");
  });

  it("NodeBadge shows the code when there is no name and ⬡ for an unknown role", () => {
    render(<NodeBadge n={5} max={40} role="astronaut" />);
    const chip = screen.getByText("N000005").closest(".mc-id-chip");
    expect(chip).not.toHaveAttribute("aria-label");
    expect(chip?.querySelector(".mc-chip-icon")).toHaveTextContent("⬡");
    expect(chip).not.toHaveClass("mc-r0");
  });

  it("SiteBadge and RealmChip carry the realm class, wrapping past ten", () => {
    render(
      <>
        <SiteBadge n={17} max={120} realm={4} name="deploy-squad" inverse />
        <RealmChip n={13} name="engineering" />
      </>,
    );
    const site = screen.getByLabelText("S017 deploy-squad");
    expect(site).toHaveClass("mc-r4", "mc-id-chip-inverse");
    const realm = screen.getByLabelText("R3 engineering");
    expect(realm).toHaveClass("mc-realm-chip", "mc-r3");
  });
});
