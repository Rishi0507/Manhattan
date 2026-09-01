package guards

import (
	"fmt"

	"github.com/Rishi0507/manhattan/internal/model"
	"github.com/Rishi0507/manhattan/internal/money"
	"github.com/Rishi0507/manhattan/internal/narrow"
)

// CheckState distinguishes a guard that ran and passed from one that could
// not carry information at all. Logging a green tick for a check that cannot
// fail is worse than logging nothing, because it looks like assurance.
type CheckState string

const (
	CheckPass     CheckState = "pass"
	CheckFail     CheckState = "fail"
	CheckInactive CheckState = "inactive"
)

// Check is one completeness guard's finding.
type Check struct {
	Name   string     `json:"name"`
	State  CheckState `json:"state"`
	Detail string     `json:"detail"`
}

// CardinalityCrossCheck compares the witness size against the transaction
// count the settlement report declares for the batch. A mismatch is
// decisive, and the witness size is native to the enumeration rather than
// being recovered afterwards.
//
// This check is weakened, but not voided, when the dispatch scope was itself
// taken from the declared count. Bounding the search at k_max = 6 still
// admits witnesses of size 4 and 5, so a mismatch remains possible and the
// check can still fail. That is the difference between this guard and the
// gross-ratio one below, which under a policy-derived contribution cannot
// fail by construction.
func CardinalityCrossCheck(witnessSize int, declared *int, zeroDropped int, scopedByDeclared bool) Check {
	const name = "cardinality_cross_check"
	if declared == nil {
		return Check{
			Name:   name,
			State:  CheckInactive,
			Detail: "no transaction count was declared for this batch",
		}
	}

	// Records with a zero net contribution were removed before the search, so
	// the reconstruction cannot and need not name them. The count still has to
	// reconcile, and it does: the batch is the witness plus those records.
	//
	// This is worth stating on the receipt rather than quietly tolerating,
	// because it is a genuine and slightly surprising fact about the data. A
	// UPI payment refunded in full nets to nothing, so it moved no money and
	// cannot be identified from an amount, and yet the gateway counts it as a
	// transaction in the batch. Both things are true at once.
	if witnessSize == *declared {
		detail := fmt.Sprintf("declared %d, witness %d", *declared, witnessSize)
		if scopedByDeclared {
			detail += "; note the search scope was itself bounded by this count, so the check is weakened though not void"
		}
		return Check{Name: name, State: CheckPass, Detail: detail}
	}
	if zeroDropped > 0 && witnessSize+zeroDropped == *declared {
		return Check{
			Name:  name,
			State: CheckPass,
			Detail: fmt.Sprintf(
				"declared %d, reconciled as %d reconstructed records plus %d that net to exactly zero; "+
					"those moved no money and cannot be identified from the credit, but they are accounted for by count",
				*declared, witnessSize, zeroDropped),
		}
	}
	return Check{
		Name:  name,
		State: CheckFail,
		Detail: fmt.Sprintf(
			"the report declares %d transactions but the reconstruction uses %d, with %d zero-contribution records set aside",
			*declared, witnessSize, zeroDropped),
	}
}

