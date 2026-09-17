import { useEffect, useRef, useState } from "react";
import { readSSE, type SSEHandler } from "../lib/sse";

export type StreamState = "offline" | "connecting" | "live" | "reconnecting";

const RECONNECT_MS = 3000;

export function useEventStream(
  enabled: boolean,
  session: number,
  apiFetch: (path: string, opts?: RequestInit) => Promise<Response>,
  onEvent: SSEHandler,
): StreamState {
  const [state, setState] = useState<StreamState>("offline");
  const handler = useRef(onEvent);
  handler.current = onEvent;

  useEffect(() => {
    if (!enabled) {
      setState("offline");
      return;
    }
    const abort = new AbortController();
    let timer: ReturnType<typeof setTimeout> | undefined;

    const run = async () => {
      setState("connecting");
      try {
        const resp = await apiFetch("/api/events", { signal: abort.signal });
        setState("live");
        await readSSE(resp, (t, d) => handler.current(t, d));
        if (abort.signal.aborted) return;
        setState("offline");
      } catch {
        if (abort.signal.aborted) return;
        setState("reconnecting");
      }
      timer = setTimeout(run, RECONNECT_MS);
    };

    void run();
    return () => {
      abort.abort();
      if (timer) clearTimeout(timer);
    };
  }, [enabled, session, apiFetch]);

  return state;
}
