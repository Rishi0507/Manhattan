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
func CardinalityCrossCheck(witnessSize int, declared *int, scopedByDeclared bool) Check {
	if declared == nil {
		return Check{
			Name:   "cardinality_cross_check",
			State:  CheckInactive,
			Detail: "no transaction count was declared for this batch",
		}
	}
	if witnessSize != *declared {
		return Check{
			Name:  "cardinality_cross_check",
			State: CheckFail,
			Detail: fmt.Sprintf("the report declares %d transactions but the reconstruction uses %d",
				*declared, witnessSize),
		}
	}
	detail := fmt.Sprintf("declared %d, witness %d", *declared, witnessSize)
	if scopedByDeclared {
		detail += "; note the search scope was itself bounded by this count, so the check is weakened though not void"
	}
	return Check{Name: "cardinality_cross_check", State: CheckPass, Detail: detail}
}

// GrossRatioCheck compares the effective fee rate implied across the witness
// against the configured band.
//
// It is a genuine guard only where fees are observed independently of the
// policy that built the contributions. If contributions are policy-derived,
// so that contribution = gross - f(gross) - refund, then any subset whose
// contributions sum to the target automatically implies the policy rate by
// construction. The check cannot fail and therefore carries no information,
// so it reports itself inactive rather than passing.
func GrossRatioCheck(witness []model.Record, feesObserved bool, bandBps int64) Check {
	const name = "gross_ratio_sanity"
	if !feesObserved {
		return Check{
			Name:  name,
			State: CheckInactive,
			Detail: "contributions are policy-derived in this data mode, which makes the implied rate " +
				"tautological; the check would pass for any subset and is therefore not run",
		}
	}

	var gross, observed, expected money.Paise
	seen := 0
	for _, r := range witness {
		if r.Kind != model.KindPayment {
			continue
		}
		gross += r.Gross
		expected += r.PolicyMDR
		observed += r.MDR
		seen++
	}
	if gross == 0 || seen == 0 {
		return Check{Name: name, State: CheckInactive, Detail: "no fee-bearing records in this witness"}
	}

	delta := money.BPS(observed-expected, gross)
	if delta < 0 {
		delta = -delta
	}
	if delta > bandBps {
		return Check{
			Name:  name,
			State: CheckFail,
			Detail: fmt.Sprintf("the implied effective rate across this witness is %d bps from policy, past the %d bps band",
				delta, bandBps),
		}
	}
	return Check{
		Name:   name,
		State:  CheckPass,
		Detail: fmt.Sprintf("implied effective rate is within %d bps of policy (band %d bps)", delta, bandBps),
	}
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