// GrossRatioCheck compares the effective fee rate implied across the witness
// against the rate prevailing across the pool it was drawn from.
//
// Two things about the comparand matter, and the second was a genuine design
// error before it was corrected here.
//
// It is a real guard only where fees are observed independently of the
// policy that built the contributions. If contributions are policy-derived,
// so that contribution = gross - f(gross) - refund, then any subset whose
// contributions sum to the target automatically implies the policy rate by
// construction. The check cannot fail, carries no information, and reports
// itself inactive rather than passing.
//
// And the comparand is the POOL's rate, not the policy's. Comparing the
// witness against policy makes this guard a duplicate of the fee anomaly
// detector, with the result that a merchant whose gateway is genuinely
// overcharging fails a completeness check and cannot post at all. That
// conflates two questions the whole design insists on separating: whether
// the money is accounted for, and whether the fee applied to it was right.
// Against the pool's own prevailing rate the guard asks the question it is
// actually for, which is whether this subset looks like it was drawn from
// this population or assembled by coincidence, and a uniform drift affecting
// every record cancels out of the comparison.
func GrossRatioCheck(witness, pool []model.Record, feesObserved bool, bandBps int64) Check {
	const name = "gross_ratio_sanity"
	if !feesObserved {
		return Check{
			Name:  name,
			State: CheckInactive,
			Detail: "contributions are policy-derived in this data mode, which makes the implied rate " +
				"tautological; the check would pass for any subset and is therefore not run",
		}
	}

	witnessRate, wn := effectiveRateBps(witness)
	poolRate, pn := effectiveRateBps(pool)
	if wn == 0 || pn == 0 {
		return Check{Name: name, State: CheckInactive, Detail: "no fee-bearing records to compare"}
	}

	delta := witnessRate - poolRate
	if delta < 0 {
		delta = -delta
	}
	// The band is widened here relative to the anomaly detector's, because
	// this is a coincidence test rather than a pricing test: a genuine batch
	// with an unusual instrument mix should not be blocked from posting.
	band := bandBps + 25
	if delta > band {
		return Check{
			Name:  name,
			State: CheckFail,
			Detail: fmt.Sprintf(
				"this reconstruction implies an effective rate of %d bps against %d bps across the pool it was drawn from, "+
					"a gap of %d bps past the %d bps band; the subset does not look like it came from this population",
				witnessRate, poolRate, delta, band),
		}
	}
	return Check{
		Name:  name,
		State: CheckPass,
		Detail: fmt.Sprintf("effective rate %d bps against %d bps across the pool, within the %d bps band",
			witnessRate, poolRate, band),
	}
}

// effectiveRateBps is the fee actually applied across a set of records, as
// basis points of their gross.
func effectiveRateBps(rs []model.Record) (int64, int) {
	var gross, fee money.Paise
	n := 0
	for _, r := range rs {
		if r.Kind != model.KindPayment || r.Gross == 0 {
			continue
		}
		gross += r.Gross
		fee += r.MDR
		n++
	}
	if gross == 0 {
		return 0, 0
	}
	return money.BPS(fee, gross), n
}

// DriftBaseline is a stored per-constraint drop rate from a prior run.
type DriftBaseline struct {
	RunID string                        `json:"baseline_source"`
	Rates map[narrow.Constraint]float64 `json:"rates"`
}

// DriftFinding is the run-level narrowing drift result.
//
// The neighbourhood probe tests sensitivity for one settlement. It cannot
// catch a constraint that is wrong systematically, because a misconfigured
// value-date window is stable under a one-day relaxation on every settlement
// in the run. So drift is tracked across the whole run and against a stored
// baseline.
//
// It is deliberately a run-level finding rather than a per-settlement flag.
// A diagnostic about a population does not belong on a receipt about one
// member of it, and putting it there would invite an analyst to clear it
// settlement by settlement, which is exactly the wrong response.
type DriftFinding struct {
	Constraint     narrow.Constraint `json:"constraint"`
	Observed       float64           `json:"drop_rate_observed"`
	Baseline       float64           `json:"drop_rate_baseline"`
	BaselineSource string            `json:"baseline_source"`
	Gate           string            `json:"gate"`
	Note           string            `json:"note"`
}

// DetectDrift compares this run's aggregate drop rates against the baseline
// and returns any constraint that deviates materially.
//
// The honest boundary: this catches configuration drift. It does not catch a
// constraint that was wrong from the first run, because there is no baseline
// to deviate from. That residual risk is real and it is stated rather than
// papered over.
func DetectDrift(observed map[narrow.Constraint]float64, base DriftBaseline, tolerance float64) []DriftFinding {
	if base.Rates == nil {
		return nil
	}
	var out []DriftFinding
	for _, c := range narrow.RelaxationOrder {
		b, ok := base.Rates[c]
		if !ok {
			continue
		}
		o := observed[c]
		if diff := o - b; diff > tolerance || diff < -tolerance {
			out = append(out, DriftFinding{
				Constraint:     c,
				Observed:       o,
				Baseline:       b,
				BaselineSource: base.RunID,
				Gate:           "hold_batch",
				Note: "detects drift from a stored baseline; cannot detect a constraint " +
					"that was misconfigured from the first run",
			})
		}
	}
	return out
}
