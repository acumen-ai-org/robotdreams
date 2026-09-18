import { VocabularySections } from "./DesignSpecimens";
import { useVocabulary } from "../../../state/VocabularyContext";
import { adminRoute } from "../../../lib/routes";
import { Link } from "../../../components/Link";

export default function DocsHierarchy() {
  const vocab = useVocabulary();
  return (
    <>
      <section className="mc-admin-section">
        <div
          className="mc-level-ladder"
          role="img"
          aria-label="Five levels: Universe contains Worlds, which contain Realms, which contain Sites, which contain Nodes."
        >
          {[
            { name: vocab.one(0), code: "by name", note: "one only" },
            { name: vocab.one(1), code: "W01", note: "circle badge" },
            { name: vocab.one(2), code: "R0–R9", note: "carries the colour" },
            { name: vocab.one(3), code: "S01", note: "factory icon" },
            { name: vocab.one(4), code: "N000001", note: "role emoji" },
          ].map((l, i) => (
            <div className="mc-level-step" key={l.name} style={{ marginInlineStart: i * 18 }}>
              <span className="mc-level-step-name">{l.name}</span>
              <span className="mc-level-step-code mc-id-code">{l.code}</span>
              <span className="mc-level-step-note mc-body-muted">{l.note}</span>
            </div>
          ))}
        </div>
        <p className="mc-body-muted">
          Five levels, each contained by the one above. The crucial thing: this is a{" "}
          <strong>labelling convention, not a container model</strong>. Nothing in the control plane enforces that a
          depth-3 scope is a realm, and a deployment can use four levels or two. The names exist so that people and
          dashboards can agree what they are looking at — see{" "}
          <Link to={adminRoute("docs/scope")}>Scope &amp; deep links</Link> for how addressing actually works.
        </p>
      </section>

      <VocabularySections />
    </>
  );
}
