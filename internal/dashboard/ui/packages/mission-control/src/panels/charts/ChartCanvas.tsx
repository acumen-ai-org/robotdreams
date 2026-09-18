import { useEffect, useRef, useState } from "react";
import { useThemeRoot } from "../../state/ThemeContext";
import {
  BarController,
  BarElement,
  CategoryScale,
  Chart,
  Filler,
  LineController,
  LineElement,
  LinearScale,
  PointElement,
  Tooltip,
  type ChartConfiguration,
} from "chart.js";

Chart.register(
  LineController,
  LineElement,
  PointElement,
  BarController,
  BarElement,
  CategoryScale,
  LinearScale,
  Filler,
  Tooltip,
);

interface Props {
  build: () => ChartConfiguration;
  deps: unknown[];
  height?: number;
  label: string;
}

export function ChartCanvas({ build, deps, height = 200, label }: Props) {
  const ref = useRef<HTMLCanvasElement>(null);
  const [themeTick, setThemeTick] = useState(0);
  const root = useThemeRoot();

  useEffect(() => {
    const bump = () => setThemeTick((n) => n + 1);
    const mo = new MutationObserver(bump);
    mo.observe(root, { attributes: true, attributeFilter: ["data-theme", "class"] });
    const mq = window.matchMedia?.("(prefers-color-scheme: dark)");
    mq?.addEventListener?.("change", bump);
    return () => {
      mo.disconnect();
      mq?.removeEventListener?.("change", bump);
    };
  }, [root]);

  useEffect(() => {
    const canvas = ref.current;
    if (!canvas) return;
    const chart = new Chart(canvas, build());
    return () => chart.destroy();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [...deps, themeTick]);

  return (
    <div className="mc-chart-canvas" style={{ height }}>
      <canvas ref={ref} role="img" aria-label={label} />
    </div>
  );
}
