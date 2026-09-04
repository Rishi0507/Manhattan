# What Manhattan cannot do

State these before anyone asks. Every one is a design decision with a reason, and several were discovered by running the system rather than by reasoning about it.

---

## The method has a narrow regime, and it is a property of the combinatorics

**Uniqueness is attainable only when free cardinality is roughly 3 to 7** for realistic pool sizes and amount spreads. Outside that band, thousands of subsets hit the target and no amount of compute changes it. `UNDERDETERMINED` is the honest answer and Manhattan returns it.

No solver improvement changes this. It is not a limitation of meet-in-the-middle, of the implementation, or of the hardware. It is what the arithmetic permits.

**The measured consequence is a bounded auto-post rate.** On the shipped 498-settlement benchmark Manhattan auto-posts 15% overall, against B0's 69%. B0's figure carries 290 wrong postings and Manhattan's carries 0, which is the comparison that matters, but the ceiling is real and belongs here rather than buried: **this system posts less, and there are settlements it will never post.**

**Auto-post rate depends on the merchant, not on the algorithm.** Measured across the 6 archetypes it runs from 45% (travel, wide ticket spread) down to 0% (subscription SaaS, three repeated price points). The segmentation makes this a sales input rather than an excuse, but the underlying fact stands.

---

## Uniqueness is scoped, not absolute

Meet-in-the-middle at `k_max` searches `{S : k(S) at most k_max}` completely and nothing beyond. **The receipt prints the scope; it is never implied.**

Where `k_max = k*`, the justification is that above `k*` the collision index shows rivals are near-certain, so a `VERIFIED` up there would be wrong regardless of whether the system had looked. That justification is a heuristic, and it is precisely where the seam is.

**A declared-count dispatch scopes the proof by the counterparty's own claim.** Where the settlement report states a transaction count, using it is cheaper and is frequently the only route to `VERIFIED` at all, since `k*` sits at the contested boundary by construction. But *unique among subsets with k(S) ≤ 6, where 6 is what the report said* is a materially weaker statement than *unique among subsets with k(S) ≤ k\**, and it is weaker in exactly the direction that matters: it borrows from the artifact it is supposed to be validating.

The receipt records `scope_source` and the `k*` it declined to use, so the two claims are never confused. The weaker one is still weaker. It also mildly weakens the cardinality cross-check, which can still fail but over a narrower range of witness sizes.

---

## The collision index is an estimator, and the closed form is measurably wrong

The published analytic form assumes subset sums are locally uniform near the target, with a density read off a moment-matched normal. **On lognormal ticket distributions, which is what real merchant data looks like, this is wrong by roughly an order of magnitude.**

Measured: on a 313-record travel pool at `k = 3` it predicted 0.38 rival reconstructions. Brute-force enumeration found three, on every one of sixty seeds tried.

Manhattan now measures the density by sampling subset sums from the actual pool. Two figures are published for this and they are not interchangeable, so both are scoped explicitly.

**On one lognormal pool**, the fixture in `internal/feasibility/empirical_test.go`, checked against exhaustive enumeration of every 3-subset: the sampled estimator scores **0.09** mean absolute log-ratio against the counted truth, the analytic form **0.72**.

**Across the calibration sweep**, the 57 configurations of 96 where exhaustive counting was feasible and there is therefore a true count to score against: the sampled estimator scores **1.94**, the analytic form **2.72**. Both are far worse than on the single pool, because the sweep deliberately includes regimes neither model was built for, among them pools whose true rival count runs into the thousands.

The swept numbers are the ones to quote about the system; the single-pool pair describes one fixture and nothing more. The ordering is what the design rests on and it holds in both: the sampled estimator is closer to the counted truth than the closed form, everywhere it has been measured. Both are carried on every receipt.

### The index orders outcomes within a cardinality, not across all of them

This is the sharpest limit on the commercial claim and it was found in this project's own sweep rather than pointed out afterwards.

