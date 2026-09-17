import { useMemo } from "react";
import { labelColumn, lowerColumns, numericColumn, sumByLabel } from "./tableData";
import type { Panel } from "../lib/types";

interface Stage {
  name: string;
  count: number;
}

function readStages(panel: Panel): Stage[] {
  const cols = lowerColumns(panel.table);
  const rows = (panel.table?.rows || []) as unknown[];
  if (!cols.length || !rows.length) return [];
  const label = labelColumn(cols, ["stage", "step"]);
  if (label < 0) return [];
  const value = numericColumn(cols, rows, label);
  if (value < 0) return [];
  return sumByLabel(rows, label, value).map((v) => ({ name: v.label, count: v.value }));
}

const pct = (n: number, of: number) => (of > 0 ? Math.round((n / of) * 100) : 0);

export function Funnel({ panel }: { panel: Panel }) {
  const stages = useMemo(() => readStages(panel), [panel]);

  if (!stages.length) {
    return <p className="mc-empty-state">No stages reported.</p>;
  }

  const top = stages[0].count;
  let worst = -1;
  let worstLoss = 0;
  for (let i = 1; i < stages.length; i++) {
    const loss = stages[i - 1].count - stages[i].count;
    if (loss > worstLoss) {
      worstLoss = loss;
      worst = i;
    }
  }

  return (
    <div className="mc-funnel">
      {stages.map((s, i) => {
        const prev = i > 0 ? stages[i - 1].count : s.count;
        const lost = prev - s.count;
        return (
          <div className={"mc-funnel-stage" + (i === worst ? " is-worst" : "")} key={s.name}>
            <div className="mc-funnel-head">
              <span className="mc-funnel-name">{s.name}</span>
              <span className="mc-funnel-count mc-mono">{s.count}</span>
            </div>
            <div className="mc-funnel-track">
              <div className="mc-funnel-bar" style={{ width: Math.max(pct(s.count, top), top ? 1 : 0) + "%" }} />
            </div>
            <div className="mc-funnel-foot mc-body-muted">
              <span>
                {pct(s.count, top)}% of {stages[0].name}
              </span>
              {i > 0 && <span className="mc-funnel-loss">{lost > 0 ? `−${lost} here` : "no loss"}</span>}
            </div>
          </div>
        );
      })}
    </div>
  );
}

export default Funnel;
