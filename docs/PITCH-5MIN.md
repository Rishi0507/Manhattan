# Manhattan: a five minute pitch

A script for the demo video. Roughly 700 words of speech at a normal pace, plus
the pauses where something is happening on screen. Six beats, timed.

Numbers below are the ones this repository generates. Regenerate the run and
they change together; do not retype them from memory.

---

## 0:00 to 0:35, the problem, stated as a number

> Every finance team reconciling a payment gateway does the same thing every
> morning. A settlement lands in the bank. A report says which orders it paid
> for. Somebody checks that the report is telling the truth, and then posts it
> to the ledger.
>
> The check is the whole job, and almost nobody does it. They trust the report,
> because checking four hundred settlements by hand is not a thing a person does
> before lunch.
>
> So here is the number that this project is about. If you trust the report on
> this batch, you post 428 settlements and 20 of them are wrong. Not
> approximately wrong. Wrong: money attributed to orders that did not produce
> it, sitting in a ledger, waiting for somebody to find it at quarter end.

**On screen:** the landing page, then the Run tab with the batch loaded.

---

## 0:35 to 1:30, what Manhattan does instead

> Manhattan is an agent that closes the settlement reconciliation loop and
> refuses to post anything it cannot prove.
>
> Give it a settlement of eighteen lakh rupees and a feed of candidate orders.
> It does not score a match and give you a confidence number. It searches for
> the subset of orders whose amounts, minus fees, sum to that settlement exactly,
> to the paise, and then it proves that no other subset produces the same total.
> Unique subset, one answer, post it.
>
> If two different sets of orders both sum to the settlement, it does not pick
> one. It says AMBIGUOUS and shows you both. That is the entire design: the
> system is allowed to say "I do not know" and it is not allowed to guess.

**On screen:** open one settlement. Expand the feasibility panel. Show the
witness set, then the uniqueness proof.

---

## 1:30 to 2:40, the result, against baselines

> Measured on 498 synthetic settlements.
>
> Reconstruction alone proves 75 of them with zero wrong. That is a small
> number and it is an honest one: on a subscription merchant billing two
> hundred identical charges, no method that reads amounts can tell one group
> from another, and none ever will.
>
> So reconstruction is not the product. The product is the composite: prove it
> if you can, and if you cannot, check the claim the report already made. That
> posts 358 settlements, 75 from proof and 283 from a checked claim, and it
> posts zero of them wrong.
>
> Against the baselines. Trusting the report posts 428 and gets 20 wrong.
> Fuzzy confidence matching, the thing most tools ship, posts 345 and gets 290
> wrong, and its correct count does not move as you sweep the threshold from
> 0.10 to 0.90, which tells you the threshold was never the problem.
>
> And it does not cry wolf. Across 408 clean reports it raised zero false
> alarms.

**On screen:** the results table, three rows, posted and wrong side by side.

---

## 2:40 to 3:40, where the agent actually is

> Now, where is the model in this, because it is not where people expect.
>
> The model never decides whether a posting is correct. It cannot. The
> arithmetic decides that, and the arithmetic is integer paise with no
> tolerance. What the model does is the open-ended work around it.
>
> It runs a controller loop over a closed set of eight actions: widen the
> window, search the feed, narrow to this merchant's payout history, check the
> claim. Only two of those can ever result in a posting, and that is enforced by
> a test, not by a prompt.
>
> It diagnoses defective reports and drafts the counterparty note.
>
> And at the end of the period it investigates. Four inspection turns over the
> receipt store, reading exceptions by merchant, by status, by residual, and
> then it writes the close: these are the root causes of everything that did not
> reconcile this period. It found four of the five operational conditions we
> planted without ever being told they existed.
>
> 1,498 model calls across eight roles, every one schema-forced, none of them
> on the path that decides whether money moves.

**On screen:** the agent trace, actions scrolling. Then the period close, root
causes listed.

---

## 3:40 to 4:25, why a finance team can actually run this

> Three things make this deployable rather than a demo.
>
> One: every posting carries a receipt. The witness set, the uniqueness proof,
> the guards that ran, the model calls that were made and what they cost. You
> can hand it to an auditor.
>
> Two: it is reproducible. Same seed, byte-identical output, no network
> required. A reconciliation you cannot re-run is not one you can defend.
>
> Three: we published the break-even. Checking costs analyst time, and if your
> reports are almost never wrong it is not worth it. Below roughly a 2.3 per
> cent report defect rate, do not buy this. Above it, you should. We are telling
> you the condition under which our own product is the wrong purchase, because
> a finance team is going to work that out anyway and would rather we said it
> first.

**On screen:** a receipt, expanded. Then the break-even chart.

---

## 4:25 to 5:00, close

> The track asked for one finance-ops loop closed across a fifty record batch,
> with a match rate and an honest exception list.
>
> This is 498 settlements. 358 posted, zero wrong. 140 exceptions, every one
> with a stated reason, a residual, and what it would take to clear it. The
> whole thing regenerates from one command, and every number in the README is
> rendered from the run rather than typed into it, so it cannot go stale.
>
> One agent, one loop, closed. Thank you.

**On screen:** the exception list, then the command `./run.sh demo` and the
repository URL.

---

## Delivery notes

- **Do not read the architecture.** Nobody watching a demo video absorbs a
  package layout. The uniqueness proof and the baseline table are what land.
- **The 20 wrong postings are the hook.** Say the number in the first thirty
  seconds and everything after it has somewhere to sit.
- **Say "zero wrong" three times.** It is the only claim that matters and
  repetition is how a viewer retains a figure.
- **Do not apologise for 75.** The composite is the product. Reconstruction
  is the floor under it, and framing the floor as a shortfall invites a
  question that the composite already answers.
- **Screen recording beats slides** for beats 2, 4 and 5. Record the UI at
  1440p and crop, so text is legible after compression.
