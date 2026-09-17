const SPACING = [4, 8, 12, 16, 24, 32, 48, 64];
const RADII: Array<[number, string]> = [
  [6, "controls and small blocks"],
  [10, "cards, panels, and menus"],
  [24, "feature panels on web pages"],
];

export default function DesignSpacing() {
  return (
    <>
      <section className="mc-ongoing-section" aria-labelledby="spacing-heading">
        <h2 id="spacing-heading">Spacing scale</h2>
        <p className="mc-section-abstract">
          Every gap and padding in the app is one of these. When two values both look right, take the smaller.
        </p>
        <div className="mc-spec-scale" role="img" aria-label={"Spacing steps: " + SPACING.join(", ") + " pixels"}>
          {SPACING.map((n) => (
            <span className="mc-spec-scale-step" key={n}>
              <span className="mc-spec-scale-block" style={{ width: n, height: n }}></span>
              <span className="mc-mono mc-spec-scale-n">{n}</span>
            </span>
          ))}
        </div>
      </section>

      <section className="mc-ongoing-section" aria-labelledby="radius-heading">
        <h2 id="radius-heading">Radius</h2>
        <p className="mc-section-abstract">
          Rounding is restrained everywhere — when in doubt, less. Pill shape is reserved for tags and compact filter
          controls.
        </p>
        <div className="mc-specimen-row">
          {RADII.map(([r, what]) => (
            <span className="mc-spec-radius" key={r} style={{ borderRadius: r }}>
              <span className="mc-mono">{r}px</span> {what}
            </span>
          ))}
          <span className="mc-spec-radius" style={{ borderRadius: 999 }}>
            <span className="mc-mono">pill</span> tags and status pills
          </span>
        </div>
      </section>

      <section className="mc-ongoing-section" aria-labelledby="tiers-heading">
        <h2 id="tiers-heading">Width tiers</h2>
        <ul className="mc-doc-list">
          <li>
            <strong>Web pages</strong> — max content width <span className="mc-mono">1440px</span>.
          </li>
          <li>
            <strong>App surfaces</strong> (Mission Control) — fully fluid; only the page gutters remain at the edges (
            <span className="mc-mono">20px</span>, growing to <span className="mc-mono">40px</span> above 768px).
          </li>
          <li>
            <strong>Reading width</strong> — prose never spans the tier: <span className="mc-mono">720px</span> / 68ch.
          </li>
        </ul>
        <span className="mc-provenance">source: DESIGN.md — Layout</span>
      </section>
    </>
  );
}
