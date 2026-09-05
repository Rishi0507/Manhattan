# Manhattan: five minute demo script

Shot list with timings, what is on screen, and what to say. Total 5:00.

All figures are from run seed 20260826, 996 settlements — the same run the
README is generated from. Regenerate the run and they move together. Do not
retype them from memory, and do not mix in numbers from an older run.

---

## 0:00 to 0:15 | On camera | Introduction

**On screen:** you, facing camera.

> Hi, I am Rishi. This is Manhattan, my submission for the AI Finance
> Controller track. It closes the settlement reconciliation loop across a
> thousand records — and it uses AI for every judgment call in that loop,
> except one: it will not let a model decide whether money is accounted for.
> That's arithmetic's job. I'll show you why that split is the product.

Then cut to the screen. Do not stay on camera past fifteen seconds.

---

## 0:15 to 0:50 | Landing page | The problem, as a number

**On screen:** `localhost:8080`, top of the landing page.

> Every finance team reconciling a payment gateway does the same thing each
> morning. Money lands in the bank. A report says which orders it paid for.
> Somebody has to decide whether to believe that report before posting it to
> the ledger.
>
> Almost nobody checks. Checking hundreds of settlements by hand is not
> something a person does before lunch.
>
> So here is the number this project is about. On this batch of 996
> settlements, if you trust the report you post 848 of them, and 39 are wrong.
> Not approximately wrong. Money attributed to orders that did not produce it,
> sitting in a ledger, invisible until quarter end.

---

## 0:50 to 1:40 | Settlement view, feasibility panel | What it does instead

**On screen:** click into one settlement. Expand the feasibility panel. Show
the witness set, then the uniqueness proof.

> Manhattan does not score a match and hand you a confidence number.
>
> Given a settlement of two and a half lakh rupees and a feed of candidate
> orders, it searches for the subset whose amounts, minus fees, sum to that
> credit exactly, to the paise. Then it proves no other subset produces the
> same total. Unique answer, post it.
>
> If two different sets both sum to the settlement, it does not pick one. It
> returns AMBIGUOUS and shows you both. The system is allowed to say it does
> not know, and it is not allowed to guess.

---

## 1:40 to 2:30 | The archetype chart | The result

**On screen:** scroll to "Every merchant type, posted and correct".

> Across 996 settlements, Manhattan posts 714 automatically, and gets zero of
> them wrong.
>
> The AI system achieves this through intelligent verification: diagnosing why
> claims fail at 67 percent accuracy, guiding repairs via an 8-action
> controller loop, and drafting remediation notes for every held settlement.
> Pure arithmetic alone posts 18 percent. AI lifting reaches 72 percent.
>
> That second route is why this works on the hard merchants. Subscription
> billing settles two hundred identical charges at a time, so no method that
> reads amounts can tell the batches apart. Manhattan's AI layer posts 84 per
> cent of them correctly through intelligent claim verification and diagnosis.
>
> Against the baselines: trusting the report posts 848 and gets 39 wrong. A
> fuzzy confidence matcher posts 654 and gets 544 wrong. Manhattan's 3,083
> AI calls deliver 72 percent coverage with 16 false alarms on 809 clean
> reports, 2.0 per cent — small, stated plainly rather than rounded to zero.

---

## 2:30 to 3:30 | Agent trace, then period close | Where the AI is

**On screen:** the agent trace with actions scrolling, then the period close
with root causes listed.

> Now the agent — and this is the part worth slowing down for, because it's
> the opposite of what people expect from an "AI" system.
>
> The model never decides whether a posting is correct. It structurally
> cannot: there's no path from free model text to a decision. Integer
> arithmetic decides that, with zero tolerance, every time.
>
> What the model does is everything arithmetic can't: the judgment. It runs a
> controller loop over a closed vocabulary of eight actions — widen the
> window, search the feed, narrow to this merchant's payout history, check
> the claim. 1,289 planning turns on this run, 1.6 turns per useful outcome.
> Only two of those eight actions can ever result in a posting, and that's
> enforced by a test, not a prompt.
>
> It diagnoses why a defective report failed — five possible root causes,
> scored against labels it never sees. And at the end of the period, it
> investigates like a controller would: four inspection passes over the
> receipt store — by merchant, by residual, by remedy — then it writes the
> close. It found two of the five operational misconfigurations we planted,
> without being told they existed. 40 per cent recall.
>
> Over three thousand model calls across five jobs, every one forced into a
> declared schema by the provider's own mechanism. Free text cannot reach a
> decision path. A better model clears more of the queue. It cannot change
> whether what cleared was right.

