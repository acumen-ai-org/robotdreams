import { useMemo } from "react";
import { groupByState, humanLabel, isBlocked, isLate, daysLate, overWip, triageItems } from "../lib/plan";
import type { Panel, PlanItem, PlanView } from "../lib/types";

const CARDS_PER_COLUMN = 25;

const MAX_LANES = 6;

export function Board({ panel }: { panel: Panel }) {
  const plan = panel.plan;
  const items = useMemo(() => plan?.items || [], [plan]);
  const now = Date.now();

  const columns = useMemo(() => {
    if (!plan) return [];
    const grouped = groupByState(plan, items);
    const declared = (plan.states || []).map((c) => ({
      state: c.state,
      total: c.items,
      limit: c.limit || 0,
      cards: triageItems(grouped.get(c.state) || [], now),
    }));
    const unplaced = grouped.get("") || [];
    if (unplaced.length) {
      declared.push({ state: "", total: unplaced.length, limit: 0, cards: triageItems(unplaced, now) });
    }
    return declared;
  }, [plan, items, now]);

  if (!plan || !(plan.states || []).length) {
    return <p className="mc-empty-state">This report declares no plan.</p>;
  }
  if (!items.length) {
    return <p className="mc-empty-state">Nothing is planned at this scope.</p>;
  }

  const busiest = Math.max(1, ...columns.map((c) => c.total));
  const over = overWip(plan);
  const lanes = (plan.lanes || []).map((l) => l.name);
  const laneChips = lanes.length > MAX_LANES;
  const blockedTotal = plan.blocked ?? items.filter(isBlocked).length;
  const lateTotal = items.filter((it) => isLate(it, now)).length;

  return (
    <div className="mc-plan-board">
      <div className="mc-plan-columns" style={{ gridTemplateColumns: `repeat(${columns.length}, minmax(11rem, 1fr))` }}>
        {columns.map((col) => {
          const limitHit = col.limit > 0 && col.total > col.limit;
          const blocked = col.cards.filter(isBlocked).length;
          const label = col.state ? humanLabel(col.state) : "unplaced";
          const hidden = col.total - col.cards.length;
          return (
            <section
              className={"mc-plan-column" + (limitHit ? " is-over" : "")}
              key={col.state || "__unplaced"}
              aria-label={`${label} — ${col.total} items${col.limit ? `, limit ${col.limit}` : ""}${
                blocked ? `, ${blocked} blocked` : ""
              }`}
            >
              <header className="mc-plan-column-head">
                <h4 className="mc-plan-column-title">{label}</h4>
                <p className="mc-plan-column-count">
                  <strong>{col.total}</strong>
                  {col.limit > 0 && <span className="mc-plan-limit"> / {col.limit}</span>}
                  {limitHit && <span className="mc-plan-over"> over by {col.total - col.limit}</span>}
                  {blocked > 0 && <span className="mc-plan-blocked-count"> · {blocked} blocked</span>}
                </p>
                <div className="mc-plan-fill" aria-hidden="true">
                  <span style={{ width: `${Math.round((col.total / busiest) * 100)}%` }} />
                </div>
              </header>
              <ul className="mc-plan-cards" role="list">
                {col.cards.slice(0, CARDS_PER_COLUMN).map((it) => (
                  <BoardCard key={cardKey(it)} item={it} now={now} plan={plan} laneChip={laneChips} />
                ))}
              </ul>
              {(hidden > 0 || col.cards.length > CARDS_PER_COLUMN) && (
                <p className="mc-plan-more">+ {col.total - Math.min(col.cards.length, CARDS_PER_COLUMN)} more</p>
              )}
            </section>
          );
        })}
      </div>

      <p className="mc-sr-only" role="status" aria-live="polite">
        {plan.total ?? items.length} items · {blockedTotal} blocked · {over.length} column
        {over.length === 1 ? "" : "s"} over the work-in-progress limit · {lateTotal} past due
      </p>
      {plan.truncated && (
        <p className="mc-plan-note">
          Showing the work that needs attention first; this scope rolls up more than the board draws.
        </p>
      )}
    </div>
  );
}

function cardKey(it: PlanItem): string {
  return `${it.scope || ""}/${it.id}`;
}

function BoardCard({ item, now, plan, laneChip }: { item: PlanItem; now: number; plan: PlanView; laneChip: boolean }) {
  const blocked = isBlocked(item);
  const late = daysLate(item, now);
  const overdue = !Number.isNaN(late) && late > 0;
  const soon = !Number.isNaN(late) && late <= 0 && late > -3;
  const unit = plan.size_unit ? ` ${plan.size_unit}` : "";
  return (
    <li className={"mc-plan-card" + (blocked ? " is-blocked" : "") + (overdue ? " is-late" : "")}>
      <p className="mc-plan-card-title">{item.title || item.id}</p>
      <p className="mc-plan-card-meta">
        {item.owner && <span className="mc-plan-owner">{item.owner}</span>}
        {laneChip && item.lane && <span className="mc-plan-chip">{humanLabel(item.lane)}</span>}
        {item.size ? (
          <span className="mc-plan-size">
            {item.size}
            {unit}
          </span>
        ) : null}
        {item.scope && <span className="mc-plan-scope">{leaf(item.scope)}</span>}
      </p>
      {(blocked || overdue || soon) && (
        <p className="mc-plan-card-flags">
          {blocked && (
            <span className="mc-plan-flag is-blocked" title={`blocked by ${(item.blocked_by || []).join(", ")}`}>
              blocked
            </span>
          )}
          {overdue && <span className="mc-plan-flag is-late">{Math.round(late)}d late</span>}
          {!overdue && soon && <span className="mc-plan-flag">due in {Math.abs(Math.round(late))}d</span>}
        </p>
      )}
    </li>
  );
}

function leaf(scope: string): string {
  const parts = scope.split("/");
  return parts[parts.length - 1] || scope;
}

export default Board;
