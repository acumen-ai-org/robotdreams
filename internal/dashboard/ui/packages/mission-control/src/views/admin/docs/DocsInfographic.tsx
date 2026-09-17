import { useState, type ReactNode } from "react";
import { CATEGORIES, adminRoute } from "../../../lib/routes";
import { Link } from "../../../components/Link";
import { UPDATE_PHASES } from "../../../lib/updates";
import { NodeBadge, RealmChip, SiteBadge, UniverseIcon, WorldBadge } from "../../../lib/vocabulary";

type PartId = "control" | "updates" | "node" | "messaging" | "library" | "outcomes" | "hierarchy" | "apps";

interface Part {
  title: string;
  docs: { label: string; section: string };
  body: ReactNode;
}

const PARTS: Record<PartId, Part> = {
  control: {
    title: "The control plane",
    docs: { label: "The spine", section: "docs/spine" },
    body: (
      <>
        <p>
          The fixed operational surface over the two swappable primitives: it registers nodes, issues their identity and
          tokens, carries their inboxes, and keeps the update and app registries. Mission Control — this dashboard — is
          its face. The primitives can be exchanged; this surface is what stays.
        </p>
      </>
    ),
  },
  updates: {
    title: "Updates are announcements",
    docs: { label: "Updates", section: "docs/updates" },
    body: (
      <>
        <p>
          <span className="mc-mono">dream updates announce</span> tells every node a version exists — and that is all it
          does. Each node decides for itself, and reports a phase back:
        </p>
        <div className="mc-info-phases">
          {UPDATE_PHASES.map((p, i) => (
            <span key={p.id} className="mc-info-phase">
              <span className={"mc-org-update is-" + p.tone} title={p.hint}>
                {p.label}
              </span>
              {i < 3 && (
                <span className="mc-info-phase-arrow" aria-hidden="true">
                  →
                </span>
              )}
            </span>
          ))}
        </div>
        <p className="mc-body-muted">
          Silence is counted, never omitted — and declined is a legitimate answer, not a failure.
        </p>
      </>
    ),
  },
  node: {
    title: "A node",
    docs: { label: "Hierarchy & identity", section: "docs/hierarchy" },
    body: (
      <>
        <p>
          Anything that runs <span className="mc-mono">dream worker connect</span> — a person's laptop, a server, an
          agent. A node has an identity, a role (the icon follows it), a place in the reporting chart, and an inbox.
          What runs <em>on</em> the node is deliberately not Robot Dreams' business: the contract is the messages, not
          the runtime.
        </p>
      </>
    ),
  },
  messaging: {
    title: "Messages move the work",
    docs: { label: "The spine", section: "docs/spine" },
    body: (
      <>
        <ul className="mc-info-msgs">
          <li>
            <span className="mc-mono">status_update</span>
            <span className="mc-info-msg-route" aria-hidden="true">
              ↑ one hop up
            </span>
            <span className="mc-body-muted">how it is going — always to the parent</span>
          </li>
          <li>
            <span className="mc-mono">escalation</span>
            <span className="mc-info-msg-route" aria-hidden="true">
              ↑ one hop up
            </span>
            <span className="mc-body-muted">this needs someone above me</span>
          </li>
          <li>
            <span className="mc-mono">completed_work</span>
            <span className="mc-info-msg-route" aria-hidden="true">
              → addressed
            </span>
            <span className="mc-body-muted">here is the result you asked for</span>
          </li>
          <li>
            <span className="mc-mono">request_for_input</span>
            <span className="mc-info-msg-route" aria-hidden="true">
              → addressed
            </span>
            <span className="mc-body-muted">I need something from you to continue</span>
          </li>
        </ul>
        <p>
          The routing <em>is</em> the org chart — which is exactly what the Network rendering draws, with each line
          weighted by the traffic on it right now.
        </p>
      </>
    ),
  },
  library: {
    title: "The library",
    docs: { label: "Reporting model", section: "docs/reporting" },
    body: (
      <>
        <p>
          Reports are books with a standard binding: a <strong>definition</strong> is the type, an{" "}
          <strong>instance</strong> is the data, and any node can deposit one. The standard binding is what makes them
          aggregable — a site's number becomes a realm's, a world's, a universe's.
        </p>
        <div className="mc-info-chips">
          <span className="mc-info-chips-label">The subjects a report can be about</span>
          <span className="mc-ig-chiprow">
            {CATEGORIES.map((c) => (
              <span className="mc-tag" key={c}>
                {c}
              </span>
            ))}
          </span>
        </div>
      </>
    ),
  },
  outcomes: {
    title: "Reading it back: Outcomes",
    docs: { label: "Modalities & views", section: "docs/modalities" },
    body: (
      <>
        <p>
          Any dashboard can read the library back at any scope. A <strong>perspective</strong> — Glance, Delta,
          Timeline, Spatial, Board, Narrative, Ask — is how you choose to read; a <strong>stance</strong> — operational,
          strategic, diagnostic — re-weights a page without hiding anything.
        </p>
      </>
    ),
  },
  hierarchy: {
    title: "A place for everything",
    docs: { label: "Hierarchy & identity", section: "docs/hierarchy" },
    body: (
      <>
        <p>
          Five levels, each with its own mark. A <strong>scope</strong> is a path down the ladder —{" "}
          <span className="mc-mono">acme/revenue/sales</span> — and it is the same filter everywhere: the org chart, the
          reports, the deep links. A realm's colour cascades to everything it contains.
        </p>
      </>
    ),
  },
  apps: {
    title: "Nodes can serve apps",
    docs: { label: "Hierarchy & identity", section: "docs/hierarchy" },
    body: (
      <>
        <p>
          Any node can register one app it serves — a URL and a line of description in the central registry. The tree
          marks it <span className="mc-org-app">↗</span>, and selecting the node offers Open and an embedded Preview.
          The demo fleet's Ping Pong pair is two of these, passing messages through the control plane and counting what
          they receive.
        </p>
      </>
    ),
  },
};

