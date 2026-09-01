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
	// CycleWindow is how far either side of the credit's value date an event
	// may fall and still belong to the batch. Narrowing applies it; the
	// accounting engine only reports it onto receipts.
	CycleWindow time.Duration
}

// DefaultConfig returns declared mode with the shipped policy.
func DefaultConfig() Config {
	return Config{Policy: DefaultPolicy(), Mode: ModeDeclared, Delta: 0}
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

	records := make([]model.Record, 0, len(ds.Payments)+len(ds.Chargebacks)+len(ds.Adjustments))

	for _, p := range ds.Payments {
		mdr := cfg.Policy.MDR(p.Instrument, p.Gross)
		gst := cfg.Policy.GST(mdr)

		var refunded money.Paise
		full := false
		for _, r := range refundsByPayment[p.ID] {
			refunded += r.Amount
			if r.Full {
				full = true
			}
		}

		rec := model.Record{
			ID:           p.ID,
			Kind:         model.KindPayment,
			MerchantID:   p.MerchantID,
			Instrument:   p.Instrument,
			Currency:     p.Currency,
			EventAt:      p.CapturedAt,
			Gross:        p.Gross,
			MDR:          mdr,
			GST:          gst,
			Refund:       refunded,
			Reconciled:   p.Reconciled,
			SettlementID: p.SettlementID,
			FeeObserved:  p.FeeObserved,
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

	for _, c := range ds.Chargebacks {
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
