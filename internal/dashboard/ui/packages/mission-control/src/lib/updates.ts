export interface Announcement {
  announcement_id: string;
  kind: string;
  version: string;
  source?: string;
  min_version?: string;
  severity?: string;
  notes?: string;
  announced_by: string;
  announced_at: string;
}

export interface RolloutNode {
  worker_id: string;
  worker_status: string;
  kind: string;
  current_version?: string;
  announcement_id?: string;
  target_version?: string;
  status: string;
  detail?: string;
  reported_at?: string;
}

export interface Rollout {
  kind: string;
  announcement?: Announcement;
  counts: Record<string, number>;
  nodes: RolloutNode[];
}

export interface UpdatePhase {
  id: string;
  label: string;
  hint: string;
  tone: "silent" | "moving" | "done" | "declined" | "failed";
}

export const UPDATE_PHASES: UpdatePhase[] = [
  {
    id: "unknown",
    label: "silent",
    hint: "Announced to it; it has not said anything yet.",
    tone: "silent",
  },
  {
    id: "acknowledged",
    label: "acknowledged",
    hint: "It has seen the announcement and considers it applicable.",
    tone: "moving",
  },
  {
    id: "in_progress",
    label: "in progress",
    hint: "It is applying the update now.",
    tone: "moving",
  },
  {
    id: "applied",
    label: "applied",
    hint: "Done — it reports running the announced version.",
    tone: "done",
  },
  {
    id: "declined",
    label: "declined",
    hint: "It deliberately is not applying this. A legitimate answer, not a failure.",
    tone: "declined",
  },
  {
    id: "failed",
    label: "failed",
    hint: "It tried and it did not work.",
    tone: "failed",
  },
];

const PHASE_BY_ID = new Map(UPDATE_PHASES.map((p) => [p.id, p]));
const SILENT = UPDATE_PHASES[0];

export function phaseOf(status: string | undefined): UpdatePhase {
  if (!status) return SILENT;
  return PHASE_BY_ID.get(status) || SILENT;
}

export function phaseClass(status: string | undefined): string {
  return "is-" + phaseOf(status).tone;
}

export function latestByKind(announcements: Announcement[]): Map<string, Announcement> {
  const out = new Map<string, Announcement>();
  for (const a of announcements) {
    if (!out.has(a.kind)) out.set(a.kind, a);
  }
  return out;
}
