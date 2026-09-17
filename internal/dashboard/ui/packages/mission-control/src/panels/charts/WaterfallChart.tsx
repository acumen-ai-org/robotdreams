import { useMemo } from "react";
import type { ChartConfiguration } from "chart.js";
import { ChartCanvas } from "./ChartCanvas";
import { alpha, axisScale, baseOptions, chartColors } from "./chartTheme";
import { useThemeRoot } from "../../state/ThemeContext";
import { cellText, fmtNum } from "../../lib/format";
import { columnIndex, labelColumn, lowerColumns, numericColumn, sumByLabel } from "../tableData";
import type { Panel } from "../../lib/types";

interface Step {
  name: string;
  amount: number;
  total: boolean;
  from: number;
  to: number;
}

function read(panel: Panel): Step[] {
  const cols = lowerColumns(panel.table);
  const rows = (panel.table?.rows || []) as unknown[];
  if (!cols.length || !rows.length) return [];
  const label = labelColumn(cols, ["step", "stage", "driver"]);
  if (label < 0) return [];
  const value =
    columnIndex(cols, ["amount", "value", "delta"]) >= 0
      ? columnIndex(cols, ["amount", "value", "delta"])
      : numericColumn(cols, rows, label);
  if (value < 0) return [];
  const kind = cols.indexOf("kind");

  const summed = sumByLabel(rows, label, value);
  let running = 0;
  return summed.map((s, i) => {
    const total = kind >= 0 ? cellText(s.row[kind]).toLowerCase() === "total" : i === 0 || i === summed.length - 1;
    const from = total ? 0 : running;
    const to = total ? s.value : running + s.value;
    running = to;
    return { name: s.label, amount: s.value, total, from, to };
  });
}

export default function WaterfallChart({ panel }: { panel: Panel }) {
  const themeRoot = useThemeRoot();
  const steps = useMemo(() => read(panel), [panel]);

  const build = (): ChartConfiguration => {
    const c = chartColors(themeRoot);
    const colour = (s: Step) => (s.total ? c.brandStrong : s.amount < 0 ? c.success : c.warm);
    return {
      type: "bar",
      data: {
        labels: steps.map((s) => s.name),
        datasets: [
          {
            data: steps.map((s) => [s.from, s.to] as [number, number]),
            backgroundColor: steps.map((s) => alpha(colour(s), s.total ? 0.85 : 0.7)),
            borderColor: steps.map(colour),
            borderWidth: 1,
            borderRadius: 3,
            barPercentage: 0.7,
          },
        ],
      },
      options: {
        ...baseOptions(themeRoot),
        indexAxis: "y",
        scales: {
          x: { ...axisScale(themeRoot), ticks: { ...axisScale(themeRoot).ticks, callback: (v) => fmtNum(v) } },
          y: { ...axisScale(themeRoot, { grid: false }), ticks: { ...axisScale(themeRoot).ticks, autoSkip: false } },
        },
        plugins: {
          ...baseOptions(themeRoot).plugins,
          tooltip: {
            ...baseOptions(themeRoot).plugins?.tooltip,
            callbacks: {
              label: (ctx) => {
                const s = steps[ctx.dataIndex];
                if (!s) return "";
                return s.total
                  ? fmtNum(s.amount)
                  : (s.amount >= 0 ? "+" : "") + fmtNum(s.amount) + " → " + fmtNum(s.to);
              },
            },
          },
        },
      },
    };
  };

  if (!steps.length) return <p className="mc-empty-state">No steps reported.</p>;

  const last = steps[steps.length - 1];
  const carried = steps.length >= 3 && last.total ? steps[steps.length - 2].to : null;
  const gap = carried === null ? 0 : last.to - carried;

  return (
    <div className="mc-chart-wrap">
      <ChartCanvas
        build={build}
        deps={[steps]}
        height={Math.max(140, steps.length * 30 + 30)}
        label={(panel.title || "value") + " broken into the steps that moved it"}
      />
      {carried !== null && Math.abs(gap) >= 0.5 && (
        <p className="mc-chart-caption mc-label-muted">
          The steps add up to {fmtNum(carried)}, but the closing total is {fmtNum(last.to)} — a gap of{" "}
          {(gap > 0 ? "+" : "−") + fmtNum(Math.abs(gap))}. The bars are as reported; the bridge does not close.
        </p>
      )}
    </div>
  );
}