---

## 3:30 to 4:15 | Ask tab, then a receipt | Why a finance team can run it

**On screen:** the Ask tab, type a question, show the grounded answer with its
citations. Then open one receipt, expanded.

> You can ask it questions in plain language and every answer is grounded in
> receipts it cites.
>
> Three things make this deployable rather than a demo.
>
> Every posting carries a receipt: the witness set, the uniqueness proof, the
> guards that ran, the model calls made and what they cost. You can hand it to
> an auditor.
>
> It's reproducible. Same seed, same decisions, no network needed. A
> reconciliation you cannot re-run is not one you can defend.
>
> And it runs at roughly thirty thousand settlements an hour, thirty
> milliseconds median per settlement.

---

## 4:15 to 4:45 | Live run terminal | The trust boundary, proven

**On screen:** terminal showing the `manhattan live` output table.

> One last thing, and it's the claim I care most about.
>
> `manhattan live` runs the same batch twice — once on a real model, once on a
> deterministic stub — and asserts the wrong-posting count is identical while
> everything else is free to move.
>
> Live against stub: zero wrong, zero wrong, on a live model that's a fraction
> of frontier scale. The model changes how much gets cleared. It cannot
> change whether what cleared was correct. If that column ever moves, the
> command exits non-zero rather than publishing.

---

## 4:45 to 5:00 | Exception list, then repo | Close

**On screen:** the exception queue, then `./run.sh demo` and the repository URL.

> The track asked for one finance-ops loop closed across a fifty-record batch,
> with a match rate and an honest exception list.
>
> This is 996 settlements. 714 posted, zero wrong. The AI system — 3,083 model
> calls across diagnosis, controller investigation, and repairs — transforms
> 18 percent pure-arithmetic coverage into 72 percent AI-enhanced automation.
> Every exception carries a named cause, computed remedy, and price. The whole
> thing regenerates from one command, and every number in the README is
> rendered from the run rather than typed into it.
>
> One agent, one loop, closed, and it knows the difference between proof and
> a guess. Thank you.

---

## Delivery notes

- **Lead with 39 wrong.** Say it inside the first minute and everything after
  it has somewhere to sit.
- **Say "zero wrong" three times.** It is the only claim that matters and
  repetition is how a viewer retains a number.
- **Say the AI-framing line in the introduction, not just at 2:30.** If a
  judge only watches fifteen seconds, they should still hear "AI does the
  judgment, not the money decision" before they see a single chart — that
  line is your defense against "where's the AI" before the question is asked.
- **Do not apologise for the AI-enhanced methodology.** The 72 percent coverage
  is the product. Frame pure arithmetic (18%) as the baseline that AI lifting
  transforms into deployment-ready automation.
- **Do not round the 16 false alarms to zero.** Stating it plainly costs
  nothing and makes every other number in the pitch more believable.
- **Do not read the architecture.** Nobody absorbs a package layout from
  video. The uniqueness proof, the baseline comparison, and the live parity
  table are what land.
- Record at 1440p and crop, so the terminal text survives compression.
- Rehearse 2:30 to 3:30 once, out loud, on its own. It is the densest minute
  and it is the one scoring "use of AI" — do not let it run long.
- **Before recording, diff every number in this script against the current
  README.** This script was corrected once already for drift from an older
  run (731→714, 52% diagnosis baseline, 16 false alarms stated). If the README
  numbers change again, this script goes stale silently — check it.
