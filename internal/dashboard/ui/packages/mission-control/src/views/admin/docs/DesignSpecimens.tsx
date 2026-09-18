import { StatusBadge, KpiStat } from "../../../components/shared";
import { Sparkline } from "../../../panels/Sparkline";
import {
  entityCode,
  nodeCode,
  NodeBadge,
  NODE_ROLE_EMOJI,
  NODE_DEFAULT_EMOJI,
  RealmChip,
  REALM_COLORS,
  SiteBadge,
  UniverseIcon,
  WorldBadge,
} from "../../../lib/vocabulary";

const PALETTE: Array<{ name: string; hex: string; token: string; role: string }> = [
  { name: "Ink", hex: "#080A0F", token: "--page (dark)", role: "Dark page background" },
  { name: "Carbon", hex: "#10141D", token: "--surface (dark)", role: "Dark elevated surface" },
  { name: "Slate", hex: "#1A2230", token: "--surface-soft (dark)", role: "Dark border and tertiary surface" },
  { name: "Pearl", hex: "#F6F3F8", token: "--page (light)", role: "Light page background, dark-theme text" },
  { name: "Snow", hex: "#FFFFFF", token: "--surface (light)", role: "Light elevated surface" },
  { name: "Powder", hex: "#A9D9F2", token: "--brand", role: "Brand primary — fills only on light" },
  { name: "Sky", hex: "#7FCBEE", token: "--brand-strong (dark)", role: "Interactive accent" },
  { name: "Deep Blue", hex: "#174F7D", token: "--brand-strong (light)", role: "Interactive text and strokes on light" },
  { name: "Blush", hex: "#EBCFDC", token: "--secondary-fill", role: "Secondary brand accent, tints" },
  { name: "Amber", hex: "#FFD58A", token: "--warm (dark)", role: "Warm highlight, status attention" },
  { name: "Coral", hex: "#FF9B7A", token: "--expressive (dark)", role: "Small expressive accent" },
  { name: "Mist", hex: "#9BA8B8", token: "--text-muted (dark)", role: "Dark-theme secondary text" },
  { name: "Graphite", hex: "#465365", token: "--text-muted (light)", role: "Light-theme secondary text" },
];

const SEMANTIC_TOKENS = [
  "--page",
  "--surface",
  "--surface-soft",
  "--text",
  "--text-muted",
  "--border",
  "--brand",
  "--brand-strong",
  "--secondary-fill",
  "--warm",
  "--danger",
  "--success",
];

const SPARK_DEMO = [4, 6, 5, 9, 7, 11, 10, 14, 12, 16];

function ThemePreview({ theme }: { theme: "light" | "dark" }) {
  return (
    <div className="mc-theme-preview" data-theme={theme}>
      <h3>{theme === "light" ? "Light (default)" : "Dark"}</h3>
      {SEMANTIC_TOKENS.map((t) => (
        <div className="mc-token-row" key={t}>
          <span className="mc-token-chip" style={{ background: `var(${t})` }}></span>
          <span className="mc-token-name">{t}</span>
        </div>
      ))}
      <div className="mc-specimen-row">
        <button className="mc-button mc-button-primary" type="button">
          Primary
        </button>
        <button className="mc-button mc-button-secondary" type="button">
          Secondary
        </button>
        <span className="mc-pill mc-pill-live">
          <span className="mc-dot" aria-hidden="true"></span>live
        </span>
        <StatusBadge status="warn" />
        <RealmChip n={0} />
        <RealmChip n={4} />
        <RealmChip n={7} />
      </div>
    </div>
  );
}

export function PaletteSection() {
  return (
    <>
      <section className="mc-ongoing-section" aria-labelledby="palette-heading">
        <h2 id="palette-heading">Color palette</h2>
        <p className="mc-section-abstract">
          Sampled and normalized from the robot logo: white and pearl carry the page, Powder blue and Blush pink carry
          the brand, Deep Blue does the interactive work on light backgrounds. Proportions ~70% neutrals, 20% blue, 7%
          blush, 3% amber/coral.
        </p>
        <div className="mc-swatch-grid">
          {PALETTE.map((c) => (
            <div className="mc-swatch" key={c.name}>
              <span className="mc-swatch-color" style={{ background: c.hex }}></span>
              <span className="mc-swatch-name">{c.name}</span>
              <span className="mc-swatch-hex">{c.hex}</span>
              <span className="mc-swatch-token">{c.token}</span>
              <span className="mc-swatch-role">{c.role}</span>
            </div>
          ))}
        </div>
      </section>
    </>
  );
}

