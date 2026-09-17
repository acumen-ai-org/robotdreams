import { useCallback, useRef, type ReactNode } from "react";
import { ChevronsLeftIcon, ChevronsRightIcon } from "./icons";

export const SIDE_MIN = 260;
export const SIDE_MAX = 720;

export function clampSideWidth(w: number): number {
  return Math.min(SIDE_MAX, Math.max(SIDE_MIN, Math.round(w)));
}

interface Props {
  label: string;
  kind?: string;
  name?: ReactNode;
  icon?: ReactNode;
  code?: string;
  header?: ReactNode;
  empty?: boolean;
  collapsed: boolean;
  onToggle: () => void;
  onWidth: (w: number) => void;
  children: ReactNode;
}

export function SidePanel({
  label,
  kind,
  name,
  icon,
  code,
  header,
  empty,
  collapsed,
  onToggle,
  onWidth,
  children,
}: Props) {
  const ref = useRef<HTMLElement | null>(null);

  const onPointerDown = useCallback(
    (e: React.PointerEvent) => {
      e.preventDefault();
      const grid = ref.current?.parentElement;
      if (!grid) return;
      const right = grid.getBoundingClientRect().right;
      const move = (ev: PointerEvent) => onWidth(clampSideWidth(right - ev.clientX));
      const up = () => {
        window.removeEventListener("pointermove", move);
        window.removeEventListener("pointerup", up);
        grid.classList.remove("is-resizing");
      };
      grid.classList.add("is-resizing");
      window.addEventListener("pointermove", move);
      window.addEventListener("pointerup", up);
    },
    [onWidth],
  );

  const onKeyDown = useCallback(
    (e: React.KeyboardEvent) => {
      const cur = ref.current?.getBoundingClientRect().width ?? SIDE_MIN;
      const step = e.shiftKey ? 48 : 16;
      if (e.key === "ArrowLeft") {
        e.preventDefault();
        onWidth(clampSideWidth(cur + step));
      } else if (e.key === "ArrowRight") {
        e.preventDefault();
        onWidth(clampSideWidth(cur - step));
      }
    },
    [onWidth],
  );

  if (collapsed) {
    return (
      <aside className="mc-side-rail" aria-label={label}>
        <button
          className="mc-side-toggle mc-side-rail-toggle"
          type="button"
          onClick={onToggle}
          aria-expanded={false}
          aria-label={"Show " + label}
          title={"Show " + label}
        >
          <ChevronsLeftIcon size={18} />
        </button>
        <span className="mc-side-rail-label" aria-hidden="true">
          {label}
        </span>
      </aside>
    );
  }

  return (
    <aside className="mc-col-side mc-side-panel" ref={ref} aria-label={label}>
      {/* eslint-disable-next-line jsx-a11y/no-noninteractive-element-interactions */}
      <div
        className="mc-side-resizer"
        role="separator"
        aria-orientation="vertical"
        aria-label={"Resize " + label}
        // eslint-disable-next-line jsx-a11y/no-noninteractive-tabindex
        tabIndex={0}
        onPointerDown={onPointerDown}
        onKeyDown={onKeyDown}
      />
      <div className="mc-side-panel-chrome">
        {/* eslint-disable-next-line jsx-a11y/click-events-have-key-events, jsx-a11y/no-static-element-interactions */}
        <div className="mc-side-panel-head" onClick={onToggle} title={"Hide " + label}>
          {!empty && (
            <>
              {icon}
              <div className="mc-side-panel-heading">
                {kind && <span className="mc-side-panel-kind">{kind}</span>}
                <h2 className="mc-side-panel-title">{name ?? label}</h2>
              </div>
            </>
          )}
          <button
            className="mc-side-toggle mc-side-panel-toggle"
            type="button"
            onClick={(e) => {
              e.stopPropagation();
              onToggle();
            }}
            aria-expanded={true}
            aria-label={"Hide " + label}
          >
            <ChevronsRightIcon size={18} />
          </button>
          {code && (
            <span className="mc-side-panel-code mc-mono" title="Its code in the identification vocabulary">
              {code}
            </span>
          )}
        </div>
        {header}
      </div>
      <div className={"mc-side-panel-body" + (empty ? " is-empty" : "")}>{children}</div>
    </aside>
  );
}
