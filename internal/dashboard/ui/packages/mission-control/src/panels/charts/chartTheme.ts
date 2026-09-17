import type { ChartOptions, Chart as ChartType } from "chart.js";
import { cssVar } from "../../lib/themeRead";

export { cssVar };

export function chartColors(root: HTMLElement) {
  return {
    text: cssVar(root, "--text", "#10141d"),
    muted: cssVar(root, "--text-muted", "#465365"),
    border: cssVar(root, "--border", "#e3e9ef"),
    surface: cssVar(root, "--surface", "#fff"),
    surfaceSoft: cssVar(root, "--surface-soft", "#eef5fa"),
    brand: cssVar(root, "--brand", "#a9d9f2"),
    brandStrong: cssVar(root, "--brand-strong", "#174f7d"),
    success: cssVar(root, "--success", "#147a5a"),
    warm: cssVar(root, "--warm", "#9b6500"),
    danger: cssVar(root, "--danger", "#b4233b"),
  };
}

export function alpha(color: string, a: number): string {
  return `color-mix(in srgb, ${color} ${Math.round(a * 100)}%, transparent)`;
}

export function baseOptions(root: HTMLElement): ChartOptions {
  const c = chartColors(root);
  return {
    responsive: true,
    maintainAspectRatio: false,
    animation: { duration: 260 },
    interaction: { mode: "nearest", intersect: false },
    plugins: {
      legend: { display: false },
      tooltip: {
        backgroundColor: c.text,
        titleColor: c.surface,
        bodyColor: c.surface,
        padding: 8,
        displayColors: false,
        cornerRadius: 6,
      },
    },
    font: { family: cssVar(root, "--font-body", "system-ui"), size: 11 },
  };
}

export function axisScale(root: HTMLElement, opts: { grid?: boolean; ticksCallback?: (v: unknown) => string } = {}) {
  const c = chartColors(root);
  return {
    grid: {
      display: opts.grid !== false,
      color: c.border,
      drawTicks: false,
      tickLength: 0,
    },
    border: { display: false },
    ticks: {
      color: c.muted,
      padding: 6,
      maxRotation: 0,
      autoSkipPadding: 12,
      ...(opts.ticksCallback ? { callback: opts.ticksCallback } : {}),
    },
  };
}

export function destroy(chart: ChartType | null | undefined): void {
  chart?.destroy();
}
