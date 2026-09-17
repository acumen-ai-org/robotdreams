import { useEffect, useRef, type ReactNode } from "react";
import { Shell } from "./components/Shell";
import { AdminLink, ThemeToggle } from "./components/TopbarActions";
import { OverviewView } from "./views/overview/OverviewView";
import { ReportingView } from "./views/reporting/ReportingView";
import { SchedulesView } from "./views/SchedulesView";
import { AdminView } from "./views/admin/AdminView";
import { useConnection } from "./state/ConnectionContext";
import { adminRoute, crossViewRoute, type ViewName } from "./lib/routes";
import { useRouter } from "./state/RouterContext";

export interface MissionControlShellProps {
  brand?: ReactNode;
  brandLabel?: string;
  title?: string;
  redirectToConnection?: boolean;
}

export function MissionControlShell({
  brand,
  brandLabel,
  title = "Mission Control",
  redirectToConnection,
}: MissionControlShellProps) {
  const conn = useConnection();
  const { route, navigate } = useRouter();
  const lastMain = useRef<ViewName>("overview");
  if (route.view !== "admin") lastMain.current = route.view;
  const mainView = route.view === "admin" ? lastMain.current : route.view;

  const redirect = redirectToConnection ?? conn.canConnect;
  useEffect(() => {
    if (redirect && !conn.hasToken && route.view !== "admin") {
      navigate(adminRoute("connection"));
    }
  }, [redirect, conn.hasToken, route.view, navigate]);

  return (
    <Shell
      brand={brand}
      brandLabel={brandLabel}
      title={title}
      actions={
        <>
          <ThemeToggle />
          <AdminLink />
        </>
      }
    >
      <OverviewView hidden={mainView !== "overview"} />
      <ReportingView active={mainView === "reporting"} />
      <SchedulesView active={mainView === "schedules"} />
      <AdminView
        hidden={route.view !== "admin"}
        onClose={() => {
          navigate(crossViewRoute(route, lastMain.current));
        }}
      />
    </Shell>
  );
}
