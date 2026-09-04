package accounting

import (
	"sort"
	"time"

	"github.com/Rishi0507/manhattan/internal/model"
	"github.com/Rishi0507/manhattan/internal/money"
)

// Config governs how contributions are derived.
type Config struct {
	Policy Policy
	Mode   RoundingMode
	// Delta is the per-item rounding allowance in inferred mode, in paise.
	// One paise is the usual value: it covers a single unknown rounding step
	// per contribution.
	Delta money.Paise
	// UseObservedFees makes contributions follow the fee rows the settlement
	// report actually carries, rather than the fee the policy says should
	// have been charged.
	//
	// This distinction is the whole of Leg C. Where the report supplies
	// per-payment fee rows, they are what genuinely came out of the bank
	// credit, so the reconstruction must use them and the policy figure is
	// then a genuinely independent second opinion: a settlement can
	// reconstruct exactly while the fee applied to it is wrong, and Manhattan
	// reports both facts separately.
	//
	// Where no such rows exist, contributions are necessarily policy-derived,
	// the fee comparison becomes a comparison of the policy against itself,
	// and no anomaly claim is made at all.
	UseObservedFees bool

	// CalibrateMissingFees prices a payment whose report carries no fee row at
	// the rate this merchant's OTHER payments demonstrably paid, rather than
	// at the configured schedule.
	//
	// Real settlement reports have gaps. The obvious fallback is the published
	// schedule, and it is wrong for every merchant on a negotiated rate, which
	// is most of the large ones. A 22 basis point difference on a 30,000 rupee
	// ticket is 66 rupees, nowhere near a pricing tolerance and fatal to a sum
	// that has to close to zero.
	//
	// The counterparty's own report is the better source. If forty of this
	// merchant's card payments carry fee rows averaging 178 bps, then 178 is
	// what this merchant pays, and pricing the missing rows at 200 because a
	// config file says so is choosing the worse of two available answers.
	//
	// Calibration is per merchant and per instrument, because that is the
	// granularity a rate card actually varies at. Where a merchant has too few
	// observed rows on an instrument to establish anything, the schedule is
	// still the fallback, and the fee-basis guard is what stops a
	// reconstruction resting on that.
	CalibrateMissingFees bool

	// MinRowsToCalibrate is how many observed fee rows an instrument needs
	// before its effective rate is trusted. Six is enough to see past
	// per-transaction rounding without waiting for a month of data.
	MinRowsToCalibrate int
	// CycleWindow is how far either side of the credit's value date an event
	// may fall and still belong to the batch. Narrowing applies it; the
	// accounting engine only reports it onto receipts.
	CycleWindow time.Duration
}

// DefaultConfig returns declared mode with the shipped policy.
func DefaultConfig() Config {
	return Config{
		Policy:               DefaultPolicy(),
		Mode:                 ModeDeclared,
		Delta:                0,
		CalibrateMissingFees: true,
		MinRowsToCalibrate:   6,
	}
}

// Delta returns the effective per-item tolerance, which is zero in declared
// mode whatever the configured value says.
func (c Config) EffectiveDelta() money.Paise {
	if c.Mode == ModeDeclared {
		return 0
	}
	if c.Delta <= 0 {
		return 1
	}
	return c.Delta
}

