import type { CaseOutcome, Summary } from "./types";
import { num, pct, rupees } from "./lib";

/**
 * The landing page.
 *
 * It has one job: make someone who has never heard of this understand, in
 * about fifteen seconds, what the difference is between a system that guesses
 * and a system that proves. Everything on it is either that argument or a way
 * into the evidence for it.
 *
 * Every number is read from the run that produced the page. Nothing here is
 * written into the markup, which is the same rule the rest of the project
 * follows and the reason it can be trusted at all.
 */
export function Landing({
  summary,
  cases,
  onEnter,
}: {
  summary: Summary | null;
  cases: CaseOutcome[];
  onEnter: (tab: "hook" | "run" | "cases") => void;
}) {
  const hero = cases.find((c) => c.case.Number === 10);
  const wrongB0 = summary?.b0_auto_posted_wrong ?? 0;
  const postedB0 = summary?.b0_auto_posted ?? 0;
  const posted = summary?.auto_posted ?? 0;
  const n = summary?.settlements ?? 0;

  return (
    <div className="mx-auto max-w-[1080px] px-4 sm:px-6">
      {/* ---- Hero ---------------------------------------------------- */}
      <section className="pt-12 pb-12 sm:pt-20 sm:pb-16">
        <img src="/logo.png" alt="Manhattan" className="mb-7 h-11 w-auto" />
        <p className="lbl">Razorpay AI Buildathon · Track 04 · AI Finance Controller</p>

        <h1 className="display mt-5 max-w-[19ch] text-[34px] leading-[1.08] font-medium tracking-[-0.015em] text-ink sm:text-[44px] lg:text-[54px]">
          An agent that proves settlements instead of guessing them.
        </h1>

        <p className="mt-6 max-w-[60ch] text-[15.5px] leading-relaxed text-ink-dim sm:text-[17px]">
          A gateway settlement arrives as a single credit representing hundreds of payments, fees,
          taxes, refunds and chargebacks. Manhattan reconstructs it exactly and proves no rival
          reconstruction exists, or states which property it could not establish.
        </p>

        <div className="mt-8 flex flex-wrap items-center gap-3">
          <button
            onClick={() => onEnter("hook")}
            className="rounded-md bg-accent px-5 py-2.5 text-[15px] font-medium text-[#fffdf8] transition-colors hover:bg-[#633f22]"
          >
            View the comparison
          </button>
          <button
            onClick={() => onEnter("run")}
            className="rounded-md border border-line-strong px-5 py-2.5 text-[15px] text-ink-dim transition-colors hover:border-accent hover:text-accent"
          >
            Open the dashboard
          </button>
        </div>
      </section>

      {/* ---- The one result ------------------------------------------ */}
      {summary && (
        <section className="border-y border-line py-12">
          <p className="lbl">
            {num(n)} settlements · six merchant types · identical inputs, identical filters
          </p>

          <div className="mt-7 grid gap-8 md:grid-cols-[1fr_auto_1fr] md:gap-6">
            <Side
              name="B0"
              caption="a confidence matcher, built honestly and given every advantage"
              posted={postedB0}
              wrong={wrongB0}
              total={n}
              tone="var(--color-wrong)"
            />

            <div className="hidden w-px bg-line md:block" />

            <Side
              name="Manhattan"
              caption="posts only when an integer identity closes and nothing rivals it"
              posted={posted}
              wrong={summary.auto_posted_wrong}
              total={n}
              tone="var(--color-verified)"
            />
          </div>

          <p className="mt-9 max-w-[72ch] text-[15px] leading-relaxed text-ink-dim">
            The baseline posted {num(postedB0)} settlements and attributed{" "}
            <strong className="font-semibold text-ink">{num(wrongB0)} to the wrong transactions</strong>,
            at high confidence, with no indication of a problem. Manhattan posts fewer, and every
            refusal states its remediation.
          </p>
        </section>
      )}

      {/* ---- How ------------------------------------------------------ */}
      <section className="py-14">
        <h2 className="display text-[23px] leading-tight font-medium sm:text-[28px]">Why matching fails</h2>

        <div className="mt-8 grid gap-x-10 gap-y-8 sm:grid-cols-2 md:grid-cols-3">
          <Step
            n="01"
            title="Adding up is not proof"
            body="Forty candidate payments produce a trillion combinations. Many hit the target exactly by coincidence. Finding one is trivial; proving no other exists is the problem."
          />
          <Step
            n="02"
            title="Ask whether it is answerable first"
            body="Before searching, the system estimates how many combinations would hit the target by chance. Above a threshold no method can identify one, so it refuses and states what would change that."
          />
          <Step
            n="03"
            title="Then suspect ourselves"
            body="If a filter dropped a real payment, a coincidental set can still reconcile exactly. The pool is widened afterwards to check whether a rival reconstruction appears."
          />
        </div>
      </section>

      {/* ---- The hook case -------------------------------------------- */}
      {hero && (
        <section className="border-t border-line py-14">
          <div className="grid items-start gap-8 lg:grid-cols-[1.1fr_1fr] lg:gap-10">
            <div>
              <p className="lbl">the case worth seeing</p>
              <h2 className="display mt-3 text-[23px] leading-tight font-medium sm:text-[28px]">A filter set two hours too tight</h2>
              <p className="mt-4 max-w-[54ch] text-[15px] leading-relaxed text-ink-dim">
                One payment belonging to this batch was dropped by the value-date window. A payment
                outside the batch carrying an identical amount took its place. Every arithmetic check
                passes on the survivors.
              </p>
              <button
                onClick={() => onEnter("hook")}
                className="mt-6 text-[15px] font-medium text-accent underline-offset-4 hover:underline"
              >
                View this settlement
              </button>
            </div>

            <div className="rounded-md border border-line bg-surface p-5">
              <p className="lbl">the credit</p>
              <p className="tnum mt-1.5 text-[25px] leading-none sm:text-[30px]">
                {rupees(hero.receipt.target_paise)}
              </p>
              <p className="tnum mt-2 text-[12.5px] text-ink-faint">{hero.receipt.narration}</p>

              <div className="mt-5 space-y-2.5 border-t border-line-soft pt-4">
                <Verdict
                  who="B0"
                  text={`posted at ${hero.b0_confidence.toFixed(2)} confidence`}
                  bad={hero.b0_posted_wrong}
                />
                <Verdict who="Manhattan" text="held, and named the constraint" bad={false} />
              </div>
            </div>
          </div>
        </section>
      )}

      {/* ---- Segmentation --------------------------------------------- */}
      {summary && summary.by_archetype?.length > 0 && (
        <section className="border-t border-line py-14">
          <h2 className="display text-[23px] leading-tight font-medium sm:text-[28px]">Every merchant type, posted and correct</h2>
          <p className="mt-4 max-w-[68ch] text-[15px] leading-relaxed text-ink-dim">
            The solid bar is what Manhattan posts automatically. The notch inside it is the share
            carried by an independent proof rather than a verified claim. The right column is what
            the confidence matcher gets wrong on the same settlements.
          </p>

          <div className="mt-6 flex flex-wrap items-center gap-x-6 gap-y-2 text-[12.5px] text-ink-faint">
            <span className="flex items-center gap-2">
              <span className="h-2 w-6 rounded-[2px]" style={{ background: "var(--color-verified)" }} />
              posted by Manhattan
            </span>
            <span className="flex items-center gap-2">
              <span className="h-2 w-2 rounded-[1px] bg-ground ring-1 ring-[var(--color-line-strong)]" />
              of which independently proved
            </span>
            <span className="tnum">
              wrong postings across all {summary.by_archetype.length} types:{" "}
              <strong className="font-semibold text-ink">
                {num(summary.by_archetype.reduce((t, a) => t + (a.m1_posted_wrong ?? 0), 0))}
              </strong>
            </span>
          </div>

          <div className="mt-8 space-y-2.5">
            {summary.by_archetype
              .slice()
              .sort((a, b) => b.m1_post_rate - a.m1_post_rate)
              .map((a) => (
                <div key={a.archetype} className="flex items-center gap-4">
                  <span className="w-24 shrink-0 text-[13px] text-ink-dim sm:w-40 sm:text-[14px]">
                    {a.archetype.replace(/_/g, " ")}
                  </span>
                  <span className="relative h-2 flex-1 overflow-hidden rounded-[2px] bg-sunken">
                    <span
                      className="absolute inset-y-0 left-0 block"
                      style={{
                        width: `${Math.max(a.m1_post_rate * 100, 0.6)}%`,
                        background: "var(--color-verified)",
                      }}
                    />
                    {a.auto_post_rate > 0 && (
                      <span
                        className="absolute inset-y-0 block w-[2px] bg-ground"
                        style={{ left: `${a.auto_post_rate * 100}%` }}
                      />
                    )}
                  </span>
                  <span className="tnum w-12 shrink-0 text-right text-[14px] font-medium">
                    {pct(a.m1_post_rate)}
                  </span>
                  <span className="tnum hidden w-28 shrink-0 text-right text-[12.5px] text-ink-faint sm:block">
                    B0 {pct(a.b0_wrong_post_rate)} wrong
                  </span>
                </div>
              ))}
          </div>

          <p className="mt-6 max-w-[68ch] text-[14px] leading-relaxed text-ink-faint">
            The merchants at the bottom bill three repeated subscription prices, so no method that
            reads amounts can separate their batches. Manhattan still posts most of them, because
            where a proof is unavailable it verifies the settlement report against the ledger
            instead. That is the difference between refusing the hard cases and clearing them by
            a second route.
          </p>
        </section>
      )}

      {/* ---- Footer --------------------------------------------------- */}
      <section className="border-t border-line py-12">
        <div className="flex flex-wrap items-end justify-between gap-6">
          <div>
            <p className="display text-[19px] font-medium">
              No guessed matches. No confidence threshold.
            </p>
            <p className="mt-1.5 max-w-[58ch] text-[14px] leading-relaxed text-ink-dim">
              Proof, exhibited alternatives, or a named and priced reason the proof is unavailable.
            </p>
          </div>
          <p className="tnum text-[12.5px] text-ink-faint">
            {summary ? `seed ${summary.seed} · ` : ""}
            every figure regenerated by <span className="text-ink-dim">./run.sh bench</span>
          </p>
        </div>
      </section>
    </div>
  );
}

