import { useEffect, useRef, useState } from "react";
import { storageKey } from "../../lib/storage";
import { useConnection } from "../../state/ConnectionContext";

export function ConnectionPage() {
  const conn = useConnection();
  const [server, setServer] = useState(conn.apiBase);
  const [token, setToken] = useState("");
  const [error, setError] = useState("");
  const [saved, setSaved] = useState(false);
  const serverRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    setServer(conn.apiBase);
  }, [conn.apiBase]);

  useEffect(() => {
    if (!conn.hasToken) serverRef.current?.focus();
  }, [conn.hasToken]);

  const submit = async () => {
    const t = token.trim();
    if (!t) {
      setError("A bearer token is required.");
      return;
    }
    setError("");
    try {
      await conn.connect(server, t);
    } catch (e) {
      setError("Could not connect: " + (e instanceof Error ? e.message : String(e)));
      return;
    }
    setToken("");
    setSaved(true);
  };

  return (
    <>
      <section className="mc-admin-section">
        <div className="mc-admin-status">
          <span className={"mc-pill " + (conn.hasToken ? "mc-pill-live" : "mc-pill-error")}>
            <span className="mc-dot" aria-hidden="true"></span>
            {conn.hasToken ? "connected" : "not connected"}
          </span>
          {conn.hasToken && <span className="mc-body-muted mc-mono">{conn.apiBase || "same origin"}</span>}
        </div>

        <p className="mc-body-muted">
          Mission Control is a plain browser client of the control plane's own HTTP API — it has no privileged access of
          its own. Paste a worker or admin bearer token (from <code>dream worker connect</code>, or an admin token
          you've minted) to authenticate every request it makes. This is a phase-10 simplification: a full browser-based
          keypair / challenge-response login is out of scope for this phase — see the comment in{" "}
          <code>ConnectionContext.tsx</code> for the full rationale.
        </p>

        <div className="mc-admin-form">
          <label htmlFor="server-input">Control plane base URL</label>
          <input
            id="server-input"
            ref={serverRef}
            type="text"
            placeholder="http://127.0.0.1:8420"
            autoComplete="off"
            value={server}
            onChange={(e) => setServer(e.target.value)}
          />
          <label htmlFor="token-input">Bearer token</label>
          <input
            id="token-input"
            type="password"
            placeholder={conn.hasToken ? "•••••••• — paste a new one to replace it" : "eyJhbGciOi…"}
            autoComplete="off"
            value={token}
            onChange={(e) => {
              setToken(e.target.value);
              setSaved(false);
            }}
            onKeyDown={(e) => {
              if (e.key === "Enter") void submit();
            }}
          />
          {error && <p className="mc-form-error">{error}</p>}
          {saved && !error && <p className="mc-form-ok">Connected. The token is stored in this browser.</p>}
          <div className="mc-admin-form-actions">
            <button
              id="token-submit"
              className="mc-button mc-button-primary"
              type="button"
              onClick={() => void submit()}
            >
              {conn.hasToken ? "Replace token" : "Connect"}
            </button>
            {conn.hasToken && (
              <button className="mc-button mc-button-tertiary" type="button" onClick={conn.logout}>
                Disconnect
              </button>
            )}
          </div>
        </div>
      </section>

      <section className="mc-admin-section">
        <h2>Where the token lives</h2>
        <p className="mc-body-muted">
          In this browser's <code>localStorage</code>, under <code className="mc-mono">{storageKey("token")}</code>,
          with the server under <code className="mc-mono">{storageKey("serverBase")}</code>. It is never sent anywhere
          except as an <code>Authorization: Bearer</code> header to the control plane above. Disconnecting removes it
          from this browser; it does not revoke it on the server.
        </p>
      </section>
    </>
  );
}
