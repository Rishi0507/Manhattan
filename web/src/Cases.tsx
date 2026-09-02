import type { CaseOutcome, Receipt } from "./types";
import { cls, idx } from "./lib";
import { Empty, Flag, Note, Panel, StatusPill } from "./ui";

/**
 * The eleven adversarial cases.
 *
 * The track brief is explicit that one cherry-picked match proves nothing, so
 * each of these is a specific, nameable way a confidence matcher produces a
 * wrong answer. The expectations are written into the test suite, which means
 * a regression shows up as a failing build rather than as a demo that quietly
 * stopped proving anything.
 */
export function Cases({ cases, onOpen }: { cases: CaseOutcome[]; onOpen: (r: Receipt) => void }) {
  if (cases.length === 0) {
    return (
      <Empty>
        No case results. Run <code className="tnum">manhattan bench</code> or{" "}
        <code className="tnum">manhattan cases</code>.
      </Empty>
    );
  }

  const posted = cases.filter((c) => c.posted).length;
  const wrong = cases.filter((c) => c.posted_wrong).length;
  const b0Posted = cases.filter((c) => c.b0_posted).length;
  const b0Wrong = cases.filter((c) => c.b0_posted_wrong).length;
  const met = cases.filter((c) => c.expectation_met).length;

  return (
    <div className="space-y-3">
      <Panel
        title="Eleven adversarial cases, head to head"
        hint="Both systems see identical inputs and identical narrowing. The only difference is what each is willing to conclude from them."
        right={
          <div className="text-right text-[11px]">
            <div className="tnum text-ink-dim">
              Manhattan posted {posted}, wrong{" "}
              <span style={{ color: wrong ? "var(--color-wrong)" : "var(--color-verified)" }}>{wrong}</span>
            </div>
            <div className="tnum text-ink-faint">
              B0 posted {b0Posted}, wrong{" "}
              <span style={{ color: b0Wrong ? "var(--color-wrong)" : "var(--color-ink-faint)" }}>{b0Wrong}</span>
            </div>
            <div className="tnum mt-0.5 text-ink-faint">
              {met} of {cases.length} expectations met
            </div>
          </div>
        }
      >
        <div className="space-y-2">
          {cases.map((c) => (
            <button
              key={c.case.Number}
              onClick={() => onOpen(c.receipt)}
              className="w-full rounded-md border border-line px-3.5 py-2.5 text-left transition-colors hover:bg-raised"
            >
              <div className="flex flex-wrap items-baseline justify-between gap-x-4 gap-y-1">
                <span className="flex items-baseline gap-2.5">
                  <span className="tnum text-[11px] text-ink-faint">
                    {String(c.case.Number).padStart(2, "0")}
                  </span>
                  <span className="text-[12.5px] text-ink">{c.case.Name.replace(/_/g, " ")}</span>
                </span>
                <span className="tnum text-[11px] text-ink-faint">
                  pool {c.pool_n} · k* {c.k_star} · index {idx(c.collision_index)} · {c.latency_ms} ms
                </span>
              </div>

              <p className="mt-1 text-[12px] leading-snug text-ink-faint">{c.case.Scenario}</p>

              <div className="mt-2.5 grid gap-2 sm:grid-cols-2">
                {/* B0 */}
                <div className="rounded-md border border-line-soft px-3 py-2">
                  <div className="lbl">B0</div>
                  <div
                    className="mt-1 text-[12px]"
                    style={{
                      color: c.b0_posted_wrong
                        ? "var(--color-wrong)"
                        : c.b0_posted
                          ? "var(--color-ink-dim)"
                          : "var(--color-ink-faint)",
                    }}
                  >
                    {c.b0_posted
                      ? `posted at ${c.b0_confidence.toFixed(2)}${c.b0_posted_wrong ? " — the wrong batch" : ""}`
                      : "held, low confidence"}
                  </div>
                </div>

                {/* Manhattan */}
                <div className="rounded-md border border-line-soft px-3 py-2">
                  <div className="lbl">Manhattan</div>
                  <div className="mt-1 flex flex-wrap items-center gap-1.5">
                    <StatusPill status={c.status} size="sm" />
                    {c.flags.map((f) => (
                      <Flag key={f} name={f} />
                    ))}
                  </div>
                </div>
              </div>

              <p
                className={cls(
                  "mt-2 text-[11px] leading-snug",
                  c.expectation_met ? "text-ink-faint" : "text-ink",
                )}
                style={c.posted_wrong ? { color: "var(--color-wrong)" } : undefined}
              >
                {c.posted_wrong ? "AUTO-POSTED THE WRONG BATCH — " : ""}
                {c.case.Why}
              </p>
            </button>
          ))}
        </div>
      </Panel>

      <Note tone="var(--color-accent)">
        Manhattan posts fewer of these than B0 does, and that is the result rather than a
        concession. Every case B0 posts and Manhattan holds is one where a rival reconstruction
        exists, the amounts cannot distinguish the transactions, or the answer came from a filtering
        decision. B0 cannot see any of those, because it never looks.
      </Note>
    </div>
  );
}
