package agent

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/Rishi0507/manhattan/internal/evidence"
	"github.com/Rishi0507/manhattan/internal/llm"
	"github.com/Rishi0507/manhattan/internal/model"
	"github.com/Rishi0507/manhattan/internal/money"
	"github.com/Rishi0507/manhattan/internal/narrow"
	"github.com/Rishi0507/manhattan/internal/pipeline"
)

// Controller is the agent loop: observe, choose, act, re-verify, repeat.
//
// This is the part that makes Manhattan an agent rather than a pipeline with
// two model calls in it. The difference is not that a model is involved, it
// is that the model is deciding WHAT TO DO NEXT from a state it can observe,
// and that its choice changes what happens.
//
//	observe    the receipt as it stands: status, pool size, collision index,
//	           twin mass, the exact residual, and everything already tried
//	choose     one action from a closed, typed vocabulary
//	act        the action becomes an overlay, which is data
//	verify     the ENTIRE unmodified stack re-runs over the edited inputs
//	repeat     bounded, with the outcome of the last step in the observation
//
// The loop is bounded at MaxSteps and every step is recorded. What keeps it
// safe is not the bound, it is that the agent's action space consists
// entirely of edits to the pipeline's inputs. It cannot reach the decision.
// After it retunes a window, the neighbourhood probe still runs against that
// retuned pool, so an agent that tightens too far is caught by exactly the
// guard that catches a human who misconfigures the same thing.
//
// Two consequences worth stating.
//
// The agent can repair the large refusal population, not just the small one.
// A hypothesis-only agent can fix UNRESOLVED, which is a minority. Most
// refusals are UNDERDETERMINED, and those are caused by a pool that is too
// wide, which is a narrowing decision rather than a missing record.
//
// And an agent that chooses to escalate has made a real move. Deciding that
// nothing further is worth trying, and recording why, is more useful than
// exhausting a budget.
type Controller struct {
	Provider llm.Provider
	MaxSteps int
	// Resolver handles the residual-explanation loop, which the controller
	// delegates to when it judges that a missing record is the problem.
	Resolver *Resolver
}

// NewController returns a controller with the shipped bounds.
func NewController(p llm.Provider) *Controller {
	return &Controller{Provider: p, MaxSteps: 3, Resolver: NewResolver(p)}
}

const controlSystem = `You are the operator of an audited payment settlement reconciler, working one
settlement that did not auto-post.

The verifier has already run. You are given what it concluded and why, and you choose
ONE action to try next. Your action edits the reconciler's INPUTS. It cannot edit the
decision: after you act, the entire verification stack re-runs unchanged, and it is
free to conclude that you made things worse.

How to think about the state you are shown:

UNDERDETERMINED means the candidate pool is too large for any unique answer to exist.
The number of candidate subsets grows like C(n, k), so pool size dominates everything.
This is almost never a missing-record problem and almost always a narrowing problem.
TIGHTEN_WINDOW is usually right. Look at the pool size: if it is in the hundreds for a
merchant settling daily, the value-date window is admitting several days of captures
into one cycle. Halving the window roughly halves the pool, which reduces C(n,k) by far
more than half.

AMBIGUOUS means several reconstructions fit. Tightening helps if the pool is large.
If twin mass is high the amounts genuinely do not distinguish these transactions and no
narrowing will help; escalate and say so.

UNRESOLVED means nothing reconstructs the credit. The residual is exact. Something is
missing from the pool, or the window is too tight and cut a real record out.
SEARCH_FEED first if an unjoined feed exists. WIDEN_WINDOW if the residual looks like a
whole transaction. PROPOSE_ADJUSTMENT only when neither applies.

NARROWING_SENSITIVE means a reconstruction was found but a rival appears when the pool
is widened. Do not try to make this post. The filtering decided it rather than the
arithmetic, and a human needs to confirm the constraint. ESCALATE.

MERCHANT HISTORY, when the observation shows one, is the most important block on the
page. Only two actions in the whole vocabulary can turn a refusal into a posting, and
this is one of them.

NARROW_TO_HISTORY tightens the window to a bound this merchant's OWN PROVED settlements
demonstrate. It is TIGHTEN_WINDOW with a second source attached, and the second source
is what lets it post. Prefer it over TIGHTEN_WINDOW whenever CORROBORATED_WINDOW_HOURS
is shown and is tighter than the window in use, and set window_hours to exactly that
number. A tighter value is refused, because it would cut out records this system has
already proved belong to a batch. TIGHTEN_WINDOW remains available and remains unable
to post, so it is worth choosing only to establish a proven cure for an analyst.

Choose ESCALATE whenever nothing in the action space plausibly helps. Recording that
you considered the options and none applies is a useful outcome and costs an analyst
less than a wrong suggestion.

Amounts are integer paise. Windows are in hours, as a half-width either side of the
capture day's midpoint.`

