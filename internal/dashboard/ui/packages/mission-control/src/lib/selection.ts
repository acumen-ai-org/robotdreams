export type Selection =
  | { kind: "place"; path: string }
  | { kind: "node"; id: string }
  | { kind: "report"; def: string; scope: string }
  | null;

export function formatSelection(sel: Selection): string {
  if (!sel) return "";
  if (sel.kind === "place") return sel.path ? "place:" + sel.path : "";
  if (sel.kind === "node") return sel.id ? "node:" + sel.id : "";
  return sel.def ? "report:" + sel.def + "@" + sel.scope : "";
}

export function parseSelection(raw: string | null | undefined): Selection {
  const s = (raw || "").trim();
  if (!s) return null;
  const at = s.indexOf(":");
  if (at < 0) return null;
  const kind = s.slice(0, at);
  const rest = s.slice(at + 1);
  if (!rest) return null;
  if (kind === "place") return { kind: "place", path: rest };
  if (kind === "node") return { kind: "node", id: rest };
  if (kind === "report") {
    const split = rest.lastIndexOf("@");
    if (split < 0) return null;
    const def = rest.slice(0, split);
    const scope = rest.slice(split + 1);
    return def ? { kind: "report", def, scope } : null;
  }
  return null;
}

export function sameSelection(a: Selection, b: Selection): boolean {
  if (!a || !b) return a === b;
  if (a.kind !== b.kind) return false;
  if (a.kind === "place" && b.kind === "place") return a.path === b.path;
  if (a.kind === "node" && b.kind === "node") return a.id === b.id;
  if (a.kind === "report" && b.kind === "report") return a.def === b.def && a.scope === b.scope;
  return false;
}

export const placeID = (path: string) => "scope:" + path;

export function selectedID(sel: Selection): string | null {
  if (!sel) return null;
  if (sel.kind === "place") return placeID(sel.path);
  if (sel.kind === "node") return sel.id;
  return null;
}

export function selectionFromID(id: string | null): Selection {
  if (!id) return null;
  return id.startsWith("scope:") ? { kind: "place", path: id.slice("scope:".length) } : { kind: "node", id };
}
