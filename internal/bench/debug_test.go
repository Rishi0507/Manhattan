package bench

import (
	"context"
	"testing"

	"github.com/Rishi0507/manhattan/internal/generate"
	"github.com/Rishi0507/manhattan/internal/llm"
)

// TestCaseInternals prints the decision internals for each case. It is a
// development aid and asserts nothing; the assertions live in TestElevenCases.
func TestCaseInternals(t *testing.T) {
	ctx := context.Background()
	for _, c := range generate.Cases() {
		oc := RunCase(ctx, c, llm.NewOffline())
		r := oc.Receipt
		kmax, scope, matches := -1, "", -1
		if r.Solver != nil {
			kmax, scope = r.Solver.KMax, string(r.Solver.KMaxSource)
		}
		if r.Uniqueness != nil {
			matches = r.Uniqueness.MatchesFound
		}
		nearest := "none"
		if r.Solver != nil && r.Solver.NearestMiss != nil {
			nearest = r.Solver.NearestMiss.Gap.String()
		}
		t.Logf("case %2d %-22s n=%3d |S|=%2d k*=%2d kmax=%2d(%s) E=%.3g matches=%d twins=%d gap=%s -> %s %v",
			c.Number, c.Name, r.Pool.N, r.WitnessSize, r.Feasibility.KStar, kmax, scope,
			r.Feasibility.IndexAtKStar, matches, r.AmountEntropy.TwinClassCount, nearest, r.Status, r.Flags)
		if r.Agent.Invoked {
			t.Logf("        agent: %d hypotheses, note=%q", len(r.Agent.Hypotheses), r.Agent.Note)
			for _, h := range r.Agent.Hypotheses {
				t.Logf("          %s %s -> %s", h.Kind, h.Amount, h.Outcome)
			}
		}
		if r.Uniqueness != nil && len(r.Uniqueness.AlternativeWitnesses) > 1 {
			a, b := r.Uniqueness.AlternativeWitnesses[0], r.Uniqueness.AlternativeWitnesses[1]
			t.Logf("        rivals differ by: only-in-A=%v only-in-B=%v", diff(a, b), diff(b, a))
		}
		if r.Narrowing.Neighbourhood != nil && !r.Narrowing.Neighbourhood.Stable {
			t.Logf("        probe rival: removed=%v added=%v via %s",
				r.Narrowing.Neighbourhood.Rival.Removed, r.Narrowing.Neighbourhood.Rival.Added,
				r.Narrowing.Neighbourhood.Culprit)
		}
	}
}

func diff(a, b []string) []string {
	in := map[string]bool{}
	for _, x := range b {
		in[x] = true
	}
	var out []string
	for _, x := range a {
		if !in[x] {
			out = append(out, x)
		}
	}
	return out
}
