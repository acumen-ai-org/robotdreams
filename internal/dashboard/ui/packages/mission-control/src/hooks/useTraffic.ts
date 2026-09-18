import { useCallback, useEffect, useRef, useState } from "react";

const DECAY_MS = 4000;
const DECAY = 0.8;
const FLOOR = 0.08;

export function edgeKey(a: string, b: string): string {
  return a < b ? a + "\u0000" + b : b + "\u0000" + a;
}

export interface Traffic {
  weights: ReadonlyMap<string, number>;
  peak: number;
}

export interface TrafficApi extends Traffic {
  bump: (from: string, to: string, n: number) => void;
}

export function useTraffic(): TrafficApi {
  const live = useRef(new Map<string, number>());
  const dirty = useRef(false);
  const [state, setState] = useState<Traffic>({ weights: new Map(), peak: 0 });

  const bump = useCallback((from: string, to: string, n: number) => {
    if (!from || !to || n <= 0) return;
    const k = edgeKey(from, to);
    live.current.set(k, (live.current.get(k) || 0) + n);
    dirty.current = true;
  }, []);

  useEffect(() => {
    const id = setInterval(() => {
      const m = live.current;
      if (!m.size && !dirty.current) return;
      let peak = 0;
      for (const [k, v] of m) {
        const next = v * DECAY;
        if (next < FLOOR) m.delete(k);
        else {
          m.set(k, next);
          if (next > peak) peak = next;
        }
      }
      dirty.current = false;
      setState({ weights: new Map(m), peak });
    }, DECAY_MS);
    return () => clearInterval(id);
  }, []);

  return { weights: state.weights, peak: state.peak, bump };
}
