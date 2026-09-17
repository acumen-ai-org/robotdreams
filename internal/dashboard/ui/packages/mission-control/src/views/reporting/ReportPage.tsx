import { useMemo, useRef, useState } from "react";
import { reportingRoute, type Route } from "../../lib/routes";
import { StatusBadge, KpiStat } from "../../components/shared";
import { Link } from "../../components/Link";
import { PanelSection, panelIsWide } from "../../panels/registry";
import { TimeModeSwitch, TimeView } from "../../components/time/TimeView";
import { reportEntries } from "../../lib/time";
import { KpiBullet } from "../../components/KpiBullet";
import type { Panel, ReportResponse } from "../../lib/types";
import { Tabs, type TabDef } from "../../components/Tabs";
import { ReportSources } from "./ReportSources";
import { TypeIcon } from "../../components/detail/TypeIcon";
import { PanelRightOpenIcon } from "../../components/icons";
import { useSelection } from "../../state/SelectionContext";
import { useShell } from "../../state/ShellContext";

const SECTIONS: Record<string, { title: string; question: string; stances: string[] }> = {
  overview: { title: "Overview", question: "What happened?", stances: ["operational", "strategic", "diagnostic"] },
  performance: { title: "Performance", question: "How does that compare?", stances: ["strategic"] },
  drivers: { title: "Drivers", question: "Why did it change?", stances: ["strategic", "operational"] },
  exceptions: { title: "Exceptions", question: "What needs attention?", stances: ["operational"] },
  next: { title: "Next", question: "What are we going to do about it?", stances: ["strategic"] },
  detail: { title: "Detail", question: "What are the precise numbers?", stances: ["diagnostic"] },
};
const SECTION_ORDER = Object.keys(SECTIONS);
const domID = (id: string) => "v2-section-" + id;

interface Group {
  id: string;
  title: string;
  panels: Panel[];
  onStance: boolean;
}

function groupPanels(panels: Panel[], stance: string): Group[] {
  const out: Group[] = [];
  const flat = panels.filter((p) => !p.section || !SECTIONS[p.section]);
  if (flat.length) out.push({ id: "panels", title: "Panels", panels: flat, onStance: true });
  for (const id of SECTION_ORDER) {
    const inSection = panels.filter((p) => p.section === id);
    if (!inSection.length) continue;
    const onStance =
      !stance || SECTIONS[id].stances.includes(stance) || inSection.some((p) => (p.stances || []).includes(stance));
    out.push({ id, title: SECTIONS[id].title, panels: inSection, onStance });
  }
  return [...out.filter((g) => g.onStance), ...out.filter((g) => !g.onStance)];
}

