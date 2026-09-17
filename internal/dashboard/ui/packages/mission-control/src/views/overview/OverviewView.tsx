import { Suspense, lazy, useCallback, useEffect, useMemo, useState, type CSSProperties } from "react";
import { OrgTree } from "./OrgTree";
import { OverviewLegend } from "./Legend";
import { SearchControl } from "../../components/SearchControl";
import { PageToolbox } from "../../components/PageToolbox";
import { ScopeSelector } from "../../components/ScopeSelector";
import { UniverseIcon } from "../../lib/vocabulary";
import { ViewSelector } from "../../components/ViewSelector";
import { aboutVariant } from "../../lib/variants";
import { scopeOf, useWorkers } from "../../hooks/useWorkers";
import { useApps } from "../../hooks/useApps";
import { useScopes } from "../../hooks/useScopes";
import { useReveal } from "../../hooks/useReveal";
import { useSelection, selectedID, selectionFromID } from "../../state/SelectionContext";
import { DetailPanel } from "../../components/DetailPanel";
import { useExploreMode, useSpaceMode } from "../../lib/display";
import { ToolboxRenderingToggle } from "../../components/RenderingToggle";
import { CirclesIcon, GridIcon, OrbitIcon, SitemapIcon } from "../../components/icons";
import { useConnection } from "../../state/ConnectionContext";
import { useRoute } from "../../state/RouterContext";
import { useLive } from "../../state/LiveContext";
import { useShell } from "../../state/ShellContext";
import type { SummaryTile } from "../../lib/types";

const ExploreView = lazy(() => import("./ExploreView"));
const PlanetMap = lazy(() => import("./PlanetMap"));
const ForceGraph = lazy(() => import("./ForceGraph"));
const GalaxyMap = lazy(() => import("./GalaxyMap"));

interface Props {
  hidden?: boolean;
}

