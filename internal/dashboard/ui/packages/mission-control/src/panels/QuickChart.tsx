import { useEffect, useMemo, useRef } from "react";
import { ArcElement, Chart, DoughnutController, Legend, Tooltip } from "chart.js";
import { fmtNum, unitLabel } from "../lib/format";
import type { Panel } from "../lib/types";
import { cssVar } from "../lib/themeRead";
import { useThemeRoot } from "../state/ThemeContext";

Chart.register(DoughnutController, ArcElement, Tooltip, Legend);

export default function QuickChart({ panel }: { panel: Panel }) {
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const root = useThemeRoot();
  const kpi = panel.kpi;

  const { value, max } = useMemo(() => {
    const v = typeof kpi?.value === "number" ? kpi.value : 0;
    const m = Math.max(1, Math.pow(10, Math.ceil(Math.log10(Math.max(1, v)))));
    return { value: v, max: m };
  }, [kpi]);

  useEffect(() => {
    const canvas = canvasRef.current;
    if (!canvas) return;
    const chart = new Chart(canvas, {
      type: "doughnut",
      data: {
        datasets: [
          {
            data: [value, Math.max(0, max - value)],
            backgroundColor: [cssVar(root, "--brand-strong"), cssVar(root, "--surface-soft")],
            borderWidth: 0,
          },
        ],
      },
      options: {
        circumference: 180,
        rotation: -90,
        cutout: "72%",
        plugins: { legend: { display: false }, tooltip: { enabled: false } },
        responsive: true,
        maintainAspectRatio: true,
      },
    });
    return () => chart.destroy();
  }, [value, max, root]);

  if (!kpi) return <p className="mc-empty-state">No value.</p>;
  const unit = unitLabel(kpi.unit);

  return (
    <div className="mc-quickchart-wrap">
      <canvas ref={canvasRef} role="img" aria-label={(kpi.name || "gauge") + ": " + fmtNum(value)} />
      <div className="mc-kpi-card">
        <span className="mc-kpi">
          <span className="mc-kpi-value">
            {fmtNum(value)}
            {unit ? " " + unit : ""}
          </span>
          <span className="mc-kpi-name mc-label-muted">{kpi.name || ""}</span>
        </span>
      </div>
    </div>
  );
}
