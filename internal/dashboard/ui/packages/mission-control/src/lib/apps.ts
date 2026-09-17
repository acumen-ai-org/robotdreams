export interface NodeApp {
  worker_id: string;
  url: string;
  description?: string;
  declared_at: string;
}

const SAFE_SCHEMES = new Set(["http:", "https:"]);

export function safeAppURL(raw: string | undefined): string {
  if (!raw) return "";
  let u: URL;
  try {
    u = new URL(raw);
  } catch {
    return "";
  }
  return SAFE_SCHEMES.has(u.protocol) ? u.href : "";
}

export function appHost(raw: string | undefined): string {
  const safe = safeAppURL(raw);
  if (!safe) return "";
  try {
    return new URL(safe).host;
  } catch {
    return "";
  }
}

export function appsByWorker(apps: NodeApp[]): Map<string, NodeApp> {
  const out = new Map<string, NodeApp>();
  for (const a of apps) {
    if (safeAppURL(a.url)) out.set(a.worker_id, a);
  }
  return out;
}
