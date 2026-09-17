import { useEffect, useRef, useState } from "react";
import mermaid from "mermaid";
import type { Panel, PlanItem } from "../lib/types";
import { resolveTheme } from "../lib/themeRead";
import { useThemeRoot } from "../state/ThemeContext";

let initialized = false;
let renderSeq = 0;

function graphSource(panel: Panel): string {
  const g = panel.graph as Record<string, unknown> | string | undefined;
  if (typeof g === "string") return g;
  if (g && typeof g === "object" && typeof g.mermaid === "string") return g.mermaid;
  if (panel.plan?.items?.length) return itemsToMermaid(panel.plan.items, panel.plan.states?.at(-1)?.state);
  return "";
}

function nodeID(it: PlanItem): string {
  return ("n_" + (it.scope || "") + "_" + it.id).replace(/[^A-Za-z0-9_]/g, "_");
}

function label(text: string): string {
  return text.replace(/["[\]{}|]/g, " ").slice(0, 60);
}

function itemsToMermaid(items: PlanItem[], terminal?: string): string {
  const byId = new Map<string, PlanItem>();
  for (const it of items) byId.set((it.scope || "") + "/" + it.id, it);

  const lines = ["graph LR"];
  for (const it of items) {
    lines.push("  " + nodeID(it) + '["' + label(it.title || it.id) + '"]');
    if ((it.blocked_by || []).length) lines.push("  class " + nodeID(it) + " blockedNode;");
    else if (terminal && it.state === terminal) lines.push("  class " + nodeID(it) + " doneNode;");
  }
  for (const it of items) {
    for (const dep of it.blocked_by || []) {
      const from = byId.get((it.scope || "") + "/" + dep);
      if (!from) continue;
      lines.push("  " + nodeID(from) + " --> " + nodeID(it));
    }
  }
  lines.push("  classDef blockedNode stroke-width:2px;");
  lines.push("  classDef doneNode opacity:0.55;");
  return lines.join("\n");
}

export default function MermaidDiagram({ panel }: { panel: Panel }) {
  const ref = useRef<HTMLDivElement>(null);
  const root = useThemeRoot();
  const [error, setError] = useState("");
  const src = graphSource(panel);

  useEffect(() => {
    if (!src || !ref.current) return;
    if (!initialized) {
      mermaid.initialize({
        startOnLoad: false,
        securityLevel: "strict",
        theme: resolveTheme(root) === "dark" ? "dark" : "neutral",
      });
      initialized = true;
    }
    const target = ref.current;
    mermaid
      .render("mermaid-panel-" + ++renderSeq, src)
      .then(({ svg }) => {
        target.innerHTML = svg;
        setError("");
      })
      .catch((e) => setError(String(e instanceof Error ? e.message : e)));
  }, [src, root]);

  if (!src) return <p className="mc-empty-state">No diagram source.</p>;
  if (error) return <p className="mc-empty-state">Could not render diagram: {error}</p>;
  return <div className="mc-mermaid-wrap" ref={ref} />;
}
