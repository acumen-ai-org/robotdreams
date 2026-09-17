import { useCallback, useEffect, useRef, useState } from "react";
import { usePageControls } from "../state/PageControlsContext";
import { viewRoute, type Route } from "../lib/routes";
import { useRouter } from "../state/RouterContext";
import {
  entityCode,
  nodeEmoji,
  NodeBadge,
  NODE_DEFAULT_EMOJI,
  RealmChip,
  SiteBadge,
  UniverseIcon,
  WorldBadge,
} from "../lib/vocabulary";
import { ChipPopover } from "./ChipPopover";
import { NodePicker } from "./NodePicker";
import { BarExpander } from "./BarExpander";
import { CheckIcon, EraserIcon, XIcon } from "./icons";
import { scopeChildren } from "../lib/scopeTree";
import { useVocabulary } from "../state/VocabularyContext";
import { useWorkers } from "../hooks/useWorkers";
import type { ScopeEntry } from "../lib/types";

export interface ScopeSeg {
  name: string;
  path: string;
  depth: number;
  index: number;
  siblings: number;
  realm?: number;
}

export function describePath(scopes: ScopeEntry[], path: string): ScopeSeg[] {
  if (!path) return [];
  const segs = path.split("/");
  const out: ScopeSeg[] = [];
  let prefix = "";
  let realm: number | undefined;
  for (let i = 0; i < segs.length; i++) {
    const kids = scopeChildren(scopes, prefix);
    const idx = kids.findIndex((k) => k.name === segs[i]);
    const depth = i + 1;
    const full = prefix ? prefix + "/" + segs[i] : segs[i];
    if (depth === 3) realm = idx < 0 ? 0 : idx % 10;
    out.push({
      name: segs[i],
      path: full,
      depth,
      index: depth === 3 ? (idx < 0 ? 0 : idx % 10) : idx + 1,
      siblings: kids.length,
      realm,
    });
    prefix = full;
  }
  return out;
}

export function ScopeChip({ seg, childCount, nodeCount }: { seg: ScopeSeg; childCount?: number; nodeCount?: number }) {
  const vocab = useVocabulary();
  const childType = seg.depth < 4 ? vocab.many(seg.depth) : undefined;
  const kids =
    seg.depth < 5 ? { ...(childType ? { childType, childCount } : {}), nodeType: vocab.many(4), nodeCount } : {};
  switch (seg.depth) {
    case 1:
      return (
        <ChipPopover info={{ name: seg.name, ...kids }}>
          <span className="mc-id-chip mc-chip-universe">
            <UniverseIcon size={13} className="mc-chip-icon" />
            {seg.name}
          </span>
        </ChipPopover>
      );
    case 2:
      return (
        <ChipPopover info={{ code: entityCode("W", seg.index, seg.siblings), name: seg.name, ...kids }}>
          <span className="mc-chip-pair">
            <WorldBadge n={seg.index} max={seg.siblings} size={18} />
            <span className="mc-chip-pair-name">{seg.name}</span>
          </span>
        </ChipPopover>
      );
    case 3:
      return <RealmChip n={seg.index} name={seg.name} {...kids} />;
    case 4:
      return <SiteBadge n={seg.index} max={seg.siblings} name={seg.name} realm={seg.realm} {...kids} />;
    default:
      return <NodeBadge n={seg.index} max={seg.siblings} name={seg.name} realm={seg.realm} />;
  }
}

const MAX_SCOPE_ITEMS = 1000;

