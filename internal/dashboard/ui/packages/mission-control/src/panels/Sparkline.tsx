import { useMemo } from "react";
import { scaleLinear } from "d3-scale";
import { line as d3line, curveMonotoneX } from "d3-shape";
import { pointValues } from "../lib/format";
import type { SeriesPoint } from "../lib/types";

const W = 120;
const H = 28;
const PAD = 2;

export function Sparkline({ points }: { points: SeriesPoint[] | undefined }) {
  const vals = useMemo(() => pointValues(points), [points]);
  if (vals.length < 2) return null;

  const x = scaleLinear()
    .domain([0, vals.length - 1])
    .range([PAD, W - PAD]);
  const y = scaleLinear()
    .domain([Math.min(...vals), Math.max(...vals) === Math.min(...vals) ? Math.min(...vals) + 1 : Math.max(...vals)])
    .range([H - PAD, PAD]);
  const gen = d3line<number>()
    .x((_, i) => x(i))
    .y((v) => y(v))
    .curve(curveMonotoneX);

  return (
    <svg
      className="mc-sparkline"
      viewBox={`0 0 ${W} ${H}`}
      width={W}
      height={H}
      role="img"
      aria-label="trend sparkline"
    >
      <path d={gen(vals) || undefined} fill="none" strokeWidth={1.5} />
    </svg>
  );
}
