# Dream Robot Web Design System

This is the shared design system for every Robot Dreams surface: the
marketing page (acumen-web `/robotdreams`) and Mission Control (the
dashboard embedded in the `dream` binary). This file is the source of
truth; other repos carry copies with a provenance header — edit here,
then re-copy.

## Direction

A calm, optimistic interface built around the soft 3D robot logo. The
default experience is bright: white and pearl surfaces let the
powder-blue face, blush shell, and amber antenna lights carry the color.
The dark theme is a first-class peer (`[data-theme="dark"]`), keeping
the same personality with near-black surfaces that make the logo feel
luminous.

Design qualities: friendly, intelligent, quiet, rounded, spacious, gently futuristic.

Avoid: pure sci-fi neon, glass everywhere, heavy gradients, excessive glow, hard corners, dense dashboards, and large areas of pastel text.

## Logo

Use the full robot mark as the primary brand asset.

Canonical assets (source of truth:
`acumen-web/src/robotdreams/assets/`):

- `logo-animated.webp` — the blinking logo: a 12-frame animated WebP
  (long open-eyes holds, then a quick blink). Use it for brand marks in
  navigation and heroes. Mission Control carries a downscaled copy in
  `internal/dashboard/ui/app/src/brand/`; re-copy from the source when it
  changes.
- `logo-web.png` — the 480×480 still, for favicons, small sizes, and
  wherever animation is inappropriate (including as the
  `prefers-reduced-motion` replacement — animated WebP ignores that
  media query, so swap the asset, not the CSS).

Rules:

- Place it on a plain background with no container when possible.
- Keep clear space equal to one antenna bulb around the full silhouette.
- Minimum size: `40px` tall for an icon, `72px` tall in navigation, `180px` wide in hero areas.
- Never recolor, crop the antennae, add a drop shadow, or place text over the face.
- The blink is the one sanctioned face animation; do not add others, and do not flash the antenna bulbs.
- On dark backgrounds, a restrained ambient halo is allowed: `0 0 64px rgb(127 203 238 / 12%)`. On light backgrounds, prefer a grounded soft shadow such as `drop-shadow(0 16px 40px rgb(23 79 125 / 16%))`.

## Color palette

The palette is sampled and normalized from the logo rather than copied pixel-for-pixel. This keeps contrast predictable across screens.

| Token | Hex | Role |
| --- | --- | --- |
| Ink | `#080A0F` | Dark page background |
| Carbon | `#10141D` | Dark elevated surface |
| Slate | `#1A2230` | Dark border and tertiary surface |
| Pearl | `#F6F3F8` | Light page background and dark-theme primary text |
| Snow | `#FFFFFF` | Light elevated surface |
| Powder | `#A9D9F2` | Brand primary |
| Sky | `#7FCBEE` | Interactive accent |
| Deep Blue | `#174F7D` | Strong accent and light-theme links |
| Blush | `#EBCFDC` | Secondary brand accent |
| Amber | `#FFD58A` | Warm highlight and status attention |
| Coral | `#FF9B7A` | Small expressive accent |
| Mist | `#9BA8B8` | Dark-theme secondary text |
| Graphite | `#465365` | Light-theme secondary text |

Use approximate visual proportions of `70%` neutrals, `20%` blue, `7%` blush, and `3%` amber/coral. Amber and coral should remain rare enough to feel alive.

## Theme tokens

Light is the default. Tokens are declared on both `:root` and the bare
`[data-theme="light"]` attribute selector so any *nested* element can
force a theme for its subtree (used by design-showcase preview panels);
dark is `[data-theme="dark"]`.

In the light theme, Powder is a **fill**, never a text color; Deep Blue
is the interactive text/stroke color. In the dark theme those roles
relax: Powder-family blues are bright enough to serve as text accents.

```css
:root,
[data-theme="light"] {
  color-scheme: light;
  --page: #f6f3f8;
  --surface: #ffffff;
  --surface-raised: #ffffff;
  --surface-soft: #eef5fa;
  --text: #10141d;
  --text-muted: #465365;
  --border: #e3e9ef;
  --brand: #a9d9f2;          /* Powder — fills only, never text */
  --brand-hover: #8fcdec;
  --brand-strong: #174f7d;   /* Deep Blue — interactive text and strokes */
  --brand-ink: #0d2b42;      /* ink on Powder fills */
  --secondary: #8f5f78;      /* blush darkened for AA text */
  --secondary-fill: #ebcfdc; /* Blush proper, for tints */
  --warm: #9b6500;           /* amber darkened for AA text */
  --expressive: #b9492e;
  --focus: #1678aa;
  --danger: #b4233b;
  --success: #147a5a;
  --shadow: 0 10px 30px rgb(23 79 125 / 10%);
  --font-mono: "JetBrains Mono", "IBM Plex Mono", ui-monospace,
               "SF Mono", Menlo, Consolas, monospace;
}

[data-theme="dark"] {
  color-scheme: dark;
  --page: #080a0f;
  --surface: #10141d;
  --surface-raised: #151b26;
  --surface-soft: #1a2230;
  --text: #f6f3f8;
  --text-muted: #9ba8b8;
  --border: #263142;
  --brand: #a9d9f2;
  --brand-hover: #c4e8f8;
  --brand-strong: #7fcbee;
  --brand-ink: #07131c;
  --secondary: #ebcfdc;
  --secondary-fill: #ebcfdc;
  --warm: #ffd58a;
  --expressive: #ff9b7a;
  --focus: #7fcbee;
  --danger: #ff7f8f;
  --success: #72d6b1;
  --shadow: 0 18px 60px rgb(0 0 0 / 40%);
}
```