export function ScopeSelector({ route, scopes }: { route: Route; scopes: ScopeEntry[] }) {
  const controls = usePageControls();
  const { navigate } = useRouter();
  const vocab = useVocabulary();
  const index = useWorkers(0, true, route.scopes);
  const LEVEL_ROWS = [vocab.one(0), vocab.many(1), vocab.many(2), vocab.many(3), vocab.many(4)];
  const open = controls.openPanel === "scope";

  const draft = route.scopes;
  const draftRoles = route.roles;
  const [showNodes, setShowNodes] = useState(false);
  const triggerRef = useRef<HTMLButtonElement>(null);
  const original = useRef<{ scopes: string[]; roles: string[] } | null>(null);
  const wasOpen = useRef(false);
  const committed = useRef(false);

  const setSelection = useCallback(
    (scopesNext: string[], rolesNext: string[]) => {
      navigate(viewRoute(route, { scopes: scopesNext, scope: scopesNext[0] || "", roles: rolesNext }), {
        replace: true,
      });
    },
    [route, navigate],
  );

  useEffect(() => {
    if (open && !wasOpen.current) {
      original.current = { scopes: route.scopes, roles: route.roles };
      committed.current = false;
      setShowNodes(false);
    }
    if (!open && wasOpen.current && !committed.current && original.current) {
      const o = original.current;
      if (o.scopes.join(",") !== route.scopes.join(",") || o.roles.join(",") !== route.roles.join(",")) {
        setSelection(o.scopes, o.roles);
      }
    }
    wasOpen.current = open;
  }, [open, route.scopes, route.roles, setSelection]);

  const toggle = (path: string) =>
    setSelection(
      draft.includes(path) ? draft.filter((p) => p !== path && !p.startsWith(path + "/")) : [...draft, path],
      draftRoles,
    );

  const selectOnly = (path: string) => {
    const d = path.split("/").length;
    const next = draft.filter((p) => {
      const segs = p.split("/");
      if (segs.length < d) return true;
      return segs.slice(0, d).join("/") === path;
    });
    if (!next.includes(path)) next.push(path);
    setSelection(next, draftRoles);
  };

  const toggleRole = (r: string) =>
    setSelection(draft, draftRoles.includes(r) ? draftRoles.filter((x) => x !== r) : [...draftRoles, r]);

  const apply = () => {
    committed.current = true;
    controls.setOpenPanel(null);
  };

  const picks = draft.length + draftRoles.length;

  const rows: Array<{ title: string; depth: number; kids: ScopeSeg[]; hidden: number }> = [];
  {
    let parents: Array<{ path: string; realm?: number }> = [{ path: "" }];
    let budget = MAX_SCOPE_ITEMS;
    for (let d = 0; d < LEVEL_ROWS.length && budget > 0; d++) {
      const depth = d + 1;
      const kids: ScopeSeg[] = [];
      for (const p of parents) {
        const cs = scopeChildren(scopes, p.path);
        cs.forEach((c, i) => {
          kids.push({
            name: c.name,
            path: c.path,
            depth,
            index: depth === 3 ? i % 10 : i + 1,
            siblings: cs.length,
            realm: depth === 3 ? i % 10 : p.realm,
          });
        });
      }
      if (!kids.length) break;
      if (!(depth === 1 && kids.length === 1)) {
        const shown = kids.slice(0, budget);
        budget -= shown.length;
        rows.push({ title: LEVEL_ROWS[d], depth, kids: shown, hidden: kids.length - shown.length });
      }

      const selected = kids.filter((k) => draft.includes(k.path));
      if (selected.length) {
        parents = selected.map((k) => ({ path: k.path, realm: k.realm }));
        continue;
      }
      if (kids.length === 1) {
        parents = [{ path: kids[0].path, realm: kids[0].realm }];
        continue;
      }
      break;
    }
  }

  const liveSegs = describePath(scopes, route.scope);
  const tail = liveSegs[liveSegs.length - 1];
  const extra = route.scopes.length - 1;

  const chip = open ? (
    <span className="mc-control-empty">.......</span>
  ) : (
    <>
      {tail ? (
        <>
          <ScopeChip
            seg={tail}
            childCount={scopeChildren(scopes, tail.path).length}
            nodeCount={index.nodesUnder.get(tail.path)}
          />
          {extra > 0 && <span className="mc-control-more">+{extra}</span>}
        </>
      ) : (
        <span className="mc-control-empty">All scopes</span>
      )}
      {route.roles.length > 0 && (
        <span className="mc-control-roles" title={route.roles.join(", ")}>
          <span aria-hidden="true">{route.roles.length === 1 ? nodeEmoji(route.roles[0]) : NODE_DEFAULT_EMOJI}</span>
          {route.roles.length > 1 && <span className="mc-control-more">{route.roles.length}</span>}
        </span>
      )}
    </>
  );

  return (
    <>
      <button
        ref={triggerRef}
        className={"mc-toolbox-button mc-toolbox-scope" + (open ? " is-open" : "")}
        type="button"
        aria-expanded={open}
        aria-label={
          (route.scopes.length ? "Scope: " + route.scopes.join(", ") : "Scope: all scopes") +
          (route.roles.length ? " — roles: " + route.roles.join(", ") : "")
        }
        onClick={() => controls.togglePanel("scope")}
        data-mc-panel-trigger=""
      >
        {chip}
        <span className="mc-rc-caret" aria-hidden="true">
          {open ? "\u25b4" : "\u25be"}
        </span>
      </button>

      <BarExpander
        open={open}
        triggerRef={triggerRef}
        actions={
          <>
            {picks > 0 && (
              <button
                className="mc-bar-action"
                type="button"
                aria-label="Clear selection"
                title="Clear selection"
                onClick={() => setSelection([], [])}
              >
                <EraserIcon size={14} />
              </button>
            )}
            <button className="mc-bar-action is-primary" type="button" aria-label="Apply" title="Apply" onClick={apply}>
              <CheckIcon size={14} />
            </button>
            <button
              className="mc-bar-action"
              type="button"
              aria-label="Cancel"
              title="Cancel"
              onClick={() => controls.setOpenPanel(null)}
            >
              <XIcon size={14} />
            </button>
          </>
        }
        controlCopy={
          <span className="mc-view-select">
            <span className="mc-view-select-label">
              <UniverseIcon size={11} /> Scope
            </span>
            <span className="mc-control-group">
              <button
                className="mc-toolbox-button mc-toolbox-scope is-open"
                type="button"
                aria-expanded={true}
                onClick={() => controls.setOpenPanel(null)}
              >
                {chip}
                <span className="mc-rc-caret" aria-hidden="true">
                  {"\u25b4"}
                </span>
              </button>
            </span>
          </span>
        }
        left={
          <div className="mc-scope-hierarchy">
            {rows.map((row) => {
              const asCircles = row.depth === 2;
              return (
                <div className={"mc-scope-row" + (asCircles ? " mc-scope-row-worlds" : "")} key={row.title}>
                  <span className="mc-scope-row-label">{row.title}</span>
                  <div className={asCircles ? "mc-world-grid" : "mc-scope-row-items"}>
                    {row.kids.map((seg) => {
                      const childCount = scopeChildren(scopes, seg.path).length;
                      const nodeCount = index.nodesUnder.get(seg.path);
                      const picked = draft.includes(seg.path);
                      const cls = (base: string) => base + (picked ? " is-picked" : "");
                      if (asCircles) {
                        return (
                          <button
                            key={seg.path}
                            type="button"
                            className={cls("mc-world-pick")}
                            aria-pressed={picked}
                            title="Click to select; double-click to select only this"
                            onClick={() => toggle(seg.path)}
                            onDoubleClick={() => selectOnly(seg.path)}
                          >
                            <span className="mc-pick-tick" aria-hidden="true">
                              {picked ? "\u2713" : ""}
                            </span>
                            <ChipPopover
                              info={{
                                code: entityCode("W", seg.index, seg.siblings),
                                name: seg.name,
                                childType: vocab.many(2),
                                childCount,
                                nodeType: vocab.many(4),
                                nodeCount,
                              }}
                            >
                              <WorldBadge n={seg.index} max={seg.siblings} size={46} />
                            </ChipPopover>
                            <span className="mc-world-pick-name">{seg.name}</span>
                          </button>
                        );
                      }
                      return (
                        <button
                          key={seg.path}
                          type="button"
                          className={cls("mc-scope-pick")}
                          aria-pressed={picked}
                          title={seg.path + " — double-click to select only this"}
                          onClick={() => toggle(seg.path)}
                          onDoubleClick={() => selectOnly(seg.path)}
                        >
                          <span className="mc-pick-tick" aria-hidden="true">
                            {picked ? "\u2713" : ""}
                          </span>
                          <ScopeChip seg={seg} childCount={childCount} nodeCount={nodeCount} />
                        </button>
                      );
                    })}
                    {row.hidden > 0 && <span className="mc-label-muted">+{row.hidden} more not shown</span>}
                  </div>
                </div>
              );
            })}
          </div>
        }
        right={
          <>
            <button
              type="button"
              className="mc-link-button mc-scope-nodes-toggle"
              aria-expanded={showNodes}
              onClick={() => setShowNodes((v) => !v)}
            >
              Select nodes
              <span aria-hidden="true">{showNodes ? " \u25be" : " \u25b8"}</span>
            </button>
            {showNodes && <NodePicker index={index} roles={draftRoles} onToggleRole={toggleRole} />}
          </>
        }
      />
    </>
  );
}
