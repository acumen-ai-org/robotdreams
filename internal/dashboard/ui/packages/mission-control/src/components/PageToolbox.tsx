import { useCallback, useState, type ReactNode } from "react";
import { createPortal } from "react-dom";
import { usePageControls } from "../state/PageControlsContext";
import { LegendIcon } from "./icons";
import { WindowControl } from "./WindowControl";
import { readStored, writeStored } from "../lib/storage";

const LS_INFO = "showInfo";

interface Props {
  active: boolean;
  title: string;
  subtitle?: string;
  info?: string;
  legend?: ReactNode;
  children?: ReactNode;
}

export function PageToolbox({ active, title, subtitle, info, legend, children }: Props) {
  const controls = usePageControls();
  const [showInfo, setShowInfo] = useState<boolean>(() => readStored(LS_INFO) === "1");
  const toggleInfo = useCallback(() => {
    setShowInfo((cur) => {
      const next = !cur;
      writeStored(LS_INFO, next ? "1" : "0");
      return next;
    });
  }, []);

  if (!active) return null;
  return (
    <>
      {controls.slot &&
        createPortal(
          <div
            className="mc-toolbox"
            role="group"
            aria-label={title + " controls" + (subtitle ? " — " + subtitle : "")}
          >
            <span className="mc-toolbox-tools" ref={controls.registerToolsSlot}>
              {children}
            </span>
            <span className="mc-toolbox-filters-slot" ref={controls.registerFiltersSlot} />
            {info && showInfo && (
              <span className="mc-toolbox-info">
                <span className="mc-toolbox-info-text">{info}</span>
              </span>
            )}
            <WindowControl />
            <span className="mc-toolbox-rail" ref={controls.registerRailSlot} />
            {(info || legend) && (
              <div className="mc-toolbox-aside">
                {info ? (
                  <button
                    type="button"
                    className={"mc-toolbox-info-toggle" + (showInfo ? " is-on" : "")}
                    aria-pressed={showInfo}
                    aria-label={showInfo ? "Hide what this shows" : "What am I looking at?"}
                    title={showInfo ? "Hide what this shows" : info}
                    onClick={toggleInfo}
                  >
                    i
                  </button>
                ) : (
                  <span />
                )}
                {legend && (
                  <button
                    type="button"
                    className={"mc-toolbox-info-toggle" + (controls.openPanel === "legend" ? " is-on" : "")}
                    aria-expanded={controls.openPanel === "legend"}
                    aria-label="Legend — what the marks mean"
                    title="Legend — what the marks mean"
                    onClick={() => controls.togglePanel("legend")}
                    data-mc-panel-trigger=""
                  >
                    <LegendIcon size={14} />
                  </button>
                )}
              </div>
            )}
          </div>,
          controls.slot,
        )}
      {legend &&
        controls.openPanel === "legend" &&
        controls.expandedSlot &&
        createPortal(legend, controls.expandedSlot)}
    </>
  );
}