// Build converts a dataset into the uniform Record shape, computing each
// record's signed net contribution to a settlement under the policy.
//
// The mapping from source rows to records is deliberately not one-to-one
// with payments. A payment that was fully refunded in this cycle becomes a
// single record contributing minus the retained MDR and its tax. A
// chargeback becomes its own record. Collapsing these into "payments with
// adjustments" is how sign bugs get in.
func Build(ds *model.Dataset, cfg Config) []model.Record {
	refundsByPayment := map[string][]model.Refund{}
	for _, r := range ds.Refunds {
		refundsByPayment[r.PaymentID] = append(refundsByPayment[r.PaymentID], r)
	}

	// What this merchant's own report says it actually pays, per instrument.
	rates := calibrate(ds, cfg)

	records := make([]model.Record, 0, len(ds.Payments)+len(ds.Chargebacks)+len(ds.Adjustments))

	for _, p := range ds.Payments {
		// What policy says should have been charged.
		policyMDR := cfg.Policy.MDR(p.Instrument, p.Gross)
		policyGST := cfg.Policy.GST(policyMDR)

		// What was actually deducted, where the report says so.
		mdr, gst := policyMDR, policyGST
		calibrated := false
		switch {
		case cfg.UseObservedFees && p.FeeObserved != nil:
			mdr = *p.FeeObserved
			gst = cfg.Policy.GST(mdr)
			if p.TaxObserved != nil {
				gst = *p.TaxObserved
			}
		case cfg.UseObservedFees && cfg.CalibrateMissingFees:
			// No fee row. Price it at what this merchant's other payments on
			// this instrument demonstrably paid, rather than at the schedule.
			if bps, ok := rates[rateKey{p.MerchantID, p.Instrument}]; ok {
				mdr = money.MulRateBPS(p.Gross, bps, cfg.Policy.FeeRounding)
				gst = cfg.Policy.GST(mdr)
				calibrated = true
			}
		}

		var refunded money.Paise
		full := false
		for _, r := range refundsByPayment[p.ID] {
			refunded += r.Amount
			if r.Full {
				full = true
			}
		}

		rec := model.Record{
			ID:            p.ID,
			Kind:          model.KindPayment,
			MerchantID:    p.MerchantID,
			Instrument:    p.Instrument,
			Currency:      p.Currency,
			EventAt:       p.CapturedAt,
			Gross:         p.Gross,
			MDR:           mdr,
			GST:           gst,
			PolicyMDR:     policyMDR,
			PolicyGST:     policyGST,
			Refund:        refunded,
			Reconciled:    p.Reconciled,
			SettlementID:  p.SettlementID,
			FeeObserved:   p.FeeObserved,
			FeeCalibrated: calibrated,
		}

		// gross - MDR - GST(MDR) - refunds settled this cycle.
		//
		// When the whole gross comes back and the gateway keeps its fee, the
		// gross and the refund cancel and what remains is a debit of the fee
		// and its tax. That record is negative, and it is negative for a
		// reason a finance team would recognise rather than as an artefact.
		contribution := p.Gross - mdr - gst - refunded
		if full && cfg.Policy.RefundReturnsMDR {
			contribution += mdr + gst
		}
		rec.Contribution = contribution
		records = append(records, rec)
	}

	// A disputes feed that was never joined is not in the candidate pool, and
	// pretending otherwise would hide the exact condition the resolution
	// agent exists to detect: a residual with the shape of a chargeback and
	// no record in the pool that could produce it.
	for _, c := range ds.Chargebacks {
		if !ds.DisputesJoined {
			continue
		}
		total := c.Disputed + c.Fee
		records = append(records, model.Record{
			ID:           c.ID,
			Kind:         model.KindChargeback,
			MerchantID:   c.MerchantID,
			Currency:     "INR",
			EventAt:      c.DebitedAt,
			Gross:        c.Disputed,
			Chargeback:   total,
			Contribution: -total,
		})
	}

	for _, a := range ds.Adjustments {
		records = append(records, model.Record{
			ID:           a.ID,
			Kind:         model.KindAdjustment,
			MerchantID:   a.MerchantID,
			Currency:     "INR",
			EventAt:      a.AppliedAt,
			Adjustment:   a.Amount,
			Contribution: a.Amount,
		})
	}

	delta := cfg.EffectiveDelta()
	for i := range records {
		records[i].Lo = records[i].Contribution - delta
		records[i].Hi = records[i].Contribution + delta
		if delta == 0 {
			records[i].Lo, records[i].Hi = records[i].Contribution, records[i].Contribution
		}
	}

	// A stable pool order makes receipts diffable and replays byte-identical.
	sort.SliceStable(records, func(i, j int) bool { return records[i].ID < records[j].ID })
	return records
}

