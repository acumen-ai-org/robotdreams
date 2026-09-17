import { useCallback, useEffect, useLayoutEffect, useRef, useState } from "react";
import { createPortal } from "react-dom";
import { usePageControls } from "../../state/PageControlsContext";
import { ScopeSelector } from "../../components/ScopeSelector";
import { BarExpander } from "../../components/BarExpander";
import { CheckIcon, EraserIcon, FunnelIcon, XIcon } from "../../components/icons";
import { UniverseIcon } from "../../lib/vocabulary";
import { CategoryFilters } from "./CategoryFilters";
import { Tabs } from "../../components/Tabs";
import { StanceFilter } from "./StanceFilter";
import { reportingRoute, type Route } from "../../lib/routes";
import type { ScopeEntry } from "../../lib/types";
import { useRouter } from "../../state/RouterContext";

const MAX_LISTED = 2;

const HYSTERESIS = 32;

function useBarHasRoom(): [boolean, (el: HTMLElement | null) => void] {
  const [inline, setInline] = useState(true);
  const [host, setHost] = useState<HTMLElement | null>(null);
  const costs = useRef(0);

  const measure = useCallback(
    (toolbox: HTMLElement, self: HTMLElement) => {
      const tools = toolbox.querySelector<HTMLElement>(".mc-toolbox-tools");
      if (!tools || !toolbox.clientWidth) return;
      if (inline) {
        const open = self.querySelector<HTMLElement>(".mc-category-filters");
        if (open) costs.current = open.offsetWidth;
        if (tools.scrollWidth > tools.clientWidth + 1) setInline(false);
        return;
      }
      const kids = [...toolbox.children] as HTMLElement[];
      const gap = parseFloat(getComputedStyle(toolbox).columnGap) || 0;
      const used = kids.reduce((sum, el) => sum + el.offsetWidth, 0) + gap * Math.max(0, kids.length - 1);
      const slack = toolbox.clientWidth - used;
      if (costs.current && slack >= costs.current + HYSTERESIS) setInline(true);
    },
    [inline],
  );

  useLayoutEffect(() => {
    if (!host) return;
    const toolbox = host.closest<HTMLElement>(".mc-toolbox");
    if (!toolbox) return;
    const run = () => measure(toolbox, host);
    run();
    if (typeof ResizeObserver === "undefined") return;
    const ro = new ResizeObserver(run);
    ro.observe(toolbox);
    const tools = toolbox.querySelector<HTMLElement>(".mc-toolbox-tools");
    if (tools) ro.observe(tools);
    return () => ro.disconnect();
  }, [host, measure]);

  return [inline, setHost];
}

