import type { ReactNode } from "react";
import { Topbar } from "./Topbar";
import { PageControlsProvider } from "../state/PageControlsContext";
import { useLive } from "../state/LiveContext";
import { useRouter } from "../state/RouterContext";
import { useTheme } from "../state/ThemeContext";

const STREAM_SAID: Record<string, string> = {
  live: "Live updates connected.",
  connecting: "Connecting to the live update stream.",
  reconnecting: "Live updates dropped; reconnecting. Reports refresh every 15 seconds meanwhile.",
  offline: "Live updates offline. Reports refresh every 15 seconds.",
};

export interface ShellProps {
  brand?: ReactNode;
  brandLabel?: string;
  title?: string;
  actions?: ReactNode;
  children?: ReactNode;
}

export function Shell({ brand, brandLabel, title, actions, children }: ShellProps) {
  const { route } = useRouter();
  const { stream } = useLive();
  const { registerScope } = useTheme();
  return (
    <PageControlsProvider view={route.view}>
      <div className="mc-app mc-scope" ref={registerScope}>
        <a className="mc-skip-link" href="#main-content">
          Skip to content
        </a>
        <Topbar brand={brand} brandLabel={brandLabel} title={title} actions={actions} />
        <p className="mc-sr-only" role="status" aria-live="polite">
          {STREAM_SAID[stream]}
        </p>
        {children}
      </div>
    </PageControlsProvider>
  );
}
