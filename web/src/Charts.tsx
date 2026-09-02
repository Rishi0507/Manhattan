import { useState, type ReactNode } from "react";
import { idx } from "./lib";

/**
 * Charts.
 *
 * Four forms, each chosen for the job its data does rather than for variety.
 * The categorical pair is ochre and blue, validated for colourblind separation
 * against the cream surface rather than chosen by eye: adjacent-pair Delta E
 * 28.7 under protanopia, 26.8 under tritanopia, both well clear of the floor.
 *
 * Status colours are reserved for status and never reused as a series, and
 * every series carries a legend as well as its colour, so identity is never
 * conveyed by hue alone.
 */

export const SERIES = {
  a: "#bf6206", // ochre, sits in the page palette
  b: "#0969da", // blue
} as const;

const GRID = "#e0d4bd";
const AXIS = "#9a8a72";

function useTip() {
  const [tip, setTip] = useState<{ x: number; y: number; body: ReactNode } | null>(null);
  const node = tip ? (
    <div
      className="pointer-events-none absolute z-10 rounded-md border border-line-strong bg-surface px-2.5 py-1.5 text-[12px] shadow-sm"
      style={{ left: tip.x + 12, top: tip.y - 8 }}
    >
      {tip.body}
    </div>
  ) : null;
  return { tip, setTip, node };
}

export function Legend({ items }: { items: { label: string; color: string; dashed?: boolean }[] }) {
  return (
    <div className="mt-2 flex flex-wrap gap-x-5 gap-y-1">
      {items.map((it) => (
        <span key={it.label} className="inline-flex items-center gap-1.5 text-[12px] text-ink-dim">
          {it.dashed ? (
            <svg width="16" height="8" aria-hidden>
              <line x1="0" y1="4" x2="16" y2="4" stroke={it.color} strokeWidth="2" strokeDasharray="3 3" />
            </svg>
          ) : (
            <span className="size-2.5 rounded-full" style={{ background: it.color }} />
          )}
          {it.label}
        </span>
      ))}
    </div>
  );
}

/**
 * Predicted against observed, on log scales, with an identity line.
 *
 * The job is agreement: does the estimate match what actually happened? A
 * scatter against y = x is the only form that answers it directly, because
 * distance from the diagonal is the error and the eye reads it without
 * arithmetic. Points below the line are under-predictions.
 */
