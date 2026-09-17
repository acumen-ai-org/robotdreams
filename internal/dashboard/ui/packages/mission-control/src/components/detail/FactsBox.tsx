import type { ReactNode } from "react";

export function FactsBox({
  caption,
  icon,
  figure,
  of,
  lines,
  tone,
}: {
  caption: string;
  icon?: ReactNode;
  figure: ReactNode;
  of?: ReactNode;
  lines?: ReactNode[];
  tone?: "attention" | "ok";
}) {
  return (
    <div className={"mc-facts-box" + (tone === "attention" ? " is-attention" : "")}>
      <div className="mc-facts-head">
        {icon && <span className="mc-facts-icon">{icon}</span>}
        <span className="mc-facts-caption">{caption}</span>
      </div>
      <div
        className={"mc-facts-figure" + (typeof figure === "string" ? " is-word" : "") + (tone === "ok" ? " is-ok" : "")}
      >
        {figure}
        {of != null && <span className="mc-facts-of">/{of}</span>}
      </div>
      {lines && lines.length > 0 && (
        <ul className="mc-facts-lines">
          {lines.map((l, i) => (
            <li key={i} className="mc-facts-line">
              {l}
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
