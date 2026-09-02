import { useMemo, useState } from "react";
import type { Receipt, Status, Summary } from "./types";
import { STATUSES } from "./types";
import { cls, idx, num, pct, rupeesShort, statusColor, statusGlyph, statusMeaning } from "./lib";
import { Bar, Empty, Flag, Note, Panel, Row, StatusPill, SummaryBar, Td, Th } from "./ui";

/**
 * The run view.
 *
 * The status mix is the headline rather than an accuracy percentage, because
 * the five outcomes are the product. A viewer who reads four of them as
 * "failed" has missed the argument, so the meanings are one keystroke away
 * without occupying the screen by default.
 */
export function Run({
  receipts,
  summary,
  onOpen,
  streaming,
  progress,
}: {
  receipts: Receipt[];
  summary: Summary | null;
  onOpen: (r: Receipt) => void;
  streaming: boolean;
  progress: { done: number; total: number } | null;
}) {
  const [filter, setFilter] = useState<Status | "ALL">("ALL");
  const [query, setQuery] = useState("");
  const [legend, setLegend] = useState(false);

  const counts = useMemo(() => {
    const c: Record<string, number> = {};
    for (const r of receipts) c[r.status] = (c[r.status] ?? 0) + 1;
    return c;
  }, [receipts]);

  const shown = useMemo(() => {
    const q = query.trim().toLowerCase();
    return receipts.filter(
      (r) =>
        (filter === "ALL" || r.status === filter) &&
        (q === "" ||
          r.settlement_ref.toLowerCase().includes(q) ||
          (r.merchant_archetype ?? "").toLowerCase().includes(q) ||
          r.flags.some((f) => f.toLowerCase().includes(q))),
    );
  }, [receipts, filter, query]);

  if (receipts.length === 0) {
    return (
      <Empty>
        No receipts loaded. Run <code className="tnum">./run.sh bench</code>, or start a run from the
        header.
      </Empty>
    );
  }

  const posted = counts["VERIFIED"] ?? 0;
  const n = receipts.length;

  return (
    <div className="space-y-3">
      <SummaryBar
        items={[
          {
            label: "auto-posted",
            value: num(posted),
            sub: `${pct(posted / n)} of ${num(n)}`,
            tone: "var(--color-verified)",
          },
          {
            label: "posted wrong",
            value: summary ? num(summary.auto_posted_wrong) : "0",
            sub: summary ? `B0: ${num(summary.b0_auto_posted_wrong)}` : "vs ground truth",
            tone: summary && summary.auto_posted_wrong > 0 ? "var(--color-wrong)" : "var(--color-verified)",
          },
          { label: "held", value: num(n - posted), sub: "each with a cure" },
          {
            label: "per hour",
            value: summary ? num(Math.round(summary.settlements_per_hour)) : "—",
            sub: summary ? `${summary.median_latency_ms.toFixed(1)} ms median` : undefined,
          },
          {
            label: "cost / 1k",
            value: summary ? `₹${summary.inr_per_1k_settlements.toFixed(0)}` : "—",
            sub: summary ? `B0: ₹${summary.b0_inr_per_1k_settlements.toFixed(0)}` : undefined,
          },
          {
            label: "peak memory",
            value: summary ? `${Math.round(summary.peak_memory_mb)} MB` : "—",
            sub: summary ? `p95 ${summary.p95_latency_ms.toFixed(0)} ms` : undefined,
          },
        ]}
      />

      {streaming && progress && (
        <div className="rounded-md border border-line bg-surface px-3.5 py-2">
          <div className="flex items-baseline justify-between text-[12px]">
            <span className="text-ink-dim">reconciling</span>
            <span className="tnum text-ink-faint">
              {progress.done} / {progress.total}
            </span>
          </div>
          <div className="mt-1.5 h-1 w-full overflow-hidden rounded-[2px] bg-sunken">
            <div
              className="h-full bg-accent transition-[width] duration-200"
              style={{ width: `${(progress.done / Math.max(progress.total, 1)) * 100}%` }}
            />
          </div>
        </div>
      )}

      {/* Status mix as one compact strip of filter chips. */}
      <Panel
        title="Outcomes"
        hint="four of the five stop the money, and none of them is a failure"
        right={
          <button
            onClick={() => setLegend(!legend)}
            className="text-[11.5px] text-ink-faint transition-colors hover:text-accent"
          >
            {legend ? "hide" : "what these mean"}
          </button>
        }
      >
        <Bar
          segments={STATUSES.map((s) => ({
            value: counts[s] ?? 0,
            color: statusColor(s),
            label: `${s}: ${counts[s] ?? 0}`,
          }))}
        />
        <div className="mt-2.5 flex flex-wrap gap-1.5">
          {STATUSES.map((s) => {
            const active = filter === s;
            const c = statusColor(s);
            return (
              <button
                key={s}
                onClick={() => setFilter(active ? "ALL" : s)}
                title={statusMeaning(s)}
                className={cls(
                  "inline-flex items-center gap-1.5 rounded-[3px] border px-2 py-1 text-[11.5px] transition-colors",
                  active ? "border-accent bg-accent-soft" : "border-line hover:bg-raised",
                )}
              >
                <span className="tnum" style={{ color: c }} aria-hidden>
                  {statusGlyph(s)}
                </span>
                <span className="text-ink-dim">{s === "NARROWING_SENSITIVE" ? "SENSITIVE" : s}</span>
                <span className="tnum font-medium" style={{ color: c }}>
                  {counts[s] ?? 0}
                </span>
              </button>
            );
          })}
          {filter !== "ALL" && (
            <button
              onClick={() => setFilter("ALL")}
              className="px-2 py-1 text-[11.5px] text-ink-faint hover:text-accent"
            >
              clear
            </button>
          )}
        </div>

        {legend && (
          <div className="mt-3 space-y-1.5 border-t border-line-soft pt-2.5">
            {STATUSES.map((s) => (
              <div key={s} className="flex gap-2.5">
                <span className="w-[92px] shrink-0">
                  <StatusPill status={s} size="sm" />
                </span>
                <span className="text-[11.5px] leading-snug text-ink-dim">{statusMeaning(s)}</span>
              </div>
            ))}
          </div>
        )}
      </Panel>

      {/* Receipts */}
      <Panel
        flush
        title="Receipts"
        hint={`${shown.length} shown, click for the derivation`}
        right={
          <input
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder="filter"
            className="tnum w-44 rounded border border-line bg-surface px-2 py-1 text-[11.5px] text-ink placeholder:text-ink-faint focus:border-accent focus:outline-none"
          />
        }
      >
        <div className="max-h-[520px] overflow-auto">
          <table className="w-full border-separate border-spacing-0">
            <thead>
              <tr>
                <Th w="170px">settlement</Th>
                <Th>merchant</Th>
                <Th right w="96px">credit</Th>
                <Th right w="56px">pool</Th>
                <Th right w="52px">|S|</Th>
                <Th right w="72px">index</Th>
                <Th w="110px">status</Th>
                <Th>flags</Th>
              </tr>
            </thead>
            <tbody>
              {shown.map((r, i) => (
                <tr
                  key={r.settlement_ref}
                  onClick={() => onOpen(r)}
                  className={cls(
                    "cursor-pointer hover:bg-raised",
                    streaming && i >= shown.length - 2 && "arrive",
                  )}
                >
                  <Td mono dim>
                    {r.settlement_ref.replace("bank_credit_", "")}
                  </Td>
                  <Td dim>{(r.merchant_archetype ?? "").replace(/_/g, " ")}</Td>
                  <Td right mono>
                    {rupeesShort(r.target_paise)}
                  </Td>
                  <Td right mono dim>
                    {r.pool.n}
                  </Td>
                  <Td right mono dim>
                    {r.witness_size || "—"}
                  </Td>
                  <Td right mono dim>
                    {idx(r.feasibility.collision_index_at_k_star)}
                  </Td>
                  <Td>
                    <StatusPill status={r.status} size="sm" />
                  </Td>
                  <Td>
                    <div className="flex flex-wrap gap-1">
                      {r.flags.slice(0, 3).map((f) => (
                        <Flag key={f} name={f} />
                      ))}
                    </div>
                  </Td>
                </tr>
              ))}
            </tbody>
          </table>
          {shown.length === 0 && (
            <div className="py-10 text-center text-[12px] text-ink-faint">nothing matches</div>
          )}
        </div>
      </Panel>

      {/* Segmentation, the commercial claim */}
      {summary && summary.by_archetype?.length > 0 && (
        <Panel
          flush
          title="Which merchants this works for"
          hint="the rate is predictable from the amount distribution alone, before any integration"
        >
          <table className="w-full border-separate border-spacing-0">
            <thead>
              <tr>
                <Th>merchant type</Th>
                <Th right w="90px">spread</Th>
                <Th right w="80px">twin mass</Th>
                <Th right w="110px">auto-post</Th>
                <Th right w="64px">wrong</Th>
                <Th right w="90px">B0 wrong</Th>
                <Th>expected regime</Th>
              </tr>
            </thead>
            <tbody>
              {summary.by_archetype.map((a) => (
                <tr key={a.archetype}>
                  <Td>{a.archetype.replace(/_/g, " ")}</Td>
                  <Td right mono dim>
                    {rupeesShort(Math.round(a.mean_sigma_paise))}
                  </Td>
                  <Td right mono dim>
                    {a.mean_twin_mass.toFixed(2)}
                  </Td>
                  <Td right>
                    <span className="inline-flex items-center gap-1.5">
                      <span className="h-1 w-10 overflow-hidden rounded-[1px] bg-sunken">
                        <span
                          className="block h-full"
                          style={{
                            width: `${a.auto_post_rate * 100}%`,
                            background: "var(--color-verified)",
                          }}
                        />
                      </span>
                      <span className="tnum w-8 text-right">{pct(a.auto_post_rate)}</span>
                    </span>
                  </Td>
                  <Td right mono>
                    <span
                      style={{
                        color: a.auto_posted_wrong > 0 ? "var(--color-wrong)" : "var(--color-verified)",
                      }}
                    >
                      {a.auto_posted_wrong}
                    </span>
                  </Td>
                  <Td right mono>
                    <span
                      style={{
                        color: a.b0_wrong_post_rate > 0.2 ? "var(--color-wrong)" : "var(--color-ink-faint)",
                      }}
                    >
                      {pct(a.b0_wrong_post_rate)}
                    </span>
                  </Td>
                  <Td dim>{a.expected_regime}</Td>
                </tr>
              ))}
            </tbody>
          </table>
          <div className="p-3.5">
            <Note tone="var(--color-accent)">
              Read the two wrong-posting columns against twin mass. Where amounts genuinely fail to
              distinguish transactions, our auto-post rate falls to zero and B0's wrong-posting rate
              climbs to 73%. Both systems see the same data. One reacts to it.
            </Note>
          </div>
        </Panel>
      )}

      {/* Cost, compact */}
      {summary && (
        <div className="grid gap-3 lg:grid-cols-2">
          <Panel title="What it costs to be right" hint="accuracy that costs 10x is not obviously a win">
            <div className="space-y-0.5">
              <Row
                label="model calls per settlement"
                value={(summary.model_calls / Math.max(summary.settlements, 1)).toFixed(2)}
              />
              <Row label="exception rate" value={pct(summary.exception_rate, 1)} dim />
              <Row
                label="input tokens per 1k"
                value={`${(summary.input_tokens_per_1k / 1e6).toFixed(2)}M`}
              />
              <Row
                label="B0 input tokens per 1k"
                value={`${(summary.b0_input_tokens_per_1k / 1e6).toFixed(2)}M`}
                dim
              />
              <Row
                label="cost per 1k settlements"
                value={`₹${summary.inr_per_1k_settlements.toFixed(2)}`}
                tone="var(--color-accent)"
                strong
              />
              <Row
                label="B0 cost per 1k"
                value={`₹${summary.b0_inr_per_1k_settlements.toFixed(2)}`}
                dim
              />
            </div>
            <p className="mt-2.5 text-[11.5px] leading-relaxed text-ink-faint">
              Priced at <span className="tnum">{summary.priced_at_model}</span>
              {summary.price_is_real_spend ? "" : ", modelled rather than billed"}. B0's input scales
              with pool size because it must read the pool; ours reads one line of narration whatever
              the pool size.
            </p>
          </Panel>

          {summary.narrowing_drift && summary.narrowing_drift.length > 0 ? (
            <Panel title="This run is gated" hint="a property of the batch, not of any settlement">
              {summary.narrowing_drift.map((d) => (
                <div key={d.constraint}>
                  <p className="text-[12px] leading-relaxed text-ink">
                    <span className="tnum" style={{ color: "var(--color-sensitive)" }}>
                      {d.constraint}
                    </span>{" "}
                    dropped {pct(d.drop_rate_observed, 1)} of the record universe against a stored
                    baseline of {pct(d.drop_rate_baseline, 1)}.
                  </p>
                  <p className="mt-1.5 text-[11.5px] leading-relaxed text-ink-faint">
                    It holds the whole batch rather than appearing on receipts an analyst could clear
                    one by one. {d.note}
                  </p>
                </div>
              ))}
            </Panel>
          ) : (
            <Panel title="Reproducibility" hint="a decision that cannot be reproduced cannot be audited">
              <div className="space-y-0.5">
                <Row label="run" value={summary.run_id} dim />
                <Row label="seed" value={summary.seed} />
                <Row label="provider" value={summary.provider} dim />
                <Row label="models" value={summary.provider_models.split(" ")[0]} dim />
              </div>
              <p className="mt-2.5 text-[11.5px] leading-relaxed text-ink-faint">
                Every amount is an integer count of paise, with no floating point anywhere in the
                verification path. The same seed reproduces the same receipts byte for byte.
              </p>
            </Panel>
          )}
        </div>
      )}
    </div>
  );
}
