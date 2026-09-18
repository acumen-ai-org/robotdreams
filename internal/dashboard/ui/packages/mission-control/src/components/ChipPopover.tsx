import {
  useCallback,
  useEffect,
  useId,
  useLayoutEffect,
  useRef,
  useState,
  type CSSProperties,
  type ReactNode,
} from "react";
import { createPortal } from "react-dom";

export interface ChipInfo {
  code?: string;
  name?: string;
  childType?: string;
  childCount?: number;
  nodeType?: string;
  nodeCount?: number;
}

const GAP = 6;
const MARGIN = 8;
const HOVER_DELAY_MS = 1250;

interface Props {
  info: ChipInfo;
  children: ReactNode;
  className?: string;
}

export function ChipPopover({ info, children, className }: Props) {
  const [open, setOpen] = useState(false);
  const [style, setStyle] = useState<CSSProperties>({ visibility: "hidden" });
  const wrapRef = useRef<HTMLSpanElement>(null);
  const panelRef = useRef<HTMLDivElement>(null);
  const timer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  const id = useId();

  const cancel = useCallback(() => {
    clearTimeout(timer.current);
    timer.current = undefined;
  }, []);

  useLayoutEffect(() => {
    if (!open) return;
    const w = wrapRef.current;
    const p = panelRef.current;
    if (!w || !p) return;
    const anchor = w.firstElementChild ?? w;
    const t = anchor.getBoundingClientRect();
    const pr = p.getBoundingClientRect();
    const fitsBelow = t.bottom + GAP + pr.height <= window.innerHeight - MARGIN;
    const top = fitsBelow ? t.bottom + GAP : Math.max(MARGIN, t.top - pr.height - GAP);
    const left = Math.max(MARGIN, Math.min(t.left, window.innerWidth - pr.width - MARGIN));
    setStyle({ top, left, visibility: "visible" });
  }, [open]);

  useEffect(() => {
    if (!open) return;
    const close = () => setOpen(false);
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") close();
    };
    window.addEventListener("scroll", close, true);
    window.addEventListener("resize", close);
    window.addEventListener("keydown", onKey);
    return () => {
      window.removeEventListener("scroll", close, true);
      window.removeEventListener("resize", close);
      window.removeEventListener("keydown", onKey);
    };
  }, [open]);

  useEffect(() => cancel, [cancel]);

  const reveal = () => {
    setStyle({ visibility: "hidden" });
    setOpen(true);
  };

  const show = (e: { ctrlKey?: boolean; metaKey?: boolean }) => {
    cancel();
    if (e.ctrlKey || e.metaKey) {
      reveal();
      return;
    }
    timer.current = setTimeout(reveal, HOVER_DELAY_MS);
  };

  const hide = () => {
    cancel();
    setOpen(false);
  };

  return (
    <span
      ref={wrapRef}
      className={"mc-chip-pop-wrap" + (className ? " " + className : "")}
      onMouseEnter={show}
      onMouseMove={(e) => {
        if (!open && (e.ctrlKey || e.metaKey)) {
          cancel();
          reveal();
        }
      }}
      onMouseLeave={hide}
      onFocus={reveal}
      onBlur={hide}
      aria-describedby={open ? id : undefined}
    >
      {children}
      {open &&
        createPortal(
          <div id={id} ref={panelRef} className="mc-chip-pop" role="tooltip" style={style}>
            <dl>
              {info.code && (
                <>
                  <dt>Id</dt>
                  <dd className="mc-mono">{info.code}</dd>
                </>
              )}
              {info.name && (
                <>
                  <dt>Name</dt>
                  <dd>{info.name}</dd>
                </>
              )}
              {info.childType && info.childCount != null && (
                <>
                  <dt>#{info.childType}</dt>
                  <dd className="mc-mono">{info.childCount}</dd>
                </>
              )}
              {info.nodeType && info.nodeCount != null && (
                <>
                  <dt>#{info.nodeType}</dt>
                  <dd className="mc-mono">{info.nodeCount}</dd>
                </>
              )}
            </dl>
          </div>,
          document.body,
        )}
    </span>
  );
}
