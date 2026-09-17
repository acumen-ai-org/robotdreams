import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from "react";
import type { ViewName } from "../lib/routes";

export type PanelId = "experiments" | "search" | "scope" | "filters" | "legend";

interface PageControlsValue {
  slot: HTMLElement | null;
  expandedSlot: HTMLElement | null;
  actionsSlot: HTMLElement | null;
  toolsSlot: HTMLElement | null;
  filtersSlot: HTMLElement | null;
  railSlot: HTMLElement | null;
  openPanel: PanelId | null;
  setOpenPanel: (id: PanelId | null) => void;
  togglePanel: (id: PanelId) => void;
  registerSlot: (el: HTMLElement | null) => void;
  registerExpandedSlot: (el: HTMLElement | null) => void;
  registerActionsSlot: (el: HTMLElement | null) => void;
  registerToolsSlot: (el: HTMLElement | null) => void;
  registerFiltersSlot: (el: HTMLElement | null) => void;
  registerRailSlot: (el: HTMLElement | null) => void;
}

const PageControlsContext = createContext<PageControlsValue | null>(null);

interface ProviderProps {
  view: ViewName;
  chrome?: boolean;
  children: ReactNode;
}

export function PageControlsProvider({ view, chrome = false, children }: ProviderProps) {
  const [slot, setSlot] = useState<HTMLElement | null>(null);
  const [expandedSlot, setExpandedSlot] = useState<HTMLElement | null>(null);
  const [actionsSlot, setActionsSlot] = useState<HTMLElement | null>(null);
  const [toolsSlot, setToolsSlot] = useState<HTMLElement | null>(null);
  const [filtersSlot, setFiltersSlot] = useState<HTMLElement | null>(null);
  const [railSlot, setRailSlot] = useState<HTMLElement | null>(null);
  const [openPanel, setOpenPanel] = useState<PanelId | null>(null);

  useEffect(() => {
    setOpenPanel(null);
  }, [view]);

  useEffect(() => {
    if (!openPanel) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") setOpenPanel(null);
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [openPanel]);

  useEffect(() => {
    if (!openPanel || !expandedSlot) return;
    const panel = expandedSlot.closest(".mc-topbar-expander") ?? expandedSlot;
    const onDown = (e: Event) => {
      const t = e.target as Element | null;
      if (!t || typeof t.closest !== "function") return;
      if (panel.contains(t)) return;
      if (t.closest("[data-mc-panel-trigger]")) return;
      if (t.closest(".mc-chip-pop")) return;
      setOpenPanel(null);
    };
    window.addEventListener("pointerdown", onDown, true);
    return () => window.removeEventListener("pointerdown", onDown, true);
  }, [openPanel, expandedSlot]);

  const togglePanel = useCallback((id: PanelId) => setOpenPanel((cur) => (cur === id ? null : id)), []);
  const registerSlot = useCallback(
    (el: HTMLElement | null) =>
      setSlot((cur) => {
        if (el && cur && cur !== el) {
          console.warn(
            "PageControls: the page toolbox slot was registered twice — render either a Topbar or a PageChrome, not both.",
          );
        }
        return el;
      }),
    [],
  );
  const registerExpandedSlot = useCallback((el: HTMLElement | null) => setExpandedSlot(el), []);
  const registerActionsSlot = useCallback((el: HTMLElement | null) => setActionsSlot(el), []);
  const registerToolsSlot = useCallback((el: HTMLElement | null) => setToolsSlot(el), []);
  const registerFiltersSlot = useCallback((el: HTMLElement | null) => setFiltersSlot(el), []);
  const registerRailSlot = useCallback((el: HTMLElement | null) => setRailSlot(el), []);

  const value = useMemo(
    () => ({
      slot,
      expandedSlot,
      actionsSlot,
      toolsSlot,
      filtersSlot,
      railSlot,
      openPanel,
      setOpenPanel,
      togglePanel,
      registerSlot,
      registerExpandedSlot,
      registerActionsSlot,
      registerToolsSlot,
      registerFiltersSlot,
      registerRailSlot,
    }),
    [
      slot,
      expandedSlot,
      actionsSlot,
      toolsSlot,
      filtersSlot,
      railSlot,
      openPanel,
      togglePanel,
      registerSlot,
      registerExpandedSlot,
      registerActionsSlot,
      registerToolsSlot,
      registerFiltersSlot,
      registerRailSlot,
    ],
  );

  return (
    <PageControlsContext.Provider value={value}>
      {chrome && <PageChrome />}
      {children}
    </PageControlsContext.Provider>
  );
}

export function PageChrome() {
  const controls = usePageControls();
  return (
    <div className="mc-chrome">
      <div className="mc-topbar-bar">
        <div className="mc-topbar-main">
          <div className="mc-page-toolbox" ref={controls.registerSlot} />
        </div>
        <div className="mc-topbar-actions">
          <span className="mc-topbar-action-slot" ref={controls.registerActionsSlot} />
        </div>
      </div>
      {controls.openPanel && (
        <div className="mc-topbar-expander">
          <div className="mc-expander-inner" ref={controls.registerExpandedSlot} />
        </div>
      )}
    </div>
  );
}

export function usePageControls(): PageControlsValue {
  const v = useContext(PageControlsContext);
  if (!v) throw new Error("usePageControls outside PageControlsProvider");
  return v;
}
