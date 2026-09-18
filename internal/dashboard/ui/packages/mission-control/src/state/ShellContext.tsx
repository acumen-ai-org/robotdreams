import { createContext, useCallback, useContext, useMemo, useState, type ReactNode } from "react";
import { clampSideWidth } from "../components/SidePanel";
import { VIEW_VARIANTS, getVariant, setVariant } from "../lib/variants";
import type { ViewName } from "../lib/routes";
import { readStored, writeStored } from "../lib/storage";

const DEFAULT_SIDE_WIDTH = 380;

export interface ShellState {
  sideCollapsed: boolean;
  sideWidth: number;
  toggleSide: () => void;
  setSideWidth: (w: number) => void;
  variant: (view: ViewName) => string;
  selectVariant: (view: ViewName, id: string) => void;
}

const ShellContext = createContext<ShellState | null>(null);

interface Options {
  persist?: boolean;
  initialVariants?: Partial<Record<ViewName, string>>;
}

function useShellState({ persist = true, initialVariants }: Options): ShellState {
  const [sideCollapsed, setSideCollapsed] = useState<boolean>(() => persist && readStored("sideCollapsed") === "1");
  const [sideWidth, setSideWidthState] = useState<number>(() =>
    clampSideWidth((persist ? Number(readStored("sideWidth")) : 0) || DEFAULT_SIDE_WIDTH),
  );
  const [variants, setVariants] = useState<Record<ViewName, string>>(() => ({
    overview: initialVariants?.overview ?? (persist ? getVariant("overview") : VIEW_VARIANTS.overview[0].id),
    reporting: initialVariants?.reporting ?? (persist ? getVariant("reporting") : VIEW_VARIANTS.reporting[0].id),
    schedules: initialVariants?.schedules ?? (persist ? getVariant("schedules") : VIEW_VARIANTS.schedules[0].id),
    admin: initialVariants?.admin ?? (persist ? getVariant("admin") : VIEW_VARIANTS.admin[0].id),
  }));

  const toggleSide = useCallback(() => {
    setSideCollapsed((cur) => {
      const next = !cur;
      if (persist) writeStored("sideCollapsed", next ? "1" : "0");
      return next;
    });
  }, [persist]);

  const setSideWidth = useCallback(
    (w: number) => {
      const next = clampSideWidth(w);
      setSideWidthState(next);
      if (persist) writeStored("sideWidth", String(next));
    },
    [persist],
  );

  const variant = useCallback((view: ViewName) => variants[view], [variants]);
  const selectVariant = useCallback(
    (view: ViewName, id: string) => {
      if (persist) setVariant(view, id);
      setVariants((cur) => ({ ...cur, [view]: id }));
    },
    [persist],
  );

  return useMemo(
    () => ({ sideCollapsed, sideWidth, toggleSide, setSideWidth, variant, selectVariant }),
    [sideCollapsed, sideWidth, toggleSide, setSideWidth, variant, selectVariant],
  );
}

export function ShellProvider({ children, ...opts }: Options & { children: ReactNode }) {
  return <ShellContext.Provider value={useShellState(opts)}>{children}</ShellContext.Provider>;
}

const LOCAL: Options = { persist: false };

export function useShell(): ShellState {
  const shared = useContext(ShellContext);
  const local = useShellState(LOCAL);
  return shared ?? local;
}