export function AgreementScatter({
  points,
  height = 300,
}: {
  points: { observed: number; empirical: number; analytic: number; label: string }[];
  height?: number;
}) {
  const { setTip, node } = useTip();
  const pad = { l: 52, r: 16, t: 12, b: 36 };
  const w = 640;
  const h = height;
  const iw = w - pad.l - pad.r;
  const ih = h - pad.t - pad.b;

  const all = points.flatMap((p) => [p.observed, p.empirical, p.analytic]).filter((v) => v > 0);
  if (all.length === 0) return null;
  const lo = Math.log10(Math.max(Math.min(...all), 0.005));
  const hi = Math.log10(Math.max(...all, 1));
  const span = hi - lo || 1;

  const X = (v: number) => pad.l + ((Math.log10(Math.max(v, Math.pow(10, lo))) - lo) / span) * iw;
  const Y = (v: number) => pad.t + ih - ((Math.log10(Math.max(v, Math.pow(10, lo))) - lo) / span) * ih;

  const ticks: number[] = [];
  for (let e = Math.ceil(lo); e <= Math.floor(hi); e++) ticks.push(Math.pow(10, e));

  return (
    <div className="relative">
      <svg viewBox={`0 0 ${w} ${h}`} className="w-full" role="img" aria-label="Predicted against observed reconstruction counts">
        {ticks.map((t) => (
          <g key={t}>
            <line x1={X(t)} y1={pad.t} x2={X(t)} y2={pad.t + ih} stroke={GRID} strokeWidth="1" />
            <line x1={pad.l} y1={Y(t)} x2={pad.l + iw} y2={Y(t)} stroke={GRID} strokeWidth="1" />
            <text x={X(t)} y={h - 14} textAnchor="middle" fontSize="11" fill={AXIS} className="tnum">
              {idx(t)}
            </text>
            <text x={pad.l - 8} y={Y(t) + 4} textAnchor="end" fontSize="11" fill={AXIS} className="tnum">
              {idx(t)}
            </text>
          </g>
        ))}

        {/* Perfect agreement. Everything is read relative to this line. */}
        <line
          x1={pad.l}
          y1={pad.t + ih}
          x2={pad.l + iw}
          y2={pad.t}
          stroke={AXIS}
          strokeWidth="2"
          strokeDasharray="4 4"
        />
        <text x={pad.l + iw - 6} y={pad.t + 14} textAnchor="end" fontSize="11" fill={AXIS}>
          perfect agreement
        </text>

        {points.map((p, i) => (
          <g key={`a${i}`}>
            <circle
              cx={X(p.observed)}
              cy={Y(p.analytic)}
              r="5"
              fill={SERIES.b}
              fillOpacity="0.75"
              stroke="#fffdf8"
              strokeWidth="2"
              onMouseEnter={(e) =>
                setTip({
                  x: e.nativeEvent.offsetX,
                  y: e.nativeEvent.offsetY,
                  body: (
                    <span className="tnum">
                      {p.label}
                      <br />
                      observed {idx(p.observed)} · analytic {idx(p.analytic)}
                    </span>
                  ),
                })
              }
              onMouseLeave={() => setTip(null)}
            />
          </g>
        ))}
        {points.map((p, i) => (
          <circle
            key={`e${i}`}
            cx={X(p.observed)}
            cy={Y(p.empirical)}
            r="5"
            fill={SERIES.a}
            fillOpacity="0.85"
            stroke="#fffdf8"
            strokeWidth="2"
            onMouseEnter={(e) =>
              setTip({
                x: e.nativeEvent.offsetX,
                y: e.nativeEvent.offsetY,
                body: (
                  <span className="tnum">
                    {p.label}
                    <br />
                    observed {idx(p.observed)} · measured {idx(p.empirical)}
                  </span>
                ),
              })
            }
            onMouseLeave={() => setTip(null)}
          />
        ))}

        <text x={pad.l + iw / 2} y={h - 1} textAnchor="middle" fontSize="11" fill={AXIS}>
          reconstructions actually counted
        </text>
        <text
          x={14}
          y={pad.t + ih / 2}
          textAnchor="middle"
          fontSize="11"
          fill={AXIS}
          transform={`rotate(-90 14 ${pad.t + ih / 2})`}
        >
          predicted
        </text>
      </svg>
      {node}
      <Legend
        items={[
          { label: "measured, by sampling the pool", color: SERIES.a },
          { label: "analytic closed form", color: SERIES.b },
          { label: "perfect agreement", color: AXIS, dashed: true },
        ]}
      />
    </div>
  );
}

/**
 * The collision index against what the system decided.
 *
 * The job is composition across an ordered band, so a stacked bar per band is
 * the form. A 2px surface gap separates segments, which is what lets adjacent
 * fills read as distinct without outlines.
 */
