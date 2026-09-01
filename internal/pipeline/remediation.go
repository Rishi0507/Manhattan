package pipeline

import (
	"fmt"

	"github.com/Rishi0507/manhattan/internal/evidence"
	"github.com/Rishi0507/manhattan/internal/feasibility"
	"github.com/Rishi0507/manhattan/internal/model"
	"github.com/Rishi0507/manhattan/internal/money"
)

// refusalClaim states, in plain language, the size of the population the
// refusal is asserting exists.
func refusalClaim(feas feasibility.Report, n int) string {
	if feas.Decision == feasibility.DecideResourceCeiling {
		return fmt.Sprintf(
			"the region k(S) <= %d is decidable in principle for this %d-record pool, but enumerating it "+
				"would need %.0f MB against a configured ceiling of %.0f MB",
			feas.KStar, n,
			float64(feas.PredictedBytes)/(1<<20),
			float64(feas.MemoryCeiling)/(1<<20))
	}
	est := feas.EstimatedReconstructions()
	if feas.ImpliedFreeCardinality != nil {
		return fmt.Sprintf(
			"this batch is claimed to be %d records of a %d-record pool, a free cardinality of %d, at which the pool "+
				"admits an estimated %.3g distinct reconstructions of this target; no arithmetic procedure can single one out",
			*feas.DeclaredTxnCount, n, *feas.ImpliedFreeCardinality, est)
	}
	return fmt.Sprintf(
		"the pool admits an estimated %.3g distinct reconstructions of this target at every cardinality worth "+
			"searching; no arithmetic procedure can single one out", est)
}

// underdeterminedRemediation computes what would have to change, and what
// the collision index would become if it did.
//
// This is the difference between a refusal and a refusal that is useful. A
// line that says "narrow the window" is advice. A line that says "narrowing
// the window from plus or minus two days to zero drops an estimated 180
// candidates, taking the pool to 140 and the index from 8.4e7 to 0.30" is a
// decision a finance lead can act on this afternoon.
func underdeterminedRemediation(feas feasibility.Report, contribs []money.Paise, gcd int64, cfg Config) []evidence.Remediation {
	out := []evidence.Remediation{{
		Action: "supply the settlement reference, or the settlement_id to payment_id mapping",
		Effect: "collapses this reconciliation leg from a search to a lookup",
	}}

	// Project what a tighter value-date window would do. The window is the
	// dominant driver of pool size on almost every real merchant, so it is
	// the projection worth computing rather than merely naming.
	if feas.N > 8 {
		for _, frac := range []float64{0.5, 0.25} {
			projN := int(float64(feas.N) * frac)
			if projN < 4 {
				continue
			}
			k := feas.KStar
			if feas.ImpliedFreeCardinality != nil {
				k = *feas.ImpliedFreeCardinality
			}
			if k < 1 {
				k = 1
			}
			if k > projN/2 {
				k = projN / 2
			}
			if k < 1 {
				continue
			}
			idx := feasibility.Index(projN, k, feas.SigmaPaise, gcd)
			pn := projN
			pi := idx
			out = append(out, evidence.Remediation{
				Action:         fmt.Sprintf("tighten the value-date window so the pool falls to about %d candidates", projN),
				Effect:         fmt.Sprintf("takes the collision index at k=%d from %.3g to %.3g", k, feas.EstimatedReconstructions(), idx),
				ProjectedIndex: &pi,
				ProjectedPoolN: &pn,
			})
			if idx <= cfg.Feasibility.UnderdeterminedAbove {
				break // the first projection that crosses the boundary is the one to act on
			}
		}
	}

	out = append(out, evidence.Remediation{
		Action: "split the settlement by instrument, where the payout is instrument-segregated",
		Effect: "cuts the pool by the instrument mix, which lowers the index super-linearly",
	})
	return out
}

// entropyRemediation is the cure for a pool whose amounts do not distinguish
// its transactions. No amount of narrowing fixes this one, so the advice is
// different in kind.
func entropyRemediation(m model.Merchant) []evidence.Remediation {
	return []evidence.Remediation{
		{
			Action: "supply the settlement reference",
			Effect: "collapses this reconciliation leg to a lookup, which is the only reliable route for this amount distribution",
		},
		{
			Action: "split by instrument or by subscription plan",
			Effect: "raises the count of distinct contribution values and lowers twin mass",
		},
		{
			Action: "carry a per-transaction identifier through to the bank narration where the sponsor bank supports it",
			Effect: "removes the dependence on amounts entirely",
		},
	}
}
