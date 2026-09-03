# Manhattan

**An agent that proves settlements instead of guessing them.**

Razorpay AI Buildathon · Track 04, AI Finance Controller · Multi-source reconciliation

Manhattan reconciles payment settlements. It never guesses about money, because the instrument it reconciles with is a solver rather than a similarity score. It either produces an exact, auditable reconstruction under declared accounting rules together with a proof that no rival reconstruction exists, or it names precisely which property it could not establish and routes the settlement to review.

```
git clone <this repo> && cd manhattan
./run.sh demo          # or:  .\run.ps1 demo    or:  make demo
```

No API key required. **Every number in this file, in [RESULTS.md](RESULTS.md) and in [LIMITATIONS.md](LIMITATIONS.md) is emitted by that command.** None of the three is typed by hand; all three are rendered from the same run, in the same command, so they cannot drift apart. This one is generated from run `{{ .S.RunID }}`, seed `{{ .S.Seed }}`.

### Track compliance, stated first

The brief asks for one finance-ops loop closed across a batch of 50 or more synthetic records, reporting match rate and the exceptions it could not resolve.

| requirement | what this run did |
|---|---|
| 50+ record batch | **{{ n .S.Pools.TotalRecords }} records** across **{{ .S.Settlements }} settlements**, pools reaching **{{ .S.Pools.RawMax }} records** before narrowing and **{{ .S.Pools.NarrowedMax }}** after |
| one loop, closed | bank credit to posted ledger entry, or to a named exception, end to end |
| match rate reported | **{{ .S.AutoPosted }} of {{ .S.Settlements }}** auto-posted, {{ pct .D.PostRate }}, with **{{ .S.AutoPostedWrong }} wrong** |
| exceptions it could not resolve | **{{ .S.Exceptions }}**, each with a named cause, a computed remedy and a price. [The list is below.](#the-exception-list-is-the-deliverable) |
| throughput | **{{ i .S.PerHour }} settlements per hour**, {{ f1 .S.MedianLatencyMS }} ms median, peak {{ i .S.PeakMemoryMB }} MB |

---

## First question first: doesn't the settlement report already tell you?

It is the first thing anyone at a payments company asks, so it goes at the top.

For Razorpay's own settlements it largely does: each `settlement_id` maps to its payments, and Optimizer's Single View Recon surfaces that mapping even for externally processed transactions. Where it is present and trusted, this leg is a lookup and the solver is unnecessary. The solver earns its place where that mapping is absent or unverified: a bank credit whose narration carries no usable settlement reference; a merchant reconciling their own OMS against a lump credit; multi-gateway merchants where one aggregator ships a transaction-level mapping and another ships only a net figure; historical backfill, migrations and disputed periods.

And one more, which is the strongest framing. *The settlement report is a claim. Manhattan verifies the claim independently, from the merchant's own records.* A reconciliation system that trusts its input is not reconciling, it is transcribing.

The demo posture reflects that: per-payment fee rows are retained and the `settlement_id` mapping is **withheld**, so the system is not handed the answer it is meant to derive.

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

{{ .S.Settlements }} settlements across {{ len .S.ByArchetype }} merchant archetypes. B0 is a deliberately competent confidence matcher given identical inputs and identical narrowing; it is what most tooling in this space reduces to once the marketing is removed.

| {{ .S.Settlements }} settlements | Manhattan | B0 |
|---|---:|---:|
| `VERIFIED` | **{{ .D.StatusVerified }}** | |
| `AMBIGUOUS` | {{ .D.StatusAmbiguous }} | |
| `UNDERDETERMINED` | {{ .D.StatusUnderdetermined }} | |
| `NARROWING_SENSITIVE` | {{ .D.StatusNarrowing }} | |
| `UNRESOLVED` | {{ .D.StatusUnresolved }} | {{ .S.B0Unresolved }} |
| | | |
| **auto-posted** | **{{ .S.AutoPosted }}** ({{ pct .D.PostRate }}) | **{{ .S.B0Posted }}** ({{ pct .D.B0PostRate }}) |
| **auto-posted WRONG** | **{{ .S.AutoPostedWrong }}** | **{{ .S.B0PostedWrong }}** ({{ pct .D.B0WrongOfPosted }} of its postings) |
| held for review | {{ .S.Exceptions }} | {{ .S.B0Unresolved }} |
| median latency | {{ f1 .S.MedianLatencyMS }} ms | {{ f1 .S.B0MedianMS }} ms |
| throughput | {{ i .S.PerHour }} / hour | |
| input tokens per 1k | {{ f2 (div .S.AxiomTokPer1k 1e6) }} M | {{ f2 (div .S.B0TokensPer1k 1e6) }} M |
| cost per 1k settlements | {{ i .D.INRPer1k }} INR | {{ i .D.B0INRPer1k }} INR |

The five statuses are above the headline on purpose. `AMBIGUOUS` at {{ .D.StatusAmbiguous }} and `UNDERDETERMINED` at {{ .D.StatusUnderdetermined }} are real, sized populations rather than rhetoric: {{ add .D.StatusAmbiguous .D.StatusUnderdetermined }} settlements where rivals were found or proved to exist. A tool reporting those as matches is reporting a coin flip.

**Manhattan posts fewer, and that is an operating point rather than a concession.** B0's {{ .S.B0Posted }} postings contain {{ .S.B0PostedWrong }} errors nobody can identify, so all {{ .S.B0Posted }} have to be checked and the coverage was worth nothing. {{ .S.AutoPosted }} postings with {{ .S.AutoPostedWrong }} errors is {{ .S.AutoPosted }} a finance team never touches.

**Throughput** is measured end to end: {{ .S.Settlements }} settlements in {{ f1 .S.WallClockS }} seconds of wall clock, including the agent loop, B0 running alongside, and receipt serialisation. That divides to {{ f1 .D.MsPerSettlement }} ms per settlement against a {{ f1 .S.MedianLatencyMS }} ms median, and the gap is not an error: the median is *pipeline* time for one settlement, while wall clock also carries generation, the baseline, the agent's re-verification passes and I/O. Both are printed rather than the flattering one.

<details>
<summary><b>The cost row, derived</b></summary>

Priced at published `{{ .S.Cost.Model }}` rates, `{{ if .S.PriceIsReal }}actual spend{{ else }}modelled: no model was billed on this run, so measured token counts are priced at what the live path would have cost{{ end }}`.

| | |
|---|---:|
| input, uncached | {{ n .S.Cost.UncachedInput }} tok @ ${{ f2 .S.Cost.InputUSDPerMTok }}/Mtok |
| input, cache reads | {{ n .S.Cost.CachedInput }} tok @ ${{ f2 .S.Cost.CacheReadUSD }}/Mtok |
| cache writes | {{ n .S.Cost.CacheWrite }} tok @ ${{ f2 .S.Cost.CacheWriteUSD }}/Mtok |
| output | {{ n .S.Cost.Output }} tok @ ${{ f2 .S.Cost.OutputUSDPerMTok }}/Mtok |
| model calls | {{ n .S.Cost.Calls }} over {{ .S.Settlements }} settlements |
| cache hit rate | {{ pct1 (mul .S.Cost.CacheHitRate 100) }} |
| USD to INR | {{ i .S.Cost.USDToINR }} |
| **Manhattan** | **${{ f2 .D.USDPer1k }} = {{ i .D.INRPer1k }} INR per 1k** |
| **B0** | **${{ f2 .D.B0USDPer1k }} = {{ i .D.B0INRPer1k }} INR per 1k** |

The cache hit rate is {{ pct1 (mul .S.Cost.CacheHitRate 100) }} because a replay run reports no cache reads, so every input token here is priced at the **uncached** rate. A live run caches the parse system block, byte-identical across every settlement, so the real figure is below the one published. The claim is made against Manhattan deliberately.

**B0's token model, since it decides the comparison.** {{ .S.Cost.B0Overhead }} tokens of instruction plus {{ .S.Cost.B0PerRecord }} per candidate record, over a mean narrowed pool of {{ f1 .S.Cost.B0MeanPoolN }}, giving {{ i .S.Cost.B0TokensPerCall }} input tokens per settlement against Manhattan's {{ i .D.InputPerSettl }}. Forty tokens covers one candidate rendered as an id, an amount, a timestamp, an instrument and a kind.

That is low on purpose. **B0 is handed Manhattan's narrowing for free**, so it reads a few dozen records rather than the {{ f1 .S.Pools.RawMean }} in the mean unnarrowed universe. Without that narrowing it would pay roughly {{ i .D.UnnarrowedTokens }} tokens per settlement and the gap reported here would be several times wider. A cost advantage argued from a handicapped baseline is not an advantage.

</details>

### The baseline, published so it can be attacked

{{ .S.B0PostedWrong }} wrong out of {{ .S.B0Posted }} posted is {{ pct .D.B0WrongOfPosted }}, and nobody who has built a fuzzy matcher should accept that on assertion. So here is everything B0 scores on:

{{ range .S.B0Features }}- {{ . }}
{{ end }}
It posts above a threshold of {{ f2 .D.B0Shipped.Threshold }}, the value such tools typically ship with. **[RESULTS.md](RESULTS.md#the-baseline-across-every-threshold) sweeps that threshold across the same {{ .S.Settlements }} decisions.** Two points from it:

| | threshold | posted | wrong | precision | F1 |
|---|---:|---:|---:|---:|---:|
| shipped | {{ f2 .D.B0Shipped.Threshold }} | {{ .D.B0Shipped.Posted }} | {{ .D.B0Shipped.Wrong }} | {{ rate .D.B0Shipped.Precision }} | {{ f2 .D.B0Shipped.F1 }} |
| best F1, where a team tuning it would land | {{ f2 .D.B0Best.Threshold }} | {{ .D.B0Best.Posted }} | {{ .D.B0Best.Wrong }} | {{ rate .D.B0Best.Precision }} | {{ f2 .D.B0Best.F1 }} |

The curve is the argument, not the operating point. **Tuned to its own best F1, B0 still posts {{ .D.B0Best.Wrong }} wrong out of {{ .D.B0Best.Posted }}.** Its precision never exceeds {{ rate .D.B0Best.Precision }} anywhere on the curve, because the confidence score measures *how good the match looks*, not *whether it is the only one*, and those come apart exactly where the money is. Raising the threshold does not find the rivals; it posts fewer things without knowing which.

And one number falls out of the sweep that settles the comparison:

> **At no threshold does B0 produce more correct postings than Manhattan.** Its maximum is {{ .D.B0MaxRight }} right answers, at threshold {{ f2 .D.B0MaxRightAt }}, against Manhattan's {{ .S.AutoPosted }}. Every extra posting the baseline appears to offer is a wrong one, and it cannot tell you which.

That is why Manhattan's {{ .S.AutoPostedWrong }} means something. The baseline is not trading accuracy for coverage; it has no more coverage of the truth to trade.

### The rate is predictable before integration

| merchant type | expected regime | spread sigma (paise) | twin mass | auto-post | wrong | B0 posts | B0 wrong |
|---|---|---:|---:|---:|---:|---:|---:|
{{- range .S.ByArchetype }}
| {{ .Archetype }} | {{ .ExpectedRegime }} | {{ f3g .MeanSigmaPaise }} | {{ f2 .MeanTwinMass }} | **{{ rate .AutoPostRate }}** | {{ .AutoPostedWrong }} | {{ rate .B0PostRate }} | {{ rate .B0WrongRate }} |
{{- end }}

Read the right-hand columns against the left. Where amounts genuinely fail to distinguish transactions, Manhattan's auto-post rate falls to zero and B0's wrong-posting rate climbs. Both systems see the same data. One reacts to it.

This is also the commercial claim: one pass over a merchant's historical settlement amounts yields the spread and the twin mass, which yield the collision index, which yields the expected status mix, before any integration.

**[RESULTS.md](RESULTS.md) carries the calibration that backs it**: the collision-index band table, the two-estimator comparison, and the full {{ len .Sweep }}-configuration sweep. That section answers the track's measured-accuracy bar and is the thing separating this from a solver demo, because it says whether the system knows in advance when it is about to be wrong.

### Two cross-checks nobody arranged

Counters incremented in different packages, for different reasons, that agree anyway. `RESOLVED_BY_HYPOTHESIS` is **{{ .D.HypothesisFlag }}** and the agent repaired **{{ .S.AgentRepaired }}** settlements{{ if .D.AgentRepairedCrossFlag }}, matching exactly as they must, since every repair carries that flag and nothing else sets it{{ else }}, which differ and is a defect being tracked{{ end }}. `AMOUNT_ENTROPY_INSUFFICIENT` is **{{ .D.EntropyFlag }}**, exactly the {{ .D.EntropyArchCount }} zero-auto-post archetypes at {{ .D.PerArchetype }} settlements each. Free evidence that the instrumentation agrees with itself, and it would have caught a miscount in either direction.

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

`./run.sh demo` builds the frontend, runs the batch and serves it on
`localhost:8080`.

| | |
|---|---|
| ![head to head](docs/screenshots/02-head-to-head.jpg) | **Head to head.** One credit, identical inputs to both systems. B0 proposes six records at 0.95 confidence and posts, and is wrong. Manhattan finds a witness, closes the identity to zero, then widens the pool and finds a rival, so it holds. |
| ![calibration](docs/screenshots/03-calibration.jpg) | **Calibration.** Outcome mix against the collision index predicted *before* any search ran. Verified gives way to ambiguous and then to refusal as the index climbs, and the wrong-posting rate stays at zero across every band while B0's reaches {{ pct .D.TopBandB0Wrong }}. |
| ![receipt](docs/screenshots/04-receipt.jpg) | **A receipt.** The full derivation for one settlement: the narrowing waterfall with a reason per dropped record, both collision-index estimators plotted against the refusal threshold, the witness, the completeness checks and the identity. |
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
{{- range .D.FlagRows }}
| `{{ .Flag }}` | {{ .Count }} |
{{- end }}

{{ if not .D.TwinSwapInCases }}`TWIN_SWAP` does not appear in this batch and does in adversarial case 5, which is not a contradiction: the batch generator draws merchant amount distributions that rarely produce an exact twin *inside a witness*, while case 5 constructs one deliberately. A flag absent from a run is not a flag that cannot fire, which is why the case suite exists separately from the batch.{{ end }}

---

## The agent

The track asks for an agent. What distinguishes this one is not that a model is in the loop, it is *where* the model sits relative to the decision.

*(The loop as a diagram is in [docs/DESIGN.md](docs/DESIGN.md); the shape is: triage, observe, choose one action, apply it as an overlay, re-run the entire stack, keep the result only if it improved.)*

```

### What it did, as a flow

The counts do not read as a partition and a reader who tries will conclude something is broken, so here they are in the order they happen:

```
{{ .D.ExceptionsEntered }}  settlements entered the loop as unresolved
{{ printf "%3d" .S.AgentSkipped }}  settled by deterministic triage, with no model call        ({{ pct .D.TriagePct }})
{{ printf "%3d" .S.AgentInvoked }}  reached the agent                                          ({{ pct .D.AgentPct }})
{{ printf "%3d" .S.AgentSteps }}  actions taken
{{ printf "%3d" .S.AgentRepaired }}  repaired into a posting, each citing a real record
{{ printf "%3d" .S.AgentProvenCures }}  given a proven cure: verified remedy, deliberately not posted
{{ printf "%3d" .S.Exceptions }}  remain held for review
  {{ printf "%d" .S.AutoPostedWrong }}  wrong postings caused
```

{{ .D.ExceptionsEntered }} in, {{ .S.AgentRepaired }} out as postings, {{ .S.Exceptions }} held. That is where the missing {{ .S.AgentRepaired }} went.

**{{ pct .D.TriagePct }} of the queue is settled without a model call at all**, because a cheap deterministic check establishes that no action in the vocabulary could change the outcome: the amounts do not distinguish the transactions, or a rival already appears when the pool is widened, or there is nothing left to search or tighten. Paying a model to conclude that nothing can help, across most of a queue, is the same mistake as paying it to add up a column.

### The action space is closed

| Action | What it does | May post? |
|---|---|---|
| `SEARCH_FEED` | looks in a source nobody joined, for a named class of record | **yes, with the citation** |
| `TIGHTEN_WINDOW` | narrows the value-date window | no |
| `WIDEN_WINDOW` | loosens it, for a batch partly cut out | no |
| `SPLIT_BY_INSTRUMENT` | restricts to the payout's own payment method | no |
| `RELAX_RECONCILED` | admits records posted in a prior cycle | no |
| `PROPOSE_ADJUSTMENT` | asserts an unmodelled event | no |
| `ESCALATE` | stops, deliberately, with everything tried recorded | no |

**Only a corroborated action may post, and that rule was learned rather than designed.** The first version let narrowing changes post if the identity closed, and produced **two wrong postings in three hundred settlements**: the agent tightened a window, the pool fell from 44 records to 40, an `AMBIGUOUS` settlement became `VERIFIED`, every check passed, and the answer was wrong because the tightening had cut real records out of the batch.

> Removing candidates cannot make the survivor unique. It makes it **unexamined**.

So narrowing actions are assertions about a merchant's settlement behaviour, and assertions need corroboration. A second rule has the same shape: if the feed holds more candidates of the named class than the agent can afford to test and exactly one of the tested ones verifies, **that still does not post**, because an untested record might have verified too.

### The loop closed, end to end

This is adversarial case 9 and it is what the {{ .S.AgentRepaired }} repairs look like. A chargeback exists in a disputes feed nobody wired into the candidate pool, so nothing reconstructs the credit.

The verifier searches under `k(S)` at most 7 and finds nothing, with the nearest achievable sum 1038851 paise away. It hands the agent the exact residual, its sign and its cardinality. The agent answers `CHARGEBACK_DEBIT, add_item`, typed, from a closed vocabulary. The verifier searches the unjoined feed itself, finds `cbk_000223`, applies it, and re-runs the entire stack unmodified. It closes exactly and uniquely, and the hypothesis cites a real record, so it posts as `VERIFIED / RESOLVED_BY_HYPOTHESIS / cites cbk_000223`.

The division of labour is the point. The agent contributes the judgement about *what kind* of event to look for. The system contributes the evidence that one occurred, and it tries **every** candidate of that class against the verifier rather than only the one whose amount the model guessed.

The eleven-case suite passes on a deliberately unintelligent offline stub proposing from a fixed list in a fixed order. The quality of the proposer changes how often an exception clears; it cannot change whether a posting is correct.

**Adversarial cases: {{ .D.AdversarialMet }} of {{ .D.AdversarialTotal }} expectations met, Manhattan wrong postings {{ .D.CasesManhattanWrong }}, B0 wrong postings {{ .D.AdversarialB0Wrong }}.** Full table in [RESULTS.md](RESULTS.md).

---

## The exception list is the deliverable

Most systems treat the exception list as an apology. Every entry here carries a status distinguishing *two answers exist and here they both are* from *ten million answers exist* from *a filter decided this*; a named cause traceable to a specific gate, constraint or residual; a **computed** remediation; and a price. Which means the queue can be sorted by cost and worked in the order that clears the most money per hour.

The head of the queue, sorted by cost, as an operations lead would work it. **[The full top {{ len .S.TopExceptions }} is in RESULTS.md](RESULTS.md#the-exception-queue); all {{ .S.Exceptions }} are in `out/receipts.ndjson`.**

| settlement | merchant | status | cost | cause | computed remedy |
|---|---|---|---:|---|---|
{{- range first 6 .S.TopExceptions }}
| `{{ .Ref }}` | {{ .Archetype }} | `{{ .Status }}` | {{ .CostINR }} | {{ clip .Cause 58 }} | {{ if .Remediation }}{{ clip .Remediation 48 }}{{ else }}none available{{ end }} |
{{- end }}

### What refusing is worth

| | |
|---|---:|
| the whole held queue, at configured analyst handling time | **{{ n .D.ExceptionCostINR }} INR** |
| B0's {{ .S.B0PostedWrong }} wrong postings, at {{ n .D.RemediationEachINR }} INR each to unwind | **{{ n .D.WrongPostingCostINR }} INR** |
| difference | **{{ n .D.NetINR }} INR in Manhattan's favour, per {{ .S.Settlements }} settlements** |

The {{ n .D.RemediationEachINR }} INR is an assumption, printed so it can be replaced. It is roughly two hours of a mid-level finance analyst at Indian metro rates: noticing the error, usually at month end and from a reconciliation difference rather than from the posting itself; finding which credit it belonged to; reversing the journal; re-posting; explaining the movement to whoever signs the accounts. It excludes any case that reaches an auditor.

Substitute your own figure; the arithmetic is one multiplication. The conclusion survives a wide range, because B0 only comes out ahead if unwinding a wrong posting costs under {{ i .D.BreakEvenINR }} INR, which is well under an hour of analyst time for an error nobody knows about yet.

So {{ pct .D.PostRate }} is not a philosophical position. It is the cheaper one.

---

## Ask the receipts

The receipt store is queryable, and the Q&A agent answers only from stored evidence. Three exchanges from this run, verbatim:

{{ range .D.QATranscript }}
> **Q. {{ .Question }}**
>
{{ indent "> " (clip .Answer 420) }}
>
> `{{ .Path }}`{{ range first 2 .Citations }}
> `{{ . }}`{{ end }}
{{ end }}
The third is why any of them are shown. **An agent that answers every question is not grounded in anything.** The receipts record what the system decided and why. They do not record who approved anything, because no human approval step exists in this pipeline, so declining is the correct answer and it is the whole thesis in one exchange.

---

## The receipt

Every decision emits one, including every refusal. It carries the target, the witness, the pool statistics, both collision-index estimators, the uniqueness method and its **scope in words**, every completeness check with its verdict, the rival reconstructions where they exist, the agent's decision trace, the computed remediation, and the exception price.

A full annotated `VERIFIED` receipt and the `UNRESOLVED` and `NARROWING_SENSITIVE` shapes are in [docs/DESIGN.md](docs/DESIGN.md). Every receipt from this run is in `out/receipts.ndjson`, one JSON object per line.

---

## Where the model sits

The boundary is one interface that speaks only in schemas. No method on it returns free text into a decision path, so the trust boundary is enforced by the type system rather than by discipline.

The model reads unstructured bank narrations, names the class of event behind a residual, orders which constraints to relax first, answers a finance lead's question from receipts, and explains a result in English. **None of those five is arithmetic, and none is available to a solver.** The verifier owns the entropy gate, the feasibility gate, the exhaustive enumeration and its uniqueness count, the completeness guards, and the independent re-derivation of the identity. It owns all of them regardless of what the model proposed.

Structured output uses **strict forced tool use**: the schema is the only tool, `additionalProperties` is closed, every field is required, and the model must call it. Three providers ship: the live Anthropic API when `ANTHROPIC_API_KEY` is set, recording to a cassette; cassette replay with no network, byte for byte; and a deterministic offline stub when neither is available.

**This run used `{{ .S.Provider }}` ({{ .S.ProviderModels }}).** Live-path parity is not yet demonstrated at batch scale; see [LIMITATIONS.md](LIMITATIONS.md#no-live-model-run-at-batch-scale). To run it:

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

Longer: [docs/EXPLAIN.md](docs/EXPLAIN.md) is the whole system from first principles in plain language; [docs/DESIGN.md](docs/DESIGN.md) has every derivation.

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
docs/diagrams/     rendered SVGs of every Mermaid diagram
RESULTS.md         generated; never typed
```

**Two tests worth reading.** `internal/solver/solve_test.go` verifies 400 randomised configurations against a 2ⁿ brute-force oracle and caught two real bugs before anything was built on it. `internal/bench/cases_test.go` runs all eleven adversarial cases and **fails if B0 posts nothing wrong**, because a suite the baseline survives is not adversarial enough to demonstrate anything.

---

Traditional reconciliation asks: *how confident are we that these records match?* Manhattan asks: *can we prove these records explain the settlement under the accounting rules, and if we cannot, do we know that before we act?*

**No guessed matches. No arbitrary confidence threshold. Proof, exhibited alternatives, or a named and priced reason the proof is unavailable.**

---

Built by **Rishi0507** for the Razorpay AI Buildathon, Track 04. Licensed under the MIT License; see [LICENSE](LICENSE).