Persist the visitor's explicit theme choice. Otherwise follow `prefers-color-scheme`, defaulting to light when no preference is available. The theme control should use a sun/moon icon plus an accessible label, not color alone.

Tinted fills use pure blush: mix from `--secondary-fill`, not
`--secondary` (which is darkened for legible light-theme text).

## Identification vocabulary

The hierarchy's entities share one identification system across every
surface — full spec in
[docs/design/vocabulary.md](docs/design/vocabulary.md) (taxonomy →
notation → iconography; edit there, then re-sync copies). The short
version:

- **Universe** — one only, never numbered; lucide `orbit` icon (the
  `galaxy` stand-in), single color: Ink on light, Pearl on dark.
- **Worlds** — `W01`, `W02`, … zero-padded to `max(2, digits(max))`;
  rendered as the WorldBadge component: an icon-sized circle with the
  code inside, fill alternating Powder (odd) / Blush (even) — rhythm,
  not identity.
- **Realms** — `R0`–`R9` (max 10), each owning a symmetric tint/deep
  color pair (≥ 4.5:1 both themes): chip bg = tint / text = deep on
  light, swapped on dark. Color follows the number forever; the code is
  always shown, so color never carries identity alone.
- **Sites** — `S01`, `S02`, … same padding rule as worlds; a label
  chip in **its realm's color pair** (gray fallback without realm
  context) with the lucide `factory` icon inside the chip.
- **Nodes** — `N000001`, … six-digit floor, same dynamic rule
  (`max(6, digits(max))`); the same realm-colored label chip with the
  role emoji inside it (changeable per-role set, generic ⬡ default).
- **One label principle** — the chip boundary includes the icon/emoji;
  one shape (6px box, no underlines or pills); a name replaces the
  number, which reappears on hover and stays in the accessible name.
- **Cascade and modes** — the realm's pair colors every label inside
  the realm, and content boxes (realm boards, site boxes) take it as
  their surface on the web; chips border in `currentColor` and offer
  filled/inverse modes so they survive same-colored backgrounds (see
  vocabulary.md).

Codes are set in the mono voice. Realm colors are an identity scale —
never reused as status or chart-series colors.

## Typography

Three voices, each with a job:

- **Space Grotesk** — headings and brand: the "computer/science"
  voice, geometric and instrument-like without turning sci-fi.
- **Inter** — interface and body copy.
- **Mono** (`--font-mono`: JetBrains Mono / IBM Plex Mono /
  `ui-monospace`) — every number the system asserts: KPIs, counts,
  timestamps, identifiers, revisions, code. Always with
  `font-variant-numeric: tabular-nums`. Never for prose.

If external fonts are undesirable, headings fall back to
`system-ui, sans-serif` (Space Grotesk is not rounded — don't
substitute a rounded face), body to `ui-rounded, system-ui,
sans-serif`; the mono stack already ends in system fonts. App surfaces
that must work offline self-host the font files — no CDN or Google
Fonts at runtime.

| Style | Size / line-height | Weight |
| --- | --- | --- |
| Display | `clamp(3rem, 7vw, 6.5rem) / 0.98` | 700 |
| H1 | `clamp(2.5rem, 5vw, 4.5rem) / 1.02` | 700 |
| H2 | `clamp(2rem, 3vw, 3rem) / 1.1` | 650 |
| H3 | `1.375rem / 1.25` | 650 |
| Body large | `1.125rem / 1.7` | 400 |
| Body | `1rem / 1.65` | 400 |
| Label | `0.875rem / 1.3` | 600 |

Headlines should be short and slightly tight. Body text should stay under `68ch`. Use sentence case; avoid all caps except tiny technical tags.

## Layout

Two width tiers, both responsive and both using as much of the viewport
as their tier allows:

- **Web pages** (marketing, docs): maximum content width `1440px`.
- **App surfaces** (Mission Control): fully fluid — no maximum width;
  only the page gutters remain at the edges.

Shared rules:

