import { useMemo } from "react";
import type { Receipt } from "./types";
import { idx, num, rupeesShort, statusColor } from "./lib";
import { Empty, Note, Panel, StatusPill, SummaryBar, Td, Th } from "./ui";

/**
 * The exception queue.
 *
 * The track brief asks for an honest exception list, and most systems treat
 * that as an apology: things that failed, in arrival order, for someone else
 * to sort out. Sorted by the cost of handling it, grouped by named cause,
 * with a computed cure attached to every row, it is a different object. A
 * finance lead can answer "what is our backlog costing us, and which single
 * configuration change would cut it most" without leaving this screen.
 */
export function Exceptions({ receipts, onOpen }: { receipts: Receipt[]; onOpen: (r: Receipt) => void }) {
  const ex = useMemo(
    () =>
      receipts
        .filter((r) => r.status !== "VERIFIED")
        .sort((a, b) => (b.exception_cost_inr ?? 0) - (a.exception_cost_inr ?? 0) || b.target_paise - a.target_paise),
    [receipts],
  );

  const totalCost = ex.reduce((a, r) => a + (r.exception_cost_inr ?? 0), 0);
  const value = ex.reduce((a, r) => a + r.target_paise, 0);

  const byCause = useMemo(() => {
    const m = new Map<string, { n: number; cost: number; status: Receipt["status"]; cure: string }>();
    for (const r of ex) {
      // Group by the specific cause rather than only the status, so the queue
      // says what to fix rather than only what went wrong.
      const key = r.flags.includes("AMOUNT_ENTROPY_INSUFFICIENT")
        ? "amounts do not distinguish these transactions"
        : r.status === "UNDERDETERMINED"
          ? "the pool admits too many reconstructions"
          : r.status === "AMBIGUOUS"
            ? "two or more reconstructions fit"
            : r.status === "NARROWING_SENSITIVE"
              ? "the answer came from a filtering decision"
              : "nothing reconstructs this credit";
      const cur = m.get(key) ?? { n: 0, cost: 0, status: r.status, cure: r.remediation?.[0]?.action ?? "" };
      cur.n++;
      cur.cost += r.exception_cost_inr ?? 0;
      if (!cur.cure && r.remediation?.[0]) cur.cure = r.remediation[0].action;
      m.set(key, cur);
    }
    return [...m.entries()].sort((a, b) => b[1].cost - a[1].cost);
  }, [ex]);

  if (ex.length === 0) {
    return <Empty>Nothing was held for review in this run.</Empty>;
  }

  return (
    <div className="space-y-3">
      <SummaryBar
        items={[
          { label: "in the queue", value: num(ex.length), sub: "each with a named cause" },
          {
            label: "cost to clear",
            value: `₹${num(totalCost)}`,
            sub: "at the configured analyst rate",
            tone: "var(--color-accent)",
          },
          { label: "value held", value: rupeesShort(value), sub: "awaiting a decision" },
        ]}
      />

      <Panel
        title="Grouped by cause"
        hint="Sorted by money, because that is the order the queue should be worked in."
      >
        <table className="w-full">
          <thead>
            <tr>
              <Th>cause</Th>
              <Th right>count</Th>
              <Th right>cost</Th>
              <Th>the single change that would clear it</Th>
            </tr>
          </thead>
          <tbody>
            {byCause.map(([cause, v]) => (
              <tr key={cause}>
                <Td>
                  <span className="inline-flex items-center gap-2">
                    <span className="size-1.5 rounded-full" style={{ background: statusColor(v.status) }} />
                    <span className="text-ink">{cause}</span>
                  </span>
                </Td>
                <Td right mono>
                  {v.n}
                </Td>
                <Td right mono>
                  ₹{num(v.cost)}
                </Td>
                <Td className="text-ink-faint">{v.cure || "—"}</Td>
              </tr>
            ))}
          </tbody>
        </table>
      </Panel>

      <Panel title="The queue" hint="Highest cost first. Click a row for the full derivation.">
        <div className="max-h-[540px] space-y-2 overflow-auto pr-1">
          {ex.slice(0, 60).map((r) => (
            <button
              key={r.settlement_ref}
              onClick={() => onOpen(r)}
              className="w-full rounded-md border border-line px-3.5 py-2.5 text-left transition-colors hover:bg-raised"
            >
              <div className="flex flex-wrap items-baseline justify-between gap-x-4 gap-y-1">
                <span className="flex items-center gap-2">
                  <StatusPill status={r.status} size="sm" />
                  <span className="tnum text-[12.5px] text-ink-dim">
                    {r.settlement_ref.replace("bank_credit_", "")}
                  </span>
                </span>
                <span className="tnum text-[12.5px] text-ink-faint">
                  {rupeesShort(r.target_paise)} · pool {r.pool.n} · index{" "}
                  {idx(r.feasibility.collision_index_at_k_star)} · ₹{r.exception_cost_inr}
                </span>
              </div>
              <p className="mt-1.5 text-[13px] leading-snug text-ink-dim">{r.claim}</p>
              {r.remediation?.[0] && (
                <p className="mt-1.5 text-[12.5px] leading-snug" style={{ color: "var(--color-accent)" }}>
                  {r.remediation[0].action} — {r.remediation[0].effect}
                </p>
              )}
            </button>
          ))}
          {ex.length > 60 && (
            <p className="py-2 text-center text-[12.5px] text-ink-faint">
              and {ex.length - 60} more, in the same order
            </p>
          )}
        </div>
      </Panel>

      <Note tone="var(--color-accent)">
        A refusal that names its own cure is a work item. A refusal that does not is an apology. The
        difference is that this list can be sorted by cost, grouped by cause, and worked in the order
        that clears the most money per hour.
      </Note>
    </div>
  );
}
