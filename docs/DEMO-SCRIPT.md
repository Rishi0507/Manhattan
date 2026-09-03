# Sixty seconds

A shot list for the submission recording. One take, no cuts needed, no slides.
Everything shown is live and comes from `./run.sh demo`.

Have the batch already run before recording, so the dashboard opens populated
and the sixty seconds are spent on the argument rather than on a progress bar:

```
./run.sh bench          # about 20 seconds, writes out/ and the three documents
./run.sh serve          # then record against localhost:8080
```

---

## 0:00 to 0:08  The problem

**On screen:** the landing page, top of the hero.

> A gateway settlement arrives as one bank credit. It is the net of hundreds of
> payments, minus fees, tax, refunds and chargebacks. Somebody has to work out
> which transactions produced it.

## 0:08 to 0:14  The claim

**On screen:** scroll to the two-column comparison on the landing page. Both
figures are visible at once.

> Most tools guess and report a confidence score. On the same 498 settlements,
> the baseline posts 384 and 226 of them are wrong. Manhattan posts 161 and
> none of them are.

## 0:14 to 0:32  Why, in one settlement

**On screen:** click through to Head to head. Let the single credit and both
panels sit on screen for a beat.

> Same credit, same inputs, same filters. The baseline finds six records that
> sum correctly, scores it 0.95, and posts. It is wrong.
>
> Manhattan finds a reconstruction too, and the accounting identity closes to
> zero. Then it does the thing the baseline never does: it widens the pool and
> looks for a second answer.

**Point at the rival panel.**

> It finds one. Swap one record for another and the sum is identical. So the
> first answer was not unique, it was just the one that survived the filter.
> Manhattan holds it and names the constraint responsible.

## 0:32 to 0:44  It knows in advance

**On screen:** the Calibration tab, on the outcome-band chart.

> This is the part that is not a solver demo. The collision index on the left
> is computed before any search runs. The bars are what actually happened.
>
> As the predicted index climbs, verified gives way to ambiguous and then to
> refusal, and the wrong-posting rate stays at zero the whole way across. The
> baseline's climbs to 41 per cent.
>
> Which means the system can tell a merchant what fraction of their
> settlements it will post before a single file is exchanged.

## 0:44 to 0:54  The evidence

**On screen:** back to Run, click any row to open a receipt. Expand Narrowing
and Feasibility.

> Every decision emits one of these, including every refusal. The narrowing
> waterfall with a reason for each dropped record. Both estimators against the
> refusal threshold. The witness, the completeness checks, the identity closing
> to zero.
>
> Nothing here is a score. It is a derivation somebody can be asked to defend
> at audit.

## 0:54 to 1:00  The close

**On screen:** the Exceptions tab, sorted by cost.

> And the 337 it refused are not an apology. Each one has a named cause, a
> computed remedy and a price. Refusing costs 112,000 rupees of analyst time.
> Unwinding the baseline's wrong postings costs 542,000.
>
> Proving is not the expensive option. It is the cheap one.

---

## Notes for the take

- The figures spoken above are from run `run_20260903_0358`. If you re-run the
  benchmark, read them off the regenerated README rather than this file, which
  is the only document here with numbers typed into it by hand.
- Do not narrate the architecture. A judge who wants it will read the README,
  and sixty seconds spent on boxes and arrows is sixty seconds not spent on the
  one number that distinguishes the submission.
- The rival panel at 0:14 is the single most important frame. Hold it.
