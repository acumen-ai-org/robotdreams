import { useEffect, useMemo, useState, type ReactNode } from "react";
import { SidePanel } from "./SidePanel";
import { Tabs, TabPanel, type TabDef } from "./Tabs";
import { placeID, useSelection } from "../state/SelectionContext";
import { useShell } from "../state/ShellContext";
import { useRoute } from "../state/RouterContext";
import { useLive } from "../state/LiveContext";
import { useVocabulary } from "../state/VocabularyContext";
import { underScope, type ViewName } from "../lib/routes";
import { scopeDepth, scopeLeaf } from "../lib/scopePath";
import type { ActivityEntry, ScopeEntry } from "../lib/types";
import { countSubtree, scopeOf, type WorkerIndex } from "../hooks/useWorkers";
import { narrowStorage, useStorage } from "../hooks/useStorage";
import { ActivityList } from "../views/overview/ActivityList";
import { StorageTable } from "../views/overview/StorageTable";
import { AppBox } from "../views/overview/AppBox";
import { NodeDetail } from "../views/overview/NodeDetail";
import type { NodeApp } from "../lib/apps";
import type { RolloutNode } from "../lib/updates";
import { ViewBar } from "./detail/ViewBar";
import { NODE_TARGETS, PLACE_TARGETS } from "./detail/targets";
import { PlaceFacts, PlaceHierarchy, PlaceNodes, PlaceReports, useScopeTiles } from "./detail/PlacePanel";
import { NodeFacts, NodeHierarchy, NodeReports } from "./detail/NodePanel";
import { ReportContributors, ReportFacts, ReportHeadline, ReportHierarchy, reportLevels } from "./detail/ReportPanel";
import { ReportIcon } from "./icons";
import { TypeIcon } from "./detail/TypeIcon";
import { ScopeChip, describePath } from "./ScopeSelector";
import { nodeCodeOf, placeCode } from "./detail/entityCodeOf";
import { placeRows } from "../lib/orgRows";

interface Props {
  view: ViewName;
  active: boolean;
  scopes: ScopeEntry[];
  index?: WorkerIndex;
  updateOf?: Map<string, RolloutNode>;
  appOf?: Map<string, NodeApp>;
  storageVersion?: number;
  onOpenInView?: (id: string) => void;
}

function activityFor(entries: ActivityEntry[], test: (workerID: string) => boolean): ActivityEntry[] {
  return entries.filter((e) => (e.workers || []).some(test));
}

