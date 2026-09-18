import { cellText } from "../lib/format";
import type { Panel, ReportTable } from "../lib/types";

export function tableColumns(t: ReportTable | undefined): string[] {
  const rows = t?.rows || [];
  let columns = t?.columns || [];
  if (!columns.length && rows.length && rows[0] && !Array.isArray(rows[0])) {
    columns = Object.keys(rows[0]);
  }
  return columns;
}

export function DataGrid({ panel }: { panel: Panel }) {
  const t = panel.table;
  const rows = t?.rows || [];
  const columns = tableColumns(t);

  if (!columns.length && !rows.length) {
    return <p className="mc-empty-state">No rows.</p>;
  }

  return (
    <div className="mc-storage-table-wrap">
      <table className="mc-storage-table">
        <thead>
          <tr>
            {columns.map((c) => (
              <th key={c} scope="col">
                {String(c)}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {!rows.length ? (
            <tr>
              <td colSpan={columns.length || 1} className="mc-empty-state">
                No rows.
              </td>
            </tr>
          ) : (
            rows.map((row, i) => {
              const cells = Array.isArray(row)
                ? row
                : columns.map((c) => (row && typeof row === "object" ? row[c] : row));
              return (
                <tr key={i}>
                  {cells.map((v, j) => (
                    <td key={j}>{v == null ? "—" : cellText(v)}</td>
                  ))}
                </tr>
              );
            })
          )}
        </tbody>
      </table>
    </div>
  );
}
