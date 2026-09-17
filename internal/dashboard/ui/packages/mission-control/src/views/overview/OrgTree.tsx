import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { statusDotClass } from "../../components/shared";
import { scopeOf, type WorkerIndex } from "../../hooks/useWorkers";
import { severityRank } from "../../lib/triage";
import { placeRows, reportRows, rowInSelection, type Row } from "../../lib/orgRows";
import { useVocabulary } from "../../state/VocabularyContext";
import type { ScopeEntry, SummaryTile } from "../../lib/types";
import { phaseClass, phaseOf, type RolloutNode } from "../../lib/updates";
import { appHost, type NodeApp } from "../../lib/apps";
import { DelayedLoading } from "../../components/Async";

export type OrgTreeMode = "places" | "reports";

interface Props {
  index: WorkerIndex;
  scopes: ScopeEntry[];
  mode?: OrgTreeMode;
  tilesByScope: Map<string, SummaryTile[]>;
  search: string;
  pulsed: ReadonlySet<string>;
  selected: string | null;
  updateOf?: Map<string, RolloutNode>;
  appOf?: Map<string, NodeApp>;
  onSelect: (id: string | null) => void;
  onMatchCount?: (n: number) => void;
}

function worstByScope(byScope: Map<string, SummaryTile[]>): Map<string, string> {
  const out = new Map<string, string>();
  for (const [scope, tiles] of byScope) {
    let worst = "";
    for (const t of tiles) {
      if (!t.status) continue;
      if (!worst || severityRank(t.status) < severityRank(worst)) worst = t.status;
    }
    if (worst) out.set(scope, worst);
  }
  return out;
}

const ROW_BUDGET = 25;