// Work runs the loop over one settlement.
func (c *Controller) Work(
	ctx context.Context,
	eng *pipeline.Engine,
	credit model.BankCredit,
	rec *evidence.Receipt,
	profile *Profile,
) (*evidence.Receipt, []Step, llm.Usage) {
	var usage llm.Usage
	var steps []Step

	if rec.Status.Postable() {
		return rec, nil, usage
	}

	// Decide deterministically whether the agent can plausibly help before
	// spending a call on it.
	//
	// This is the same principle the question-answering side uses: do not ask
	// a model something arithmetic already settles. Roughly a third of all
	// refusals are pools whose amounts genuinely cannot distinguish their
	// transactions, and no action in the vocabulary changes that. Invoking the
	// agent on them tripled the run's model spend and repaired nothing.
	if why, ok := triage(rec, eng, profile); !ok {
		rec.Agent.Invoked = false
		rec.Agent.Note = "the agent was not invoked: " + why
		return rec, nil, usage
	}

	best := rec
	tried := map[ActionKind]int{}

	for n := 1; n <= c.MaxSteps; n++ {
		obs := observe(best, steps, eng, profile)

		res, err := c.Provider.Structured(ctx, llm.Request{
			Role:       llm.RolePlan,
			System:     controlSystem,
			User:       obs,
			SchemaName: "choose_action",
			SchemaDesc: "Choose one action to try next on a settlement that did not auto-post.",
			Schema:     actionSchema(),
			MaxTokens:  700,
		})
		if err != nil {
			best.Agent.Note = "the agent was unavailable: " + err.Error()
			break
		}
		usage.Add(res.Usage)

		var a Action
		if err := res.Into(&a); err != nil {
			best.Agent.Note = "the agent returned an action outside its own vocabulary: " + err.Error()
			break
		}

		step := Step{
			N: n, Action: a, Observed: string(best.Status),
			PoolBefore: best.Pool.N, IndexBefore: best.Feasibility.IndexAtKStar,
			PoolAfter: best.Pool.N, IndexAfter: best.Feasibility.IndexAtKStar,
			Result: best.Status,
		}

		if a.Kind == ActionEscalate {
			step.Note = "escalated deliberately: " + a.Rationale
			steps = append(steps, step)
			break
		}

		// Repeating an action that already failed is a loop, not a plan.
		if tried[a.Kind] >= 2 {
			step.Note = "already tried this twice without progress; stopping rather than looping"
			steps = append(steps, step)
			break
		}
		tried[a.Kind]++

		var (
			ov    *pipeline.Overlay
			note  string
			ok    bool
			trial *evidence.Receipt
		)

		if a.Kind == ActionSearchFeed {
			// Every candidate of the named class is tried, and the action
			// succeeds only if exactly one of them verifies. Two records that
			// both close the identity are two explanations, and this system
			// does not choose between explanations it cannot tell apart.
			var residual money.Paise
			if best.Solver != nil && best.Solver.NearestMiss != nil && best.Solver.NearestMiss.Valid {
				residual = best.Solver.NearestMiss.Sum - best.TargetPaise
			}
			cands, complete := Candidates(eng.Unjoined, model.RecordKind(a.RecordKind), credit, residual)
			if len(cands) == 0 {
				step.Note = fmt.Sprintf(
					"not applicable: no %s record exists in any unjoined feed for this merchant and window",
					a.RecordKind)
				steps = append(steps, step)
				continue
			}
			var verified []model.Record
			var first *evidence.Receipt
			for _, cand := range cands {
				t := eng.ReconcileWith(credit, overlayForRecord(cand))
				if t.Status.Postable() {
					verified = append(verified, cand)
					if first == nil {
						first = t
						ov = overlayForRecord(cand)
					}
				}
			}
			switch len(verified) {
			case 0:
				step.Note = fmt.Sprintf(
					"searched %d %s record(s) in the unjoined feed; none makes the identity close",
					len(cands), a.RecordKind)
				steps = append(steps, step)
				continue
			case 1:
				if !complete {
					// The candidate list was truncated, so an untested record
					// might also have verified. One verification out of a
					// partial search is not a unique citation.
					step.Note = fmt.Sprintf(
						"%s reconciles this credit, but the feed holds more records of this class than "+
							"could be tested, so the citation cannot be shown to be the only one",
						verified[0].ID)
					step.Result = evidence.StatusAmbiguous
					steps = append(steps, step)
					continue
				}
				trial = first
				note = fmt.Sprintf("found %s in the unjoined %s feed, contributing %s",
					verified[0].ID, verified[0].Kind, verified[0].Contribution)
				ok = true
			default:
				step.Note = fmt.Sprintf(
					"%d different %s records each make the identity close, so the citation is not "+
						"unique and none of them can be posted on",
					len(verified), a.RecordKind)
				step.Result = evidence.StatusAmbiguous
				steps = append(steps, step)
				best.Remediation = append(best.Remediation, evidence.Remediation{
					Action: fmt.Sprintf("join the %s feed and identify which record belongs to this settlement",
						a.RecordKind),
					Effect: fmt.Sprintf(
						"%d records in that feed each reconcile this credit exactly; the arithmetic cannot choose between them",
						len(verified)),
				})
				continue
			}
		} else {
			ov, note, ok = a.apply(eng.Cfg.Narrow, eng.Merchants[credit.MerchantID], credit, eng.Unjoined, profile)
			if !ok {
				step.Note = "not applicable: " + note
				steps = append(steps, step)
				continue
			}
			trial = eng.ReconcileWith(credit, ov)
		}
		_ = ok
		step.PoolAfter = trial.Pool.N
		step.IndexAfter = trial.Feasibility.IndexAtKStar
		step.Result = trial.Status
		step.Note = note

		if strings.HasPrefix(ov.Provenance, "cited:") {
			step.Citation = strings.TrimPrefix(ov.Provenance, "cited:")
		}

		// A trial may post only if the verifier verified it AND the action that
		// produced it was corroborated by a real record. See
		// ActionKind.Corroborated: an action that merely changes a filter is an
		// assertion about the merchant's settlement behaviour, and removing
		// candidates cannot make the survivor unique, only unexamined.
		uncited := !a.Kind.Corroborated()
		if trial.Status.Postable() && !uncited {
			step.Accepted = true
			steps = append(steps, step)

			trial.Agent = best.Agent
			trial.Agent.Invoked = true
			trial.Agent.Provider = c.Provider.Name()
			trial.Agent.Iterations = n
			trial.Agent.Note = fmt.Sprintf(
				"resolved by agent action: %s. The verification stack re-ran unmodified over the amended inputs.",
				note)
			trial.Agent.Steps = trace(steps)
			trial.AddFlag(evidence.FlagResolvedByHypothesis)
			trial.Claim = trial.Claim + ". " + strings.ToUpper(note[:1]) + note[1:] +
				", and the reconstruction was re-verified from scratch afterwards"
			return trial, steps, usage
		}

		if trial.Status.Postable() && uncited {
			// The most valuable thing this loop produces on the refusal
			// population. Not a posting, and not advice either: a remediation
			// whose outcome has actually been computed and verified rather
			// than estimated.
			step.Note = note + "; this yields a unique reconstruction, but it is an assertion about " +
				"the merchant's settlement behaviour rather than corroborated evidence, so it cannot post"
			step.Result = trial.Status
			steps = append(steps, step)
			best.Remediation = append(best.Remediation, evidence.Remediation{
				Action: fmt.Sprintf("confirm that %s is correct for this merchant", note),
				Effect: fmt.Sprintf(
					"verified: under that change the pool falls from %d to %d records and exactly one "+
						"reconstruction of this credit exists, with the accounting identity closing to zero. "+
						"Applied without confirmation it would be a filtering decision rather than an "+
						"arithmetic one, which is the failure this system exists to prevent",
					step.PoolBefore, step.PoolAfter),
			})
			continue
		}

		steps = append(steps, step)

		// Keep a trial that improved the situation without solving it, so the
		// next observation reasons from the better state. Progress is measured
		// by the collision index rather than by the status, because moving
		// from an index of 8e7 to 40 is real progress even though both are
		// refusals.
		if improved(best, trial) {
			carry := best.Agent
			trial.Agent = carry
			trial.Remediation = best.Remediation
			best = trial
		}
	}

	best.Agent.Invoked = true
	best.Agent.Provider = c.Provider.Name()
	best.Agent.Iterations = len(steps)
	best.Agent.Steps = trace(steps)
	if best.Agent.Note == "" {
		proven := 0
		for _, s := range steps {
			if s.Result == evidence.StatusVerified {
				proven++
			}
		}
		if proven > 0 {
			best.Agent.Note = fmt.Sprintf(
				"the agent found %d change(s) that would make this settlement reconstruct uniquely, and verified each by re-running the full stack. "+
					"None is corroborated by a record, so none can post; they are attached as remediations for an analyst to confirm",
				proven)
		} else {
			best.Agent.Note = fmt.Sprintf(
				"the agent tried %d action(s) and none produced a unique reconstruction; the settlement is held with everything attempted recorded",
				len(steps))
		}
	}
	return best, steps, usage
}

