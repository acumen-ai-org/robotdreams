import { Suspense, lazy, useCallback, useEffect, useRef, useState, type CSSProperties } from "react";
import { useConnection } from "../../state/ConnectionContext";
import { useRoute } from "../../state/RouterContext";
import { useLive } from "../../state/LiveContext";
import { useShell } from "../../state/ShellContext";
import { reportingRoute } from "../../lib/routes";
import type { ReportPeriod, ReportResponse, ScopeEntry, SummaryTile } from "../../lib/types";
import { periodCaption, periodQuery, usePeriod } from "../../lib/period";
import { PeriodControl } from "./PeriodControl";
import { tileName } from "../../components/shared";
import { ReportingControls } from "./ReportingControls";
import { StanceControl } from "./StanceControl";
import { PageToolbox } from "../../components/PageToolbox";
import { DetailPanel } from "../../components/DetailPanel";
import { useSelection } from "../../state/SelectionContext";
import { useWorkers } from "../../hooks/useWorkers";
import { ToolboxRenderingToggle } from "../../components/RenderingToggle";
import { GridIcon, TrendIcon } from "../../components/icons";
import { useGlanceMode } from "../../lib/display";
import { severityKey, type SeverityKey } from "../../lib/triage";
import { ViewSelector } from "../../components/ViewSelector";
import { aboutVariant } from "../../lib/variants";
import { ReportPage } from "./ReportPage";
import { Async, DelayedLoading, NoMatches, Onboarding, Skeleton, TileSkeletons } from "../../components/Async";
import { GlanceBoard, GlanceRibbon } from "./GlanceBoard";

const POLL_MS = 15000;

const VARIANT_TITLES: Record<string, string> = {
  panels: "Glance",
  timeline: "Timeline",
  charts: "Delta",
  topology: "Spatial",
  plan: "Board",
  narrative: "Narrative",
  ask: "Ask",
};

const TimelineView = lazy(() => import("./ReportingTimelineView"));
const ChartsView = lazy(() => import("./ReportingChartsView"));
const TopologyView = lazy(() => import("./ReportingTopologyView"));
const NarrativeView = lazy(() => import("./ReportingNarrativeView"));
const PlanView = lazy(() => import("./ReportingPlanView"));

interface Props {
  active?: boolean;
}

