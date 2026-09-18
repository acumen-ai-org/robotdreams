import { StrictMode, useCallback, useState } from "react";
import { createRoot } from "react-dom/client";
import "@robotdreams/mission-control/style.css";
import "@robotdreams/mission-control/theme.css";
import {
  MissionControlProviders,
  ReportingView,
  Shell,
  type Theme,
  parseHash,
  buildHash,
  type Route,
} from "@robotdreams/mission-control";
import { OverviewView } from "@robotdreams/mission-control/views/overview/OverviewView";

function Host() {
  const [route, setRoute] = useState<Route>(() => parseHash("#/outcomes/spookify"));
  const onNavigate = useCallback((next: Route) => setRoute(next), []);
  const [el, setEl] = useState<HTMLElement | null>(null);
  const [theme, setTheme] = useState<Theme>("light");
  return (
    <div ref={setEl} className="mc-scope" style={{ height: "100vh" }}>
      <MissionControlProviders
        connection={{ apiBase: "http://127.0.0.1:8420", token: "smoke" }}
        router={{ route, onNavigate, href: buildHash }}
        themeTarget={el}
        theme={theme}
        onThemeChange={setTheme}
        live={false}
      >
        <Shell
          title="Smoke host"
          actions={
            <button type="button" onClick={() => setTheme(theme === "dark" ? "light" : "dark")}>
              {theme === "dark" ? "Light" : "Dark"}
            </button>
          }
        >
          {route.view === "overview" ? <OverviewView /> : <ReportingView />}
        </Shell>
      </MissionControlProviders>
    </div>
  );
}

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <Host />
  </StrictMode>,
);
