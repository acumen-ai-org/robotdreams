const ROWS: Array<{ q: string; primitive: string; data: string; note: string; core?: boolean }> = [
  {
    q: "Are we on target?",
    primitive: "kpi_card",
    data: "kpi",
    core: true,
    note: "Actual, target, and the variance between them. A value with no target is a number, not an answer.",
  },
  {
    q: "How is it changing?",
    primitive: "timeseries",
    data: "series",
    core: true,
    note: "With the target or the previous period drawn as a reference line — a trend alone cannot be read as good or bad.",
  },
  {
    q: "Which areas perform best?",
    primitive: "bar_ranked",
    data: "table",
    core: true,
    note: "Sorted, longest first, labelled in full. The ranking is the comparison, so it is done for the reader.",
  },
  {
    q: "Why did the result change?",
    primitive: "waterfall",
    data: "table",
    core: true,
    note: "Opening total, the signed movements, closing total. The one chart that answers why rather than what.",
  },
  {
    q: "Where is attention needed?",
    primitive: "heatmap · status_matrix",
    data: "table",
    core: true,
    note: "Colour means severity here, never identity.",
  },
  {
    q: "What are the precise numbers?",
    primitive: "datagrid",
    data: "table",
    core: true,
    note: "With sparklines and variance columns where a row has history. The table should not force a trip to a chart.",
  },
  {
    q: "How does performance vary across units?",
    primitive: "small_multiples",
    data: "series",
    note: "One small chart per unit on a shared scale, instead of flipping between filtered views.",
  },
  {
    q: "What drives the outcome?",
    primitive: "scatter · decomposition_tree",
    data: "table · graph",
    note: "Two measures per unit, or the total broken down along the dimension that explains most of it.",
  },
  {
    q: "How is a process progressing?",
    primitive: "funnel · dag",
    data: "table · graph · items",
    note: "Stages narrowing, each showing what reached it and what was lost since the stage before. A dag now also draws over a plan's items, whose blocked_by edges are the graph it never had a producer for.",
  },
  {
    q: "What is the portfolio composition?",
    primitive: "stacked_bar",
    data: "table",
    note: "Bounded: past a handful of segments the reader is decoding a legend, not reading a chart.",
  },
  {
    q: "When will work happen?",
    primitive: "gantt",
    data: "spans",
    note: "Start/end bounds, milestones, dependencies.",
  },
  {
    q: "What are we going to do next?",
    primitive: "board",
    data: "items",
    note: "Items in the flow their own plan declares, left to right. The order is the argument: a pile-up is visible before a single card is read.",
  },
  {
    q: "What lands when, and in which track?",
    primitive: "roadmap_lanes",
    data: "items",
    note: "Horizons across, lanes down. Commitment is drawn as weight, not colour \u2014 colour is already spoken for by status.",
  },
  {
    q: "What is left before this is done?",
    primitive: "checklist",
    data: "items",
    note: "The degenerate plan: two states and no flow. Most work is this, and a board would be furniture around it.",
  },
  {
    q: "Will the remaining work fit the time left?",
    primitive: "burndown",
    data: "series",
    note: "Remaining against the ideal line from the plan's baseline. A series, not items: a plan is a snapshot with no item identity between instances, so the curve is published rather than derived.",
  },
  {
    q: "Where is performance happening?",
    primitive: "geomap",
    data: "table",
    note: "Only when the geography changes the decision. Otherwise a sorted bar chart says the same thing in less space.",
  },
];

const SECTIONS = [
  {
    name: "overview",
    q: "What happened?",
    holds:
      "Four to six KPIs with their targets, one trend, one breakdown. The whole answer, before any of the reasoning.",
  },
  {
    name: "performance",
    q: "How does that compare?",
    holds:
      "Actual against target and against the previous period — the comparison that turns a number into a judgement.",
  },
  {
    name: "drivers",
    q: "Why did it change?",
    holds: "Waterfall, ranked bars, or a decomposition tree: the movement attributed to what caused it.",
  },
  {
    name: "exceptions",
    q: "What needs attention?",
    holds: "The items requiring a decision or an intervention, and nothing else. This is the section a reader acts on.",
  },
  {
    name: "next",
    q: "What are we going to do about it?",
    holds:
      "The committed response \u2014 the board, the lanes, or the next horizon. Every section above establishes a finding; this is the one that answers it.",
  },
  { name: "detail", q: "What are the precise numbers?", holds: "The searchable table underneath everything above." },
];

