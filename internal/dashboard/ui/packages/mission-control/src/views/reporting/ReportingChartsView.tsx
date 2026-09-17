import { useEffect, useMemo, useRef, useState } from "react";
import {
  CategoryScale,
  Chart,
  Filler,
  LineController,
  LineElement,
  LinearScale,
  PointElement,
  TimeScale,
  Tooltip,
} from "chart.js";
import { useConnection } from "../../state/ConnectionContext";
import { reportingRoute, type Route } from "../../lib/routes";
import { KpiStat, StatusBadge, tileName } from "../../components/shared";
import { Link } from "../../components/Link";
import { seriesMovement, type Movement } from "../../lib/triage";
import { fmtNum } from "../../lib/format";
import { useTheme, useThemeRoot } from "../../state/ThemeContext";
import { cssVar } from "../../lib/themeRead";
import type { ReportResponse, SeriesPoint, SummaryTile } from "../../lib/types";

Chart.register(LineController, LineElement, PointElement, LinearScale, CategoryScale, TimeScale, Tooltip, Filler);

const MAX_DEFINITIONS = 12;

function seriesPoints(series: SeriesPoint[] | undefined): { label: string; y: number }[] {
  const out: { label: string; y: number }[] = [];
  (series || []).forEach((p, i) => {
    if (typeof p === "number") {
      out.push({ label: String(i), y: p });
      return;
    }
    if (!p || typeof p !== "object") return;
    const o = p;
    const y = typeof o.value === "number" ? o.value : typeof o.v === "number" ? o.v : o.y;
    if (typeof y === "number" && isFinite(y))
      out.push({ label: o.t ? new Date(o.t).toLocaleTimeString() : String(i), y });
  });
  return out;
}

const MAX_SHOWN_PCT = 10;

function MovementTag({ move, unit }: { move?: Movement; unit?: string }) {
  if (!move) return <span className="mc-delta-tag is-flat">—</span>;
  const dir = move.abs > 0 ? "up" : move.abs < 0 ? "down" : "flat";
  const arrow = dir === "up" ? "\u2191" : dir === "down" ? "\u2193" : "\u2192";
  return (
    <span className={"mc-delta-tag is-" + dir} title={"from " + fmtNum(move.from) + " to " + fmtNum(move.to)}>
      <span aria-hidden="true">{arrow}</span>
      {fmtNum(Math.abs(move.abs))}
      {unit ? " " + unit : ""}
      {move.pct !== undefined && Math.abs(move.pct) <= MAX_SHOWN_PCT && (
        <span className="mc-delta-pct">{(move.pct * 100).toFixed(Math.abs(move.pct) < 0.1 ? 1 : 0)}%</span>
      )}
    </span>
  );
}

interface Props {
  route: Route;
  tiles: SummaryTile[];
  refreshTick: number;
  active: boolean;
  periodQuery?: string;
}

