import { useMemo } from "react";
import { scaleTime } from "d3-scale";
import { timeFormat } from "d3-time-format";
import { humanDuration } from "../lib/format";
import type { Panel } from "../lib/types";

const SPANS_CAP = 100;
const fmtAxis = timeFormat("%b %d %H:%M");

const ONGOING = NaN;

interface ParsedSpan {
  name: string;
  label: string;
  scope: string;
  start: number;
  end: number;
}

export function SpansGantt({ panel }: { panel: Panel }) {
  const parsed = useMemo<ParsedSpan[]>(
    () =>
      (panel.spans || [])
        .map((sp) => ({
          name: sp.name || "span",
          label: sp.label || "",
          scope: sp.scope || "",
          start: new Date(sp.start).getTime(),
          end: sp.end ? new Date(sp.end).getTime() : ONGOING,
        }))
        .filter((sp) => isFinite(sp.start)),
    [panel.spans],
  );

  if (!parsed.length) return <p className="mc-empty-state">No spans.</p>;

  const now = Date.now();
  let minT = Infinity;
  let maxT = -Infinity;
  parsed.forEach((sp) => {
    minT = Math.min(minT, sp.start);
    maxT = Math.max(maxT, isFinite(sp.end) ? sp.end : now);
  });
  const x = scaleTime()
    .domain([new Date(minT), new Date(maxT)])
    .range([0, 100]);

  const sorted = parsed.slice().sort((a, b) => a.name.localeCompare(b.name) || a.start - b.start);
  const shown = sorted.slice(0, SPANS_CAP);

  return (
    <div className="mc-spans-chart">
      <div className="mc-spans-axis" aria-hidden="true">
        <span>{fmtAxis(new Date(minT))}</span>
        <span>{fmtAxis(new Date(maxT))}</span>
      </div>
      {shown.map((sp, i) => {
        const end = isFinite(sp.end) ? sp.end : now;
        const left = x(new Date(sp.start));
        const width = Math.max(0.5, x(new Date(end)) - left);
        const title =
          sp.name +
          (sp.label ? " — " + sp.label : "") +
          (isFinite(sp.end) ? " (" + humanDuration(end - sp.start) + ")" : " (ongoing)");
        return (
          <div className="mc-span-row" key={i}>
            <span className="mc-span-name" title={title}>
              {sp.name}
              {sp.label ? " · " + sp.label : ""}
            </span>
            <span className="mc-span-track">
              <span
                className={"mc-span-bar" + (isFinite(sp.end) ? "" : " mc-open")}
                title={title}
                style={{ left: left.toFixed(2) + "%", width: Math.min(width, 100 - left).toFixed(2) + "%" }}
              ></span>
            </span>
          </div>
        );
      })}
      {parsed.length > shown.length && (
        <p className="mc-empty-state">
          Showing {shown.length} of {parsed.length} spans.
        </p>
      )}
    </div>
  );
}
