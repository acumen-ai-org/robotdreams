# Visual vocabulary

The shared identification system for Robot Dreams surfaces (Mission
Control and the acumen-web `/robotdreams` page): how the hierarchy's
entities are named, coded, colored, and drawn, so the same thing looks
the same everywhere.

Terminology — three layers, and it helps to keep their names straight:

- **Taxonomy** — the classification itself: Universe → World → Realm →
  Site. What the levels *are*.
- **Notation** (nomenclature) — the code scheme: `W01`, `R3`, `S07`.
  How instances are *named*.
- **Iconography** — icons, shapes, and colors. How they are *drawn*.

Together they form this **visual vocabulary**. ("Copy" is something
else — the written UX text; that stays governed by DESIGN.md's tone
rules.)

This file is the source of truth, next to the repo-root
[DESIGN.md](../../DESIGN.md) (colors, type, layout). Other repos carry
provenance-headed copies — edit here, then re-copy.

## The identification table

| Level | Code | Icon (lucide) | Shape | Color |
| --- | --- | --- | --- | --- |
| Universe | none — there is only one | `orbit`¹ | — | Single color: Ink on light, Pearl on dark. Never tinted. |
| World | `W01`, `W02`, … | — (no icon) | circle badge, code inside | Alternating Powder / Blush by number parity (odd = blue, even = pink; `W01` is blue). |
| Realm | `R0` … `R9` (max 10) | — | label chip | One of the ten realm pairs below. |
| Site | `S01`, `S02`, … | `factory`, inside the chip | label chip | Its realm's pair; gray fallback with no realm context. |
| Node | `N000001`, … | emoji, by role (default ⬡), inside the chip | label chip | Its realm's pair; gray fallback with no realm context. |

¹ lucide has no `galaxy` icon; `orbit` is the stand-in. Code comment
where it's used: maybe we use 🌌 later instead.

## Standard badge components

Each level renders through **one shared component per site** (Mission
Control: `ui/packages/mission-control/src/lib/vocabulary.tsx`; acumen-web: the hierarchy's
identity components) so it is visualized the same everywhere — never
hand-rolled markup.

- **WorldBadge** — a circle **the same size as an icon slot** (16/20/24
  px), with the `W##` code inside, its text scaled down to fit the
  code's length. Fill alternates by number parity: **odd = Powder blue,
  even = Blush pink** (`W01` blue, `W02` pink, …), both with Ink text
  (≥ 12:1). Like the logo, the pastel fills are theme-invariant. This
  parity coloring is rhythm, not identity — nothing may *mean* by it.
  World *bodies* (planets) use the same two fills and nothing else.
- **RealmChip** — the colored label: **6px radius** (less rounded than
  a pill), `line-height: 1` with balanced padding so the code sits
  optically centered, mono. With a name it reads `R4 ·platform` — a
  small gap before the dot, **no space after it**.
- **SiteBadge** — the label chip with the `factory` icon **inside the
  chip boundary**, followed by the code or name.
- **NodeBadge** — the label chip with the role emoji **inside the chip
  boundary**, followed by the code or name.

### One label principle

All label chips follow the same rules — no per-level variations:

- **The chip boundary includes the icon/emoji.** Icons never float
  outside the box; the box is the label.
- **One shape**: the box with slightly rounded corners (6px). No
  underlines, no pills, no alternate shapes.
- **A name replaces the number.** When the label has a name, only the
  name shows; the code appears on hover (revealed inside the chip as
  `R4 ·platform`) and is always available to assistive tech via the
  accessible name. Without a name, the code shows.

### Label colors cascade from the realm

The realm's color pair colors **every label inside that realm**: a site
in `R4` renders its chip in R4's magenta pair, and so do the nodes
within it. The realm's hue is the ancestry color — one glance says
"this belongs to R4".

When there is **no realm context** (a node at universe level, a generic
specimen, an unscoped listing), site and node chips fall back to the
shared bright-gray pair: tint `#E7EAEE` / deep `#49525C` (6.58:1),
symmetric like the realm pairs and deliberately outside the ten realm
hues.

### Chip modes

Every label chip carries a **border in its text color**
(`currentColor`), so a chip survives sitting on a background that
matches its own fill. Two modes, same pair:

- **filled** (default) — bg = tint, text = deep, border = deep (on
  light; swapped on dark). Use on neutral surfaces and on
  same-tint surfaces — the deep border keeps the edge.
- **inverse** — bg = deep, text = tint, border = tint. Use when the
  chip sits on a strongly tinted surface of its own hue and needs to
  pop, or as the emphasis variant.

Pick per context; never mix modes within one row of siblings.

### Larger surfaces carry the realm color

The realm color is not confined to chips: on the web, **content boxes
take the label's background color** — the realm board *and* the site
boxes inside it use the realm pair as their surface (tint surface with
deep text/borders on light, deep surface with tint text on dark).
Contained chips then typically use the **inverse** mode to stay
legible. Worlds and the universe never take realm colors.

## Notation rules

- **Codes are set in the mono voice** (`--font-mono`, tabular-nums) —
  they are identifiers the system asserts, like revisions and KPIs.
- **Universe**: never numbered, never pluralized. There is exactly one;
  it is referred to by name (e.g. `spookify`), and its icon carries the
  identity.
- **Worlds and Sites**: `W` / `S` prefix + 1-based number, zero-padded
  based on the **largest current count**: padding width =
  `max(2, len(str(max)))` — two digits minimum, growing when the count
  does. 8 worlds → `W01`…`W08`; 12 worlds → `W01`…`W12`; 120 sites →
  `S001`…`S120`. Padding is dynamic — recompute when the count
  changes; never hardcode a width.

  ```js
  const code = (prefix, n, max) =>
    prefix + String(n).padStart(Math.max(2, String(max).length), "0");
  ```

  (Both sites implement exactly this helper; keep the semantics if you
  reimplement it.)
