import { useTheme } from "../state/ThemeContext";
import { usePageControls } from "../state/PageControlsContext";
import { SettingsIcon } from "./icons";
import type { StreamState } from "../hooks/useEventStream";
import { ADMIN_DEFAULT, adminRoute } from "../lib/routes";
import { useRoute } from "../state/RouterContext";
import { useLive } from "../state/LiveContext";
import { Link } from "./Link";

export function ThemeToggle() {
  const { theme, toggle } = useTheme();
  return (
    <button
      className="mc-icon-button"
      type="button"
      onClick={toggle}
      aria-pressed={theme === "dark"}
      aria-label={theme === "dark" ? "Switch to light theme" : "Switch to dark theme"}
    >
      <span className="mc-theme-icon" aria-hidden="true">
        {theme === "dark" ? "🌙" : "☀️"}
      </span>
    </button>
  );
}

const CONN_DOT: Record<StreamState, { cls: string; label: string }> = {
  live: { cls: "mc-conn-live", label: "live" },
  connecting: { cls: "mc-conn-warn", label: "connecting" },
  reconnecting: { cls: "mc-conn-warn", label: "reconnecting" },
  offline: { cls: "mc-conn-off", label: "offline" },
};

export function AdminLink() {
  const view = useRoute().view;
  const { stream, updatesPending } = useLive();
  usePageControls();
  const dot = CONN_DOT[stream];
  return (
    <Link
      className={"mc-icon-button mc-conn-settings" + (view === "admin" ? " is-active" : "")}
      to={adminRoute(updatesPending ? "updates" : ADMIN_DEFAULT)}
      aria-current={view === "admin" ? "page" : undefined}
      aria-label={"Admin — event stream " + dot.label + (updatesPending ? "; update rollout in progress" : "")}
      title={"Admin · event stream: " + dot.label + (updatesPending ? " · update rollout in progress" : "")}
    >
      <SettingsIcon size={18} />
      <span className={"mc-conn-dot " + dot.cls} aria-hidden="true"></span>
      {updatesPending && <span className="mc-upd-dot" aria-hidden="true"></span>}
    </Link>
  );
}
