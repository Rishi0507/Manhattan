package pipeline

import (
	"fmt"
	"sort"

	"github.com/Rishi0507/manhattan/internal/evidence"
	"github.com/Rishi0507/manhattan/internal/model"
	"github.com/Rishi0507/manhattan/internal/money"
)

// CheckClaim verifies a mapping somebody else asserted, without searching for
// one.
//
// This is the piece the rest of the system was missing, and the argument for
// it is one line:
//
//	Deriving a batch is a search. Checking a batch somebody claimed is not.
//
// Reconstruction costs C(n, k) and is only decisive in a narrow regime, which
// is why two of six merchant archetypes reconstruct nothing at all. Verifying
// a claimed batch costs O(|claim|): do these records exist, do they belong to
// this merchant, were they already posted in a prior cycle, do their signed
// contributions sum to the credit, does the count agree with the declaration.
// None of that touches the combinatorics.
//
// So this works exactly where reconstruction does not. A subscription merchant
// with two hundred identical charges has settlements no method can DERIVE from
// amounts, and the gateway's own mapping for those settlements can still be
// checked against the money in microseconds. The UNDERDETERMINED population
// and both zero-posting archetypes come back into scope.
//
// The claim this produces is deliberately weaker than VERIFIED and the
// difference matters:
//
//	VERIFIED           exactly one batch in the searched region produces this
//	                   credit, counted exhaustively. Nobody had to be trusted.
//	CLAIM_CONSISTENT   the batch the report named does produce this credit.
//	                   Other batches may also produce it; this one was not
//	                   derived, it was checked.
//
// A consistent claim is worth posting because the alternative is posting it
// unchecked, which is what a lookup does. It is not worth calling a proof, and
// the receipt never does.
//
// THE SEARCH NEVER SEES THE MAPPING. This is a separate entry point on the
// engine, called after Reconcile has already reached its own conclusion from
// the merchant's records alone. Feeding the mapping into narrowing would make
// the whole benchmark circular.
func (e *Engine) CheckClaim(credit model.BankCredit, claimed []string) *evidence.ClaimCheck {
	if len(claimed) == 0 {
		return nil
	}
	cc := &evidence.ClaimCheck{
		Source:      "gateway_settlement_report",
		ClaimedSize: len(claimed),
	}

	var sum money.Paise
	var zeros int
	seen := make(map[string]bool, len(claimed))
	for _, id := range claimed {
		if seen[id] {
			cc.Findings = append(cc.Findings, fmt.Sprintf("%s is named twice", id))
			continue
		}
		seen[id] = true

		r, ok := e.ByID[id]
		if !ok {
			// Distinguish a record nobody joined from a record that does not
			// exist. The first is our problem and the second is the report's,
			// and conflating them blames the counterparty for a feed we did
			// not connect.
			if u, inFeed := e.unjoinedByID()[id]; inFeed {
				cc.Unjoined = append(cc.Unjoined, id)
				sum += u.Contribution
				continue
			}
			cc.Missing = append(cc.Missing, id)
			continue
		}
		if r.MerchantID != credit.MerchantID {
			cc.Findings = append(cc.Findings,
				fmt.Sprintf("%s belongs to a different merchant", id))
			continue
		}
		if r.Reconciled {
			cc.Findings = append(cc.Findings,
				fmt.Sprintf("%s was already posted in a prior cycle", id))
		}
		if r.Contribution == 0 {
			zeros++
		}
		sum += r.Contribution
	}

	cc.SumPaise = int64(sum)
	cc.TargetPaise = int64(credit.Amount)
	cc.ResidualPaise = int64(sum - credit.Amount)
	cc.ZeroContribution = zeros

	// The declared count is the report's other claim about itself, and it is
	// free to check against the first one.
	if credit.DeclaredTxnCount != nil && *credit.DeclaredTxnCount != len(claimed) {
		cc.Findings = append(cc.Findings, fmt.Sprintf(
			"the report declares %d transactions and names %d",
			*credit.DeclaredTxnCount, len(claimed)))
	}
	if len(cc.Missing) > 0 {
		shown := cc.Missing
		if len(shown) > 4 {
			shown = shown[:4]
		}
		cc.Findings = append(cc.Findings, fmt.Sprintf(
			"%d named record(s) are not in this merchant's data at all (%v)", len(cc.Missing), shown))
	}

	if len(cc.Unjoined) > 0 {
		shown := cc.Unjoined
		if len(shown) > 4 {
			shown = shown[:4]
		}
		cc.Note = fmt.Sprintf(
			"the report names %d record(s) that exist only in a feed nobody joined (%v). The "+
				"claim cannot be checked against data this run cannot see, and that is a "+
				"connection missing on our side rather than an error in the report",
			len(cc.Unjoined), shown)
		cc.Verdict = evidence.ClaimUncheckable
		sort.Strings(cc.Missing)
		sort.Strings(cc.Unjoined)
		return cc
	}

	// The tolerance is the same one the solver uses, for the same reason: a
	// per-transaction rounding convention can legitimately move the total by a
	// paisa per record, and holding a settlement over that would be a false
	// alarm rather than a finding.
	band := money.Paise(0)
	if !e.Mode.FeesObserved() {
		band = money.Paise(len(claimed))
	}
	within := sum-credit.Amount <= band && credit.Amount-sum <= band

	switch {
	case !within:
		cc.Verdict = evidence.ClaimContradicted
		cc.Note = fmt.Sprintf(
			"the batch the report names sums to %s against a credit of %s, a residual of %s. "+
				"The report's own account of this settlement does not add up",
			money.Paise(sum), credit.Amount, money.Paise(sum-credit.Amount))
	case len(cc.Findings) > 0:
		cc.Verdict = evidence.ClaimContradicted
		cc.Note = "the batch the report names sums correctly, but its membership does not " +
			"survive checking: " + cc.Findings[0]
	default:
		cc.Verdict = evidence.ClaimConsistent
		cc.Note = fmt.Sprintf(
			"the %d records the report names do sum to this credit exactly, under the declared "+
				"fee policy and the merchant's own data. This is a consistent claim rather than a "+
				"unique reconstruction: other batches may also produce this credit, and none was "+
				"searched for", len(claimed))
	}
	sort.Strings(cc.Missing)
	return cc
}

// unjoinedByID indexes the sources nobody wired in.
//
// Used only by CheckClaim, and only to tell "we cannot see this record" apart
// from "this record does not exist". The search still never touches it.
func (e *Engine) unjoinedByID() map[string]model.Record {
	out := make(map[string]model.Record, len(e.Unjoined))
	for _, r := range e.Unjoined {
		out[r.ID] = r
	}
	return out
}
