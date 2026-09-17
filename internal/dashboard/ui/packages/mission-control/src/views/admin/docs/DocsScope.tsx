export default function DocsScope() {
  return (
    <>
      <section className="mc-admin-section">
        <div
          className="mc-scope-anatomy"
          role="img"
          aria-label="A scope path: spookify slash advertising slash adtech slash ad-serving-squad, with each segment labelled universe, world, realm, site."
        >
          {[
            { seg: "spookify", level: "universe", depth: 1 },
            { seg: "advertising", level: "world", depth: 2 },
            { seg: "adtech", level: "realm", depth: 3 },
            { seg: "ad-serving-squad", level: "site", depth: 4 },
          ].map((s, i) => (
            <span className="mc-scope-anatomy-seg" key={s.seg}>
              {i > 0 && (
                <span className="mc-scope-anatomy-slash" aria-hidden="true">
                  /
                </span>
              )}
              <span className="mc-scope-anatomy-body">
                <span className="mc-scope-anatomy-name mc-mono">{s.seg}</span>
                <span className="mc-scope-anatomy-level">{s.level}</span>
              </span>
            </span>
          ))}
        </div>
        <p className="mc-body-muted">
          A scope path is slash-separated names, and the level of each segment is its depth. The names in{" "}
          <code className="mc-mono">LEVEL_NAMES</code> are <em>advisory</em>: nothing enforces that depth 3 is a realm.
          The hierarchy is a labelling convention, not a container model — which is what lets a deployment use four
          levels, or two, without the reporting engine caring.
        </p>
      </section>

      <section className="mc-admin-section">
        <h2>Prefix matching is the whole trick</h2>
        <p className="mc-body-muted">
          A report attaches at an exact scope, but a query at any ancestor sees it, because scopes match by path prefix.
          Asking for <code className="mc-mono">spookify</code> returns everything beneath it, already aggregated; asking
          for the squad returns only that squad. There is no separate roll-up table — the path <em>is</em> the index.
        </p>
        <ul className="mc-doc-list">
          <li>
            <code className="mc-mono">spookify</code> matches <code className="mc-mono">spookify/advertising/…</code>{" "}
            and every other descendant.
          </li>
          <li>
            <code className="mc-mono">spookify/advertising</code> does <strong>not</strong> match{" "}
            <code className="mc-mono">spookify/advertising-labs</code> — the boundary is a slash, not a string prefix.
          </li>
          <li>
            Where two instances of one definition exist at the same exact scope, the latest wins. Ancestors aggregate;
            they never duplicate.
          </li>
        </ul>
      </section>

      <section className="mc-admin-section">
        <h2>The URL grammar</h2>
        <p className="mc-body-muted">
          Everything the Outcomes view can show is addressable, so any state can be linked to — from the node detail
          panel, from a doc, or from outside the app entirely.
        </p>
        <div className="mc-storage-table-wrap">
          <table className="mc-storage-table">
            <thead>
              <tr>
                <th scope="col">Link</th>
                <th scope="col">Shows</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td className="mc-mono">#/outcomes</td>
                <td>Every report, all scopes</td>
              </tr>
              <tr>
                <td className="mc-mono">#/outcomes/spookify/advertising</td>
                <td>Everything under one world, aggregated</td>
              </tr>
              <tr>
                <td className="mc-mono">#/outcomes/spookify?category=delivery,failures</td>
                <td>Two categories at one scope</td>
              </tr>
              <tr>
                <td className="mc-mono">#/outcomes/spookify?report=incident</td>
                <td>One definition&rsquo;s drill-down page</td>
              </tr>
              <tr>
                <td className="mc-mono">#/outcomes/spookify?selected=node:music-playback-01</td>
                <td>The same page with that node open in the side panel</td>
              </tr>
              <tr>
                <td className="mc-mono">#/control-plane/spookify/music</td>
                <td>The fleet, focused on one world</td>
              </tr>
              <tr>
                <td className="mc-mono">#/settings/docs/scope</td>
                <td>This page</td>
              </tr>
            </tbody>
          </table>
        </div>
        <p className="mc-body-muted">
          The path says <strong>where</strong> you are: the view, named as this interface names it, then the scope one
          segment per level. The query says <strong>how you are reading it</strong> —{" "}
          <code className="mc-mono">category</code>, <code className="mc-mono">stance</code>,{" "}
          <code className="mc-mono">window</code>, <code className="mc-mono">report</code> — spelled the way the product
          spells the concept.
        </p>
        <p className="mc-body-muted">
          Two things do not fit that shape and say so. <code className="mc-mono">also</code> carries the extra scopes of
          a multi-select, because a path can only be one place. And <code className="mc-mono">selected</code> carries
          what the side panel is showing, because that is part of what you are looking at — a link to &ldquo;this
          node&rdquo; that arrived with nothing selected would not be a link to this node.
        </p>
        <p className="mc-body-muted">
          Values stay raw wherever the character is legal, so slashes in a scope and the{" "}
          <code className="mc-mono">:</code> of a selection survive intact. It all lives in the fragment: nothing after{" "}
          <code className="mc-mono">#</code> reaches the server, so deep links work from a static file server with no
          rewrite rule. An unrecognised link falls back to the Control Plane, and unknown category names are dropped
          rather than breaking the view.
        </p>
        <span className="mc-provenance">
          source: ui/src/lib/routes.ts · ui/src/lib/selection.ts · docs/dashboard.md
        </span>
      </section>
    </>
  );
}
