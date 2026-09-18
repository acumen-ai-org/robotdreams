import { useMemo } from "react";
import type { ChartConfiguration } from "chart.js";
import { ChartCanvas } from "./ChartCanvas";
import { alpha, axisScale, baseOptions, chartColors } from "./chartTheme";
import { useThemeRoot } from "../../state/ThemeContext";
import { fmtNum, pickTimeFormat } from "../../lib/format";
import type { Panel, SeriesPoint } from "../../lib/types";

interface Pt {
  x: number;
  y: number;
}

function extract(points: SeriesPoint[] | undefined): { pts: Pt[]; timed: boolean; contributors: number } {
  const pts: Pt[] = [];
  let timed = true;
  (points || []).forEach((p, i) => {
    let y: number | undefined;
    let t: number | undefined;
    if (typeof p === "number") {
      y = p;
      timed = false;
    } else if (p && typeof p === "object") {
      const o = p;
      y = typeof o.value === "number" ? o.value : typeof o.v === "number" ? o.v : o.y;
      if (o.t) {
        const ms = new Date(o.t).getTime();
        if (isFinite(ms)) t = ms;
      }
    }
    if (typeof y === "number" && isFinite(y)) {
      if (t == null) timed = false;
      pts.push({ x: t ?? i, y });
    }
  });
  if (!timed) pts.forEach((pt, i) => (pt.x = i));
  pts.sort((a, b) => a.x - b.x);

  const perX = new Map<number, { sum: number; n: number }>();
  for (const p of pts) {
    const at = perX.get(p.x) || { sum: 0, n: 0 };
    at.sum += p.y;
    at.n += 1;
    perX.set(p.x, at);
  }
  const collapsed = perX.size < pts.length;
  const merged: Pt[] = [...perX.entries()].map(([x, v]) => ({ x, y: v.sum / v.n }));
  const contributors = collapsed ? Math.round(pts.length / perX.size) : 1;
  return { pts: merged, timed, contributors };
}

export default function LineChart({ panel }: { panel: Panel }) {
  const themeRoot = useThemeRoot();
  const { pts, timed, contributors } = useMemo(() => extract(panel.series), [panel.series]);
  const fmtX = useMemo(() => pickTimeFormat(timed ? pts.map((p) => new Date(p.x).toISOString()) : []), [pts, timed]);

  const build = (): ChartConfiguration => {
    const c = chartColors(themeRoot);
    const mean = pts.reduce((sum, p) => sum + p.y, 0) / (pts.length || 1);
    const tick = (v: unknown) => (timed ? fmtX(new Date(Number(v)).toISOString()) : fmtNum(v));
    return {
      type: "line",
      data: {
        datasets: [
          {
            data: pts.map((p) => ({ x: p.x, y: p.y })),
            borderColor: c.brandStrong,
            borderWidth: 2,
            backgroundColor: alpha(c.brand, 0.45),
            fill: "origin",
            tension: 0.25,
            pointRadius: 0,
            pointHoverRadius: 4,
            pointHoverBackgroundColor: c.brandStrong,
          },
          {
            label: "average",
            data: pts.map((p) => ({ x: p.x, y: mean })),
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
              title: (items) => (timed ? tick(items[0]?.parsed.x) : ""),
              label: (ctx) => (ctx.datasetIndex === 1 ? "average " : "") + fmtNum(ctx.parsed.y),
            },
          },
        },
      },
    };
  };

  if (pts.length < 2) {
    return <p className="mc-empty-state">Not enough data points to chart.</p>;
  }

  const values = pts.map((p) => p.y);
  return (
    <div className="mc-chart-wrap">
      <ChartCanvas build={build} deps={[pts]} label={(panel.title || "series") + " over time"} />
      <div className="mc-chart-caption mc-label-muted">
        min {fmtNum(Math.min(...values))} · max {fmtNum(Math.max(...values))} · last {fmtNum(values[values.length - 1])}
        {contributors > 1 && " · mean of ~" + contributors + " scopes per point"}
      </div>
    </div>
  );
}
