import { Fragment, useMemo } from "react";
import { statusDotClass } from "../../components/shared";
import { useRoute } from "../../state/RouterContext";
import { windowLabel } from "../../lib/window";
import { MAX_PLACE_DEPTH, placeRows, rowInSelection, type Row } from "../../lib/orgRows";
import { type WorkerIndex } from "../../hooks/useWorkers";
import { useVocabulary } from "../../state/VocabularyContext";
import { useNodeListMode } from "../../lib/display";
import { NodeBadge } from "../../lib/vocabulary";
import { ScopeChip, type ScopeSeg } from "../../components/ScopeSelector";
import { RenderingToggle } from "../../components/RenderingToggle";
import { GridIcon, NotepadIcon, PanelRightIcon, TableIcon } from "../../components/icons";
import { DelayedLoading } from "../../components/Async";
import { NodeDetail } from "./NodeDetail";
import type { RolloutNode } from "../../lib/updates";
import type { ScopeEntry } from "../../lib/types";

const PEEK = 6;

interface Props {
  index: WorkerIndex;
  scopes: ScopeEntry[];
  focus: string;
  onFocus: (id: string) => void;
  selected: string | null;
  onSelect: (id: string | null) => void;
  updateOf?: Map<string, RolloutNode>;
}

function segOf(row: Row): ScopeSeg {
  return {
    name: row.label,
    path: row.path || "",
    depth: row.level + 1,
    index: row.index,
    siblings: row.siblings,
    realm: row.realm,
  };
}

