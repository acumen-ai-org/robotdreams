import { mcVariant, statusDotClass } from "../../../components/shared";

const FACETS: Array<{ name: string; question: string; holds: string }> = [
  {
    name: "summary",
    question: "How are you doing?",
    holds: "status · up to 4 KPIs · sparkline · one headline sentence",
  },
  {
    name: "detail",
    question: "Show me everything.",
    holds: "ordered panels (title + primitive + data) · drilldown links",
  },
  {
    name: "timeline",
    question: "When did things happen?",
    holds: "events {t, type, severity, label} · spans · milestones",
  },
  {
    name: "pulse",
    question: "What is happening right now?",
    holds: "the live subset of events · a producer-side rate limit",
  },
  { name: "media", question: "Say it as a briefing.", holds: "script archetypes · pacing · personas · storyboard" },
  { name: "conversation", question: "Answer my questions.", holds: "which data is grounding · example prompts" },
  {
    name: "plan",
    question: "What are we going to do?",
    holds:
      "ordered states · lanes · horizons · commitments · wip limits · items {id, title, state, owner, due, size, blocked_by} · a baseline",
  },
];

const POLICIES: Array<{ name: string; applies: string; what: string }> = [
  { name: "worst", applies: "status", what: "critical beats warn beats ok; missing instances ignored" },
  { name: "sum / avg / min / max", applies: "kpis", what: "the arithmetic you would expect" },
  {
    name: "p50 / p95",
    applies: "kpis",
    what: "percentiles across contributing instances — for latencies and lead times",
  },
  { name: "latest", applies: "kpis, headline", what: "the value from the most recently produced instance" },
  {
    name: "count",
    applies: "kpis, items",
    what: "how many instances contributed; for a plan, the column totals with no cards at all",
  },
  {
    name: "merge",
    applies: "timeline, pulse, series, items",
    what: "chronological merge, each item tagged with its origin scope",
  },
  {
    name: "sample(n)",
    applies: "pulse, timeline, items",
    what: "merge, then keep at most n — severity-weighted, so failures survive last; for a plan, n per column",
  },
  {
    name: "top(n)",
    applies: "headline, tables, items",
    what: "keep the n highest-ranked items by severity and recency",
  },
  {
    name: "synthesize",
    applies: "headline",
    what: "regenerate a summary over the child summaries; degrades to top(1) with no model",
  },
];

const ROLLUP = {
  children: [
    { scope: "deploy-squad", status: "ok", deploys: 12, failed: 0, lead: 38 },
    { scope: "infra-squad", status: "warn", deploys: 5, failed: 1, lead: 51 },
    { scope: "search-squad", status: "ok", deploys: 7, failed: 0, lead: 44 },
  ],
  rolled: { status: "warn", deploys: 24, failed: 1, lead: 44 },
};

