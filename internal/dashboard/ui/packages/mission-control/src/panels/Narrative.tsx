import { useEffect, useRef } from "react";
import { marked, type Token, type Tokens } from "marked";
import hljs from "highlight.js/lib/core";
import json from "highlight.js/lib/languages/json";
import yaml from "highlight.js/lib/languages/yaml";
import bash from "highlight.js/lib/languages/bash";
import "highlight.js/styles/github.css";
import type { Panel } from "../lib/types";

hljs.registerLanguage("json", json);
hljs.registerLanguage("yaml", yaml);
hljs.registerLanguage("bash", bash);

const isHeading = (t: Token): t is Tokens.Heading => t.type === "heading";
const isCode = (t: Token): t is Tokens.Code => t.type === "code";
const isList = (t: Token): t is Tokens.List => t.type === "list";

function markdownSource(panel: Panel): string {
  const p = panel as Record<string, unknown>;
  if (typeof p.markdown === "string") return p.markdown;
  if (typeof p.text === "string") return p.text;
  const events = panel.events || [];
  if (events.length) return events.map((e) => "- " + (e.label || e.type || "event")).join("\n");
  return "";
}

export default function Narrative({ panel }: { panel: Panel }) {
  const ref = useRef<HTMLDivElement>(null);
  const src = markdownSource(panel);

  useEffect(() => {
    const root = ref.current;
    if (!root) return;
    root.innerHTML = "";
    const tokens = marked.lexer(src);
    for (const tok of tokens) {
      if (isHeading(tok)) {
        const h = document.createElement("h" + Math.min(6, tok.depth + 2));
        h.textContent = tok.text;
        root.appendChild(h);
      } else if (isCode(tok)) {
        const pre = document.createElement("pre");
        const code = document.createElement("code");
        code.textContent = tok.text;
        if (tok.lang && hljs.getLanguage(tok.lang)) {
          code.innerHTML = hljs.highlight(tok.text, { language: tok.lang }).value;
        }
        pre.appendChild(code);
        root.appendChild(pre);
      } else if (isList(tok)) {
        const list = document.createElement(tok.ordered ? "ol" : "ul");
        for (const item of tok.items) {
          const li = document.createElement("li");
          li.textContent = item.text;
          list.appendChild(li);
        }
        root.appendChild(list);
      } else if (tok.type === "paragraph" || tok.type === "text") {
        const para = document.createElement("p");
        para.textContent = tok.raw.trim();
        root.appendChild(para);
      }
    }
  }, [src]);

  if (!src) return <p className="mc-empty-state">No narrative.</p>;
  return <div className="mc-narrative" ref={ref} />;
}
