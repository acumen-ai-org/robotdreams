import { useMemo } from "react";
import { useCallback, useRef, useState } from "react";
import { useReveal } from "../../hooks/useReveal";
import { useSelection } from "../../state/SelectionContext";
import { createPortal } from "react-dom";
import { reportingRoute, type Route } from "../../lib/routes";
import { tileName } from "../../components/shared";
import { bandTiles, statusCounts, type SeverityKey } from "../../lib/triage";
import { usePageControls } from "../../state/PageControlsContext";
import { SignalTile } from "./SignalTile";
import type { SummaryTile } from "../../lib/types";

export function GlanceBoard({ route, tiles }: { route: Route; tiles: SummaryTile[] }) {
  const { selection, toggle } = useSelection();
  const gridRef = useRef<HTMLDivElement>(null);
  const [mine, setMine] = useState(0);
  const scrollToTile = useCallback((key: string) => {
    const el = gridRef.current?.querySelector<HTMLElement>('[data-mc-tile="' + CSS.escape(key) + '"]');
    el?.scrollIntoView({ block: "center", behavior: "smooth" });
  }, []);
  useReveal(selection?.kind === "report" ? selection.def + "@" + selection.scope : null, true, scrollToTile, mine);
  const shown = useMemo(() => bandTiles(tiles), [tiles]);

  return (
    <div className="mc-glance-board" ref={gridRef}>
      {shown.map((band) =>
        band.tiles.length === 0 ? null : (
          <section className={"mc-glance-band mc-band-" + band.id} key={band.id} aria-label={band.title}>
            <div className="mc-band-head">
              <h3 className="mc-band-title">{band.title}</h3>
              <span className="mc-band-count mc-mono">{band.tiles.length}</span>
              <span className="mc-label-muted mc-band-note">{band.note}</span>
            </div>
            <div className="mc-tiles-grid">
              {band.tiles.map((t) => {
                const def = tileName(t);
                const scope = t.scope || route.scope;
                return (
                  <SignalTile
                    key={(t.scope || "") + "|" + def}
                    tile={t}
                    to={reportingRoute(route, { report: def, ...(t.scope ? { scope: t.scope } : {}) })}
                    tileKey={def + "@" + scope}
                    onSelect={() => {
                      setMine((n) => n + 1);
                      toggle({ kind: "report", def, scope });
                    }}
                    selected={selection?.kind === "report" && selection.def === def && selection.scope === scope}
                  />
                );
              })}
            </div>
          </section>
        ),
      )}
    </div>
  );
}

export function GlanceRibbon({
  active,
  tiles,
  only,
  onFocus,
}: {
  active: boolean;
  tiles: SummaryTile[];
  only: SeverityKey | null;
  onFocus: (key: SeverityKey) => void;
}) {
  const controls = usePageControls();
  const counts = useMemo(() => statusCounts(tiles), [tiles]);
  if (!active || !controls.toolsSlot) return null;
  return createPortal(
    <span className="mc-view-select">
      <span className="mc-view-select-label">
        {tiles.length} {tiles.length === 1 ? "report" : "reports"}
      </span>
      <span className="mc-glance-ribbon" role="group" aria-label="Filter by status">
        <RibbonCount
          label="critical"
          n={counts.critical}
          on={only === "critical"}
          onClick={() => onFocus("critical")}
        />
        <RibbonCount label="warn" n={counts.warn} on={only === "warn"} onClick={() => onFocus("warn")} />
        <RibbonCount label="ok" n={counts.ok} on={only === "ok"} onClick={() => onFocus("ok")} />
        {counts.none > 0 && (
          <RibbonCount label="no data" n={counts.none} on={only === "none"} onClick={() => onFocus("none")} />
        )}
      </span>
    </span>,
    controls.toolsSlot,
  );
}

function RibbonCount({ label, n, on, onClick }: { label: string; n: number; on: boolean; onClick: () => void }) {
  const slug = label.replace(/\s+/g, "-");
  return (
    <button
      type="button"
      className={"mc-ribbon-count is-" + slug + (on ? " is-on" : "")}
      aria-pressed={on}
      onClick={onClick}
      disabled={n === 0}
    >
      <span className="mc-ribbon-n mc-mono">{n}</span>
      <span className="mc-ribbon-label">{label}</span>
    </button>
  );
}
