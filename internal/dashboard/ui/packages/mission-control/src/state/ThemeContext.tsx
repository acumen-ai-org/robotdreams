import { createContext, useCallback, useContext, useEffect, useState, type ReactNode } from "react";
import { readStored, writeStored } from "../lib/storage";

const LS_THEME = "theme";

export type Theme = "light" | "dark";

interface ThemeValue {
  theme: Theme;
  toggle: () => void;
  setTheme: (next: Theme) => void;
  controlled: boolean;
  root: HTMLElement;
  registerScope: (el: HTMLElement | null) => void;
}

const ThemeContext = createContext<ThemeValue | null>(null);

export interface ThemeProviderProps {
  children: ReactNode;
  target?: HTMLElement | null;
  theme?: Theme;
  onThemeChange?: (next: Theme) => void;
  defaultTheme?: Theme;
}

function initialTheme(fallback?: Theme): Theme {
  const stored = readStored(LS_THEME);
  if (stored === "light" || stored === "dark") return stored;
  if (fallback) return fallback;
  return window.matchMedia?.("(prefers-color-scheme: dark)").matches ? "dark" : "light";
}

export function ThemeProvider({
  children,
  target,
  theme: controlledTheme,
  onThemeChange,
  defaultTheme,
}: ThemeProviderProps) {
  const controlled = controlledTheme !== undefined;
  const [ownTheme, setOwnTheme] = useState<Theme>(() => initialTheme(defaultTheme));
  const [scope, setScope] = useState<HTMLElement | null>(null);
  const theme = controlledTheme ?? ownTheme;
  const root = target ?? scope ?? document.documentElement;
  const registerScope = useCallback((el: HTMLElement | null) => setScope(el), []);

  useEffect(() => {
    if (target) {
      target.setAttribute("data-theme", theme);
      return;
    }
    if (theme === "dark") {
      document.documentElement.setAttribute("data-theme", "dark");
    } else {
      document.documentElement.removeAttribute("data-theme");
    }
  }, [theme, target]);

  useEffect(() => {
    if (controlled || defaultTheme || readStored(LS_THEME)) return;
    const mq = window.matchMedia?.("(prefers-color-scheme: dark)");
    if (!mq) return;
    const onChange = (e: MediaQueryListEvent) => setOwnTheme(e.matches ? "dark" : "light");
    mq.addEventListener("change", onChange);
    return () => mq.removeEventListener("change", onChange);
  }, [controlled, defaultTheme]);

  const setTheme = useCallback(
    (next: Theme) => {
      onThemeChange?.(next);
      if (controlled) return;
      writeStored(LS_THEME, next);
      setOwnTheme(next);
    },
    [controlled, onThemeChange],
  );

  const toggle = useCallback(() => setTheme(theme === "dark" ? "light" : "dark"), [setTheme, theme]);

  return (
    <ThemeContext.Provider value={{ theme, toggle, setTheme, controlled, root, registerScope }}>
      {children}
    </ThemeContext.Provider>
  );
}

export function useTheme(): ThemeValue {
  const v = useContext(ThemeContext);
  if (!v) throw new Error("useTheme outside ThemeProvider");
  return v;
}

export function useThemeRoot(): HTMLElement {
  return useContext(ThemeContext)?.root ?? document.documentElement;
}