export function OrgTree({
  index,
  scopes,
  mode = "places",
  tilesByScope,
  search,
  pulsed,
  selected,
  updateOf,
  appOf,
  onSelect,
  onMatchCount,
}: Props) {
  const vocab = useVocabulary();
  const [collapsed, setCollapsed] = useState<Set<string>>(new Set());
  const [focusID, setFocusID] = useState<string>("");
  const treeRef = useRef<HTMLUListElement>(null);
  const opened = useRef(false);

  const rows = useMemo(
    () => (mode === "reports" ? reportRows(index) : placeRows(index, scopes)),
    [mode, index, scopes],
  );

  useEffect(() => {
    if (opened.current) return;
    const roots = rows.childrenOf.get("") || [];
    if (!roots.length) return;
    opened.current = true;

    const open = new Set<string>();
    let shown = roots.length;
    let level = roots.map((r) => r.id);
    while (level.length) {
      const next: string[] = [];
      for (const id of level) {
        const kids = rows.childrenOf.get(id) || [];
        if (!kids.length) continue;
        if (shown + kids.length > ROW_BUDGET) continue;
        shown += kids.length;
        open.add(id);
        for (const k of kids) next.push(k.id);
      }
      if (!next.length) break;
      level = next;
    }

    const shut = new Set<string>();
    const walk = (parent: string) => {
      for (const r of rows.childrenOf.get(parent) || []) {
        if ((rows.childrenOf.get(r.id) || []).length && !open.has(r.id)) shut.add(r.id);
        walk(r.id);
      }
    };
    walk("");
    if (shut.size) setCollapsed(shut);
  }, [rows]);

  const preFilter = useRef<Set<string> | null>(null);
  useEffect(() => {
    if (!rows.byId.size) return;
    if (index.matchAll) {
      if (preFilter.current) {
        setCollapsed(preFilter.current);
        preFilter.current = null;
      }
      return;
    }
    setCollapsed((cur) => {
      if (!preFilter.current) preFilter.current = cur;

      const hasMatch = new Map<string, boolean>();
      const compute = (id: string): boolean => {
        const row = rows.byId.get(id);
        let has = !!row && row.kind === "node" && rowInSelection(index, row);
        for (const k of rows.childrenOf.get(id) || []) if (compute(k.id)) has = true;
        if (row && row.kind === "place") {
          has = rowInSelection(index, row) && (has || (rows.roll.get(row.id)?.total ?? 0) === 0);
        }
        hasMatch.set(id, has);
        return has;
      };
      for (const r of rows.childrenOf.get("") || []) compute(r.id);

      const open = new Set<string>();
      const roots = (rows.childrenOf.get("") || []).map((r) => r.id);
      let shown = roots.length;
      let level = roots.filter((id) => hasMatch.get(id));
      while (level.length) {
        const next: string[] = [];
        for (const id of level) {
          const kids = (rows.childrenOf.get(id) || []).map((k) => k.id);
          if (!kids.length) continue;
          if (shown + kids.length > ROW_BUDGET) continue;
          shown += kids.length;
          open.add(id);
          for (const k of kids) if (hasMatch.get(k)) next.push(k);
        }
        if (!next.length) break;
        level = next;
      }

      const shut = new Set<string>();
      const walk = (parent: string) => {
        for (const r of rows.childrenOf.get(parent) || []) {
          if ((rows.childrenOf.get(r.id) || []).length && !open.has(r.id)) shut.add(r.id);
          walk(r.id);
        }
      };
      walk("");
      return shut;
    });
  }, [index, rows]);

  const worst = useMemo(() => worstByScope(tilesByScope), [tilesByScope]);

  const matches = useMemo(() => {
    const q = search.trim().toLowerCase();
    if (!q) return new Set<string>();
    const hit = new Set<string>();
    for (const row of rows.byId.values()) {
      const hay = row.kind === "node" ? row.id + " " + (row.worker?.role || "") : row.label + " " + (row.path || "");
      if (hay.toLowerCase().includes(q)) hit.add(row.id);
    }
    return hit;
  }, [rows, search]);

  useEffect(() => {
    onMatchCount?.(matches.size);
  }, [matches, onMatchCount]);

  const visible = useMemo(() => {
    const out: Array<{ row: Row; depth: number }> = [];
    const walk = (parent: string, depth: number) => {
      for (const r of rows.childrenOf.get(parent) || []) {
        out.push({ row: r, depth });
        if (!collapsed.has(r.id)) walk(r.id, depth + 1);
      }
    };
    walk("", 0);
    return out;
  }, [rows, collapsed]);

  const current = focusID || visible[0]?.row.id || "";

  const move = useCallback(
    (delta: number) => {
      const i = visible.findIndex((r) => r.row.id === current);
      const next = visible[Math.min(Math.max(i + delta, 0), visible.length - 1)];
      if (next) setFocusID(next.row.id);
    },
    [visible, current],
  );

  const onKeyDown = (e: React.KeyboardEvent) => {
    const hasKids = (rows.childrenOf.get(current) || []).length > 0;
    switch (e.key) {
      case "ArrowDown":
        e.preventDefault();
        move(1);
        break;
      case "ArrowUp":
        e.preventDefault();
        move(-1);
        break;
      case "ArrowRight":
        e.preventDefault();
        if (hasKids && collapsed.has(current)) {
          setCollapsed((c) => new Set([...c].filter((x) => x !== current)));
        } else if (hasKids) {
          move(1);
        }
        break;
      case "ArrowLeft": {
        e.preventDefault();
        if (hasKids && !collapsed.has(current)) {
          setCollapsed((c) => new Set(c).add(current));
        } else {
          const parent = rows.parentOf.get(current);
          if (parent) setFocusID(parent);
        }
        break;
      }
      case "Home":
        e.preventDefault();
        if (visible[0]) setFocusID(visible[0].row.id);
        break;
      case "End":
        e.preventDefault();
        if (visible.length) setFocusID(visible[visible.length - 1].row.id);
        break;
      case "Enter":
      case " ":
        e.preventDefault();
        onSelect(selected === current ? null : current);
        break;
    }
  };

  useEffect(() => {
    if (!focusID) return;
    treeRef.current?.querySelector(`[data-id="${CSS.escape(focusID)}"]`)?.scrollIntoView({ block: "nearest" });
  }, [focusID]);

  if (index.error) return <p className="mc-form-error">{index.error}</p>;
  if (index.pending) return <DelayedLoading />;
  if (!visible.length) return <p className="mc-empty-state">No workers connected yet.</p>;

  return (
    <>
      <div className="mc-org-head" aria-hidden="true">
        <span>{mode === "reports" ? "Node" : "Place · node"}</span>
        <span>Role</span>
        <span className="mc-org-h-right">Beneath</span>
        <span>App</span>
        <span>Update</span>
        <span>Health</span>
      </div>
      <ul
        className="mc-org-tree"
        role="tree"
        aria-label={mode === "reports" ? "Reporting hierarchy" : "Structure: places and the nodes reporting for them"}
        ref={treeRef}
        tabIndex={0}
        onKeyDown={onKeyDown}
        onFocus={() => !focusID && setFocusID(visible[0]?.row.id || "")}
      >
        {visible.map(({ row, depth }) => {
          const kids = rows.childrenOf.get(row.id) || [];
          const worker = row.worker;
          const scope = row.kind === "place" ? row.path || "" : scopeOf(worker);
          const roll = rows.roll.get(row.id) || { active: 0, total: 0 };
          const worstHere = scope ? worst.get(scope) || "" : "";
          const outside = !rowInSelection(index, row);
          const upd = worker ? updateOf?.get(worker.id) : undefined;
          const app = worker ? appOf?.get(worker.id) : undefined;
          return (
            // eslint-disable-next-line jsx-a11y/click-events-have-key-events
            <li
              key={row.id}
              role="treeitem"
              aria-level={depth + 1}
              aria-selected={selected === row.id}
              aria-expanded={kids.length ? !collapsed.has(row.id) : undefined}
              data-id={row.id}
              className={
                "mc-org-row" +
                (row.kind === "place" ? " mc-org-place" : "") +
                (selected === row.id ? " is-selected" : "") +
                (current === row.id ? " is-focused" : "") +
                (matches.has(row.id) ? " is-match" : "") +
                (outside ? " is-outside" : "")
              }
              onClick={() => {
                setFocusID(row.id);
                onSelect(selected === row.id ? null : row.id);
              }}
            >
              <span className="mc-org-node" style={{ paddingLeft: depth * 16 }}>
                <button
                  className="mc-org-twisty"
                  type="button"
                  tabIndex={-1}
                  aria-hidden="true"
                  disabled={!kids.length}
                  onClick={(e) => {
                    e.stopPropagation();
                    setCollapsed((c) => {
                      const next = new Set(c);
                      if (next.has(row.id)) next.delete(row.id);
                      else next.add(row.id);
                      return next;
                    });
                  }}
                >
                  {kids.length ? (collapsed.has(row.id) ? "▸" : "▾") : "·"}
                </button>
                {row.kind === "node" ? (
                  <span
                    className={"mc-status-dot " + statusDotClass(worker?.status || "disconnected")}
                    aria-hidden="true"
                  />
                ) : (
                  <span className="mc-org-place-mark" aria-hidden="true">
                    ◻
                  </span>
                )}
                <span className="mc-org-id">{row.label}</span>
                <span className={"mc-org-pulse" + (pulsed.has(row.id) ? " mc-pulse" : "")} aria-hidden="true">
                  ●
                </span>
              </span>

              <span className="mc-org-role mc-label-muted">
                {row.kind === "node" ? worker?.role || "" : vocab.lower(row.level)}
              </span>

              <span
                className="mc-org-roll mc-mono"
                title={
                  row.kind === "place" || roll.total > 0
                    ? `${roll.active} of ${roll.total} connected beneath`
                    : undefined
                }
              >
                {row.kind === "place" || roll.total > 0 ? `${roll.active}/${roll.total}` : ""}
              </span>
              <span
                className="mc-org-app"
                title={
                  app ? "Serves " + appHost(app.url) + (app.description ? " — " + app.description : "") : undefined
                }
              >
                {app ? "↗" : ""}
              </span>
              {upd ? (
                <span
                  className={"mc-org-update " + phaseClass(upd.status)}
                  title={phaseOf(upd.status).hint + (upd.current_version ? " (on " + upd.current_version + ")" : "")}
                >
                  {phaseOf(upd.status).label}
                </span>
              ) : (
                <span aria-hidden="true" />
              )}
              {worstHere && worstHere !== "ok" ? (
                <span className={"mc-org-worst is-" + worstHere} title={"Worst report status beneath: " + worstHere}>
                  {worstHere}
                </span>
              ) : (
                <span aria-hidden="true" />
              )}
            </li>
          );
        })}
      </ul>
    </>
  );
}
