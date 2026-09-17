import { STANCES, STANCE_QUESTIONS } from "../../../lib/routes";

const MEASURED_AGAINST: Record<string, string> = {
  operational: "a threshold, an SLA, or a steady state",
  strategic: "a plan, a target, an intended end state",
  diagnostic: "nothing — the record is the answer",
};

const READER_IS: Record<string, string> = {
  operational: "keeping a system alive, and acting from inside it",
  strategic: "steering, and acting on it",
  diagnostic: "finding out, and not yet acting at all",
};

const SECTION_AFFINITY: Array<[string, string, string]> = [
  ["overview", "every stance", "What happened? — the answer before any reasoning, which every reader wants first."],
  ["performance", "strategic", "Actual against target and against the previous period."],
  ["drivers", "strategic · operational", "Why it moved — attribution serves both steering and running."],
  ["exceptions", "operational", "The items needing a decision or an intervention now."],
  ["detail", "diagnostic", "The searchable matrix underneath everything above."],
];

const CLASSIFICATION: Array<[string, string, string]> = [
  [
    "logs",
    "diagnostic",
    "The pure evidence report — the only definition with no target on any KPI. A log has no steady state to hold and no end state to reach.",
  ],
  ["performance", "operational", "Latency and throughput against thresholds. The canonical operational report."],
  ["roadmap", "strategic", "Milestones against plan."],
  ["portfolio", "strategic", "Attainment and the bets at risk."],
  ["budget-variance", "strategic", "Plan against actual against forecast — strategic by definition."],
  ["company-scorecard", "strategic", "The goal tree at company altitude."],
  ["subscriber-growth", "strategic", "Growth against plan."],
  [
    "cost",
    "operational · strategic",
    "Run-rate against a ceiling, and burn against a budget. The clearest proof that stance is orthogonal to category: one report, one number, two readings.",
  ],
  ["incident", "operational · diagnostic", "What is broken now, and the forensic record of how."],
  [
    "workflow",
    "operational · strategic",
    "What is stuck right now, and whether the process is getting where it should.",
  ],
];

export default function DocsStances() {
  return (
    <>
      <section className="mc-admin-section">
        <p className="mc-body-muted">
          The contract had four axes — <strong>subject</strong> (categories), <strong>shape</strong> (facets),{" "}
          <strong>delivery</strong> (modalities) and <strong>drawing</strong> (primitives). None of them answers a
          question every reader brings to a report <em>before</em> they look at it: am I keeping this running, am I
          steering it, or am I finding out what happened?
        </p>
        <p className="mc-body-muted">
          A <strong>stance</strong> is that footing, and what separates the three is{" "}
          <strong>what the number is measured against</strong>. A definition declares which stances it can serve; a
          reader picks one. That is the same grammar modalities use — the report does not get a vote about how it is
          read.
        </p>
      </section>

      <section className="mc-admin-section">
        <h2>The three</h2>
        <div className="mc-storage-table-wrap">
          <table className="mc-storage-table">
            <thead>
              <tr>
                <th scope="col">Stance</th>
                <th scope="col">Question</th>
                <th scope="col">Measured against</th>
                <th scope="col">The reader is…</th>
              </tr>
            </thead>
            <tbody>
              {STANCES.map((s) => (
                <tr key={s}>
                  <td>
                    <span className="mc-tag mc-tag-stance">{s}</span>
                  </td>
                  <td>{STANCE_QUESTIONS[s]}</td>
                  <td className="mc-body-muted">{MEASURED_AGAINST[s]}</td>
                  <td className="mc-body-muted">{READER_IS[s]}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
        <p className="mc-body-muted">
          Two of these are the strategic/operational split every methodology ends up naming. The third earns its place
          from the data rather than from symmetry: <code className="mc-mono">logs</code> and{" "}
          <code className="mc-mono">activity</code> are the only definitions in the library with no target on any KPI,
          because a log has nothing to be measured against. Without a diagnostic stance they would have to claim one.
        </p>
      </section>

      <section className="mc-admin-section">
        <h2>Why not a fourth</h2>
        <ul className="mc-doc-list">
          <li>
            <strong>Assurance / compliance</strong>
            <p className="mc-body-muted">
              Auditability is a property of every report — as a filter value it produces nonsense ("is this delivery
              report auditable?" — it had better be). The subject half is already the{" "}
              <code className="mc-mono">compliance</code> definition, and the reader half is already a row in the reader
              lens.
            </p>
          </li>
          <li>
            <strong>Decision / attention</strong>
            <p className="mc-body-muted">
              Collides twice: with the <code className="mc-mono">decisions</code> category and with the{" "}
              <code className="mc-mono">exceptions</code> section. An axis whose value duplicates a section name is not
              an axis.
            </p>
          </li>
        </ul>
      </section>

      <section className="mc-admin-section">
        <h2>The reading order was already a stance projection</h2>
        <p className="mc-body-muted">
          This is what makes the axis cheap rather than a second layout engine. The panel sections in{" "}
          <code className="mc-mono">facets.yaml</code> already sort by what a reader is doing, so choosing a stance
          re-weights sections the report page groups anyway.
        </p>
        <div className="mc-storage-table-wrap">
          <table className="mc-storage-table">
            <thead>
              <tr>
                <th scope="col">Section</th>
                <th scope="col">Serves</th>
                <th scope="col">Because</th>
              </tr>
            </thead>
            <tbody>
              {SECTION_AFFINITY.map(([section, serves, why]) => (
                <tr key={section}>
                  <td className="mc-mono">{section}</td>
                  <td>{serves}</td>
                  <td className="mc-body-muted">{why}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
        <p className="mc-body-muted">
          A panel may override its section's affinity — "bets at risk" sits in{" "}
          <code className="mc-mono">exceptions</code> but is a strategic reading — and the server resolves that, so the
          affinity table lives once, beside the contract.
        </p>
        <p className="mc-body-muted">
          <strong>The stance switch re-orders and dims; it never hides.</strong> A reader who switches stance onto a
          blank page concludes the report is broken, not that they asked the wrong question of it.
        </p>
      </section>

      <section className="mc-admin-section">
        <h2>How definitions classify</h2>
        <p className="mc-body-muted">
          Every definition in the shipped library declares its stances. The schema keeps the field optional — a
          definition with no opinion is readable from any footing, and that is a real answer — but the library is the
          reference implementation and is held to a higher bar.
        </p>
        <div className="mc-storage-table-wrap">
          <table className="mc-storage-table">
            <thead>
              <tr>
                <th scope="col">Definition</th>
                <th scope="col">Stances</th>
                <th scope="col">Why</th>
              </tr>
            </thead>
            <tbody>
              {CLASSIFICATION.map(([def, stances, why]) => (
                <tr key={def}>
                  <td className="mc-mono">{def}</td>
                  <td>{stances}</td>
                  <td className="mc-body-muted">{why}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </section>

      <section className="mc-admin-section">
        <h2>One asymmetry worth knowing</h2>
        <p className="mc-body-muted">
          The stance filter and the category filter treat an absent declaration in <em>opposite</em> ways, and the
          difference is not an oversight. A category is a claim about subject matter, so its absence means "not about
          that" and the report is filtered out. A stance is a claim about how the numbers may be judged, so its absence
          means "no opinion" — and a report with no opinion passes every stance filter. Excluding it would hide exactly
          the general-purpose reports a reader most wants.
        </p>
        <span className="mc-provenance">source: reporting/contracts/stances.yaml · pkg/reporting/definition.go</span>
      </section>
    </>
  );
}
