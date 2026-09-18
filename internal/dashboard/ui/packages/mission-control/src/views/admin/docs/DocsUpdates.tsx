import { UPDATE_PHASES } from "../../../lib/updates";

export default function DocsUpdates() {
  return (
    <>
      <section className="mc-admin-section">
        <div
          className="mc-update-flow"
          role="img"
          aria-label="The control plane announces one update; the announcement is delivered to every node; each node decides for itself and reports back applied, declined, failed, or nothing at all."
        >
          <div className="mc-update-flow-row">
            <span className="mc-update-flow-box mc-update-flow-control">
              <span className="mc-update-flow-title">Control plane</span>
              <span className="mc-update-flow-sub">announces a kind and a version</span>
            </span>
          </div>
          <div className="mc-update-flow-arrow" aria-hidden="true">
            <span>to every node, durably</span>
          </div>
          <div className="mc-update-flow-row mc-update-flow-nodes">
            <span className="mc-update-flow-node is-done">applied</span>
            <span className="mc-update-flow-node is-moving">in progress</span>
            <span className="mc-update-flow-node is-declined">declined</span>
            <span className="mc-update-flow-node is-failed">failed</span>
            <span className="mc-update-flow-node is-silent">silent</span>
          </div>
          <div className="mc-update-flow-arrow mc-update-flow-back" aria-hidden="true">
            <span>each node reports back</span>
          </div>
        </div>
        <p className="mc-body-muted">
          An announcement is a broadcast: every registered node receives it and decides for itself whether the{" "}
          <em>kind</em> is any of its business. The control plane never installs, restarts or supervises anything, and
          never compares two version strings — a version is an opaque token it stores and hands back.
        </p>
      </section>

      <section className="mc-admin-section">
        <h2>The server announces; the node decides</h2>
        <p className="mc-body-muted">
          That line governs everything here. Delivery is durable, so a node that was offline finds the announcement
          waiting when it reconnects — which is why announcements go to disconnected nodes too, and why every row in a
          rollout carries the node's connectivity. A node that is not answering because it is down is a different
          problem from one that is ignoring you, and the panel refuses to conflate them.
        </p>
        <p className="mc-body-muted">
          Robot Dreams records what a node says about its own runtime and does not check it: it never verifies that a
          node claiming to have applied an update really did. Auditing declared state against observed reality is a
          different problem than this one.
        </p>
      </section>

      <section className="mc-admin-section">
        <h2>What a node can say</h2>
        <div className="mc-storage-table-wrap">
          <table className="mc-storage-table">
            <thead>
              <tr>
                <th scope="col">Phase</th>
                <th scope="col">What it means</th>
              </tr>
            </thead>
            <tbody>
              {UPDATE_PHASES.map((p) => (
                <tr key={p.id}>
                  <td>
                    <span className={"mc-rollout-phase is-" + p.tone}>{p.label}</span>
                  </td>
                  <td className="mc-body-muted">{p.hint}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
        <p className="mc-body-muted">
          Only <strong>applied</strong> and <strong>declined</strong> settle an announcement; acknowledged, in progress
          and failed all leave it outstanding, because none of them says the matter is closed.
        </p>
        <p className="mc-body-muted">
          <strong>Declined is not a failure.</strong> A node deliberately not applying an update has answered, and
          answered legitimately — so it is never drawn in the failure colour. Reading a fleet of deliberate opt-outs as
          a fleet of breakages would be the most useful thing this page could get wrong.
        </p>
      </section>

      <section className="mc-admin-section">
        <h2>Where it appears</h2>
        <ul className="mc-doc-list">
          <li>
            <strong>Admin → Updates</strong> — the rollout page lists every announced kind, the counts per phase (which
            are also the filter), and the nodes behind them. While a rollout is incomplete the topbar's cog carries a
            dot that links straight to it.
          </li>
          <li>
            <strong>Control Plane</strong> — the tree's Update column and the network rendering tint each node by its
            phase.
          </li>
          <li>
            <strong>Node detail</strong> — what one node reports for the selected kind, shown above the reporting
            sections because a node with no reporting scope still runs a version.
          </li>
          <li>
            <strong>Live activity</strong> — one entry per announcement, however many inboxes it fans out into.
          </li>
        </ul>
        <p className="mc-body-muted">
          Reading the fleet needs an admin token. With a worker token the announcements still list — they were broadcast
          to every node anyway — but the per-node view is withheld rather than faked.
        </p>
        <p className="mc-body-muted">
          Planets and Galaxy do not draw update phase yet. They read the same per-node state the tree and network do,
          and will show it once their current rework lands.
        </p>
        <span className="mc-provenance">source: pkg/updates · docs/updates.md · lib/updates.ts</span>
      </section>
    </>
  );
}
