import { rollupOf, rollupTitle } from "../lib/aggregation";
import type { SummaryTile } from "../lib/types";
import { LayersIcon } from "./icons";

export function RollupMark({ tile, scope, className }: { tile: SummaryTile; scope: string; className?: string }) {
  const r = rollupOf(tile, scope);
  if (!r) return null;
  return (
    <span
      className={"mc-rollup-mark" + (r.levels ? " is-rolled" : "") + (className ? " " + className : "")}
      title={rollupTitle(r)}
    >
      <LayersIcon size={11} />
      <span className="mc-mono">{r.leaves}</span>
      {r.levels > 0 && <span className="mc-rollup-levels">+{r.levels}</span>}
    </span>
  );
}
