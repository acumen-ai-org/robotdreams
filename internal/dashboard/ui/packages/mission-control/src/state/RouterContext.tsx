import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from "react";
import { buildHash, parseHash, type Route } from "../lib/routes";

export interface NavigateOptions {
  replace?: boolean;
}

export interface RouterValue {
  route: Route;
  navigate: (to: Route | string, opts?: NavigateOptions) => void;
  href: (to: Route | string) => string;
}

const RouterContext = createContext<RouterValue | null>(null);

export function useRouter(): RouterValue {
  const v = useContext(RouterContext);
  if (!v) throw new Error("useRouter outside a RouterProvider or HashRouterProvider");
  return v;
}

export function useRoute(): Route {
  return useRouter().route;
}

export function HashRouterProvider({ children }: { children: ReactNode }) {
  const [route, setRoute] = useState<Route>(() => parseHash(window.location.hash));

  useEffect(() => {
    const onHash = () => setRoute(parseHash(window.location.hash));
    window.addEventListener("hashchange", onHash);
    return () => window.removeEventListener("hashchange", onHash);
  }, []);

  const href = useCallback((to: Route | string) => (typeof to === "string" ? to : buildHash(to)), []);
  const navigate = useCallback(
    (to: Route | string, opts?: NavigateOptions) => {
      const h = href(to);
      if (opts?.replace) {
        window.history.replaceState(null, "", h);
        window.dispatchEvent(new HashChangeEvent("hashchange"));
      } else {
        window.location.hash = h;
      }
    },
    [href],
  );

  const value = useMemo(() => ({ route, navigate, href }), [route, navigate, href]);
  return <RouterContext.Provider value={value}>{children}</RouterContext.Provider>;
}

export interface RouterProviderProps {
  route: Route;
  onNavigate: (next: Route, opts: { replace: boolean }) => void;
  href?: (next: Route) => string;
  children: ReactNode;
}

export function RouterProvider({ route, onNavigate, href: hrefProp, children }: RouterProviderProps) {
  const href = useCallback(
    (to: Route | string) => (typeof to === "string" ? to : hrefProp ? hrefProp(to) : buildHash(to)),
    [hrefProp],
  );
  const navigate = useCallback(
    (to: Route | string, opts?: NavigateOptions) => {
      if (typeof to === "string") {
        window.location.assign(to);
        return;
      }
      onNavigate(to, { replace: !!opts?.replace });
    },
    [onNavigate],
  );
  const value = useMemo(() => ({ route, navigate, href }), [route, navigate, href]);
  return <RouterContext.Provider value={value}>{children}</RouterContext.Provider>;
}