export function VocabularySections() {
  return (
    <>
      <section className="mc-ongoing-section" aria-labelledby="vocab-heading">
        <h2 id="vocab-heading">Identification vocabulary</h2>
        <p className="mc-section-abstract">
          The shared system for naming, coding, and drawing the hierarchy's entities — taxonomy (what the levels are),
          notation (<span className="mc-id-code">W03</span>, <span className="mc-id-code">R4</span>,{" "}
          <span className="mc-id-code">S017</span>), and iconography. Spec: docs/design/vocabulary.md.
        </p>

        <div className="mc-storage-table-wrap mc-vocab-table-wrap">
          <table className="mc-storage-table">
            <thead>
              <tr>
                <th scope="col">Level</th>
                <th scope="col">Identity</th>
                <th scope="col">Code rule</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td>Universe</td>
                <td>
                  <span className="mc-vocab-icon-cell">
                    <UniverseIcon size={20} />
                    <span className="mc-body-muted">single color, one only</span>
                  </span>
                </td>
                <td>never numbered — referred to by name</td>
              </tr>
              <tr>
                <td>World</td>
                <td>
                  <span className="mc-vocab-icon-cell">
                    <WorldBadge n={3} max={12} />
                    <WorldBadge n={4} max={12} />
                    <span className="mc-body-muted">circle badge, blue/pink by parity</span>
                  </span>
                </td>
                <td className="mc-id-code">W01… pad = max(2, digits(max))</td>
              </tr>
              <tr>
                <td>Realm</td>
                <td>
                  <span className="mc-vocab-icon-cell">
                    <RealmChip n={4} name="platform" />
                    <span className="mc-body-muted">colored label chip</span>
                  </span>
                </td>
                <td className="mc-id-code">R0–R9, max 10, never padded</td>
              </tr>
              <tr>
                <td>Site</td>
                <td>
                  <span className="mc-vocab-icon-cell">
                    <SiteBadge n={17} max={120} realm={2} />
                    <span className="mc-body-muted">factory inside the realm-colored label</span>
                  </span>
                </td>
                <td className="mc-id-code">S01… same padding rule as worlds</td>
              </tr>
              <tr>
                <td>Node</td>
                <td>
                  <span className="mc-vocab-icon-cell">
                    <NodeBadge n={1} max={40} realm={2} role="builder" />
                    <span className="mc-body-muted">role emoji, ⬡ default, realm-colored label</span>
                  </span>
                </td>
                <td className="mc-id-code">N000001… pad = max(6, digits(max))</td>
              </tr>
            </tbody>
          </table>
        </div>

        <p className="mc-section-abstract">
          Dynamic padding, demonstrated: with 8 worlds → <span className="mc-id-code">{entityCode("W", 8, 8)}</span>;
          with 12 → <span className="mc-id-code">{entityCode("W", 8, 12)}</span>; with 120 sites →{" "}
          <span className="mc-id-code">{entityCode("S", 8, 120)}</span>; nodes floor at six digits →{" "}
          <span className="mc-id-code">{nodeCode(42, 500)}</span>, expanding past a million →{" "}
          <span className="mc-id-code">{nodeCode(42, 2_000_000)}</span>. The width follows the largest current count.
        </p>

        <p className="mc-section-abstract">
          Node role icons — one emoji per role type, a changeable convention with{" "}
          <span aria-hidden="true">{NODE_DEFAULT_EMOJI}</span> as the fixed default for unmapped roles. The emoji is
          decoration next to the code, never the identity.
        </p>
        <div className="mc-specimen-row">
          <NodeBadge n={7} max={40} name="unmapped role" />
          {Object.entries(NODE_ROLE_EMOJI).map(([role], i) => (
            <NodeBadge key={role} n={i + 1} max={40} role={role} name={role} />
          ))}
        </div>

        <p className="mc-section-abstract">
          World badges alternate Powder blue (odd) and Blush pink (even) — rhythm, not identity — sized to the icon slot
          with the code scaled to fit:
        </p>
        <div className="mc-specimen-row">
          {[1, 2, 3, 4, 5, 6].map((n) => (
            <WorldBadge key={n} n={n} max={12} size={20} />
          ))}
          <WorldBadge n={7} max={120} size={20} />
          <WorldBadge n={8} max={12} size={24} />
        </div>

        <p className="mc-section-abstract">
          One label principle: the icon/emoji lives inside the chip boundary, and a name replaces the number — hover a
          named chip to reveal its code. The realm's color cascades to every label inside it (a site in R4 and its nodes
          wear R4's pair); without realm context, labels fall back to the bright gray:
        </p>
        <div className="mc-specimen-row">
          <RealmChip n={4} name="platform" />
          <SiteBadge n={1} max={12} realm={4} name="deploy-squad" />
          <NodeBadge n={3} max={40} realm={4} role="builder" />
          <span className="mc-body-muted">·</span>
          <SiteBadge n={17} max={120} />
          <NodeBadge n={5} max={40} role="librarian" name="archive" />
        </div>

        <p className="mc-section-abstract">
          Chips border in their own text color, so they survive a background of their own fill; the inverse mode swaps
          the pair for emphasis on tinted surfaces:
        </p>
        <div
          className="mc-specimen-row"
          style={{ background: "var(--realm-4-bg)", padding: 12, borderRadius: 10, maxWidth: 480 }}
        >
          <SiteBadge n={1} max={12} realm={4} />
          <SiteBadge n={2} max={12} realm={4} inverse />
          <NodeBadge n={9} max={40} realm={4} role="ops" inverse />
        </div>

        <p className="mc-section-abstract">
          The ten realm pairs — each hue is a symmetric tint/deep pair (≥ 4.5:1 both themes: tint background with deep
          text on light, swapped on dark). The color follows the realm number forever, and the code is always shown, so
          color never carries identity alone.
        </p>
        <div className="mc-specimen-row">
          {REALM_COLORS.map((c, i) => (
            <RealmChip key={i} n={i} name={c.hue} />
          ))}
        </div>
      </section>
    </>
  );
}

