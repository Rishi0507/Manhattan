import { useEffect, useState } from "react";
import { api } from "./lib";

type Prediction = {
  date: string;
  expected_settlements: number;
  expected_amount_inr: number;
  low_bound_inr: number;
  high_bound_inr: number;
  merchant_breakdown: Record<string, number>;
};

type Forecast = {
  generated: string;
  horizon: string;
  predictions: Prediction[];
  confidence: string;
  assumptions: string[];
  risk_factors: string[];
  analysis: string;
};

export function Forecast() {
  const [loading, setLoading] = useState(false);
  const [horizon, setHorizon] = useState<"7d" | "30d">("7d");
  const [forecast, setForecast] = useState<Forecast | null>(null);
  const [error, setError] = useState<string | null>(null);

  const loadForecast = async (h: "7d" | "30d") => {
    setLoading(true);
    setError(null);
    try {
      // Add artificial delay for loading animation (makes it look like AI is thinking)
      const [fc] = await Promise.all([
        api<Forecast>("/api/forecast", {
          method: "POST",
          body: JSON.stringify({ horizon: h }),
        }),
        new Promise(resolve => setTimeout(resolve, 2000)) // 2s minimum
      ]);
      setForecast(fc);
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    void loadForecast(horizon);
  }, []);

  const formatINR = (amount: number) => {
    if (amount >= 10000000) return `₹${(amount / 10000000).toFixed(2)} Cr`;
    if (amount >= 100000) return `₹${(amount / 100000).toFixed(2)} L`;
    return `₹${amount.toLocaleString("en-IN")}`;
  };

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-[19px] font-semibold text-ink">Forward Cash Forecaster</h2>
          <p className="mt-1 text-[13.5px] text-ink-dim">
            Predicted settlement cash flows based on historical patterns
          </p>
        </div>
        <div className="flex gap-2">
          <button
            onClick={() => {
              setHorizon("7d");
              void loadForecast("7d");
            }}
            className={`rounded-md border px-3 py-1.5 text-[13px] transition-colors ${
              horizon === "7d"
                ? "border-accent bg-accent/10 text-accent"
                : "border-line text-ink-dim hover:border-accent"
            }`}
          >
            7 days
          </button>
          <button
            onClick={() => {
              setHorizon("30d");
              void loadForecast("30d");
            }}
            className={`rounded-md border px-3 py-1.5 text-[13px] transition-colors ${
              horizon === "30d"
                ? "border-accent bg-accent/10 text-accent"
                : "border-line text-ink-dim hover:border-accent"
            }`}
          >
            30 days
          </button>
        </div>
      </div>

      {loading && (
        <div className="rounded-lg border border-line bg-ground-alt p-6 text-center">
          <div className="mx-auto h-8 w-8 animate-spin rounded-full border-2 border-accent border-t-transparent"></div>
          <p className="mt-3 text-[13px] text-ink-dim">Analyzing settlement patterns...</p>
        </div>
      )}

      {error && (
        <div className="rounded-lg border border-line bg-ground-alt p-4 text-[13px] text-wrong">
          {error}
        </div>
      )}

      {forecast && !loading && (
        <div className="space-y-4">
          {/* Summary */}
          <div className="grid gap-4 sm:grid-cols-2">
            <div className="rounded-lg border border-line bg-ground-alt p-4">
              <div className="text-[12px] uppercase tracking-wide text-ink-faint">Confidence</div>
              <div className="mt-1 text-[15px] font-medium text-ink">{forecast.confidence}</div>
            </div>
            <div className="rounded-lg border border-line bg-ground-alt p-4">
              <div className="text-[12px] uppercase tracking-wide text-ink-faint">Horizon</div>
              <div className="mt-1 text-[15px] font-medium text-ink">
                {forecast.predictions.length} days
              </div>
            </div>
          </div>

          {/* Analysis */}
          {forecast.analysis && (
            <div className="rounded-lg border border-line bg-ground-alt p-4">
              <div className="text-[13px] text-ink-dim">{forecast.analysis}</div>
            </div>
          )}

          {/* Predictions Table */}
          <div className="overflow-hidden rounded-lg border border-line">
            <div className="overflow-x-auto">
              <table className="w-full">
                <thead className="bg-ground-alt">
                  <tr>
                    <th className="px-4 py-2.5 text-left text-[11.5px] font-medium uppercase tracking-wide text-ink-faint">
                      Date
                    </th>
                    <th className="px-4 py-2.5 text-right text-[11.5px] font-medium uppercase tracking-wide text-ink-faint">
                      Settlements
                    </th>
                    <th className="px-4 py-2.5 text-right text-[11.5px] font-medium uppercase tracking-wide text-ink-faint">
                      Expected Amount
                    </th>
                    <th className="px-4 py-2.5 text-right text-[11.5px] font-medium uppercase tracking-wide text-ink-faint">
                      Range
                    </th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-line">
                  {forecast.predictions.slice(0, 14).map((pred, i) => (
                    <tr key={i} className="hover:bg-ground-alt/50">
                      <td className="px-4 py-2.5 text-[13px] text-ink">{pred.date}</td>
                      <td className="px-4 py-2.5 text-right text-[13px] tabular-nums text-ink">
                        {pred.expected_settlements}
                      </td>
                      <td className="px-4 py-2.5 text-right text-[13px] tabular-nums font-medium text-ink">
                        {formatINR(pred.expected_amount_inr)}
                      </td>
                      <td className="px-4 py-2.5 text-right text-[12px] tabular-nums text-ink-dim">
                        {formatINR(pred.low_bound_inr)} – {formatINR(pred.high_bound_inr)}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </div>

          {/* Assumptions & Risks */}
          <div className="grid gap-4 sm:grid-cols-2">
            {forecast.assumptions && forecast.assumptions.length > 0 && (
              <div className="rounded-lg border border-line bg-ground-alt p-4">
                <div className="text-[12px] font-medium uppercase tracking-wide text-ink-faint">
                  Assumptions
                </div>
                <ul className="mt-2 space-y-1.5">
                  {forecast.assumptions.map((a, i) => (
                    <li key={i} className="text-[13px] text-ink-dim">
                      • {a}
                    </li>
                  ))}
                </ul>
              </div>
            )}

            {forecast.risk_factors && forecast.risk_factors.length > 0 && (
              <div className="rounded-lg border border-line bg-ground-alt p-4">
                <div className="text-[12px] font-medium uppercase tracking-wide text-ink-faint">
                  Risk Factors
                </div>
                <ul className="mt-2 space-y-1.5">
                  {forecast.risk_factors.map((r, i) => (
                    <li key={i} className="text-[13px] text-ink-dim">
                      • {r}
                    </li>
                  ))}
                </ul>
              </div>
            )}
          </div>
        </div>
      )}
    </div>
  );
}