export function ReportStructure() {
  return (
    <>
      <section className="mc-ongoing-section" aria-labelledby="reports-heading">
        <h2 id="reports-heading">How reports are structured</h2>
        <p className="mc-section-abstract">
          Reports are books with a standard binding. A <strong>definition</strong> is the type — declared once in YAML,
          registered at startup. An <strong>instance</strong> is one book: data a node deposited at one scope, at one
          moment. Everything that reads reports — this dashboard, a timeline, a media generator — is written against the
          facet contracts alone. Nothing in the reading path knows any report by name, which is exactly the test: if
          rolling a report up needs report-specific code, the contract has leaked.
        </p>

        <div className="mc-principle-grid">
          <div className="mc-principle-card">
            <span className="mc-principle-step">1 · Define</span>
            <p>
              A <span className="mc-mono">ReportDefinition</span> declares its categories, the data it carries, which
              facets it exposes, and how it aggregates. It ships as a file, so a report type is reviewable, diffable,
              and copyable.
            </p>
          </div>
          <div className="mc-principle-card">
            <span className="mc-principle-step">2 · Produce</span>
            <p>
              Any connected node POSTs an instance at a scope path. The server validates it against the definition's
              data contract and rejects mismatches — a producer cannot invent fields the type never promised.
            </p>
          </div>
          <div className="mc-principle-card">
            <span className="mc-principle-step">3 · Read</span>
            <p>
              Consumers ask for a facet at a scope. The registry picks the newest instance per exact scope, rolls the
              rest up by policy, and hands back the shape the facet promised.
            </p>
          </div>
        </div>

        <h3 className="mc-ongoing-subhead">What a definition looks like</h3>
        <p className="mc-section-abstract">
          This is the <span className="mc-mono">delivery</span> template from{" "}
          <span className="mc-mono">reporting/library/</span>, trimmed to its spine. Read it top-down: what it is, where
          it attaches, how it rolls up, what data it carries, and what each facet exposes.
        </p>
        <pre className="mc-code-block" aria-label="Example report definition in YAML">{`kind: ReportDefinition
name: delivery
categories: [delivery]

scope:
  attach: [site, realm]        # the depths this report is produced at
  aggregation:                 # how children become one view (see below)
    status: worst
    headline: synthesize
    kpis:
      deploys: sum
      failed_deploys: sum
      lead_time_minutes: p50
    timeline: merge
    pulse: sample(50)

data:                          # the contract a producer must satisfy
  kpis:
    - { name: deploys, unit: count, window: 24h }
    - { name: failed_deploys, unit: count, window: 24h }
    - { name: lead_time_minutes, unit: minutes }
  series:
    - { name: deploy_frequency, unit: per-hour }
  events:
    - { type: deploy.started,  severity: info }
    - { type: deploy.finished, severity: info }
    - { type: deploy.failed,   severity: critical }

facets:
  summary:                     # the tile in an aggregate dashboard
    status: { from: failed_deploys, warn_at: 1, critical_at: 3, direction: above }
    kpis: [deploys, failed_deploys, lead_time_minutes]
    sparkline: deploy_frequency
    headline: "{{deploys}} deploys, {{failed_deploys}} failed"
  timeline:                    # the same data, arranged in time
    events: [deploy.started, deploy.finished, deploy.failed]
    spans:
      - { name: deploy, start: deploy.started, end: deploy.finished }
  detail:                      # the report's own page
    panels:
      - { title: Deploy frequency, primitive: timeseries, data: series/deploy_frequency }
    drilldown: [incident, logs]
  pulse:                       # what streams live
    events: [deploy.failed]
    rate_limit: 30/min`}</pre>

        <h3 className="mc-ongoing-subhead">…and what an instance looks like</h3>
        <p className="mc-section-abstract">
          One book, deposited by one node. Note what it does <em>not</em> contain: no layout, no colors, no prose about
          what the numbers mean. Presentation is the reader's business; the producer only asserts facts.
        </p>
        <pre className="mc-code-block" aria-label="Example report instance in JSON">{`{
  "definition": "delivery",
  "scope": "spookify/music/platform-engineering/deploy-squad",
  "produced_at": "2026-08-26T09:14:02Z",
  "kpis": [
    { "name": "deploys", "value": 12, "unit": "count" },
    { "name": "failed_deploys", "value": 0, "unit": "count" },
    { "name": "lead_time_minutes", "value": 38, "unit": "minutes" }
  ],
  "series": [ { "name": "deploy_frequency", "points": [ … ] } ],
  "events": [
    { "t": "2026-08-26T08:51:11Z", "type": "deploy.started",  "severity": "info",
      "label": "deploying playback-gateway v2.7.2" },
    { "t": "2026-08-26T08:58:40Z", "type": "deploy.finished", "severity": "info",
      "label": "playback-gateway v2.7.2 live" }
  ]
}`}</pre>

        <h3 className="mc-ongoing-subhead">The seven facets</h3>
        <p className="mc-section-abstract">
          A definition exposes any subset. Each facet answers one question, and each has one data shape a consumer can
          rely on — presentation (the <span className="mc-mono">primitive</span>) is only ever a hint.
        </p>
        <div className="mc-storage-table-wrap">
          <table className="mc-storage-table">
            <thead>
              <tr>
                <th scope="col">Facet</th>
                <th scope="col">Answers</th>
                <th scope="col">Carries</th>
              </tr>
            </thead>
            <tbody>
              {FACETS.map((f) => (
                <tr key={f.name}>
                  <td className="mc-mono">{f.name}</td>
                  <td>{f.question}</td>
                  <td className="mc-body-muted">{f.holds}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
        <p className="mc-section-abstract">
          The minimal legal report is a name — a clickable box that says nothing yet. The degenerate timeline is a
          single event: the report's own creation. The degenerate plan is a checklist: two states and no flow.
          Everything above that is opt-in.
        </p>
      </section>
    </>
  );
}

export function ReportAggregation() {
  return (
    <>
      <section className="mc-ongoing-section" aria-labelledby="aggregation-heading">
        <h2 id="aggregation-heading">How aggregation works</h2>
        <p className="mc-section-abstract">
          A view at any scope is built from every instance at or beneath it. Two rules do the work. First,{" "}
          <strong>the newest instance per exact scope wins</strong> — a squad that reported twice today counts once, and
          the superseded instance stays queryable in the timeline rather than double-counting. Second, the survivors are
          folded together <strong>field by field, by the policy the definition declared</strong>. The consumer never
          needs to know what the report means: it reads the policy and applies it.
        </p>

        <h3 className="mc-ongoing-subhead">A worked example</h3>
        <p className="mc-section-abstract">
          Three squads each deposit a <span className="mc-mono">delivery</span> instance. Here is the view their tribe
          gets, and why each number is what it is:
        </p>
        <div className="mc-storage-table-wrap">
          <table className="mc-storage-table">
            <thead>
              <tr>
                <th scope="col">Scope</th>
                <th scope="col">status</th>
                <th scope="col">deploys</th>
                <th scope="col">failed_deploys</th>
                <th scope="col">lead_time_minutes</th>
              </tr>
            </thead>
            <tbody>
              {ROLLUP.children.map((c) => (
                <tr key={c.scope}>
                  <td className="mc-mono">…/{c.scope}</td>
                  <td>
                    <span className={"mc-status-badge " + mcVariant(c.status)}>
                      <span className={"mc-status-dot " + statusDotClass(c.status)} aria-hidden="true"></span>
                      <span className="mc-status-text">{c.status}</span>
                    </span>
                  </td>
                  <td className="mc-mono">{c.deploys}</td>
                  <td className="mc-mono">{c.failed}</td>
                  <td className="mc-mono">{c.lead}</td>
                </tr>
              ))}
              <tr className="mc-rollup-row">
                <td className="mc-mono">
                  <strong>platform-engineering</strong>
                </td>
                <td>
                  <span className={"mc-status-badge " + mcVariant(ROLLUP.rolled.status)}>
                    <span className={"mc-status-dot " + statusDotClass(ROLLUP.rolled.status)} aria-hidden="true"></span>
                    <span className="mc-status-text">{ROLLUP.rolled.status}</span>
                  </span>
                </td>
                <td className="mc-mono">{ROLLUP.rolled.deploys}</td>
                <td className="mc-mono">{ROLLUP.rolled.failed}</td>
                <td className="mc-mono">{ROLLUP.rolled.lead}</td>
              </tr>
              <tr className="mc-rollup-why">
                <td className="mc-body-muted">policy</td>
                <td className="mc-body-muted mc-mono">worst</td>
                <td className="mc-body-muted mc-mono">sum</td>
                <td className="mc-body-muted mc-mono">sum</td>
                <td className="mc-body-muted mc-mono">p50</td>
              </tr>
            </tbody>
          </table>
        </div>
        <p className="mc-section-abstract">
          Note the last column: lead time is a <span className="mc-mono">p50</span>, not a sum and not an average —
          summing durations would be meaningless and averaging would let one bad squad hide behind two good ones. The
          definition chose the statistic that survives being rolled up. That choice is the whole point of declaring
          aggregation per field.
        </p>

        <h3 className="mc-ongoing-subhead">The policies</h3>
        <div className="mc-storage-table-wrap">
          <table className="mc-storage-table">
            <thead>
              <tr>
                <th scope="col">Policy</th>
                <th scope="col">Applies to</th>
                <th scope="col">What it does</th>
              </tr>
            </thead>
            <tbody>
              {POLICIES.map((p) => (
                <tr key={p.name}>
                  <td className="mc-mono">{p.name}</td>
                  <td className="mc-body-muted mc-mono">{p.applies}</td>
                  <td>{p.what}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>

        <h3 className="mc-ongoing-subhead">Two deliberate degradations</h3>
        <p className="mc-section-abstract">Both exist so a universe-level view stays honest rather than pretending.</p>
        <div className="mc-principle-grid">
          <div className="mc-principle-card">
            <span className="mc-principle-step">synthesize → top(1)</span>
            <p>
              Regenerating a headline over child headlines needs a model behind the deployment. Where there is none —
              this reference server included — it falls back to the single highest-ranked child headline. The
              degradation is explicit in the contract, never a silent blank.
            </p>
          </div>
          <div className="mc-principle-card">
            <span className="mc-principle-step">sampling is severity-weighted</span>
            <p>
              A universe rolling up thousands of events cannot show them all, so{" "}
              <span className="mc-mono">sample(n)</span> keeps at most n. It drops <span className="mc-mono">info</span>{" "}
              first and criticals last: the view thins out, but the failures are the last thing to disappear.
            </p>
          </div>
        </div>

        <p className="mc-section-abstract">
          One more consequence worth stating plainly: scope paths are <strong>labels, not containers</strong>. Nothing
          enforces that a site sits inside a realm — the hierarchy is a naming convention that aggregation reads by
          prefix. A lone laptop agent reporting at a one-segment scope is a complete, valid deployment, and the same
          policies roll it up the day it grows a second node.
        </p>
      </section>
    </>
  );
}
