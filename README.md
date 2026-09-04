# Manhattan

**Settlement reconciliation that proves its answers, and refuses when it cannot.**

Razorpay AI Buildathon · Track 04, AI Finance Controller · Multi-source reconciliation

Manhattan posts **406 of 498** settlements automatically (82%) with **0 wrong**, and hands back 92 exceptions each carrying a named cause, a computed remedy and a price.

### Who this is for

Three readers, and the middle one is the buyer.

**A gateway's support desk.** "Why is my payout short by 4,180 rupees" is a ticket with a cost attached. Manhattan answers it from a receipt: here are the records that make up the credit, here is the fee applied to each, here is the chargeback debited this cycle but raised against the last one. That is a settlement dispute closed without an engineer reading a CSV.

**A gateway's own reconciliation, made provable.** Manhattan does not replace Single View Recon. It sits on top of it, checking the mapping the report already ships against the money that actually moved. In this run it contradicted **25 of 29** defective reports while raising **0 false alarms on 469 clean ones**. A report you can prove is a report support never has to argue about.

**A merchant reconciling independently**, where the mapping is absent or unverified: no settlement reference on the narration, a lump credit, two aggregators shipping different formats, historical backfill.

```
git clone <this repo> && cd manhattan
./run.sh demo          # or:  .\run.ps1 demo    or:  make demo
```

No API key required. **Every number in this file, in [RESULTS.md](RESULTS.md) and in [LIMITATIONS.md](LIMITATIONS.md) is emitted by that command**, rendered from one run in one pass so the three cannot drift. Generated from run `run_20260904_0158`, seed `20260826`, on windows/amd64, 4 logical cores, go1.27.0.

---

## For a judge, in four minutes

