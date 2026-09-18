import { useMemo } from "react";
import type { ChartConfiguration } from "chart.js";
import { ChartCanvas } from "./ChartCanvas";
import { alpha, axisScale, baseOptions, chartColors } from "./chartTheme";
import { useThemeRoot } from "../../state/ThemeContext";
import { fmtNum } from "../../lib/format";
import { labelColumn, lowerColumns, numericColumn, sumByLabel, type LabelledValue } from "../tableData";
import type { Panel } from "../../lib/types";

const MAX_BARS = 12;
const ROW_HEIGHT = 22;

function read(panel: Panel): LabelledValue[] {
  const cols = lowerColumns(panel.table);
  const rows = (panel.table?.rows || []) as unknown[];
  if (!cols.length || !rows.length) return [];
  const label = labelColumn(cols, ["unit", "name", "node"]);
  if (label < 0) return [];
  const value = numericColumn(cols, rows, label);
  if (value < 0) return [];
  return sumByLabel(rows, label, value).sort((a, b) => b.value - a.value);
}

export default function RankedBarsChart({ panel }: { panel: Panel }) {
  const themeRoot = useThemeRoot();
  const all = useMemo(() => read(panel), [panel]);
  const bars = all.slice(0, MAX_BARS);
  const hidden = all.length - bars.length;

  const build = (): ChartConfiguration => {
    const c = chartColors(themeRoot);
    return {
      type: "bar",
      data: {
        labels: bars.map((b) => b.label),
        datasets: [
          {
            data: bars.map((b) => b.value),
            backgroundColor: bars.map((_, i) => (i === 0 ? c.brandStrong : alpha(c.brand, 0.85))),
            borderRadius: 3,
            barPercentage: 0.8,
            categoryPercentage: 0.85,
          },
        ],
      },
      options: {
        ...baseOptions(themeRoot),
        indexAxis: "y",
        scales: {
          x: {
            ...axisScale(themeRoot),
            beginAtZero: true,
            ticks: { ...axisScale(themeRoot).ticks, callback: (v) => fmtNum(v) },
          },
          y: { ...axisScale(themeRoot, { grid: false }), ticks: { ...axisScale(themeRoot).ticks, autoSkip: false } },
        },
        plugins: {
          ...baseOptions(themeRoot).plugins,
          tooltip: {
            ...baseOptions(themeRoot).plugins?.tooltip,
            callbacks: { label: (ctx) => fmtNum(ctx.parsed.x) },
          },
        },
      },
    };
  };

  if (!bars.length) return <p className="mc-empty-state">No rows.</p>;

  return (
    <div className="mc-chart-wrap">
      <ChartCanvas
        build={build}
        deps={[bars]}
        height={Math.max(120, bars.length * ROW_HEIGHT + 30)}
        label={(panel.title || "values") + " ranked, highest first"}
      />
      {hidden > 0 && <p className="mc-chart-caption mc-label-muted">{hidden} more not shown</p>}
    </div>
  );
}
