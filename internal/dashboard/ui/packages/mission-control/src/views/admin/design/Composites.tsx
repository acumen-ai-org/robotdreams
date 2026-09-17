import { CompositeSpecimens } from "../docs/DesignSpecimens";

export default function DesignComposites() {
  return (
    <>
      <CompositeSpecimens />
      <section className="mc-ongoing-section" aria-labelledby="summary-heading">
        <h2 id="summary-heading">The panel header's two boxes</h2>
        <p className="mc-section-abstract">
          Whatever is selected opens in the side panel behind the same header: what it is on the left, where it sits on
          the right. One shape for a place, a node and a report, so the reader does not relearn the box when the
          selection changes.
        </p>
        <div className="mc-panel-boxes" style={{ maxWidth: 420 }}>
          <div className="mc-facts-box">
            <div className="mc-facts-head">
              <span className="mc-facts-icon" aria-hidden="true">
                🔨
              </span>
              <span className="mc-facts-caption">builder</span>
            </div>
            <div className="mc-facts-figure is-word is-ok">connected</div>
            <ul className="mc-facts-lines">
              <li className="mc-facts-line">
                reports to <strong>dev-vp</strong>
              </li>
            </ul>
          </div>
          <div className="mc-hier-box">
            <div className="mc-hier-part">
              <span className="mc-hier-caption">Answers to</span>
              <span className="mc-hier-list">
                <button type="button" className="mc-hier-item">
                  dev-vp
                </button>
              </span>
            </div>
            <div className="mc-hier-part">
              <span className="mc-hier-caption">Nodes</span>
              <span className="mc-hier-list">
                <button type="button" className="mc-hier-item">
                  build-01
                </button>
                <button type="button" className="mc-hier-item">
                  build-02
                </button>
              </span>
            </div>
          </div>
        </div>
        <span className="mc-provenance">source: ui/src/components/detail/FactsBox.tsx, HierarchyBox.tsx</span>
      </section>
    </>
  );
}
