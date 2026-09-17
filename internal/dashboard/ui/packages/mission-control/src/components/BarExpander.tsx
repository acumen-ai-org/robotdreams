import { useCallback, useLayoutEffect, useRef, useState, type ReactNode, type RefObject } from "react";
import { createPortal } from "react-dom";
import { usePageControls } from "../state/PageControlsContext";

interface Props {
  open: boolean;
  triggerRef: RefObject<HTMLButtonElement | null>;
  controlCopy: ReactNode;
  actions: ReactNode;
  left: ReactNode;
  right: ReactNode;
}

export function BarExpander({ open, triggerRef, controlCopy, actions, left, right }: Props) {
  const controls = usePageControls();

  const chromeTop = (el: HTMLElement): number => {
    const frame = el.closest(".mc-topbar") ?? el.closest(".mc-app") ?? el.closest(".mc-scope");
    return (frame ?? document.documentElement).getBoundingClientRect().top;
  };

  const rootRef = useRef<HTMLDivElement | null>(null);
  const [pull, setPull] = useState<number | null>(null);
  const measurePull = useCallback((el: HTMLDivElement | null) => {
    rootRef.current = el;
    if (el) setPull((cur) => (cur === null ? Math.round(el.getBoundingClientRect().top - chromeTop(el)) : cur));
  }, []);

  const [anchor, setAnchor] = useState<{ left: number; top: number; width: number } | null>(null);
  useLayoutEffect(() => {
    if (!open) {
      setAnchor(null);
      setPull(null);
      return;
    }
    if (pull == null) return;
    const measure = () => {
      const root = rootRef.current;
      const wrap = triggerRef.current?.closest(".mc-view-select");
      if (!root || !wrap) return;
      const rr = root.getBoundingClientRect();
      const r = wrap.getBoundingClientRect();
      setAnchor({ left: Math.round(r.left - rr.left), top: Math.round(r.top - rr.top), width: Math.round(r.width) });
    };
    measure();
    window.addEventListener("resize", measure);
    return () => window.removeEventListener("resize", measure);
  }, [open, pull, triggerRef]);

  const actionsRef = useRef<HTMLDivElement>(null);
  const anchorElRef = useRef<HTMLDivElement>(null);
  const [actionsW, setActionsW] = useState(0);
  const [copyW, setCopyW] = useState(0);
  // eslint-disable-next-line react-hooks/exhaustive-deps
  useLayoutEffect(() => {
    if (!open) return;
    const aw = actionsRef.current?.offsetWidth || 0;
    setActionsW((cur) => (cur === aw ? cur : aw));
    const cw = anchorElRef.current?.offsetWidth || 0;
    setCopyW((cur) => (cur === cw ? cur : cw));
  });

  if (!open || !controls.expandedSlot) return null;

  return createPortal(
    <div
      ref={measurePull}
      className="mc-bar-expander"
      style={pull != null ? { marginTop: -pull } : { visibility: "hidden" }}
    >
      {anchor && (
        <div ref={anchorElRef} className="mc-bar-expander-anchor" style={{ left: anchor.left, top: anchor.top }}>
          <div className="mc-bar-expander-actions" ref={actionsRef}>
            {actions}
          </div>
          {controlCopy}
        </div>
      )}
      <div className="mc-bar-expander-main" style={anchor ? { width: Math.max(220, anchor.left - 24) } : undefined}>
        {left}
      </div>
      <div
        className="mc-bar-expander-gap"
        style={anchor ? { width: (copyW || anchor.width) + (actionsW ? actionsW + 48 : 100) } : undefined}
        aria-hidden="true"
      />
      <div className="mc-bar-expander-side">{right}</div>
    </div>,
    controls.expandedSlot,
  );
}
