package guards

import (
	"fmt"
	"strings"

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

// GrossRatioCheck compares how far the witness's fees deviate from policy
// against how far the pool's fees deviate from policy.
//
// The comparand took two corrections to get right, and the second was found
// by measurement rather than by reasoning.
//
// It is a real guard only where fees are observed independently of the policy
// that built the contributions. If contributions are policy-derived, any
// subset whose contributions sum to the target implies the policy rate by
// construction, so the check cannot fail. It reports itself inactive rather
// than passing.
//
// Comparing the witness's raw rate against POLICY made this a duplicate of the
// fee anomaly detector, so a merchant whose gateway genuinely overcharges
// could not post at all.
//
// Comparing the witness's raw rate against the POOL's raw rate fixed that and
// introduced a worse fault: it rejected correct reconstructions at scale. Fee
// rates are per instrument, and UPI carries none at all under Indian
// regulation while cards carry two per cent. A six-record witness drawn from a
// thirty-record pool has a materially different instrument mix by ordinary
// sampling variation, so its blended rate differs from the pool's by fifty or
// more basis points with nothing wrong. Measured on the benchmark, this
// rejected eighty correct reconstructions, which was the single largest cause
// of settlements failing to post.
//
// Both faults come from comparing blended rates across different instrument
// mixes. The deviation from policy is the quantity that does not have that
// problem: the policy figure already accounts for each record's instrument, so
// subtracting it normalises the mix away, while a uniform gateway drift shifts
// witness and pool together and cancels.
//
// What survives is a narrow guard, and the receipt says so. It can catch a
// witness containing records individually mispriced against policy in a way
// the rest of the pool is not. It cannot catch a coincidental subset drawn
// from the same population, because such a subset has the same fee profile as
// a genuine one. Completeness rests on the neighbourhood probe and the
// cardinality cross-check; this is a supporting check, not a load-bearing one.
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

	witnessDev, wn := deviationFromPolicyBps(witness)
	poolDev, pn := deviationFromPolicyBps(pool)
	if wn == 0 || pn == 0 {
		return Check{Name: name, State: CheckInactive, Detail: "no fee-bearing records to compare"}
	}

	delta := witnessDev - poolDev
	if delta < 0 {
		delta = -delta
	}
	// The band is wider than the anomaly detector's because this is a
	// coincidence test rather than a pricing test, and because a small witness
	// carries real sampling variation even after the mix is normalised away.
	band := bandBps + 25
	if delta > band {
		return Check{
			Name:  name,
			State: CheckFail,
			Detail: fmt.Sprintf(
				"this reconstruction's fees deviate from policy by %d bps against %d bps across the pool it was "+
					"drawn from, a gap of %d bps past the %d bps band; records in it appear to be priced "+
					"differently from the rest of the population",
				witnessDev, poolDev, delta, band),
		}
	}
	return Check{
		Name:  name,
		State: CheckPass,
		Detail: fmt.Sprintf(
			"fees deviate from policy by %d bps against %d bps across the pool, within the %d bps band "+
				"(instrument mix is normalised out, so this compares pricing rather than composition)",
			witnessDev, poolDev, band),
	}
}

// deviationFromPolicyBps is how far the fees actually applied to a set of
// records sit from what policy prescribes for those same records, in basis
// points of their gross.
//
// Subtracting the policy figure per record is what makes this comparable
// across sets with different instrument mixes: policy already prices each
// instrument, so what remains is pricing error rather than composition.
func deviationFromPolicyBps(rs []model.Record) (int64, int) {
	var gross, applied, expected money.Paise
	n := 0
	for _, r := range rs {
		if r.Kind != model.KindPayment || r.Gross == 0 {
			continue
		}
		gross += r.Gross
		applied += r.MDR
		expected += r.PolicyMDR
		n++
	}
	if gross == 0 {
		return 0, 0
	}
	return money.BPS(applied-expected, gross), n
}

