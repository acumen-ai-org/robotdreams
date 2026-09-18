import { useMemo, useState } from "react";
import { Link } from "../../components/Link";
import { reportingRoute, LEVEL_NAMES, type Route } from "../../lib/routes";
import { rollupOf, rollupTitle } from "../../lib/aggregation";
import type { SummaryTile } from "../../lib/types";
import { LayersIcon, NotepadIcon } from "../../components/icons";

interface Node {
  path: string;
  name: string;
  leaf: boolean;
  children: Node[];
}

function buildTree(paths: string[], from: string): Node[] {
  const roots: Node[] = [];
  const byPath = new Map<string, Node>();
  const fromSegs = from ? from.split("/").filter(Boolean) : [];

  const ensure = (segs: string[]): Node => {
    const path = segs.join("/");
    let n = byPath.get(path);
    if (n) return n;
    n = { path, name: segs[segs.length - 1] || path, leaf: false, children: [] };
    byPath.set(path, n);
    if (segs.length > fromSegs.length + 1) ensure(segs.slice(0, -1)).children.push(n);
    else roots.push(n);
    return n;
  };

  for (const p of paths) {
    const segs = p.split("/").filter(Boolean);
    if (segs.length <= fromSegs.length) continue;
    ensure(segs).leaf = true;
  }

  const sort = (ns: Node[]) => {
    ns.sort((a, b) => a.name.localeCompare(b.name));
    for (const n of ns) sort(n.children);
  };
  sort(roots);
  return roots;
}

function Branch({ node, route, def, depth }: { node: Node; route: Route; def: string; depth: number }) {
  const level = LEVEL_NAMES[node.path.split("/").filter(Boolean).length - 1] || "";
  const to = reportingRoute(route, { scope: node.path, report: def });
  return (
    <li className={"mc-source-row" + (node.leaf ? " is-leaf" : "")} style={{ ["--source-depth" as string]: depth }}>
      <Link
        className="mc-source-link"
        to={to}
        title={
          node.leaf
            ? "Open the report as it was written at " + node.path + " — nothing rolled up"
            : "Open this report rolled up to " + node.path
        }
      >
        {node.leaf ? <NotepadIcon size={12} /> : <LayersIcon size={12} />}
        <span className="mc-mono">{node.name}</span>
        {level && <span className="mc-source-level">{level}</span>}
        <span className="mc-source-kind mc-label-muted">{node.leaf ? "written here" : "aggregated"}</span>
      </Link>
      {node.children.length > 0 && (
        <ul className="mc-source-kids">
          {node.children.map((c) => (
            <Branch key={c.path} node={c} route={route} def={def} depth={depth + 1} />
          ))}
        </ul>
      )}
    </li>
  );
}

export function ReportSources({ summary, route, def }: { summary: SummaryTile; route: Route; def: string }) {
  const scope = summary.scope || route.scope;
  const roll = rollupOf(summary, scope);
  const [open, setOpen] = useState(false);
  const tree = useMemo(() => (roll ? buildTree(roll.paths, scope) : []), [roll, scope]);

  if (!roll) return null;

  const headline =
    roll.levels > 0
      ? "Aggregated from " + roll.leaves + (roll.leaves === 1 ? " report" : " reports")
      : "Written here — " + roll.leaves + (roll.leaves === 1 ? " report" : " reports") + ", nothing rolled up";

  return (
    <div className={"mc-report-sources" + (open ? " is-open" : "")}>
      <button
        className="mc-report-sources-toggle"
        type="button"
        aria-expanded={open}
        title={rollupTitle(roll)}
        onClick={() => setOpen((v) => !v)}
        disabled={!tree.length}
      >
        <LayersIcon size={12} />
        <span className="mc-mono">{roll.leaves}</span>
        <span>{headline}</span>
        {roll.levels > 0 && (
          <span className="mc-label-muted">
            through {roll.levels} {roll.levels === 1 ? "level" : "levels"}
          </span>
        )}
        {tree.length > 0 && (
          <span className="mc-rc-caret" aria-hidden="true">
            {open ? "▴" : "▾"}
          </span>
        )}
      </button>

      {open && tree.length > 0 && (
        <div className="mc-report-sources-body">
          <p className="mc-body-muted">
            Each rung below is the same report rolled up to that scope; the ones marked{" "}
            <span className="mc-source-kind">written here</span> are the end reports this page is computed from, shown
            exactly as their producer wrote them.
          </p>
          <ul className="mc-source-tree">
            {tree.map((n) => (
              <Branch key={n.path} node={n} route={route} def={def} depth={0} />
            ))}
          </ul>
          {roll.direct > 0 && (
            <p className="mc-body-muted">
              {roll.direct} {roll.direct === 1 ? "report was" : "reports were"} written at this scope itself.
            </p>
          )}
        </div>
      )}
    </div>
  );
}
