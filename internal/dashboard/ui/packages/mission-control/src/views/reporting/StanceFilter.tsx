import { reportingRoute, STANCES, STANCE_QUESTIONS, type Route } from "../../lib/routes";
import { Link } from "../../components/Link";

export function StanceFilter({ route }: { route: Route }) {
  return (
    <div className="mc-stance-filter" role="group" aria-label="Read these reports as">
      <Link
        className={"mc-stance-option" + (route.stance === "" ? " is-on" : "")}
        to={reportingRoute(route, { stance: "", report: "" })}
        aria-current={route.stance === "" ? "true" : undefined}
      >
        <span className="mc-stance-name">Any</span>
        <span className="mc-stance-question">however it is written</span>
      </Link>
      {STANCES.map((s) => (
        <Link
          key={s}
          className={"mc-stance-option" + (route.stance === s ? " is-on" : "")}
          to={reportingRoute(route, { stance: s, report: "" })}
          aria-current={route.stance === s ? "true" : undefined}
        >
          <span className="mc-stance-name">{s}</span>
          <span className="mc-stance-question">{STANCE_QUESTIONS[s]}</span>
        </Link>
      ))}
    </div>
  );
}