export default function DocsInfographic() {
  const [sel, setSel] = useState<PartId | null>(null);
  const pick = (id: PartId) => setSel((cur) => (cur === id ? null : id));
  const cls = (id: PartId, base: string) => base + " mc-ig-region" + (sel === id ? " is-sel" : "");
  const part = sel ? PARTS[sel] : null;

  return (
    <div className="mc-ig">
      <p className="mc-ig-hint mc-body-muted">
        One picture, the whole system. Click any part of it to expand what it is.
      </p>

      <div className="mc-ig-map" role="group" aria-label="The system, as one picture">
        <button
          type="button"
          className={cls("control", "mc-ig-control")}
          aria-pressed={sel === "control"}
          onClick={() => pick("control")}
        >
          <span className="mc-ig-region-name">Control plane</span>
          <span className="mc-ig-chiprow" aria-hidden="true">
            <span className="mc-tag">registry</span>
            <span className="mc-tag">identity</span>
            <span className="mc-tag">inboxes</span>
            <span className="mc-tag">mission control</span>
          </span>
        </button>

        <div className="mc-ig-flows" aria-hidden={false}>
          <button
            type="button"
            className={cls("updates", "mc-ig-flow")}
            aria-pressed={sel === "updates"}
            onClick={() => pick("updates")}
          >
            <span className="mc-ig-flow-arrow" aria-hidden="true">
              ⇣
            </span>{" "}
            announces updates
          </button>
          <span className="mc-ig-flow mc-ig-flow-quiet">
            <span className="mc-ig-flow-arrow" aria-hidden="true">
              ⇣
            </span>{" "}
            watches
          </span>
          <button
            type="button"
            className={cls("outcomes", "mc-ig-flow")}
            aria-pressed={sel === "outcomes"}
            onClick={() => pick("outcomes")}
          >
            <span className="mc-ig-flow-arrow" aria-hidden="true">
              ⇡
            </span>{" "}
            reads Outcomes
          </button>
        </div>

        <div className="mc-ig-floor">
          <button
            type="button"
            className={cls("node", "mc-ig-node")}
            aria-pressed={sel === "node"}
            onClick={() => pick("node")}
          >
            <span className="mc-ig-node-emoji" aria-hidden="true">
              🔨
            </span>
            <span className="mc-ig-region-name">node</span>
          </button>
          <button
            type="button"
            className={cls("messaging", "mc-ig-rail")}
            aria-pressed={sel === "messaging"}
            onClick={() => pick("messaging")}
          >
            <span className="mc-ig-rail-arrow" aria-hidden="true">
              ⇄
            </span>
            <span className="mc-ig-region-name">messaging</span>
            <span className="mc-ig-rail-arrow" aria-hidden="true">
              ⇄
            </span>
          </button>
          <button
            type="button"
            className={cls("node", "mc-ig-node")}
            aria-pressed={sel === "node"}
            onClick={() => pick("node")}
          >
            <span className="mc-ig-node-emoji" aria-hidden="true">
              🔬
            </span>
            <span className="mc-ig-region-name">node</span>
          </button>
          <span className="mc-ig-deposit" aria-hidden="true">
            deposits reports ⇢
          </span>
          <button
            type="button"
            className={cls("library", "mc-ig-library")}
            aria-pressed={sel === "library"}
            onClick={() => pick("library")}
          >
            <span className="mc-ig-node-emoji" aria-hidden="true">
              📚
            </span>
            <span className="mc-ig-region-name">library</span>
          </button>
        </div>

        <div className="mc-ig-ground">
          <button
            type="button"
            className={cls("hierarchy", "mc-ig-band")}
            aria-pressed={sel === "hierarchy"}
            onClick={() => pick("hierarchy")}
          >
            <span className="mc-ig-region-name">The universe</span>
            <span className="mc-ig-ladder" aria-hidden="true">
              <UniverseIcon size={14} />
              <span className="mc-ig-ladder-sep">›</span>
              <WorldBadge n={1} max={4} />
              <span className="mc-ig-ladder-sep">›</span>
              <RealmChip n={0} />
              <span className="mc-ig-ladder-sep">›</span>
              <SiteBadge n={1} max={9} realm={0} />
              <span className="mc-ig-ladder-sep">›</span>
              <NodeBadge n={1} max={9} role="builder" realm={0} />
            </span>
          </button>
          <button
            type="button"
            className={cls("apps", "mc-ig-band mc-ig-band-apps")}
            aria-pressed={sel === "apps"}
            onClick={() => pick("apps")}
          >
            <span className="mc-org-app" aria-hidden="true">
              ↗
            </span>
            <span className="mc-ig-region-name">apps</span>
          </button>
        </div>
      </div>

      {part ? (
        <div className="mc-ig-detail" key={sel}>
          <div className="mc-ig-detail-head">
            <h2>{part.title}</h2>
            <button className="mc-button mc-button-tertiary" type="button" onClick={() => setSel(null)}>
              ✕ close
            </button>
          </div>
          {part.body}
          <p className="mc-ig-detail-more">
            <Link to={adminRoute(part.docs.section)}>Read the full page — {part.docs.label} →</Link>
          </p>
        </div>
      ) : (
        <p className="mc-ig-empty mc-body-muted">Nothing selected — the picture is the summary.</p>
      )}
    </div>
  );
}
