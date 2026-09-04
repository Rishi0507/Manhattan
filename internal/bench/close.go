package bench

import (
	"context"
	"sort"
	"strings"

	"github.com/Rishi0507/manhattan/internal/agent"
	"github.com/Rishi0507/manhattan/internal/evidence"
	"github.com/Rishi0507/manhattan/internal/llm"
)

// buildCloseInput aggregates a finished run into the view a controller works
// from.
//
// Aggregates only, and deliberately. A controller does not read four hundred
// receipts and neither does this call: it reads what four hundred receipts add
// up to, per merchant and per cause, which is both what the job actually looks
// like and what keeps the close to a single bounded request.
func buildCloseInput(store *evidence.Store, sum Summary, archOf map[string]string) agent.CloseInput {
	in := agent.CloseInput{
		Settlements: sum.Settlements,
		Posted:      sum.M1Posted,
		PostedWrong: sum.M1PostedWrong,
		Held:        sum.M1Held,
		HeldCost:    sum.ExceptionCostINR,
		HeldValue:   sum.ExceptionValuePaise,
	}

	type acc struct {
		v      *agent.ArchetypeView
		batchN float64
		poolN  float64
		twin   float64
		index  float64
		n      float64
	}
	byArch := map[string]*acc{}
	causeN := map[string]int{}
	causeV := map[string]int64{}
	remedyN := map[string]int{}
	remedyV := map[string]int64{}

	for _, r := range store.All() {
		a := archOf[r.SettlementRef]
		if a == "" {
			a = r.Archetype
		}
		e, ok := byArch[a]
		if !ok {
			e = &acc{v: &agent.ArchetypeView{Name: a, StatusMix: map[string]int{}}}
			byArch[a] = e
		}
		v := e.v
		v.Settlements++
		v.StatusMix[string(r.Status)]++

		e.n++
		e.poolN += float64(r.Pool.N)
		e.twin += r.AmountEntropy.TwinMass
		e.index += r.Feasibility.IndexAtKStar
		if n := r.Feasibility.DeclaredTxnCount; n != nil {
			e.batchN += float64(*n)
		}

		posted := r.Status == evidence.StatusVerified
		if r.Status == evidence.StatusVerified {
			v.FromProof++
		} else if r.ReportClaim != nil && r.ReportClaim.Verdict == evidence.ClaimConsistent {
			v.FromClaim++
			posted = true
		}
		if posted {
			v.Posted++
		} else {
			v.Held++
			v.HeldValue += r.TargetPaise.Abs()
			v.HeldCost += r.ExceptionCostINR
			causeN[r.Claim]++
			causeV[r.Claim] += int64(r.TargetPaise.Abs()) / 100
			for _, rem := range r.Remediation {
				remedyN[rem.Action]++
				remedyV[rem.Action] += int64(r.TargetPaise.Abs()) / 100
			}
		}

		if r.ReportClaim != nil && r.ReportClaim.Verdict == evidence.ClaimContradicted {
			v.ClaimFails++
		}
		if r.HasFlag(evidence.FlagFeeAnomaly) {
			v.FeeAnomalies++
		}
		// The signature of a record the pipeline could not see: nothing
		// reconstructs the credit and the shortfall is exact.
		if r.Status == evidence.StatusUnresolved &&
			r.Solver != nil && r.Solver.NearestMiss != nil && r.Solver.NearestMiss.Valid {
			v.ExactResidua++
		}
	}

	for _, e := range byArch {
		if e.n > 0 {
			e.v.MeanPoolN = e.poolN / e.n
			e.v.MeanBatchN = e.batchN / e.n
			e.v.MeanTwinMass = e.twin / e.n
			e.v.MeanIndex = e.index / e.n
		}
		in.ByArchetype = append(in.ByArchetype, *e.v)
	}
	sort.Slice(in.ByArchetype, func(i, j int) bool {
		return in.ByArchetype[i].HeldValue > in.ByArchetype[j].HeldValue
	})

	for c, n := range causeN {
		in.ByCause = append(in.ByCause, agent.CauseView{Cause: c, Settlements: n, ValueINR: causeV[c]})
	}
	sort.Slice(in.ByCause, func(i, j int) bool { return in.ByCause[i].ValueINR > in.ByCause[j].ValueINR })
	if len(in.ByCause) > 12 {
		in.ByCause = in.ByCause[:12]
	}

	for a, n := range remedyN {
		in.Remedies = append(in.Remedies, agent.RemedyView{Action: a, Settlements: n, ValueINR: remedyV[a]})
	}
	sort.Slice(in.Remedies, func(i, j int) bool { return in.Remedies[i].ValueINR > in.Remedies[j].ValueINR })
	if len(in.Remedies) > 8 {
		in.Remedies = in.Remedies[:8]
	}

	for _, d := range sum.Drift {
		in.Drift = append(in.Drift, string(d.Constraint))
	}
	return in
}

