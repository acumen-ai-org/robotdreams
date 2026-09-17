import { adminRoute, CATEGORIES } from "../../../lib/routes";
import { Link } from "../../../components/Link";

const BOUNDARIES: Record<string, { is: string; not: string }> = {
  logs: {
    is: "Raw and structured log output; console buffers, search.",
    not: "Not failures — a log line is evidence, not a verdict.",
  },
  performance: {
    is: "Latency, throughput, resource use, capacity, trends.",
    not: "Not cost — different owner, and percentiles where cost sums.",
  },
  roadmap: {
    is: "Planned work, milestones, targets, schedule drift.",
    not: "Not delivery — a plan is not a shipment.",
  },
  delivery: {
    is: "Shipped work — deploys, releases, completed handoffs.",
    not: "Not quality — shipping says nothing about being right.",
  },
  failures: {
    is: "Errors, incidents, rollbacks, degradations, postmortems.",
    not: "Not decisions — a decision is not a fault.",
  },
  activity: {
    is: "Current activity — the live pulse at every scope level.",
    not: "Not logs — activity is now, logs are the record.",
  },
  decisions: {
    is: "Escalations, approvals, and work blocked on a human.",
    not: "Not failures — waiting on a person is not breakage.",
  },
  cost: {
    is: "Tokens, compute, spend — what the work consumed.",
    not: "Not performance — finance reads it, and it sums when rolled up.",
  },
  quality: {
    is: "Review outcomes, acceptance, rework.",
    not: "Not delivery — work can ship, not break, and still be wrong twice.",
  },
};

const NOT_CATEGORIES = [
  {
    what: "Bottlenecks / blocks",
    why: "A bottleneck is not something a node reports about itself — it only exists in the comparison between scopes, where a queue grows here because throughput is low there. Its constituent signals already live in decisions, performance, delivery and roadmap.",
    instead: "A question asked of reports. The conversation facet's own example is “where are today's bottlenecks?”",
  },
  {
    what: "Audit / compliance",
    why: "This splits in two, which is why it reads awkwardly as one word. Auditability — retention, immutability, provenance — is a property of every report, and as a filter value it produces nonsense (“is this delivery report auditable?” — it had better be).",
    instead:
      "The property belongs beside the aggregation policy in the contract. The subject half — access reviews, policy attestation, licence checks — would be a legitimate category.",
  },
];

export default function DocsCategories() {
  return (
    <>
      <section className="mc-admin-section">
        <div className="mc-cat-chips">
          {CATEGORIES.map((c) => (
            <span className="mc-tag" key={c}>
              {c}
            </span>
          ))}
        </div>
        <p className="mc-body-muted">
          A category is what a report is <em>about</em>. A definition declares which it covers, and every dashboard,
          timeline and pulse view filters by the same set. Deployments may add their own — but a report using a
          non-standard category is invisible to standard filters, and that trade-off is the point of having a standard.
        </p>
      </section>

      <section className="mc-admin-section">
        <h2>The nine, and where each one stops</h2>
        <div className="mc-storage-table-wrap">
          <table className="mc-storage-table">
            <thead>
              <tr>
                <th scope="col">Category</th>
                <th scope="col">What it covers</th>
                <th scope="col">Where it stops</th>
              </tr>
            </thead>
            <tbody>
              {CATEGORIES.map((c) => (
                <tr key={c}>
                  <td>
                    <span className="mc-tag">{c}</span>
                  </td>
                  <td>{BOUNDARIES[c]?.is}</td>
                  <td className="mc-body-muted">{BOUNDARIES[c]?.not}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </section>

      <section className="mc-admin-section">
        <h2>What deliberately isn't a category</h2>
        <p className="mc-body-muted">
          Two things that feel like they should be, and are more useful once they are not. Both point at the same
          missing axis — see below.
        </p>
        <ul className="mc-doc-list">
          {NOT_CATEGORIES.map((n) => (
            <li key={n.what}>
              <strong>{n.what}</strong>
              <p className="mc-body-muted">{n.why}</p>
              <p className="mc-body-muted">
                <em>Instead:</em> {n.instead}
              </p>
            </li>
          ))}
        </ul>
      </section>

      <section className="mc-admin-section">
        <h2>The axis we have half captured</h2>
        <p className="mc-body-muted">
          The contract now has five axes: <strong>subject</strong> (categories), <strong>shape</strong> (facets),{" "}
          <strong>delivery</strong> (modalities), <strong>drawing</strong> (primitives) and — since{" "}
          <Link to={adminRoute("docs/stances")}>stances</Link> — <strong>footing</strong>: what the numbers are measured
          against.
        </p>
        <p className="mc-body-muted">
          That closes one half of the gap this section used to describe. A stance says{" "}
          <em>what decision the reader is about to make</em> — steer, sustain, or investigate. It still does not say{" "}
          <strong>who is asking</strong>. The word “consumer” appears throughout the contracts — “the consumer picks the
          modality that fits its context” — but nothing anywhere defines one. The only personas that exist (in{" "}
          <code className="mc-mono">media.yaml</code>) describe who <em>narrates</em> a report, not who reads it.
        </p>
        <p className="mc-body-muted">
          The remaining half composes out of what already exists rather than needing a new primitive: a reader's need is
          a named tuple of categories × facet × modality × <strong>stance</strong> × scope level × cadence. A saved
          lens, not a facet of its own.
        </p>
        <div className="mc-storage-table-wrap">
          <table className="mc-storage-table">
            <thead>
              <tr>
                <th scope="col">Reader</th>
                <th scope="col">Categories</th>
                <th scope="col">Modality</th>
                <th scope="col">Scope</th>
                <th scope="col">Cadence</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td>On-call</td>
                <td>failures, activity</td>
                <td>glance</td>
                <td>site / realm</td>
                <td>seconds</td>
              </tr>
              <tr>
                <td>Tribe lead</td>
                <td>decisions, delivery, quality</td>
                <td>delta</td>
                <td>realm</td>
                <td>daily</td>
              </tr>
              <tr>
                <td>Exec</td>
                <td>cost, delivery, roadmap</td>
                <td>narrative</td>
                <td>world / universe</td>
                <td>weekly</td>
              </tr>
              <tr>
                <td>Auditor</td>
                <td>any</td>
                <td>storyboard</td>
                <td>any</td>
                <td>retrospective</td>
              </tr>
              <tr>
                <td>The agent itself</td>
                <td>decisions</td>
                <td>conversational</td>
                <td>own site</td>
                <td>on demand</td>
              </tr>
            </tbody>
          </table>
        </div>
        <span className="mc-provenance">source: reporting/contracts/categories.yaml · pkg/reporting/definition.go</span>
      </section>
    </>
  );
}
