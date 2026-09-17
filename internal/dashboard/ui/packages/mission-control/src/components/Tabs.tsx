import { useCallback, useLayoutEffect, useRef, useState, type CSSProperties, type ReactNode } from "react";
import { Link } from "./Link";
import type { Route } from "../lib/routes";

export interface TabDef {
  id: string;
  label: string;
  badge?: ReactNode;
  hint?: string;
  icon?: ReactNode;
  to?: Route;
  disabled?: boolean;
}

interface Props {
  tabs: TabDef[];
  active: string;
  onSelect?: (id: string) => void;
  label: string;
  variant?: "tabs" | "switch";
  className?: string;
}

export function Tabs({ tabs, active, onSelect, label, variant = "tabs", className }: Props) {
  const ref = useRef<HTMLDivElement>(null);
  const [mark, setMark] = useState<CSSProperties | null>(null);
  const isTabs = variant === "tabs";

  useLayoutEffect(() => {
    const strip = ref.current;
    if (!strip) return;
    const measure = () => {
      const on = strip.querySelector<HTMLElement>("[data-mc-tab-on='1']");
      if (!on) {
        setMark(null);
        return;
      }
      setMark({ transform: "translateX(" + on.offsetLeft + "px)", width: on.offsetWidth + "px" });
    };
    measure();
    const raf = requestAnimationFrame(measure);
    if (typeof ResizeObserver === "undefined") return () => cancelAnimationFrame(raf);
    const ro = new ResizeObserver(measure);
    ro.observe(strip);
    for (const el of strip.querySelectorAll<HTMLElement>(".mc-tab")) ro.observe(el);
    return () => {
      cancelAnimationFrame(raf);
      ro.disconnect();
    };
  }, [tabs, active]);

  const onKeyDown = useCallback(
    (e: React.KeyboardEvent) => {
      if (!isTabs || !onSelect) return;
      const i = tabs.findIndex((t) => t.id === active);
      if (i < 0) return;
      let next = i;
      if (e.key === "ArrowRight") next = (i + 1) % tabs.length;
      else if (e.key === "ArrowLeft") next = (i - 1 + tabs.length) % tabs.length;
      else if (e.key === "Home") next = 0;
      else if (e.key === "End") next = tabs.length - 1;
      else return;
      e.preventDefault();
      onSelect(tabs[next].id);
      ref.current?.querySelectorAll<HTMLButtonElement>("[role=tab]")[next]?.focus();
    },
    [tabs, active, onSelect, isTabs],
  );

  return (
    <div
      className={"mc-tabs " + (isTabs ? "is-folder" : "is-switch") + (className ? " " + className : "")}
      role={isTabs ? "tablist" : "group"}
      aria-label={label}
      ref={ref}
      onKeyDown={onKeyDown}
    >
      <span className="mc-tab-mark" style={mark ?? { opacity: 0 }} aria-hidden="true" />
      {tabs.map((t) => {
        const on = t.id === active;
        const inner = (
          <>
            {t.icon && <span className="mc-tab-icon">{t.icon}</span>}
            <span className="mc-tab-label">{t.label}</span>
            {t.badge != null && <span className="mc-tab-badge">({t.badge})</span>}
          </>
        );
        const shared = {
          className: "mc-tab" + (on ? " is-on" : "") + (t.disabled ? " is-disabled" : ""),
          title: t.hint,
          "data-mc-tab-on": on ? "1" : undefined,
        };
        if (t.to && !t.disabled) {
          return (
            <Link key={t.id} {...shared} to={t.to} aria-current={on ? "true" : undefined}>
              {inner}
            </Link>
          );
        }
        return (
          <button
            key={t.id}
            type="button"
            {...shared}
            role={isTabs ? "tab" : undefined}
            aria-selected={isTabs ? on : undefined}
            aria-pressed={isTabs ? undefined : on}
            aria-controls={isTabs ? "tabpanel-" + t.id : undefined}
            id={isTabs ? "tab-" + t.id : undefined}
            tabIndex={isTabs ? (on ? 0 : -1) : undefined}
            disabled={t.disabled}
            onClick={() => onSelect?.(t.id)}
          >
            {inner}
          </button>
        );
      })}
    </div>
  );
}

export function TabPanel({ id, children }: { id: string; children: ReactNode }) {
  return (
    <div className="mc-tabpanel" role="tabpanel" id={"tabpanel-" + id} aria-labelledby={"tab-" + id} tabIndex={0}>
      {children}
    </div>
  );
}
