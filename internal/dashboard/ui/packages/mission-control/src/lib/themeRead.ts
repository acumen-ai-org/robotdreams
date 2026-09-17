export function resolveTheme(root: HTMLElement): "light" | "dark" {
  const el = root.closest("[data-theme]");
  return el?.getAttribute("data-theme") === "dark" ? "dark" : "light";
}

export function cssVar(root: HTMLElement, name: string, fallback = ""): string {
  const v = getComputedStyle(root).getPropertyValue(name).trim();
  return v || fallback;
}
