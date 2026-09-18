import { useState } from "react";

const PAGE_SIZE = 12;
import { Async, Skeleton } from "../../components/Async";
import { fmtTime } from "../../lib/format";
import { UPDATE_PHASES, phaseClass, phaseOf, type Rollout, type RolloutNode } from "../../lib/updates";
import type { RolloutState } from "../../hooks/useRollout";

interface Props {
  state: RolloutState;
  kind: string;
  onSelectKind: (kind: string) => void;
  onSelectNode?: (id: string) => void;
}

export function RolloutPanel({ state, kind, onSelectKind, onSelectNode }: Props) {
  const [only, setOnly] = useState<string | null>(null);
  const [showAll, setShowAll] = useState(false);
  const { kinds, latest, rollout, adminOnly, error } = state;

  const nothingAnnounced = !error && kinds.length === 0;

  return (
    <section className="mc-panel mc-rollout-panel" aria-labelledby="rollout-heading">
      <div className="mc-panel-header">
        <h2 id="rollout-heading">Update rollout</h2>
        <span className="mc-label-muted">
          {kinds.length ? kinds.length + (kinds.length === 1 ? " kind" : " kinds") : ""}
        </span>
      </div>

      {nothingAnnounced ? (
        <p className="mc-empty-state">
          Nothing announced yet. <code>dream updates announce</code> tells every node a version is available.
        </p>
      ) : (
        <>
          {kinds.length > 0 && (
            <div className="mc-rollout-kinds" role="group" aria-label="Update kinds">
              {kinds.map((k: string) => {
                const a = latest.get(k);
                return (
                  <button
                    key={k}
                    type="button"
                    className={"mc-rollout-kind" + (k === kind ? " is-active" : "")}
                    aria-pressed={k === kind}
                    onClick={() => {
                      onSelectKind(k);
                      setOnly(null);
                    }}
                  >
                    <span className="mc-rollout-kind-name mc-mono">{k}</span>
                    {a && <span className="mc-rollout-kind-version mc-mono">{a.version}</span>}
                  </button>
                );
              })}
            </div>
          )}

          {kind && latest.get(kind) && <AnnouncementLine kind={kind} state={state} />}

          {adminOnly ? (
            <p className="mc-empty-state">
              Reading the fleet needs an admin token. The announcements above are what any connected node can see.
            </p>
          ) : (
            <Async<RolloutNode>
              data={error ? null : rollout ? rowsInPhaseOrder(rollout, only) : null}
              error={error}
              empty={<p className="mc-empty-state">No nodes are registered yet.</p>}
              skeleton={<Skeleton lines={4} />}
            >
              {(visible) => (
                <>
                  <Counts
                    rollout={rollout!}
                    only={only}
                    onOnly={(p) => {
                      setOnly(p);
                      setShowAll(false);
                    }}
                  />
                  <ul className="mc-rollout-list">
                    {visible.length === 0 && <li className="mc-empty-state">No nodes in that state.</li>}
                    {(showAll ? visible : visible.slice(0, PAGE_SIZE)).map((n) => (
                      <li key={n.worker_id} className="mc-rollout-item">
                        {onSelectNode ? (
                          <button
                            type="button"
                            className="mc-link-button mc-rollout-node"
                            onClick={() => onSelectNode(n.worker_id)}
                          >
                            {n.worker_id}
                          </button>
                        ) : (
                          <span className="mc-rollout-node mc-mono">{n.worker_id}</span>
                        )}
                        <span className={"mc-rollout-phase " + phaseClass(n.status)} title={phaseOf(n.status).hint}>
                          {phaseOf(n.status).label}
                        </span>
                        <span className="mc-rollout-meta mc-label-muted">
                          {n.current_version || "—"}
                          {n.worker_status && n.worker_status !== "connected" ? " · " + n.worker_status : ""}
                        </span>
                        {n.detail && <span className="mc-rollout-detail mc-body-muted">{n.detail}</span>}
                      </li>
                    ))}
                  </ul>
                  {!showAll && visible.length > PAGE_SIZE && (
                    <div className="mc-rollout-more">
                      <button className="mc-button mc-button-secondary" type="button" onClick={() => setShowAll(true)}>
                        Show all {visible.length}
                      </button>
                    </div>
                  )}
                </>
              )}
            </Async>
          )}
        </>
      )}
    </section>
  );
}

function rowsInPhaseOrder(rollout: Rollout, only: string | null): RolloutNode[] {
  const order = new Map(UPDATE_PHASES.map((p, i) => [p.id, i]));
  return rollout.nodes
    .filter((n: RolloutNode) => (only ? phaseOf(n.status).id === only : true))
    .slice()
    .sort((a: RolloutNode, b: RolloutNode) => {
      const d = (order.get(phaseOf(a.status).id) ?? 0) - (order.get(phaseOf(b.status).id) ?? 0);
      return d !== 0 ? d : a.worker_id.localeCompare(b.worker_id);
    });
}

function AnnouncementLine({ kind, state }: { kind: string; state: RolloutState }) {
  const a = state.latest.get(kind);
  if (!a) return null;
  return (
    <p className="mc-rollout-announcement mc-body-muted">
      <strong className="mc-mono">{a.version}</strong>
      {a.severity ? <span className="mc-rollout-severity">{a.severity}</span> : null} announced{" "}
      {fmtTime(a.announced_at)}
      {a.announced_by ? " by " + a.announced_by : ""}
      {a.notes ? <span className="mc-rollout-notes">{a.notes}</span> : null}
    </p>
  );
}

function Counts({
  rollout,
  only,
  onOnly,
}: {
  rollout: Rollout;
  only: string | null;
  onOnly: (p: string | null) => void;
}) {
  const total = rollout.nodes.length;
  return (
    <div className="mc-rollout-counts" role="group" aria-label="Rollout status summary">
      <span className="mc-ribbon-total">
        <strong>{total}</strong> {total === 1 ? "node" : "nodes"}
      </span>
      {UPDATE_PHASES.map((p) => {
        const n = rollout.counts[p.id] || 0;
        if (!n) return null;
        const on = only === p.id;
        return (
          <button
            key={p.id}
            type="button"
            className={"mc-rollout-count " + "is-" + p.tone + (on ? " is-on" : "")}
            aria-pressed={on}
            title={p.hint}
            onClick={() => onOnly(on ? null : p.id)}
          >
            <span className="mc-rollout-count-n">{n}</span>
            <span className="mc-rollout-count-label">{p.label}</span>
          </button>
        );
      })}
    </div>
  );
}
