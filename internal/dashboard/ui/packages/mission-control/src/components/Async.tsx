import { useEffect, useState, type ReactNode } from "react";
import { Link } from "./Link";
import type { Route } from "../lib/routes";

interface Props<T> {
  data: T[] | null;
  error?: string;
  onRetry?: () => void;
  empty: ReactNode;
  skeleton: ReactNode;
  children: (data: T[]) => ReactNode;
}

export function Async<T>({ data, error, onRetry, empty, skeleton, children }: Props<T>) {
  if (error) {
    return (
      <div className="mc-async-error" role="alert">
        <strong>Could not load this.</strong>
        <p className="mc-body-muted">{error}</p>
        {onRetry && (
          <button className="mc-button mc-button-primary" type="button" onClick={onRetry}>
            Try again
          </button>
        )}
      </div>
    );
  }
  if (data === null) return <>{skeleton}</>;
  if (data.length === 0) return <>{empty}</>;
  return <>{children(data)}</>;
}

export function DelayedLoading({ delayMs = 500, children }: { delayMs?: number; children?: ReactNode }) {
  const [show, setShow] = useState(false);
  useEffect(() => {
    const t = setTimeout(() => setShow(true), delayMs);
    return () => clearTimeout(t);
  }, [delayMs]);
  if (!show) return null;
  return <>{children ?? <Skeleton lines={3} />}</>;
}

export function Skeleton({ lines = 3, className = "" }: { lines?: number; className?: string }) {
  return (
    <div className={"mc-skeleton " + className} aria-hidden="true">
      {Array.from({ length: lines }, (_, i) => (
        <span className="mc-skeleton-line" key={i} />
      ))}
    </div>
  );
}

export function TileSkeletons({ count = 6 }: { count?: number }) {
  return (
    <div className="mc-tiles-grid" aria-busy="true" aria-label="Loading reports">
      {Array.from({ length: count }, (_, i) => (
        <div className="mc-tile mc-skeleton-tile" key={i}>
          <Skeleton lines={4} />
        </div>
      ))}
    </div>
  );
}

export function NoMatches({ clearTo }: { clearTo?: Route }) {
  return (
    <div className="mc-async-empty">
      <strong>No reports match this filter.</strong>
      <p className="mc-body-muted">
        There is data at this scope, but nothing in it answers the current category and stance selection.
      </p>
      {clearTo && (
        <Link className="mc-button mc-button-tertiary" to={clearTo}>
          Clear filters
        </Link>
      )}
    </div>
  );
}

export function Onboarding() {
  return (
    <div className="mc-reporting-onboard">
      <strong>No report scopes yet.</strong>
      <span>Register report definitions when starting the control plane:</span>
      <code>dream server --reports reporting/library/</code>
      <span>
        Then submit instances (POST /api/reports/instances) or pulse events (POST /api/reports/events) from any
        connected worker — scopes and tiles appear here as reports arrive. For a fully populated demo, run: dream
        simulate
      </span>
    </div>
  );
}
