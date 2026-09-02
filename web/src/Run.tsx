import { useMemo, useState } from "react";
import type { Receipt, Status, Summary } from "./types";
import { STATUSES } from "./types";
import { cls, idx, num, pct, rupees, rupeesShort, statusColor, statusMeaning } from "./lib";
import { Bar, Empty, Field, Flag, Note, Panel, Stat, StatusPill, Td, Th } from "./ui";

/**
 * The run view: what a batch produced, and every receipt in it.
 *
 * The status mix is the headline rather than an accuracy percentage, because
 * the five outcomes are the product. A viewer who reads four of them as
 * "failed" has missed the argument, so each one carries its meaning inline.
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
          (r.merchant_name ?? "").toLowerCase().includes(q) ||
          r.flags.some((f) => f.toLowerCase().includes(q))),
    );
  }, [receipts, filter, query]);

  if (receipts.length === 0) {
    return (
      <Empty>
        No receipts loaded. Run <code className="tnum">manhattan bench</code>, or start a run from
        the header.
      </Empty>
    );
  }

  const posted = counts["VERIFIED"] ?? 0;

  return (
    <div className="space-y-4">
      {/* Counters */}
      <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
        <Stat
          label="auto-posted"
          value={num(posted)}
          sub={`${pct(posted / Math.max(receipts.length, 1))} of ${num(receipts.length)} settlements`}
          tone="var(--color-verified)"
          emphasis
        />
        <Stat
          label="auto-posted wrong"
          value={summary ? num(summary.auto_posted_wrong) : "0"}
          sub={
            summary
              ? `B0, on the same inputs: ${num(summary.b0_auto_posted_wrong)}`
              : "against ground truth the pipeline never saw"
          }
          tone={summary && summary.auto_posted_wrong > 0 ? "var(--color-wrong)" : "var(--color-verified)"}
          emphasis
        />
        <Stat
          label="held for review"
          value={num(receipts.length - posted)}
          sub="each with a named cause and a computed cure"
        />
        <Stat
          label="throughput"
          value={summary ? num(Math.round(summary.settlements_per_hour)) : "—"}
          sub={
            summary
              ? `${summary.median_latency_ms.toFixed(1)} ms median, ${summary.p95_latency_ms.toFixed(0)} ms p95`
              : "settlements per hour"
          }
        />
      </div>

      {streaming && progress && (
        <div className="rounded border border-line bg-surface px-4 py-3">
          <div className="flex items-baseline justify-between">
            <span className="text-[12.5px] text-ink-dim">reconciling</span>
            <span className="tnum text-[12.5px] text-ink-faint">
              {progress.done} / {progress.total}
            </span>
          </div>
          <div className="mt-2 h-1 w-full overflow-hidden rounded-[2px] bg-raised">
            <div
              className="h-full transition-[width] duration-200"
              style={{
                width: `${(progress.done / Math.max(progress.total, 1)) * 100}%`,
                background: "var(--color-accent)",
              }}
            />
          </div>
        </div>
      )}

      {/* Status mix */}
      <Panel
        title="The five outcomes"
        subtitle="Four of them stop the money, and they are not degrees of failure. They are different findings that call for different actions."
      >
        <Bar
          segments={STATUSES.map((s) => ({
            value: counts[s] ?? 0,
            color: statusColor(s),
            label: `${s}: ${counts[s] ?? 0}`,
          }))}
          height={8}
        />
        <div className="mt-4 grid gap-3 md:grid-cols-2 xl:grid-cols-3">
          {STATUSES.map((s) => (
            <button
              key={s}
              onClick={() => setFilter(filter === s ? "ALL" : s)}
              className={cls(
                "rounded border px-3.5 py-3 text-left transition-colors",
                filter === s ? "border-accent bg-raised" : "border-line hover:bg-raised/60",
              )}
            >
              <div className="flex items-baseline justify-between gap-2">
                <StatusPill status={s} size="sm" />
                <span className="tnum text-[15px]" style={{ color: statusColor(s) }}>
                  {counts[s] ?? 0}
                </span>
              </div>
              <p className="mt-1.5 text-[11.5px] leading-snug text-ink-faint">{statusMeaning(s)}</p>
            </button>
          ))}
        </div>
      </Panel>

      {/* Receipts */}
      <Panel
        title="Receipts"
        subtitle="Every decision, with the evidence behind it. Click a row."
        right={
          <input
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder="filter by reference, merchant or flag"
            className="tnum w-64 rounded border border-line bg-ground px-2.5 py-1.5 text-[12px] text-ink placeholder:text-ink-faint focus:border-accent focus:outline-none"
          />
        }
      >
        <div className="max-h-[560px] overflow-auto">
          <table className="w-full">
            <thead className="sticky top-0 bg-surface">
              <tr>
                <Th>settlement</Th>
                <Th>merchant</Th>
                <Th right>credit</Th>
                <Th right>pool</Th>
                <Th right>index</Th>
                <Th>status</Th>
                <Th>flags</Th>
              </tr>
            </thead>
            <tbody>
              {shown.map((r, i) => (
                <tr
                  key={r.settlement_ref}
                  onClick={() => onOpen(r)}
                  className={cls("cursor-pointer hover:bg-raised", i >= receipts.length - 3 && streaming && "arrive")}
                >
                  <Td mono className="text-ink-dim">
                    {r.settlement_ref.replace("bank_credit_", "")}
                  </Td>
                  <Td className="text-ink-faint">{r.merchant_archetype}</Td>
                  <Td right mono>
                    {rupeesShort(r.target_paise)}
                  </Td>
                  <Td right mono className="text-ink-faint">
                    {r.pool.n}
                  </Td>
                  <Td right mono className="text-ink-faint">
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
        </div>
        {shown.length === 0 && <div className="py-8 text-center text-[12.5px] text-ink-faint">nothing matches</div>}
      </Panel>

      {/* Segmentation */}
      {summary && summary.by_archetype?.length > 0 && (
        <Panel
          title="Which merchants this works for, and why"
          subtitle="The auto-post rate is predictable from the amount distribution alone, before any integration. That is only possible because the system knows exactly what makes it fail."
        >
          <table className="w-full">
            <thead>
              <tr>
                <Th>merchant type</Th>
                <Th>expected regime</Th>
                <Th right>spread σ</Th>
                <Th right>twin mass</Th>
                <Th right>auto-post</Th>
                <Th right>wrong</Th>
                <Th right>B0 wrong</Th>
              </tr>
            </thead>
            <tbody>
              {summary.by_archetype.map((a) => (
                <tr key={a.archetype}>
                  <Td className="text-ink">{a.archetype.replace(/_/g, " ")}</Td>
                  <Td className="text-ink-faint">{a.expected_regime}</Td>
                  <Td right mono className="text-ink-faint">
                    {rupeesShort(Math.round(a.mean_sigma_paise))}
                  </Td>
                  <Td right mono className="text-ink-faint">
                    {a.mean_twin_mass.toFixed(2)}
                  </Td>
                  <Td right mono>
                    <span style={{ color: a.auto_post_rate > 0 ? "var(--color-verified)" : "var(--color-underdetermined)" }}>
                      {pct(a.auto_post_rate)}
                    </span>
                  </Td>
                  <Td right mono>
                    <span style={{ color: a.auto_posted_wrong > 0 ? "var(--color-wrong)" : "var(--color-verified)" }}>
                      {a.auto_posted_wrong}
                    </span>
                  </Td>
                  <Td right mono>
                    <span style={{ color: a.b0_wrong_post_rate > 0.2 ? "var(--color-wrong)" : "var(--color-ink-faint)" }}>
                      {pct(a.b0_wrong_post_rate)}
                    </span>
                  </Td>
                </tr>
              ))}
            </tbody>
          </table>
          <div className="mt-3">
            <Note tone="var(--color-accent)">
              Read the last two columns against the two before them. Where amounts genuinely do not
              distinguish transactions, Manhattan's auto-post rate falls to zero and B0's
              wrong-posting rate climbs. Both systems are looking at the same data. One of them is
              reacting to it.
            </Note>
          </div>
        </Panel>
      )}

      {/* Cost */}
      {summary && (
        <Panel
          title="What it costs to be right"
          subtitle="Accuracy that costs ten times as much per settlement is not obviously a win, so the comparison includes the bill."
        >
          <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
            <Field
              label="model calls per settlement"
              value={(summary.model_calls / Math.max(summary.settlements, 1)).toFixed(2)}
              hint={`${summary.parse_calls} parses, ${summary.agent_calls} agent calls`}
            />
            <Field
              label="exception rate"
              value={pct(summary.exception_rate, 1)}
              hint="only these reach the expensive agent loop"
            />
            <Field
              label="input tokens per 1k"
              value={`${(summary.input_tokens_per_1k / 1e6).toFixed(2)}M`}
              hint={`B0: ${(summary.b0_input_tokens_per_1k / 1e6).toFixed(2)}M`}
            />
            <Field
              label="cost per 1k settlements"
              value={`₹${summary.inr_per_1k_settlements.toFixed(2)}`}
              hint={`B0: ₹${summary.b0_inr_per_1k_settlements.toFixed(2)}`}
              tone="var(--color-accent)"
            />
          </div>
          <p className="mt-3 text-[12px] leading-relaxed text-ink-faint">
            Both are priced at <span className="tnum text-ink-dim">{summary.priced_at_model}</span>{" "}
            rates
            {summary.price_is_real_spend
              ? ""
              : ", modelled rather than billed, because this run used the offline provider"}
            . B0's input scales with pool size, because a matcher that reasons over the candidate
            pool has to put the pool in the context window. Manhattan's parse call reads one line of
            bank narration whatever the pool size, and its expensive loop runs only on exceptions.
          </p>
        </Panel>
      )}

      {/* Run-level gate */}
      {summary?.narrowing_drift && summary.narrowing_drift.length > 0 && (
        <Panel title="This run is gated">
          {summary.narrowing_drift.map((d) => (
            <div key={d.constraint}>
              <p className="text-[13px] leading-relaxed text-ink">
                The narrowing constraint{" "}
                <span className="tnum" style={{ color: "var(--color-sensitive)" }}>
                  {d.constraint}
                </span>{" "}
                dropped {pct(d.drop_rate_observed, 1)} of the record universe, against a stored
                baseline of {pct(d.drop_rate_baseline, 1)} from{" "}
                <span className="tnum">{d.baseline_source}</span>.
              </p>
              <p className="mt-2 text-[12px] leading-relaxed text-ink-faint">
                This is a property of the batch rather than of any one settlement, so it lives on the
                run object and holds the whole batch. Putting it on receipts would invite an analyst
                to clear it settlement by settlement, which is exactly the wrong response. {d.note}
              </p>
            </div>
          ))}
        </Panel>
      )}

      <p className="pb-2 text-center text-[11.5px] text-ink-faint">
        Every amount above is an integer count of paise. There is no floating point anywhere in the
        verification path; {summary ? `run ${summary.run_id}, ` : ""}
        {receipts[0] && `seed ${receipts[0].replay_seed}`}, and the same seed reproduces the same
        receipts byte for byte.
      </p>
      <div className="hidden">{rupees(0)}</div>
    </div>
  );
}
