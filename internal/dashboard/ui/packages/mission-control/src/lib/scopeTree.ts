import { underScope } from "./routes";
import type { ScopeEntry } from "./types";

export interface ScopeChild {
  name: string;
  path: string;
}

export function instancesUnder(scopes: ScopeEntry[], scope: string): number {
  let n = 0;
  for (const s of scopes) if (underScope(s.path, scope)) n += s.instances || 0;
  return n;
}

export function scopeChildren(scopes: ScopeEntry[], prefix: string): ScopeChild[] {
  const kids = new Map<string, string>();
  for (const s of scopes) {
    const p = s.path;
    if (p === prefix || !underScope(p, prefix)) continue;
    const rest = prefix ? p.slice(prefix.length + 1) : p;
    const seg = rest.split("/")[0];
    if (!seg) continue;
    const full = prefix ? prefix + "/" + seg : seg;
    if (!kids.has(seg)) kids.set(seg, full);
  }
  return Array.from(kids.entries())
    .sort((a, b) => a[0].localeCompare(b[0]))
    .map(([name, path]) => ({ name, path }));
}
