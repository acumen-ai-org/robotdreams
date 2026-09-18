import { createPortal } from "react-dom";
import type { ReactNode } from "react";
import { usePageControls } from "../state/PageControlsContext";

export interface RenderingOption<T extends string> {
  id: T;
  label: string;
  icon: ReactNode;
}

interface Props<T extends string> {
  label: string;
  value: T;
  options: ReadonlyArray<RenderingOption<T>>;
  onChange: (id: T) => void;
  className?: string;
}

export function RenderingToggle<T extends string>({ label, value, options, onChange, className }: Props<T>) {
  return (
    <div className={"mc-rendering-toggle" + (className ? " " + className : "")} role="group" aria-label={label}>
      {options.map((o) => (
        <button
          key={o.id}
          type="button"
          className={"mc-rendering-option" + (o.id === value ? " is-on" : "")}
          aria-pressed={o.id === value}
          aria-label={o.label}
          title={o.label}
          onClick={() => onChange(o.id)}
        >
          {o.icon}
        </button>
      ))}
    </div>
  );
}

export function ToolboxRenderingToggle<T extends string>({
  active,
  label,
  value,
  options,
  onChange,
}: Props<T> & { active: boolean }) {
  const controls = usePageControls();
  if (!active || !controls.toolsSlot) return null;
  return createPortal(
    <span className="mc-view-select">
      <span className="mc-view-select-label">{label}</span>
      <RenderingToggle label={label} value={value} options={options} onChange={onChange} />
    </span>,
    controls.toolsSlot,
  );
}