Read flat across all 96 swept configurations, the collision index does **not** order outcomes cleanly. Travel at pool 220 with index 4.64 verifies nothing; travel at pool 70 with index 6.03 verifies everything. Marketplace at 150 with index 5.87 verifies nothing; marketplace at 48 with index 5.96 verifies everything. Higher predicted index, better observed outcome, twice.

Segmented by batch cardinality, which is the variable the index has to be read against, the verified rate is monotone in the index at cardinality 9 and not at 4 and 6. The reason is a property of the estimator: the index is an *expected* number of colliding subsets, and at small cardinality the enumeration is small enough that the realised count is frequently one where the expectation is five.

Two consequences, and only one of them is comfortable.

**The failure direction is safe.** Being conservative means refusing configurations that could have verified. The wrong-posting rate is zero in every band of every cardinality, so this costs recall and never precision.

**The sales claim has to be narrower than "one pass over historical amounts predicts your auto-post rate".** It predicts it at the cardinalities where refusal actually binds, and under-predicts it below them. Quoting the flat version would be quoting something this repository's own data contradicts.

## The estimator is still an estimator

**But the sampled estimator is still an estimator.** Its errors run in the direction that costs recall rather than precision (an index that is too low makes the gate accept a slightly larger region, and the exhaustive count inside that region catches the rivals anyway), and that asymmetry is the entire justification for admitting a heuristic into an otherwise deterministic pipeline. A badly calibrated threshold will still reject verifiable settlements, and because the index also sets `k*`, a bad threshold shrinks the searched region as well as the accepted one.

At large `n` the sampled estimator remains conservative by a factor of several. The calibration sweep measures this rather than assuming it away.

---

## The completeness guards each have a boundary

### The neighbourhood probe is bounded by substitution depth

At depth 2 it covers any rival differing from the witness by up to two records, at any witness size. **A rival sharing fewer than `|S| − 2` records with the found witness is not covered.**

That is a different failure from the one this guard exists for, since narrowing dropping one true record and a coincidental record taking its place is depth 1 by construction. But the boundary is real at every depth.

### The probe reduces its own depth, and sometimes gives up

The probe searches for a coincidence, so it has a multiple-comparisons problem. At depth 2, a witness of size `|S|` against `m` spare records compares roughly `|S|²m²/4` pairs. For a 138-record witness that is over 10⁸, and a chance collision is a near certainty.

**Before this was fixed, the probe fired on every large batch.** It now estimates its expected chance collisions, reduces depth until that is small, and reports itself `inconclusive` rather than `stable` when even depth 1 cannot distinguish signal from coincidence. On large witnesses it therefore covers less than it does on small ones, and it says so.

### The gross-ratio guard is inactive in lump-credit mode

If contributions are policy-derived, any subset whose contributions sum to the target automatically implies the policy rate by construction. The check cannot fail and carries no information, so it reports itself **inactive** rather than passing. Completeness in that mode rests on the neighbourhood probe and the cardinality cross-check alone, which is a weaker guard set.

It was also initially compared against *policy* rather than against the pool, which made it a duplicate of the fee anomaly detector and meant a merchant whose gateway genuinely overcharges could not post at all. It now compares against the rate prevailing across the pool the witness was drawn from.

### The drift monitor catches drift, not original error

It compares each constraint's drop rate against a stored baseline across a whole run. **A narrowing constraint that has been wrong since the first run has no baseline to deviate from, and is stable under relaxation, so neither the run-level monitor nor the per-settlement probe fires.**

This is the sharpest remaining hole in the completeness argument. It is stated here rather than papered over.

---

## Records that net to zero cannot be attributed at all

A UPI payment carries no MDR under Indian regulation, so a UPI payment refunded in full before settlement nets to precisely nothing. Such a record moved no money and **no amount-based method can place it in or out of a batch**, because any witness plus or minus a zero has an identical sum.

Manhattan removes them before searching, names them individually on the receipt, and reconciles them against the declared transaction count as *witness plus zeros*. That is the strongest correct claim available, and it is genuinely weaker than naming every record in the batch.

This was not in the original design. It was found by running the system, and before it was handled a single such record made every affected settlement ambiguous.

