import { KpiStat } from "../components/shared";
import type { Panel } from "../lib/types";

export function KpiCard({ panel }: { panel: Panel }) {
  if (!panel.kpi) return <p className="mc-empty-state">No value.</p>;
  return (
    <div className="mc-kpi-card">
      <KpiStat kpi={panel.kpi} />
    </div>
  );
}