export function DetailPanel({ view, active, scopes, index, updateOf, appOf, storageVersion = 0, onOpenInView }: Props) {
  const { selection, select, clear } = useSelection();
  const { sideCollapsed, toggleSide, setSideWidth } = useShell();
  const route = useRoute();
  const vocab = useVocabulary();
  const { activity, pulse } = useLive();

  const kind = selection?.kind ?? null;
  const place = selection?.kind === "place" ? selection.path : null;
  const worker = selection?.kind === "node" && index ? index.byId.get(selection.id) : undefined;
  const report = selection?.kind === "report" ? selection : null;

  const tileScope = !active
    ? null
    : place !== null
      ? place
      : report
        ? report.scope
        : worker
          ? scopeOf(worker) || null
          : null;
  const tiles = useScopeTiles(tileScope);
  const storage = useStorage(storageVersion);
  const scopeOfWorker = useMemo(
    () => (id: string) => {
      const w = index?.byId.get(id);
      return w ? scopeOf(w) : "";
    },
    [index],
  );

  const rows = useMemo(() => placeRows(index ?? { workers: [] }, scopes), [index, scopes]);

  const reportTile = useMemo(
    () => (report ? (tiles || []).find((t) => (t.definition || t.name) === report.def) || null : null),
    [tiles, report],
  );

  const defaultTab =
    kind === "report" ? "report" : view === "reporting" ? "reports" : kind === "node" ? "node" : "place";
  const [tab, setTab] = useState(defaultTab);
  useEffect(() => {
    setTab(defaultTab);
  }, [defaultTab, place, worker?.id, report?.def, report?.scope]);

  if (!active) return null;

  if (!selection) {
    return (
      <SidePanel label="Details" empty collapsed={sideCollapsed} onToggle={toggleSide} onWidth={setSideWidth}>
        <p className="mc-nothing-selected">Nothing selected</p>
      </SidePanel>
    );
  }

  const onPickPlace = (path: string) => select({ kind: "place", path });
  const onPickNode = (id: string) => select({ kind: "node", id });
  const onPickReport = (scope: string) => report && select({ kind: "report", def: report.def, scope });

  let heading: { kind: string; name: ReactNode; icon: ReactNode; code?: string };
  let facts: ReactNode;
  let hierarchy: ReactNode;
  let bar: ReactNode;
  let tabs: TabDef[];
  let body: ReactNode;

  if (place !== null) {
    const segs = describePath(scopes, place);
    const seg = segs[segs.length - 1];
    heading = {
      kind: vocab.one(Math.max(0, scopeDepth(place) - 1)),
      name: seg ? <ScopeChip seg={seg} /> : <span className="mc-mono">{scopeLeaf(place) || "All scopes"}</span>,
      icon: <TypeIcon kind="place" depth={scopeDepth(place)} />,
      code: placeCode(seg),
    };
    facts = <PlaceFacts path={place} index={index} tiles={tiles} />;
    hierarchy = <PlaceHierarchy path={place} scopes={scopes} onPickPlace={onPickPlace} />;
    bar = (
      <ViewBar
        route={route}
        targets={PLACE_TARGETS}
        label="Look at it in"
        overrides={{ scope: place }}
        onArrive={(t) => t.view === "overview" && onOpenInView?.(placeID(place))}
      />
    );
    const here = activityFor(activity, (id) => {
      const w = index?.byId.get(id);
      return !!w && !!scopeOf(w) && underScope(scopeOf(w), place);
    });
    const placeStorage = narrowStorage(storage.objects, { underScope: place, scopeOfWorker });
    const nodesHere = index ? index.workers.filter((w) => scopeOf(w) && underScope(scopeOf(w), place)).length : 0;
    tabs = [
      { id: "place", label: vocab.one(Math.max(0, scopeDepth(place) - 1)), badge: nodesHere },
      { id: "reports", label: "Reports", badge: tiles?.length ?? undefined },
      { id: "activity", label: "Activity", badge: here.length },
      { id: "storage", label: "Storage", badge: placeStorage.length },
    ];
    body =
      tab === "reports" ? (
        <PlaceReports path={place} route={route} tiles={tiles} />
      ) : tab === "activity" ? (
        <ActivityList entries={here} />
      ) : tab === "storage" ? (
        <StorageTable version={storageVersion} objects={placeStorage} />
      ) : (
        <PlaceNodes path={place} index={index} onPickNode={onPickNode} />
      );
  } else if (worker) {
    const mine = activityFor(activity, (id) => id === worker.id);
    heading = {
      kind: vocab.one(4),
      name: <span className="mc-mono">{worker.id}</span>,
      icon: <TypeIcon kind="node" role={worker.role} />,
      code: nodeCodeOf(rows.byId.get(worker.id)),
    };
    facts = <NodeFacts worker={worker} index={index} />;
    hierarchy = <NodeHierarchy worker={worker} index={index} onPickNode={onPickNode} onPickPlace={onPickPlace} />;
    bar = (
      <ViewBar
        route={route}
        targets={NODE_TARGETS}
        label="Look at it in"
        overrides={scopeOf(worker) ? { scope: scopeOf(worker) } : {}}
        onArrive={() => onOpenInView?.(worker.id)}
      />
    );
    const nodeStorage = narrowStorage(storage.objects, { updatedBy: worker.id });
    const wrote = (tiles || []).filter((t) => (t.contributors || []).some((c) => c.producer === worker.id)).length;
    const under = index ? countSubtree(index.childrenOf, worker.id).total : 0;
    tabs = [
      { id: "node", label: vocab.one(4), badge: under },
      { id: "reports", label: "Wrote", badge: tiles === null ? undefined : wrote },
      { id: "activity", label: "Activity", badge: mine.length },
      { id: "storage", label: "Storage", badge: nodeStorage.length },
    ];
    body =
      tab === "reports" ? (
        <NodeReports worker={worker} route={route} tiles={tiles} />
      ) : tab === "activity" ? (
        <ActivityList entries={mine} />
      ) : tab === "storage" ? (
        <StorageTable version={storageVersion} objects={nodeStorage} />
      ) : (
        <>
          {appOf?.get(worker.id) && <AppBox app={appOf.get(worker.id) as NodeApp} />}
          <NodeDetail
            worker={worker}
            index={index as WorkerIndex}
            update={updateOf?.get(worker.id)}
            onSelect={(id) => (id ? onPickNode(id) : clear())}
          />
        </>
      );
  } else if (report) {
    const levels = reportLevels(reportTile, report.scope);
    const events = pulse.filter((p) => p.definition === report.def && underScope(p.scope, report.scope));
    heading = {
      kind: "Report",
      name: <span className="mc-mono">{report.def}</span>,
      icon: <TypeIcon kind="report" />,
    };
    facts = <ReportFacts tile={reportTile} scope={report.scope} />;
    hierarchy = <ReportHierarchy tile={reportTile} scope={report.scope} onPickReport={onPickReport} />;
    bar = (
      <div className="mc-view-bar">
        <span className="mc-view-bar-label">Read it at</span>
        <Tabs
          tabs={levels.map((l) => ({
            id: l.path,
            label: scopeLeaf(l.path) || "all",
            hint: (l.leaf ? "As written at " : "Rolled up to ") + l.path,
            icon: l.leaf ? <ReportIcon size={11} /> : undefined,
          }))}
          active={report.scope}
          onSelect={onPickReport}
          label="Read it at"
          variant="switch"
          className="mc-view-bar-tabs"
        />
      </div>
    );
    tabs = [
      { id: "report", label: "Report", badge: reportTile?.kpis?.length ?? undefined },
      { id: "sources", label: "Sources", badge: reportTile ? (reportTile.contributors?.length ?? 0) : undefined },
      { id: "activity", label: "Events", badge: events.length },
    ];
    body =
      tab === "sources" ? (
        <ReportContributors
          tile={reportTile}
          scope={report.scope}
          route={route}
          onPickNode={onPickNode}
          onPickReport={onPickReport}
        />
      ) : tab === "activity" ? (
        <ReportEvents events={events} />
      ) : (
        <ReportHeadline tile={reportTile} />
      );
  } else {
    heading = {
      kind: vocab.one(4),
      name: <span className="mc-mono">{selection.kind === "node" ? selection.id : ""}</span>,
      icon: <TypeIcon kind="node" />,
    };
    facts = null;
    hierarchy = null;
    bar = null;
    tabs = [];
    body = <p className="mc-body-muted">This node is not in the current selection, or has disconnected.</p>;
  }

  const openTab = tabs.some((t) => t.id === tab) ? tab : tabs[0]?.id || "";

  return (
    <SidePanel
      label="Details"
      kind={heading.kind}
      name={heading.name}
      icon={heading.icon}
      code={heading.code}
      collapsed={sideCollapsed}
      onToggle={toggleSide}
      onWidth={setSideWidth}
      header={
        <>
          {(facts || hierarchy) && (
            <div className="mc-panel-boxes">
              {facts}
              {hierarchy}
            </div>
          )}
          {bar}
          {tabs.length > 0 && <Tabs tabs={tabs} active={openTab} onSelect={setTab} label="What to show about this" />}
        </>
      }
    >
      <TabPanel id={openTab}>{body}</TabPanel>
    </SidePanel>
  );
}

function ReportEvents({ events }: { events: { t: string; label: string; severity: string; scope: string }[] }) {
  if (!events.length) return <p className="mc-body-muted">Nothing from this report since the page opened.</p>;
  return (
    <ul className="mc-report-events">
      {events.map((e, i) => (
        <li key={i} className="mc-report-event">
          <span className={"mc-sev-dot mc-" + (e.severity || "info")} aria-hidden="true" />
          <span className="mc-report-event-label">{e.label}</span>
          <span className="mc-mono mc-label-muted">{e.scope}</span>
        </li>
      ))}
    </ul>
  );
}