export function ReportingView({ active = true }: Props) {
  const route = useRoute();
  const { stream, refreshTick, orgVersion, storageVersion } = useLive();
  const live = stream === "live";
  const { sideCollapsed, sideWidth, variant: variantOf, selectVariant: onSelectVariant } = useShell();
  const variant = variantOf("reporting");
  const index = useWorkers(orgVersion, active, route.scopes, route.roles);
  const { selection, select, clear } = useSelection();
  const picked = selection?.kind === "place" ? selection.path : null;
  const onPick = useCallback(
    (path: string | null) => (path === null ? clear() : select({ kind: "place", path })),
    [select, clear],
  );
  const [glanceMode, setGlanceMode] = useGlanceMode();
  const [only, setOnly] = useState<SeverityKey | null>(null);
  const [stuckOnly, setStuckOnly] = useState(false);
  const periodApi = usePeriod();
  const periodQ = periodQuery(periodApi.period);
  const [echoed, setEchoed] = useState<ReportPeriod | null>(null);
  const conn = useConnection();
  const [scopes, setScopes] = useState<ScopeEntry[] | null>(null);
  const [tiles, setTiles] = useState<SummaryTile[] | null>(null);
  const [report, setReport] = useState<ReportResponse | null>(null);
  const [error, setError] = useState("");
  const [pollTick, setPollTick] = useState(0);
  const loadSeq = useRef(0);

  const fetchTiles = useCallback(async (): Promise<SummaryTile[]> => {
    const sel = route.scopes.length ? route.scopes : [""];
    const cats = route.cats.length ? route.cats : [""];
    const reqs: Array<{ scope: string; url: string }> = [];
    for (const s of sel) {
      for (const c of cats) {
        reqs.push({
          scope: s,
          url:
            "/api/reports/summary?scope=" +
            encodeURIComponent(s) +
            (c ? "&category=" + encodeURIComponent(c) : "") +
            (route.stance ? "&stance=" + encodeURIComponent(route.stance) : "") +
            periodQ,
        });
      }
    }
    const results = await Promise.all(
      reqs.map(async (r) => ({
        scope: r.scope,
        data: await conn.apiJSON<{ tiles?: SummaryTile[]; period?: ReportPeriod }>(r.url),
      })),
    );
    setEchoed(periodQ ? results.find((r) => r.data.period)?.data.period || null : null);
    const byKey = new Map<string, SummaryTile>();
    for (const r of results) {
      for (const t of r.data.tiles || []) {
        const key = r.scope + "\u0000" + tileName(t);
        if (!byKey.has(key)) byKey.set(key, sel.length > 1 ? { ...t, scope: r.scope } : t);
      }
    }
    return Array.from(byKey.values()).sort(
      (a, b) => tileName(a).localeCompare(tileName(b)) || (a.scope || "").localeCompare(b.scope || ""),
    );
  }, [conn, route.scopes, route.cats, route.stance, periodQ]);

  useEffect(() => {
    if (!active || !conn.hasToken) return;
    const seq = ++loadSeq.current;
    void (async () => {
      try {
        const data = await conn.apiJSON<{ scopes?: ScopeEntry[] }>("/api/reports/scopes");
        if (seq !== loadSeq.current) return;
        setScopes(data.scopes || []);
        if (route.report) {
          const rep = await conn.apiJSON<ReportResponse>(
            "/api/reports/report?definition=" +
              encodeURIComponent(route.report) +
              "&scope=" +
              encodeURIComponent(route.scope) +
              periodQ,
          );
          if (seq !== loadSeq.current) return;
          setReport(rep);
          setEchoed(periodQ ? rep.period || null : null);
        } else {
          const t = await fetchTiles();
          if (seq !== loadSeq.current) return;
          setTiles(t);
          setReport(null);
        }
        setError("");
      } catch (e) {
        if (seq === loadSeq.current) {
          setError("Could not load reports: " + (e instanceof Error ? e.message : String(e)));
        }
      }
    })();
  }, [active, conn, conn.hasToken, conn.session, route, fetchTiles, refreshTick, pollTick, periodQ]);

  useEffect(() => {
    if (!(active && !live && conn.hasToken)) return;
    const t = setInterval(() => setPollTick((n) => n + 1), POLL_MS);
    return () => clearInterval(t);
  }, [active, live, conn.hasToken]);

  const onReport = !!route.report;
  const lazyFallback = (
    <DelayedLoading>
      <Skeleton lines={5} />
    </DelayedLoading>
  );

  return (
    <main
      id="view-reporting"
      className={"mc-layout mc-grid-12 has-side" + (sideCollapsed ? " mc-side-collapsed" : "")}
      style={{ "--side-w": sideWidth + "px" } as CSSProperties}
      hidden={!active}
    >
      <PageToolbox
        active={active}
        title={onReport ? route.report : "Outcomes"}
        subtitle={
          onReport
            ? route.report
            : variant === "panels"
              ? glanceMode === "delta"
                ? "Delta"
                : "Glance"
              : VARIANT_TITLES[variant] || "Summaries"
        }
        info={
          onReport
            ? "Reading this report as " +
              (route.stance || "any stance") +
              ". A stance re-weights the page — the sections it does not serve move down and dim, and nothing is hidden."
            : aboutVariant("reporting", variant)
        }
      >
        <ViewSelector route={route} view="reporting" active={variant} onSelect={onSelectVariant} />
        {conn.hasToken && (scopes?.length ?? 0) > 0 && !onReport && (
          <ReportingControls
            route={route}
            scopes={scopes || []}
            stuck={variant === "plan" ? { on: stuckOnly, set: setStuckOnly } : undefined}
          />
        )}
        {onReport && <StanceControl route={route} />}
        {conn.hasToken && <PeriodControl {...periodApi} />}
      </PageToolbox>

      <div className="mc-reporting-main mc-col-main" id={active ? "main-content" : undefined} tabIndex={-1}>
        {conn.hasToken && periodQ && (
          <p className="mc-period-caption" role="status">
            {echoed ? periodCaption(echoed) : "Aggregating over one " + periodApi.period.kind + "…"}
          </p>
        )}
        {!conn.hasToken ? (
          <section aria-label="Report summaries">
            <p className="mc-empty-state">Connect to a server to see reports.</p>
          </section>
        ) : onReport && report ? (
          <ReportPage route={route} report={report} />
        ) : (
          <section aria-label="Report summaries">
            {variant === "timeline" ? (
              <Suspense fallback={lazyFallback}>
                <TimelineView
                  route={route}
                  refreshTick={refreshTick}
                  pollTick={pollTick}
                  active={active}
                  periodQuery={periodQ}
                />
              </Suspense>
            ) : variant === "topology" ? (
              <Suspense fallback={lazyFallback}>
                <TopologyView
                  route={route}
                  scopes={scopes || []}
                  refreshTick={refreshTick}
                  active={active}
                  picked={picked}
                  onPick={onPick}
                  periodQuery={periodQ}
                />
              </Suspense>
            ) : variant === "plan" ? (
              <Suspense fallback={lazyFallback}>
                <PlanView
                  route={route}
                  tiles={tiles}
                  refreshTick={refreshTick}
                  active={active}
                  stuckOnly={stuckOnly}
                  periodQuery={periodQ}
                />
              </Suspense>
            ) : variant === "narrative" ? (
              <Suspense fallback={lazyFallback}>
                <NarrativeView
                  route={route}
                  tiles={tiles || []}
                  refreshTick={refreshTick}
                  active={active}
                  periodQuery={periodQ}
                />
              </Suspense>
            ) : (
              <>
                <ToolboxRenderingToggle
                  active={active}
                  label="Reading"
                  value={glanceMode}
                  onChange={setGlanceMode}
                  options={[
                    { id: "board", label: "Where things stand", icon: <GridIcon size={15} /> },
                    { id: "delta", label: "How they have moved", icon: <TrendIcon size={15} /> },
                  ]}
                />
                <Async
                  data={scopes === null || tiles === null ? null : tiles}
                  error={error}
                  onRetry={() => setPollTick((n) => n + 1)}
                  skeleton={<TileSkeletons />}
                  empty={
                    scopes && scopes.length === 0 ? (
                      <Onboarding />
                    ) : (
                      <NoMatches clearTo={reportingRoute(route, { cats: [], stance: "", report: "" })} />
                    )
                  }
                >
                  {(rows) => {
                    const shown = only ? rows.filter((t) => severityKey(t.status) === only) : rows;
                    return (
                      <>
                        <GlanceRibbon
                          active={active}
                          tiles={rows}
                          only={only}
                          onFocus={(key) => setOnly(only === key ? null : key)}
                        />
                        {glanceMode === "delta" ? (
                          <Suspense fallback={lazyFallback}>
                            <ChartsView
                              route={route}
                              tiles={shown}
                              refreshTick={refreshTick}
                              active={active}
                              periodQuery={periodQ}
                            />
                          </Suspense>
                        ) : (
                          <GlanceBoard route={route} tiles={shown} />
                        )}
                      </>
                    );
                  }}
                </Async>
              </>
            )}
          </section>
        )}
      </div>

      <DetailPanel
        view="reporting"
        active={active}
        scopes={scopes || []}
        index={index}
        storageVersion={storageVersion}
      />
    </main>
  );
}
