# What Manhattan cannot do

State these before anyone asks. Every one is a design decision with a reason, and several were discovered by running the system rather than by reasoning about it.

---

## The method has a narrow regime, and it is a property of the combinatorics

**Uniqueness is attainable only when free cardinality is roughly 3 to 7** for realistic pool sizes and amount spreads. Outside that band, thousands of subsets hit the target and no amount of compute changes it. `UNDERDETERMINED` is the honest answer and Manhattan returns it.

No solver improvement changes this. It is not a limitation of meet-in-the-middle, of the implementation, or of the hardware. It is what the arithmetic permits.

**The measured consequence is a modest auto-post rate.** On the shipped 500-settlement benchmark Manhattan auto-posts 15% overall, against B0's 81%. The comparison that matters is that B0's 81% includes 218 wrong postings and Manhattan's 15% includes zero, but the trade is real and it should be named rather than buried: this system posts less.

**Auto-post rate depends on the merchant, not on the algorithm.** Measured across the six archetypes it ranges from 41% (travel, wide ticket spread) to 0% (subscription SaaS, three repeated price points). The segmentation makes this a sales input rather than an excuse, but the underlying fact stands.

---

## Uniqueness is scoped, not absolute

Meet-in-the-middle at `k_max` searches `{S : k(S) ≤ k_max}` completely and nothing beyond. **The receipt prints the scope; it is never implied.**

Where `k_max = k*`, the justification is that above `k*` the collision index shows rivals are near-certain, so a `VERIFIED` up there would be wrong regardless of whether the system had looked. That justification is a heuristic, and it is precisely where the seam is.

**A declared-count dispatch scopes the proof by the counterparty's own claim.** Where the settlement report states a transaction count, using it is cheaper and is frequently the only route to `VERIFIED` at all, since `k*` sits at the contested boundary by construction. But *unique among subsets with k(S) ≤ 6, where 6 is what the report said* is a materially weaker statement than *unique among subsets with k(S) ≤ k\**, and it is weaker in exactly the direction that matters: it borrows from the artifact it is supposed to be validating.

The receipt records `scope_source` and the `k*` it declined to use, so the two claims are never confused. The weaker one is still weaker. It also mildly weakens the cardinality cross-check, which can still fail but over a narrower range of witness sizes.

---

## The collision index is an estimator, and the closed form is measurably wrong

The published analytic form assumes subset sums are locally uniform near the target, with a density read off a moment-matched normal. **On lognormal ticket distributions, which is what real merchant data looks like, this is wrong by roughly an order of magnitude.**

Measured: on a 313-record travel pool at `k = 3` it predicted 0.38 rival reconstructions. Brute-force enumeration found three, on every one of sixty seeds tried.

Manhattan now measures the density by sampling subset sums from the actual pool. Validated against exhaustive enumeration on a lognormal pool, the sampled estimator has a mean absolute log-ratio of 0.09 against the true count; the analytic form has 0.72. Both are carried on every receipt.

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

## The agent's contribution to recall is bounded by the data

**A hypothesis can only clear an exception when it cites a real record.** Speculative hypotheses are shown to analysts and never posted, whatever the arithmetic says. So the agent's contribution to auto-post rate is bounded by how often a genuinely unjoined data source exists.

The question-answering agent can only answer what a receipt recorded. It has no access to raw records, cannot re-run a reconciliation, and declines rather than inferring. That is deliberate and it is also a real ceiling on how useful it is.

The offline stub is a stub. It proposes from a fixed list in a fixed order and it will clear fewer exceptions than a real model. That the eleven-case suite passes on it is a statement about the verifier, not about the stub.

---

## Performance depends on an implementation detail

Every published timing assumes the flat, array-shaped enumeration: parallel primitive slices, a radix sort, binary search over contiguous memory. **A structurally identical implementation built out of per-entry objects is correct and one to two orders of magnitude slower**, at which point the entire resource envelope becomes fiction.

This is a real dependency and it is asserted by a test rather than hoped for.

Memory grows with `C(n/2, ≤k*)`. It is bounded across the accept region and guarded by a configured ceiling checked *before* allocation, but a merchant with pools above roughly 1,000 candidates at `k = 3` sits near it: 41.7M entries, 714 MB measured.

---

## Benchmarked on synthetic data

The pathology mix reflects documented Razorpay settlement mechanics: paise-denominated amounts, T+2 cycles, MDR with 18% GST charged on the fee, refunds netted in the cycle they clear, chargeback debits with a flat dispute fee, zero-MDR UPI. Ground truth is recorded by the generator and never read by the pipeline, only by the benchmark and only after a decision has been made.

**Real merchant data will contain things the generator does not model.** Every accuracy figure in this repository should be read as a statement about the system's behaviour on a distribution chosen to be plausible, not as a measurement of the Indian merchant base.

The archetype table is likewise modelled. The spread and twin-mass values come from configured ticket distributions chosen to be plausible for each merchant type. It describes how the estimator behaves across distribution shapes over stated assumptions. It is not market data and must not be presented as such.

---

## Deliberately deferred

**The escalation ladder.** Climbing `k_max` through 2, 4, `k*` would find small-`|S|` answers sooner, but the enumeration at `k*` already contains every entry at every lower cardinality, so nothing is missed by dispatching straight to `k*`. It is a latency optimisation worth roughly a factor of two on easy settlements and it buys no additional correctness.

**Probabilistic residual matching.** Fellegi and Sunter with EM for calibrated, label-free match weights, and distribution-free risk control on any probabilistic path, are both well motivated and both out of scope here.

The deterministic path needs no statistical gate, because a closed integer identity is not a probabilistic claim. Those techniques govern the *residual*, and after this design the residual is large, measured, and **segmented by merchant archetype**, because `UNDERDETERMINED` is now an explicit population with a known shape rather than a hidden one. A calibrated probabilistic layer over the `UNDERDETERMINED` and `AMBIGUOUS` buckets, with a distribution-free error guarantee, is the obvious next thing to build.

Being able to name what was left out, why, and exactly which population it would serve is worth more than a diagram with eight techniques on it.
