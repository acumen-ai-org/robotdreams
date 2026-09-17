import { CalendarIcon } from "../../components/icons";
import { PERIOD_KINDS, PERIOD_LABELS, type PeriodApi } from "../../lib/period";

export function PeriodControl({ period, setKind, step, reset }: PeriodApi) {
  const live = !period.kind;
  const unit = period.kind ? PERIOD_LABELS[period.kind].toLowerCase() : "";
  return (
    <span className="mc-view-select mc-period-select">
      <span className="mc-view-select-label">
        <CalendarIcon size={11} /> Period
      </span>
      <span className="mc-control-group mc-period-group" role="group" aria-label="Aggregation period">
        <button
          type="button"
          className={"mc-impl" + (live ? " is-on" : "")}
          aria-pressed={live}
          title="Live: the newest report at every scope, no period"
          onClick={() => setKind("")}
        >
          Live
        </button>
        {PERIOD_KINDS.map((k) => (
          <button
            key={k}
            type="button"
            className={"mc-impl" + (period.kind === k ? " is-on" : "")}
            aria-pressed={period.kind === k}
            title={"Fold every report over one " + k}
            onClick={() => setKind(k)}
          >
            {PERIOD_LABELS[k]}
          </button>
        ))}
        <span className="mc-control-sep" aria-hidden="true" />
        <button
          className="mc-window-step"
          type="button"
          aria-label={live ? "Choose a period to step" : "One " + unit + " earlier"}
          title={live ? "Choose a period first" : "One " + unit + " earlier"}
          disabled={live}
          onClick={() => step(-1)}
        >
          ‹
        </button>
        <button
          className="mc-window-step"
          type="button"
          aria-label={live ? "Choose a period to step" : "One " + unit + " later"}
          title={live ? "Choose a period first" : "One " + unit + " later"}
          disabled={live}
          onClick={() => step(1)}
        >
          ›
        </button>
        <button
          className="mc-window-step mc-period-now"
          type="button"
          aria-label={live ? "Already live" : "This " + unit}
          title={live ? "Already live" : "Back to this " + unit}
          disabled={live}
          onClick={reset}
        >
          •
        </button>
      </span>
    </span>
  );
}