export function ExploreView({ index, scopes, focus, onFocus, selected, onSelect, updateOf }: Props) {
  const vocab = useVocabulary();
  const win = useRoute().win;
  const [nodeList, setNodeList] = useNodeListMode();
  const rows = useMemo(() => placeRows(index, scopes), [index, scopes]);

  const universe = useMemo(() => {
    for (const r of rows.byId.values()) if (r.path) return r.path.split("/")[0];
    return "";
  }, [rows]);

  const trail = useMemo(() => {
    const out: Row[] = [];
    let id = focus;
    while (id) {
      const row = rows.byId.get(id);
      if (!row) break;
      out.unshift(row);
      id = rows.parentOf.get(id) ?? "";
    }
    return out;
  }, [focus, rows]);

  const focusRow = focus ? rows.byId.get(focus) : undefined;
  const children = rows.childrenOf.get(focus) || [];
  const places = children.filter((c) => c.kind === "place");
  const nodes = children.filter((c) => c.kind === "node");
  const placesOf = (id: string) => (rows.childrenOf.get(id) || []).filter((c) => c.kind === "place");

  if (index.error) return <p className="mc-form-error">{index.error}</p>;
  if (index.pending) return <DelayedLoading />;

  if (focus && !focusRow) {
    return (
      <section className="mc-panel mc-panel-fill mc-explore">
        <p className="mc-empty-state">
          That is no longer here.{" "}
          <button className="mc-link-button" type="button" onClick={() => onFocus("")}>
            Back to {universe || "the start"}
          </button>
        </p>
      </section>
    );
  }

  const crumbs = (
    <nav className="mc-explore-crumbs" aria-label="Where you are">
      <button
        className={"mc-explore-crumb" + (focus ? "" : " is-here")}
        type="button"
        onClick={() => onFocus("")}
        aria-current={focus ? undefined : "page"}
      >
        <ScopeChip
          seg={{ name: universe || vocab.one(0), path: universe, depth: 1, index: 1, siblings: 1 }}
          childCount={places.length || undefined}
        />
      </button>
      {trail.map((r, i) => (
        <Fragment key={r.id}>
          <span className="mc-explore-sep" aria-hidden="true">
            ›
          </span>
          <button
            className={"mc-explore-crumb" + (i === trail.length - 1 ? " is-here" : "")}
            type="button"
            onClick={() => onFocus(r.id)}
            aria-current={i === trail.length - 1 ? "page" : undefined}
          >
            {r.kind === "node" ? (
              <NodeBadge n={r.index} max={r.siblings} role={r.worker?.role} name={r.label} realm={r.realm} />
            ) : (
              <ScopeChip
                seg={segOf(r)}
                childCount={placesOf(r.id).length || undefined}
                nodeCount={rows.roll.get(r.id)?.total}
              />
            )}
          </button>
        </Fragment>
      ))}
    </nav>
  );

  if (focusRow?.kind === "node" && focusRow.worker) {
    return (
      <section className="mc-panel mc-panel-fill mc-explore" aria-label="Explore">
        {crumbs}
        <div className="mc-explore-body mc-explore-one">
          <NodeDetail
            worker={focusRow.worker}
            index={index}
            update={updateOf?.get(focusRow.worker.id)}
            onSelect={(id) => onFocus(id && rows.byId.has(id) ? id : (rows.parentOf.get(focus) ?? ""))}
          />
        </div>
      </section>
    );
  }

  const nodeIcon = (row: Row) => (
    <button
      className="mc-icon-button mc-explore-box-open"
      type="button"
      aria-label={"Open " + row.label + " in this view"}
      title="Open in this view"
      onClick={() => onFocus(row.id)}
    >
      <NotepadIcon size={14} />
    </button>
  );

  return (
    <section className="mc-panel mc-panel-fill mc-explore" aria-label="Explore">
      {crumbs}
      <div className="mc-explore-body">
        {!places.length && !nodes.length && (
          <p className="mc-empty-state">Nothing is inside this {vocab.lower(focusRow?.level ?? 0)} yet.</p>
        )}

        {places.length > 0 && (
          <>
            <div className="mc-explore-section-head">
              <h3 className="mc-explore-heading">
                {places.length}{" "}
                {places.length === 1
                  ? vocab.lower((focusRow?.level ?? 0) + 1)
                  : vocab.many((focusRow?.level ?? 0) + 1).toLowerCase()}
              </h3>
            </div>
            <div className="mc-explore-grid">
              {places.map((place) => {
                const kids = placesOf(place.id);
                const shown = kids.slice(0, PEEK);
                const more = kids.length - shown.length;
                const roll = rows.roll.get(place.id) || { active: 0, total: 0 };
                const own = (rows.childrenOf.get(place.id) || []).filter((c) => c.kind === "node").length;
                const reports = rows.reports.get(place.id);
                const outside = !rowInSelection(index, place);
                return (
                  <div
                    key={place.id}
                    className={
                      "mc-explore-box" +
                      (place.realm != null ? " mc-realm-" + place.realm : "") +
                      (outside ? " is-outside" : "") +
                      (selected === place.id ? " is-selected" : "")
                    }
                  >
                    <div className="mc-explore-box-head">
                      <button className="mc-explore-box-name" type="button" onClick={() => onFocus(place.id)}>
                        <ScopeChip seg={segOf(place)} childCount={kids.length || undefined} nodeCount={roll.total} />
                      </button>
                      <button
                        className="mc-icon-button mc-explore-box-open"
                        type="button"
                        aria-label={"Show " + place.label + " in the side panel"}
                        title="Show in the side panel"
                        onClick={() => onSelect(selected === place.id ? null : place.id)}
                      >
                        <PanelRightIcon size={14} />
                      </button>
                    </div>

                    <button className="mc-explore-box-body" type="button" onClick={() => onFocus(place.id)}>
                      {shown.length > 0 ? (
                        <span className="mc-explore-kids">
                          {shown.map((k) => (
                            <ScopeChip key={k.id} seg={segOf(k)} nodeCount={rows.roll.get(k.id)?.total} />
                          ))}
                          {more > 0 && <span className="mc-explore-more">… {more} more …</span>}
                        </span>
                      ) : place.level < MAX_PLACE_DEPTH ? (
                        <span className="mc-explore-kids-none">no {vocab.many(place.level + 1).toLowerCase()}</span>
                      ) : null}
                    </button>

                    <div className="mc-explore-box-foot mc-label-muted">
                      <span className="mc-mono">
                        {roll.active}/{roll.total}
                      </span>{" "}
                      {vocab.many(4).toLowerCase()}
                      {own > 0 && <span className="mc-explore-box-own">{own} here</span>}
                      {reports && (
                        <span
                          className="mc-explore-box-reports"
                          title={
                            reports.under +
                            " report" +
                            (reports.under === 1 ? "" : "s") +
                            " in the " +
                            windowLabel(win) +
                            (reports.here ? " — " + reports.here + " produced at this " + vocab.lower(place.level) : "")
                          }
                        >
                          <span className="mc-mono">{reports.under}</span> report{reports.under === 1 ? "" : "s"}
                        </span>
                      )}
                    </div>
                  </div>
                );
              })}
            </div>
          </>
        )}

        {nodes.length > 0 && (
          <>
            <div className="mc-explore-section-head">
              <h3 className="mc-explore-heading">
                {nodes.length} {nodes.length === 1 ? vocab.lower(4) : vocab.many(4).toLowerCase()} working here
              </h3>
              <RenderingToggle
                label={vocab.many(4) + " as"}
                value={nodeList}
                onChange={setNodeList}
                options={[
                  { id: "table", label: "As a table", icon: <TableIcon size={14} /> },
                  { id: "boxes", label: "As boxes", icon: <GridIcon size={14} /> },
                ]}
              />
            </div>

            {nodeList === "boxes" ? (
              <div className="mc-explore-grid">
                {nodes.map((n) => {
                  const w = n.worker!;
                  const outside = !rowInSelection(index, n);
                  return (
                    <div
                      key={n.id}
                      className={
                        "mc-explore-box mc-explore-node-box" +
                        (n.realm != null ? " mc-realm-" + n.realm : "") +
                        (outside ? " is-outside" : "") +
                        (selected === n.id ? " is-selected" : "")
                      }
                    >
                      <div className="mc-explore-box-head">
                        <button
                          className="mc-explore-box-name"
                          type="button"
                          onClick={() => onSelect(selected === n.id ? null : n.id)}
                        >
                          <NodeBadge n={n.index} max={n.siblings} role={w.role} name={n.label} realm={n.realm} />
                        </button>
                        {nodeIcon(n)}
                      </div>
                      <div className="mc-explore-box-foot mc-label-muted">
                        <span
                          className={"mc-status-dot " + statusDotClass(w.status || "disconnected")}
                          aria-hidden="true"
                        />
                        {w.status || "unknown"}
                        {w.role && <span className="mc-explore-box-own">{w.role}</span>}
                      </div>
                    </div>
                  );
                })}
              </div>
            ) : (
              <div className="mc-storage-table-wrap">
                <table className="mc-storage-table mc-explore-nodes">
                  <thead>
                    <tr>
                      <th scope="col">{vocab.one(4)}</th>
                      <th scope="col">Role</th>
                      <th scope="col">Status</th>
                      <th scope="col">
                        <span className="mc-sr-only">Open in this view</span>
                      </th>
                    </tr>
                  </thead>
                  <tbody>
                    {nodes.map((n) => {
                      const w = n.worker!;
                      const outside = !rowInSelection(index, n);
                      return (
                        <tr
                          key={n.id}
                          className={
                            "mc-explore-node" +
                            (outside ? " is-outside" : "") +
                            (selected === n.id ? " is-selected" : "")
                          }
                        >
                          <td>
                            <button
                              className="mc-explore-node-id"
                              type="button"
                              onClick={() => onSelect(selected === n.id ? null : n.id)}
                            >
                              <NodeBadge n={n.index} max={n.siblings} role={w.role} name={n.label} realm={n.realm} />
                            </button>
                          </td>
                          <td className="mc-label-muted">{w.role || ""}</td>
                          <td className="mc-label-muted">
                            <span
                              className={"mc-status-dot " + statusDotClass(w.status || "disconnected")}
                              aria-hidden="true"
                            />{" "}
                            {w.status || "unknown"}
                          </td>
                          <td>{nodeIcon(n)}</td>
                        </tr>
                      );
                    })}
                  </tbody>
                </table>
              </div>
            )}
          </>
        )}
      </div>
    </section>
  );
}

export default ExploreView;
