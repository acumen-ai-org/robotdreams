import { entityCode, nodeCode } from "../../lib/vocabulary";
import type { ScopeSeg } from "../ScopeSelector";
import type { Row } from "../../lib/orgRows";

export function placeCode(seg: ScopeSeg | undefined): string {
  if (!seg) return "";
  switch (seg.depth) {
    case 2:
      return entityCode("W", seg.index, seg.siblings);
    case 3:
      return "R" + (((seg.index % 10) + 10) % 10);
    case 4:
      return entityCode("S", seg.index, seg.siblings);
    default:
      return "";
  }
}

export function nodeCodeOf(row: Row | undefined): string {
  return row ? nodeCode(row.index, row.siblings) : "";
}