// FeedCompletenessCheck fails when records are known to exist that the search
// was never shown.
//
// This is the guard that closes the last hole, and it took five wrong postings
// to find. A merchant's disputes feed was never joined, so a chargeback that
// genuinely belonged to the batch was not in the record universe at all. A
// different subset of the pool summed to the credit exactly and uniquely,
// the accounting identity closed, and every other guard passed, because none
// of them could see the record that was missing.
//
// The neighbourhood probe cannot cover this. It widens by relaxing narrowing
// constraints, so it can only recover a record that reached narrowing; one
// sitting in an unjoined feed never did. And the wrong witness shared no
// records at all with the true batch, which is far outside any substitution
// depth.
//
// The principle is the one the whole system runs on, applied to data rather
// than to arithmetic:
//
//	A pool that is knowingly incomplete cannot support a claim that the money
//	is fully accounted for. Not because the sum is wrong, but because the
//	question was asked of the wrong set.
//
// So the settlement is held, and the remedy is named: join the feed. The agent
// can then search it, cite the record, and the reconstruction posts with that
// citation attached. For a merchant in this state that is the only route to a
// posting, which is correct.
func FeedCompletenessCheck(unjoinedInWindow []model.Record, poolIDs map[string]bool) Check {
	const name = "feed_completeness"

	var missing []string
	for _, r := range unjoinedInWindow {
		if !poolIDs[r.ID] {
			missing = append(missing, r.ID)
		}
	}
	if len(missing) == 0 {
		return Check{
			Name:   name,
			State:  CheckPass,
			Detail: "every source known to this run was joined into the candidate pool",
		}
	}

	shown := missing
	if len(shown) > 4 {
		shown = shown[:4]
	}
	return Check{
		Name:  name,
		State: CheckFail,
		Detail: fmt.Sprintf(
			"%d record(s) exist in a feed that was never joined and fall inside this settlement's "+
				"window (%s). The pool the search ran over is knowingly incomplete, so a "+
				"reconstruction from it cannot show the money is fully accounted for",
			len(missing), strings.Join(shown, ", ")),
	}
}

// MinCalibrationRows is how many observed fee rows a pool needs before a
// contribution calibrated from them is treated as evidence rather than a guess.
const MinCalibrationRows = 6

// FeeBasisCheck weighs how much of a pool was priced from what a report said
// against how much was inferred.
//
// It cost wrong postings twice and the second time is the instructive one.
//
// Where a settlement report carries a per-payment fee row, a contribution is
// derived from money that demonstrably moved. Where the row is missing, and
// real reports have gaps, something has to be assumed about a number the whole
// proof rests on.
//
// The first version tested only the WITNESS and never fired, for the same
// reason every other failure in this file exists. Mispriced records are the
// ones that do not sum, so the search excludes them and selects a witness made
// entirely of correctly priced records. Testing the witness asks "did the
// search avoid the bad data", and the answer is always yes, because avoiding it
// is what a search does. Meanwhile the TRUE batch contained a mispriced record,
// did not sum, and lost to a coincidence that did.
//
//	The question is not whether the answer used bad data. It is whether bad
//	data was in the pool the answer was selected from.
//
// The second version tested the pool and priced missing rows at the configured
// schedule, which held almost everything, correctly and uselessly. The right
// answer was not a better guard, it was a better assumption: price a missing
// row at the rate this merchant's OWN report demonstrates rather than at a
// schedule they are visibly not on. That is accounting.CalibrateMissingFees,
// and it is what this check now weighs.
//
// A calibrated contribution is still inferred. What makes it defensible is
// that the inference came from the counterparty's own data, on the same
// merchant and the same instrument, and this check refuses to treat it as
// evidence when there was not enough of that data to establish anything.
func FeeBasisCheck(pool []model.Record, feesObserved bool) Check {
	const name = "fee_basis"

	if !feesObserved {
		return Check{
			Name:  name,
			State: CheckInactive,
			Detail: "every contribution in this data mode is policy-derived, so there is no " +
				"observed subset to test an assumption against",
		}
	}

	var calibrated, assumed, observed int
	for _, r := range pool {
		if r.Kind != model.KindPayment || r.Gross == 0 {
			continue
		}
		switch {
		case r.FeeObserved != nil:
			observed++
		case r.FeeCalibrated:
			calibrated++
		default:
			assumed++
		}
	}

	switch {
	case calibrated == 0 && assumed == 0:
		return Check{
			Name:   name,
			State:  CheckPass,
			Detail: "every contribution in this pool came from a fee row the report actually carried",
		}

	case assumed > 0:
		return Check{
			Name:  name,
			State: CheckFail,
			Detail: fmt.Sprintf(
				"%d candidate(s) have no fee row and too few comparable rows on the same "+
					"instrument to infer this merchant's rate, so their contributions fall back "+
					"to the configured schedule. A merchant on a negotiated rate is priced "+
					"wrong by the difference, which can put the true batch out of reach and a "+
					"coincidence within it", assumed),
		}

	case observed < MinCalibrationRows:
		return Check{
			Name:  name,
			State: CheckFail,
			Detail: fmt.Sprintf(
				"%d candidate(s) were priced from this merchant's observed rate, inferred from "+
					"only %d rows against a floor of %d. That is not enough to establish a rate",
				calibrated, observed, MinCalibrationRows),
		}
	}

	return Check{
		Name:  name,
		State: CheckPass,
		Detail: fmt.Sprintf(
			"%d of %d contributions came from fee rows the report carried; the other %d had no "+
				"row and were priced at the effective rate those %d rows demonstrate for this "+
				"merchant and instrument, rather than at the configured schedule",
			observed, observed+calibrated, calibrated, observed),
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