- **Realms**: `R0`–`R9`, single digit, never padded — the maximum is
  **10 realms**, which is what makes a fixed color assignment possible.
- **Nodes**: `N` prefix + 1-based number, padded to **six digits by
  default** (`N000001`) — the same dynamic rule with a bigger floor:
  padding width = `max(6, len(str(max)))`. It expands past a million
  nodes and never shrinks below six.
- A code plus a name reads code-first: `W03 · music`.

## Node icons (emoji)

Nodes are the one level identified by **emoji** rather than a lucide
icon — a set of role icons, one per role type. The mapping is a
convention, not a contract: it can be changed and extended per
deployment; only the default is fixed.

| Role | Emoji |
| --- | --- |
| *(default / unknown role)* | ⬡ |
| ceo | 👑 |
| vp | 🌐 |
| lead | ⭐ |
| orchestrator | 🧭 |
| builder | 🔧 |
| researcher | 🔬 |
| writer | ✍️ |
| reviewer | 🔍 |
| ops | ⚙️ |
| librarian | 📚 |
| messenger | ✉️ |
| guardian | 🛡️ |
| delegate | 🤝 |

Rules:

- **⬡ (the generic hexagon) is the default** — any node whose role has
  no mapping renders ⬡, so an unmapped role is never invisible.
- The emoji is decoration next to the code, never the identity: the
  `N######` code (and usually the node's id/name) always rides along —
  same principle as realm colors.
- One emoji per role *type*, not per node; two nodes sharing a role
  share an icon.
- Keep the set calm: single emoji, no skin-tone or gender variants, no
  flags. When a deployment remaps icons, it remaps the whole role, not
  individual nodes.

## Realm colors (the label-safe scheme)

Each realm owns a **tint / deep** pair of the same hue. The pair is
symmetric, so one validated pair serves both themes:

- **Light theme**: chip background = tint, text = deep.
- **Dark theme**: chip background = deep, text = tint.

Every pair measures ≥ 4.5:1 WCAG contrast (checked computationally —
re-verify after any hex change), so realm chips are AA at any text size
in both themes. Chips always carry a 1px `--border` hairline, per
DESIGN.md's badge spec.

| Realm | Hue | Tint | Deep | Contrast |
| --- | --- | --- | --- | --- |
| R0 | blue | `#CDE2FB` | `#1C5CAB` | 5.01 |
| R1 | orange | `#FBD9C8` | `#9A3D12` | 5.20 |
| R2 | teal | `#C4EEDD` | `#0E5F43` | 6.07 |
| R3 | amber | `#F8E5B0` | `#755000` | 5.79 |
| R4 | magenta | `#F9D5E3` | `#92375D` | 5.35 |
| R5 | green | `#CFEBC4` | `#2A6410` | 5.58 |
| R6 | violet | `#DCD6F7` | `#40329B` | 6.99 |
| R7 | red | `#FAD2D1` | `#A02524` | 5.46 |
| R8 | cyan | `#C5E8F5` | `#0E5B76` | 5.85 |
| R9 | slate | `#DFE3E8` | `#3E4A57` | 7.02 |

Rules:

- **The color follows the realm number, never rank or order of
  appearance.** R4 is magenta everywhere, forever, even when it's the
  only realm on screen. Filtering must not repaint survivors.
- **Color is reinforcement, never the sole carrier.** The chip always
  shows its `R#` code — a colorblind reader loses nothing. (This is
  also why ten adjacent pastels are acceptable at all.)
- Realm colors are an **identity scale**. They are not status colors
  (ok/warn/critical stay on `--success`/`--warm`/`--danger`) and not
  chart-series colors; don't borrow from either direction.
- As CSS tokens: `--realm-N-bg` / `--realm-N-fg`, defined per theme
  (light: bg=tint fg=deep; dark: bg=deep fg=tint). Chip class:
  `realm-chip r4`.

## Iconography rules

- Icons come from **lucide** (acumen-web imports `lucide-react`;
  Mission Control inlines the same paths as small SVG components to
  stay dependency-light — copy paths from lucide, keep 24×24 viewBox,
  `stroke="currentColor"`, stroke-width 2).
- Icons inherit `currentColor` — they take the text color of their
  context and are never tinted with realm colors.
- The **universe icon** is the one single-color mark: Ink (`--text`) on
  light, Pearl on dark. It never appears more than once per view.
- The **WorldBadge circle** is the world's standard mark (there is no
  world icon). Larger world *bodies* (the marketing page's planets) use
  **only the defined parity fills** — Powder for odd, Blush for even,
  Ink text, theme-invariant, exactly like the badge — never their own
  invented fills. The `W##` code must ride along.
- Sizes follow DESIGN.md's control scale: 16px inline with text, 20px
  in chips and rows, 24px+ standalone.

## Where it must be used

- **Mission Control → Ongoing work** (`#ongoing`): the vocabulary is a
  showcase section there — rendered from the real tokens/components so
  it can't drift.
- **acumen-web `/robotdreams`**: the hierarchy figure ("One universe,
  any shape") labels its universe frame, planets (worlds), realm
  boards, and site boxes with this notation and iconography.
- Anything new that displays scope paths (breadcrumbs, report tiles,
  org charts) should adopt the codes as they gain hierarchy awareness —
  scope *paths* (`spookify/music/playback/player-squad`) remain
  the wire format; codes are a display-layer convention.
