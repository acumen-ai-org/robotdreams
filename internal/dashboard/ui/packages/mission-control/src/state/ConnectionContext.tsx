import { createContext, useCallback, useContext, useMemo, useRef, useState, type ReactNode } from "react";
import { readStored, removeStored, writeStored } from "../lib/storage";

const LS_TOKEN = "token";
const LS_SERVER = "serverBase";

export class ApiError extends Error {
  status: number;
  constructor(message: string, status: number) {
    super(message);
    this.status = status;
  }
}

export interface ConnectionValue {
  hasToken: boolean;
  apiBase: string;
  session: number;
  apiFetch: (path: string, opts?: RequestInit) => Promise<Response>;
  apiJSON: <T = unknown>(path: string, opts?: RequestInit) => Promise<T>;
  connect: (serverBase: string, token: string) => Promise<void>;
  logout: () => void;
  canConnect: boolean;
}

const ConnectionContext = createContext<ConnectionValue | null>(null);

export interface ConnectionProviderProps {
  children: ReactNode;
  token?: string;
  apiBase?: string;
  fetchImpl?: typeof fetch;
  persist?: boolean;
  fallbackToken?: string;
  onUnauthorized?: () => void;
}

export function ConnectionProvider({
  children,
  token,
  apiBase = "",
  fetchImpl,
  persist = false,
  fallbackToken,
  onUnauthorized,
}: ConnectionProviderProps) {
  const creds = useRef(initialCreds(persist, token, apiBase, fallbackToken));
  const [session, setSession] = useState(0);
  const [hasToken, setHasToken] = useState(!!creds.current.token);
  const canConnect = persist && !token;

  const apiFetch = useCallback(
    async (path: string, opts?: RequestInit): Promise<Response> => {
      const headers = new Headers(opts?.headers);
      if (creds.current.token) headers.set("Authorization", "Bearer " + creds.current.token);
      const doFetch = fetchImpl ?? fetch;
      const resp = await doFetch((creds.current.apiBase || "") + path, { ...opts, headers });
      if (!resp.ok) {
        const msg = (await errorMessageOf(resp)) || resp.statusText;
        throw new ApiError(msg || "HTTP " + resp.status, resp.status);
      }
      return resp;
    },
    [fetchImpl],
  );

  const apiJSON = useCallback(
    async <T,>(path: string, opts?: RequestInit): Promise<T> => {
      const resp = await apiFetch(path, opts);
      return resp.json() as Promise<T>;
    },
    [apiFetch],
  );

  const connect = useCallback(
    async (serverBase: string, token: string) => {
      const server = serverBase.trim().replace(/\/+$/, "");
      const prev = { ...creds.current };
      creds.current = { apiBase: server, token };
      try {
        await apiJSON("/api/workers");
      } catch (e) {
        creds.current = prev;
        throw e;
      }
      if (persist) {
        writeStored(LS_SERVER, server);
        writeStored(LS_TOKEN, token);
      }
      setHasToken(true);
      setSession((s) => s + 1);
    },
    [apiJSON, persist],
  );

  const logout = useCallback(() => {
    if (persist) removeStored(LS_TOKEN);
    creds.current.token = "";
    setHasToken(false);
    onUnauthorized?.();
  }, [persist, onUnauthorized]);

  const value = useMemo<ConnectionValue>(
    () => ({
      hasToken,
      apiBase: creds.current.apiBase,
      session,
      apiFetch,
      apiJSON,
      connect,
      logout,
      canConnect,
    }),
    [hasToken, session, apiFetch, apiJSON, connect, logout, canConnect],
  );

  return <ConnectionContext.Provider value={value}>{children}</ConnectionContext.Provider>;
}

function initialCreds(persist: boolean, token: string | undefined, apiBase: string, fallbackToken: string | undefined) {
  const stored = persist ? readStored(LS_TOKEN) || "" : "";
  if (stored) return { token: stored, apiBase: apiBase || (persist ? readStored(LS_SERVER) || "" : "") };
  if (token) return { token, apiBase };
  if (persist && fallbackToken) return { token: fallbackToken, apiBase: "" };
  return { token: "", apiBase: apiBase || (persist ? readStored(LS_SERVER) || "" : "") };
}

export function useConnection(): ConnectionValue {
  const v = useContext(ConnectionContext);
  if (!v) throw new Error("useConnection outside ConnectionProvider");
  return v;
}

async function errorMessageOf(resp: Response): Promise<string> {
  try {
    const body: unknown = await resp.json();
    if (body && typeof body === "object" && "error" in body && typeof body.error === "string") return body.error;
  } catch {
    return "";
  }
  return "";
}