// triage decides whether any action in the vocabulary could help.
//
// It is deliberately conservative about saying no: a false no costs recall on
// one settlement, while a false yes costs a model call on every settlement
// like it. The three cases where the answer is certain are cheap to detect.
func triage(r *evidence.Receipt, eng *pipeline.Engine, profile *Profile) (string, bool) {
	// A merchant with a corroborated window tighter than the one in use is
	// worth a call regardless of what else is true, because that is the one
	// narrowing move the system is allowed to post on and no cheaper check can
	// establish whether it helps.
	if profile != nil && profile.MaxOffsetHours < r.Narrowing.WindowHours {
		return "", true
	}
	// Amounts that do not distinguish transactions cannot be fixed by any
	// filter or any feed. This is the largest group by far and the remediation
	// for it is already on the receipt: a settlement reference, or a split by
	// instrument or plan.
	if !r.AmountEntropy.Pass {
		return "the amounts in this pool do not distinguish its transactions, and no narrowing " +
			"or feed search changes that; the remedy is a settlement reference", false
	}

	// A rival appears as soon as the pool is widened, so the answer came from
	// filtering rather than arithmetic. Retuning the filter further is the
	// wrong response and a human has to confirm the constraint.
	//
	// The exception is a rival admitted by a feed nobody joined. That is not a
	// filter to confirm, it is a source to connect, and searching it is exactly
	// what the agent is for.
	if r.Status == evidence.StatusNarrowingSensitive {
		fromFeed := r.Narrowing.Neighbourhood != nil &&
			r.Narrowing.Neighbourhood.Culprit == narrow.ConstraintUnjoinedFeed
		if !fromFeed {
			return "a rival reconstruction exists in the widened pool, so this needs a human to " +
				"confirm the constraint rather than further automated narrowing", false
		}
	}

	// Beyond those, invoke the agent only where the situation actually calls
	// for judgement rather than for a lookup.
	//
	// This screen was added after measuring: without it the agent ran on 255
	// settlements, chose to escalate on 200 of them, and proved a usable cure
	// on 8. Those 200 escalations were correct decisions, and they were also
	// decisions a two-line check could have made without a model call. Paying
	// a model to conclude "nothing here can help" on the majority of a queue
	// is the same mistake as paying it to add up a column.
	//
	// Three situations genuinely need judgement:
	hasFeed := false
	for _, u := range eng.Unjoined {
		if u.MerchantID == r.MerchantID {
			hasFeed = true
			break
		}
	}

	// One, a source exists that was never joined, and which class of record to
	// look for in it is a question about how settlement works.
	if hasFeed {
		return "", true
	}

	// Two, an exact residual with no reconstruction at all. Naming the class of
	// event that produces a gap of that size and sign is world knowledge and
	// there is no arithmetic procedure for it.
	if r.Status == evidence.StatusUnresolved && r.Solver != nil &&
		r.Solver.NearestMiss != nil && r.Solver.NearestMiss.Valid {
		return "", true
	}

	// Three, a pool large enough that the window is plausibly admitting more
	// than one capture cycle. How far to tighten, and whether tightening is
	// even the right lever for this merchant, is a judgement call.
	if r.Pool.N > 40 && r.Narrowing.WindowHours > 12 {
		return "", true
	}

	return "no unjoined feed exists for this merchant, there is no residual to explain, and the " +
		"pool is not large enough for the value-date window to be the cause; no action in the " +
		"vocabulary would change the outcome", false
}