1. **The table below.** 406 posted, 0 wrong, against a lookup's 498 posted and 29 wrong on identical data.
2. **[The controller](#the-controller)**, where the model reads the whole period and names the root causes. Graded at **100% recall** against operational conditions it was never told about.
3. **[Where the rest of the AI is](#where-the-rest-of-the-ai-is)**: 1,427 calls across 5 jobs, a second one graded at 72%.
4. **[RESULTS.md](RESULTS.md), the calibration section.** Whether the system knows *in advance* when it is about to be wrong.
5. **`./run.sh demo`**, which opens on adversarial case 10: narrowing drops a real record, a coincidental subset closes the identity exactly, and the guard catches it.

Longer: [docs/EXPLAIN.md](docs/EXPLAIN.md) is the system from first principles in plain language. [docs/DESIGN.md](docs/DESIGN.md) has every derivation. [LIMITATIONS.md](LIMITATIONS.md) is what it cannot do, and it is the document I would read first if I were judging this.

---

## The result

Three comparison systems, identical inputs, identical narrowing.

| 498 settlements | B0, fuzzy matcher | B1, trust the report | Manhattan alone | **M1, the composite** |
|---|---:|---:|---:|---:|
| posted | 314 | 498 | 104 | **406** (82%) |
| **posted wrong** | **237** | **29** | **0** | **0** |
| held | 184 | 0 | 394 | 92 |
| defective reports flagged | 0 | 0 of 29 | | **25 of 29** |
| false alarms on 469 clean reports | | | | **0** |

**M1 is the product.** 104 postings are Manhattan's own proofs; 302 are the gateway's claim, checked. It posts 302 more than reconstruction alone and 92 fewer than the lookup, and the ones it holds back are the ones the lookup gets wrong.

### And it works where reconstruction cannot

| merchant type | spread sigma (paise) | twin mass | reconstruction | **M1** |
|---|---:|---:|---:|---:|
| travel | 3.44e+06 | 0.00 | 57% | **95%** |
| marketplace | 7.7e+05 | 0.00 | 45% | **60%** |
| d2c_ecommerce | 1.74e+05 | 0.00 | 7% | **96%** |
| utility_billpay | 4.19e+04 | 0.76 | 0% | **98%** |
| subscription_saas | 6.55e+04 | 0.94 | 0% | **94%** |
| quick_commerce | 1.85e+04 | 0.00 | 17% | **46%** |

Read the last two columns together. **Subscription SaaS and utility billpay reconstruct 0%, and M1 posts 94% and 98% of them.** Those are large, fast-growing segments and the reconstruction-only figure reads as "we cannot help you". It is not.

That is not a better solver. It is a different question:

> **Deriving a batch is a search. Checking a batch somebody claimed is not.**

Reconstruction costs C(n, k) and only decides in a narrow regime. Verifying a claimed batch costs O(claim): do these records exist, do they belong to this merchant, were they already posted in a prior cycle, do their signed contributions sum to the credit, does the count match the declaration. None of it touches the combinatorics. So a subscription merchant with two hundred identical 499-rupee charges has settlements no method can **derive**, and the gateway's mapping for them can still be **checked** in microseconds.

The verdict is deliberately weaker than a proof, and the receipt never blurs them:

| | |
|---|---|
| `VERIFIED` | exactly one batch in the searched region produces this credit, counted exhaustively. Nobody was trusted. |
| `CLAIM_CONSISTENT` | the batch the report named does produce this credit. Others may too; this was checked, not derived. |
| `CLAIM_CONTRADICTED` | the report's own account does not survive checking. Here is the residual and the diagnosis. |
| `CLAIM_UNCHECKABLE` | part of the claim lives in a feed nobody joined. Our problem, not the report's. |

---

## The controller

The track is called AI Finance Controller, and a controller does not reconcile one settlement at a time. It reads the period. Which merchants are degrading, whether 92 exceptions have 92 causes or three, which single change recovers the most held value, and what needs a human this week.

Every input to that is arithmetic and every one is already computed. What is missing is the step that reads 498 receipts and notices that eighty of them are the same problem wearing different reference numbers. **That step is the model's, and it is the only output in this system that works above a single settlement.**

> This period closed with 6 systemic findings across 6 merchant types: UNJOINED_FEED on marketplace; WINDOW_TOO_WIDE on travel; UNJOINED_FEED on quick_commerce; WINDOW_TOO_WIDE on d2c_ecommerce; AMOUNTS_DO_NOT_DISTINGUISH on subscription_saas; AMOUNTS_DO_NOT_DISTINGUISH on utility_billpay. They are ranked by held value below, and each names the figures it was read from. This close was written by the deterministic stub, which applies one fixed rule per merchant and reports only the first that matches, so a merchant with two problems shows one.

| scope | cause | held INR | evidence it cited |
|---|---|---:|---|
| quick_commerce | `UNJOINED_FEED` | 23,033 | 20 settlements where nothing reconstructs the credit and the residual is... |
| marketplace | `UNJOINED_FEED` | 18,206 | 8 settlements where nothing reconstructs the credit and the residual is... |
| travel | `WINDOW_TOO_WIDE` | 2,182 | mean pool of 51 candidates for a mean batch of 6, and refusals are... |
| subscription_saas | `AMOUNTS_DO_NOT_DISTINGUISH` | 665 | twin mass 0.94, above the 0.30 refusal threshold, across 83 settlements |
| d2c_ecommerce | `WINDOW_TOO_WIDE` | 300 | mean pool of 53 candidates for a mean batch of 6, and refusals are... |
| utility_billpay | `AMOUNTS_DO_NOT_DISTINGUISH` | 266 | twin mass 0.76, above the 0.30 refusal threshold, across 83 settlements |

### It is graded

This run injects operational misconfigurations and records exactly what they are. **None of that reaches the model.** It sees status mixes, pool sizes, twin masses, held values and remedy counts, and has to infer the cause the way a controller would.

| | |
|---|---:|
| conditions injected | 4 |
| **identified, on the right merchant** | **4** |
| **recall** | **100%** |
| findings dropped for citing no evidence | 0 |

Findings corresponding to no injected condition: `subscription_saas: AMOUNTS_DO_NOT_DISTINGUISH`, `utility_billpay: AMOUNTS_DO_NOT_DISTINGUISH`. Listed rather than counted against recall, because at least some are true: the flat-price archetypes genuinely cannot be reconstructed from amounts and saying so is correct even though nobody injected it. Deciding which true findings count would be exactly the sort of scoring nobody should accept on assertion.

**The close cannot act.** It posts nothing, narrows nothing, amends no input and alters no receipt. That is precisely why it is the one model output not bounded by a closed action vocabulary: a person reads it and then decides. Everywhere the model *can* influence a posting it is fenced; here it cannot, so it is given the whole period and asked to think.

---

## Where the rest of the AI is

The fair criticism of this project is that arithmetic does the deciding, so what is the model for. Here is the accounting rather than an argument.

| job | calls | what it contributes | graded? |
|---|---:|---|---|
| `control` | 1 | reads the WHOLE period and writes the close: which merchants are degrading, whether four hundred exceptions have four hundred causes or three, which single change recovers the most held value, and what needs a human this week. The only output here that works above a single settlement, and the only one not bounded by a closed action vocabulary, because it cannot act | **yes**, on whether it found the operational conditions this run injected |
| `triage` | 25 | names WHY a report's stated mapping failed its arithmetic check, from a closed vocabulary of five defect classes. The same failed check has several causes needing different remedies, and telling them apart is reading rather than counting | **yes**, against the generator's record of what it injected |
| `plan` | 567 | chooses one action from a closed set of eight for a settlement that did not post, including whether this merchant's own proved history corroborates a tighter window | indirectly: the entire stack re-runs and rejects anything that did not improve |
| `remediate` | 336 | drafts the analyst-facing note: what to do, why it works in terms of what was measured, and what it will not fix. Facts supplied, figures substituted afterwards | no, and it carries no safety risk: the settlement is held either way |
| `parse` | 498 | reads an unstructured bank narration into typed fields. The highest volume and the lowest difficulty, and the one job a gateway would replace with a lookup table tomorrow | indirectly: a mis-parse produces an exception, never a posting |

**One of those is scored against ground truth.** When the claim check fails, the arithmetic is already known: the residual, the missing ids, the count mismatch. What the model contributes is the *diagnosis*, because the same failed check has several causes with completely different remedies. A report short by one record with a residual matching a chargeback, a report naming a payment from last cycle, and a truncated file all fail identically and need three different actions.

The generator records which defect it injected, and the pipeline never sees it, so the model can be graded:

| | |
|---|---:|
| defects diagnosed | 25 |
| **correct** | **18** (72%) |

The errors are 1 `TRUNCATED_MAPPING` read as `OMITTED_DISPUTE` and 6 `OMITTED_DISPUTE` read as `TRUNCATED_MAPPING`: exactly the pair that needs the class of record involved rather than the sign of the residual, which is what the deterministic stub reads and all it reads. That is headroom a real model has, stated as a number rather than a hope, and `manhattan live` is what turns it into a measurement.

**The highest-volume model job is the one with no safety risk at all.** 336 analyst-facing notes: what to do, why it works in terms of what was measured, and what it will **not** fix. Every fact is supplied and every figure is substituted from the receipt afterwards, so a draft containing a digit is rejected wholesale (0 were, this run). A note is attached to a settlement held either way, so the worst a bad draft costs is a confusing sentence in a work queue.

### What the model must not do, and how that is enforced

**The model never decides whether a settlement is correctly posted.** That is settled by an integer identity and an exhaustive count, both of which run unmodified regardless of what the model proposed. The eleven-case suite passes on a deliberately unintelligent stub, which proves the boundary holds and simultaneously proves no published result *requires* a model.

Those are two different claims and this repository owes both. **`manhattan live` measures the difference:**

```
export ANTHROPIC_API_KEY=sk-ant-...
./bin/manhattan live -n 60
```

It runs the same batch on the live API and on the stub, and asserts that wrong postings are **identical** while diagnosis accuracy, repairs and note quality are **free to improve**. If the wrong-posting column moves, the trust boundary has leaked and the command exits non-zero rather than publishing.

**This has not been run yet**, because it needs an API key. Every published figure comes from `offline-stub` (parse=replay resolve=replay answer=replay) and the cost column is modelled at published rates rather than billed. That is stated here, in [LIMITATIONS.md](LIMITATIONS.md#no-live-model-run-at-batch-scale) and in [RESULTS.md](RESULTS.md), because it is the most attackable sentence in the repository and burying it would be worse than having it.

---

## What this run deliberately gets wrong

A reconciliation benchmark on perfectly configured data measures nothing an agent could help with, so two misconfigurations are modelled. Both are things a **deployment gets wrong on its own side**:

- d2c_ecommerce: reconciliation window misconfigured to plus or minus 24 hours
- marketplace: disputes feed never joined into the pool
- quick_commerce: disputes feed never joined into the pool
- travel: reconciliation window misconfigured to plus or minus 22 hours

The obvious criticism is that the author created the problem the agent solves. The answer is the curve below, not an argument.

---

## The agent

Repairs, split by the action that produced them, because one total hides which mechanism worked:

| action | repairs | corroborated by |
|---|---:|---|
| `NARROW_TO_HISTORY` | 16 | this merchant's own prior VERIFIED settlements |
| `SEARCH_FEED` | 16 | a real record, cited by id, in a feed nobody joined |

And the contribution as a function of how bad the configuration is:

| scenario | verified | wrong | repairs | of which narrowing | proven cures |
|---|---:|---:|---:|---:|---:|
| correctly configured, reports clean | 120 | 0 | 16 | 0 | 1 |
| correctly configured, reports as modelled | 110 | 0 | 11 | 0 | 0 |
| window misconfiguration as modelled | 73 | 0 | 18 | 7 | 38 |
| window misconfiguration, reports ten times cleaner | 83 | 0 | 20 | 8 | 36 |
| window misconfiguration twice as bad | 51 | 0 | 11 | 0 | 59 |

**At zero misconfiguration the narrowing action repairs 0.** That is the loop being unnecessary, not failing. **Wrong postings are zero in every scenario**, including where the agent works hardest. And at twice the modelled misconfiguration narrowing repairs fall back to zero for a better reason: a merchant that badly configured proves almost nothing, never reaches the twelve proofs a profile needs, and has no history to corroborate against. That ceiling is in [LIMITATIONS.md](LIMITATIONS.md).

### The queue, as a flow

```
426  settlements entered the loop as unresolved
166  settled by deterministic triage, with no model call        (39%)
260  reached the agent
567  actions taken
 32  repaired into a posting, each citing evidence
 50  given a proven cure: verified remedy, deliberately not posted
394  remain held
  0  wrong postings caused
```

Paying a model to conclude that nothing can help, across most of a queue, is the same mistake as paying it to add up a column.

### Only a corroborated action may post

| action | may post? |
|---|---|
| `SEARCH_FEED` | **yes**, citing a real record by id |
| `NARROW_TO_HISTORY` | **yes**, bounded by this merchant's own proved settlements |
| `TIGHTEN_WINDOW`, `WIDEN_WINDOW`, `SPLIT_BY_INSTRUMENT`, `RELAX_RECONCILED`, `PROPOSE_ADJUSTMENT`, `ESCALATE` | no |

The rule was learned, not designed. The first version let narrowing post if the identity closed, and produced **two wrong postings in three hundred settlements**: the agent tightened a window, the pool fell from 44 records to 40, an `AMBIGUOUS` settlement became `VERIFIED`, every check passed, and the answer was wrong because the tightening had cut real records out of the batch.

> Removing candidates cannot make the survivor unique. It makes it **unexamined**.

That figure is hand-recorded from a build that no longer exists, and it is the only number in this file not emitted by run `run_20260904_0158`. The failure is rebuilt as a committed test in [`internal/agent/corroboration_test.go`](internal/agent/corroboration_test.go), which fails if `TIGHTEN_WINDOW` is ever made postable again.

**Prohibition was the right default and the wrong permanent answer.** `NARROW_TO_HISTORY` closes the gap: a merchant's prior `VERIFIED` settlements are a second source, and a strong one, since each was proved by exhaustive enumeration without reference to any window hypothesis. The profile is built only from proofs, needs at least twelve, and the bound may never be tighter than the widest offset those proofs show. The verifier still decides. Corroboration buys the right to be tested, not the right to be believed.

---

## The five outcomes

| Status | Meaning | Action |
|---|---|---|
| `VERIFIED` | One explanation exists in the searched region, counted exhaustively, and the identity closes | Post with evidence |
| `AMBIGUOUS` | Two or more explanations reconstruct the credit, and both are exhibited | Review, alternatives shown |
| `UNDERDETERMINED` | The combinatorics guarantee a large population of explanations | Review, remedy computed |
| `NARROWING_SENSITIVE` | The answer depended on a filter rather than on arithmetic | Review, constraint named |
| `UNRESOLVED` | Nothing reconstructs the credit within tolerance | Queue, with the exact residual |

Four of the five stop the money and none is a failure. `AMBIGUOUS` at 128 and `UNDERDETERMINED` at 235 are sized populations, not rhetoric. Flags are orthogonal: a settlement can be `VERIFIED` and carry `FEE_ANOMALY`, because whether the money is accounted for and whether the fee applied to it was right are different questions.

| flag | settlements |
|---|---:|
| `SIGNED_ITEMS_PRESENT` | 224 |
| `AMOUNT_ENTROPY_INSUFFICIENT` | 166 |
| `LATTICE_CORRECTED` | 69 |
| `RESOLVED_BY_HYPOTHESIS` | 32 |

**That table covers the 498-settlement batch only.** `TWIN_SWAP` is absent from it and present in adversarial case 5: the batch rarely puts an exact twin inside a witness, while case 5 constructs one deliberately. The eleven cases are a separate fixture for exactly this reason.

---

## Architecture

```mermaid
flowchart TB
    subgraph SRC["Four sources, which disagree"]
        PG["PG settlement report"]
        BANK["Bank statement<br/><small>free text, one credit</small>"]
        OMS["OMS / ledger"]
        DIS["Disputes feed"]
    end

    PG --> S1
    BANK --> S1
    OMS --> S1
    DIS --> S1

    S1["<b>1 Parse</b><br/>model reads narration into typed fields"]
    S2["<b>2 Narrow</b><br/>business constraints, every drop logged"]
    S3["<b>3 Contribute</b><br/>signed net contribution, integer paise"]
    S4["<b>4 Gate</b><br/>amount entropy, then feasibility<br/><small>outputs k*, the solver's dispatch parameter</small>"]
    S5["<b>5 Reconstruct</b><br/>cardinality-dispatched meet in the middle"]
    S6["<b>6 Prove</b><br/>count rivals, guard completeness, re-derive the identity"]

    S1 --> S2 --> S3 --> S4 --> S5 --> S6
    S6 --> D{"decision"}

    D -->|"unique, complete, closes"| V["VERIFIED"]
    D -->|"rivals exist"| A["AMBIGUOUS"]
    D -->|"combinatorially hopeless"| U["UNDERDETERMINED"]
    D -->|"a filter decided it"| NS["NARROWING_SENSITIVE"]
    D -->|"nothing reconstructs it"| UR["UNRESOLVED"]

    A --> S7
    U --> S7
    NS --> S7
    UR --> S7["<b>7 Agent</b><br/>observe, choose one action, act, re-verify<br/><small>only a corroborated action may post</small>"]

    S7 -->|"corroborated"| V
    S7 -->|otherwise| CLAIM

    V --> CLAIM["<b>8 Check the report's claim</b><br/><small>separate entry point; the search never saw it</small>"]
    CLAIM -->|"consistent"| POST["post"]
    CLAIM -->|"contradicted"| DX["<b>9 Diagnose</b><br/><small>model names the defect class, graded</small>"]
    DX --> REV
    CLAIM -->|"uncheckable"| REV["review queue<br/><small>with a drafted, sendable note</small>"]

    V --> POST
    POST --> EV["Evidence object<br/><small>replayable, diffable, queryable</small>"]
    REV --> EV
    EV --> QA["<b>10 Q and A</b><br/><small>grounded in receipts only</small>"]

    style V fill:#e7f5ee,stroke:#0a7d4e,color:#16181d
    style POST fill:#e7f5ee,stroke:#0a7d4e,color:#16181d
    style A fill:#fdf3e7,stroke:#a76100,color:#16181d
    style U fill:#f1f3f5,stroke:#6b7480,color:#16181d
    style NS fill:#fdeee7,stroke:#c2410c,color:#16181d
    style UR fill:#f2ecfc,stroke:#6335c4,color:#16181d
    style S7 fill:#eaf1fd,stroke:#1461cc,color:#16181d
    style DX fill:#eaf1fd,stroke:#1461cc,color:#16181d
    style QA fill:#eaf1fd,stroke:#1461cc,color:#16181d
```

*(Rendered SVGs of every diagram are in [docs/diagrams/](docs/diagrams/), in case Mermaid does not render here.)*

### The dashboard

`./run.sh demo` builds the frontend, runs the batch and serves it on `localhost:8080`. **Nobody should have to run it to judge this**, so the views carrying the argument are below and every figure in them is also in [RESULTS.md](RESULTS.md).

| | |
|---|---|
| ![the close](docs/screenshots/00-the-close.jpg) | **The close, and its grade.** The landing tab, because it answers the question an operations lead arrives with. The model read the period, named the root causes, and was scored on whether it found conditions it was never told about. |
| ![root causes](docs/screenshots/08-root-causes.jpg) | **Root causes, ranked by held value**, each citing the figures it was read from, each with the action it implies. Plus what needs a human, and what the close says it cannot tell. |
| ![head to head](docs/screenshots/02-head-to-head.jpg) | **Head to head.** One credit, identical inputs. B0 proposes six records at 0.95 confidence and posts, and is wrong. Manhattan closes the identity to zero, then widens the pool, finds a rival, and holds. |
| ![diagnosis](docs/screenshots/07-diagnosis-and-note.jpg) | **The model at work.** The gateway's claim fails its arithmetic check with an exact residual. The model names the defect class, the system owns the remedy for that class, and a drafted note tells an analyst what to do and what it will not fix. |
| ![receipt](docs/screenshots/04-receipt.jpg) | **A consistent claim.** `consistent` is not `verified` and the panel says so: the named batch does produce this credit, others may too, and none was searched for. |
| ![calibration](docs/screenshots/03-calibration.jpg) | **Calibration.** Outcome mix against the collision index predicted *before* any search ran, in bands of equal population. Wrong-posting rate stays at zero across every band. |
| ![exceptions](docs/screenshots/06-exceptions.jpg) | **The queue**, grouped by cause and ordered by value cleared per analyst hour. |
| ![mobile](docs/screenshots/05-mobile.png) | **At 390px.** Every view is usable on a phone; wide tables scroll inside their own panel. |

**The gate runs before the solver, not after it.** Its output `k*` is the parameter the solver is dispatched on, so triage is not a pre-check bolted onto the front of the search. It is what configures the search. Derivations in [docs/DESIGN.md](docs/DESIGN.md).

---

## The exception queue

Every entry carries a status distinguishing *two answers exist* from *ten million exist* from *a filter decided this*; a named cause; a **computed** remedy; a drafted note; and a handling estimate priced by what clearing it takes. That last part is what makes it a work plan: handling runs **83 to 933 INR**, a 11.2x spread, and every term is on the receipt in `exception_cost_basis`.

So it is ordered by **value cleared per analyst hour**. [Full top 15 in RESULTS.md](RESULTS.md#the-exception-queue); all 394 in `out/receipts.ndjson`.

| settlement | status | at stake | mins | INR/hour | cause |
|---|---|---:|---:|---:|---|
| `bank_credit_travel_2026_09_02_1030` | `UNDERDETERMINED` | 428,322 | 6 | **4,283,216** | this batch is claimed to be 9 records of a... |
| `bank_credit_travel_2026_08_07_1004` | `UNDERDETERMINED` | 250,516 | 5 | **3,006,186** | this batch is claimed to be 9 records of a... |
| `bank_credit_travel_2026_08_13_1010` | `UNDERDETERMINED` | 241,964 | 5 | **2,903,567** | this batch is claimed to be 9 records of a... |
| `bank_credit_travel_2026_08_15_1012` | `UNDERDETERMINED` | 254,447 | 6 | **2,544,467** | this batch is claimed to be 7 records of a... |
| `bank_credit_travel_2026_08_18_1015` | `UNDERDETERMINED` | 226,261 | 6 | **2,262,609** | this batch is claimed to be 7 records of a... |

### What refusing is worth

| | |
|---|---:|
| money sitting unposted | 12,212,637 INR |
| analyst time to clear it | 139 hours |
| the queue, at the configured rate | **138,689 INR** |
| B0's 237 wrong postings at 2,400 INR each to unwind | **568,800 INR** |
| difference | **430,111 INR in Manhattan's favour** |

The 2,400 INR is an assumption, printed so it can be replaced: roughly two hours of a mid-level analyst noticing the error at month end, finding the credit, reversing the journal, re-posting, and explaining the movement. Substitute your own; B0 only wins if unwinding costs under 585 INR. **And that break-even is conservative in our favour**, because it charges B0 nothing for its own 184 held settlements.

---

## The baseline, published so it can be attacked

237 wrong of 314 posted is a number my own code produced about my own code, so here is everything B0 scores on:

- exact integer hit on the target contribution sum (confidence 0.90)
- near hit within 1 basis point of the target (0.72)
- near hit within 1 per cent of the target (0.45)
- no hit found within the node budget (0.15)
- cardinality agrees with the settlement report's declared count (+0.05)

[RESULTS.md sweeps its threshold](RESULTS.md#the-baseline-across-every-threshold) across the same 498 decisions. **Its right-hand column is flat at 77 correct from 0.10 to 0.90, and that is the sharpest fact in the table.** B0's correct proposals are exact integer hits, which score at or above 0.90 under its own function, so every lower threshold admits only additional *wrong* postings. The score carries no information about correctness anywhere a team would tune.

> **At no threshold does B0 produce more correct postings than Manhattan.** Its maximum is 77, against M1's 406. Every extra posting it appears to offer is a wrong one it cannot identify.

---

## Ask the receipts

4 exchanges from this run, verbatim, showing every path the Q&A side can take.


> **Q. which constraint dropped the most records?**
>
> outside the value-date window removed the most, at 1,390,906 records across 498 settlements, which is 76.6% of everything narrowing looked at.
> 
> The full breakdown:
>   outside the value-date window       1,390,906  (76.6%)
>   a different merchant                  248,004  (13.7%)
>   already posted in a...
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
>   UNRESOLVED  ...
>
> `deterministic, no model call`
> `aggregated across the store · exception_cost_inr = 138,689`

> **Q. why do the quick commerce settlements behave differently from the travel ones, and what would I change first?**
>
> Across the 12 receipts retrieved for this question, the most common outcome is AMBIGUOUS.
> 
>   ambiguous              8
>   unresolved             4
> 
> This is the deterministic stub. It can count what the receipts say and quote a remedy they already carry. It cannot weigh two remedies against each...
>
> `retrieval over receipts, then one grounded model call`
> `bank_credit_quick_commerce_2026_08_21_1019 · status`
> `bank_credit_quick_commerce_2026_09_01_1030 · status`

> **Q. which analyst approved settlement 5502?**
>
> The receipts do not record this and I will not infer it.
> 
> There is no approval, reviewer or assignee field on a receipt, because there is no human step in this pipeline to record. A settlement is decided by an integer identity and an exhaustive uniqueness count, and the receipt carries the evidence...
>
> `deterministic, no model call`
> `schema · no approval, reviewer or assignee field exists`

The last one matters most. **An agent that answers every question is not grounded in anything.** The receipts record what the system decided and why; they do not record who approved anything, because no human approval step exists in this pipeline.

---

## Track compliance

| requirement | what this run did |
|---|---|
| 50+ record batch | **21,965 source records** across four feeds, driving **498 settlements**. Each settlement's universe reaches **3,778** before narrowing and **95** after |
| one loop, closed | bank credit to posted ledger entry, or to a named and priced exception |
| match rate reported | **406 of 498**, 82%, with **0 wrong** |
| exceptions it could not resolve | **92**, each with a cause, a computed remedy, a price and a drafted note |
| throughput | **45,945 settlements per hour**, 23.4 ms median pipeline time |
| agentic design | 1,427 model calls across 5 jobs, a closed 8-action controller loop, cross-settlement merchant memory, and a graded diagnosis |

Throughput is end to end: 498 settlements in 39.0 s of wall clock including the agent loop, both baselines and receipt serialisation. That divides to 78.4 ms against a 23.4 ms median pipeline time, and both are printed rather than the flattering one. Memory: **200 MB** deterministic solver peak; **343 MB** sampled process heap, which moves between runs and should never be quoted as a bound.

**Determinism is per commit.** Same seed and same commit gives the same decisions on every settlement. Timings are measurements and move, so receipts are not byte-identical.

---

## What this cannot do

The full list is [LIMITATIONS.md](LIMITATIONS.md). The four that matter:

**Reconstruction has a narrow regime.** Uniqueness is attainable only when free cardinality is roughly 3 to 7. Outside it, `UNDERDETERMINED` is the honest answer and no solver improvement changes that. The claim check is what reaches past it.

**The claim check's false-alarm rate is optimistic by construction.** The generator's fee model and the verifier's contribution model are the same model, so a defect-free report that disagrees with our arithmetic over a fee slab or a rounding convention is a failure this benchmark cannot produce.

**The report defect rate is a modelling choice.** 5.8%, chosen here, not observed. The sensitivity sweep varies it down to a tenth so the volume argument does not rest on it. The structural point does not move at all, because a reconciliation whose only check on the report is the report detects a defective one at no rate.

**No live model run at batch scale.** Every figure is offline-stub and the cost is modelled. `manhattan live` exists to close this and needs a key.

Benchmarked on synthetic data throughout. The pathology mix reflects documented Razorpay mechanics (paise amounts, T+2 cycles, MDR with 18% GST, netted refunds, chargeback debits, zero-MDR UPI), but real merchant data will contain things the generator does not model.

---

## Repository

```
cmd/manhattan/     CLI: bench, cases, recon, ask, serve, docs, live
internal/
  money/           integer paise; the only numeric type for value
  accounting/      signed contributions, fee policy, the identity
  narrow/          business constraints, with a full drop log
  entropy/         twin classes, twin mass, lattice gcd, zero contributions
  feasibility/     the collision index, k*, both estimators
  solver/          cardinality-dispatched meet in the middle
  guards/          neighbourhood probe, cross-checks, run-level drift
  pipeline/        the stages, the decision, and CheckClaim
  agent/           parser, action space, controller, memory, diagnosis,
                   note drafting, Q and A
  llm/             the model boundary: Anthropic, cassette, offline stub
  baseline/        B0 and B1, both built honestly
  bench/           the benchmark, calibration sweep, sensitivity, envelope
web/               the dashboard: Vite, React, TypeScript, Tailwind
docs/              DESIGN, EXPLAIN, DEMO-SCRIPT, templates, diagrams
```

**Three tests worth reading.** `internal/solver/solve_test.go` verifies 400 randomised configurations against a 2ⁿ brute-force oracle and caught two real bugs before anything was built on it. `internal/bench/cases_test.go` runs all eleven adversarial cases and **fails if B0 posts nothing wrong**, because a suite the baseline survives is not adversarial. `internal/agent/corroboration_test.go` is the posting rule as an assertion rather than an anecdote.

---

Traditional reconciliation asks *how confident are we that these records match?* Manhattan asks *can we prove these records explain the settlement, and if we cannot, do we know that before we act?*

**No guessed matches. No confidence threshold. Proof, a checked claim, exhibited alternatives, or a named and priced reason none is available.**

Built by **Rishi0507** for the Razorpay AI Buildathon, Track 04. MIT licensed; see [LICENSE](LICENSE).
