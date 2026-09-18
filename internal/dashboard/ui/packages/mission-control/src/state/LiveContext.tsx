import { createContext, useContext, type ReactNode } from "react";
import { useLiveState, type LiveState } from "../hooks/useLiveState";

const LiveContext = createContext<LiveState | null>(null);

export function LiveProvider({ children }: { children: ReactNode }) {
  return <LiveContext.Provider value={useLiveState()}>{children}</LiveContext.Provider>;
}

const EMPTY_WEIGHTS: ReadonlyMap<string, number> = new Map();

const INERT: LiveState = Object.freeze({
  stream: "offline",
  activity: [],
  pulse: [],
  traffic: { weights: EMPTY_WEIGHTS, peak: 0, bump: () => {} },
  orgVersion: 0,
  storageVersion: 0,
  updateVersion: 0,
  refreshTick: 0,
  scheduleVersion: 0,
  pulsedWorkers: new Set<string>(),
  rollout: { kinds: [], latest: new Map(), rollout: null, updateOf: new Map(), adminOnly: false, error: "" },
  updateKind: "",
  updatesPending: false,
});

export function useLive(): LiveState {
  return useContext(LiveContext) ?? INERT;
}
