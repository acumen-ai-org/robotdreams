import { useEffect, useRef } from "react";
import { reportingRoute, type Route } from "../../lib/routes";
import { useVocabulary } from "../../state/VocabularyContext";
import { Link } from "../../components/Link";
import { instancesUnder, scopeChildren, type ScopeChild } from "../../lib/scopeTree";
import type { ScopeEntry } from "../../lib/types";

function levelFor(scopes: ScopeEntry[], path: string, depth: number, word: (d: number) => string): string {
  const exact = scopes.find((s) => s.path === path);
  if (exact && exact.level) return exact.level;
  return depth >= 1 && depth <= 5 ? word(depth - 1) : "level " + depth;
}

function Menu({
  label,
  items,
  currentPath,
  route,
  scopes,
  descendLabel,
}: {
  label: string;
  items: ScopeChild[];
  currentPath: string;
  route: Route;
  scopes: ScopeEntry[];
  descendLabel?: string;
}) {
  return (
    <details className={"mc-crumb-menu" + (descendLabel ? " mc-crumb-descend" : "")}>
      <summary aria-label={label}>{descendLabel ? descendLabel + " ▾" : "▾"}</summary>
      <ul className="mc-crumb-menu-list">
        {items.map((it) => {
          const n = instancesUnder(scopes, it.path);
          return (
            <li key={it.path}>
              <Link
                to={reportingRoute(route, { scope: it.path })}
                aria-current={it.path === currentPath ? "true" : undefined}
              >
                {it.name}
                <span className="mc-label-muted">{n + (n === 1 ? " instance" : " instances")}</span>
              </Link>
            </li>
          );
        })}
      </ul>
    </details>
  );
}

function Crumb({
  name,
  path,
  level,
  isCurrent,
  siblings,
  route,
  scopes,
}: {
  name: string;
  path: string;
  level: string;
  isCurrent: boolean;
  siblings: ScopeChild[] | null;
  route: Route;
  scopes: ScopeEntry[];
}) {
  return (
    <span className="mc-crumb">
      <Link
        className="mc-crumb-link"
        to={reportingRoute(route, { scope: path })}
        aria-current={isCurrent ? "true" : undefined}
      >
        {name}
      </Link>
      {level && <span className="mc-crumb-level mc-label-muted">{level}</span>}
      {siblings && siblings.length > 1 && (
        <Menu
          label={"Switch " + (level || "scope")}
          items={siblings}
          currentPath={path}
          route={route}
          scopes={scopes}
        />
      )}
    </span>
  );
}

export function ScopeBar({ route, scopes }: { route: Route; scopes: ScopeEntry[] }) {
  const barRef = useRef<HTMLElement>(null);
  const vocab = useVocabulary();

  useEffect(() => {
    const bar = barRef.current;
    if (!bar) return;
    const closeAll = (except: Element | null) => {
      bar.querySelectorAll("details.mc-crumb-menu[open]").forEach((d) => {
        if (d !== except) d.removeAttribute("open");
      });
    };
    const onToggle = (e: Event) => {
      const d = e.target as HTMLDetailsElement;
      if (d.open) closeAll(d);
    };
    const onDocClick = (e: MouseEvent) => {
      if (!(e.target as Element)?.closest?.("details.mc-crumb-menu")) closeAll(null);
    };
    bar.addEventListener("toggle", onToggle, true);
    document.addEventListener("click", onDocClick);
    return () => {
      bar.removeEventListener("toggle", onToggle, true);
      document.removeEventListener("click", onDocClick);
    };
  }, []);

  if (!scopes.length) return null;

  const scope = route.scope;
  const segs = scope ? scope.split("/") : [];
  const crumbs: React.ReactNode[] = [
    <Crumb
      key="root"
      name="All"
      path=""
      level=""
      isCurrent={segs.length === 0}
      siblings={null}
      route={route}
      scopes={scopes}
    />,
  ];

  let prefix = "";
  segs.forEach((seg, i) => {
    crumbs.push(
      <span key={"sep" + i} className="mc-crumb-sep" aria-hidden="true">
        ›
      </span>,
    );
    const parent = prefix;
    prefix = prefix ? prefix + "/" + seg : seg;
    crumbs.push(
      <Crumb
        key={prefix}
        name={seg}
        path={prefix}
        level={levelFor(scopes, prefix, i + 1, vocab.lower)}
        isCurrent={i === segs.length - 1}
        siblings={scopeChildren(scopes, parent)}
        route={route}
        scopes={scopes}
      />,
    );
  });

  const children = scopeChildren(scopes, scope);
  if (children.length) {
    const nextLevel = levelFor(scopes, children[0].path, segs.length + 1, vocab.lower);
    crumbs.push(
      <span key="sep-descend" className="mc-crumb-sep" aria-hidden="true">
        ›
      </span>,
      <Menu
        key="descend"
        label={"Choose a " + nextLevel}
        items={children}
        currentPath=""
        route={route}
        scopes={scopes}
        descendLabel={nextLevel}
      />,
    );
  }

  return (
    <nav ref={barRef} className="mc-scope-bar" aria-label="Report scope">
      {crumbs}
    </nav>
  );
}