// Equation is the accounting identity, re-derived from raw record components
// without reusing any intermediate value the solver touched.
//
// If this disagrees with the solver, the solver is wrong and nothing posts.
// That is the entire point of computing it a second way.
type Equation struct {
	Gross         money.Paise `json:"gross_paise"`
	MDR           money.Paise `json:"mdr_paise"`
	GST           money.Paise `json:"gst_on_mdr_paise"`
	Refunds       money.Paise `json:"refunds_paise"`
	Chargebacks   money.Paise `json:"chargebacks_paise"`
	Adjustments   money.Paise `json:"adjustments_paise"`
	Reconstructed money.Paise `json:"reconstructed_paise"`
	Target        money.Paise `json:"target_paise"`
	Residual      money.Paise `json:"residual_paise"`
	SlackAllowed  money.Paise `json:"slack_allowed_paise"`
	SlackConsumed money.Paise `json:"slack_consumed_paise"`
	Closes        bool        `json:"closes"`
	NegativeItems int         `json:"negative_items"`
}

// Recompute derives the equation from the witness records and compares it
// against the bank credit.
func Recompute(witness []model.Record, target money.Paise, cfg Config) Equation {
	eq := Equation{Target: target}
	for _, r := range witness {
		eq.Gross += r.Gross
		eq.MDR += r.MDR
		eq.GST += r.GST
		eq.Refunds += r.Refund
		eq.Chargebacks += r.Chargeback
		eq.Adjustments += r.Adjustment
		if r.Contribution < 0 {
			eq.NegativeItems++
		}
	}
	// Chargeback gross is counted in Gross above for reporting, but it is
	// debited rather than credited, so it is removed from the credit side of
	// the identity to avoid double counting.
	var cbGross money.Paise
	for _, r := range witness {
		if r.Kind == model.KindChargeback {
			cbGross += r.Gross
		}
	}
	eq.Gross -= cbGross

	eq.Reconstructed = eq.Gross - eq.MDR - eq.GST - eq.Refunds - eq.Chargebacks + eq.Adjustments
	eq.Residual = eq.Reconstructed - target
	eq.SlackAllowed = money.Paise(len(witness)) * cfg.EffectiveDelta()
	eq.SlackConsumed = eq.Residual
	if eq.SlackConsumed < 0 {
		eq.SlackConsumed = -eq.SlackConsumed
	}
	eq.Closes = eq.SlackConsumed <= eq.SlackAllowed
	return eq
}

// rateKey identifies one merchant's pricing on one instrument.
type rateKey struct {
	merchant   string
	instrument model.Instrument
}

// calibrate reads the effective rate a merchant's own report demonstrates.
//
// Gross-weighted rather than a mean of per-payment rates, because a rate card
// applies to money and a mean of rates over-weights small tickets. Instruments
// with too few observed rows are left out entirely, so the schedule remains the
// fallback and the fee-basis guard remains the thing that catches it.
func calibrate(ds *model.Dataset, cfg Config) map[rateKey]int64 {
	if !cfg.UseObservedFees || !cfg.CalibrateMissingFees {
		return nil
	}
	min := cfg.MinRowsToCalibrate
	if min <= 0 {
		min = 6
	}

	type agg struct {
		gross, fee money.Paise
		n          int
	}
	sums := map[rateKey]*agg{}
	for _, p := range ds.Payments {
		if p.FeeObserved == nil || p.Gross == 0 {
			continue
		}
		k := rateKey{p.MerchantID, p.Instrument}
		a := sums[k]
		if a == nil {
			a = &agg{}
			sums[k] = a
		}
		a.gross += p.Gross
		a.fee += *p.FeeObserved
		a.n++
	}

	out := map[rateKey]int64{}
	for k, a := range sums {
		if a.n < min || a.gross == 0 {
			continue
		}
		out[k] = money.BPS(a.fee, a.gross)
	}
	return out
}
