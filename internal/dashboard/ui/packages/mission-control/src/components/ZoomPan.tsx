/* eslint-disable @typescript-eslint/unbound-method */
import { useCallback, useEffect, useRef, useState } from "react";
import { createPortal } from "react-dom";
import { select } from "d3-selection";
import { usePageControls } from "../state/PageControlsContext";
import { SearchIcon, XIcon } from "./icons";
import { zoom as d3zoom, zoomIdentity, type D3ZoomEvent, type ZoomBehavior } from "d3-zoom";

const MIN_SCALE = 0.4;
const MAX_SCALE = 8;
const STEP = 1.4;

export interface ZoomApi {
  svgRef: (el: SVGSVGElement | null) => void;
  transform: string;
  scale: number;
  zoomIn: () => void;
  zoomOut: () => void;
  setScale: (k: number) => void;
  reset: () => void;
  zoomTo: (cx: number, cy: number, r: number) => boolean;
}

const ZOOM_TO_MS = 420;

export function useD3Zoom(): ZoomApi {
  const [svg, setSvg] = useState<SVGSVGElement | null>(null);
  const behavior = useRef<ZoomBehavior<SVGSVGElement, unknown> | null>(null);
  const [state, setState] = useState({ transform: "translate(0,0) scale(1)", scale: 1 });

  useEffect(() => {
    if (!svg) return;
    const b = d3zoom<SVGSVGElement, unknown>()
      .scaleExtent([MIN_SCALE, MAX_SCALE])
      .on("zoom", (event: D3ZoomEvent<SVGSVGElement, unknown>) => {
        setState({ transform: event.transform.toString(), scale: event.transform.k });
      });
    behavior.current = b;
    select(svg).call(b);
    select(svg).on("dblclick.zoom", null);
    return () => {
      select(svg).on(".zoom", null);
      behavior.current = null;
    };
  }, [svg]);

  const by = useCallback(
    (factor: number) => {
      if (svg && behavior.current) select(svg).transition().duration(160).call(behavior.current.scaleBy, factor);
    },
    [svg],
  );

  const zoomIn = useCallback(() => by(STEP), [by]);
  const zoomOut = useCallback(() => by(1 / STEP), [by]);
  const setScale = useCallback(
    (k: number) => {
      if (svg && behavior.current)
        select(svg).call(behavior.current.scaleTo, Math.max(MIN_SCALE, Math.min(MAX_SCALE, k)));
    },
    [svg],
  );
  const reset = useCallback(() => {
    if (svg && behavior.current) select(svg).transition().duration(200).call(behavior.current.transform, zoomIdentity);
  }, [svg]);

  const zoomTo = useCallback(
    (cx: number, cy: number, r: number) => {
      if (!svg || !behavior.current || r <= 0) return false;
      const vb = svg.viewBox.baseVal;
      const w = vb?.width || svg.clientWidth;
      const h = vb?.height || svg.clientHeight;
      if (!w || !h) return false;
      const k = Math.max(MIN_SCALE, Math.min(MAX_SCALE, (Math.min(w, h) * 0.85) / (2 * r)));
      const next = zoomIdentity.translate(w / 2 - k * cx, h / 2 - k * cy).scale(k);
      select(svg).transition().duration(ZOOM_TO_MS).call(behavior.current.transform, next);
      return true;
    },
    [svg],
  );

  return { svgRef: setSvg, transform: state.transform, scale: state.scale, zoomIn, zoomOut, setScale, reset, zoomTo };
}

export function ToolboxZoom({ zoom, label, active }: { zoom: ZoomApi; label: string; active: boolean }) {
  const controls = usePageControls();
  if (!active || !controls.railSlot) return null;
  return createPortal(<ZoomControls zoom={zoom} label={label} />, controls.railSlot);
}

export function ZoomControls({ zoom, label }: { zoom: ZoomApi; label: string }) {
  const span = Math.log(MAX_SCALE / MIN_SCALE);
  const pos = Math.round((100 * Math.log(zoom.scale / MIN_SCALE)) / span);
  return (
    <span className="mc-tool-flyout" role="group" aria-label={label + " zoom"}>
      <button type="button" className="mc-tool-flyout-btn" aria-label="Zoom" title="Zoom">
        <SearchIcon size={14} />
      </button>
      <span className="mc-tool-flyout-pop">
        <input
          type="range"
          min={0}
          max={100}
          value={Math.max(0, Math.min(100, pos))}
          onChange={(e) => zoom.setScale(MIN_SCALE * Math.exp((span * Number(e.target.value)) / 100))}
          aria-label={label + " zoom level"}
        />
        <button type="button" className="mc-tool-flyout-reset mc-mono" onClick={zoom.reset} title="Reset zoom">
          {Math.round(zoom.scale * 100)}%
        </button>
      </span>
    </span>
  );
}

export function ClearSelection({ show, onClear }: { show: boolean; onClear: () => void }) {
  if (!show) return null;
  return (
    <button
      className="mc-map-clear"
      type="button"
      onClick={onClear}
      aria-label="Clear the selection and go back out"
      title="Clear the selection and go back out"
    >
      <XIcon size={15} />
    </button>
  );
}