// improved reports whether a trial is a better place to reason from.
func improved(cur, trial *evidence.Receipt) bool {
	rank := func(s evidence.Status) int {
		switch s {
		case evidence.StatusVerified:
			return 0
		case evidence.StatusAmbiguous:
			return 1
		case evidence.StatusUnresolved:
			return 2
		case evidence.StatusNarrowingSensitive:
			return 3
		}
		return 4 // UNDERDETERMINED is the furthest from an answer
	}
	if rank(trial.Status) != rank(cur.Status) {
		return rank(trial.Status) < rank(cur.Status)
	}
	// Same status: a smaller searched region with fewer expected rivals is
	// closer to decidable.
	return trial.Feasibility.IndexAtKStar < cur.Feasibility.IndexAtKStar
}

// observe renders the state the agent reasons over.
//
// It carries no candidate pool. That is deliberate and it is the whole cost
// argument: a matcher that reasons over records has to put the records in the
// context window, so its per-settlement spend scales with merchant size. This
// observation is a few hundred tokens whether the pool holds 30 records or
// 3,000.
func observe(r *evidence.Receipt, steps []Step, eng *pipeline.Engine, profile *Profile) string {
	var b strings.Builder

	fmt.Fprintf(&b, "SETTLEMENT %s\n", r.SettlementRef)
	fmt.Fprintf(&b, "merchant archetype: %s, settles on a %s cycle\n",
		r.Archetype, cycleWord(eng.Merchants[r.MerchantID].SettlementCycleDays))
	fmt.Fprintf(&b, "credit to reconstruct: %s\n", r.TargetPaise)
	fmt.Fprintf(&b, "value date: %s\n\n", r.ValueDate)

	fmt.Fprintf(&b, "VERIFIER CONCLUDED: %s\n", r.Status)
	fmt.Fprintf(&b, "  %s\n\n", r.Claim)

	fmt.Fprintf(&b, "POOL\n")
	fmt.Fprintf(&b, "  candidates after narrowing: %d\n", r.Pool.N)
	fmt.Fprintf(&b, "  contribution spread: %s\n", money.Paise(int64(r.Pool.SigmaPaise)))
	fmt.Fprintf(&b, "  twin mass: %.2f (above %.2f the amounts cannot distinguish transactions at all)\n",
		r.AmountEntropy.TwinMass, r.AmountEntropy.TwinMassThreshold)
	fmt.Fprintf(&b, "  distinct contribution values: %d\n", r.AmountEntropy.DistinctValues)
	fmt.Fprintf(&b, "  value-date window currently: plus or minus %.0f hours\n", r.Narrowing.WindowHours)

	// What this merchant's own proved settlements demonstrate. This is the
	// only block in the observation that is not about the settlement in hand,
	// and it is the one that makes a narrowing proposal corroborable.
	fmt.Fprintf(&b, "\nMERCHANT HISTORY\n")
	if profile == nil {
		fmt.Fprintf(&b, "  none yet: fewer than %d proved settlements for this merchant, so no\n",
			MinProofsForProfile)
		fmt.Fprintf(&b, "  narrowing proposal can be corroborated and NARROW_TO_HISTORY is unavailable\n")
	} else {
		fmt.Fprintf(&b, "  PROVED_SETTLEMENTS=%d\n", profile.Proofs)
		fmt.Fprintf(&b, "  proved batch sizes: %d to %d records\n", profile.MinBatch, profile.MaxBatch)
		fmt.Fprintf(&b, "  CORROBORATED_WINDOW_HOURS=%.1f (widest gap ever observed between a record\n",
			profile.MaxOffsetHours)
		fmt.Fprintf(&b, "    in a proved batch and its credit; median %.1f)\n", profile.MedianOffsetHours)
		fmt.Fprintf(&b, "  %.0f%% of proved batches contained a signed item\n", profile.SignedShare*100)
		fmt.Fprintf(&b, "  NARROW_TO_HISTORY may post. A window at or above %.1fh is supported by this\n",
			profile.MaxOffsetHours)
		fmt.Fprintf(&b, "    history; anything tighter is refused, because it would cut out records\n")
		fmt.Fprintf(&b, "    this system has already proved belong to a batch\n")
	}

	fmt.Fprintf(&b, "\nNARROWING REMOVED\n")
	type kv struct {
		k string
		v int
	}
	var rows []kv
	for k, v := range r.Narrowing.Dropped {
		rows = append(rows, kv{string(k), v})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].v > rows[j].v })
	for _, x := range rows {
		fmt.Fprintf(&b, "  %-32s %d\n", x.k, x.v)
	}

	fmt.Fprintf(&b, "\nFEASIBILITY\n")
	fmt.Fprintf(&b, "  k* (largest decidable free cardinality): %d\n", r.Feasibility.KStar)
	fmt.Fprintf(&b, "  collision index at k*: %.4g (refuses above %.0f)\n",
		r.Feasibility.IndexAtKStar, r.Feasibility.ThresholdUnderdetermined)
	if r.Feasibility.ImpliedFreeCardinality != nil {
		fmt.Fprintf(&b, "  the report declares %d transactions, implying free cardinality %d\n",
			*r.Feasibility.DeclaredTxnCount, *r.Feasibility.ImpliedFreeCardinality)
	}

	if r.Solver != nil && r.Solver.NearestMiss != nil && r.Solver.NearestMiss.Valid {
		fmt.Fprintf(&b, "\nRESIDUAL\n")
		fmt.Fprintf(&b, "  nearest achievable sum: %s at cardinality %d\n",
			r.Solver.NearestMiss.Sum, r.Solver.NearestMiss.Cardinality)
		fmt.Fprintf(&b, "  RESIDUAL_PAISE=%d (%s)\n",
			int64(r.Solver.NearestMiss.Sum-r.TargetPaise), r.Solver.NearestMiss.Gap)
	}

	fmt.Fprintf(&b, "\nUNJOINED FEEDS AVAILABLE: ")
	if len(eng.Unjoined) == 0 {
		b.WriteString("none\n")
	} else {
		kinds := map[model.RecordKind]int{}
		for _, u := range eng.Unjoined {
			if u.MerchantID == r.MerchantID {
				kinds[u.Kind]++
			}
		}
		if len(kinds) == 0 {
			b.WriteString("none for this merchant\n")
		} else {
			for k, n := range kinds {
				fmt.Fprintf(&b, "%s (%d records) ", k, n)
			}
			b.WriteString("\n")
		}
	}

	if len(steps) > 0 {
		fmt.Fprintf(&b, "\nALREADY TRIED ON THIS SETTLEMENT\n")
		for _, s := range steps {
			fmt.Fprintf(&b, "  %d. %-22s pool %d -> %d, index %.3g -> %.3g, result %s. %s\n",
				s.N, s.Action.Kind, s.PoolBefore, s.PoolAfter, s.IndexBefore, s.IndexAfter, s.Result, s.Note)
		}
		b.WriteString("  Do not repeat an action that did not help.\n")
	}

	b.WriteString("\nChoose one action.\n")
	return b.String()
}

