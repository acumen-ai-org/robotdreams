import { UniverseIcon, WorldIcon, RealmIcon, SiteIcon, nodeEmoji } from "../../lib/vocabulary";
import { NodeShapeIcon, ReportIcon } from "../icons";

const LEVEL_ICON = [UniverseIcon, WorldIcon, RealmIcon, SiteIcon];

export function TypeIcon({ kind, depth, role }: { kind: "place" | "node" | "report"; depth?: number; role?: string }) {
  const cls = "mc-type-icon";

  if (kind === "report") {
    return (
      <span className={cls} aria-hidden="true">
        <ReportIcon size={22} />
      </span>
    );
  }

  if (kind === "node") {
    const emoji = nodeEmoji(role);
    return (
      <span className={cls + " is-node"} aria-hidden="true">
        <NodeShapeIcon size={26} />
        <span className="mc-type-icon-role">{emoji}</span>
      </span>
    );
  }

  const Icon = LEVEL_ICON[Math.max(0, Math.min(LEVEL_ICON.length - 1, (depth || 1) - 1))];
  return (
    <span className={cls} aria-hidden="true">
      <Icon size={22} />
    </span>
  );
}
