import { createPortal } from "react-dom";
import type { ReactNode } from "react";
import { usePageControls } from "../state/PageControlsContext";

interface Props {
  brand?: ReactNode;
  brandLabel?: string;
  title?: string;
  actions?: ReactNode;
}

export function Topbar({ brand, brandLabel, title, actions }: Props) {
  const controls = usePageControls();

  return (
    <header className="mc-topbar">
      <div className="mc-topbar-bar">
        {(brand ?? title) != null && (
          <button
            className={"mc-brand" + (controls.openPanel === "experiments" ? " is-open" : "")}
            type="button"
            onClick={() => controls.togglePanel("experiments")}
            data-mc-panel-trigger=""
            aria-expanded={controls.openPanel === "experiments"}
            aria-label={brandLabel ?? (title ?? "Brand") + " — display options"}
            title="Display options"
          >
            {brand ?? (
              <span className="mc-brand-text">
                <span className="mc-brand-line1">{title}</span>
              </span>
            )}
          </button>
        )}

        <div className="mc-topbar-main">
          <div className="mc-page-toolbox" ref={controls.registerSlot} />
        </div>

        <div className="mc-topbar-actions">
          <span className="mc-topbar-action-slot" ref={controls.registerActionsSlot} />
          {actions != null && <div className="mc-topbar-actions-stack">{actions}</div>}
        </div>
      </div>

      {controls.openPanel && (
        <div className="mc-topbar-expander">
          <div className="mc-expander-inner" ref={controls.registerExpandedSlot} />
        </div>
      )}

      {controls.openPanel === "experiments" &&
        controls.expandedSlot &&
        createPortal(
          <div className="mc-expander-block">
            <div className="mc-expander-head">
              <span className="mc-expander-title">Display options</span>
              <button
                className="mc-button mc-button-tertiary"
                type="button"
                onClick={() => controls.setOpenPanel(null)}
              >
                ✕ close
              </button>
            </div>
            <p className="mc-expander-note mc-body-muted">
              Where a perspective is served by more than one implementation, the choice sits beside the perspective
              switcher in the page toolbox — captioned with the perspective it belongs to. That comparison is temporary;
              the perspective itself (glance, delta, storyboard, spatial) is contract vocabulary and stays.
            </p>
          </div>,
          controls.expandedSlot,
        )}
    </header>
  );
}
