import { DEFAULT_LEVELS, useVocabulary } from "../../state/VocabularyContext";
import { NodeBadge, RealmChip, SiteBadge, UniverseIcon, WorldBadge } from "../../lib/vocabulary";

const CODE = ["—", "W01", "R4", "S017", "N000001"];

export default function VocabularyPage() {
  const vocab = useVocabulary();

  return (
    <>
      <section className="mc-admin-section">
        <p className="mc-body-muted">
          Universe → World → Realm → Site → Node is what Robot Dreams ships, but nothing in the control plane enforces
          it — the hierarchy is a labelling convention, and a scope path is just names. Rename the levels here and the
          rest of the UI follows.
        </p>
        <p className="mc-callout">
          The <strong>codes stay</strong> — <code className="mc-mono">W01</code>, <code className="mc-mono">R4</code>,{" "}
          <code className="mc-mono">S017</code>. They appear in deep links, in these docs, and in the popovers that make
          a label identifiable across both sites; re-lettering them per deployment would make one person's screenshot
          unreadable to the next. The word is presentation, the code is identity.
        </p>
      </section>

      <section className="mc-admin-section">
        <h2>Level names</h2>
        <div className="mc-vocab-form">
          {vocab.levels.map((l, i) => (
            <div className="mc-vocab-row" key={i}>
              <span className="mc-vocab-row-spec">
                {i === 0 && <UniverseIcon size={16} />}
                {i === 1 && <WorldBadge n={1} max={12} size={18} />}
                {i === 2 && <RealmChip n={4} name={l.one} />}
                {i === 3 && <SiteBadge n={17} max={120} realm={2} name={l.one} />}
                {i === 4 && <NodeBadge n={1} max={40} realm={2} role="builder" name={l.one} />}
              </span>
              <span className="mc-vocab-row-code mc-id-code">{CODE[i]}</span>
              <label className="mc-vocab-field">
                <span className="mc-detail-label">Singular</span>
                <input
                  type="text"
                  value={l.one}
                  placeholder={DEFAULT_LEVELS[i].one}
                  onChange={(e) => vocab.setLevel(i, { one: e.target.value })}
                />
              </label>
              <label className="mc-vocab-field">
                <span className="mc-detail-label">Plural</span>
                <input
                  type="text"
                  value={l.many}
                  placeholder={DEFAULT_LEVELS[i].many}
                  onChange={(e) => vocab.setLevel(i, { many: e.target.value })}
                />
              </label>
            </div>
          ))}
        </div>

        <div className="mc-admin-form-actions">
          <button
            className="mc-button mc-button-tertiary"
            type="button"
            onClick={vocab.reset}
            disabled={!vocab.isCustom}
          >
            Reset to defaults
          </button>
        </div>
      </section>

      <section className="mc-admin-section">
        <h2>Reads as</h2>
        <p className="mc-body-muted">
          A {vocab.lower(0)} contains {vocab.many(1).toLowerCase()}; each {vocab.lower(1)} contains{" "}
          {vocab.many(2).toLowerCase()}; each {vocab.lower(2)} contains {vocab.many(3).toLowerCase()}; and a{" "}
          {vocab.lower(3)} is where {vocab.many(4).toLowerCase()} live.
        </p>
        <span className="mc-provenance">stored per browser, under dream.vocabulary</span>
      </section>
    </>
  );
}
