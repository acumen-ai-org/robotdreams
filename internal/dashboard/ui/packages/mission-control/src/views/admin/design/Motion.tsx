export default function DesignMotion() {
  return (
    <>
      <section className="mc-ongoing-section" aria-labelledby="motion-heading">
        <h2 id="motion-heading">Motion</h2>
        <ul className="mc-doc-list">
          <li>
            <strong>Standard transition</strong> — <span className="mc-mono">160ms cubic-bezier(.2, .8, .2, 1)</span>.
            Hover the card below to feel it.
          </li>
          <li>
            <strong>Large reveals</strong> — <span className="mc-mono">320ms</span> maximum; the topbar's slide-down
            uses this curve with a 4px drop.
          </li>
          <li>
            <strong>The logo</strong> may float vertically by 4px over 4s; the face never animates and the antenna bulbs
            never flash. Under <span className="mc-mono">prefers-reduced-motion</span> the animated logo is swapped for
            a still at the asset level.
          </li>
        </ul>
        <div className="mc-specimen-row">
          <span className="mc-spec-motion-card">hover me</span>
        </div>
      </section>

      <section className="mc-ongoing-section" aria-labelledby="depth-heading">
        <h2 id="depth-heading">Surfaces and depth</h2>
        <p className="mc-section-abstract">
          Depth is small lightness shifts, not drop shadows everywhere: 1px borders come first, and the main shadow is
          reserved for menus and dialogs — this Settings dialog is wearing it now.
        </p>
        <ol className="mc-doc-list">
          <li>Page — Pearl (light) / Ink (dark)</li>
          <li>Section or card — Snow / Carbon</li>
          <li>
            Raised interactive surface — <span className="mc-mono">--surface-raised</span>
          </li>
          <li>
            Selected or tinted — <span className="mc-mono">color-mix(in srgb, var(--brand) 10%, var(--surface))</span>
          </li>
        </ol>
        <span className="mc-provenance">source: DESIGN.md — Motion · Surfaces and depth</span>
      </section>
    </>
  );
}
