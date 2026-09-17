import { useMemo } from "react";
import { humanLabel, isBlocked, isLate, triageItems } from "../lib/plan";
import type { Panel, PlanItem, PlanView } from "../lib/types";

const CHIPS_PER_CELL = 6;

export function RoadmapLanes({ panel }: { panel: Panel }) {
  const plan = panel.plan;
  const items = useMemo(() => plan?.items || [], [plan]);
  const now = Date.now();

  const { horizons, lanes, cells } = useMemo(() => {
    const hs = (plan?.horizons || []).map((h) => h.name);
    const ls = (plan?.lanes || []).map((l) => l.name);
    const map = new Map<string, PlanItem[]>();
    let anyUnscheduled = false;
    let anyUnassigned = false;
    for (const it of items) {
      const h = it.horizon && hs.includes(it.horizon) ? it.horizon : "";
      const l = it.lane && ls.includes(it.lane) ? it.lane : "";
      if (!h) anyUnscheduled = true;
      if (!l) anyUnassigned = true;
      const key = l + " " + h;
      if (!map.has(key)) map.set(key, []);
      map.get(key)!.push(it);
    }
    return {
      horizons: anyUnscheduled ? [...hs, ""] : hs,
      lanes: anyUnassigned ? [...ls, ""] : ls,
      cells: map,
    };
  }, [plan, items]);

  if (!plan) return <p className="mc-empty-state">This report declares no plan.</p>;
  if (!items.length) return <p className="mc-empty-state">Nothing is planned at this scope.</p>;
  if (!horizons.length) {
    return <p className="mc-empty-state">This plan declares no horizons, so it has no timeline to lay out.</p>;
  }

  return (
    <div className="mc-storage-table-wrap mc-plan-lanes">
      <table className="mc-storage-table">
        <thead>
          <tr>
            <th scope="col" className="mc-plan-lane-head">
              Lane
            </th>
            {horizons.map((h) => (
              <th scope="col" key={h || "__none"}>
                {h ? humanLabel(h) : "unscheduled"}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {lanes.map((l) => (
            <tr key={l || "__none"}>
              <th scope="row" className="mc-plan-lane-head">
                {l ? humanLabel(l) : "unassigned"}
              </th>
              {horizons.map((h) => {
                const cell = triageItems(cells.get(l + " " + h) || [], now);
                return (
                  <td key={(h || "__none") + "/" + (l || "__none")}>
                    {cell.slice(0, CHIPS_PER_CELL).map((it) => (
                      <LaneChip key={(it.scope || "") + "/" + it.id} item={it} plan={plan} now={now} />
                    ))}
                    {cell.length > CHIPS_PER_CELL && (
                      <span className="mc-plan-more-inline">+{cell.length - CHIPS_PER_CELL}</span>
                    )}
                  </td>
                );
              })}
            </tr>
          ))}
        </tbody>
      </table>
      <p className="mc-plan-note">
        Weight is commitment, not colour: solid is committed, outlined is planned, dashed is a candidate.
      </p>
    </div>
  );
}

function weightClass(plan: PlanView, item: PlanItem): string {
  const order = (plan.commitments || []).map((c) => c.name);
  const i = order.indexOf(item.commitment || "");
  if (i < 0) return "is-w2";
  if (i === 0) return "is-w0";
  if (i === order.length - 1 && order.length > 2) return "is-w2";
  return "is-w1";
}

function LaneChip({ item, plan, now }: { item: PlanItem; plan: PlanView; now: number }) {
  const blocked = isBlocked(item);
  const late = isLate(item, now);
  const commitment = item.commitment ? humanLabel(item.commitment) : "";
  return (
    <span
      className={
        "mc-plan-lane-chip " + weightClass(plan, item) + (blocked ? " is-blocked" : "") + (late ? " is-late" : "")
      }
      title={[item.title || item.id, commitment && "commitment: " + commitment, item.owner].filter(Boolean).join(" · ")}
    >
      {item.title || item.id}
      {commitment && <span className="mc-sr-only"> ({commitment})</span>}
      {blocked && <span className="mc-sr-only"> (blocked)</span>}
      {late && <span className="mc-sr-only"> (past due)</span>}
    </span>
  );
}

export default RoadmapLanes;
