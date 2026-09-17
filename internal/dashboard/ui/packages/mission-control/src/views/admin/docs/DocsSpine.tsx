export default function DocsSpine() {
  return (
    <>
      <section className="mc-admin-section">
        <div
          className="mc-spine-diagram"
          role="img"
          aria-label="Nodes call the control plane, which sits on two swappable primitives: messaging and storage."
        >
          <div className="mc-spine-row mc-spine-nodes">
            <span className="mc-spine-node">⬡ node</span>
            <span className="mc-spine-node">⬡ node</span>
            <span className="mc-spine-node">⬡ node</span>
          </div>
          <div className="mc-spine-arrow" aria-hidden="true">
            <span>imports as a library</span>
          </div>
          <div className="mc-spine-row">
            <div className="mc-spine-box mc-spine-control">
              <span className="mc-spine-box-title">Control plane</span>
              <span className="mc-spine-box-sub">org chart · routing · reports · dashboard</span>
            </div>
          </div>
          <div className="mc-spine-arrow" aria-hidden="true">
            <span>depends only on the interfaces</span>
          </div>
          <div className="mc-spine-row mc-spine-primitives">
            <div className="mc-spine-box mc-spine-primitive">
              <span className="mc-spine-box-title">Messaging</span>
              <span className="mc-spine-box-sub">embedded · temporal · webhook</span>
              <span className="mc-spine-swap">swappable</span>
            </div>
            <div className="mc-spine-box mc-spine-primitive">
              <span className="mc-spine-box-title">Storage</span>
              <span className="mc-spine-box-sub">localfs · s3-compatible</span>
              <span className="mc-spine-swap">swappable</span>
            </div>
          </div>
        </div>
      </section>

      <section className="mc-admin-section">
        <h2>Why only two</h2>
        <p className="mc-body-muted">
          Agents need to talk to each other and to remember things. That is the whole of it. Everything else — the org
          chart, the reporting library, this dashboard — is built <em>on</em> those two, not beside them. Keeping the
          primitive count at two is what makes the implementations genuinely swappable: a deployment can run the
          embedded messaging and a local filesystem, or Temporal and S3, and nothing above notices.
        </p>
      </section>

      <section className="mc-admin-section">
        <h2>Nodes are libraries, not services</h2>
        <p className="mc-body-muted">
          A node imports the client and calls it. It is not a process the control plane starts, supervises, or owns —
          which is why an agent written in anything can join by speaking the same HTTP API, and why the control plane
          can be restarted without taking the fleet down with it. The org chart is a <em>description</em> of who reports
          to whom, not a runtime dependency graph.
        </p>
      </section>

      <section className="mc-admin-section">
        <h2>What the control plane actually holds</h2>
        <ul className="mc-doc-list">
          <li>
            <strong>The org chart</strong> — which nodes exist, what they are, and who they report to.
          </li>
          <li>
            <strong>Routing</strong> — messages between nodes, over whichever messaging primitive is configured.
          </li>
          <li>
            <strong>The reporting library</strong> — definitions loaded from YAML, instances deposited by nodes, and the
            aggregation that rolls them up a scope path.
          </li>
          <li>
            <strong>This dashboard</strong> — a plain client of the same HTTP API everything else uses, with no
            privileged back door.
          </li>
        </ul>
        <span className="mc-provenance">source: docs/vision/ · pkg/messaging/ · pkg/storage/</span>
      </section>
    </>
  );
}
