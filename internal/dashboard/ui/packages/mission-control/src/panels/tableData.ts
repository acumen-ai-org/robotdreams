import type { ReportTable } from "../lib/types";

const APPENDED = "scope";

export function lowerColumns(t: ReportTable | undefined): string[] {
  return (t?.columns || []).map((c) => String(c).toLowerCase());
}

export function columnIndex(cols: string[], names: string[], fallback?: (c: string, i: number) => boolean): number {
  for (const n of names) {
    const i = cols.indexOf(n);
    if (i >= 0) return i;
  }
  return fallback ? cols.findIndex(fallback) : -1;
}

export function numericColumn(cols: string[], rows: unknown[], label: number): number {
  for (const raw of rows) {
    if (!Array.isArray(raw)) continue;
    const i = raw.findIndex((cell, j) => j !== label && cols[j] !== APPENDED && Number.isFinite(Number(cell)));
    if (i >= 0) return i;
  }
  return -1;
}

export function labelColumn(cols: string[], names: string[]): number {
  return columnIndex(cols, names, (c) => c !== APPENDED);
}

export interface LabelledValue {
  label: string;
  value: number;
  row: unknown[];
}

export function sumByLabel(rows: unknown[], label: number, value: number): LabelledValue[] {
  const order: string[] = [];
  const totals = new Map<string, number>();
  const first = new Map<string, unknown[]>();
  for (const raw of rows) {
    if (!Array.isArray(raw)) continue;
    const name = String(raw[label] ?? "").trim();
    const n = Number(raw[value]);
    if (!name || !Number.isFinite(n)) continue;
    if (!totals.has(name)) {
      order.push(name);
      first.set(name, raw);
    }
    totals.set(name, (totals.get(name) || 0) + n);
  }
  return order.map((name) => ({ label: name, value: totals.get(name) || 0, row: first.get(name) || [] }));
}
