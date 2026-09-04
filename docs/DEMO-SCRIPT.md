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

## 0:00 to 0:10  The close

**On screen:** the dashboard's landing tab, The close.

> This is a settlement period, closed. Not four hundred exceptions in arrival
> order: four root causes, ranked by the money they are holding, each citing the
> figures it was read from.
>
> Two of those causes are misconfigurations this benchmark injected on purpose,
> and the model was never told about them. It read status mixes and pool sizes
> and inferred them, on the right merchant, four out of four.

**Point at the recall figure.**

> That is the AI in this system doing the job the track is named after, and
> being scored on it.

## 0:10 to 0:16  The problem

**On screen:** the landing page, top of the hero.

> A gateway settlement arrives as one bank credit. It is the net of hundreds of
> payments, minus fees, tax, refunds and chargebacks. Somebody has to work out
> which transactions produced it.

## 0:16 to 0:22  The claim

**On screen:** scroll to the two-column comparison on the landing page. Both
figures are visible at once.

> Most tools trust the settlement report and post whatever it says. On these
> 498 settlements that posts everything and gets 29 wrong, silently. Manhattan
> posts 406 and gets none wrong, because it checks the report's claim against
> the money before it believes it.

## 0:22 to 0:38  Why, in one settlement

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

## 0:38 to 0:46  The model inside one settlement

**On screen:** Run tab, open a settlement whose report claim was contradicted.
Scroll to the claim panel.

> The gateway's own mapping for this settlement does not add up. That check is
> arithmetic and it is already done. What the model does is say why: it names
> the defect class, the system owns the remedy for that class, and it drafts the
> note an analyst gets.

**Point at the diagnosis block.**

> The model reads. The arithmetic decides. Those are different jobs and this is
> the only arrangement in which an agent should be allowed near a ledger.

## 0:46 to 0:54  It knows in advance

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

## 0:54 to 0:58  The evidence

**On screen:** back to Run, click any row to open a receipt. Expand Narrowing
and Feasibility.

> Every decision emits one of these, including every refusal. The narrowing
> waterfall with a reason for each dropped record. Both estimators against the
> refusal threshold. The witness, the completeness checks, the identity closing
> to zero.
>
> Nothing here is a score. It is a derivation somebody can be asked to defend
> at audit.

## 0:58 to 1:00  The close, again

**On screen:** the Exceptions tab, sorted by cost.

> And what it holds is not an apology. Each one has a named cause, a computed
> remedy, a price, and a note an analyst can send. Refusing costs analyst time.
> Unwinding a silent wrong posting costs more.
>
> Proving is not the expensive option. It is the cheap one.

---

## Notes for the take

- Read the figures off the regenerated README before recording; they change
  with every run and this file is the only document with numbers typed by hand.
- Do not narrate the architecture. A judge who wants it will read the README,
  and sixty seconds spent on boxes and arrows is sixty seconds not spent on the
  one number that distinguishes the submission.
- Two frames matter more than the rest: the recall figure at 0:05 and the rival
  panel at 0:22. Hold both.