export default function ReportingChartsView({ route, tiles, refreshTick, active, periodQuery = "" }: Props) {
  const conn = useConnection();
  const { theme } = useTheme();
  const [reports, setReports] = useState<Map<string, ReportResponse>>(new Map());
  const [error, setError] = useState("");

  const names = useMemo(() => tiles.map(tileName).slice(0, MAX_DEFINITIONS), [tiles]);

  useEffect(() => {
    if (!active || !conn.hasToken || !names.length) return;
    let stale = false;
    Promise.all(
      names.map((n) =>
        conn
          .apiJSON<ReportResponse>(
            "/api/reports/report?definition=" +
              encodeURIComponent(n) +
              "&scope=" +
              encodeURIComponent(route.scope) +
              periodQuery,
          )
          .then((r) => [n, r] as const),
      ),
    )
      .then((pairs) => {
        if (stale) return;
        setReports(new Map(pairs));
        setError("");
      })
      .catch((e) => {
        if (!stale) setError("Could not load reports: " + (e instanceof Error ? e.message : String(e)));
      });
    return () => {
      stale = true;
    };
  }, [active, conn, conn.hasToken, conn.session, names, route.scope, refreshTick, periodQuery]);

  if (error) return <p className="mc-form-error">{error}</p>;
  if (!tiles.length) return <p className="mc-empty-state">No reports match this scope and filter.</p>;

  const cards = tiles.slice(0, MAX_DEFINITIONS).map((t) => {
    const name = tileName(t);
    const rep = reports.get(name);
    const panel = (rep?.panels || []).find((p) => p.data_kind === "series");
    const pts = seriesPoints(panel?.series);
    const move = seriesMovement(pts.map((p) => p.y));
    return { tile: t, name, rep, panel, pts, move };
  });
  cards.sort((a, b) => {
    const sa = a.move?.pct === undefined ? -1 : Math.abs(a.move.pct);
    const sb = b.move?.pct === undefined ? -1 : Math.abs(b.move.pct);
    return sb - sa;
  });

  return (
    <div>
      <div className="mc-delta-grid">
        {cards.map(({ tile: t, name, rep, panel, pts, move }) => {
          const kpis = (rep?.summary?.kpis || t.kpis || []).slice(0, 3);
          return (
            <Link
              key={name}
              className={"mc-chart-card mc-status-" + (t.status || "none")}
              to={reportingRoute(route, { report: name })}
            >
              <span className="mc-chart-card-head">
                <span className="mc-chart-card-name">{name}</span>
                <StatusBadge status={t.status} />
              </span>

              <span className="mc-delta-line">
                <MovementTag move={move} unit={panel?.kpi?.unit} />
                <span className="mc-label-muted">
                  {panel?.title ? "in " + panel.title.toLowerCase() : "over the window"}
                </span>
              </span>

              <span className="mc-chart-card-plot">
                {pts.length >= 2 ? (
                  <SeriesLine points={pts} baseline={move?.from} theme={theme} title={panel?.title || name} />
                ) : (
                  <span className="mc-empty-state">{rep ? "No series data." : "Loading\u2026"}</span>
                )}
              </span>

              <span className="mc-chart-card-kpis">
                {kpis.map((k, i) => (
                  <span className="mc-delta-kpi" key={k.name || i}>
                    <KpiStat kpi={k} />
                    {typeof k.delta === "number" && (
                      <MovementTag
                        move={{ from: (k.value ?? 0) - k.delta, to: k.value ?? 0, abs: k.delta, pct: undefined }}
                        unit={k.unit}
                      />
                    )}
                  </span>
                ))}
              </span>
            </Link>
          );
        })}
      </div>
      {tiles.length > MAX_DEFINITIONS && (
        <p className="mc-empty-state">
          Showing the first {MAX_DEFINITIONS} of {tiles.length} definitions.
        </p>
      )}
    </div>
  );
}

function SeriesLine({
  points,
  baseline,
  theme,
  title,
}: {
  points: { label: string; y: number }[];
  baseline?: number;
  theme: string;
  title: string;
}) {
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const root = useThemeRoot();

  useEffect(() => {
    const canvas = canvasRef.current;
    if (!canvas || points.length < 2) return;
    const stroke = cssVar(root, "--brand-strong");
    const grid = cssVar(root, "--border");
    const muted = cssVar(root, "--text-muted");
    const chart = new Chart(canvas, {
      type: "line",
      data: {
        labels: points.map((p) => p.label),
        datasets: [
          {
            data: points.map((p) => p.y),
            borderColor: stroke,
            backgroundColor: stroke + "30",
            fill: true,
            borderWidth: 2,
            pointRadius: 0,
            tension: 0.3,
          },
          ...(typeof baseline === "number"
            ? [
                {
                  data: points.map(() => baseline),
                  borderColor: muted,
                  borderWidth: 1,
                  borderDash: [4, 3],
                  pointRadius: 0,
                  fill: false,
                },
              ]
            : []),
        ],
      },
      options: {
        responsive: true,
        maintainAspectRatio: false,
        plugins: { legend: { display: false } },
        scales: {
          x: { ticks: { display: false }, grid: { display: false } },
          y: { ticks: { color: muted, font: { size: 10 } }, grid: { color: grid } },
        },
      },
    });
    return () => chart.destroy();
  }, [points, baseline, theme, root]);

  return <canvas ref={canvasRef} aria-label={title} />;
}
