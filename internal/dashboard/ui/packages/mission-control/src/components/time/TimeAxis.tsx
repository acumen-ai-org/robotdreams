import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { ChevronsLeftIcon, ChevronsRightIcon } from "../icons";

export interface TimeAxisItem {
  id: string;
  t?: string;
  headline: string;
  severity?: string;
  type?: string;
  scope?: string;
}

export type TimeAxisDirection = "x" | "y";

const SHAPE = {
  x: { along: 208, across: 90, gutter: 30, lanes: 5 },
  y: { along: 74, across: 240, gutter: 84, lanes: 2 },
} as const;

const LANE_GAP = 8;
const OVERSCAN = 400;
const MIN_ZOOM = 1;
const ZOOM_NOT_CHOSEN = 0;
const OPENING_ZOOM_CAP = 24;
const OPENING_ROOM_PER_EVENT = 0.55;
const MAX_ZOOM = 4096;

const STEPS = [
  1e3,
  5e3,
  15e3,
  30e3,
  60e3,
  5 * 60e3,
  15 * 60e3,
  30 * 60e3,
  36e5,
  3 * 36e5,
  6 * 36e5,
  12 * 36e5,
  864e5,
  7 * 864e5,
  30 * 864e5,
];

const startOf = (ms: number, step: number) => Math.floor(ms / step) * step;

function tickLabel(ms: number, step: number): string {
  const d = new Date(ms);
  if (step >= 864e5) return d.toLocaleDateString(undefined, { month: "short", day: "numeric" });
  if (step >= 36e5) return d.toLocaleTimeString(undefined, { hour: "2-digit", minute: "2-digit" });
  return d.toLocaleTimeString(undefined, { hour: "2-digit", minute: "2-digit", second: "2-digit" });
}

