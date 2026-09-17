import { VIEW_VARIANTS } from "../../../lib/variants";

const MODALITIES = [
  {
    name: "narrative",
    what: "Synthesized plain language: TL;DR banners and briefings — what happened, why, what to do.",
  },
  { name: "glance", what: "Sub-second signals: traffic lights, scorecards, ambient badges that need no visit." },
  { name: "delta", what: "Difference-first: only what changed against a baseline, plus direction and speed." },
  { name: "spatial", what: "Maps that match the mental model: dependency webs, drill-down trees, blast radius." },
  { name: "storyboard", what: "Linear consumption: replay and time-travel through a run, merged event feeds." },
  { name: "conversational", what: "On-demand: Q&A and debriefs, answered from the report as ground truth." },
  {
    name: "board",
    what: "Committed work laid out as work: columns in flow order, swimlanes, horizons \u2014 scanned for pile-up and blockage rather than read for values.",
  },
];

export default function DocsModalities() {
  const views = VIEW_VARIANTS.reporting;
  return (
    <>
      <section className="mc-admin-section">
        <p className="mc-body-muted">
          A modality is <em>how</em> report data is ingested, chosen by where the reader is — “the report doesn't get a
          vote about where it is read”. A definition declares which modalities each facet can serve; the consumer picks.
          This is also the honest name for what the UI calls a view.
        </p>
        <ul className="mc-doc-list">
          {MODALITIES.map((m) => (
            <li key={m.name}>
              <strong className="mc-mono">{m.name}</strong> — {m.what}
            </li>
          ))}
        </ul>
      </section>

      <section className="mc-admin-section">
        <h2>What is actually built</h2>
        <p className="mc-body-muted">
          These were numbered “Option 1–6” while they were compared, which said nothing about what any of them was and
          made them useless as link targets. Naming them after the modality they serve exposed two facts the numbering
          hid: there are only <strong>four</strong> built views, one per modality, and two modalities from the contract
          were never built at all.
        </p>
        <div className="mc-storage-table-wrap">
          <table className="mc-storage-table">
            <thead>
              <tr>
                <th scope="col">View</th>
                <th scope="col">Modality &amp; component</th>
                <th scope="col">Status</th>
              </tr>
            </thead>
            <tbody>
              {views.map((v) => (
                <tr key={v.id}>
                  <td>{v.label}</td>
                  <td className="mc-body-muted">{v.hint}</td>
                  <td>
                    {v.built === false ? (
                      <span className="mc-tag">not built</span>
                    ) : (
                      <span className="mc-body-muted">shipped</span>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
        <p className="mc-body-muted">
          Every modality in the contract now has a view. Two of them &mdash; Narrative and Ask &mdash; are templated
          rather than model-backed, and say so on their own pages: the contract requires that degradation to be explicit
          rather than silent. The <span className="mc-mono">built: false</span> mechanism stays in{" "}
          <span className="mc-mono">variants.ts</span> for the next modality that is declared before it is drawn, which
          is the state <span className="mc-mono">board</span> was in until the plan facet existed to feed it.
        </p>
        <span className="mc-provenance">source: reporting/contracts/modalities.yaml · ui/src/lib/variants.ts</span>
      </section>
    </>
  );
}