---

## The fee detector identifies an effective rate, not a schedule

Real fee structures contain slabs, floors and caps, per-instrument and per-network rates, promotional pricing and negotiated overrides. Several schedules can produce the same aggregate, so a single implied rate identifies an **effective rate**, not the actual policy.

It is circular in lump-credit mode, where Manhattan makes no anomaly claim at all. And **detection alone does not require the solver**: with per-payment fee rows and a policy config, a `GROUP BY` gets you the effective-rate delta. What the solver adds is attribution.

---

## Rounding tolerance is a configured assumption

In inferred mode the per-item tolerance, the band basis and the slack actually consumed are reported on every receipt, so the assumption is always visible. Set it too wide and false matches become possible.

The band is scaled by the **witness** cardinality, not the pool size. A pool-width band matches essentially everything on a large pool, which is worse than matching nothing. On the complement path the band must scale by `n − |C|` rather than `|C|`; getting that wrong produces a clean `UNRESOLVED` rather than a visible crash, which is why it was caught by a brute-force oracle rather than by inspection.

---

## The agent posts only on corroborated actions, which caps what it can repair

An action that cites a real record in a real feed may post. An action that changes a filter, a window or a constraint may not, however cleanly the accounting identity closes afterwards.

That rule exists because of a measured failure rather than a principle chosen in advance. Without it, an agent able to retune narrowing produced **two wrong postings in three hundred settlements**. That figure is recorded by hand from an earlier build and is the only number in this document not emitted by run `run_20260904_0832`. The build that produced it is gone, so the failure it describes is rebuilt as a committed test in `internal/agent/corroboration_test.go`: the property that prevents it is asserted directly, and the test fails if `TIGHTEN_WINDOW` is ever made postable again. What happened: it tightened a value-date window, the candidate pool fell from 44 records to 40, an `AMBIGUOUS` settlement became `VERIFIED`, and the answer was wrong because the tightening had cut real records out of the batch. Every check passed, including the completeness probe, because the surviving rival differed from the found witness by more than the probe's depth bound.

The consequence is a real ceiling, and it is a ceiling on the mechanism rather than a shortfall in the result. Most refusals are `UNDERDETERMINED`, caused by a pool that is too wide, and the agent can prove exactly what would fix them but is not permitted to act on it. On the shipped benchmark it repairs **35** settlements into postings and produces **65** proven cures, out of the 458 that entered the loop.

Read the repair count as a measurement of the data, not of the loop. **Every one of the 35 repairs cited a record sitting in a feed nobody had joined**, because that is the only class of action allowed to post. The agent's contribution to recall is therefore exactly as large as the amount of genuinely missing data in the inputs and not one settlement larger. On a dataset with well-configured narrowing and every feed connected, the correct number of repairs is zero, and the loop returning zero there would be the loop working.

The 65 proven cures are the other half of the output and they never post, by design. A cure is a remediation whose effect has been computed and re-verified rather than estimated. Handing an analyst *tightening this window to seven hours yields exactly one reconstruction, with the identity closing to zero* is stronger than handing them a bare residual, and it is still their decision to make.

## The agent is not invoked on most exceptions, deliberately

A deterministic screen settles 166 of the 458 exceptions that enter the loop with no model call at all, which is 36% of them: the amounts do not distinguish the transactions, or a rival already appears when the pool is widened, or there is nothing left to search or tighten.

This is the right trade, and it is still a trade. The screen is conservative but it is a heuristic, and a settlement it skips is one the agent never sees. A more capable model might have found something in a case the screen declared hopeless.

## The agent's contribution to recall is bounded by the data

**A hypothesis can only clear an exception when it cites a real record.** Speculative hypotheses are shown to analysts and never posted, whatever the arithmetic says. So the agent's contribution to auto-post rate is bounded by how often a genuinely unjoined data source exists.

The question-answering agent can only answer what a receipt recorded. It has no access to raw records, cannot re-run a reconciliation, and declines rather than inferring. That is deliberate and it is also a real ceiling on how useful it is.

