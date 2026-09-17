import type { ReactNode } from "react";
import { ThemeProvider, type Theme } from "./state/ThemeContext";
import { VocabularyProvider } from "./state/VocabularyContext";
import { ConnectionProvider, type ConnectionProviderProps } from "./state/ConnectionContext";
import { HashRouterProvider, RouterProvider, type RouterProviderProps } from "./state/RouterContext";
import { LiveProvider } from "./state/LiveContext";
import { ShellProvider } from "./state/ShellContext";
import type { ViewName } from "./lib/routes";

export interface MissionControlProvidersProps {
  children: ReactNode;
  connection?: Omit<ConnectionProviderProps, "children">;
  router?: Omit<RouterProviderProps, "children">;
  themeTarget?: HTMLElement | null;
  theme?: Theme;
  onThemeChange?: (next: Theme) => void;
  defaultTheme?: Theme;
  shell?: { persist?: boolean; initialVariants?: Partial<Record<ViewName, string>> };
  live?: boolean;
}

export function MissionControlProviders({
  children,
  connection,
  router,
  themeTarget,
  theme,
  onThemeChange,
  defaultTheme,
  shell,
  live = true,
}: MissionControlProvidersProps) {
  const routed = router ? (
    <RouterProvider {...router}>{children}</RouterProvider>
  ) : (
    <HashRouterProvider>{children}</HashRouterProvider>
  );
  return (
    <ThemeProvider target={themeTarget} theme={theme} onThemeChange={onThemeChange} defaultTheme={defaultTheme}>
      <VocabularyProvider>
        <ConnectionProvider {...connection}>
          <ShellProvider {...shell}>{live ? <LiveProvider>{routed}</LiveProvider> : routed}</ShellProvider>
        </ConnectionProvider>
      </VocabularyProvider>
    </ThemeProvider>
  );
}
