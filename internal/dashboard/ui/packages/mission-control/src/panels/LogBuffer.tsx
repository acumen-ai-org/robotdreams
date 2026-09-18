import { tableColumns } from "./DataGrid";
import { mcVariant } from "../components/shared";
import { cellText } from "../lib/format";
import type { Panel } from "../lib/types";

const LEVEL_KEYS = ["level", "severity", "sev"];
const TIME_KEYS = ["when", "time", "t", "at", "timestamp"];

function pick(row: Record<string, unknown>, keys: string[]): string {
  for (const k of keys) {
    if (row[k] != null) return cellText(row[k]);
  }
  return "";
}

export default function LogBuffer({ panel }: { panel: Panel }) {
  const t = panel.table;
  const rows = t?.rows || [];
  const columns = tableColumns(t);

  if (!rows.length) return <p className="mc-empty-state">No log lines.</p>;

  return (
    <div className="mc-logbuffer">
      {rows.map((raw, i) => {
        const row: Record<string, unknown> = Array.isArray(raw)
          ? Object.fromEntries(columns.map((c, j) => [c, raw[j]]))
          : raw || {};
        const level = pick(row, LEVEL_KEYS).toLowerCase();
        const time = pick(row, TIME_KEYS);
        const rest = columns
          .filter((c) => !LEVEL_KEYS.includes(c) && !TIME_KEYS.includes(c))
          .map((c) => cellText(row[c]))
          .filter(Boolean)
          .join("  ");
        return (
          <div className="mc-logbuffer-line" key={i}>
            {time && <span className="mc-lb-time">{time}</span>}
            {level && <span className={"mc-lb-level " + mcVariant(level)}>{level.toUpperCase()}</span>}
            <span className="mc-lb-msg">{rest}</span>
          </div>
        );
      })}
    </div>
  );
}
