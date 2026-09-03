# Manhattan

**An agent that proves settlements instead of guessing them.**

Razorpay AI Buildathon · Track 04, AI Finance Controller · Multi-source reconciliation

Manhattan reconciles payment settlements. It never guesses about money, because the instrument it reconciles with is a solver rather than a similarity score. It either produces an exact, auditable reconstruction under declared accounting rules together with a proof that no rival reconstruction exists, or it names precisely which property it could not establish and routes the settlement to review.

```
git clone <this repo> && cd manhattan
./run.sh demo          # or:  .\run.ps1 demo    or:  make demo
```

No API key required. **Every number in this file, in [RESULTS.md](RESULTS.md) and in [LIMITATIONS.md](LIMITATIONS.md) is emitted by that command.** None of the three is typed by hand; all three are rendered from the same run, in the same command, so they cannot drift apart. This one is generated from run `run_20260903_0925`, seed `20260826`.

### Track compliance, stated first

The brief asks for one finance-ops loop closed across a batch of 50 or more synthetic records, reporting match rate and the exceptions it could not resolve.

| requirement | what this run did |
|---|---|
| 50+ record batch | **21,965 source records** (payments, refunds, chargebacks and adjustments across four feeds) driving **498 settlements**. Each settlement's record universe reaches **3,778** before narrowing and **95** after |
| one loop, closed | bank credit to posted ledger entry, or to a named exception, end to end |
| match rate reported | **104 of 498** auto-posted, 21%, with **0 wrong** |
| exceptions it could not resolve | **394**, each with a named cause, a computed remedy and a price. [The list is below.](#the-exception-list-is-the-deliverable) |
| throughput | **41,432 settlements per hour**, 24.7 ms median, on windows/amd64, 4 logical cores, go1.27.0 |

### What this run deliberately gets wrong

Two operational misconfigurations are modelled, because a reconciliation benchmark on perfectly configured data measures nothing an agent could help with. Both are things a **deployment gets wrong on its own side**, not things a counterparty did, and both are the most common of their kind:

- d2c_ecommerce: reconciliation window misconfigured to plus or minus 24 hours
- marketplace: disputes feed never joined into the pool
- quick_commerce: disputes feed never joined into the pool
- travel: reconciliation window misconfigured to plus or minus 22 hours

The obvious criticism of any agent benchmark is that the author created the problem the agent solves. The answer is not to argue, it is to [publish the agent's contribution as a function of how much misconfiguration there is](#what-the-agent-is-worth-and-when-it-is-worth-nothing). At zero it repairs nothing through narrowing, and that is the loop being unnecessary rather than the loop failing.

---

## The first question, answered with a number

Everyone at a payments company asks the same thing, so it goes above everything else, and it gets a measurement rather than a paragraph.

> **"We already ship that mapping. Optimizer's Single View Recon gives you settlement_id to payments. What is the solver for?"**

Correct. So the benchmark runs that answer as a system of its own, and then builds on it rather than arguing with it.

**B1** reads the settlement report's stated mapping and posts it. No search, instant, free.
**Manhattan** ignores the mapping entirely and reconstructs the credit from the merchant's own records.
**M1** is both: post Manhattan's reconstruction where it proved one, otherwise **check** the report's claim against the money and post it when it holds, hold when it does not.

| 498 settlements | B1, trust the report | Manhattan alone | **M1, the composite** |
|---|---:|---:|---:|
| posted | 498 | 104 | **406** (82%) |
| **posted wrong** | **29** | **0** | **0** |
| defective reports it would flag | 0 of 29 | | **25 of 29** |
| false alarms on 469 clean reports | | | **0** |

**That third column is the product.** It sits on top of Single View Recon instead of replacing it: 104 of its postings are Manhattan's own proofs and 302 are the report's claim, checked. It posts 302 more settlements than reconstruction alone and 92 fewer than the lookup, and the ones it declines to post are the ones the lookup gets wrong.

### Why checking reaches so much further than deriving

> **Deriving a batch is a search. Checking a batch somebody claimed is not.**

Reconstruction costs C(n, k) and is only decisive in a narrow regime, which is why two of six merchant archetypes reconstruct nothing at all. Verifying a claimed batch costs O(claim): do these records exist, do they belong to this merchant, were they already posted in a prior cycle, do their signed contributions sum to the credit, does the count agree with the declaration. None of that touches the combinatorics.

So the claim check works exactly where reconstruction cannot. A subscription merchant with two hundred identical charges has settlements no method can **derive** from amounts, and the gateway's mapping for those settlements can still be **checked** against the money in microseconds.

The verdict it produces is deliberately weaker than a proof, and the receipt never blurs them:

| | |
|---|---|
| `VERIFIED` | exactly one batch in the searched region produces this credit, counted exhaustively. Nobody had to be trusted. |
| `CLAIM_CONSISTENT` | the batch the report named does produce this credit. Others may too; this one was checked, not derived. |
| `CLAIM_CONTRADICTED` | the report's own account of this settlement does not survive checking. Here is the residual. |
| `CLAIM_UNCHECKABLE` | part of the claim lives in a feed nobody joined, so the check could not run. Our problem, not the report's. |

**The defects modelled are ordinary, and the first has a documented cause.** A dispute is raised against the original transaction and debited in whatever cycle the network resolves it in, which is routinely not the cycle that carried the payment, so a settlement report whose own join is by capture date has a structural reason to omit a debit that genuinely moved money. The other two are a payment named from the previous cycle and a mapping short by one record, which is what a truncated file looks like downstream. Causes and their limits are in [LIMITATIONS.md](LIMITATIONS.md); the code is in [`internal/generate/generate.go`](internal/generate/generate.go).

### The false-alarm rate is the number that decides this

A composite that holds settlements a lookup would have posted correctly is worse than the lookup. Measured: **0 false alarms on 469 clean reports** (0.0%).

That figure is optimistic by construction and the reason must be stated. The generator's fee model and the verifier's contribution model are **the same model**, so a real report that is defect-free can still disagree with Manhattan's arithmetic over a fee slab, a promotional rate or a rounding convention, and every one of those would be a false alarm this benchmark cannot produce. The `CLAIM_UNCHECKABLE` verdict exists precisely because the first version did not make this distinction and reported 84 contradictions on 469 clean reports, every one of them a chargeback in an unjoined feed rather than an error in the report.

### And it does not rest on the defect rate

The modelled rate of defective reports is 5.8%, which is a **choice made in this repository's generator**, not an observation of any real gateway. If Razorpay's true rate is a tenth of it, the volume argument shrinks by a factor of ten. So the claim is made rate-independent instead of defended:

| report defect rate | B1 posts wrong | M1 posts wrong |
|---|---:|---:|
| 6.0% | 22 of 22 defective | 0 |
| 0.6% | 1 of 1 defective | 0 |
| 0% | 0 of 0 | 0 |

The structural point does not move with the rate, and it is the one that matters: **a reconciliation whose only check on the settlement report is the settlement report cannot detect a defective one at any rate, including zero.** The value of independent verification tracks the cost of an undetected wrong posting, not its frequency.

> **The settlement report is a claim. Manhattan verifies the claim independently, from the merchant's own records.** A reconciliation system that trusts its input is not reconciling, it is transcribing.

The demo posture follows: per-payment fee rows are retained and the `settlement_id` mapping is **withheld from the search**, so the reconstruction is not handed the answer it exists to derive. The mapping is still generated, and B1 and the claim check both read it, which is how the table above is measured. Withholding it from one stage and feeding it to another is how a control is built, not how one is hidden.

---

## The problem, in one number

A gateway settlement lands as a single bank credit:

```
48638155 paise     value date 2026-08-26     NEFT-RAZORPAY SOFTWARE PVT LTD-UTR3491-CR
```

That figure is the net of hundreds of gross payments, minus MDR, minus GST on that MDR, minus refunds that cleared this cycle, minus chargeback debits, plus rounding. Amount matching finds nothing, because no single transaction equals the credit. Fuzzy matching and LLM adjudication produce plausible pairings with no evidence they account for the money.

> **Similarity generates candidates. It does not constitute accounting proof.**

Finding one subset that sums correctly is easy and nearly worthless: subsets grow exponentially while reachable sums do not, so many hit the target and picking one is arbitrary. Manhattan asks the harder question, *can I prove no other reconstruction exists*, and four things follow. **All money is integer paise**, so exact verification is available and no floating point exists in the verification path. **Contributions are signed**, because a chargeback debit is negative and so is a fully refunded payment whose fee was retained. **The decision is not a confidence score** but one of five states, four of which stop the money. **The failure mode is a refusal, not a bad posting**, so every heuristic is placed such that being wrong costs recall and never precision.

[docs/EXPLAIN.md](docs/EXPLAIN.md) builds all of this from first principles in plain language.

---

## The measured result

498 settlements across 6 merchant archetypes. B0 is a deliberately competent confidence matcher given identical inputs and identical narrowing; it is what most tooling in this space reduces to once the marketing is removed.

| 498 settlements | Manhattan | B0 |
|---|---:|---:|
| `VERIFIED` | **104** | |
| `AMBIGUOUS` | 128 | |
| `UNDERDETERMINED` | 235 | |
| `NARROWING_SENSITIVE` | 3 | |
| `UNRESOLVED` | 28 | 184 |
| | | |
| **auto-posted** | **104** (21%) | **314** (63%) |
| **auto-posted WRONG** | **0** | **237** (75% of its postings) |
| held for review | 394 | 184 |
| median latency | 24.7 ms | 8.4 ms |
| throughput | 41,432 / hour | |
| input tokens per 1k | 1.56 M | 1.98 M |
| cost per 1k settlements | 906 INR | 1,134 INR |

The five statuses are above the headline on purpose. `AMBIGUOUS` at 128 and `UNDERDETERMINED` at 235 are real, sized populations rather than rhetoric: 363 settlements where rivals were found or proved to exist. A tool reporting those as matches is reporting a coin flip.

**Manhattan posts fewer, and that is an operating point rather than a concession.** B0's 314 postings contain 237 errors nobody can identify, so all 314 have to be checked and the coverage was worth nothing. 104 postings with 0 errors is 104 a finance team never touches.

**Throughput** is measured end to end: 498 settlements in 43.3 seconds of wall clock, including the agent loop, B0 running alongside, and receipt serialisation. That divides to 86.9 ms per settlement against a 24.7 ms median, and the gap is not an error: the median is *pipeline* time for one settlement, while wall clock also carries generation, the baseline, the agent's re-verification passes and I/O. Both are printed rather than the flattering one.

<details>
<summary><b>The cost row, derived</b></summary>

Priced at published `claude-opus-5` rates, `modelled: no model was billed on this run, so measured token counts are priced at what the live path would have cost`.

| | |
|---|---:|
| input, uncached | 778,419 tok @ $5.00/Mtok |
| input, cache reads | 0 tok @ $0.50/Mtok |
| cache writes | 0 tok @ $6.25/Mtok |
| output | 49,307 tok @ $25.00/Mtok |
| model calls | 1,065 over 498 settlements |
| cache hit rate | 0.0% |
| USD to INR | 88 |
| **Manhattan** | **$10.29 = 906 INR per 1k** |
| **B0** | **$12.89 = 1,134 INR per 1k** |

The cache hit rate is 0.0% because a replay run reports no cache reads, so every input token here is priced at the **uncached** rate. A live run caches the parse system block, byte-identical across every settlement, so the real figure is below the one published. The claim is made against Manhattan deliberately.

**B0's token model, since it decides the comparison.** 200 tokens of instruction plus 40 per candidate record, over a mean narrowed pool of 44.4, giving 1,977 input tokens per settlement against Manhattan's 1,563. Forty tokens covers one candidate rendered as an id, an amount, a timestamp, an instrument and a kind.

That is low on purpose. **B0 is handed Manhattan's narrowing for free**, so it reads a few dozen records rather than the 3645.7 in the mean unnarrowed universe. Without that narrowing it would pay roughly 146,028 tokens per settlement and the gap reported here would be several times wider. A cost advantage argued from a handicapped baseline is not an advantage.

</details>

### The baseline, published so it can be attacked

237 wrong out of 314 posted is 75%, and nobody who has built a fuzzy matcher should accept that on assertion. So here is everything B0 scores on:

- exact integer hit on the target contribution sum (confidence 0.90)
- near hit within 1 basis point of the target (0.72)
- near hit within 1 per cent of the target (0.45)
- no hit found within the node budget (0.15)
- cardinality agrees with the settlement report's declared count (+0.05)

It posts above a threshold of 0.80, the value such tools typically ship with. **[RESULTS.md](RESULTS.md#the-baseline-across-every-threshold) sweeps that threshold across the same 498 decisions.** Two points from it:

| | threshold | posted | wrong | precision | F1 |
|---|---:|---:|---:|---:|---:|
| shipped | 0.80 | 314 | 237 | 25% | 0.19 |
| best F1, where a team tuning it would land | 0.95 | 219 | 151 | 31% | 0.19 |

**The right-hand column is flat at 77 from 0.10 to 0.90 and that is the sharpest fact in the table, not a bug.** B0's correct proposals are exact integer hits, and an exact hit scores at or above 0.90 under its own scoring function. So every threshold below that admits only additional WRONG postings: lowering the bar never finds another right answer, it just posts more. The score carries no information about correctness anywhere in the range a team would actually tune within.

The curve is the argument, not the operating point. **Tuned to its own best F1, B0 still posts 151 wrong out of 219.** Its precision never exceeds 31% anywhere on the curve, because the confidence score measures *how good the match looks*, not *whether it is the only one*, and those come apart exactly where the money is. Raising the threshold does not find the rivals; it posts fewer things without knowing which.

And one number falls out of the sweep that settles the comparison:

> **At no threshold does B0 produce more correct postings than Manhattan.** Its maximum is 77 right answers, at threshold 0.10, against Manhattan's 104. Every extra posting the baseline appears to offer is a wrong one, and it cannot tell you which.

That is why Manhattan's 0 means something. The baseline is not trading accuracy for coverage; it has no more coverage of the truth to trade.

### The rate is predictable before integration

| merchant type | expected regime | spread sigma (paise) | twin mass | auto-post | wrong | B0 posts | B0 wrong |
|---|---|---:|---:|---:|---:|---:|---:|
| travel | wide ticket spread; amounts separate cleanly | 3.44e+06 | 0.00 | **57%** | 0 | 28% | 6% |
| marketplace | amounts separate; a disputes feed is unjoined | 7.7e+05 | 0.00 | **45%** | 0 | 66% | 30% |
| d2c_ecommerce | narrow spread; narrowing decides | 1.74e+05 | 0.00 | **7%** | 0 | 65% | 48% |
| utility_billpay | repeated price points; entropy gate refuses | 4.19e+04 | 0.76 | **0%** | 0 | 83% | 83% |
| subscription_saas | three price points; entropy gate refuses | 6.55e+04 | 0.94 | **0%** | 0 | 65% | 65% |
| quick_commerce | tight spread; a disputes feed is unjoined | 1.85e+04 | 0.00 | **17%** | 0 | 71% | 53% |

Read the right-hand columns against the left. Where amounts genuinely fail to distinguish transactions, Manhattan's auto-post rate falls to zero and B0's wrong-posting rate climbs. Both systems see the same data. One reacts to it.

### About those two zeros

utility billpay and subscription saas post **nothing**, and they are large, fast-growing segments. That number should be read twice rather than once.

A subscription merchant charging 499, 999 and 1999 has settlements built from three repeated price points. Any subset of 143 customers paying 499 produces exactly the same credit as any other subset of 143. **There is no method that reconstructs those batches from amounts.** Not a better solver, not a better model, not more compute. The information is not present. B0 posts 65% of them and is wrong on 65%, which is what confidently answering an unanswerable question looks like.

So the zero is not a gap in coverage, it is the correct answer, delivered in eleven milliseconds with the reason attached and the remedy named. And the remedy is real: **these merchants do not need a better matcher, they need a settlement reference on the credit or a per-payment fee row**, and the receipt says so per settlement rather than in general.

The honest commercial reading, which is in [LIMITATIONS.md](LIMITATIONS.md) and not hidden: on a flat-price merchant this system's amount-based route contributes nothing, and it says so before you integrate rather than after.

This is also the commercial claim: one pass over a merchant's historical settlement amounts yields the spread and the twin mass, which yield the collision index, which yields the expected status mix, before any integration.

**[RESULTS.md](RESULTS.md) carries the calibration that backs it**, and it carries the claim's limits with it. Across all 96 swept configurations the index does **not** order outcomes cleanly, and the reason is that it is being read against the wrong variable. Segmented by batch cardinality, which is the variable it has to be read against, the verified rate is monotone in the index at cardinality 9 and not at 4 and 6.

Where it breaks it breaks in one direction. The index is an *expected* number of colliding subsets, and at small cardinality the enumeration is small enough that the realised count is often one where the expectation is five, so the estimator is conservative exactly where the search is cheapest. **The wrong-posting rate is zero in every band of every cardinality**, so the failure costs recall and never precision.

The commercial claim is scoped to match: the index predicts a merchant's auto-post rate at the cardinalities where refusal actually binds, and under-predicts it below them. That is still a claim worth selling, and it is one that survives its own data.

### One cross-check nobody arranged

`AMOUNT_ENTROPY_INSUFFICIENT` is **166**, which is exactly the 2 zero-auto-post archetypes at 83 settlements each. The flag is set per settlement in the entropy gate; the archetype totals come from a different loop in a different package. Nothing makes them agree, and a miscount in either would show up here.

(There is a second pair, `RESOLVED_BY_HYPOTHESIS` at **32** against **32** agent repairs. They match, and they match **by construction**: every repair sets that flag and nothing else does. That is a consistency assertion, not independent corroboration, and calling it evidence would be overclaiming.)

---

## Architecture

```mermaid
flowchart TB
    subgraph SRC["Four sources, which disagree"]
        PG["PG settlement report<br/><small>payments, fees, tax, counts</small>"]
        BANK["Bank statement<br/><small>free-text narration, one credit</small>"]
        OMS["OMS / ledger<br/><small>orders, gross, GL account</small>"]
        DIS["Disputes feed<br/><small>chargebacks, dispute fees</small>"]
    end

    PG --> S1
    BANK --> S1
    OMS --> S1
    DIS --> S1

    S1["<b>1 · Parse</b><br/>agent reads narration into typed fields"]
    S2["<b>2 · Narrow</b><br/>deterministic business constraints<br/><small>every excluded record logged with its reason</small>"]
    S3["<b>3 · Contribute</b><br/>signed net contribution per record<br/><small>exact integer paise under declared policy</small>"]
    S4["<b>4 · Gate</b><br/>amount entropy, then feasibility<br/><small>outputs k*, the search's own dispatch parameter</small>"]
    S5["<b>5 · Reconstruct</b><br/>cardinality-dispatched meet-in-the-middle<br/><small>witness and complement, one enumeration</small>"]
    S6["<b>6 · Prove</b><br/>count rivals, guard completeness, re-derive the identity"]

    S1 --> S2 --> S3 --> S4 --> S5 --> S6
    S6 --> D{"decision"}

    D -->|"unique, complete, closes"| V["VERIFIED<br/><small>post with evidence</small>"]
    D -->|"rivals exist"| A["AMBIGUOUS<br/><small>both exhibited</small>"]
    D -->|"combinatorially hopeless"| U["UNDERDETERMINED<br/><small>remediation computed</small>"]
    D -->|"a filter decided it"| NS["NARROWING_SENSITIVE<br/><small>constraint named</small>"]
    D -->|"nothing reconstructs it"| UR["UNRESOLVED"]

    A --> S7
    U --> S7
    NS --> S7
    UR --> S7["<b>7 · Agent</b><br/>observe, choose one action, act, re-verify<br/><small>bounded; only a cited action may post</small>"]

    S7 -->|"a real record was cited<br/>and the identity closes uniquely"| V
    S7 -->|"otherwise"| REV["review queue<br/><small>with a proven cure where one was found</small>"]

    V --> EV["Evidence object<br/><small>replayable, diffable, queryable</small>"]
    REV --> EV
    EV --> QA["<b>7b · Q&amp;A agent</b><br/><small>answers grounded in receipts only</small>"]

    style V fill:#e7f5ee,stroke:#0a7d4e,color:#16181d
    style A fill:#fdf3e7,stroke:#a76100,color:#16181d
    style U fill:#f1f3f5,stroke:#6b7480,color:#16181d
    style NS fill:#fdeee7,stroke:#c2410c,color:#16181d
    style UR fill:#f2ecfc,stroke:#6335c4,color:#16181d
    style S7 fill:#eaf1fd,stroke:#1461cc,color:#16181d
    style QA fill:#eaf1fd,stroke:#1461cc,color:#16181d
```

*(Rendered SVGs of every diagram are in [docs/diagrams/](docs/diagrams/), in case the judging surface does not render Mermaid.)*

One property is easy to miss: **the gate runs before the solver, not after it.** Because the gate's output `k*` is the parameter the solver is dispatched on, triage is not a pre-check bolted onto the front of the search, it is what configures the search. The derivations behind the solver, the gate and the completeness probe are in [docs/DESIGN.md](docs/DESIGN.md).

### The dashboard

`./run.sh demo` builds the frontend, runs the batch and serves it on `localhost:8080`. **Nobody should have to run it to judge this**, so the four views that carry the argument are below, and every figure in them is also in [RESULTS.md](RESULTS.md).

| | |
|---|---|
| ![head to head](docs/screenshots/02-head-to-head.jpg) | **Head to head.** One credit, identical inputs to both systems. B0 proposes six records at 0.95 confidence and posts, and is wrong. Manhattan finds a witness, closes the identity to zero, then widens the pool and finds a rival, so it holds. |
| ![calibration](docs/screenshots/03-calibration.jpg) | **Calibration.** Outcome mix against the collision index predicted *before* any search ran, in bands of roughly equal population. Verified gives way to ambiguous and then to refusal as the index climbs, and the wrong-posting rate stays at zero across every band while B0's reaches 23%. |
| ![exceptions](docs/screenshots/06-exceptions.jpg) | **The exception queue.** 394 held settlements grouped by cause, each group priced and carrying the single change that would clear it, then the queue itself ordered by value cleared per analyst hour. |
| ![receipt](docs/screenshots/04-receipt.jpg) | **A receipt.** The full derivation for one settlement: the accounting identity re-derived from raw records, the fee check, and the gateway's own claim checked separately afterwards. `consistent` is not `verified` and the panel says so. |
| ![mobile](docs/screenshots/05-mobile.png) | **At 390px.** Every view is usable on a phone. Wide tables scroll inside their own panel rather than compressing to nothing. |

---

## The five outcomes

| Status | Meaning | Action |
|---|---|---|
| `VERIFIED` | Exactly one explanation exists in the region searched, uniqueness was counted exhaustively, and the accounting identity closes | Auto-post with evidence |
| `AMBIGUOUS` | Two or more distinct explanations reconstruct the credit, and both are exhibited | Review, alternatives shown |
| `UNDERDETERMINED` | The combinatorics guarantee a large population of explanations, so showing one would misrepresent it | Review, with the remedy computed |
| `NARROWING_SENSITIVE` | The answer depended on a filtering decision rather than on the arithmetic | Review, with the constraint named |
| `UNRESOLVED` | Nothing reconstructs the credit within the declared tolerance | Exception queue, with the exact residual |

`AMBIGUOUS` and `UNDERDETERMINED` are genuinely different and merging them would be dishonest. The first means two concrete rivals were found and can be put in front of an analyst who may choose between them on grounds the arithmetic cannot see. The second means the combinatorics guarantee thousands exist, so exhibiting two is pointless and the remedy is more data rather than more thinking.

Flags are **orthogonal** to status: a settlement can be `VERIFIED` and carry `FEE_ANOMALY`, because whether the money is accounted for and whether the fee applied to it was right are different questions.

| flag | settlements |
|---|---:|
| `SIGNED_ITEMS_PRESENT` | 224 |
| `AMOUNT_ENTROPY_INSUFFICIENT` | 166 |
| `LATTICE_CORRECTED` | 69 |
| `RESOLVED_BY_HYPOTHESIS` | 32 |

**The table above covers the 498-settlement batch only.** `TWIN_SWAP` is absent from it and present in adversarial case 5, which is not a contradiction: the batch draws merchant amount distributions that rarely put an exact twin *inside a witness*, while case 5 constructs one deliberately. The eleven cases are a separate fixture for exactly this reason, and a flag absent from a run is not a flag that cannot fire.

---

## The agent

The track asks for an agent. What distinguishes this one is not that a model is in the loop, it is *where* the model sits relative to the decision.

*(The loop as a diagram is in [docs/DESIGN.md](docs/DESIGN.md); the shape is: triage, observe, choose one action, apply it as an overlay, re-run the entire stack, keep the result only if it improved.)*

### What it did, as a flow

The counts do not read as a partition and a reader who tries will conclude something is broken, so here they are in the order they happen:

```
426  settlements entered the loop as unresolved
166  settled by deterministic triage, with no model call        (39%)
260  reached the agent                                          (61%)
567  actions taken
 32  repaired into a posting, each citing a real record
 50  given a proven cure: verified remedy, deliberately not posted
394  remain held for review
  0  wrong postings caused
```

426 in, 32 out as postings, 394 held. That is where the missing 32 went.

**39% of the queue is settled without a model call at all**, because a cheap deterministic check establishes that no action in the vocabulary could change the outcome: the amounts do not distinguish the transactions, or a rival already appears when the pool is widened, or there is nothing left to search or tighten. Paying a model to conclude that nothing can help, across most of a queue, is the same mistake as paying it to add up a column.

### What the agent is worth, and when it is worth nothing

Repairs split by the action that produced them, because a single total hides which mechanism did the work:

| action | repairs | corroborated by |
|---|---:|---|
| `NARROW_TO_HISTORY` | 16 | this merchant's own prior VERIFIED settlements |
| `SEARCH_FEED` | 16 | a real record, cited by id, in a feed nobody joined |

And the honest version of "does the agent matter", as a curve rather than a number. The batch is re-run with the modelled misconfiguration scaled from zero:

| scenario | verified | wrong | repairs | of which narrowing | proven cures |
|---|---:|---:|---:|---:|---:|
| correctly configured, reports clean | 120 | 0 | 16 | 0 | 1 |
| correctly configured, reports as modelled | 110 | 0 | 11 | 0 | 0 |
| window misconfiguration as modelled | 73 | 0 | 18 | 7 | 38 |
| window misconfiguration, reports ten times cleaner | 83 | 0 | 20 | 8 | 36 |
| window misconfiguration twice as bad | 51 | 0 | 11 | 0 | 59 |

Three things to read off it. **At zero misconfiguration the narrowing action repairs 0**, which is correct: a correctly configured deployment has no window slack to recover and an agent that invented some would be dangerous. **Wrong postings are zero in every scenario**, including the ones where the agent is doing the most work. And at the worst misconfiguration the narrowing repairs fall back toward zero, which is a real limitation rather than noise: a merchant so badly configured that it proves almost nothing cannot accumulate the twelve proofs a profile needs, so corroborated narrowing has nothing to corroborate against. It is in [LIMITATIONS.md](LIMITATIONS.md).

### The action space is closed

| Action | What it does | May post? |
|---|---|---|
| `SEARCH_FEED` | looks in a source nobody joined, for a named class of record | **yes, with the citation** |
| `NARROW_TO_HISTORY` | narrows the window to a bound this merchant's own proved settlements demonstrate | **yes, with the history** |
| `TIGHTEN_WINDOW` | narrows the value-date window on the model's judgement alone | no |
| `WIDEN_WINDOW` | loosens it, for a batch partly cut out | no |
| `SPLIT_BY_INSTRUMENT` | restricts to the payout's own payment method | no |
| `RELAX_RECONCILED` | admits records posted in a prior cycle | no |
| `PROPOSE_ADJUSTMENT` | asserts an unmodelled event | no |
| `ESCALATE` | stops, deliberately, with everything tried recorded | no |

**Only a corroborated action may post, and that rule was learned rather than designed.** The first version let narrowing changes post if the identity closed, and produced **two wrong postings in three hundred settlements** (a figure recorded by hand from that earlier build, and the one number in this file not emitted by run `run_20260903_0925`; the failure it describes is rebuilt as a committed test in [`internal/agent/corroboration_test.go`](internal/agent/corroboration_test.go), which fails if `TIGHTEN_WINDOW` is ever made postable again): the agent tightened a window, the pool fell from 44 records to 40, an `AMBIGUOUS` settlement became `VERIFIED`, every check passed, and the answer was wrong because the tightening had cut real records out of the batch.

> Removing candidates cannot make the survivor unique. It makes it **unexamined**.

So narrowing actions are assertions about a merchant's settlement behaviour, and assertions need corroboration. A second rule has the same shape: if the feed holds more candidates of the named class than the agent can afford to test and exactly one of the tested ones verifies, **that still does not post**, because an untested record might have verified too.

**Prohibition was the right default and it was not the right permanent answer.** The gap it left was that the system had no way to establish an assertion about a merchant's behaviour, so it banned the action instead. `NARROW_TO_HISTORY` closes that: a merchant's own prior `VERIFIED` settlements are a second source, and they are a strong one, because each was established by exhaustive enumeration without reference to any window hypothesis. If the last 16-odd proved batches for a merchant all closed inside nine hours, "this merchant settles within nine hours" is a measurement over proofs rather than the model's opinion.

Three properties keep it honest. The profile is built **only** from `VERIFIED` receipts, so it cannot bootstrap from guesses. At least twelve proofs are required before any bound is offered. And the proposed bound may never be **tighter** than the widest offset those proofs actually show, so the agent cannot invent a rule the merchant's own history contradicts. The verifier still decides: the window is applied as an overlay, the whole stack re-runs, and if the result is not unique with the identity closing then nothing posts. Corroboration buys the right to be tested, not the right to be believed.

### The loop closed, end to end

This is adversarial case 9 and it is what the 32 repairs look like. A chargeback exists in a disputes feed nobody wired into the candidate pool, so nothing reconstructs the credit.

The verifier searches under `k(S)` at most 7 and finds nothing, with the nearest achievable sum 1038851 paise away. It hands the agent the exact residual, its sign and its cardinality. The agent answers `CHARGEBACK_DEBIT, add_item`, typed, from a closed vocabulary. The verifier searches the unjoined feed itself, finds `cbk_000223`, applies it, and re-runs the entire stack unmodified. It closes exactly and uniquely, and the hypothesis cites a real record, so it posts as `VERIFIED / RESOLVED_BY_HYPOTHESIS / cites cbk_000223`.

The division of labour is the point. The agent contributes the judgement about *what kind* of event to look for. The system contributes the evidence that one occurred, and it tries **every** candidate of that class against the verifier rather than only the one whose amount the model guessed.

The eleven-case suite passes on a deliberately unintelligent offline stub proposing from a fixed list in a fixed order. The quality of the proposer changes how often an exception clears; it cannot change whether a posting is correct.

**Adversarial cases: 11 of 11 expectations met, Manhattan wrong postings 0, B0 wrong postings 2.** Full table in [RESULTS.md](RESULTS.md).

---

## The exception list is the deliverable

Most systems treat the exception list as an apology. Every entry here carries a status distinguishing *two answers exist and here they both are* from *ten million answers exist* from *a filter decided this*; a named cause traceable to a specific gate, constraint or residual; a **computed** remediation; and a handling estimate priced by what clearing it actually takes.

That last part is the one that makes the queue a work plan rather than a list. Handling time is not flat: it runs from **83 to 933 INR**, a 11.2x spread, because a refusal whose remedy has already been computed and re-verified is a decision somebody makes in five minutes, while a credit that nothing reconstructs, with nothing to act on, is an open-ended investigation. Every term that produced an estimate is named on the receipt in `exception_cost_basis`, so an operations lead can argue with the model rather than with the number.

So the queue is ordered by **value cleared per analyst hour**, which is what "work the most valuable thing first" actually means. Ordering by handling cost alone would put a forty-five minute investigation of a small credit above a five minute data fix on a large one.

**[The full top 15 is in RESULTS.md](RESULTS.md#the-exception-queue); all 394 are in `out/receipts.ndjson`.**

| settlement | status | at stake | mins | INR/hour | cause | computed remedy |
|---|---|---:|---:|---:|---|---|
| `bank_credit_travel_2026_09_02_1030` | `UNDERDETERMINED` | 428,322 | 6 | **4,283,216** | this batch is claimed to be 9 records of a... | supply the settlement reference, or the... |
| `bank_credit_travel_2026_08_07_1004` | `UNDERDETERMINED` | 250,516 | 5 | **3,006,186** | this batch is claimed to be 9 records of a... | supply the settlement reference, or the... |
| `bank_credit_travel_2026_08_13_1010` | `UNDERDETERMINED` | 241,964 | 5 | **2,903,567** | this batch is claimed to be 9 records of a... | supply the settlement reference, or the... |
| `bank_credit_travel_2026_08_15_1012` | `UNDERDETERMINED` | 254,447 | 6 | **2,544,467** | this batch is claimed to be 7 records of a... | supply the settlement reference, or the... |
| `bank_credit_travel_2026_08_18_1015` | `UNDERDETERMINED` | 226,261 | 6 | **2,262,609** | this batch is claimed to be 7 records of a... | supply the settlement reference, or the... |
| `bank_credit_travel_2026_08_12_1009` | `UNDERDETERMINED` | 252,773 | 7 | **2,166,627** | this batch is claimed to be 6 records of a... | supply the settlement reference, or the... |

### What refusing is worth

| | |
|---|---:|
| money sitting unposted in the queue | 12,212,637 INR |
| analyst time to clear it | 139 hours |
| the whole held queue, at the configured rate | **138,689 INR** |
| B0's 237 wrong postings, at 2,400 INR each to unwind | **568,800 INR** |
| difference | **430,111 INR in Manhattan's favour, per 498 settlements** |

The 2,400 INR is an assumption, printed so it can be replaced. It is roughly two hours of a mid-level finance analyst at Indian metro rates: noticing the error, usually at month end and from a reconciliation difference rather than from the posting itself; finding which credit it belonged to; reversing the journal; re-posting; explaining the movement to whoever signs the accounts. It excludes any case that reaches an auditor.

Substitute your own figure; the arithmetic is one multiplication. The conclusion survives a wide range, because B0 only comes out ahead if unwinding a wrong posting costs under 585 INR, which is well under an hour of analyst time for an error nobody knows about yet.

**And that break-even is conservative in our own favour, which is worth saying.** It charges B0 nothing for its own 184 held settlements, which a real team would also have to work. Counting them would move the number further in this direction, so the figure above is a floor rather than a best case.

So 21% is not a philosophical position. It is the cheaper one.

---

## Ask the receipts

The receipt store is queryable, and the Q&A agent answers only from stored evidence. Three exchanges from this run, verbatim:


> **Q. which constraint dropped the most records?**
>
> outside the value-date window removed the most, at 1,390,906 records across 498 settlements, which is 76.6% of everything narrowing looked at.
> 
> The full breakdown:
>   outside the value-date window       1,390,906  (76.6%)
>   a different merchant                  248,004  (13.7%)
>   already posted in a prior cycle       156,704  (8.6%)
>   nets to exactly zero                       52  (0.0%)
> 
> This matters more than it...
>
> `deterministic, no model call`
> `aggregated across the store · narrowing.dropped.outside_settlement_window = 1,390,906`
> `aggregated across the store · narrowing.dropped.mid_mismatch = 248,004`

> **Q. what is the backlog costing us?**
>
> The queue holds 394 settlements worth ₹1,21,46,859.19, and clearing it costs about INR 138,689 at the configured analyst handling time.
> 
> By cause, most expensive first:
>   AMBIGUOUS               128 settlements   INR   92,001
>   UNDERDETERMINED         235 settlements   INR   28,490
>   UNRESOLVED               28 settlements   INR   16,950
>   NARROWING_SENSITIVE       3 settlements   INR    1,248
> 
> Every row carries a...
>
> `deterministic, no model call`
> `aggregated across the store · exception_cost_inr = 138,689`

> **Q. which analyst approved settlement 5502?**
>
> The receipts do not record this and I will not infer it.
> 
> There is no approval, reviewer or assignee field on a receipt, because there is no human step in this pipeline to record. A settlement is decided by an integer identity and an exhaustive uniqueness count, and the receipt carries the evidence for that decision rather than a person's name.
> 
> What a receipt does record, and what I can answer from: the status and...
>
> `deterministic, no model call`
> `schema · no approval, reviewer or assignee field exists`

The third is why any of them are shown. **An agent that answers every question is not grounded in anything.** The receipts record what the system decided and why. They do not record who approved anything, because no human approval step exists in this pipeline, so declining is the correct answer and it is the whole thesis in one exchange.

---

## The receipt

Every decision emits one, including every refusal. It carries the target, the witness, the pool statistics, both collision-index estimators, the uniqueness method and its **scope in words**, every completeness check with its verdict, the rival reconstructions where they exist, the agent's decision trace, the computed remediation, and the exception price.

A full annotated `VERIFIED` receipt and the `UNRESOLVED` and `NARROWING_SENSITIVE` shapes are in [docs/DESIGN.md](docs/DESIGN.md). Every receipt from this run is in `out/receipts.ndjson`, one JSON object per line.

---

## Where the model sits

### On how much of the outcome the model is responsible for

A fair reading of the numbers above is that the model moves 32 settlements out of 498, that 39% of the exception queue never calls it, and that the agent is therefore doing very little. That reading is correct about the arithmetic and wrong about the architecture, and it is worth being direct about which.

**The model is not load-bearing for correctness, deliberately and permanently.** A model that could change whether a settlement is correctly posted would be a model that can put wrong numbers in a general ledger, and no amount of capability fixes that, because the failure is silent. Any design where a better model produces more correct postings is a design where a worse one produces incorrect ones. This system is built so that swapping the model changes throughput and never accuracy, and the eleven-case suite passing on a deliberately unintelligent stub is the proof that it worked.

**The model is load-bearing for the loop closing at all.** Reading `NEFT-RAZORPAY SOFTWARE PVT LTD-UTR3491-CR` into typed fields is not something a solver does. Looking at an unexplained residual and knowing that a chargeback debit is the shape of thing that produces it is not something a solver does. Choosing which of seven actions to try on a stuck settlement, and answering a finance lead's question from a store of receipts, are not things a solver does. Remove the model and there is no pipeline, only a subset-sum library.

**And deciding not to call it is itself an agent design decision**, not an absence of one. 166 deterministic skips is 166 times the system established, cheaply and provably, that no action in its vocabulary could change the outcome. An agent that burns a model call to rediscover that on every item is not more agentic, it is worse engineered and more expensive.

The claim being made is not that the model does little. It is that **the model does the open-ended work and the arithmetic does the deciding, and that this is the only arrangement in which an agent may touch a ledger at all.**

---

The boundary is one interface that speaks only in schemas. No method on it returns free text into a decision path, so the trust boundary is enforced by the type system rather than by discipline.

The model reads unstructured bank narrations, names the class of event behind a residual, orders which constraints to relax first, answers a finance lead's question from receipts, and explains a result in English. **None of those five is arithmetic, and none is available to a solver.** The verifier owns the entropy gate, the feasibility gate, the exhaustive enumeration and its uniqueness count, the completeness guards, and the independent re-derivation of the identity. It owns all of them regardless of what the model proposed.

Structured output uses **strict forced tool use**: the schema is the only tool, `additionalProperties` is closed, every field is required, and the model must call it. Three providers ship: the live Anthropic API when `ANTHROPIC_API_KEY` is set, recording to a cassette; cassette replay with no network, byte for byte; and a deterministic offline stub when neither is available.

**This run used `offline-stub` (parse=replay resolve=replay answer=replay).** Live-path parity is not yet demonstrated at batch scale; see [LIMITATIONS.md](LIMITATIONS.md#no-live-model-run-at-batch-scale). To run it:

```bash
export ANTHROPIC_API_KEY=sk-ant-...
./run.sh bench
```

---

## What this cannot do

The full list is in [LIMITATIONS.md](LIMITATIONS.md). The four that matter most:

**The verification regime is narrow.** Uniqueness is attainable only when free cardinality is roughly 3 to 7 for realistic pool sizes and amount spread. Outside that, `UNDERDETERMINED` is the honest answer, and no solver improvement changes it.

**Uniqueness is proved within a cardinality scope, not absolutely.** Meet-in-the-middle at `k_max` searches `{S : k(S) <= k_max}` completely and nothing beyond. The receipt prints the scope.

**A declared-count dispatch scopes the proof by the counterparty's own claim.** It is cheaper and often the only route to `VERIFIED` at all. It also borrows from the artifact it is meant to be validating. The receipt records `scope_source` so the two claims are never confused.

**The drift monitor catches drift, not original error.** A narrowing constraint wrong since the first run has no baseline to deviate from and is stable under relaxation, so neither the run-level monitor nor the per-settlement probe fires. This is the sharpest remaining hole in the completeness argument.

Benchmarked on synthetic data throughout. The pathology mix reflects documented Razorpay settlement mechanics (paise-denominated amounts, T+2 cycles, MDR with 18% GST, netted refunds, chargeback debits, zero-MDR UPI), but real merchant data will contain things the generator does not model.

---

## For a judge, in four minutes

1. **This page**, to the end of the measured result. Two minutes.
2. **[RESULTS.md](RESULTS.md)**, the calibration section, which answers whether the system knows in advance when it is about to be wrong. One minute.
3. **`./run.sh demo`**, which opens on adversarial case 10: narrowing drops a real record, a coincidental subset closes the identity, and the guard catches it. One minute.

Longer: [docs/EXPLAIN.md](docs/EXPLAIN.md) is the whole system from first principles in plain language; [docs/DESIGN.md](docs/DESIGN.md) has every derivation. The sixty-second walkthrough is scripted in [docs/DEMO-SCRIPT.md](docs/DEMO-SCRIPT.md).

---

## Repository

```
cmd/manhattan/     CLI: bench, cases, recon, ask, serve, docs
internal/
  money/           integer paise; the only numeric type for value
  model/           domain types across the four sources
  accounting/      signed contributions, fee policy, the identity
  narrow/          business constraints, with a full drop log
  entropy/         twin classes, twin mass, lattice gcd, zero contributions
  feasibility/     the collision index, k*, both estimators, the ceiling
  solver/          cardinality-dispatched meet-in-the-middle
  guards/          neighbourhood probe, cross-checks, run-level drift
  evidence/        the receipt, the run object, the store
  pipeline/        the seven stages, and the decision
  llm/             the model boundary: Anthropic, cassette, offline stub
  agent/           narration parser, action space, controller, Q&A
  baseline/        B0, built honestly
  bench/           the benchmark, the calibration sweep, the envelope
  server/          HTTP API and the live run stream
web/               the dashboard: Vite, React, TypeScript, Tailwind
docs/DESIGN.md     the design, the derivations, what was cut and why
docs/EXPLAIN.md    the system from first principles, in plain language
docs/*.tmpl.md     the sources of this file and LIMITATIONS.md
docs/DEMO-SCRIPT.md  the sixty-second walkthrough, shot by shot
docs/diagrams/     rendered SVGs of every Mermaid diagram
RESULTS.md         generated; never typed
```

**Two tests worth reading.** `internal/solver/solve_test.go` verifies 400 randomised configurations against a 2ⁿ brute-force oracle and caught two real bugs before anything was built on it. `internal/bench/cases_test.go` runs all eleven adversarial cases and **fails if B0 posts nothing wrong**, because a suite the baseline survives is not adversarial enough to demonstrate anything.

---

Traditional reconciliation asks: *how confident are we that these records match?* Manhattan asks: *can we prove these records explain the settlement under the accounting rules, and if we cannot, do we know that before we act?*

**No guessed matches. No arbitrary confidence threshold. Proof, exhibited alternatives, or a named and priced reason the proof is unavailable.**

---

Built by **Rishi0507** for the Razorpay AI Buildathon, Track 04. Licensed under the MIT License; see [LICENSE](LICENSE).
