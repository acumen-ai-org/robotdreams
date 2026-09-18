import { DEFAULT_WINDOW, formatWindow, parseWindow, type TimeWindow } from "./window";
import { formatSelection, parseSelection, type Selection } from "./selection";

export const CATEGORIES = [
  "logs",
  "performance",
  "roadmap",
  "delivery",
  "failures",
  "activity",
  "decisions",
  "cost",
  "quality",
];

export const STANCES = ["operational", "strategic", "diagnostic"];

export const STANCE_QUESTIONS: Record<string, string> = {
  operational: "Is it running?",
  strategic: "Are we getting where we said we would?",
  diagnostic: "What actually happened, exactly?",
};

export const LEVEL_NAMES = ["universe", "world", "realm", "site"];

export type ViewName = "overview" | "reporting" | "schedules" | "admin";

const VIEW_SLUGS: Record<ViewName, string> = {
  overview: "control-plane",
  reporting: "outcomes",
  schedules: "schedules",
  admin: "settings",
};

const VIEW_BY_SLUG: Record<string, ViewName> = {
  "control-plane": "overview",
  outcomes: "reporting",
  schedules: "schedules",
  settings: "admin",
};

export const ADMIN_DEFAULT = "connection";

export interface Route {
  view: ViewName;
  stance: string;
  scope: string;
  scopes: string[];
  cats: string[];
  roles: string[];
  report: string;
  win: TimeWindow;
  selected: Selection;
  section: string;
}

export function blankRoute(view: ViewName = "overview"): Route {
  return {
    view,
    stance: "",
    scope: "",
    scopes: [],
    cats: [],
    roles: [],
    report: "",
    win: DEFAULT_WINDOW,
    selected: null,
    section: "",
  };
}

export function parseHash(hash: string): Route {
  const raw = (hash || "").replace(/^#/, "");
  const cut = raw.indexOf("?");
  const pathPart = cut < 0 ? raw : raw.slice(0, cut);
  const query = cut < 0 ? "" : raw.slice(cut + 1);
  const segs = pathPart.split("/").filter(Boolean).map(decodeSegment);

  const view = VIEW_BY_SLUG[segs[0] || ""];
  if (!view) return blankRoute("overview");

  const route = blankRoute(view);
  const rest = segs.slice(1);

  if (view === "admin") {
    route.section = rest.join("/") || ADMIN_DEFAULT;
    return route;
  }

  const params = new URLSearchParams(query);
  route.scope = rest.join("/");
  route.scopes = [route.scope, ...parseScopes(params.get("also"))].filter(Boolean);
  route.cats = (params.get("category") || "").split(",").filter((c) => CATEGORIES.includes(c));
  route.roles = parseList(params.get("role"));
  route.stance = parseStance(params.get("stance"));
  route.win = parseWindow(params.get("window"));
  route.selected = parseSelection(params.get("selected"));
  if (view === "reporting") route.report = params.get("report") || "";
  return route;
}

export function adminRoute(section: string): Route {
  const r = blankRoute("admin");
  r.section = section || ADMIN_DEFAULT;
  return r;
}

export function adminHash(section: string): string {
  return buildHash(adminRoute(section));
}

export function buildHash(route: Route): string {
  const slug = VIEW_SLUGS[route.view];
  if (route.view === "admin") {
    return "#/" + slug + "/" + (route.section || ADMIN_DEFAULT);
  }

  const scopes = (route.scopes || []).filter(Boolean);
  const first = route.scope || scopes[0] || "";
  const path = "#/" + slug + (first ? "/" + first.split("/").filter(Boolean).map(encodeSegment).join("/") : "");

  const parts: string[] = [];
  const extra = scopes.filter((s) => s !== first);
  if (extra.length) parts.push("also=" + extra.join(","));
  if (route.view === "reporting" && route.report) parts.push("report=" + encodeValue(route.report));
  if (route.cats && route.cats.length) parts.push("category=" + route.cats.join(","));
  if (route.roles && route.roles.length) parts.push("role=" + route.roles.map(encodeValue).join(","));
  if (route.stance) parts.push("stance=" + route.stance);
  if (route.win && formatWindow(route.win) !== formatWindow(DEFAULT_WINDOW)) {
    parts.push("window=" + formatWindow(route.win));
  }
  const sel = formatSelection(route.selected);
  if (sel) parts.push("selected=" + encodeValue(sel));

  return path + (parts.length ? "?" + parts.join("&") : "");
}

function merge(base: Route, overrides: Partial<Route>): Route {
  const next = { ...base, ...overrides };
  if (overrides.scope !== undefined && overrides.scopes === undefined) {
    next.scopes = overrides.scope ? [overrides.scope] : [];
  }
  return next;
}

export function crossViewRoute(current: Route, view: ViewName): Route {
  return { ...current, view, report: "", section: "" };
}

export function crossViewHash(current: Route, view: ViewName): string {
  return buildHash(crossViewRoute(current, view));
}

export function viewRoute(current: Route, overrides: Partial<Route>): Route {
  return merge(current, overrides);
}

export function viewHash(current: Route, overrides: Partial<Route>): string {
  return buildHash(viewRoute(current, overrides));
}

export function reportingRoute(current: Route, overrides: Partial<Route>): Route {
  return merge({ ...current, view: "reporting", section: "" }, overrides);
}

export function reportingHash(current: Route, overrides: Partial<Route>): string {
  return buildHash(reportingRoute(current, overrides));
}

export function underScope(path: string, scope: string): boolean {
  if (!scope) return true;
  return path === scope || path.startsWith(scope + "/");
}

export function underAnyScope(path: string, scopes: string[]): boolean {
  if (!scopes.length) return true;
  return scopes.some((s) => underScope(path, s));
}

function encodeSegment(seg: string): string {
  return encodeURIComponent(seg);
}

function decodeSegment(seg: string): string {
  try {
    return decodeURIComponent(seg);
  } catch {
    return seg;
  }
}

function encodeValue(value: string): string {
  return value.replace(/[#&=?%\s]/g, (c) => encodeURIComponent(c));
}

function parseStance(raw: string | null): string {
  const s = (raw || "").trim();
  return STANCES.includes(s) ? s : "";
}

function parseList(raw: string | null): string[] {
  return (raw || "")
    .split(",")
    .map((p) => p.trim())
    .filter(Boolean);
}

function parseScopes(raw: string | null): string[] {
  return (raw || "")
    .split(",")
    .map((p) => p.replace(/^\/+|\/+$/g, "").trim())
    .filter(Boolean);
}