The offline stub is a stub. It proposes from a fixed list in a fixed order and it will clear fewer exceptions than a real model. That the eleven-case suite passes on it is a statement about the verifier, not about the stub.

---

## Performance depends on an implementation detail

Every published timing assumes the flat, array-shaped enumeration: parallel primitive slices, a radix sort, binary search over contiguous memory. **A structurally identical implementation built out of per-entry objects is correct and one to two orders of magnitude slower**, at which point the entire resource envelope becomes fiction.

This is a real dependency and it is asserted by a test rather than hoped for.

Memory grows with `C(n/2, at most k*)`. It is bounded across the accept region and guarded by a configured ceiling checked *before* allocation, but a merchant with pools above roughly 1,000 candidates at `k = 3` sits near it: **714 MB** at the top of the measured envelope.

Three memory figures appear in this repository and they measure different things, so each is named where it is used.

**114 MB** is the largest enumeration any single settlement in this batch actually allocated, computed from the entry counts on its receipt at twelve bytes each. It is deterministic: the same seed and the same commit produce it exactly. This is the figure a capacity estimate should use.

**747 MB** is a *sampled* process heap high-water mark, and it moves. Two runs of the same commit on the same seed have reported 15 MB and 119 MB for an identical batch, because where the sample lands depends on when the garbage collector happened to run. It is published because it is the number an operator asks for, and it is labelled as sampled everywhere it appears. It should never be quoted as a bound.

**714 MB** is the top of the resource envelope, which deliberately probes a 1,000-candidate pool at k=3 that no merchant in this benchmark has. It is a ceiling for a pool size the batch does not contain.

---

## A checked claim is not a proof, and the composite's headline rests on checked claims

283 of the composite's 358 postings are the gateway's own mapping, verified against the money. That is much stronger than posting it unchecked, which is what a lookup does, and it is materially weaker than `VERIFIED`.

`CLAIM_CONSISTENT` means the named batch produces this credit. It does not mean no other batch would. On a flat-price merchant, where the composite does its best work, a great many other batches would, and the claim check does not enumerate them because enumerating them is the intractable problem it exists to route around.

So the honest reading of 72% is: **15% of settlements carry a proof that nobody had to be trusted for, and the rest carry a counterparty's claim that has been checked against an independent account of the money.** Both are worth posting. They are not the same claim and the receipt never says they are.

What this cannot detect is a report that is wrong in a way that still balances: a substituted record of identical contribution, or a fee error that exactly offsets a membership error. The reconstruction can catch some of those and only where it is decisive at all.

---

## The report-defect rate is an assumption, and the comparison rests on it

The measured answer to "we already ship that mapping" depends on a generated defect rate of 4.0%, and that figure is a modelling choice rather than an observation of any real gateway.

It is deliberately modest, because the argument does not need reports to be bad. It needs them to be occasionally wrong in a way nothing downstream can detect, which is a much weaker and much more defensible claim. But if a gateway's true defect rate is a tenth of this, the lookup's 20 wrong postings become two or three, and the case for an independent reconstruction is correspondingly smaller in volume terms. It is not smaller in kind: the wrong postings that remain are still silent, still unattributable, and still found at audit rather than at posting.

The three defect shapes modelled are chosen as plausible rather than sampled from any observed population, and two of the three have documented real-world counterparts while the third does not.

**The chargeback cycle mismatch is documented behaviour, not a hypothetical.** A dispute is raised against the original transaction and debited in whatever cycle the network resolves it in, which is routinely a different cycle from the one that carried the payment. Razorpay's own settlement documentation describes disputes and their fees as adjustments applied to a later settlement, and the card network rules that govern the timetable (Visa and Mastercard both allow dispute windows measured in months from the transaction date) are why the two cycles come apart at all. A settlement report whose own join is by capture date therefore has a structural reason to omit a debit that genuinely moved money in the cycle it is describing.

**The cross-cycle double-count** follows from the same timetable in the other direction, and is the ordinary failure mode of any reconciliation keyed on a date rather than on a settlement identifier.

