import { createContext, useCallback, useContext, useMemo, useState, type ReactNode } from "react";
import { readStored, removeStored, writeStored } from "../lib/storage";

export interface LevelWord {
  one: string;
  many: string;
}

export const DEFAULT_LEVELS: LevelWord[] = [
  { one: "universe", many: "universes" },
  { one: "world", many: "worlds" },
  { one: "realm", many: "realms" },
  { one: "site", many: "sites" },
  { one: "node", many: "nodes" },
];

const LS_KEY = "vocabulary";

interface VocabularyValue {
  levels: LevelWord[];
  one: (depth: number) => string;
  many: (depth: number) => string;
  lower: (depth: number) => string;
  setLevel: (i: number, word: Partial<LevelWord>) => void;
  reset: () => void;
  isCustom: boolean;
}

const VocabularyContext = createContext<VocabularyValue | null>(null);

function load(): LevelWord[] {
  try {
    const raw = readStored(LS_KEY);
    if (!raw) return DEFAULT_LEVELS;
    const parsed = JSON.parse(raw) as Partial<LevelWord>[];
    if (!Array.isArray(parsed)) return DEFAULT_LEVELS;
    return DEFAULT_LEVELS.map((d, i) => ({
      one: (parsed[i]?.one || d.one).trim() || d.one,
      many: (parsed[i]?.many || d.many).trim() || d.many,
    }));
  } catch {
    return DEFAULT_LEVELS;
  }
}

function title(s: string): string {
  return s ? s[0].toUpperCase() + s.slice(1) : s;
}

export function VocabularyProvider({ children }: { children: ReactNode }) {
  const [levels, setLevels] = useState<LevelWord[]>(load);

  const persist = useCallback((next: LevelWord[]) => {
    setLevels(next);
    writeStored(LS_KEY, JSON.stringify(next));
  }, []);

  const setLevel = useCallback(
    (i: number, word: Partial<LevelWord>) => {
      persist(levels.map((l, n) => (n === i ? { ...l, ...word } : l)));
    },
    [levels, persist],
  );

  const reset = useCallback(() => {
    setLevels(DEFAULT_LEVELS);
    removeStored(LS_KEY);
  }, []);

  const value = useMemo<VocabularyValue>(() => {
    const at = (depth: number) => levels[Math.max(0, Math.min(levels.length - 1, depth))];
    return {
      levels,
      one: (d) => title(at(d).one),
      many: (d) => title(at(d).many),
      lower: (d) => at(d).one,
      setLevel,
      reset,
      isCustom: levels.some((l, i) => l.one !== DEFAULT_LEVELS[i].one || l.many !== DEFAULT_LEVELS[i].many),
    };
  }, [levels, setLevel, reset]);

  return <VocabularyContext.Provider value={value}>{children}</VocabularyContext.Provider>;
}

export function useVocabulary(): VocabularyValue {
  const v = useContext(VocabularyContext);
  if (!v) throw new Error("useVocabulary outside VocabularyProvider");
  return v;
}
