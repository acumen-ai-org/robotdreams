import { useEffect, useState } from "react";
import { AppWindowIcon } from "../../components/icons";
import { appHost, safeAppURL, type NodeApp } from "../../lib/apps";

export function AppBox({ app }: { app: NodeApp }) {
  const [embed, setEmbed] = useState(false);
  const href = safeAppURL(app.url);
  const host = appHost(app.url);

  useEffect(() => setEmbed(false), [app.worker_id]);

  if (!href) return null;

  return (
    <section className="mc-panel mc-app-box" aria-labelledby="app-box-heading">
      <div className="mc-panel-header">
        <h2 id="app-box-heading">
          <span className="mc-app-box-icon" aria-hidden="true">
            <AppWindowIcon size={14} />
          </span>{" "}
          App
        </h2>
        <button
          type="button"
          className="mc-button mc-button-secondary mc-app-embed-toggle"
          aria-pressed={embed}
          onClick={() => setEmbed((v) => !v)}
          title={embed ? "Stop showing the app here" : "Show the app here, in this panel"}
        >
          {embed ? "Hide" : "Preview"}
        </button>
      </div>

      <a className="mc-app-link" href={href} target="_blank" rel="noopener noreferrer" title={"Opens " + href}>
        <span className="mc-app-link-open">Open</span>
        <span className="mc-app-link-host mc-mono">{host}</span>
        <span className="mc-app-link-arrow" aria-hidden="true">
          ↗
        </span>
      </a>
      {app.description && <p className="mc-body-muted mc-detail-app-description">{app.description}</p>}

      {embed && (
        <div className="mc-app-frame-wrap">
          <iframe
            className="mc-app-frame"
            src={href}
            title={"App served by " + host}
            sandbox="allow-scripts allow-same-origin allow-forms allow-popups"
            referrerPolicy="no-referrer"
            loading="lazy"
          />
          <p className="mc-label-muted mc-app-frame-note">
            Served by {host}. If this stays blank, the app refuses to be embedded — open it instead.
          </p>
        </div>
      )}
    </section>
  );
}