**A mapping short by one record** is not a payments phenomenon at all; it is what a truncated file or a partial write looks like downstream, and it is included because a reconciliation should not depend on its inputs being well-formed.

A payments engineer will know better than this generator does which of these actually occurs and at what rate, and that is a conversation this document invites rather than forecloses. What the argument does not depend on is the rate: see the sensitivity sweep, where the composite's wrong-posting count stays at zero from a 6 per cent defect rate down to a tenth of it.

**What is not an assumption** is the structural point underneath it. A reconciliation whose only check on the settlement report is the settlement report cannot detect a defective one at any rate, including zero. Manhattan flagged 20 of 20 and missed 0 because it has an independent account of the money, and that property does not depend on how often the report is wrong.

---

## Determinism is per commit, and covers decisions rather than timings

Same seed and same commit produce the same **decision** on every settlement: identical statuses, witnesses, rival counts, flags and remediations. That is the property the reproducibility claim rests on and it is the one that matters, because it is what lets a receipt be re-derived by somebody checking it.

It does not extend to timings. Measured across two runs of one binary at one seed: median latency moved 14.0 ms to 13.7 ms, p95 moved 81 ms to 100 ms, sampled peak memory moved 12.6 MB to 13.5 MB. Receipts carry `timing_ms`, so they are **not byte-identical between runs** and the earlier claim that they were is withdrawn.

Across commits nothing is guaranteed at all. Changing the generator changes the random stream, so a fixed seed produces different data. Two runs an hour apart at seed 20260826 reported 161 and 151 verified settlements, and the cause was a code change between them rather than any instability.

The honest form of the claim is on every document: **same seed and same commit, same decisions.**

---

## Corroborated narrowing needs a seed of proofs, so it cannot rescue the worst cases

`NARROW_TO_HISTORY` posts only where a merchant's own prior `VERIFIED` settlements corroborate the bound, and a profile requires twelve of them. That threshold is what makes the action safe and it is also a floor on what it can reach.

The measured consequence is in the sensitivity sweep and it is worth reading. At the modelled window misconfiguration the action produces repairs. At **twice** that misconfiguration it produces none, because a merchant whose window is that badly set proves almost nothing, never accumulates twelve proofs, and therefore has no history to corroborate against.

So the action helps a deployment that is somewhat wrong and cannot help one that is very wrong. That is the correct behaviour under the corroboration rule and it is a real ceiling: the settlements most in need of the repair are the ones least able to establish the evidence for it.

---

## The operational conditions are modelled, and the agent's contribution depends on them

This run deliberately models two misconfigurations:

- d2c_ecommerce: reconciliation window misconfigured to plus or minus 24 hours
- marketplace: disputes feed never joined into the pool
- marketplace: reconciliation window misconfigured to plus or minus 26 hours
- quick_commerce: disputes feed never joined into the pool
- travel: reconciliation window misconfigured to plus or minus 22 hours

Both are things a deployment gets wrong on its own side rather than things a counterparty did, and both are the most common of their kind. They are also the reason the agent has anything to repair, and a reader is entitled to be suspicious of that.

The answer is the sensitivity sweep rather than an argument: the agent's contribution is published as a function of how much misconfiguration exists, and at zero the narrowing action repairs **0**. An agent that repaired something on a correctly configured deployment would be inventing work, which is worse than doing none.

What does not depend on these conditions is the safety property. Wrong postings are zero in every scenario of the sweep, including the ones where the agent is doing the most work.

---

## No live model run at batch scale

Every published figure comes from the deterministic offline path (`offline-stub`, parse=replay resolve=replay answer=replay). The live Anthropic path is implemented, schema-forced and cassette-recording, and it runs. What has not been done is a batch against the live API.

`manhattan live -n 60` exists precisely to close this. It runs the same batch on both providers and asserts the property that matters, that wrong postings are **identical**, while reporting the figures that are free to move: diagnosis accuracy, agent repairs, note quality and actual billed cost. It exits non-zero if the wrong-posting column moves, because that would be a leak in the trust boundary rather than an interesting result.

