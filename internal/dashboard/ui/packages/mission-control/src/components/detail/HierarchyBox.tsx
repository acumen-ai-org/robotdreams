import type { ReactNode } from "react";
import { ArrowDownIcon, ArrowUpIcon } from "../icons";

export interface HierarchyEntry {
  key: string;
  label: ReactNode;
  hint?: string;
  onPick: () => void;
}

const PEEK = 4;

export function HierarchyBox({
  upCaption,
  up,
  downCaption,
  down,
  emptyDown,
}: {
  upCaption: string;
  up: HierarchyEntry[];
  downCaption: string;
  down: HierarchyEntry[];
  emptyDown: string;
}) {
  const shown = down.slice(0, PEEK);
  const more = down.length - shown.length;
  return (
    <div className="mc-hier-box">
      <div className="mc-hier-part">
        <span className="mc-hier-caption">
          <ArrowUpIcon size={10} /> {upCaption}
        </span>
        {up.length === 0 ? (
          <span className="mc-hier-none">the top</span>
        ) : (
          <span className="mc-hier-list">
            {up.map((e) => (
              <button key={e.key} type="button" className="mc-hier-item" title={e.hint} onClick={e.onPick}>
                {e.label}
              </button>
            ))}
          </span>
        )}
      </div>

      <div className="mc-hier-part">
        <span className="mc-hier-caption">
          <ArrowDownIcon size={10} /> {downCaption}
        </span>
        {down.length === 0 ? (
          <span className="mc-hier-none">{emptyDown}</span>
        ) : (
          <span className="mc-hier-list">
            {shown.map((e) => (
              <button key={e.key} type="button" className="mc-hier-item" title={e.hint} onClick={e.onPick}>
                {e.label}
              </button>
            ))}
            {more > 0 && <span className="mc-hier-more">… {more} more</span>}
          </span>
        )}
      </div>
    </div>
  );
}
