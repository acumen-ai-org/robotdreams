import { TypeSection } from "../docs/DesignSpecimens";

const SCALE: Array<[string, string, string]> = [
  ["Display", "clamp(3rem, 7vw, 6.5rem) / 0.98", "700"],
  ["H1", "clamp(2.5rem, 5vw, 4.5rem) / 1.02", "700"],
  ["H2", "clamp(2rem, 3vw, 3rem) / 1.1", "650"],
  ["H3", "1.375rem / 1.25", "650"],
  ["Body large", "1.125rem / 1.7", "400"],
  ["Body", "1rem / 1.65", "400"],
  ["Label", "0.875rem / 1.3", "600"],
];

export default function DesignTypography() {
  return (
    <>
      <TypeSection />
      <section className="mc-ongoing-section" aria-labelledby="type-scale-heading">
        <h2 id="type-scale-heading">The scale</h2>
        <div className="mc-storage-table-wrap" style={{ maxWidth: 560 }}>
          <table className="mc-storage-table">
            <thead>
              <tr>
                <th scope="col">Style</th>
                <th scope="col">Size / line-height</th>
                <th scope="col">Weight</th>
              </tr>
            </thead>
            <tbody>
              {SCALE.map(([style, size, weight]) => (
                <tr key={style}>
                  <td>{style}</td>
                  <td className="mc-mono">{size}</td>
                  <td className="mc-mono">{weight}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
        <p className="mc-body-muted" style={{ maxWidth: "68ch" }}>
          Headlines short and slightly tight; body under 68ch; sentence case everywhere except tiny technical tags. Mono
          carries every number the system asserts — KPIs, counts, timestamps, identifiers — always with tabular-nums,
          never for prose.
        </p>
        <span className="mc-provenance">source: DESIGN.md — Typography</span>
      </section>
    </>
  );
}
