import { useCallback, useSyncExternalStore } from "react";
import { readStored, writeStored } from "./storage";

const EVENT = "dream:display";

export type TimeMode = "list" | "timeline";
export type TimeOrientation = "vertical" | "horizontal";

export const DISPLAY_KEYS = {
  timeMode: "timeMode",
  timeOrientation: "timeOrientation",
  exploreMode: "exploreMode",
  spaceMode: "spaceMode",
  nodeListMode: "nodeListMode",
  glanceMode: "glanceMode",
} as const;

function read(key: string, fallback: string): string {
  return readStored(key) ?? fallback;
}

function write(key: string, value: string): void {
  writeStored(key, value);
  window.dispatchEvent(new CustomEvent(EVENT, { detail: key }));
}

function subscribe(onChange: () => void): () => void {
  window.addEventListener(EVENT, onChange);
  window.addEventListener("storage", onChange);
  return () => {
    window.removeEventListener(EVENT, onChange);
    window.removeEventListener("storage", onChange);
  };
}

export function useDisplayPref<T extends string>(key: string, allowed: readonly T[], fallback: T): [T, (v: T) => void] {
  const value = useSyncExternalStore(
    subscribe,
    () => read(key, fallback),
    () => fallback,
  );
  const set = useCallback((v: T) => write(key, v), [key]);
  return [allowed.includes(value as T) ? (value as T) : fallback, set];
}

export function useTimeMode(): [TimeMode, (v: TimeMode) => void] {
  return useDisplayPref<TimeMode>(DISPLAY_KEYS.timeMode, ["list", "timeline"], "list");
}

export function useTimeOrientation(): [TimeOrientation, (v: TimeOrientation) => void] {
  return useDisplayPref<TimeOrientation>(DISPLAY_KEYS.timeOrientation, ["vertical", "horizontal"], "vertical");
}

export type ExploreMode = "places" | "reports";

export function useExploreMode(): [ExploreMode, (v: ExploreMode) => void] {
  return useDisplayPref<ExploreMode>(DISPLAY_KEYS.exploreMode, ["places", "reports"], "places");
}

export type SpaceMode = "planets" | "galaxy";

export function useSpaceMode(): [SpaceMode, (v: SpaceMode) => void] {
  return useDisplayPref<SpaceMode>(DISPLAY_KEYS.spaceMode, ["planets", "galaxy"], "planets");
}

export type NodeListMode = "table" | "boxes";

export function useNodeListMode(): [NodeListMode, (v: NodeListMode) => void] {
  return useDisplayPref<NodeListMode>(DISPLAY_KEYS.nodeListMode, ["table", "boxes"], "table");
}

export type GlanceMode = "board" | "delta";

export function useGlanceMode(): [GlanceMode, (v: GlanceMode) => void] {
  return useDisplayPref<GlanceMode>(DISPLAY_KEYS.glanceMode, ["board", "delta"], "board");
}