export function ReportPage({ route, report }: { route: Route; report: ReportResponse }) {
  const def = report.definition || {};
  const name = def.name || route.report;
  const s = report.summary || {};
  const drills = report.drilldowns || [];
  const groups = useMemo(() => groupPanels(report.panels || [], route.stance), [report.panels, route.stance]);
  const entries = useMemo(() => reportEntries(report), [report]);
  const bodyRef = useRef<HTMLDivElement>(null);
  const { selection, select } = useSelection();
  const { sideCollapsed, toggleSide } = useShell();
  const defName = route.report || def.name || "";
  const atScope = s.scope || route.scope;
  const inPanel = selection?.kind === "report" && selection.def === defName && selection.scope === atScope;
  const [here, setHere] = useState<string>("");

  const kpis = useMemo(() => {
    const all = (s.kpis || []).slice();
    if (route.stance === "strategic") {
      all.sort((a, b) => Number(typeof b.target === "number") - Number(typeof a.target === "number"));
    }
    return all.slice(0, 6);
  }, [s.kpis, route.stance]);

  const navItems: TabDef[] = [
    ...groups.map((g) => ({
      id: g.id,
      title: g.title,
      label: g.title,
      badge: g.panels.length || undefined,
      hint: SECTIONS[g.id]?.question,
    })),
    ...(entries.length
      ? [{ id: "timeline", label: "Timeline", badge: entries.length, hint: "When did it happen?" }]
      : []),
  ];
  const at = navItems.some((n) => n.id === here) ? here : navItems[0]?.id || "";
  const shown = groups.filter((g) => g.id === at);

  return (
    <section className="mc-report-v2" aria-labelledby="report-v2-heading">
      <div className="mc-report-masthead">
        <TypeIcon kind="report" />
        <div className="mc-hero-ident">
          <span className="mc-side-panel-kind">Report</span>
          <h2 id="report-v2-heading">{name}</h2>
        </div>
        <StatusBadge status={s.status} />
        <ReportSources summary={s} route={route} def={route.report || name} />
        <button
          type="button"
          className={"mc-toolbox-button mc-open-in-panel" + (inPanel ? " is-on" : "")}
          aria-pressed={inPanel}
          title={inPanel ? "Already open in the side panel" : "Open this report in the side panel"}
          onClick={() => {
            select({ kind: "report", def: defName, scope: atScope });
            if (sideCollapsed) toggleSide();
          }}
        >
          <PanelRightOpenIcon size={14} />
          <span>Open in panel</span>
        </button>
      </div>

      <header className="mc-report-v2-hero">
        {s.headline && <p className="mc-hero-headline">{s.headline}</p>}
        {s.headline_partial && (
          <span className="mc-label-muted mc-headline-partial" title="Written by one producer; it cannot be rolled up">
            one contributor's words — the KPIs below cover all of them
          </span>
        )}
        {def.description && <p className="mc-hero-desc mc-body-muted">{def.description}</p>}
        {kpis.length > 0 && (
          <div className="mc-hero-kpis">
            {kpis.map((k, i) =>
              typeof k.target === "number" ? (
                <KpiBullet key={k.name || i} kpi={k} />
              ) : (
                <KpiStat key={k.name || i} kpi={k} />
              ),
            )}
          </div>
        )}
        <div className="mc-hero-tags">
          {(def.stances || []).length > 0 && (
            <span className="mc-hero-tag-group">
              <span className="mc-hero-tag-label">Read as</span>
              {(def.stances || []).map((st) => (
                <span
                  key={st}
                  className="mc-tag mc-tag-stance"
                  title={"This report can be read from the " + st + " footing"}
                >
                  {st}
                </span>
              ))}
            </span>
          )}
          {(def.categories || []).length > 0 && (
            <span className="mc-hero-tag-group">
              <span className="mc-hero-tag-label">Also in</span>
              {(def.categories || []).map((c) => (
                <Link
                  key={c}
                  className="mc-tag mc-tag-link"
                  to={reportingRoute(route, { report: "", cats: [c] })}
                  title={"Leave this report and list every report in the " + c + " category"}
                >
                  {c} ↗
                </Link>
              ))}
            </span>
          )}
        </div>
      </header>

      <div className="mc-report-v2-cols">
        <Tabs tabs={navItems} active={at} onSelect={setHere} label="Sections of this report" />

        <div className="mc-report-v2-body" ref={bodyRef}>
          {shown.map((g) => (
            <section
              className={"mc-report-section" + (g.onStance ? "" : " is-offstance")}
              id={domID(g.id)}
              data-section={g.id}
              key={g.id}
              aria-label={g.title}
            >
              <div className="mc-report-section-head">
                <h3 className="mc-report-section-title">{g.title}</h3>
                <span className="mc-label-muted">{SECTIONS[g.id]?.question || ""}</span>
                {!g.onStance && <span className="mc-label-muted mc-section-offstance">other stance</span>}
              </div>
              <div className="mc-report-panels">
                {g.panels.map((p, i) => (
                  <PanelSection key={i} panel={p} wide={panelIsWide(p)} />
                ))}
              </div>
            </section>
          ))}

          {at === "timeline" && entries.length > 0 && (
            <section className="mc-report-section" id={domID("timeline")} data-section="timeline" aria-label="Timeline">
              <div className="mc-report-section-head">
                <h3 className="mc-report-section-title">Timeline</h3>
                <span className="mc-label-muted">When did it happen?</span>
                <TimeModeSwitch count={entries.length} />
              </div>
              <TimeView entries={entries} empty="No timeline entries yet." />
            </section>
          )}
        </div>
      </div>

      {drills.length > 0 && (
        <div className="mc-drilldown-row">
          <span className="mc-label-muted" title="Other reports this one names. Opening one leaves this page.">
            Related reports
          </span>
          <span className="mc-drill-chips">
            {drills.map((d) => (
              <Link
                key={d}
                className="mc-pill mc-drill-chip"
                to={reportingRoute(route, { report: d })}
                title={"Open the " + d + " report at this scope"}
              >
                {d} ↗
              </Link>
            ))}
          </span>
        </div>
      )}
    </section>
  );
}