const AVOID = [
  [
    "Gauge clusters",
    "A wall of dials says less than one sorted bar chart. Keep the gauge for a single value against a threshold that matters.",
  ],
  [
    "Pie charts past two or three slices",
    "Angle is the hardest encoding to compare. A ranked bar answers the same question and can be read.",
  ],
  [
    "Decorative 3D and rainbow palettes",
    "Colour means status or selection. A rainbow of series reads as though every series mattered equally.",
  ],
  [
    "Twenty equally prominent widgets",
    "Equal prominence claims nothing matters more than anything else, which is never true of a report someone opened for a reason.",
  ],
];

export default function DocsPrimitives() {
  return (
    <>
      <section className="mc-admin-section">
        <p className="mc-body-muted">
          Categories say what a report is <em>about</em>; modalities say how it is <em>delivered</em>. Primitives say
          how it is <em>drawn</em> — and a funnel, a waterfall or a heatmap is none of the other two. This is why the
          view switcher offers Glance and Timeline rather than Funnel: the modality is the reader's context, the
          primitive is the panel's answer to one question inside it.
        </p>
        <p className="mc-body-muted">
          The contract is the data, not the widget. Every primitive names a data kind, and a consumer that lacks the
          widget may substitute any other one rendering the same kind — see{" "}
          <code className="mc-mono">reporting/contracts/primitives.yaml</code>.
        </p>
      </section>

      <section className="mc-admin-section">
        <h2>The question decides the drawing</h2>
        <p className="mc-body-muted">
          Six of these carry most reports: <strong>kpi_card</strong>, <strong>timeseries</strong>,{" "}
          <strong>bar_ranked</strong>, <strong>waterfall</strong>, <strong>heatmap</strong> and{" "}
          <strong>datagrid</strong>. Reach past them when a question genuinely calls for it, not for variety.
        </p>
        <div className="mc-storage-table-wrap">
          <table className="mc-storage-table">
            <thead>
              <tr>
                <th scope="col">Business question</th>
                <th scope="col">Primitive</th>
                <th scope="col">Data kind</th>
                <th scope="col">Why that one</th>
              </tr>
            </thead>
            <tbody>
              {ROWS.map((r) => (
                <tr key={r.q}>
                  <td>
                    {r.q}
                    {r.core && <span className="mc-tag mc-doc-core-tag">core</span>}
                  </td>
                  <td className="mc-mono">{r.primitive}</td>
                  <td className="mc-body-muted mc-mono">{r.data}</td>
                  <td className="mc-body-muted">{r.note}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </section>

      <section className="mc-admin-section">
        <h2>The order a report is read in</h2>
        <p className="mc-body-muted">
          What happened → why → what needs attention → what should we do. A definition names a{" "}
          <code className="mc-mono">section</code> on each detail panel and the page groups itself; a definition that
          names none renders as one flat list, exactly as before. The sequence is what makes a report readable — the
          panel count never was.
        </p>
        <ol className="mc-doc-list">
          {SECTIONS.map((s) => (
            <li key={s.name}>
              <strong className="mc-mono">{s.name}</strong> — <em>{s.q}</em>
              <p className="mc-body-muted">{s.holds}</p>
            </li>
          ))}
        </ol>
      </section>

      <section className="mc-admin-section">
        <h2>What we do not draw</h2>
        <ul className="mc-doc-list">
          {AVOID.map(([what, why]) => (
            <li key={what}>
              <strong>{what}</strong>
              <p className="mc-body-muted">{why}</p>
            </li>
          ))}
        </ul>
      </section>
    </>
  );
}