// trace converts the loop's steps into the receipt's shape.
func trace(steps []Step) []evidence.AgentStep {
	out := make([]evidence.AgentStep, 0, len(steps))
	for _, s := range steps {
		out = append(out, evidence.AgentStep{
			N: s.N, Kind: string(s.Action.Kind), Rationale: s.Action.Rationale,
			Observed: s.Observed, Result: s.Result,
			PoolBefore: s.PoolBefore, PoolAfter: s.PoolAfter,
			IndexBefore: s.IndexBefore, IndexAfter: s.IndexAfter,
			Accepted: s.Accepted, Note: s.Note, Citation: s.Citation,
		})
	}
	return out
}

func cycleWord(days int) string {
	switch days {
	case 0:
		return "same-day"
	case 1:
		return "T+1"
	}
	return fmt.Sprintf("T+%d", days)
}

func actionSchema() map[string]any {
	kinds := make([]string, len(AllActions))
	for i, k := range AllActions {
		kinds[i] = string(k)
	}
	hyp := make([]string, len(AllKinds))
	for i, k := range AllKinds {
		hyp[i] = string(k)
	}

	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"kind": map[string]any{
				"type": "string", "enum": kinds,
				"description": "The single action to try next.",
			},
			"window_hours": map[string]any{
				"type":        "number",
				"description": "For TIGHTEN_WINDOW, WIDEN_WINDOW or NARROW_TO_HISTORY: the new half-width in hours, either side of the capture day's midpoint. A full trading day is about 14. For NARROW_TO_HISTORY, use CORROBORATED_WINDOW_HOURS from the observation exactly; anything tighter is refused.",
			},
			"record_kind": map[string]any{
				"type": "string", "enum": []string{"chargeback", "refund", "adjustment", "payment"},
				"description": "For SEARCH_FEED: the class of record to look for in the unjoined feeds.",
			},
			"hypothesis_kind": map[string]any{
				"type": "string", "enum": hyp,
				"description": "For PROPOSE_ADJUSTMENT: the class of unmodelled event asserted.",
			},
			"amount_paise": map[string]any{
				"type":        "integer",
				"description": "For PROPOSE_ADJUSTMENT: the magnitude in integer paise. Never a decimal.",
			},
			"rationale": map[string]any{
				"type":        "string",
				"description": "One sentence on why this action, given the state observed.",
			},
		},
		"required": []string{"kind", "rationale"},
	}
}
