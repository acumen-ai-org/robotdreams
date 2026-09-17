import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import "@fontsource-variable/inter";
import "@fontsource-variable/space-grotesk";
import "@fontsource-variable/jetbrains-mono";
import "@robotdreams/mission-control/style.css";
import "@robotdreams/mission-control/theme.css";
import "./app.css";
import { MissionControl } from "@robotdreams/mission-control";
import { Brand, BRAND_LABEL } from "./brand/Brand";

declare global {
  interface Window {
    __DREAM_API_BASE__?: string;
  }
}

const DEV_TOKEN: string = import.meta.env.DEV ? import.meta.env.VITE_DREAM_DEV_TOKEN || "" : "";

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <MissionControl
      brand={Brand}
      brandLabel={BRAND_LABEL}
      connection={{ apiBase: window.__DREAM_API_BASE__ || "", fallbackToken: DEV_TOKEN, persist: true }}
    />
  </StrictMode>,
);
