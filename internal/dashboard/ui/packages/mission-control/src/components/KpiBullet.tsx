import { fmtNum, unitLabel } from "../lib/format";
import { normalisedVariance } from "../lib/triage";
import type { KPIValue } from "../lib/types";

const W = 150;
const H = 14;

const TARGET_AT_TWO_THIRDS = 1.5;

export function KpiBullet({ kpi }: { kpi: KPIValue }) {
  const value = typeof kpi.value === "number" ? kpi.value : NaN;
  const target = typeof kpi.target === "number" ? kpi.target : NaN;
  const unit = unitLabel(kpi.unit);

  if (!isFinite(value) || !isFinite(target) || target === 0) {
    return (
      <span className="mc-kpi-bullet is-plain">
        <span className="mc-kpi-bullet-value">
          {fmtNum(kpi.value)}
          {unit ? " " + unit : ""}
        </span>
        <span className="mc-kpi-bullet-name mc-label-muted">{kpi.name || ""}</span>
      </span>
    );
  }

  const scale = Math.max(value, target * TARGET_AT_TWO_THIRDS);
  const bar = Math.max(2, (value / scale) * W);
  const tick = (target / scale) * W;
  const off = normalisedVariance(kpi);
  const state = off > 0.001 ? "is-bad" : off < -0.001 ? "is-good" : "is-flat";

  return (
    <span className="mc-kpi-bullet">
      <span className="mc-kpi-bullet-value">
        {fmtNum(value)}
        {unit ? " " + unit : ""}
      </span>
      <svg
        className={"mc-kpi-bullet-svg " + state}
        viewBox={`0 0 ${W} ${H}`}
        width={W}
        height={H}
        role="img"
        aria-label={`${kpi.name || "value"}: ${fmtNum(value)} against a target of ${fmtNum(target)}`}
      >
        <rect className="mc-bullet-track" x="0" y="3" width={W} height={H - 6} rx="3" />
        <rect className="mc-bullet-fill" x="0" y="3" width={bar} height={H - 6} rx="3" />
        <rect className="mc-bullet-target" x={Math.min(tick, W - 2)} y="0" width="2" height={H} />
      </svg>
      <span className="mc-kpi-bullet-name mc-label-muted">
        {kpi.name || ""} · target {fmtNum(target)}
      </span>
    </span>
  );
}
