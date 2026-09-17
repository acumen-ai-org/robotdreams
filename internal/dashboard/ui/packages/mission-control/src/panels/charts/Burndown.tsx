import { useMemo } from "react";
import type { ChartConfiguration } from "chart.js";
import { ChartCanvas } from "./ChartCanvas";
import { axisScale, baseOptions, chartColors } from "./chartTheme";
import { useThemeRoot } from "../../state/ThemeContext";
import { fmtNum, pickTimeFormat } from "../../lib/format";
import type { Panel, SeriesPoint } from "../../lib/types";

interface Pt {
  x: number;
  y: number;
}

function extract(points: SeriesPoint[] | undefined): Pt[] {
  const raw: Pt[] = [];
  (points || []).forEach((p, i) => {
    if (!p || typeof p !== "object") return;
    const o = p;
    const y = typeof o.v === "number" ? o.v : typeof o.value === "number" ? o.value : o.y;
    if (typeof y !== "number" || !isFinite(y)) return;
    const ms = o.t ? new Date(o.t).getTime() : NaN;
    raw.push({ x: isFinite(ms) ? ms : i, y });
  });
  const perX = new Map<number, { sum: number; n: number }>();
  for (const p of raw) {
    const at = perX.get(p.x) || { sum: 0, n: 0 };
    at.sum += p.y;
    at.n += 1;
    perX.set(p.x, at);
  }
  return [...perX.entries()].map(([x, v]) => ({ x, y: v.sum / v.n })).sort((a, b) => a.x - b.x);
}

export default function Burndown({ panel }: { panel: Panel }) {
  const themeRoot = useThemeRoot();
  const pts = useMemo(() => extract(panel.series), [panel.series]);
  const fmtX = useMemo(() => pickTimeFormat(pts.map((p) => new Date(p.x).toISOString())), [pts]);

  const build = (): ChartConfiguration => {
    const c = chartColors(themeRoot);
    const first = pts[0];
    const last = pts[pts.length - 1];
    const span = last.x - first.x || 1;
    const tick = (v: unknown) => fmtX(new Date(Number(v)).toISOString());
    return {
      type: "line",
      data: {
        datasets: [
          {
            label: "remaining",
            data: pts.map((p) => ({ x: p.x, y: p.y })),
            borderColor: c.brandStrong,
            borderWidth: 2,
            stepped: true,
            pointRadius: 0,
            pointHoverRadius: 4,
            pointHoverBackgroundColor: c.brandStrong,
            fill: false,
          },
          {
            label: "ideal",
            data: pts.map((p) => ({ x: p.x, y: first.y * (1 - (p.x - first.x) / span) })),
            borderColor: c.muted,
            borderWidth: 1,
            borderDash: [4, 4],
            pointRadius: 0,
            fill: false,
          },
        ],
      },
      options: {
        ...baseOptions(themeRoot),
        parsing: false,
        normalized: true,
        scales: {
          x: {
            ...axisScale(themeRoot, { grid: false }),
            type: "linear",
            ticks: { ...axisScale(themeRoot).ticks, callback: tick },
          },
          y: {
            ...axisScale(themeRoot),
            beginAtZero: true,
            ticks: { ...axisScale(themeRoot).ticks, callback: (v) => fmtNum(v) },
          },
        },
        plugins: {
          ...baseOptions(themeRoot).plugins,
          tooltip: {
            ...baseOptions(themeRoot).plugins?.tooltip,
            callbacks: {
              title: (items) => tick(items[0]?.parsed.x),
              label: (ctx) => (ctx.datasetIndex === 1 ? "ideal " : "remaining ") + fmtNum(ctx.parsed.y),
            },
          },
        },
      },
    };
  };

  if (pts.length < 2) {
    return <p className="mc-empty-state">Not enough of the plan&rsquo;s history to burn down.</p>;
  }

  const first = pts[0];
  const last = pts[pts.length - 1];
  const remaining = last.y;
  const dayMs = 86_400_000;
  const perDay = (first.y - last.y) / Math.max(1e-6, (last.x - first.x) / dayMs);
  const daysLeftInWindow = Math.max(0, (last.x - first.x) / dayMs);
  const projection = perDay > 0 ? remaining / perDay : Number.POSITIVE_INFINITY;

  return (
    <div className="mc-chart-wrap">
      <ChartCanvas build={build} deps={[pts]} label={(panel.title || "remaining work") + " against the plan"} />
      <div className="mc-chart-caption mc-label-muted">
        {fmtNum(remaining)} remaining
        {perDay > 0 ? (
          <>
            {" "}
            &middot; burning {fmtNum(perDay)}/day &middot;{" "}
            {projection <= daysLeftInWindow
              ? "on track to finish inside the window"
              : "about " + Math.ceil(projection) + " days to zero at this rate"}
          </>
        ) : (
          <> &middot; not burning down: the remaining work is flat or rising</>
        )}
      </div>
    </div>
  );
}
