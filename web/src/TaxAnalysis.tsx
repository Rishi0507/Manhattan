import { useEffect, useState } from "react";
import { api } from "./lib";

type TaxLine = {
  settlement_ref: string;
  fee_paise: number;
  gst_rate: number;
  gst_computed: number;
  gst_declared: number;
  difference_paise: number;
  status: string;
};

type Anomaly = {
  type: string;
  settlement_ref: string;
  description: string;
  impact_inr: string;
  severity: string;
};

type Analysis = {
  total_gst_computed: number;
  total_gst_declared: number;
  discrepancy_paise: number;
  discrepancy_inr: string;
  match_rate: number;
  lines: TaxLine[];
  anomalies: Anomaly[];
  compliance_notes: string[];
  explanation: string;
};

export function TaxAnalysis() {
  const [loading, setLoading] = useState(false);
  const [analysis, setAnalysis] = useState<Analysis | null>(null);
  const [error, setError] = useState<string | null>(null);

  const loadAnalysis = async () => {
    setLoading(true);
    setError(null);
    try {
      // Add artificial delay for loading animation
      const [data] = await Promise.all([
        api<Analysis>("/api/tax-analysis"),
        new Promise(resolve => setTimeout(resolve, 2000)) // 2s minimum
      ]);
      setAnalysis(data);
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    void loadAnalysis();
  }, []);

  const formatPaise = (paise: number) => {
    return `₹${(paise / 100).toFixed(2)}`;
  };

  const statusColor = (status: string) => {
    if (status === "match") return "text-green-600";
    if (status === "rounding") return "text-yellow-600";
    return "text-red-600";
  };

  const severityBadge = (severity: string) => {
    const colors = {
      low: "bg-green-100 text-green-800",
      medium: "bg-yellow-100 text-yellow-800",
      high: "bg-red-100 text-red-800",
    };
    return colors[severity as keyof typeof colors] || colors.low;
  };

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-[19px] font-semibold text-ink">Tax-Line Matcher</h2>
          <p className="mt-1 text-[13.5px] text-ink-dim">
            GST reconciliation across settlement fees
          </p>
        </div>
        <button
          onClick={loadAnalysis}
          disabled={loading}
          className="rounded-md border border-line px-3 py-1.5 text-[13px] text-ink-dim transition-colors hover:border-accent hover:text-accent disabled:opacity-50"
        >
          {loading ? "Analyzing..." : "Refresh"}
        </button>
      </div>

      {loading && (
        <div className="rounded-lg border border-line bg-ground-alt p-6 text-center">
          <div className="mx-auto h-8 w-8 animate-spin rounded-full border-2 border-accent border-t-transparent"></div>
          <p className="mt-3 text-[13px] text-ink-dim">Reconciling tax components...</p>
        </div>
      )}

      {error && (
        <div className="rounded-lg border border-line bg-ground-alt p-4 text-[13px] text-wrong">
          {error}
        </div>
      )}

      {analysis && !loading && (
        <div className="space-y-4">
          {/* Summary Cards */}
          <div className="grid gap-4 sm:grid-cols-3">
            <div className="rounded-lg border border-line bg-ground-alt p-4">
              <div className="text-[12px] uppercase tracking-wide text-ink-faint">
                Match Rate
              </div>
              <div className="mt-1 text-[20px] font-semibold tabular-nums text-ink">
                {(analysis.match_rate * 100).toFixed(1)}%
              </div>
            </div>
            <div className="rounded-lg border border-line bg-ground-alt p-4">
              <div className="text-[12px] uppercase tracking-wide text-ink-faint">
                Total Discrepancy
              </div>
              <div className="mt-1 text-[20px] font-semibold tabular-nums text-ink">
                {analysis.discrepancy_inr}
              </div>
            </div>
            <div className="rounded-lg border border-line bg-ground-alt p-4">
              <div className="text-[12px] uppercase tracking-wide text-ink-faint">
                Anomalies Found
              </div>
              <div className="mt-1 text-[20px] font-semibold tabular-nums text-ink">
                {analysis.anomalies.length}
              </div>
            </div>
          </div>

          {/* Explanation */}
          {analysis.explanation && (
            <div className="rounded-lg border border-line bg-ground-alt p-4">
              <div className="text-[12px] font-medium uppercase tracking-wide text-ink-faint">
                Analysis
              </div>
              <div className="mt-2 text-[13px] text-ink-dim">{analysis.explanation}</div>
            </div>
          )}

          {/* Anomalies */}
          {analysis.anomalies.length > 0 && (
            <div className="rounded-lg border border-line">
              <div className="border-b border-line bg-ground-alt px-4 py-2.5">
                <h3 className="text-[13px] font-medium text-ink">Detected Anomalies</h3>
              </div>
              <div className="divide-y divide-line">
                {analysis.anomalies.map((a, i) => (
                  <div key={i} className="p-4">
                    <div className="flex items-start justify-between">
                      <div className="flex-1">
                        <div className="flex items-center gap-2">
                          <span className="text-[13px] font-mono text-ink-dim">
                            {a.settlement_ref}
                          </span>
                          <span
                            className={`rounded px-1.5 py-0.5 text-[11px] font-medium ${severityBadge(
                              a.severity,
                            )}`}
                          >
                            {a.severity}
                          </span>
                        </div>
                        <div className="mt-1 text-[13px] text-ink">{a.description}</div>
                      </div>
                      <div className="ml-4 text-right text-[13px] tabular-nums font-medium text-ink">
                        {a.impact_inr}
                      </div>
                    </div>
                  </div>
                ))}
              </div>
            </div>
          )}

          {/* Compliance Notes */}
          {analysis.compliance_notes && analysis.compliance_notes.length > 0 && (
            <div className="rounded-lg border border-line bg-ground-alt p-4">
              <div className="text-[12px] font-medium uppercase tracking-wide text-ink-faint">
                Compliance Notes
              </div>
              <ul className="mt-2 space-y-1.5">
                {analysis.compliance_notes.map((note, i) => (
                  <li key={i} className="text-[13px] text-ink-dim">
                    • {note}
                  </li>
                ))}
              </ul>
            </div>
          )}

          {/* Sample Tax Lines */}
          {analysis.lines.length > 0 && (
            <div className="overflow-hidden rounded-lg border border-line">
              <div className="border-b border-line bg-ground-alt px-4 py-2.5">
                <h3 className="text-[13px] font-medium text-ink">
                  Tax Lines (showing first 20)
                </h3>
              </div>
              <div className="overflow-x-auto">
                <table className="w-full">
                  <thead className="bg-ground-alt">
                    <tr>
                      <th className="px-4 py-2.5 text-left text-[11.5px] font-medium uppercase tracking-wide text-ink-faint">
                        Settlement
                      </th>
                      <th className="px-4 py-2.5 text-right text-[11.5px] font-medium uppercase tracking-wide text-ink-faint">
                        GST Computed
                      </th>
                      <th className="px-4 py-2.5 text-right text-[11.5px] font-medium uppercase tracking-wide text-ink-faint">
                        GST Declared
                      </th>
                      <th className="px-4 py-2.5 text-right text-[11.5px] font-medium uppercase tracking-wide text-ink-faint">
                        Difference
                      </th>
                      <th className="px-4 py-2.5 text-center text-[11.5px] font-medium uppercase tracking-wide text-ink-faint">
                        Status
                      </th>
                    </tr>
                  </thead>
                  <tbody className="divide-y divide-line">
                    {analysis.lines.slice(0, 20).map((line, i) => (
                      <tr key={i} className="hover:bg-ground-alt/50">
                        <td className="px-4 py-2.5 text-[12px] font-mono text-ink-dim">
                          {line.settlement_ref.split("_").pop()}
                        </td>
                        <td className="px-4 py-2.5 text-right text-[13px] tabular-nums text-ink">
                          {formatPaise(line.gst_computed)}
                        </td>
                        <td className="px-4 py-2.5 text-right text-[13px] tabular-nums text-ink">
                          {formatPaise(line.gst_declared)}
                        </td>
                        <td className="px-4 py-2.5 text-right text-[13px] tabular-nums text-ink">
                          {formatPaise(line.difference_paise)}
                        </td>
                        <td className="px-4 py-2.5 text-center">
                          <span className={`text-[12px] font-medium ${statusColor(line.status)}`}>
                            {line.status}
                          </span>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </div>
          )}
        </div>
      )}
    </div>
  );
}