export function TimeAxis({
  items,
  empty,
  selected,
  onSelect,
  direction = "x",
}: {
  items: TimeAxisItem[];
  empty?: string;
  selected?: string | null;
  onSelect?: (id: string) => void;
  direction?: TimeAxisDirection;
}) {
  const vertical = direction === "y";
  const shape = SHAPE[direction];
  const scrollRef = useRef<HTMLDivElement>(null);
  const [view, setView] = useState({ pos: 0, extent: 0, cross: 0 });
  const [zoom, setZoom] = useState(ZOOM_NOT_CHOSEN);

  const timed = useMemo(() => {
    const out: Array<{ item: TimeAxisItem; ms: number }> = [];
    for (const item of items) {
      const ms = item.t ? new Date(item.t).getTime() : NaN;
      if (!isNaN(ms)) out.push({ item, ms });
    }
    return out.sort((a, b) => a.ms - b.ms);
  }, [items]);

  const span = timed.length ? Math.max(1000, timed[timed.length - 1].ms - timed[0].ms) : 1;
  const t0 = timed.length ? timed[0].ms : 0;
  const tEnd = timed.length ? timed[timed.length - 1].ms : 0;

  const measure = useCallback(() => {
    const el = scrollRef.current;
    if (!el) return;
    setView({
      pos: vertical ? el.scrollTop : el.scrollLeft,
      extent: vertical ? el.clientHeight : el.clientWidth,
      cross: vertical ? el.clientWidth : el.clientHeight,
    });
  }, [vertical]);

  const measureRef = useRef<() => void>(() => {});
  measureRef.current = measure;

  useEffect(() => {
    measure();
    const el = scrollRef.current;
    if (!el || typeof ResizeObserver === "undefined") return;
    const ro = new ResizeObserver(measure);
    ro.observe(el);
    return () => ro.disconnect();
  }, [measure]);

  const extent = view.extent || 600;

  const fitZoom = Math.min(
    OPENING_ZOOM_CAP,
    Math.max(MIN_ZOOM, (timed.length * shape.along * OPENING_ROOM_PER_EVENT) / extent),
  );
  const z = zoom || fitZoom;

  const track = Math.max(extent, extent * z);
  const pxPerMs = track / span;
  const trackFull = track + shape.along;

  const atOf = (ms: number) => (vertical ? (tEnd - ms) * pxPerMs : (ms - t0) * pxPerMs);
  const msAt = (at: number) => (vertical ? tEnd - at / pxPerMs : t0 + at / pxPerMs);

  const laid = useMemo(() => {
    const laneEnd: number[] = [];
    const ordered = vertical ? [...timed].reverse() : timed;
    return ordered.map(({ item, ms }) => {
      const at = vertical ? (tEnd - ms) * pxPerMs : (ms - t0) * pxPerMs;
      let lane = -1;
      for (let i = 0; i < laneEnd.length; i++) {
        if (laneEnd[i] <= at - 6) {
          lane = i;
          break;
        }
      }
      if (lane < 0 && laneEnd.length < shape.lanes) lane = laneEnd.push(0) - 1;
      if (lane >= 0) laneEnd[lane] = at + shape.along;
      return { item, ms, at, lane };
    });
  }, [timed, pxPerMs, t0, tEnd, vertical, shape.along, shape.lanes]);

  const lanes = Math.min(
    shape.lanes,
    Math.max(
      1,
      laid.reduce((n, p) => Math.max(n, p.lane + 1), 0),
    ),
  );
  const body = lanes * (shape.across + LANE_GAP);
  const laneAt = (lane: number) => shape.gutter + lane * (shape.across + LANE_GAP);

  const from = view.pos - shape.along - OVERSCAN;
  const to = view.pos + extent + OVERSCAN;
  const shown = laid.filter((p) => p.at >= from && p.at <= to);
  const hidden = laid.filter((p) => p.lane < 0).length;

  const step = STEPS.find((s) => s * pxPerMs >= (vertical ? 56 : 90)) ?? STEPS[STEPS.length - 1];
  const ticks: Array<{ at: number; ms: number }> = [];
  if (timed.length) {
    const a = msAt(from);
    const b = msAt(to);
    for (let ms = startOf(Math.min(a, b), step); ms <= Math.max(a, b); ms += step) {
      const at = atOf(ms);
      if (at >= -200 && at <= track + 200) ticks.push({ at, ms });
    }
  }

  const opened = useRef(false);
  useEffect(() => {
    const el = scrollRef.current;
    if (opened.current || !el || !timed.length || !view.extent) return;
    opened.current = true;
    if (!vertical) el.scrollLeft = el.scrollWidth;
    measureRef.current();
  }, [timed.length, view.extent, vertical]);

  useEffect(() => {
    const el = scrollRef.current;
    if (!el || !selected || !opened.current) return;
    const hit = timed.find((x) => x.item.id === selected);
    if (!hit) return;
    const at = atOf(hit.ms);
    const pos = vertical ? el.scrollTop : el.scrollLeft;
    const size = vertical ? el.clientHeight : el.clientWidth;
    if (at < pos || at + shape.along > pos + size) {
      const next = Math.max(0, at - size / 2);
      el.scrollTo(vertical ? { top: next, behavior: "smooth" } : { left: next, behavior: "smooth" });
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [selected, timed, t0, pxPerMs, vertical, shape.along]);

  const nudge = (factor: number) => {
    const el = scrollRef.current;
    const next = Math.min(MAX_ZOOM, Math.max(MIN_ZOOM, z * factor));
    const size = el ? (vertical ? el.clientHeight : el.clientWidth) : 0;
    const centre = el ? ((vertical ? el.scrollTop : el.scrollLeft) + size / 2) / track : 0.5;
    setZoom(next);
    requestAnimationFrame(() => {
      const e2 = scrollRef.current;
      if (!e2) return;
      const s2 = vertical ? e2.clientHeight : e2.clientWidth;
      const at = centre * Math.max(s2, s2 * next) - s2 / 2;
      if (vertical) e2.scrollTop = at;
      else e2.scrollLeft = at;
    });
  };

  const jump = (to2: "start" | "end") => {
    const el = scrollRef.current;
    if (!el) return;
    const at = to2 === "start" ? 0 : vertical ? el.scrollHeight : el.scrollWidth;
    el.scrollTo(vertical ? { top: at, behavior: "smooth" } : { left: at, behavior: "smooth" });
  };

  const stamp = (ms: number) =>
    new Date(ms).toLocaleString(undefined, { month: "short", day: "numeric", hour: "2-digit", minute: "2-digit" });

  const walk = (delta: number) => {
    if (!onSelect || !timed.length) return;
    const order = vertical ? [...timed].reverse() : timed;
    const at = order.findIndex((x) => x.item.id === selected);
    const next = order[Math.min(Math.max((at < 0 ? -1 : at) + delta, 0), order.length - 1)];
    if (next) onSelect(next.item.id);
  };

  if (!timed.length) return <p className="mc-empty-state">{empty || "Nothing to show yet."}</p>;

  const first = { ms: vertical ? tEnd : t0, title: vertical ? "Go to the newest" : "Go to the oldest" };
  const last = { ms: vertical ? t0 : tEnd, title: vertical ? "Go to the oldest" : "Go to the newest" };

  return (
    <div className={"mc-tstrip" + (vertical ? " is-vertical" : "")}>
      <div className="mc-tstrip-tools">
        <span className="mc-tstrip-hint mc-label-muted">
          {timed.length} events
          {hidden > 0 && <> · {hidden} stacked, zoom in to separate</>}
        </span>
        <span className="mc-rendering-toggle" role="group" aria-label="Zoom the timeline">
          <button
            className="mc-rendering-option"
            type="button"
            aria-label="Zoom out"
            title="Zoom out"
            disabled={z <= MIN_ZOOM}
            onClick={() => nudge(0.5)}
          >
            −
          </button>
          <button
            className="mc-rendering-option"
            type="button"
            aria-label="Fit the whole span"
            title="Fit the whole span"
            onClick={() => {
              setZoom(MIN_ZOOM);
              requestAnimationFrame(() => jump("start"));
            }}
          >
            ⤢
          </button>
          <button
            className="mc-rendering-option"
            type="button"
            aria-label="Zoom in"
            title="Zoom in"
            disabled={z >= MAX_ZOOM}
            onClick={() => nudge(2)}
          >
            +
          </button>
        </span>
      </div>
      {/* eslint-disable-next-line jsx-a11y/no-noninteractive-element-interactions */}
      <div
        className="mc-tstrip-scroll"
        ref={scrollRef}
        onScroll={measure}
        // eslint-disable-next-line jsx-a11y/no-noninteractive-tabindex
        tabIndex={0}
        role="group"
        aria-label={timed.length + " events on a time axis; arrow keys to step through them"}
        onKeyDown={(e) => {
          const fwdKey = vertical ? "ArrowDown" : "ArrowRight";
          const backKey = vertical ? "ArrowUp" : "ArrowLeft";
          if (e.key === fwdKey) {
            e.preventDefault();
            walk(1);
          } else if (e.key === backKey) {
            e.preventDefault();
            walk(-1);
          } else if (e.key === "Home") {
            e.preventDefault();
            jump("start");
          } else if (e.key === "End") {
            e.preventDefault();
            jump("end");
          }
        }}
        style={vertical ? undefined : { height: body + shape.gutter + 26 }}
      >
        <div
          className="mc-tstrip-track"
          style={
            vertical
              ? { height: trackFull, minWidth: shape.gutter + lanes * (shape.across + LANE_GAP) }
              : { width: trackFull, height: body + shape.gutter }
          }
        >
          <div className="mc-tstrip-axis" style={vertical ? { left: shape.gutter - 12 } : { top: body }}>
            {ticks.map((t) => (
              <span className="mc-tstrip-tick" key={t.ms} style={vertical ? { top: t.at } : { left: t.at }}>
                <span className="mc-tstrip-tick-label mc-mono">{tickLabel(t.ms, step)}</span>
              </span>
            ))}
          </div>

          {shown.map(({ item, at, lane }) => {
            const sev = (item.severity || "info").toLowerCase();
            const segs = (item.scope || "").split("/").filter(Boolean);
            const site = segs.length ? segs[segs.length - 1] : "";
            const axisAt = shape.gutter - 12;
            const cardAt = lane < 0 ? axisAt : laneAt(lane);
            const stem = (
              <span
                className={"mc-tstrip-stem mc-sev-dot-" + sev}
                key={item.id + ":stem"}
                style={
                  vertical
                    ? { top: at + 10, left: axisAt, width: lane < 0 ? 8 : cardAt + shape.across - axisAt }
                    : {
                        left: at,
                        top: lane < 0 ? body - 8 : lane * (shape.across + LANE_GAP),
                        height: lane < 0 ? 8 : body - lane * (shape.across + LANE_GAP),
                      }
                }
              />
            );
            if (lane < 0) return stem;
            return (
              <span key={item.id}>
                {stem}
                <button
                  type="button"
                  tabIndex={-1}
                  aria-pressed={selected === item.id}
                  className={"mc-tstrip-card" + (selected === item.id ? " is-selected" : "")}
                  style={
                    vertical
                      ? { top: at, left: laneAt(lane), width: shape.across, height: shape.along }
                      : { left: at, top: lane * (shape.across + LANE_GAP), width: shape.along, height: shape.across }
                  }
                  onClick={(e) => {
                    onSelect?.(item.id);
                    e.currentTarget.blur();
                    scrollRef.current?.focus({ preventScroll: true });
                  }}
                >
                  <span className="mc-tstrip-when mc-mono">
                    {new Date(item.t!).toLocaleTimeString(undefined, { hour: "2-digit", minute: "2-digit" })}
                  </span>
                  <span className="mc-tstrip-headline">{item.headline}</span>
                  <span className="mc-tstrip-meta">
                    <span className={"mc-sev mc-sev-" + sev}>{sev}</span>
                    {site && <span className="mc-tstrip-site">{site}</span>}
                  </span>
                </button>
              </span>
            );
          })}
        </div>
      </div>

      <div className="mc-tstrip-ends">
        <button className="mc-tstrip-end" type="button" onClick={() => jump("start")} title={first.title}>
          <ChevronsLeftIcon size={14} />
          <span className="mc-mono">{stamp(first.ms)}</span>
        </button>
        <span className="mc-tstrip-ends-rule" aria-hidden="true" />
        <button className="mc-tstrip-end" type="button" onClick={() => jump("end")} title={last.title}>
          <span className="mc-mono">{stamp(last.ms)}</span>
          <ChevronsRightIcon size={14} />
        </button>
      </div>
    </div>
  );
}

export default TimeAxis;
