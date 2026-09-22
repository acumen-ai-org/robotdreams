import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useConnection } from "../state/ConnectionContext";
import { useEventStream, type StreamState } from "./useEventStream";
import { useRollout, type RolloutState } from "./useRollout";
import { useTraffic, type TrafficApi } from "./useTraffic";
import type { ActivityEntry, PulseItem } from "../lib/types";

const ACTIVITY_CAP = 100;
const PULSE_RAIL_CAP = 60;
const PULSE_KEEP = 2 * PULSE_RAIL_CAP;
const REFRESH_DEBOUNCE_MS = 2000;
const PULSE_FLASH_MS = 900;

let activitySeq = 0;

export interface LiveState {
  stream: StreamState;
  activity: ActivityEntry[];
  pulse: PulseItem[];
  traffic: TrafficApi;
  orgVersion: number;
  storageVersion: number;
  updateVersion: number;
  refreshTick: number;
  scheduleVersion: number;
  pulsedWorkers: ReadonlySet<string>;
  rollout: RolloutState;
  updateKind: string;
  updatesPending: boolean;
}

export function useLiveState(): LiveState {
  const conn = useConnection();
  const [activity, setActivity] = useState<ActivityEntry[]>([]);
  const [pulse, setPulse] = useState<PulseItem[]>([]);
  const [orgVersion, setOrgVersion] = useState(0);
  const [storageVersion, setStorageVersion] = useState(0);
  const [updateVersion, setUpdateVersion] = useState(0);
  const [refreshTick, setRefreshTick] = useState(0);
  const [scheduleVersion, setScheduleVersion] = useState(0);
  const [updateKind, setUpdateKind] = useState("");
  const [pulsedWorkers, setPulsedWorkers] = useState<Set<string>>(new Set());
  const traffic = useTraffic();
  const rollout = useRollout(updateKind, orgVersion + updateVersion, conn.hasToken);
  useEffect(() => {
    if (!updateKind && rollout.kinds.length) setUpdateKind(rollout.kinds[0]);
  }, [updateKind, rollout.kinds]);
  const updatesPending = useMemo(() => {
    const c = rollout.rollout?.counts;
    if (!c) return false;
    return (c.unknown || 0) + (c.acknowledged || 0) + (c.in_progress || 0) + (c.failed || 0) > 0;
  }, [rollout.rollout]);
  const bumpTraffic = traffic.bump;
  const refreshTimer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  const pulseTimers = useRef<Map<string, ReturnType<typeof setTimeout>>>(new Map());

  const addActivity = useCallback(
    (typeClass: string, label: string, detail: string, pulseIds: (string | undefined)[]) => {
      const ids = pulseIds.filter((id): id is string => !!id);
      setActivity((cur) =>
        [{ id: ++activitySeq, typeClass, label, detail, at: new Date().toISOString(), workers: ids }, ...cur].slice(
          0,
          ACTIVITY_CAP,
        ),
      );
      if (ids.length) {
        setPulsedWorkers((cur) => {
          const next = new Set(cur);
          ids.forEach((id) => next.add(id));
          return next;
        });
        ids.forEach((id) => {
          const existing = pulseTimers.current.get(id);
          if (existing) clearTimeout(existing);
          pulseTimers.current.set(
            id,
            setTimeout(() => {
              pulseTimers.current.delete(id);
              setPulsedWorkers((cur) => {
                const next = new Set(cur);
                next.delete(id);
                return next;
              });
            }, PULSE_FLASH_MS),
          );
        });
      }
    },
    [],
  );

  const onSSE = useCallback(
    (eventType: string, raw: unknown) => {
      const data = (raw || {}) as Record<string, string | number>;
      const str = (k: string) => (data[k] == null ? "" : String(data[k]));
      switch (eventType) {
        case "worker_connected":
          addActivity("worker", "worker connected", str("worker_id") + " (" + (str("role") || "?") + ")", [
            str("worker_id"),
          ]);
          setOrgVersion((v) => v + 1);
          break;
        case "worker_disconnected":
          addActivity("worker", "worker disconnected", str("worker_id"), [str("worker_id")]);
          setOrgVersion((v) => v + 1);
          break;
        case "worker_reassigned":
          addActivity(
            "worker",
            "worker reassigned",
            str("worker_id") + ": " + (str("old_reports_to") || "root") + " → " + (str("new_reports_to") || "root"),
            [str("worker_id")],
          );
          setOrgVersion((v) => v + 1);
          break;
        case "worker_role_changed":
          addActivity(
            "worker",
            "role changed",
            str("worker_id") + ": " + (str("old_role") || "—") + " → " + (str("new_role") || "—"),
            [str("worker_id")],
          );
          setOrgVersion((v) => v + 1);
          break;
        case "worker_status_changed":
          addActivity("worker", "status changed", str("worker_id") + " → " + str("status"), [str("worker_id")]);
          setOrgVersion((v) => v + 1);
          break;
        case "message":
          bumpTraffic(str("from"), str("to"), 1);
          addActivity(
            str("type"),
            str("type").replace(/_/g, " "),
            str("from") + " → " + (str("to") || "(unrouted)") + (str("subject") ? ": " + str("subject") : ""),
            [str("from"), str("to")],
          );
          break;
        case "message_volume":
          bumpTraffic(str("from"), str("to"), Number(data.count) || 0);
          addActivity(
            "message_volume",
            "status updates",
            str("from") + " → " + str("to") + ": " + str("count") + " in " + str("window_seconds") + "s",
            [str("from"), str("to")],
          );
          break;
        case "update_announced":
          addActivity(
            "update",
            "update announced",
            str("kind") + " " + str("version") + (str("recipients") ? " → " + str("recipients") + " nodes" : ""),
            [],
          );
          setUpdateVersion((v) => v + 1);
          break;
        case "storage_write":
        case "storage_delete":
          setStorageVersion((v) => v + 1);
          break;
        case "schedule_fired":
          addActivity(
            "schedule",
            "schedule fired",
            (str("id") || str("schedule_id")) +
              (str("to") ? " → " + str("to") : "") +
              (str("subject") ? ": " + str("subject") : ""),
            [str("to")],
          );
          setScheduleVersion((v) => v + 1);
          break;
        case "report_event":
          setPulse((cur) =>
            [
              {
                kind: "event" as const,
                definition: str("definition"),
                scope: str("scope"),
                t: str("t") || new Date().toISOString(),
                severity: str("severity"),
                label: str("label") || str("type") || "event",
              },
              ...cur,
            ].slice(0, PULSE_KEEP),
          );
          break;
        case "report_instance":
          setPulse((cur) =>
            [
              {
                kind: "instance" as const,
                definition: str("definition"),
                scope: str("scope"),
                t: str("produced_at") || new Date().toISOString(),
                severity: "info",
                label: "report updated",
              },
              ...cur,
            ].slice(0, PULSE_KEEP),
          );
          clearTimeout(refreshTimer.current);
          refreshTimer.current = setTimeout(() => setRefreshTick((n) => n + 1), REFRESH_DEBOUNCE_MS);
          break;
        default:
          break;
      }
    },
    [addActivity, bumpTraffic],
  );

  const stream = useEventStream(conn.hasToken, conn.session, conn.apiFetch, onSSE);

  return useMemo(
    () => ({
      stream,
      activity,
      pulse,
      traffic,
      orgVersion,
      storageVersion,
      updateVersion,
      refreshTick,
      scheduleVersion,
      pulsedWorkers,
      rollout,
      updateKind,
      updatesPending,
    }),
    [
      stream,
      activity,
      pulse,
      traffic,
      orgVersion,
      storageVersion,
      updateVersion,
      refreshTick,
      scheduleVersion,
      pulsedWorkers,
      rollout,
      updateKind,
      updatesPending,
    ],
  );
}