- Reading width: `720px` (prose never spans the full tier).
- Page gutters: `20px`, growing to `40px` above `768px`.
- Spacing scale: `4, 8, 12, 16, 24, 32, 48, 64, 96, 128px`.
- Major sections: at least `96px` vertical padding on desktop and `64px` on mobile.
- Border radius: `6px` controls and small blocks, `10px` cards, panels, and menus, `24px` feature panels on web pages, pill only for tags, status pills, and compact filter controls. Rounding is restrained everywhere — when in doubt, less.

The hero should pair a large, concise promise with the robot logo. On desktop, use a balanced two-column composition; on mobile, place the logo above the message. Keep generous negative space around the antennae.

## Surfaces and depth

Neither theme should be a stack of identical rectangles. Establish depth through small lightness shifts:

1. Page: Pearl (light) / Ink (dark).
2. Section or card: Snow (light) / Carbon (dark).
3. Raised interactive surface: `--surface-raised`.
4. Selected or tinted surface: `color-mix(in srgb, var(--brand) 10%, var(--surface))`.

Use `1px` borders before shadows. Reserve the main shadow for menus, dialogs, and a few feature cards. A subtle blue-to-blush ambient gradient may appear behind the hero only, at no more than `14%` opacity.

## Components

### Navigation

Use a transparent header over the page, becoming a slightly blurred `--page` surface after scrolling. Keep the logo left, primary links centered or right, and theme toggle beside the main action. Header height: `72px` desktop, `64px` mobile.

### Buttons

Primary buttons use `--brand` with `--brand-ink`; in the light theme, the darker brand token ensures contrast. Secondary buttons are transparent with a `--border` outline. Tertiary actions are text-only. Minimum target: `44 × 44px`.

Button geometry: `6px` radius, `12px 18px` padding, semibold label, `160ms` color and transform transition. Hover may move upward by `1px`; active returns to baseline.

### Cards

Cards use `--surface`, a `1px` border, and `10px` radius. Default padding is `24px`, increasing to `32px` for feature cards. Use blue for selected states, blush for human or creative themes, and amber only for noteworthy information.

### Tables

Table headers are quiet labels, not bars: `11px`, uppercase, `0.08em`
letter-spacing, `--text-muted`, transparent background, a single hairline
`border-bottom`. No zebra striping — hairlines between rows only, and no
border after the last row. Numeric columns use the mono voice.

### Badges and pills

Bordered pills with tint fills: `1px` hairline border, pill radius,
small label. State variants tint the fill from the state color (mixed
into the surface) and keep AA text — never white on a pastel fill.
Never state by color alone; the label carries the meaning.

### Stat tiles

A stat tile is a card (`--surface`, hairline border, card radius) with a
small muted label and a large mono value (`tabular-nums`); units and
deltas render smaller and muted. Sparklines are allowed inside tiles;
axes are not.

### Forms

Inputs are at least `48px` tall with a visible label above. Use `--surface-soft`, `--border`, and a `2px` focus ring. Placeholder text is supplementary and never replaces a label. Error states require an icon or message in addition to color.

### Theme toggle

Use a compact two-state control. Animate the icon with opacity and a small rotation, never a large sweep. Its state must be available to assistive technology through `aria-pressed` or an equivalent native control.

## Motion

Motion should feel sleepy and reassuring.

- Standard transition: `160ms cubic-bezier(.2, .8, .2, 1)`.
- Large reveal: `320ms` maximum.
- The logo may float vertically by `4px` over `4s`; do not animate the face or flash the antenna bulbs.
- Disable nonessential animation under `prefers-reduced-motion: reduce`.

## Accessibility

- Meet WCAG AA: `4.5:1` for normal text and `3:1` for large text and UI boundaries.
- Never place white text directly on Powder or Amber.
- Use Deep Blue or Ink text on light pastel fills.
- In the light theme: Powder is a fill, never a text color; Deep Blue `#174F7D` is the interactive text color; the focus ring uses deepened Sky `#1678AA`.
- Focus rings must remain visible in both themes and should not be removed.
- Every icon-only control requires an accessible name.
- Maintain logical heading order and keyboard navigation.
- Do not communicate state solely through blush, amber, or coral.

## Recommended first page

1. Header with compact robot logo, navigation, theme toggle, and one primary action.
2. Hero with the full robot, a short human headline, supporting sentence, and two actions.
3. Three benefit cards with restrained blue, blush, and amber accents.
4. One raised feature panel demonstrating the product or idea.
5. Trust or proof section with minimal monochrome marks.
6. Final call to action with a soft blue/blush ambient glow.
7. Quiet footer with product, company, legal, and theme controls.

The page should feel like meeting a thoughtful little machine on a bright morning: calm enough to trust, warm enough to like — and at night (the dark theme), the same machine, luminous.