Until it has been run, the honest summary of this repository's AI evidence is: **the architecture is demonstrated and the model quality is not measured.**

Two consequences, stated rather than glossed.

**The cost figures are modelled, not billed.** Measured token counts priced at published rates. The direction of that error is known: the replay path reports no cache reads, so every input token is priced at the uncached rate, and a live run caching the byte-identical parse system block would come in under the 1,357 INR per thousand published here.

**One model job is graded and the rest are not.** Defect diagnosis scores 65% against the generator's own record of what it injected, which is a real accuracy figure for a real model output. Every other role is constrained rather than scored: a parse that goes wrong produces an exception, an action that goes wrong is rejected by the verifier, a drafted note that goes wrong is a confusing sentence. Those constraints are the safety argument and they are not accuracy measurements, and a reader should not read them as one.

**The drafted notes are unevaluated.** 384 of them, and nobody has read a sample and scored it. The digits guard rejects a draft that smuggles in a figure (0 this run), which catches the one failure mode that would put a wrong number in front of an analyst. It does not catch a note that is merely useless, and on the offline stub many of them are, because the stub assembles sentences from a fixed table rather than writing them.

**No delta is published for what a capable model buys over the stub.** The offline stub proposes from a fixed list in a fixed order. It cannot change whether a posting is correct, because the model is never asked whether it was right, and the eleven-case suite passing on it is a statement about the verifier rather than about the stub. But how many more exceptions a real model would clear is unmeasured, and putting a number on it would be exactly the class of unverified claim this document exists to prevent.

---

## Benchmarked on synthetic data

The pathology mix reflects documented Razorpay settlement mechanics: paise-denominated amounts, T+2 cycles, MDR with 18% GST charged on the fee, refunds netted in the cycle they clear, chargeback debits with a flat dispute fee, zero-MDR UPI. Ground truth is recorded by the generator and never read by the pipeline, only by the benchmark and only after a decision has been made.

**Real merchant data will contain things the generator does not model.** Every accuracy figure in this repository should be read as a statement about the system's behaviour on a distribution chosen to be plausible, not as a measurement of the Indian merchant base.

The exception economics are modelled too. The 2,400 INR cost of unwinding one wrong posting is an assumption, printed wherever it is used so a reader can substitute their own, and the 153,236 INR held-queue total rests on a configured analyst handling time.

The archetype table is likewise modelled. The spread and twin-mass values come from configured ticket distributions chosen to be plausible for each merchant type. It describes how the estimator behaves across distribution shapes over stated assumptions. It is not market data and must not be presented as such.

---

## Deliberately deferred

**The escalation ladder.** Climbing `k_max` through 2, 4, `k*` would find small-`|S|` answers sooner, but the enumeration at `k*` already contains every entry at every lower cardinality, so nothing is missed by dispatching straight to `k*`. It is a latency optimisation worth roughly a factor of two on easy settlements and it buys no additional correctness.

**Probabilistic residual matching.** Fellegi and Sunter with EM for calibrated, label-free match weights, and distribution-free risk control on any probabilistic path, are both well motivated and both out of scope here.

The deterministic path needs no statistical gate, because a closed integer identity is not a probabilistic claim. Those techniques govern the *residual*, and after this design the residual is large, measured, and **segmented by merchant archetype**, because `UNDERDETERMINED` is now an explicit population with a known shape rather than a hidden one. A calibrated probabilistic layer over the `UNDERDETERMINED` and `AMBIGUOUS` buckets, with a distribution-free error guarantee, is the obvious next thing to build.

Being able to name what was left out, why, and exactly which population it would serve is the point of this document.

---

*This document is generated from run `run_20260904_0832`, seed `20260826`, by `manhattan bench`. Its source is `docs/LIMITATIONS.tmpl.md`. No figure in it is typed by hand, so it cannot come to describe a run other than the one that produced [RESULTS.md](RESULTS.md) and [README.md](README.md).*
