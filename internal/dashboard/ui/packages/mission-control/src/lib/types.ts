export interface Worker {
  id: string;
  role?: string;
  status?: string;
  reports_to?: string;
  metadata?: Record<string, string>;
  connected_at?: string;
  last_seen_at?: string;
}

export interface StorageObject {
  path: string;
  revision: string | number;
  updated_by?: string;
  updated_at?: string;
}

export interface KPIValue {
  name?: string;
  value?: number;
  unit?: string;
  delta?: number;
  target?: number;
  variance?: number;
  direction?: string;
}

export type SeriesPoint = number | { value?: number; v?: number; y?: number; t?: string };

export interface ReportEvent {
  t?: string;
  type?: string;
  severity?: string;
  label?: string;
  scope?: string;
}

export interface ReportSpan {
  name?: string;
  start: string;
  end?: string;
  label?: string;
  scope?: string;
}

export interface ReportTable {
  name?: string;
  columns?: string[];
  rows?: Array<Record<string, unknown> | unknown[]>;
}

export interface PlanItem {
  id: string;
  title?: string;
  state?: string;
  lane?: string;
  owner?: string;
  due?: string;
  size?: number;
  blocked_by?: string[];
  horizon?: string;
  commitment?: string;
  level?: string;
  scope?: string;
  definition?: string;
}

export interface PlanColumn {
  state: string;
  items: number;
  shown?: number;
  size?: number;
  limit?: number;
}

export interface PlanBucket {
  name: string;
  items: number;
  size?: number;
}

export interface PlanView {
  definition?: string;
  scope?: string;
  policy?: string;
  states?: PlanColumn[];
  lanes?: PlanBucket[];
  horizons?: PlanBucket[];
  commitments?: PlanBucket[];
  items?: PlanItem[];
  total?: number;
  blocked?: number;
  truncated?: boolean;
  size_unit?: string;
  baseline?: { name?: string; ref?: string };
  instances?: number;
  scopes?: string[];
}

export interface SummaryTile {
  definition?: string;
  name?: string;
  scope?: string;
  description?: string;
  status?: string;
  headline?: string;
  headline_partial?: boolean;
  kpis?: KPIValue[];
  sparkline?: SeriesPoint[] | { points?: SeriesPoint[] };
  categories?: string[];
  stances?: string[];
  facets?: string[];
  instances?: number | unknown[];
  scopes?: number | unknown[];
  contributors?: Contributor[];
}

export interface Contributor {
  scope: string;
  producer?: string;
  produced_at?: string;
}

export interface Panel {
  title?: string;
  primitive?: string;
  stances?: string[];
  section?: string;
  data_kind?: string;
  series?: SeriesPoint[];
  table?: ReportTable;
  events?: ReportEvent[];
  spans?: ReportSpan[];
  kpi?: KPIValue;
  graph?: unknown;
  plan?: PlanView;
}

export interface ReportDefinitionDoc {
  name?: string;
  description?: string;
  categories?: string[];
  stances?: string[];
  facets?: {
    timeline?: { milestones?: string[] };
    [k: string]: unknown;
  };
  [k: string]: unknown;
}

export interface ReportPeriod {
  kind: "day" | "week" | "month" | "quarter" | "year";
  start: string;
  end: string;
}

export interface ReportResponse {
  definition?: ReportDefinitionDoc;
  period?: ReportPeriod;
  summary?: SummaryTile;
  timeline?: { events?: ReportEvent[]; spans?: ReportSpan[] };
  plan?: PlanView | null;
  panels?: Panel[];
  drilldowns?: string[];
}

export interface ScopeEntry {
  path: string;
  depth?: number;
  level?: string;
  instances?: number;
  recent?: number;
}

export interface PulseItem {
  kind: "event" | "instance";
  definition: string;
  scope: string;
  t: string;
  severity: string;
  label: string;
}

export interface ActivityEntry {
  id: number;
  typeClass: string;
  label: string;
  detail: string;
  at: string;
  workers?: string[];
}
