import { useRouter } from "../../state/RouterContext";
import { useShell } from "../../state/ShellContext";
import { crossViewRoute, reportingRoute, viewRoute, type Route, type ViewName } from "../../lib/routes";
import { Tabs, type TabDef } from "../Tabs";

export interface ViewTarget {
  id: string;
  label: string;
  view: ViewName;
  hint?: string;
  overrides?: Partial<Route>;
}

export function ViewBar({
  route,
  targets,
  label,
  overrides,
  onArrive,
}: {
  route: Route;
  targets: ViewTarget[];
  label: string;
  overrides?: Partial<Route>;
  onArrive?: (t: ViewTarget) => void;
}) {
  const { navigate } = useRouter();
  const { variant: variantOf, selectVariant } = useShell();

  if (!targets.length) return null;

  const here = (t: ViewTarget) => t.view === route.view && variantOf(t.view) === t.id;
  const current = targets.find(here)?.id ?? "";

  const tabs: TabDef[] = targets.map((t) => ({ id: t.id, label: t.label, hint: t.hint }));

  const go = (id: string) => {
    const t = targets.find((x) => x.id === id);
    if (!t) return;
    selectVariant(t.view, t.id);
    const all = { ...overrides, ...t.overrides };
    const base = t.view === route.view ? viewRoute(route, {}) : crossViewRoute(route, t.view);
    navigate(t.view === "reporting" ? reportingRoute(base, all) : viewRoute(base, all));
    onArrive?.(t);
  };

  return (
    <div className="mc-view-bar">
      <span className="mc-view-bar-label">{label}</span>
      <Tabs tabs={tabs} active={current} onSelect={go} label={label} variant="switch" className="mc-view-bar-tabs" />
    </div>
  );
}
