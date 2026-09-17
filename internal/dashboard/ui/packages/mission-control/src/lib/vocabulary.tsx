import type { SVGProps } from "react";
import { ChipPopover } from "../components/ChipPopover";

export function entityCode(prefix: string, n: number, max: number, minWidth = 2): string {
  return prefix + String(n).padStart(Math.max(minWidth, String(max).length), "0");
}

export function nodeCode(n: number, max: number): string {
  return entityCode("N", n, max, 6);
}

type IconProps = SVGProps<SVGSVGElement> & { size?: number };

function Icon({ size = 16, children, ...rest }: IconProps) {
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth={2}
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
      {...rest}
    >
      {children}
    </svg>
  );
}

export function UniverseIcon(props: IconProps) {
  return (
    <Icon {...props}>
      <path d="M20.341 6.484A10 10 0 0 1 10.266 21.85" />
      <path d="M3.659 17.516A10 10 0 0 1 13.74 2.152" />
      <circle cx="12" cy="12" r="3" />
      <circle cx="19" cy="5" r="2" />
      <circle cx="5" cy="19" r="2" />
    </Icon>
  );
}

export function WorldBadge({ n, max, size = 20 }: { n: number; max: number; size?: number }) {
  const code = entityCode("W", n, max);
  const fontSize = Math.min(size * 0.45, (size * 1.35) / code.length);
  return (
    <span
      className={"mc-world-badge " + (n % 2 === 1 ? "mc-world-blue" : "mc-world-pink")}
      style={{ width: size, height: size, fontSize }}
    >
      {code}
    </span>
  );
}

export function WorldIcon(props: IconProps) {
  return (
    <Icon {...props}>
      <circle cx="12" cy="12" r="10" />
      <path d="M12 2a14.5 14.5 0 0 0 0 20 14.5 14.5 0 0 0 0-20" />
      <path d="M2 12h20" />
    </Icon>
  );
}

export function RealmIcon(props: IconProps) {
  return (
    <Icon {...props}>
      <path d="M21 8a2 2 0 0 0-1-1.73l-7-4a2 2 0 0 0-2 0l-7 4A2 2 0 0 0 3 8v8a2 2 0 0 0 1 1.73l7 4a2 2 0 0 0 2 0l7-4A2 2 0 0 0 21 16Z" />
      <path d="m3.3 7 8.7 5 8.7-5" />
      <path d="M12 22V12" />
    </Icon>
  );
}

export function SiteIcon(props: IconProps) {
  return (
    <Icon {...props}>
      <path d="M12 16h.01" />
      <path d="M16 16h.01" />
      <path d="M3 19a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2V8.5a.5.5 0 0 0-.769-.422l-4.462 2.844A.5.5 0 0 1 15 10.5v-2a.5.5 0 0 0-.769-.422L9.77 10.922A.5.5 0 0 1 9 10.5V5a2 2 0 0 0-2-2H5a2 2 0 0 0-2 2z" />
      <path d="M8 16h.01" />
    </Icon>
  );
}

export const REALM_COLORS: Array<{ hue: string; tint: string; deep: string }> = [
  { hue: "blue", tint: "#CDE2FB", deep: "#1C5CAB" },
  { hue: "orange", tint: "#FBD9C8", deep: "#9A3D12" },
  { hue: "teal", tint: "#C4EEDD", deep: "#0E5F43" },
  { hue: "amber", tint: "#F8E5B0", deep: "#755000" },
  { hue: "magenta", tint: "#F9D5E3", deep: "#92375D" },
  { hue: "green", tint: "#CFEBC4", deep: "#2A6410" },
  { hue: "violet", tint: "#DCD6F7", deep: "#40329B" },
  { hue: "red", tint: "#FAD2D1", deep: "#A02524" },
  { hue: "cyan", tint: "#C5E8F5", deep: "#0E5B76" },
  { hue: "slate", tint: "#DFE3E8", deep: "#3E4A57" },
];

export const NODE_ROLE_EMOJI: Record<string, string> = {
  ceo: "👑",
  vp: "🌐",
  lead: "⭐",
  orchestrator: "🧭",
  builder: "🔧",
  researcher: "🔬",
  writer: "✍️",
  reviewer: "🔍",
  ops: "⚙️",
  librarian: "📚",
  messenger: "✉️",
  guardian: "🛡️",
  delegate: "🤝",
};

export const NODE_DEFAULT_EMOJI = "⬡";

export function nodeEmoji(role: string | undefined): string {
  return (role && NODE_ROLE_EMOJI[role.toLowerCase()]) || NODE_DEFAULT_EMOJI;
}

function chipClass(realm: number | undefined, inverse: boolean | undefined, extra = ""): string {
  let cls = "mc-id-chip";
  if (realm != null) cls += " mc-r" + (((realm % 10) + 10) % 10);
  if (inverse) cls += " mc-id-chip-inverse";
  if (extra) cls += " " + extra;
  return cls;
}

interface BadgeScope {
  realm?: number;
  inverse?: boolean;
}

function ChipText({ code, name }: { code: string; name?: string }) {
  return <>{name || code}</>;
}

export interface ChipChildren {
  childType?: string;
  childCount?: number;
  nodeType?: string;
  nodeCount?: number;
}

export function NodeBadge({
  n,
  max,
  role,
  name,
  realm,
  inverse,
  childType,
  childCount,
  nodeType,
  nodeCount,
}: { n: number; max: number; role?: string; name?: string } & BadgeScope & ChipChildren) {
  const code = nodeCode(n, max);
  return (
    <ChipPopover info={{ code, name, childType, childCount, nodeType, nodeCount }}>
      <span className={chipClass(realm, inverse)} aria-label={name ? code + " " + name : undefined}>
        <span className="mc-chip-icon" aria-hidden="true">
          {nodeEmoji(role)}
        </span>
        <ChipText code={code} name={name} />
      </span>
    </ChipPopover>
  );
}

export function SiteBadge({
  n,
  max,
  name,
  realm,
  inverse,
  childType,
  childCount,
  nodeType,
  nodeCount,
}: { n: number; max: number; name?: string } & BadgeScope & ChipChildren) {
  const code = entityCode("S", n, max);
  return (
    <ChipPopover info={{ code, name, childType, childCount, nodeType, nodeCount }}>
      <span className={chipClass(realm, inverse)} aria-label={name ? code + " " + name : undefined}>
        <SiteIcon size={13} className="mc-chip-icon" />
        <ChipText code={code} name={name} />
      </span>
    </ChipPopover>
  );
}

export function RealmChip({
  n,
  name,
  inverse,
  childType,
  childCount,
  nodeType,
  nodeCount,
}: { n: number; name?: string; inverse?: boolean } & ChipChildren) {
  const idx = ((n % 10) + 10) % 10;
  const code = "R" + idx;
  return (
    <ChipPopover info={{ code, name, childType, childCount, nodeType, nodeCount }}>
      <span className={chipClass(idx, inverse, "mc-realm-chip")} aria-label={name ? code + " " + name : undefined}>
        <RealmIcon size={13} className="mc-chip-icon" />
        <ChipText code={code} name={name} />
      </span>
    </ChipPopover>
  );
}
