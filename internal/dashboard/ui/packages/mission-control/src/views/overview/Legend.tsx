import { usePageControls } from "../../state/PageControlsContext";
import { UPDATE_PHASES } from "../../lib/updates";
import { NodeBadge, RealmChip, SiteBadge, UniverseIcon, WorldBadge } from "../../lib/vocabulary";

function Item({ mark, name, what }: { mark: React.ReactNode; name: string; what: string }) {
  return (
    <li className="mc-legend-item">
      <span className="mc-legend-mark">{mark}</span>
      <span className="mc-legend-what">
        <span className="mc-legend-name">{name}</span> — {what}
      </span>
    </li>
  );
}

export function OverviewLegend() {
  const controls = usePageControls();
  return (
    <div className="mc-expander-block">
      <div className="mc-expander-head">
        <span className="mc-expander-title">Legend</span>
        <button className="mc-button mc-button-tertiary" type="button" onClick={() => controls.setOpenPanel(null)}>
          ✕ close
        </button>
      </div>
      <div className="mc-legend-sections">
        <section className="mc-legend-section">
          <h3 className="mc-legend-title">Places</h3>
          <ul className="mc-legend-items">
            <Item mark={<UniverseIcon size={14} />} name="Universe" what="everything one control plane runs" />
            <Item mark={<WorldBadge n={1} max={4} />} name="World" what="a top-level division (W1…)" />
            <Item
              mark={<RealmChip n={0} />}
              name="Realm"
              what="a department within a world (R0–R9); its colour cascades to what it contains"
            />
            <Item mark={<SiteBadge n={1} max={9} />} name="Site" what="the team or place reports attach to" />
            <Item mark={<NodeBadge n={1} max={9} />} name="Node" what="a worker; the icon follows its role" />
          </ul>
        </section>

        <section className="mc-legend-section">
          <h3 className="mc-legend-title">Row marks</h3>
          <ul className="mc-legend-items">
            <Item
              mark={<span className="mc-status-dot mc-connected" />}
              name="Connected"
              what="the node is on the wire now"
            />
            <Item
              mark={<span className="mc-status-dot mc-degraded" />}
              name="Degraded"
              what="connected, but struggling"
            />
            <Item mark={<span className="mc-status-dot mc-disconnected" />} name="Disconnected" what="not connected" />
            <Item
              mark={<span className="mc-org-pulse mc-pulse">●</span>}
              name="Pulse"
              what="a message just passed through it"
            />
            <Item mark={<span className="mc-org-app">↗</span>} name="App" what="serves an app you can open or embed" />
            <Item
              mark={<span className="mc-legend-fade">node-id</span>}
              name="Faded"
              what="outside the current scope or role filter; kept so a match keeps its branch"
            />
            <Item mark={<span className="mc-legend-swatch is-match" />} name="Highlighted" what="matches the search" />
          </ul>
        </section>

        <section className="mc-legend-section">
          <h3 className="mc-legend-title">Update phase</h3>
          <ul className="mc-legend-items">
            {UPDATE_PHASES.map((p) => (
              <Item
                key={p.id}
                mark={<span className={"mc-org-update is-" + p.tone}>{p.label}</span>}
                name={p.label}
                what={p.hint}
              />
            ))}
          </ul>
        </section>

        <section className="mc-legend-section">
          <h3 className="mc-legend-title">Health beneath</h3>
          <ul className="mc-legend-items">
            <Item
              mark={<span className="mc-org-worst is-warn">warn</span>}
              name="Warn"
              what="a report under this place is past its warning line"
            />
            <Item
              mark={<span className="mc-org-worst is-critical">critical</span>}
              name="Critical"
              what="a report under this place is past its critical line"
            />
          </ul>
        </section>
      </div>
    </div>
  );
}