export function OutcomeBands({
  bands,
  height = 260,
}: {
  bands: {
    lo: number;
    hi: number;
    n: number;
    parts: { label: string; value: number; color: string }[];
    wrong: number;
    b0Wrong: number;
  }[];
  height?: number;
}) {
  const { setTip, node } = useTip();
  const pad = { l: 92, r: 120, t: 8, b: 26 };
  const w = 640;
  const h = Math.max(height, bands.length * 30 + pad.t + pad.b);
  const iw = w - pad.l - pad.r;
  const rowH = (h - pad.t - pad.b) / Math.max(bands.length, 1);

  return (
    <div className="relative">
      <svg viewBox={`0 0 ${w} ${h}`} className="w-full" role="img" aria-label="Outcome mix by predicted collision index">
        {bands.map((band, i) => {
          const y = pad.t + i * rowH;
          const barH = Math.min(rowH - 10, 20);
          const total = band.parts.reduce((a, p) => a + p.value, 0) || 1;
          let x = pad.l;
          return (
            <g key={i}>
              <text x={pad.l - 10} y={y + barH / 2 + 4} textAnchor="end" fontSize="11" fill={AXIS} className="tnum">
                {idx(band.lo)} to {idx(band.hi)}
              </text>
              {band.parts.map((p, j) => {
                const segW = (p.value / total) * iw;
                const rect = (
                  <rect
                    key={j}
                    x={x}
                    y={y}
                    width={Math.max(segW - 2, 0)}
                    height={barH}
                    rx={j === 0 || j === band.parts.length - 1 ? 3 : 0}
                    fill={p.color}
                    onMouseEnter={(e) =>
                      setTip({
                        x: e.nativeEvent.offsetX,
                        y: e.nativeEvent.offsetY,
                        body: (
                          <span className="tnum">
                            {p.label} {Math.round((p.value / total) * 100)}%
                          </span>
                        ),
                      })
                    }
                    onMouseLeave={() => setTip(null)}
                  />
                );
                x += segW;
                return rect;
              })}
              <text
                x={pad.l + iw + 10}
                y={y + barH / 2 + 4}
                fontSize="11"
                className="tnum"
                fill={band.b0Wrong > 0.15 ? "#a93226" : AXIS}
              >
                B0 {Math.round(band.b0Wrong * 100)}% wrong
              </text>
            </g>
          );
        })}
        <text x={pad.l} y={h - 6} fontSize="11" fill={AXIS}>
          predicted collision index, low to high
        </text>
      </svg>
      {node}
    </div>
  );
}

/**
 * Solve time against pool size, with the free cardinality labelled.
 *
 * The job is magnitude across a small ordered set, so bars. The label matters
 * more than the bar here: it shows cost tracking k rather than n, which is
 * the point of the whole solver design.
 */
export function EnvelopeBars({
  rows,
  height = 220,
}: {
  rows: { pool: number; k: number; ms: number; mb: number }[];
  height?: number;
}) {
  const { setTip, node } = useTip();
  const pad = { l: 44, r: 12, t: 14, b: 46 };
  const w = 640;
  const h = height;
  const iw = w - pad.l - pad.r;
  const ih = h - pad.t - pad.b;
  const max = Math.max(...rows.map((r) => r.ms), 1);
  const bw = iw / Math.max(rows.length, 1);

  return (
    <div className="relative">
      <svg viewBox={`0 0 ${w} ${h}`} className="w-full" role="img" aria-label="Solve and prove time by pool size">
        {[0, 0.5, 1].map((f) => (
          <g key={f}>
            <line x1={pad.l} y1={pad.t + ih - f * ih} x2={pad.l + iw} y2={pad.t + ih - f * ih} stroke={GRID} strokeWidth="1" />
            <text x={pad.l - 8} y={pad.t + ih - f * ih + 4} textAnchor="end" fontSize="11" fill={AXIS} className="tnum">
              {Math.round(f * max)}
            </text>
          </g>
        ))}
        {rows.map((r, i) => {
          const bh = (r.ms / max) * ih;
          const x = pad.l + i * bw + bw * 0.22;
          const bwid = bw * 0.56;
          return (
            <g key={i}>
              <rect
                x={x}
                y={pad.t + ih - bh}
                width={bwid}
                height={Math.max(bh, 2)}
                rx="4"
                fill={SERIES.a}
                onMouseEnter={(e) =>
                  setTip({
                    x: e.nativeEvent.offsetX,
                    y: e.nativeEvent.offsetY,
                    body: (
                      <span className="tnum">
                        pool {r.pool}, k {r.k}
                        <br />
                        {r.ms.toFixed(0)} ms · {r.mb.toFixed(0)} MB
                      </span>
                    ),
                  })
                }
                onMouseLeave={() => setTip(null)}
              />
              <text x={x + bwid / 2} y={h - 26} textAnchor="middle" fontSize="11" fill={AXIS} className="tnum">
                {r.pool}
              </text>
              <text x={x + bwid / 2} y={h - 12} textAnchor="middle" fontSize="10" fill={AXIS} className="tnum">
                k={r.k}
              </text>
            </g>
          );
        })}
        <text x={14} y={pad.t + ih / 2} textAnchor="middle" fontSize="11" fill={AXIS} transform={`rotate(-90 14 ${pad.t + ih / 2})`}>
          ms
        </text>
      </svg>
      {node}
    </div>
  );
}

