const segs = (path: string) => path.split("/").filter(Boolean);

export function scopeParent(path: string): string {
  const parts = segs(path);
  return parts.length > 1 ? parts.slice(0, -1).join("/") : "";
}

export function scopeAncestors(path: string): string[] {
  const parts = segs(path);
  const out: string[] = [];
  for (let i = 1; i < parts.length; i++) out.push(parts.slice(0, i).join("/"));
  return out;
}

export function scopeLeaf(path: string): string {
  const parts = segs(path);
  return parts[parts.length - 1] || "";
}

export function scopeDepth(path: string): number {
  return segs(path).length;
}
