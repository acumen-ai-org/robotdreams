import { ClockIcon, PulseIcon } from "./icons";
import { UniverseIcon } from "../lib/vocabulary";
import { perspectivesFor, perspectiveOf, variantForPerspective } from "../lib/variants";
import { crossViewRoute, viewRoute, type Route, type ViewName } from "../lib/routes";
import { useRouter } from "../state/RouterContext";

interface Props {
  route: Route;
  view: ViewName;
  active: string;
  onSelect: (view: ViewName, variantID: string) => void;
}

export function ViewSelector({ route, view, active, onSelect }: Props) {
  const { navigate } = useRouter();
  const outcomes = perspectivesFor("reporting");
  const universe = perspectivesFor("overview");
  const activeOutcome = view === "reporting" ? perspectiveOf("reporting", active) : "";

  return (
    <>
      <div className="mc-view-select" role="group" aria-label="Views">
        <span className="mc-view-select-label">
          <PulseIcon size={11} /> Views
        </span>
        <div className="mc-perspectives">
          {view === "overview" ? (
            <span className="mc-perspective-open" role="group" aria-label="Universe rendering">
              <span className="mc-perspective-open-label">
                <UniverseIcon size={11} /> Universe
              </span>
              {universe.map((p) => (
                <button
                  key={p.id}
                  type="button"
                  className={"mc-impl" + (p.id === active ? " is-on" : "")}
                  aria-pressed={p.id === active}
                  title={p.hint}
                  onClick={() => {
                    onSelect("overview", variantForPerspective("overview", p));
                    if (route.selected) navigate(viewRoute(route, { selected: null }), { replace: true });
                  }}
                >
                  {p.label}
                </button>
              ))}
            </span>
          ) : (
            <button
              type="button"
              className="mc-perspective"
              title="The Control Plane: the fleet itself, drawn as a structure"
              onClick={() => {
                navigate({ ...crossViewRoute(route, "overview"), selected: null });
              }}
            >
              Universe
              <span className="mc-perspective-count" aria-hidden="true">
                {universe.length}
              </span>
            </button>
          )}
          {outcomes.map((p) => {
            const on = view === "reporting" && p.id === activeOutcome;
            return (
              <button
                key={p.id}
                type="button"
                className={"mc-perspective" + (on ? " is-on" : "") + (p.built ? "" : " is-unbuilt")}
                aria-pressed={on}
                disabled={!p.built}
                title={p.built ? p.hint : p.hint + " (not built)"}
                onClick={() => {
                  onSelect("reporting", variantForPerspective("reporting", p));
                  if (view !== "reporting" || route.report) {
                    navigate({ ...crossViewRoute(route, "reporting"), selected: null });
                  } else if (route.selected) {
                    navigate(viewRoute(route, { selected: null }), { replace: true });
                  }
                }}
              >
                {on && <PulseIcon size={11} className="mc-perspective-icon" />}
                {p.label}
              </button>
            );
          })}
          <button
            type="button"
            className={"mc-perspective" + (view === "schedules" ? " is-on" : "")}
            aria-pressed={view === "schedules"}
            title="Every cron schedule the control plane holds, by the worker it wakes"
            onClick={() => {
              if (view === "schedules") return;
              navigate({ ...crossViewRoute(route, "schedules"), selected: null });
            }}
          >
            {view === "schedules" && <ClockIcon size={11} className="mc-perspective-icon" />}
            Schedules
          </button>
        </div>
      </div>
    </>
  );
}
