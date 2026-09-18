import { useEffect, useRef } from "react";
import { createPortal } from "react-dom";
import { usePageControls } from "../state/PageControlsContext";
import { SearchIcon } from "./icons";

interface Props {
  active: boolean;
  value: string;
  onChange: (v: string) => void;
  placeholder: string;
  label: string;
  resultCount?: number;
}

export function SearchControl({ active, value, onChange, placeholder, label, resultCount }: Props) {
  const controls = usePageControls();
  const open = controls.openPanel === "search";
  const inputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    if (open) inputRef.current?.focus();
  }, [open]);

  if (!active) return null;

  return (
    <>
      {controls.actionsSlot &&
        createPortal(
          <button
            className={"mc-icon-button mc-search-trigger" + (open || value ? " is-on" : "")}
            type="button"
            onClick={() => controls.togglePanel("search")}
            data-mc-panel-trigger=""
            aria-expanded={open}
            aria-label={label}
            title={label}
          >
            <SearchIcon size={16} />
            {value && <span className="mc-search-dot" aria-hidden="true"></span>}
          </button>,
          controls.actionsSlot,
        )}

      {open &&
        controls.expandedSlot &&
        createPortal(
          <div className="mc-expander-block">
            <div className="mc-expander-head">
              <span className="mc-expander-title">{label}</span>
              <button
                className="mc-button mc-button-tertiary"
                type="button"
                onClick={() => controls.setOpenPanel(null)}
              >
                ✕ close
              </button>
            </div>
            <span className="mc-search-control">
              <SearchIcon size={15} className="mc-search-control-icon" />
              <label className="mc-sr-only" htmlFor="search-input">
                {label}
              </label>
              <input
                id="search-input"
                ref={inputRef}
                type="search"
                placeholder={placeholder}
                autoComplete="off"
                value={value}
                onChange={(e) => onChange(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === "Enter") controls.setOpenPanel(null);
                }}
              />
              {value && (
                <button
                  className="mc-search-clear"
                  type="button"
                  aria-label="Clear search"
                  onClick={() => onChange("")}
                >
                  ✕
                </button>
              )}
            </span>
            {value && (
              <p className="mc-expander-note mc-body-muted">
                {resultCount === 0
                  ? "No matches."
                  : resultCount === 1
                    ? "1 match — its ancestors are expanded."
                    : `${resultCount} matches — their ancestors are expanded.`}
              </p>
            )}
          </div>,
          controls.expandedSlot,
        )}
    </>
  );
}