export function ThemeTokensSection() {
  return (
    <section className="mc-ongoing-section" aria-labelledby="themes-heading">
      <h2 id="themes-heading">Semantic tokens, both themes</h2>
      <p className="mc-section-abstract">
        The same token names resolve per theme; each panel below forces its theme locally (a nested{" "}
        <code className="mc-mono">data-theme</code> attribute), so both are visible regardless of the active toggle.
      </p>
      <div className="mc-theme-previews">
        <ThemePreview theme="light" />
        <ThemePreview theme="dark" />
      </div>
    </section>
  );
}

export function TypeSection() {
  return (
    <section className="mc-ongoing-section" aria-labelledby="type-heading">
      <h2 id="type-heading">Type voices</h2>
      <p className="mc-section-abstract">
        Three voices, each with a job. Mono carries every number the system asserts.
      </p>
      <div className="mc-type-specimen">
        <span className="mc-spec-label">Manrope — headings</span>
        <h2>A universe big enough for wherever your dreams may take you.</h2>
      </div>
      <div className="mc-type-specimen">
        <span className="mc-spec-label">Inter — interface and body</span>
        <p style={{ margin: 0, maxWidth: "68ch" }}>
          Reports are books with a standard binding: any node can deposit one in the library, and any dashboard can read
          it back at any scope.
        </p>
      </div>
      <div className="mc-type-specimen">
        <span className="mc-spec-label">Mono — asserted numbers (tabular-nums)</span>
        <p className="mc-mono" style={{ margin: 0 }}>
          1,024.5 · a1b2c3d · 99.98% · 2026-08-26T14:30:00Z
        </p>
      </div>
    </section>
  );
}

export function PrimitiveSpecimens() {
  return (
    <section className="mc-ongoing-section" aria-labelledby="components-heading">
      <h2 id="components-heading">The primitives, live</h2>
      <p className="mc-section-abstract">The real classes and components, not copies.</p>

      <div className="mc-specimen-row">
        <button className="mc-button mc-button-primary" type="button">
          Primary
        </button>
        <button className="mc-button mc-button-secondary" type="button">
          Secondary
        </button>
        <button className="mc-button mc-button-tertiary" type="button">
          Tertiary
        </button>
        <button className="mc-icon-button" type="button" aria-label="Icon button specimen">
          ⚙
        </button>
      </div>

      <div className="mc-specimen-row">
        <span className="mc-pill mc-pill-live">
          <span className="mc-dot" aria-hidden="true"></span>live
        </span>
        <span className="mc-pill mc-pill-muted">
          <span className="mc-dot" aria-hidden="true"></span>polling
        </span>
        <span className="mc-pill mc-pill-error">
          <span className="mc-dot" aria-hidden="true"></span>reconnecting…
        </span>
        <StatusBadge status="ok" />
        <StatusBadge status="warn" />
        <StatusBadge status="critical" />
        <span className="mc-tag">delivery</span>
        <span className="mc-tag mc-tag-milestone">milestone</span>
        <span className="mc-sev mc-sev-info">info</span>
        <span className="mc-sev mc-sev-warn">warn</span>
        <span className="mc-sev mc-sev-critical">critical</span>
      </div>
    </section>
  );
}

export function CompositeSpecimens() {
  return (
    <section className="mc-ongoing-section" aria-labelledby="composites-heading">
      <h2 id="composites-heading">The composites, live</h2>
      <p className="mc-section-abstract">Primitives assembled into the shapes the app actually draws.</p>

      <div className="mc-specimen-row">
        <div className="mc-kpi-card mc-panel" style={{ flexDirection: "row", alignItems: "center", gap: 24 }}>
          <KpiStat kpi={{ name: "deploys", value: 42, delta: 6 }} />
          <KpiStat kpi={{ name: "lead time", value: 38.5, unit: "minutes", delta: -4 }} />
          <Sparkline points={SPARK_DEMO} />
        </div>
      </div>

      <div className="mc-storage-table-wrap" style={{ maxWidth: 560 }}>
        <table className="mc-storage-table">
          <thead>
            <tr>
              <th scope="col">Definition</th>
              <th scope="col">Scope</th>
              <th scope="col">Instances</th>
            </tr>
          </thead>
          <tbody>
            <tr>
              <td>delivery</td>
              <td className="mc-mono">spookify/music/platform-engineering</td>
              <td className="mc-mono">14</td>
            </tr>
            <tr>
              <td>incident</td>
              <td className="mc-mono">spookify/advertising</td>
              <td className="mc-mono">3</td>
            </tr>
          </tbody>
        </table>
      </div>
    </section>
  );
}
