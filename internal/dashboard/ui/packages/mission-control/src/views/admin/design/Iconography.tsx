import {
  AlertIcon,
  AllClearIcon,
  AppWindowIcon,
  LayersIcon,
  LegendIcon,
  MapIcon,
  PanelRightCloseIcon,
  PanelRightOpenIcon,
  PulseIcon,
  SearchIcon,
  SettingsIcon,
} from "../../../components/icons";
import { NodeBadge, NODE_ROLE_EMOJI, RealmChip, SiteBadge, UniverseIcon, WorldBadge } from "../../../lib/vocabulary";

const ICONS: Array<[string, React.ReactNode, string]> = [
  ["map", <MapIcon size={18} key="i" />, "Universe — the Control Plane"],
  ["pulse", <PulseIcon size={18} key="i" />, "Outcomes, and every jump to it"],
  ["settings", <SettingsIcon size={18} key="i" />, "the Settings gear"],
  ["search", <SearchIcon size={18} key="i" />, "search, and the zoom flyout"],
  ["layers", <LayersIcon size={18} key="i" />, "the network's depth flyout"],
  ["legend", <LegendIcon size={18} key="i" />, "the toolbox legend toggle"],
  ["alert", <AlertIcon size={18} key="i" />, "wants attention"],
  ["all-clear", <AllClearIcon size={18} key="i" />, "nothing is asking for you"],
  ["app-window", <AppWindowIcon size={18} key="i" />, "a node serves an app"],
  ["panel-close", <PanelRightCloseIcon size={18} key="i" />, "collapse the side panel"],
  ["panel-open", <PanelRightOpenIcon size={18} key="i" />, "reopen it"],
];

export default function DesignIconography() {
  return (
    <>
      <section className="mc-ongoing-section" aria-labelledby="icons-heading">
        <h2 id="icons-heading">Interface icons</h2>
        <p className="mc-section-abstract">
          Lucide paths inlined (24×24, currentColor, stroke 2) — no icon font, no runtime fetch. Every icon-only control
          carries an accessible name.
        </p>
        <div className="mc-icon-sheet">
          {ICONS.map(([name, node, what]) => (
            <span className="mc-icon-cell" key={name}>
              {node}
              <span className="mc-mono mc-icon-cell-name">{name}</span>
              <span className="mc-icon-cell-what">{what}</span>
            </span>
          ))}
        </div>
      </section>

      <section className="mc-ongoing-section" aria-labelledby="badges-heading">
        <h2 id="badges-heading">Place badges</h2>
        <p className="mc-section-abstract">The identification vocabulary's marks — one per hierarchy level.</p>
        <div className="mc-specimen-row">
          <UniverseIcon size={18} />
          <WorldBadge n={1} max={4} />
          <RealmChip n={0} name="engineering" />
          <SiteBadge n={1} max={9} name="platform" realm={0} />
          <NodeBadge n={1} max={9} name="builder-01" role="builder" realm={0} />
        </div>
      </section>

      <section className="mc-ongoing-section" aria-labelledby="emoji-heading">
        <h2 id="emoji-heading">Role emoji</h2>
        <p className="mc-section-abstract">One per role type; a node with no listed role wears the hexagon.</p>
        <div className="mc-icon-sheet">
          {Object.entries(NODE_ROLE_EMOJI).map(([role, e]) => (
            <span className="mc-icon-cell" key={role}>
              <span style={{ fontSize: "1.125rem" }}>{e}</span>
              <span className="mc-mono mc-icon-cell-name">{role}</span>
            </span>
          ))}
        </div>
        <span className="mc-provenance">source: ui/src/components/icons.tsx · ui/src/lib/vocabulary.tsx</span>
      </section>
    </>
  );
}
