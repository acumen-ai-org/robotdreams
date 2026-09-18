import { PaletteSection, ThemeTokensSection } from "../docs/DesignSpecimens";

export default function DesignColors() {
  return (
    <>
      <PaletteSection />
      <ThemeTokensSection />
      <section className="mc-admin-section">
        <p className="mc-body-muted" style={{ maxWidth: "68ch" }}>
          Two rules carry most of the system: Powder is a fill, never a text color — Deep Blue does the interactive work
          on light backgrounds — and state is never said with color alone. Blush, amber, and coral accent; they do not
          assert.
        </p>
        <span className="mc-provenance">source: DESIGN.md · ui/src/styles.css</span>
      </section>
    </>
  );
}
