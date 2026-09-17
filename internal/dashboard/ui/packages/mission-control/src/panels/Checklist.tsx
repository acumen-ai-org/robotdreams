import { humanLabel, isBlocked, isLate, triageItems } from "../lib/plan";
import type { Panel, PlanItem } from "../lib/types";

const CAP = 200;
const DONE_MARK = "✓";
const OPEN_MARK = "·";

export function Checklist({ panel }: { panel: Panel }) {
  const plan = panel.plan;
  const items = plan?.items || [];
  const now = Date.now();

  if (!plan || !(plan.states || []).length) {
    return <p className="mc-empty-state">This report declares no plan.</p>;
  }
  if (!items.length) return <p className="mc-empty-state">Nothing on the list.</p>;

  const terminal = (plan.states || []).at(-1)?.state;
  const done = items.filter((it) => it.state === terminal);
  const open = triageItems(
    items.filter((it) => it.state !== terminal),
    now,
  );
  const ordered = [...open, ...done].slice(0, CAP);

  return (
    <div className="mc-plan-checklist">
      <header className="mc-plan-checklist-head">
        <p className="mc-plan-checklist-count">
          {done.length} of {items.length} done
        </p>
        <div
          className="mc-plan-progress"
          role="progressbar"
          aria-valuenow={done.length}
          aria-valuemin={0}
          aria-valuemax={items.length}
          aria-label="Items complete"
        >
          <span style={{ width: Math.round((done.length / Math.max(1, items.length)) * 100) + "%" }} />
        </div>
      </header>
      <ul className="mc-plan-list" role="list">
        {ordered.map((it) => (
          <ChecklistRow key={(it.scope || "") + "/" + it.id} item={it} isDone={it.state === terminal} now={now} />
        ))}
      </ul>
      {items.length > CAP && <p className="mc-plan-more">+ {items.length - CAP} more</p>}
    </div>
  );
}

function ChecklistRow({ item, isDone, now }: { item: PlanItem; isDone: boolean; now: number }) {
  const blocked = isBlocked(item);
  const late = isLate(item, now);
  return (
    <li className={"mc-plan-list-item" + (isDone ? " is-done" : "") + (blocked ? " is-blocked" : "")}>
      <span className="mc-plan-mark" aria-hidden="true">
        {isDone ? DONE_MARK : OPEN_MARK}
      </span>
      <span className="mc-sr-only">{isDone ? "done" : "not done"}: </span>
      <span className="mc-plan-list-title">{item.title || item.id}</span>
      {item.owner && <span className="mc-plan-owner">{item.owner}</span>}
      {blocked && <span className="mc-plan-flag is-blocked">blocked</span>}
      {late && !isDone && <span className="mc-plan-flag is-late">past due</span>}
      {item.lane && <span className="mc-plan-chip">{humanLabel(item.lane)}</span>}
    </li>
  );
}

export default Checklist;
