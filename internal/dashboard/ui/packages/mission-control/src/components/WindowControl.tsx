import { useEffect, useState } from "react";
import { useRoute, useRouter } from "../state/RouterContext";
import { viewRoute } from "../lib/routes";
import { ClockIcon } from "./icons";
import { clampWindow, windowLabel, WINDOW_UNITS, type TimeWindow, type WindowUnit } from "../lib/window";

const UNIT_LABEL: Record<WindowUnit, string> = { h: "hours", d: "days", w: "weeks", mo: "months" };
const UNIT_ONE: Record<WindowUnit, string> = { h: "hour", d: "day", w: "week", mo: "month" };

export function WindowControl() {
  const route = useRoute();
  const { navigate } = useRouter();
  const win = route.win;

  const [draft, setDraft] = useState(String(win.n));
  useEffect(() => {
    setDraft(String(win.n));
  }, [win.n]);

  const set = (next: TimeWindow) => {
    const c = clampWindow(next);
    navigate(viewRoute(route, { win: c, report: route.report }), { replace: true });
  };

  const commit = () => {
    const n = Number(draft);
    if (!Number.isFinite(n) || n < 1) {
      setDraft(String(win.n));
      return;
    }
    if (n !== win.n) set({ n, unit: win.unit });
    else setDraft(String(win.n));
  };

  return (
    <span className="mc-view-select mc-window-select">
      <span className="mc-view-select-label">
        <ClockIcon size={11} /> Window
      </span>
      <span className="mc-control-group mc-window-group" title={"Showing the " + windowLabel(win)}>
        <button
          className="mc-window-step"
          type="button"
          aria-label={"Shorten the window by one " + UNIT_ONE[win.unit]}
          title="Shorter"
          disabled={win.n <= 1}
          onClick={() => set({ n: win.n - 1, unit: win.unit })}
        >
          −
        </button>
        <input
          className="mc-window-n"
          type="number"
          min={1}
          max={999}
          step={1}
          value={draft}
          aria-label="How many, back from now"
          onChange={(e) => setDraft(e.target.value)}
          onBlur={commit}
          onKeyDown={(e) => {
            if (e.key === "Enter") (e.target as HTMLInputElement).blur();
          }}
        />
        <button
          className="mc-window-step"
          type="button"
          aria-label={"Lengthen the window by one " + UNIT_ONE[win.unit]}
          title="Longer"
          disabled={win.n >= 999}
          onClick={() => set({ n: win.n + 1, unit: win.unit })}
        >
          +
        </button>
        <select
          className="mc-window-unit"
          value={win.unit}
          aria-label="Window unit"
          onChange={(e) => set({ n: win.n, unit: e.target.value as WindowUnit })}
        >
          {WINDOW_UNITS.map((u) => (
            <option key={u} value={u}>
              {UNIT_LABEL[u]}
            </option>
          ))}
        </select>
      </span>
    </span>
  );
}