export function OverviewView({ hidden = false }: Props) {
  const route = useRoute();
  const { orgVersion, storageVersion, rollout, pulsedWorkers: pulsed, traffic } = useLive();
  const {
    sideCollapsed,
    sideWidth,
    toggleSide: onToggleSide,
    variant: variantOf,
    selectVariant: onSelectVariant,
  } = useShell();
  const variant = variantOf("overview");
  const [search, setSearch] = useState("");
  const [matchCount, setMatchCount] = useState(0);
  const { selection, select, clear } = useSelection();
  const selectedId = selectedID(selection);
  const [exploreFocus, setExploreFocus] = useState<string>("");
  const [ownPick, setOwnPick] = useState(0);
  const [exploreMode, setExploreMode] = useExploreMode();
  const [spaceMode, setSpaceMode] = useSpaceMode();

  const scopes = useScopes(!hidden, route.win);
  const conn = useConnection();
  const index = useWorkers(orgVersion, !hidden, route.scopes, route.roles);

  const apps = useApps(orgVersion, !hidden);

  const onSelect = useCallback(
    (id: string | null) => {
      setOwnPick((n) => n + 1);
      if (id) select(selectionFromID(id));
      else clear();
      if (id && sideCollapsed) onToggleSide();
    },
    [select, clear, sideCollapsed, onToggleSide],
  );

  const revealInExplore = useCallback(
    (key: string) => {
      if (key.startsWith("scope:")) {
        setExploreFocus(key);
        return;
      }
      const w = index.byId.get(key);
      const sc = w ? scopeOf(w) : "";
      setExploreFocus(sc ? "scope:" + sc : "");
    },
    [index],
  );
  useReveal(selectedId, !hidden && variant === "explore", revealInExplore, ownPick);

  const [tilesByScope, setTilesByScope] = useState<Map<string, SummaryTile[]>>(new Map());
  const isOutline = variant === "explore" && exploreMode === "reports";
  const wantTiles = !hidden && isOutline && conn.hasToken;
  const worldScopes = useMemo(
    () =>
      Array.from(new Set(scopes.map((s) => s.path.split("/").slice(0, 2).join("/"))))
        .filter(Boolean)
        .slice(0, 24),
    [scopes],
  );
  useEffect(() => {
    if (!wantTiles || !worldScopes.length) return;
    let stale = false;
    void Promise.all(
      worldScopes.map(async (scope) => {
        try {
          const d = await conn.apiJSON<{ tiles?: SummaryTile[] }>(
            "/api/reports/summary?scope=" + encodeURIComponent(scope),
          );
          return [scope, d.tiles || []] as [string, SummaryTile[]];
        } catch {
          return [scope, [] as SummaryTile[]] as [string, SummaryTile[]];
        }
      }),
    ).then((pairs) => {
      if (!stale) setTilesByScope(new Map(pairs));
    });
    return () => {
      stale = true;
    };
  }, [conn, conn.session, wantTiles, worldScopes, orgVersion]);

  useEffect(() => {
    if (!selectedId || hidden) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") clear();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [selectedId, hidden, clear]);

  const title =
    variant === "force"
      ? "Network"
      : variant === "galaxy"
        ? spaceMode === "planets"
          ? "Planets"
          : "Galaxy"
        : exploreMode === "reports"
          ? "Reporting"
          : "Explore";
  const fallback = (what: string) => (
    <section className="mc-panel">
      <p className="mc-empty-state">Loading {what}…</p>
    </section>
  );

  return (
    <main
      id="view-overview"
      className={"mc-layout mc-grid-12 has-side" + (sideCollapsed ? " mc-side-collapsed" : "")}
      style={{ "--side-w": sideWidth + "px" } as CSSProperties}
      hidden={hidden}
    >
      <PageToolbox
        active={!hidden}
        title="Control Plane"
        subtitle={title}
        info={aboutVariant("overview", variant)}
        legend={<OverviewLegend />}
      >
        <ViewSelector route={route} view="overview" active={variant} onSelect={onSelectVariant} />
        <span className="mc-view-select">
          <span className="mc-view-select-label">
            <UniverseIcon size={11} /> Scope
          </span>
          <span className="mc-control-group">
            <ScopeSelector route={route} scopes={scopes} />
          </span>
        </span>
      </PageToolbox>
      <SearchControl
        active={!hidden && isOutline}
        value={search}
        onChange={setSearch}
        resultCount={matchCount}
        label="Search workers"
        placeholder="Search workers by id or role…"
      />

      <div className="mc-col-main" id={!hidden ? "main-content" : undefined} tabIndex={-1}>
        {variant === "explore" ? (
          <>
            <ToolboxRenderingToggle
              active={!hidden}
              label="Structure"
              value={exploreMode}
              onChange={setExploreMode}
              options={[
                { id: "places", label: "Places, one level at a time", icon: <GridIcon size={15} /> },
                { id: "reports", label: "The reporting hierarchy", icon: <SitemapIcon size={15} /> },
              ]}
            />
            {exploreMode === "places" ? (
              <Suspense fallback={fallback("explore")}>
                <ExploreView
                  index={index}
                  scopes={scopes}
                  focus={exploreFocus}
                  onFocus={setExploreFocus}
                  selected={selectedId}
                  onSelect={onSelect}
                  updateOf={rollout.updateOf}
                />
              </Suspense>
            ) : (
              <section className="mc-panel mc-panel-fill">
                <OrgTree
                  index={index}
                  scopes={scopes}
                  mode="reports"
                  tilesByScope={tilesByScope}
                  search={search}
                  pulsed={pulsed}
                  selected={selectedId}
                  onSelect={onSelect}
                  onMatchCount={setMatchCount}
                  updateOf={rollout.updateOf}
                  appOf={apps.byWorker}
                />
              </section>
            )}
          </>
        ) : variant === "galaxy" ? (
          <>
            <ToolboxRenderingToggle
              active={!hidden}
              label="Space"
              value={spaceMode}
              onChange={setSpaceMode}
              options={[
                { id: "planets", label: "Planets — packed flat", icon: <CirclesIcon size={15} /> },
                { id: "galaxy", label: "Galaxy — with a camera among them", icon: <OrbitIcon size={15} /> },
              ]}
            />
            {spaceMode === "planets" ? (
              <Suspense fallback={fallback("planets")}>
                <PlanetMap index={index} scopes={scopes} active={!hidden} selected={selectedId} onSelect={onSelect} />
              </Suspense>
            ) : (
              <Suspense fallback={fallback("galaxy")}>
                <GalaxyMap index={index} scopes={scopes} active={!hidden} selected={selectedId} onSelect={onSelect} />
              </Suspense>
            )}
          </>
        ) : (
          <Suspense fallback={fallback("network")}>
            <ForceGraph
              index={index}
              active={!hidden}
              selected={selectedId}
              onSelect={onSelect}
              traffic={traffic}
              updateOf={rollout.updateOf}
              appOf={apps.byWorker}
            />
          </Suspense>
        )}
      </div>

      <DetailPanel
        view="overview"
        active={!hidden}
        scopes={scopes}
        index={index}
        updateOf={rollout.updateOf}
        appOf={apps.byWorker}
        storageVersion={storageVersion}
        onOpenInView={setExploreFocus}
      />
    </main>
  );
}
