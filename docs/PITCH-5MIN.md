# Manhattan: five minute demo script

Shot list with timings, what is on screen, and what to say. Total 5:00.

All figures are from run seed 20260826, 996 settlements. Regenerate the run and
they move together. Do not retype them from memory.

---

## 0:00 to 0:15 | On camera | Introduction

**On screen:** you, facing camera.

> Hi, I am Rishi. This is Manhattan, my submission for the AI Finance
> Controller track. It is an agent that closes the settlement reconciliation loop
> across a thousand records, and the one thing it will not do is guess.

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
> settlements, if you trust the report you post 848 of them and 39 are wrong.
> Not approximately wrong. Money attributed to orders that did not produce it,
> sitting in a ledger, waiting to be found at quarter end.

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

> Across 996 settlements Manhattan posts 731 automatically, and gets zero of
> them wrong.
>
> Two routes get there. 212 carry an independent proof. The other 519 are
> settlements where the report made a claim and Manhattan verified that claim
> against the ledger before accepting it.
>
> That second route is why this works on the hard merchants. Subscription
> billing settles two hundred identical charges at a time, so no method that
> reads amounts can separate the batches. Proof rate there is zero and always
> will be. Manhattan still posts 84 per cent of them, correctly.
>
> Against the baselines: trusting the report posts 848 and gets 39 wrong.
> Fuzzy confidence matching, which is what most tools ship, posts 671 and gets
> 561 wrong.
>
> And it does not cry wolf. Zero false alarms on clean reports.

---

## 2:30 to 3:30 | Agent trace, then period close | Where the AI is

**On screen:** the agent trace with actions scrolling, then the period close
with root causes listed.

> Now the agent, and it is not where people expect.
>
> The model never decides whether a posting is correct. It cannot. Integer
> arithmetic decides that, with no tolerance.
>
> What the model does is everything else. It runs a controller loop over a
> closed vocabulary of eight actions: widen the window, search the feed, narrow
> to this merchant's payout history, check the claim. 1,233 planning turns on
> this run. Only two of those eight actions can ever result in a posting, and
> that is enforced by a test rather than by a prompt.
>
> It diagnoses defective reports at 76 per cent accuracy against labels it
> never sees.
>
> And at the end of the period it investigates: four inspection passes over the
> receipt store, reading exceptions by merchant, by status, by residual, and
> then it writes the close. It found four of the five operational
> misconfigurations we planted without being told they existed.
>
> Nearly three thousand model calls across five jobs, every one forced into a
> declared schema by the provider's own mechanism. Free text cannot reach a
> decision path.

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
> It is reproducible. Same seed, byte identical output, no network needed. A
> reconciliation you cannot re-run is not one you can defend.
>
> And it runs at thirty one thousand settlements an hour, median twenty six
> milliseconds each.

---

## 4:15 to 4:45 | Live run terminal | The trust boundary, proven

**On screen:** terminal showing the `manhattan live` output table, MUST NOT
MOVE block visible.

> One last thing, and it is the claim I care most about.
>
> `manhattan live` runs the same batch twice, once on a real model and once on
> a deterministic stub, and asserts that the wrong posting count is identical
> while everything else is free to move.
>
> Live against stub: zero wrong, zero wrong. The model changes how much gets
> cleared. It cannot change whether what cleared was correct. If that column
> ever moves, the command exits non zero rather than publishing.

---

## 4:45 to 5:00 | Exception list, then repo | Close

**On screen:** the exception queue, then `./run.sh demo` and the repository URL.

> The track asked for one finance ops loop closed across a fifty record batch,
> with a match rate and an honest exception list.
>
> This is 996 settlements. 731 posted, zero wrong. Every exception carries a
> named cause, a computed remedy and a price. The whole thing regenerates from
> one command, and every number in the README is rendered from the run rather
> than typed into it.
>
> One agent, one loop, closed. Thank you.

---

## Delivery notes

- **Lead with 39 wrong.** Say it inside the first minute and everything after
  it has somewhere to sit.
- **Say "zero wrong" three times.** It is the only claim that matters and
  repetition is how a viewer retains a number.
- **Do not apologise for the 212 proof count.** The composite is the product.
  Framing the proof rate as a shortfall invites a question the composite
  already answers.
- **Do not read the architecture.** Nobody absorbs a package layout from video.
  The uniqueness proof, the baseline comparison and the live parity table are
  what land.
- Record at 1440p and crop, so the terminal text survives compression.
- Rehearse the 2:30 to 3:30 minute once. It is the densest and it is the one
  scoring "use of AI".