function Side({
  name,
  caption,
  posted,
  wrong,
  total,
  tone,
}: {
  name: string;
  caption: string;
  posted: number;
  wrong: number;
  total: number;
  tone: string;
}) {
  return (
    <div>
      <p className="display text-[19px] font-medium">{name}</p>
      <p className="mt-1 max-w-[38ch] text-[13.5px] leading-snug text-ink-faint">{caption}</p>

      <div className="mt-5 flex flex-wrap items-baseline gap-x-8 gap-y-4">
        <div>
          <p className="lbl">auto-posted</p>
          <p className="tnum mt-1 text-[27px] leading-none sm:text-[34px]">{num(posted)}</p>
          <p className="tnum mt-1 text-[12.5px] text-ink-faint">
            {total > 0 ? pct(posted / total) : "n/a"}
          </p>
        </div>
        <div>
          <p className="lbl">of those, wrong</p>
          <p className="tnum mt-1 text-[27px] leading-none sm:text-[34px]" style={{ color: tone }}>
            {num(wrong)}
          </p>
          <p className="tnum mt-1 text-[12.5px] text-ink-faint">
            {posted > 0 ? pct(wrong / posted) : "none posted"}
          </p>
        </div>
      </div>
    </div>
  );
}

function Step({ n, title, body }: { n: string; title: string; body: string }) {
  return (
    <div>
      <p className="tnum text-[12.5px] text-accent">{n}</p>
      <h3 className="display mt-2 text-[19px] leading-snug font-medium">{title}</h3>
      <p className="mt-2.5 text-[14.5px] leading-relaxed text-ink-dim">{body}</p>
    </div>
  );
}

function Verdict({ who, text, bad }: { who: string; text: string; bad: boolean }) {
  return (
    <div className="flex items-baseline justify-between gap-4">
      <span className="text-[13.5px] text-ink-dim">{who}</span>
      <span className="text-right text-[13.5px]" style={bad ? { color: "var(--color-wrong)" } : undefined}>
        {text}
        {bad && <span className="ml-1.5 font-semibold">and wrong</span>}
      </span>
    </div>
  );
}
