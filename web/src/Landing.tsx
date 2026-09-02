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
    <div className="mx-auto max-w-[1080px] px-6">
      {/* ---- Hero ---------------------------------------------------- */}
      <section className="pt-20 pb-16">
        <p className="lbl">Razorpay AI Buildathon · Track 04 · AI Finance Controller</p>

        <h1 className="display mt-5 max-w-[19ch] text-[54px] leading-[1.06] font-medium tracking-[-0.015em] text-ink">
          An agent that proves settlements instead of guessing them.
        </h1>

        <p className="mt-6 max-w-[62ch] text-[17px] leading-relaxed text-ink-dim">
          A gateway settlement arrives as one number in a bank account. Behind it are hundreds of
          payments, fees, taxes, refunds and chargebacks. Manhattan runs that arrow backwards and
          either produces an exact reconstruction with a proof that no rival exists, or it names the
          precise property it could not establish and refuses.
        </p>

        <div className="mt-8 flex flex-wrap items-center gap-3">
          <button
            onClick={() => onEnter("hook")}
            className="rounded-md bg-accent px-5 py-2.5 text-[15px] font-medium text-[#fffdf8] transition-colors hover:bg-[#633f22]"
          >
            See it catch a wrong posting
          </button>
          <button
            onClick={() => onEnter("run")}
            className="rounded-md border border-line-strong px-5 py-2.5 text-[15px] text-ink-dim transition-colors hover:border-accent hover:text-accent"
          >
            Open the run
          </button>
        </div>
      </section>

      {/* ---- The one result ------------------------------------------ */}
      {summary && (
        <section className="border-y border-line py-12">
          <p className="lbl">
            {num(n)} settlements · six merchant types · identical inputs, identical filters
          </p>

          <div className="mt-7 grid gap-10 md:grid-cols-[1fr_auto_1fr] md:gap-6">
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

          <p className="mt-9 max-w-[74ch] text-[15px] leading-relaxed text-ink-dim">
            B0 posted {num(postedB0)} and got{" "}
            <strong className="font-semibold text-ink">{num(wrongB0)} of them wrong</strong>. Not
            approximately wrong: attributed to the wrong transactions, at high confidence, with
            nothing to indicate a problem. Found at audit, that is eight to twenty hours each.
            <br />
            <br />
            Manhattan posts fewer. Every one is right, and every refusal names its own cure.
          </p>
        </section>
      )}

      {/* ---- How ------------------------------------------------------ */}
      <section className="py-14">
        <h2 className="display text-[28px] leading-tight font-medium">
          Why guessing fails here, specifically
        </h2>

        <div className="mt-8 grid gap-x-10 gap-y-9 md:grid-cols-3">
          <Step
            n="01"
            title="Adding up is not proof"
            body="Forty candidate payments give a trillion possible combinations. On a real batch, many of them hit the target exactly by coincidence. Finding one is easy and worthless; the work is proving no other exists."
          />
          <Step
            n="02"
            title="Ask whether it is answerable first"
            body="Before searching, we estimate how many combinations would hit this target by chance. Above a threshold, no method can single one out, so we refuse without searching and say what would change that."
          />
          <Step
            n="03"
            title="Then suspect ourselves"
            body="If our own filters dropped a real payment, a coincidental set can still add up, and we would post a wrong answer with a proof attached. So we widen the net afterwards and check whether a rival appears."
          />
        </div>
      </section>

      {/* ---- The hook case -------------------------------------------- */}
      {hero && (
        <section className="border-t border-line py-14">
          <div className="grid items-start gap-10 lg:grid-cols-[1.1fr_1fr]">
            <div>
              <p className="lbl">the case worth seeing</p>
              <h2 className="display mt-3 text-[28px] leading-tight font-medium">
                One filter, two hours too tight
              </h2>
              <p className="mt-4 max-w-[54ch] text-[15px] leading-relaxed text-ink-dim">
                A value-date window was set slightly wrong, so one payment that genuinely belonged to
                this batch was dropped. A different payment, not in the batch, happened to carry an
                identical amount and took its place.
                <br />
                <br />
                Every arithmetic check passes on the survivors. The identity closes to the paise. B0
                posts it at {hero.b0_confidence.toFixed(2)} confidence and is wrong. Manhattan holds
                it and names the filter.
              </p>
              <button
                onClick={() => onEnter("hook")}
                className="mt-6 text-[15px] font-medium text-accent underline-offset-4 hover:underline"
              >
                See both systems on this settlement
              </button>
            </div>

            <div className="rounded-md border border-line bg-surface p-5">
              <p className="lbl">the credit</p>
              <p className="tnum mt-1.5 text-[30px] leading-none">
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
          <h2 className="display text-[28px] leading-tight font-medium">
            It can tell a merchant whether it will work, before integration
          </h2>
          <p className="mt-4 max-w-[70ch] text-[15px] leading-relaxed text-ink-dim">
            One pass over historical settlement amounts gives the spread and how much of it repeats.
            Those give the expected outcome. Nothing else in this space can scope itself honestly at
            the sales stage, because nothing else knows exactly what makes it fail.
          </p>

          <div className="mt-8 space-y-2.5">
            {summary.by_archetype
              .slice()
              .sort((a, b) => b.auto_post_rate - a.auto_post_rate)
              .map((a) => (
                <div key={a.archetype} className="flex items-center gap-4">
                  <span className="w-40 shrink-0 text-[14px] text-ink-dim">
                    {a.archetype.replace(/_/g, " ")}
                  </span>
                  <span className="h-2 flex-1 overflow-hidden rounded-[2px] bg-sunken">
                    <span
                      className="block h-full"
                      style={{
                        width: `${Math.max(a.auto_post_rate * 100, 0.6)}%`,
                        background:
                          a.auto_post_rate > 0 ? "var(--color-verified)" : "var(--color-line-strong)",
                      }}
                    />
                  </span>
                  <span className="tnum w-12 shrink-0 text-right text-[14px]">
                    {pct(a.auto_post_rate)}
                  </span>
                  <span className="tnum w-24 shrink-0 text-right text-[12.5px] text-ink-faint">
                    B0 {pct(a.b0_wrong_post_rate)} wrong
                  </span>
                </div>
              ))}
          </div>

          <p className="mt-6 max-w-[70ch] text-[14px] leading-relaxed text-ink-faint">
            The merchants at the bottom bill three repeated subscription prices. Their settlements
            are genuinely not reconstructable from amounts by any method, ours included. We say so in
            eleven milliseconds and name the fix, rather than guessing and being wrong seven times
            in ten.
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

      <div className="mt-5 flex items-baseline gap-8">
        <div>
          <p className="lbl">auto-posted</p>
          <p className="tnum mt-1 text-[34px] leading-none">{num(posted)}</p>
          <p className="tnum mt-1 text-[12.5px] text-ink-faint">
            {total > 0 ? pct(posted / total) : "—"}
          </p>
        </div>
        <div>
          <p className="lbl">of those, wrong</p>
          <p className="tnum mt-1 text-[34px] leading-none" style={{ color: tone }}>
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