export function ReportingControls({
  route,
  scopes,
  stuck,
}: {
  route: Route;
  scopes: ScopeEntry[];
  stuck?: { on: boolean; set: (v: boolean) => void };
}) {
  const controls = usePageControls();
  const { navigate } = useRouter();
  const open = controls.openPanel === "filters";
  const count = route.cats.length;
  const stuckOn = !!stuck?.on;
  const triggerRef = useRef<HTMLButtonElement>(null);
  const [inline, setHost] = useBarHasRoom();

  const original = useRef<{ cats: string[]; stance: string; stuck: boolean } | null>(null);
  const wasOpen = useRef(false);
  const committed = useRef(false);

  const setFilters = useCallback(
    (cats: string[], stance: string) => {
      navigate(reportingRoute(route, { cats, stance, report: "" }), { replace: true });
    },
    [route, navigate],
  );

  useEffect(() => {
    if (open && !wasOpen.current) {
      original.current = { cats: route.cats, stance: route.stance, stuck: stuckOn };
      committed.current = false;
    }
    if (!open && wasOpen.current && !committed.current && original.current) {
      const o = original.current;
      if (o.cats.join(",") !== route.cats.join(",") || o.stance !== route.stance) {
        setFilters(o.cats, o.stance);
      }
      if (stuck && o.stuck !== stuck.on) stuck.set(o.stuck);
    }
    wasOpen.current = open;
  }, [open, route.cats, route.stance, stuck, stuckOn, setFilters]);

  const apply = () => {
    committed.current = true;
    controls.setOpenPanel(null);
  };

  const picks = count + (route.stance ? 1 : 0) + (stuckOn ? 1 : 0);

  const beyond = (route.stance ? 1 : 0) + (stuckOn ? 1 : 0);
  const more =
    "More filters" +
    (beyond
      ? " — " + [route.stance, stuckOn ? "stuck only" : ""].filter(Boolean).join(", ")
      : ": how to read these, and more");

  const chip = open ? (
    <span className="mc-control-empty">.......</span>
  ) : (
    <>
      {route.stance && <span className="mc-tag mc-tag-stance">{route.stance}</span>}
      {stuckOn && <span className="mc-tag mc-tag-stuck">stuck</span>}
      {!count ? (
        <span className="mc-control-empty">Any category</span>
      ) : count <= MAX_LISTED ? (
        <span className="mc-control-tags">
          {route.cats.map((c) => (
            <span className="mc-tag" key={c}>
              {c}
            </span>
          ))}
        </span>
      ) : (
        <span className="mc-control-count">{count} selected</span>
      )}
    </>
  );

  const caret = (
    <button
      ref={triggerRef}
      className={"mc-toolbox-button mc-toolbox-filters" + (open ? " is-open" : "") + (inline ? " is-inline" : "")}
      type="button"
      aria-expanded={open}
      aria-label={
        inline
          ? more
          : (count ? "Filters: " + route.cats.join(", ") : "Filters: any category") +
            (route.stance ? " — read as " + route.stance : "")
      }
      title={inline ? more : count > MAX_LISTED ? route.cats.join(", ") : undefined}
      onClick={() => controls.togglePanel("filters")}
      data-mc-panel-trigger=""
    >
      {inline ? (
        <>
          {route.stance && <span className="mc-tag mc-tag-stance">{route.stance}</span>}
          {stuckOn && <span className="mc-tag mc-tag-stuck">stuck</span>}
          {!beyond && <span className="mc-control-empty">More</span>}
        </>
      ) : (
        chip
      )}
      <span className="mc-rc-caret" aria-hidden="true">
        {open ? "▴" : "▾"}
      </span>
    </button>
  );

  const filters = (
    <span className={"mc-view-select mc-filters-select" + (inline ? " is-inline" : "")} ref={setHost}>
      <span className="mc-view-select-label">
        <FunnelIcon size={11} /> Filters
      </span>
      <span className="mc-control-group">
        {inline && !open && <CategoryFilters route={route} compact />}
        {caret}
      </span>
    </span>
  );

  return (
    <>
      <span className="mc-view-select">
        <span className="mc-view-select-label">
          <UniverseIcon size={11} /> Scope
        </span>
        <span className="mc-control-group">
          <ScopeSelector route={route} scopes={scopes} />
        </span>
      </span>

      {controls.filtersSlot && createPortal(filters, controls.filtersSlot)}

      <BarExpander
        open={open}
        triggerRef={triggerRef}
        actions={
          <>
            {picks > 0 && (
              <button
                className="mc-bar-action"
                type="button"
                aria-label="Clear filters"
                title="Clear filters"
                onClick={() => {
                  setFilters([], "");
                  if (stuck) stuck.set(false);
                }}
              >
                <EraserIcon size={14} />
              </button>
            )}
            <button className="mc-bar-action is-primary" type="button" aria-label="Apply" title="Apply" onClick={apply}>
              <CheckIcon size={14} />
            </button>
            <button
              className="mc-bar-action"
              type="button"
              aria-label="Cancel"
              title="Cancel"
              onClick={() => controls.setOpenPanel(null)}
            >
              <XIcon size={14} />
            </button>
          </>
        }
        controlCopy={
          <span className="mc-view-select">
            <span className="mc-view-select-label">
              <FunnelIcon size={11} /> Filters
            </span>
            <span className="mc-control-group">
              <button
                className="mc-toolbox-button mc-toolbox-filters is-open"
                type="button"
                aria-expanded={true}
                onClick={() => controls.setOpenPanel(null)}
              >
                {chip}
                <span className="mc-rc-caret" aria-hidden="true">
                  ▴
                </span>
              </button>
            </span>
          </span>
        }
        left={
          <div className="mc-filter-group">
            <span className="mc-filter-group-label">Categories</span>
            <CategoryFilters route={route} />
          </div>
        }
        right={
          <>
            <div className="mc-filter-group">
              <span className="mc-filter-group-label">Read these as</span>
              <StanceFilter route={route} />
            </div>
            {stuck && (
              <div className="mc-filter-group">
                <span className="mc-filter-group-label">Board items</span>
                <Tabs
                  tabs={[
                    { id: "all", label: "all" },
                    { id: "stuck", label: "stuck", hint: "Only what is blocked or past due" },
                  ]}
                  active={stuck.on ? "stuck" : "all"}
                  onSelect={(id) => stuck.set(id === "stuck")}
                  label="Which board items to show"
                  variant="switch"
                />
              </div>
            )}
          </>
        }
      />
    </>
  );
}