/**
 * The collision index across cardinality, for one settlement.
 *
 * The job is change across an ordinal with a decision boundary on it, so a
 * line with a threshold rule and the accepted region shaded. This is the
 * clearest single picture of why a settlement was accepted or refused.
 */
export function FeasibilityCurve({
  curve,
  kStar,
  threshold,
  height = 200,
}: {
  curve: { k: number; collision_index: number; collision_index_analytic: number }[];
  kStar: number;
  threshold: number;
  height?: number;
}) {
  const { setTip, node } = useTip();
  if (curve.length < 2) return null;

  const pad = { l: 48, r: 14, t: 12, b: 30 };
  const w = 560;
  const h = height;
  const iw = w - pad.l - pad.r;
  const ih = h - pad.t - pad.b;

  const vals = curve.flatMap((c) => [c.collision_index, c.collision_index_analytic]).filter((v) => v > 0);
  const lo = Math.log10(Math.max(Math.min(...vals, threshold) / 10, 1e-6));
  const hi = Math.log10(Math.max(...vals, threshold) * 4);
  const span = hi - lo || 1;

  const X = (k: number) => pad.l + ((k - curve[0].k) / Math.max(curve.length - 1, 1)) * iw;
  const Y = (v: number) => pad.t + ih - ((Math.log10(Math.max(v, Math.pow(10, lo))) - lo) / span) * ih;

  const path = (pick: (c: (typeof curve)[number]) => number) =>
    curve.map((c, i) => `${i === 0 ? "M" : "L"} ${X(c.k)} ${Y(pick(c))}`).join(" ");

  return (
    <div className="relative">
      <svg viewBox={`0 0 ${w} ${h}`} className="w-full" role="img" aria-label="Collision index across free cardinality">
        {/* The region the gate accepts. */}
        {kStar >= curve[0].k && (
          <rect
            x={pad.l}
            y={pad.t}
            width={Math.max(X(kStar) - pad.l, 0)}
            height={ih}
            fill={SERIES.a}
            fillOpacity="0.06"
          />
        )}
        <line x1={pad.l} y1={Y(threshold)} x2={pad.l + iw} y2={Y(threshold)} stroke="#a93226" strokeWidth="2" strokeDasharray="4 4" />
        <text x={pad.l + iw - 4} y={Y(threshold) - 6} textAnchor="end" fontSize="11" fill="#a93226">
          refusal threshold
        </text>

        <path d={path((c) => c.collision_index_analytic)} fill="none" stroke={SERIES.b} strokeWidth="2" strokeOpacity="0.55" />
        <path d={path((c) => c.collision_index)} fill="none" stroke={SERIES.a} strokeWidth="2" />

        {curve.map((c) => (
          <circle
            key={c.k}
            cx={X(c.k)}
            cy={Y(c.collision_index)}
            r="4.5"
            fill={SERIES.a}
            stroke="#fffdf8"
            strokeWidth="2"
            onMouseEnter={(e) =>
              setTip({
                x: e.nativeEvent.offsetX,
                y: e.nativeEvent.offsetY,
                body: (
                  <span className="tnum">
                    k = {c.k}
                    <br />
                    measured {idx(c.collision_index)} · analytic {idx(c.collision_index_analytic)}
                  </span>
                ),
              })
            }
            onMouseLeave={() => setTip(null)}
          />
        ))}

        {curve.map((c) => (
          <text key={`x${c.k}`} x={X(c.k)} y={h - 12} textAnchor="middle" fontSize="11" fill={AXIS} className="tnum">
            {c.k}
          </text>
        ))}
        <text x={pad.l + iw / 2} y={h - 1} textAnchor="middle" fontSize="11" fill={AXIS}>
          free cardinality k
        </text>
      </svg>
      {node}
      <Legend
        items={[
          { label: "measured", color: SERIES.a },
          { label: "analytic", color: SERIES.b },
          { label: "refusal threshold", color: "#a93226", dashed: true },
        ]}
      />
    </div>
  );
}
