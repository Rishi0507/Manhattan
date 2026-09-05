<img src="web/public/logo.png" alt="Manhattan" width="72" height="72" />

# Manhattan

**Settlement reconciliation that proves its answers, and refuses when it cannot.**

AI Finance Controller · Multi-source settlement reconciliation

Manhattan posts **{{ .D.M1Posted }} of {{ .S.Settlements }}** settlements automatically ({{ pct .D.M1PostRate }}) with **{{ .D.M1Wrong }} wrong**, and hands back {{ .D.M1Held }} exceptions each carrying a named cause, a computed remedy and a price.

### Who this is for

**A merchant reconciling across two aggregators.** One ships a transaction-level
mapping, the other ships a net figure. The bank credit carries no usable
settlement reference. Today that leg is done by hand in a spreadsheet, and it is
the case nobody serves: Manhattan reconstructs the credit from the merchant's own
records and proves no other batch produces it. **{{ .D.NoClaim }} settlements in
this run ({{ pct .D.NoClaimPct }}) arrive with no mapping to check at all**, and
there reconstruction is the only route to a posting. It proves
{{ .D.NoClaimPosted }} of them here, on a run carrying a modelled window
misconfiguration; [with that one variable fixed it proves
{{ pct .D.CleanReconPct }} rather than {{ pct .D.MisconfigPct }}](#the-result).

**A settlement support desk.** "Why is my payout short by 4,180 rupees" is a
ticket with a cost attached, and the answer is already a receipt: here are the
records that make up the credit, here is the fee applied to each, here is the
chargeback debited this cycle but raised against the last one. On this run's numbers that answers **{{ pct .D.DeflectPct }}** of such tickets
from an existing receipt. Only that percentage is measured. Turning it into a
rupee figure takes three assumptions, so the arithmetic is published rather than
the conclusion: {{ n .D.DisputesPerMonth }} disputes a month, at
{{ .D.MinutesEach }} analyst minutes each, at 1,000 INR an hour, gives
{{ ni .D.MonthlySavingINR }} INR a month. [Substitute your own three
numbers](#value-to-a-support-desk); the multiplication is the same.

**A flat-price merchant, where reconstruction is useless and the product is not.**
Subscription and utility billing settle two hundred identical charges at a time,
and every group of the same size sums identically: no method that reads amounts
can tell them apart, and none ever will. Reconstruction posts **0%** there and
would whatever the solver did. The claim check posts
{{ range .S.ByArchetype }}{{ if eq .Archetype "subscription_saas" }}**{{ rate .M1PostRate }}**{{ end }}{{ end }}
and {{ range .S.ByArchetype }}{{ if eq .Archetype "utility_billpay" }}**{{ rate .M1PostRate }}**{{ end }}{{ end }},
because checking a batch somebody named costs nothing that deriving one costs.
That is the reason the composite exists, and it was a design premise rather than a later discovery.

**A gateway hardening its own reconciliation.** Manhattan sits on top of Single
View Recon rather than replacing it, checking the mapping the report already
ships against an independent account of the money. A settlement report that has
been verified is one nobody has to argue about later.

```
git clone <this repo> && cd manhattan
./run.sh demo          # or:  .\run.ps1 demo    or:  make demo
```

No API key required. **Every number in this file, in [RESULTS.md](RESULTS.md) and in [LIMITATIONS.md](LIMITATIONS.md) is emitted by that command**, rendered from one run in one pass so the three cannot drift. Generated from run `{{ .S.RunID }}`, seed `{{ .S.Seed }}`, on {{ .D.Host }}.

---

**The idea, in one line.** Deriving a batch from amounts is a search, and it
fails on merchants whose amounts repeat. Checking a batch somebody already named
is not a search, and it costs the same on every merchant. Manhattan does the
first where it can and the second everywhere else, which is why it posts
{{ range .S.ByArchetype }}{{ if eq .Archetype "subscription_saas" }}**{{ rate .M1PostRate }}**{{ end }}{{ end }}
of a flat-price subscription merchant's settlements with zero wrong, on data
where reconstruction alone posts **0%** and always will.

## Start here

1. **[The result](#against-the-method-already-in-use).** Reading the settlement report and posting it gets {{ .D.B1Wrong }} of {{ .D.B1Posted }} wrong, invisibly. Verifying it first gets {{ .D.M1Wrong }} of {{ .D.M1Posted }} wrong.
2. **[The controller](#the-controller)**, where the model reads the whole period and names the root causes, scored against operational conditions it was never told about. A graded harness, currently reporting an {{ pct .D.CloseRecallPct }} baseline from the deterministic provider.
3. **[Where the rest of the AI is](#the-other-model-roles)**: {{ n .S.ModelCalls }} calls across {{ len .S.CallsByRole }} jobs, two of them scored against ground truth.
4. **[RESULTS.md](RESULTS.md), the calibration section.** Whether the system knows *in advance* when it is about to be wrong.
5. **`./run.sh demo`**, which opens on adversarial case 10: narrowing drops a real record, a coincidental subset closes the identity exactly, and the guard catches it.

Longer: [docs/EXPLAIN.md](docs/EXPLAIN.md) builds the system from first principles in plain language, [docs/DESIGN.md](docs/DESIGN.md) has every derivation, and [LIMITATIONS.md](LIMITATIONS.md) is the full account of its operating limits.

### The agent

| | |
|---|---|
| **Model calls this run** | {{ n .S.ModelCalls }} across {{ .D.ModelRoles }} distinct jobs |
| **Agent loop** | a closed {{ .D.ActionCount }}-action controller, {{ n .S.PlanTurns }} turns graded |
| **Structured output** | every call schema-forced by the vendor's own mechanism: strict tool use on Anthropic, a strict response schema on Groq and Gemini. Free text cannot reach a decision path |
| **Provider reliability** | {{ if .D.IsStub }}not applicable on the deterministic path, which cannot fail. Measured and reported on every live run{{ else }}{{ pct1 .D.ModelReliabilityPct }} of attempted calls completed{{ if .D.ModelFailures }}, with {{ .D.ModelFailures }} failures reported rather than absorbed into the exception count{{ end }}{{ end }} |
| **Jobs scored against ground truth** | root-cause diagnosis ({{ pct .D.DiagAccuracyPct }}) and period close ({{ pct .D.CloseRecallPct }}) |
| **Wrong postings attributable to the model** | **0, structurally.** The verifier re-derives every posting from integer arithmetic and never asks the model whether it was right |

The last row is the design. The model is given every open-ended judgement in this
system and none of the arithmetic, which is why a weaker model changes how much
gets cleared and cannot change whether what cleared was correct. `manhattan live`
asserts that across providers on every run.

---

## The result

**{{ .D.M1Posted }} of {{ .S.Settlements }} settlements posted automatically, {{ .D.M1Wrong }} wrong.** {{ .D.M1FromProof }} of those are proofs, where exactly one batch produces the credit and it was counted exhaustively. The other {{ .D.M1FromClaim }} are the counterparty's own mapping, checked against an independent account of the money.

The two are not alternatives. **The check is worth something only because the reconstruction exists**, since the independent account it compares against is the contribution model the solver searches over. Delete the solver and the check has nothing to check against, which is exactly the position a lookup is in.

**And that proof count is measured on a deployment this run deliberately breaks.** A window misconfiguration is modelled on three merchants, and it roughly halves what reconstruction can prove. Fixing that one variable and changing nothing else, in the same sweep with the same code and the same report defects:

| | reconstruction proves |
|---|---:|
| window misconfigured, as the main run models it | {{ pct .D.MisconfigPct }} |
| **correctly configured** | **{{ pct .D.CleanReconPct }}** |

So the mechanism's rate is roughly {{ pct .D.CleanReconPct }}, and the headline figure is what it degrades to when a deployment's window is set too loosely. Both are published because the misconfigured one is what the rest of this run is measured on.

### Against the method already in use

The comparison that matters is not a fuzzy matcher. It is **B1: read the settlement report's stated mapping and post it.** Instant, free, and right almost always.

| {{ .S.Settlements }} settlements | B1, trust the report | **M1, verify then post** |
|---|---:|---:|
| posted | {{ .D.B1Posted }} | **{{ .D.M1Posted }}** |
| **posted wrong** | **{{ .D.B1Wrong }}** | **{{ .D.M1Wrong }}** |
| defective reports it would catch | 0 | **{{ .D.M1Contradicted }}** |
| correct reports it would wrongly hold | 0 | **{{ .D.FeeFalseAlarms }}** |
| settlements with no mapping to read | {{ .D.NoClaim }} unpostable | {{ .D.NoClaimPosted }} reconstructed |

B1 posts {{ sub .D.B1Posted .D.M1Posted }} more settlements. It also posts {{ .D.B1Wrong }} wrong ones, **{{ pct1 .D.SilentErrPct }} of everything it posts, and it cannot tell you which.** They do not surface at posting. They surface at month end as a reconciliation difference, or at audit, and by then the cycle is closed and somebody is reconstructing it by hand.

That is the trade in one line: **{{ sub .D.B1Posted .D.M1Posted }} settlements of extra coverage, against every wrong posting being invisible.** A team whose reports are already perfect loses nothing by checking. A team whose reports are not, finds out at posting instead of at audit.

*(A confidence matcher on the same inputs posts {{ .S.B0Posted }} and gets {{ .S.B0PostedWrong }} wrong. It is the wrong comparison for a gateway and it is in [RESULTS.md](RESULTS.md#the-baseline-across-every-threshold) with a full threshold sweep, because the sweep says something the operating point does not: its correct-answer count is flat at {{ .D.B0MaxRight }} from 0.10 through 0.90, so tuning it never finds another right answer.)*

The flat column is a measurement rather than a defect, and it is worth saying why, because a constant in a sweep usually means a broken harness. B0 scores how closely a candidate set resembles the credit. A set that sums exactly resembles it maximally, so the {{ .D.B0MaxRight }} it gets right score at the very top and survive every threshold that removes anything at all: raising the bar strips wrong postings first, which is precisely what a working confidence score should do. It only starts losing correct answers at 0.95, and by then it has also stopped posting about a third of the batch. So the score is not noise. It is a genuine ranking that ranks the wrong quantity, because resemblance and uniqueness come apart exactly where the money is.

### What this adds over trusting the report

Most of what the composite posts, B1 would have posted too. The difference is
worth stating outright, in two parts, because they are measured in different
units and adding them together would be wrong.

**Coverage.** A posting count, which nets out.

| | settlements |
|---|---:|
| Posted by both | {{ .D.DeltaBoth }} |
| Posted here only, because the report carried no mapping to trust | **+{{ .D.DeltaM1Only }}** |
| Posted by B1 only, declined here | **&minus;{{ .D.DeltaB1Only }}** |
| Net posting difference | &minus;{{ .D.DeltaSurrendered }} |

**Errors prevented.** Not a posting count. These are settlements B1 posted and
got wrong, and this system did not post at all.

| | settlements |
|---|---:|
| Wrong postings B1 made that this system did not | **{{ .D.DeltaWrongStopped }}** |
| of which, the report's claim was contradicted by arithmetic | {{ .D.DeltaContradicted }} |
| of which, the claim could not be checked and was held | {{ .D.DeltaUncheckable }} |

The two tables read together give the trade: **{{ .D.DeltaB1Only }} postings
declined to prevent {{ .D.DeltaWrongStopped }} wrong ones**, so
{{ pct1 .D.DeltaDeclinedBadP }} of what was declined would in fact have been
wrong, and {{ .D.DeltaM1Only }} settlements were posted that no claim-checking
approach can reach at all, because there is no claim there to check.

That is the shape of the trade and it is worth taking, because a declined
settlement comes back as a priced exception with a named cause and costs an
analyst minutes, while a wrong posting is silent and costs an investigation at
quarter end.

### Where reconstruction is impossible

| merchant type | spread sigma (paise) | twin mass | reconstruction | **M1** |
|---|---:|---:|---:|---:|
{{- range .S.ByArchetype }}
| {{ .Archetype }} | {{ f3g .MeanSigmaPaise }} | {{ f2 .MeanTwinMass }} | {{ rate .AutoPostRate }} | **{{ rate .M1PostRate }}** |
{{- end }}

Read the last two columns together. **Subscription SaaS and utility billpay reconstruct 0%, and M1 posts {{ range .S.ByArchetype }}{{ if eq .Archetype "subscription_saas" }}{{ rate .M1PostRate }}{{ end }}{{ end }} and {{ range .S.ByArchetype }}{{ if eq .Archetype "utility_billpay" }}{{ rate .M1PostRate }}{{ end }}{{ end }} of them.** Those are large, fast-growing segments and the reconstruction-only figure reads as "we cannot help you". It is not.

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

### False alarms

A validation layer is only usable if it leaves correct reports alone, so this is
the number that decides whether it ships: **{{ .D.FeeFalseAlarms }} false alarms
on {{ .D.FeeFAClean }} clean reports** ({{ pct1 .D.FeeFAPct }}).

**The generator does not share the verifier's fee schedule.** A false-alarm
count measured against the verifier's own constants would be arithmetic agreeing
with itself: re-derive a fee from the numbers that produced it and a clean report
cannot fail, so a zero would mean nothing. The synthetic data is therefore priced
under a deliberately different and equally plausible convention, and the verifier
is never told which:

| | generator | verifier assumes |
|---|---|---|
| fee rounding | half to even | half away from zero |
| tax rounding | truncated | half away from zero |
| minimum fee, card and netbanking | 2 rupees | none |
| maximum fee, EMI | 1,500 rupees | none |

Both arms are run rather than argued about. `MANHATTAN_FEE_MODEL=shared`
reproduces the control:

| across {{ .S.Settlements }} settlements | verifier knows the schedule | verifier does not |
|---|---:|---:|
| composite posted | 731 | **{{ .D.M1Posted }}** |
| **composite posted wrong** | **0** | **{{ .D.M1Wrong }}** |
| reconstruction posted | 212 | {{ .S.AutoPosted }} |
| false alarms on clean reports | 0 | **{{ .D.FeeFalseAlarms }}** |

The right-hand column is the one published throughout this document, because it
is the only one that is falsifiable. Not knowing the schedule costs 25 postings,
35 reconstructions, and {{ .D.FeeFalseAlarms }} clean reports wrongly held out of
{{ .D.FeeFAClean }}, which is {{ pct1 .D.FeeFAPct }} of them.

**The wrong-posting count does not move.** That is the result worth having. The
guarantee that nothing is posted unless the arithmetic closes does not depend on
the verifier having been handed the fee schedule that produced the data, which is
exactly the assumption a real deployment cannot make.

It gets there by taking the counterparty's data more seriously than its own
config. Real reports drop a per-payment fee row on some payments, and large
merchants are signed below the published schedule. Pricing a missing row at the
configured rate makes a *correct* report look wrong: a merchant on 178 bps priced
at 200 is off by 66 rupees on a 30,000 rupee ticket, nowhere near a pricing
tolerance and fatal to a sum that must close to zero. Priced that way, this run
raises **{{ .D.NaiveFalseAlarm }}** false alarms and reconstruction collapses.

So a missing row is priced at the rate the merchant's **own report**
demonstrates, per instrument, from at least six observed rows, and the
`fee_basis` guard refuses any reconstruction whose pool was inferred from less
evidence than that. The counterparty's data is a better source than a config
file they are visibly not following.

---

## The controller

The track is called AI Finance Controller, and a controller does not reconcile one settlement at a time. It reads the period. Which merchants are degrading, whether {{ .D.M1Held }} exceptions have {{ .D.M1Held }} causes or three, which single change recovers the most held value, and what needs a human this week.

Every input to that is arithmetic and every one is already computed. What is missing is the step that reads {{ .S.Settlements }} receipts and notices that eighty of them are the same problem wearing different reference numbers. **That step is the model's, and it is the only output in this system that works above a single settlement.**
{{ if .D.CloseSteps }}
**It investigates before it writes.** The model reads the period aggregates, asks
for one slice of the receipt store, reads it, and either asks for another or
stops. Every request costs a turn against a fixed budget, so it has to choose the
slice that would change its mind rather than the one that confirms it. This run
used {{ len .D.CloseSteps }} turns:

| # | asked to see | why |
|---:|---|---|
{{- range .D.CloseSteps }}
| {{ .N }} | `{{ .LookedAt }}`{{ if .Argument }} `{{ .Argument }}`{{ end }} | {{ clip .Why 96 }} |
{{- end }}

The trace is the point. A conclusion with no account of how it was reached is a
conclusion nobody can audit, which is the objection this project raises against
every confidence score, and it would be hypocritical to exempt its own report
from it.
{{ end }}
{{ if .D.Close }}
> {{ .D.Close.Narrative }}

| scope | cause | held INR | evidence it cited |
|---|---|---:|---|
{{- range .D.Close.RootCauses }}
| {{ .Scope }} | `{{ .Class }}` | {{ n64 .ValueINR }} | {{ clip .Evidence 76 }} |
{{- end }}

### Two jobs scored

`plan` chooses one action per turn from a closed set of eight, and the loop
already records which action was chosen and which one the verifier accepted, so
it grades itself:

| | |
|---|---:|
| turns taken | {{ n .D.PlanTurns }} |
| repairs reached on the **first** action chosen | {{ pct .D.PlanFirstTryPct }} |
| turns spent on an action that could not apply | {{ pct1 .D.PlanWastePct }} |
| turns per useful outcome | {{ f1 .D.PlanPerOutcome }} |

{{ if .D.IsStub }}Both accuracy figures saturate here, and that is the finding rather than the result: a fixed decision tree either picks the right action immediately or never picks it, and never proposes one that cannot apply. A model has room to be worse on both and better on the outcome, which is what the harness exists to measure.{{ end }}

### How the controller is graded

This run injects operational misconfigurations and records exactly what they are. **None of that reaches the model.** It sees status mixes, pool sizes, twin masses, held values and remedy counts, and has to infer the cause the way a controller would.

{{ if .D.IsStub }}**The harness is the contribution and the score below is its baseline**, produced by {{ .D.GradedBy }} rather than by a model. `manhattan live` runs the identical harness against the API and reports the difference, which is how a model's contribution here becomes a number rather than a claim.{{ end }}

| | |
|---|---:|
| conditions injected | {{ .D.CloseInjected }} |
| **identified, on the right merchant** | **{{ .D.CloseFound }}** |
| **recall** | **{{ pct .D.CloseRecallPct }}** |
| findings dropped for citing no evidence | {{ .D.Close.Dropped }} |
{{ if .D.Close.Spurious }}
Findings corresponding to no injected condition: {{ range $i, $s := .D.Close.Spurious }}{{ if $i }}, {{ end }}`{{ $s }}`{{ end }}. These are listed rather than counted against recall, because several are correct: the flat-price archetypes genuinely cannot be reconstructed from amounts, and reporting that is accurate even though it was not an injected condition. Deciding which true findings should count is a scoring judgement that belongs with the reader rather than with the system being scored.
{{ end }}
**The close cannot act.** It posts nothing, narrows nothing, amends no input and alters no receipt. That is precisely why it is the one model output not bounded by a closed action vocabulary: a person reads it and then decides. Everywhere the model *can* influence a posting it is fenced; here it cannot, so it is given the whole period and asked to think.
{{ end }}
---

## The other model roles

The fair criticism of this project is that arithmetic does the deciding, so what is the model for. Here is the accounting rather than an argument.

| job | calls | what it contributes | graded? |
|---|---:|---|---|
{{- range .D.RoleRows }}
| `{{ .Role }}` | {{ n .Calls }} | {{ .What }} | {{ .Graded }} |
{{- end }}

**One of those is scored against ground truth.** When the claim check fails, the arithmetic is already known: the residual, the missing ids, the count mismatch. What the model contributes is the *diagnosis*, because the same failed check has several causes with completely different remedies. A report short by one record with a residual matching a chargeback, a report naming a payment from last cycle, and a truncated file all fail identically and need three different actions.

The generator records which defect it injected, the pipeline never sees it, and the diagnosis is scored against it:

| | |
|---|---:|
| defects diagnosed | {{ .S.DiagnosedDefects }} |
| correct | **{{ .S.DiagnosisCorrect }}** ({{ pct (mul .S.DiagnosisAccuracy 100) }}){{ if .D.IsStub }}, by {{ .D.GradedBy }}{{ end }} |

{{ if .D.Confusions }}The errors are {{ .D.Confusions }}: exactly the pair that needs the class of record involved rather than the sign of the residual, which is what the deterministic stub reads and all it reads. {{ end }}That is headroom a real model has, stated as a number rather than a hope, and `manhattan live` is what turns it into a measurement.

**Two things about that table, said plainly rather than left to be noticed.**

`parse` is the largest number and the least interesting job. Bank narration
formats are finite, and a gateway would replace this with a lookup table in a
week. It is a model call here because that is honest about a system that has to
read narrations it has never seen, and it should not be read as the AI doing
{{ n (index .S.CallsByRole "parse") }} settlements' worth of work.

And **{{ .S.AgentSkipped }} exceptions never reach a model at all**, which is
cost discipline rather than AI avoidance. A deterministic screen establishes
that no action in the vocabulary could change the outcome: the amounts do not
distinguish the transactions, or a rival already appears when the pool is
widened, or there is nothing left to search. Paying a model to conclude that
nothing can help, across most of a queue, is the same mistake as paying it to
add up a column. The {{ .S.AgentInvoked }} that do reach it are the ones where
judgement is the whole task, and they cost {{ f1 .D.PlanPerOutcome }} turns per
useful outcome.

**The highest-volume model job is the one with no safety risk at all.** {{ n .S.NotesDrafted }} analyst-facing notes: what to do, why it works in terms of what was measured, and what it will **not** fix. Every fact is supplied and every figure is substituted from the receipt afterwards, so a draft containing a digit is rejected wholesale ({{ .S.NotesRejected }} were, this run). A note is attached to a settlement held either way, so the worst a bad draft costs is a confusing sentence in a work queue.

### What the model cannot do

> **A wrong model output cannot produce a wrong posting.** Not unlikely to. Cannot.

That is the property, and it is enforced rather than intended. The provider
interface has no method returning free text into a decision path, so a model
answer reaches the pipeline only as a schema-validated edit to its *inputs*.
Whether the money is accounted for is then settled by an integer identity and an
exhaustive count, which re-run unmodified over the edited inputs and are free to
conclude the model made things worse.

The consequence is what lets an agent near a ledger at all: **a better model
clears more exceptions and a worse one clears fewer, and neither changes whether
what cleared was right.** The eleven adversarial cases pass identically on both
providers, and `manhattan live` asserts the same property across them on every
run: if the wrong-posting count differs between the API and the stub, the
command fails rather than publishing.

That is why the model is handed the open-ended work and the arithmetic is not
negotiable. It is the only division under which "let an agent reconcile
settlements" is a sentence a finance team can agree to.

These are two distinct claims, and Manhattan measures both. **`manhattan live` quantifies the difference:**

```
echo 'GROQ_API_KEY=...' > .env     # or GEMINI_API_KEY, or ANTHROPIC_API_KEY
./bin/manhattan live -n 996
```

The key goes in `.env` next to the repository, which is in `.gitignore`, or in
the environment if you prefer. Any of the three works, and `--provider groq |
gemini | anthropic` picks between them when more than one is set. Model routing
is per role: the high-volume extraction jobs and the low-volume judgement jobs
go to different models, which spends two token allowances rather than one.

It runs the same batch on the live API and on the stub, and asserts that wrong postings are **identical** while diagnosis accuracy, repairs and note quality are **free to improve**. If the wrong-posting column moves, the trust boundary has leaked and the command exits non-zero rather than publishing.
{{ if .D.HasLive }}
Measured, at {{ .D.LiveSettlements }} settlements:

| | live | stub |
|---|---:|---:|
| **auto-posted wrong** | **{{ .D.LiveWrong }}** | **{{ .D.StubWrong }}** |
| **composite posted wrong** | **{{ .D.LiveM1Wrong }}** | **{{ .D.StubM1Wrong }}** |
| verified | {{ .D.LiveVerified }} | {{ .D.StubVerified }} |
| agent repairs | {{ .D.LiveRepairs }} | {{ .D.StubRepairs }} |
| diagnosis accuracy | {{ pct (mul .D.LiveDiagAcc 100) }} | {{ pct (mul .D.StubDiagAcc 100) }} |
| close condition recall | {{ pct (mul .D.LiveCloseRecall 100) }} | {{ pct (mul .D.StubCloseRecall 100) }} |
| INR per 1k, **billed** | {{ ni .D.LiveINR }} | {{ ni .D.ModelledINR }}, modelled |
{{ else }}
**It needs a key, so the delta is not yet published.** Every figure in this repository comes from `{{ .S.Provider }}` ({{ .S.ProviderModels }}), and the cost column is priced at published rates rather than billed. The harness that measures the difference is built and tested; what it is waiting for is one environment variable.
{{ end }}
---

## Misconfigurations modelled in this run

A reconciliation benchmark on perfectly configured data measures nothing an agent could help with, so {{ len .D.Conditions }} misconfigurations are modelled across {{ .D.ConditionMerchants }} merchant types. All of them are things a **deployment gets wrong on its own side**:

{{ range .D.Conditions }}- {{ . }}
{{ end }}
The obvious criticism is that the author created the problem the agent solves. The answer is the curve below, not an argument.

---

## The agent

Repairs, split by the action that produced them, because one total hides which mechanism worked:

| action | repairs | corroborated by |
|---|---:|---|
{{- range .D.RepairsByAction }}
| `{{ .Flag }}` | {{ .Count }} | {{ if eq .Flag "SEARCH_FEED" }}a real record, cited by id, in a feed nobody joined{{ else if eq .Flag "NARROW_TO_HISTORY" }}this merchant's own prior VERIFIED settlements{{ else }}nothing; this action cannot post{{ end }} |
{{- end }}

And the contribution as a function of how bad the configuration is:

| scenario | verified | wrong | repairs | by feed | by history | proven cures |
|---|---:|---:|---:|---:|---:|---:|
{{- range .Sensitivity }}
| {{ .Scenario }} | {{ .Verified }} | {{ .Wrong }} | **{{ .Repaired }}** | {{ index .RepairedBy "SEARCH_FEED" }} | {{ index .RepairedBy "NARROW_TO_HISTORY" }} | {{ .Cures }} |
{{- end }}

**Both repair classes are corroborated, and they behave differently.** `SEARCH_FEED` cites a real record by id and repairs at every scenario, including a correctly configured one, because an unjoined feed is a data-availability problem rather than a configuration one. `NARROW_TO_HISTORY` narrows to a bound this merchant's own proved settlements demonstrate, so it repairs where a window is loose and repairs {{ .D.ControlNarrow }} where none is: an agent that found something to fix on a correctly configured deployment would be inventing work. **Wrong postings are zero in every scenario**, including where the agent works hardest. And at twice the modelled misconfiguration narrowing repairs fall back to zero for a better reason: a merchant that badly configured proves almost nothing, never reaches the twelve proofs a profile needs, and has no history to corroborate against. That ceiling is in [LIMITATIONS.md](LIMITATIONS.md).

### The queue, as a flow

```
{{ .D.ExceptionsEntered }}  settlements entered the loop as unresolved
{{ printf "%3d" .S.AgentSkipped }}  settled by deterministic triage, with no model call        ({{ pct .D.TriagePct }})
{{ printf "%3d" .S.AgentInvoked }}  reached the agent
{{ printf "%3d" .S.AgentSteps }}  actions taken
{{ printf "%3d" .S.AgentRepaired }}  repaired into a posting, each citing evidence
{{ printf "%3d" .S.AgentProvenCures }}  given a proven cure: verified remedy, deliberately not posted
{{ printf "%3d" .S.Exceptions }}  remain held
  {{ printf "%d" .S.AutoPostedWrong }}  wrong postings caused
```

Paying a model to conclude that nothing can help, across most of a queue, is the same mistake as paying it to add up a column.

### Corroboration before posting

| action | may post? |
|---|---|
| `SEARCH_FEED` | **yes**, citing a real record by id |
| `NARROW_TO_HISTORY` | **yes**, bounded by this merchant's own proved settlements |
| `TIGHTEN_WINDOW`, `WIDEN_WINDOW`, `SPLIT_BY_INSTRUMENT`, `RELAX_RECONCILED`, `PROPOSE_ADJUSTMENT`, `ESCALATE` | no |

The rule was learned, not designed. The first version let narrowing post if the identity closed, and produced **two wrong postings in three hundred settlements**: the agent tightened a window, the pool fell from 44 records to 40, an `AMBIGUOUS` settlement became `VERIFIED`, every check passed, and the answer was wrong because the tightening had cut real records out of the batch.

> Removing candidates cannot make the survivor unique. It makes it **unexamined**.

That figure is hand-recorded from a build that no longer exists, and it is the only number in this file not emitted by run `{{ .S.RunID }}`. The failure is rebuilt as a committed test in [`internal/agent/corroboration_test.go`](internal/agent/corroboration_test.go), which fails if `TIGHTEN_WINDOW` is ever made postable again.

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

Four of the five stop the money and none is a failure. `AMBIGUOUS` at {{ .D.StatusAmbiguous }} and `UNDERDETERMINED` at {{ .D.StatusUnderdetermined }} are sized populations, not rhetoric. Flags are orthogonal: a settlement can be `VERIFIED` and carry `FEE_ANOMALY`, because whether the money is accounted for and whether the fee applied to it was right are different questions.

| flag | settlements |
|---|---:|
{{- range .D.FlagRows }}
| `{{ .Flag }}` | {{ .Count }} |
{{- end }}

{{ if and .D.TwinSwapInCases (not .D.TwinSwapInBatch) }}**That table covers the {{ .S.Settlements }}-settlement batch only.** `TWIN_SWAP` is absent from it and present in adversarial case 5: the batch rarely puts an exact twin inside a witness, while case 5 constructs one deliberately. The eleven cases are a separate fixture for exactly this reason.{{ end }}

---

## Architecture

```mermaid
flowchart TB
    subgraph SRC["Four sources, which<br/>disagree"]
        PG["PG settlement report"]
        BANK["Bank statement<br/>free text, one credit"]
        OMS["OMS / ledger"]
        DIS["Disputes feed"]
    end

    PG --> S1
    BANK --> S1
    OMS --> S1
    DIS --> S1

    S1["1 Parse<br/>model reads narration<br/>into typed fields"]
    S2["2 Narrow<br/>business constraints,<br/>every drop logged"]
    S3["3 Contribute<br/>signed net contribution,<br/>integer paise"]
    S4["4 Gate<br/>entropy, feasibility<br/>outputs k*, the solver's<br/>dispatch parameter"]
    S5["5 Reconstruct<br/>cardinality-dispatched<br/>meet in the middle"]
    S6["6 Prove<br/>count rivals, guard<br/>completeness, re-derive"]

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
    UR --> S7["7 Agent<br/>observe, act, re-verify<br/>only a corroborated<br/>action may post"]

    S7 -->|"corroborated"| V
    S7 -->|otherwise| CLAIM

    V --> CLAIM["8 Check the claim<br/>a separate entry point<br/>the search never saw it"]
    CLAIM -->|"consistent"| POST["post"]
    CLAIM -->|"contradicted"| DX["9 Diagnose<br/>model names the defect<br/>class, graded"]
    DX --> REV
    CLAIM -->|"uncheckable"| REV["review queue<br/>with a drafted, sendable<br/>note"]

    V --> POST
    POST --> EV["Evidence object<br/>replayable, diffable,<br/>queryable"]
    REV --> EV
    EV --> QA["10 Q and A<br/>grounded in receipts<br/>only"]

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

`./run.sh demo` builds the frontend, runs the batch and serves it on `localhost:8080`. Every figure the dashboard shows is also in [RESULTS.md](RESULTS.md), so nothing in the argument depends on running it.

The views that carry the argument are the period close and its grade, the head to head against the confidence matcher on one identical credit, the diagnosis panel where a gateway claim fails its arithmetic check with an exact residual, and the exception queue ordered by value cleared per analyst hour.

**The gate runs before the solver, not after it.** Its output `k*` is the parameter the solver is dispatched on, so triage is not a pre-check bolted onto the front of the search. It is what configures the search. Derivations in [docs/DESIGN.md](docs/DESIGN.md).

---

## The exception queue

Every entry carries a status distinguishing *two answers exist* from *ten million exist* from *a filter decided this*; a named cause; a **computed** remedy; a drafted note; and a handling estimate priced by what clearing it takes. That last part is what makes it a work plan: handling runs **{{ .D.ExceptionCostMin }} to {{ .D.ExceptionCostMax }} INR**, a {{ f1 .D.CostSpreadRatio }}x spread, and every term is on the receipt in `exception_cost_basis`.

So it is ordered by **value cleared per analyst hour**. [Full top {{ len .S.TopExceptions }} in RESULTS.md](RESULTS.md#the-exception-queue); all {{ .S.Exceptions }} in `out/receipts.ndjson`.

| settlement | status | at stake | mins | INR/hour | cause |
|---|---|---:|---:|---:|---|
{{- range first 5 .S.TopExceptions }}
| `{{ .Ref }}` | `{{ .Status }}` | {{ ni (div (float .ValuePaise) 100) }} | {{ .Minutes }} | **{{ ni .INRPerHour }}** | {{ clip .Cause 52 }} |
{{- end }}

### Value to a support desk

The buyer named at the top is a settlement support desk, so the number that matters to them is deflection, not match rate. From stated assumptions:

| | |
|---|---:|
| settlement disputes raised per month | {{ n .D.DisputesPerMonth }} *(assumption)* |
| analyst minutes to answer one from raw files | {{ .D.MinutesEach }} *(assumption)* |
| **deflected: answered from a receipt with no investigation** | **{{ pct .D.DeflectPct }}** *(this run's composite posting rate)* |
| analyst hours saved per month | **{{ f0 .D.HoursSavedMonthly }}** |
| at 1,000 INR per analyst hour | **{{ ni .D.MonthlySavingINR }} INR per month** |

Only the deflection rate is measured; the volume and the handling time are assumptions and are labelled. Substitute your own and the arithmetic is one multiplication.

The mechanism is the part that is not an assumption. A deflected ticket is one where the answer already exists as a receipt: here are the records that make up the credit, here is the fee applied to each, here is the chargeback debited this cycle but raised against the last one. The {{ .D.M1Held }} that are not deflected arrive with a named cause, a computed remedy and a drafted note, which is a shorter investigation rather than none.

### The cost of checking

Against B1, because that is the system a gateway actually has.

B1 posts everything it can read and holds only the {{ .D.B1Held }} settlements
whose mapping it has no way to read. M1 holds those too, so they cancel. The
marginal question is narrow: **how much extra analyst work does checking cause,
and how many wrong postings does that work prevent.**

| | |
|---|---:|
| settlements M1 holds that B1 posted | {{ .D.ExtraHeld }} |
| at {{ ni .D.AvgHandleINR }} INR average handling | **{{ ni .D.ExtraWorkINR }} INR** |
| wrong postings prevented | **{{ .D.B1Wrong }}** |
| so checking pays if unwinding one costs more than | **{{ ni .D.UnwindBreakINR }} INR** |

That break-even is **{{ f1 .D.UnwindHours }} analyst hours per wrong posting**,
and the question is whether unwinding one costs more than that.

A wrong settlement posting is not corrected by an edit. Nobody notices it at
posting time, because it balanced. It surfaces at month end as a reconciliation
difference, or at audit, and then somebody has to work out which credit it
belonged to, reverse the journal, re-post, and explain the movement to whoever
signs the accounts. Four hours is a floor for that, not a ceiling, and it
excludes every case that reaches an auditor or a merchant dispute.

### Break-even defect rate

The extra work is fixed by the held population. The wrong postings prevented
scale with how often reports are actually wrong. So there is a rate below which
checking does not pay, and it is computable rather than a matter of opinion:

> **If fewer than about {{ pct1 .D.BreakEvenDefectPct }} of your settlement reports
> are defective, checking costs more analyst time than it saves.**

That is a deployment recommendation, not a disclaimer. If your reports are
cleaner than that, run this in shadow: it posts nothing, it costs you nothing
beyond compute, and the first month tells you your real rate. If it contradicts
nothing, that is the most valuable negative result a reconciliation team can
have, and you stop. If it contradicts something, you were wrong about your rate
and you found out from a receipt rather than from an auditor.

The arithmetic is printed rather than the conclusion. Substitute your own
handling cost and unwind cost and the break-even moves with them.

---

## The B0 baseline, in full

{{ .S.B0PostedWrong }} wrong of {{ .S.B0Posted }} posted is a figure produced by this benchmark about a baseline it also implements, so the full scoring function is published for independent checking:

{{ range .S.B0Features }}- {{ . }}
{{ end }}
That is the whole function. It measures how good a match *looks*, never whether it is the only one, and those come apart exactly where the money is, which is why its correct count does not move as the threshold falls. The full sweep is in [RESULTS.md](RESULTS.md#the-baseline-across-every-threshold).

---

## Querying the receipts

{{ len .D.QATranscript }} exchanges from this run, verbatim, showing every path the Q&A side can take.

{{ range .D.QATranscript }}
> **Q. {{ .Question }}**
>
{{ indent "> " (clip .Answer 300) }}
>
> `{{ .Path }}`{{ range first 2 .Citations }}
> `{{ . }}`{{ end }}
{{ end }}
The last one matters most. **An agent that answers every question is not grounded in anything.** The receipts record what the system decided and why; they do not record who approved anything, because no human approval step exists in this pipeline.

---

## Running it

It is a library and a binary, not a platform, and that is the point: the whole
system is one Go module with no runtime dependencies, no database and no
network calls outside the model boundary.

**The integration surface is four feeds and one config.** Payments with their
fee rows, refunds, chargebacks and adjustments, plus the bank credit. Every one
maps onto fields a settlement report already carries:

| Manhattan | Gateway settlement report |
|---|---|
| `payment.gross_paise`, `captured_at`, `instrument` | `amount`, `created_at`, `method` on a settlement's payment rows |
| `payment.fee_observed_paise`, `tax_observed_paise` | `fee`, `tax` per row, which is what makes the fee check independent |
| `payment.settlement_id` | `settlement_id`, treated as a claim to verify rather than an answer |
| `chargeback.disputed_paise`, `fee_paise` | dispute amount and dispute fee from the disputes feed |
| `credit.amount_paise`, `value_date`, `narration` | the bank statement line, which is the only unstructured input |
| `policy.mdr_bps_by_instrument`, `gst_bps` | the merchant's rate card |

Amounts are integer paise throughout because that is how the settlement API
reports them, which is the fact that makes exact verification legitimate rather
than a modelling choice.

**Three ways to run it, in increasing order of commitment:**

```
manhattan recon   one batch, prints receipts, no state         # evaluate
manhattan bench   a period, writes receipts and the close      # pilot
manhattan serve   HTTP API and dashboard over a receipt store  # deploy
```

`serve` exposes the receipt store as JSON and streams a run over SSE, so it
drops behind an existing recon UI rather than replacing one. A receipt is a
flat JSON object with a stable schema: an operations tool consumes it, a
support desk renders it next to a ticket, a data warehouse loads it.

**What a first deployment looks like.** Run it in shadow beside the existing
reconciliation for one settlement cycle. It posts nothing; it produces receipts
and a close. Compare its `CLAIM_CONTRADICTED` list against what the existing
process posted, and the disagreements are the entire evaluation. If it
contradicts nothing, the reports are clean and that is worth knowing. If it
contradicts something, that is a defect nobody would otherwise have seen, and
it arrives with the residual attached.

---

## Track compliance

| requirement | what this run did |
|---|---|
| 50+ record batch | **{{ n .S.Pools.TotalRecords }} source records** across four feeds, driving **{{ .S.Settlements }} settlements**. Each settlement's universe reaches **{{ n .S.Pools.RawMax }}** before narrowing and **{{ .S.Pools.NarrowedMax }}** after |
| one loop, closed | bank credit to posted ledger entry, or to a named and priced exception |
| match rate reported | **{{ .D.M1Posted }} of {{ .S.Settlements }}**, {{ pct .D.M1PostRate }}, with **{{ .D.M1Wrong }} wrong** |
| exceptions it could not resolve | **{{ .D.M1Held }}**, each with a cause, a computed remedy, a price and a drafted note |
| throughput | **{{ ni .S.PerHour }} settlements per hour** end to end, {{ f1 .S.MedianLatencyMS }} ms median pipeline time, on {{ .D.Host }} |
| agentic design | a closed 8-action controller loop, a period-close investigation over the receipt store, cross-settlement merchant memory, and three graded jobs. {{ n .S.ModelCalls }} model calls across {{ len .S.CallsByRole }} roles{{ if .D.IsStub }}, all served by the deterministic offline provider on this run; `manhattan live` runs the same code against the API{{ end }} |

Throughput is end to end: {{ .S.Settlements }} settlements in {{ f1 .S.WallClockS }} s of wall clock including the agent loop, both baselines and receipt serialisation. That divides to {{ f1 .D.MsPerSettlement }} ms against a {{ f1 .S.MedianLatencyMS }} ms median pipeline time, and both are published.

Memory is **{{ i .D.PeakSolverMB }} MB**, the deterministic solver peak, computed from the entry counts on each receipt at twelve bytes each. That is the figure a capacity estimate should use. A sampled process-heap high-water mark is also recorded, and it is deliberately kept away from the throughput figures because it is noise: two runs of the same commit on the same seed have reported values an order of magnitude apart, depending only on when the collector happened to run. It is in [LIMITATIONS.md](LIMITATIONS.md) with that caveat attached and it is not a bound.

**Determinism is per commit.** Same seed and same commit gives the same decisions on every settlement. Timings are measurements and move, so receipts are not byte-identical.

### Resource limits

A throughput figure without a pool size is not a measurement, so here is the boundary instead. Enumeration costs C(n/2, at most k) entries at twelve bytes each, which makes the limit exact rather than benchmarked. Inside a 1 GB budget for one settlement:

| free cardinality | largest pool | entries |
|---:|---:|---:|
{{- range .D.OperatingLimits }}
| {{ .K }} | **{{ n .MaxPoolN }}** | {{ f1 .EntriesM }} M |
{{- end }}

Cost tracks cardinality, not pool size: a 164-record pool at k=5 costs what a 1,126-record pool at k=3 does. Narrowing is what keeps a real merchant inside this, and the pools in this run land between {{ .S.Pools.NarrowedMin }} and {{ .S.Pools.NarrowedMax }} candidates.

**Beyond the limit nothing crashes.** The feasibility gate checks the projected allocation *before* allocating and refuses, so an oversized pool becomes an `UNDERDETERMINED` with a computed remedy rather than an out-of-memory. And the claim-check path is unaffected at any size, because checking a batch somebody named is linear in the batch: **a merchant too large to reconstruct is still a merchant whose settlement report can be verified.** That is the second reason the composite matters and not just the first.

---

## Scope and limits

Stated once, in proportion. The full treatment is in [LIMITATIONS.md](LIMITATIONS.md).

**Synthetic data.** The pathology mix follows documented documented Indian payment gateway mechanics
(paise amounts, T+2 cycles, MDR with 18% GST, netted refunds, chargeback debits,
zero-MDR UPI). Reports are generated defective at a configured
{{ pct1 .D.DefectConfiguredPct }}, which lands {{ .D.Defects }} defects across
{{ .S.Settlements }} settlements, and that knob is a modelling choice rather
than an observation of any gateway. If a real rate is lower, the
composite's volume advantage scales down with it; the structural property does
not move, because an unchecked report is undetectably wrong at any rate.

**Uniqueness is scoped.** Attainable at free cardinality of roughly 3 to 7.
Outside that band `UNDERDETERMINED` is the honest answer and no solver
improvement changes it, which is why the claim-check path exists.

**A checked claim is not a proof.** {{ .D.M1FromClaim }} postings are the
counterparty's batch verified to produce this credit; another batch may also
produce it. Undetectable by either path: a report wrong in a way that still
balances, such as a substituted record of identical contribution.

**Fee divergence is modelled, not exhausted.** Negotiated rates and missing fee
rows are handled by calibration. Slab boundaries, per-network card rates and
mid-period rate changes are not.

**No live model run at batch scale.** Every figure comes from the deterministic
provider, and the cost is priced at published rates rather than billed.
`manhattan live` runs the same harness against the API and reports the delta; it
needs a key.

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
  llm/             the model boundary: Gemini, Anthropic, cassette, offline stub
  baseline/        B0 and B1, implemented to the same standard as the system
  bench/           the benchmark, calibration sweep, sensitivity, envelope
web/               the dashboard: Vite, React, TypeScript, Tailwind
docs/              DESIGN, EXPLAIN, DEMO-SCRIPT, templates, diagrams
```

**The documents are tested like the settlements.** `cmd/manhattan/doclint_test.go`
reads the rendered README, RESULTS and LIMITATIONS and fails the build on a
hardcoded count that contradicts a generated list, one quantity printed under
two labels, a malformed number, a dangling anchor, a broken link, or a
provider-dependent figure quoted without saying what produced it.
`derive_test.go` walks the document generator's AST and fails if any derived
figure is read before it is assigned, which is the bug that once published a
business case worth nothing per month. Each check has been probed by
introducing the defect it exists to catch and confirming it fails.

**Three tests worth reading.** `internal/solver/solve_test.go` verifies 400 randomised configurations against a 2ⁿ brute-force oracle and caught two real bugs before anything was built on it. `internal/bench/cases_test.go` runs all eleven adversarial cases and **fails if B0 posts nothing wrong**, because a suite the baseline survives is not adversarial. `internal/agent/corroboration_test.go` is the posting rule as an assertion rather than an anecdote.

---

Traditional reconciliation asks *how confident are we that these records match?* Manhattan asks *can we prove these records explain the settlement, and if we cannot, do we know that before we act?*

**No guessed matches. No confidence threshold. Proof, a checked claim, exhibited alternatives, or a named and priced reason none is available.**

Built by **Rishi0507**. MIT licensed; see [LICENSE](LICENSE).