// runClose writes and grades the period close.
//
// The grading is what turns this from a nice summary into a measurement. The
// benchmark injects specific operational conditions and records exactly what
// they are, so the close can be scored on whether it found them from the
// receipts alone. Nothing about the injected conditions reaches the model: it
// sees status mixes, pool sizes, twin masses and held values, and has to infer
// the cause the same way a controller would.
func runClose(
	ctx context.Context, closer *agent.Closer, store *evidence.Store,
	sum Summary, archOf map[string]string,
) (*evidence.PeriodClose, llm.Usage) {
	in := buildCloseInput(store, sum, archOf)
	pc, usage, err := closer.Close(ctx, in, agent.NewInspector(store, archOf))
	if err != nil || pc == nil {
		return nil, usage
	}
	agent.SortCauses(pc.RootCauses)
	gradeClose(pc, sum.Conditions)
	return pc, usage
}

// gradeClose scores the close against the conditions the run injected.
//
// A condition counts as found when the close names its class on its merchant.
// Naming the right class on the wrong merchant does not count, because "some
// merchant somewhere has an unjoined feed" is not an actionable finding and an
// operations lead cannot do anything with it.
//
// Findings the close makes that correspond to no injected condition are listed
// as Spurious rather than counted against recall. Some of them are real: the
// flat-price archetypes genuinely cannot be reconstructed from amounts and
// saying so is correct even though nobody injected it. The distinction is left
// visible rather than resolved, because resolving it would mean deciding which
// true findings count, which is exactly the sort of scoring nobody should
// accept on assertion.
func gradeClose(pc *evidence.PeriodClose, conditions []string) {
	pc.ConditionsInjected = conditions

	// The injected conditions are printed as prose, so they are matched on the
	// merchant name plus the shape of the condition.
	type want struct{ scope, class string }
	var wants []want
	for _, c := range conditions {
		scope := strings.TrimSpace(strings.SplitN(c, ":", 2)[0])
		switch {
		case strings.Contains(c, "disputes feed never joined"):
			wants = append(wants, want{scope, agent.CauseUnjoinedFeed})
		case strings.Contains(c, "window misconfigured"):
			wants = append(wants, want{scope, agent.CauseWindowTooWide})
		}
	}

	found := map[int]bool{}
	matched := map[int]bool{}
	for ci, rc := range pc.RootCauses {
		for wi, w := range wants {
			if rc.Class == w.class && strings.EqualFold(rc.Scope, w.scope) {
				found[wi] = true
				matched[ci] = true
			}
		}
	}
	for wi, w := range wants {
		label := w.scope + ": " + w.class
		if found[wi] {
			pc.ConditionsFound = append(pc.ConditionsFound, label)
		} else {
			pc.ConditionsMissed = append(pc.ConditionsMissed, label)
		}
	}
	for ci, rc := range pc.RootCauses {
		if !matched[ci] && rc.Class != agent.CauseNone {
			pc.Spurious = append(pc.Spurious, rc.Scope+": "+rc.Class)
		}
	}
	if len(wants) > 0 {
		pc.Recall = float64(len(pc.ConditionsFound)) / float64(len(wants))
	}
}
