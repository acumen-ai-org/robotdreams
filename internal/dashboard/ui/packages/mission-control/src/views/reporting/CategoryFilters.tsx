import { CATEGORIES, reportingRoute, type Route } from "../../lib/routes";
import { useRouter } from "../../state/RouterContext";

export function CategoryFilters({ route, compact }: { route: Route; compact?: boolean }) {
  const { navigate } = useRouter();
  return (
    <div
      className={"mc-category-filters" + (compact ? " is-compact" : "")}
      role="group"
      aria-label="Filter reports by category"
    >
      {CATEGORIES.map((c) => {
        const activeCat = route.cats.includes(c);
        return (
          <button
            key={c}
            className="mc-pill mc-filter-pill"
            type="button"
            aria-pressed={activeCat}
            onClick={() => {
              const cats = activeCat ? route.cats.filter((x) => x !== c) : route.cats.concat([c]);
              navigate(reportingRoute(route, { cats, report: "" }));
            }}
          >
            {c}
          </button>
        );
      })}
    </div>
  );
}
