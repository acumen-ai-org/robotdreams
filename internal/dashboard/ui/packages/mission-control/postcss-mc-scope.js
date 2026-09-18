const SCOPE = ".mc-scope";

function splitTop(sel) {
  const out = [];
  let depth = 0;
  let cur = "";
  for (const ch of sel) {
    if (ch === "(" || ch === "[") depth++;
    else if (ch === ")" || ch === "]") depth--;
    if (ch === "," && depth === 0) {
      out.push(cur);
      cur = "";
    } else cur += ch;
  }
  out.push(cur);
  return out.map((s) => s.trim()).filter(Boolean);
}

function scopeSelector(sel) {
  if (/\.mc-scope(?![\w-])/.test(sel)) return sel;
  if (/^(:root|html|body)$/.test(sel)) return SCOPE;
  const m = sel.match(/^(:root|html|body)\b\s*(.*)$/);
  if (m) return (SCOPE + " " + m[2]).trim();
  const t = sel.match(/^\[data-theme="(dark|light)"\]\s*(.*)$/);
  if (t) {
    const rest = t[2];
    return [
      `[data-theme="${t[1]}"] ${SCOPE} ${rest}`,
      `${SCOPE}[data-theme="${t[1]}"] ${rest}`,
      `${SCOPE} [data-theme="${t[1]}"] ${rest}`,
    ].join(", ");
  }
  return SCOPE + " " + sel;
}

export default function mcScope(options = {}) {
  const skip = options.skip ?? (() => false);
  return {
    postcssPlugin: "mc-scope",
    Once(root) {
      const file = root.source?.input?.file || "";
      if (skip(file)) return;

      const renamed = new Set();
      root.walkAtRules(/^(-\w+-)?keyframes$/, (at) => {
        if (at.params.startsWith("mc-")) return;
        renamed.add(at.params);
        at.params = "mc-" + at.params;
      });
      if (renamed.size) {
        root.walkDecls(/^(-\w+-)?animation(-name)?$/, (d) => {
          d.value = d.value.replace(/[\w-]+/g, (t) => (renamed.has(t) ? "mc-" + t : t));
        });
      }

      root.walkRules((rule) => {
        if (rule.parent?.type === "atrule" && /keyframes$/.test(rule.parent.name)) return;
        rule.selectors = splitTop(rule.selector).flatMap((s) => splitTop(scopeSelector(s)));
      });
    },
  };
}
mcScope.postcss = true;
