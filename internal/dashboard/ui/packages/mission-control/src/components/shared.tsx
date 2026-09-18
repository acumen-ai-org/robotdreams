import { fmtNum, fmtTime, sevClass, unitLabel } from "../lib/format";
import { blankRoute, reportingRoute, type Route } from "../lib/routes";
import { Link } from "./Link";
import { PulseIcon } from "./icons";
import type { KPIValue, SummaryTile } from "../lib/types";
import type { ReactNode } from "react";

const OUTCOMES_BLANK: Route = blankRoute("reporting");

export function OutcomesLink({ scope, def }: { scope: string; def?: string }) {
  return (
    <Link
      className="mc-outcomes-link"
      to={reportingRoute(OUTCOMES_BLANK, { scope, report: def || "" })}
      title={"Open " + (def ? def + " at " : "") + scope + " in Outcomes"}
    >
      <PulseIcon size={11} />
      <span>Outcomes</span>
      <span aria-hidden="true">↗</span>
    </Link>
  );
}

const DOT_STATES = ["ok", "warn", "critical", "connected", "degraded", "disconnected"];

export function statusDotClass(status?: string): string {
  const s = (status || "").trim();
  return "mc-" + (DOT_STATES.includes(s) ? s : "unknown");
}

export function mcVariant(value?: string): string {
  const v = (value || "").trim();
  return v ? "mc-" + v : "";
}

export function StatusBadge({ status }: { status?: string }) {
  if (!status) return null;
  const cls = "mc-" + (status === "ok" || status === "warn" || status === "critical" ? status : "unknown");
  return (
    <span className={"mc-status-badge " + cls}>
      <span className={"mc-status-dot " + cls} aria-hidden="true"></span>
      <span className="mc-status-text">{status}</span>
    </span>
  );
}

function varianceClass(variance: number, direction: string | undefined): string {
  if (variance === 0) return "is-flat";
  const bad = direction === "below" ? variance < 0 : variance > 0;
  return bad ? "is-bad" : "is-good";
}

export function KpiStat({ kpi }: { kpi: KPIValue }) {
  const unit = unitLabel(kpi.unit);
  const suffix = unit ? " " + unit : "";
  const hasTarget = typeof kpi.target === "number" && isFinite(kpi.target);
  const variance =
    typeof kpi.variance === "number" && isFinite(kpi.variance)
      ? kpi.variance
      : hasTarget && typeof kpi.value === "number"
        ? kpi.value - (kpi.target as number)
        : null;
  return (
    <span className="mc-kpi">
      <span className="mc-kpi-value">
        {fmtNum(kpi.value)}
        {suffix}
      </span>
      <span className="mc-kpi-name mc-label-muted">{kpi.name || ""}</span>
      {hasTarget && (
        <span className="mc-kpi-target">
          <span className="mc-kpi-target-label mc-label-muted">target</span>
          <span className="mc-kpi-target-value">
            {fmtNum(kpi.target)}
            {suffix}
          </span>
          {variance !== null && (
            <span
              className={"mc-kpi-variance " + varianceClass(variance, kpi.direction)}
              title={
                "variance against target" +
                (kpi.direction ? " (" + (kpi.direction === "below" ? "lower" : "higher") + " is worse)" : "")
              }
            >
              {(variance > 0 ? "+" : variance < 0 ? "−" : "±") + fmtNum(Math.abs(variance))}
            </span>
          )}
        </span>
      )}
      {typeof kpi.delta === "number" && kpi.delta !== 0 && (
        <span className="mc-kpi-delta" title="change vs previous window">
          {(kpi.delta > 0 ? "▲ " : "▼ ") + fmtNum(Math.abs(kpi.delta))}
        </span>
      )}
    </span>
  );
}

export function SevPill({ sev }: { sev?: string }) {
  return <span className={"mc-sev " + sevClass(sev)}>{sev || "event"}</span>;
}

export function TickerItem({
  t,
  sev,
  label,
  extra,
  fmt,
}: {
  t?: string;
  sev?: string;
  label: string;
  extra?: ReactNode;
  fmt?: (iso: string | undefined) => string;
}) {
  return (
    <li className="mc-ticker-item">
      <span className="mc-ticker-time">{(fmt || fmtTime)(t)}</span>
      <SevPill sev={sev} />
      <span className="mc-ticker-label">{label}</span>
      {extra}
    </li>
  );
}

export function tileName(t: SummaryTile): string {
  return t.definition || t.name || "report";
}

export function contributionText(t: SummaryTile): string {
  const count = (v: unknown) => (typeof v === "number" ? v : Array.isArray(v) ? v.length : null);
  const inst = count(t.instances);
  const sc = count(t.scopes);
  const parts: string[] = [];
  if (inst != null) parts.push(inst + (inst === 1 ? " instance" : " instances"));
  if (sc != null) parts.push(sc + (sc === 1 ? " scope" : " scopes"));
  return parts.join(" · ");
}
