package pipeline

import (
	"fmt"

	"github.com/Rishi0507/manhattan/internal/accounting"
	"github.com/Rishi0507/manhattan/internal/evidence"
	"github.com/Rishi0507/manhattan/internal/model"
	"github.com/Rishi0507/manhattan/internal/money"
)

// FeeAnomaly is Leg C: an independent recomputation of what the fee should
// have been, compared against what was observed. It is not a join, and it
// runs regardless of the reconciliation status, because "the money is
// accounted for" and "the fee policy applied to it looks right" are separate
// questions.
//
// Two things about this detector are stated plainly rather than glossed.
//
// It identifies an effective rate, not a schedule. Real fee structures
// contain slabs, minimum and maximum fees, per-instrument and per-network
// rates, promotional pricing and negotiated overrides, and several schedules
// can produce the same aggregate. "The observed effective rate is 8 bps
// outside the configured band" is a fact. "The fee schedule is 2.06 per
// cent" is a claim that a payments professional would correct.
//
// And in lump-credit mode it is circular. If the observed fee is derived
// from the same policy that built the contributions, agreement is
// tautological. Manhattan emits the circularity flag and makes no anomaly
// claim at all, because reporting a check that cannot fail is worse than
// reporting no check: it looks like assurance.
func FeeAnomaly(witness []model.Record, cfg accounting.Config, mode model.DataMode) evidence.FeeCheck {
	fc := evidence.FeeCheck{
		BandBps: cfg.Policy.BandBps,
	}

	if !mode.FeesObserved() {
		fc.Mode = "derived"
		fc.Circular = true
		fc.WithinBand = true
		fc.Claim = "no fee anomaly claim is made in this data mode: the observed fee is derived from the " +
			"same policy that built the contributions, so agreement would be tautological"
		return fc
	}
	fc.Mode = "observed"

	// The expected fee is computed per transaction from the policy and then
	// summed, so instrument mix is handled exactly rather than absorbed into
	// a tolerance. A single blended rate across a mixed batch would produce a
	// spurious delta on any merchant whose UPI share moved.
	var gross, expected, observed money.Paise
	seen := 0
	for _, r := range witness {
		if r.Kind != model.KindPayment {
			continue
		}
		gross += r.Gross
		// Expected is recomputed per transaction from the policy and summed,
		// so instrument mix is handled exactly rather than absorbed into a
		// tolerance. A single blended rate would produce a spurious delta on
		// any merchant whose UPI share moved between cycles.
		expected += cfg.Policy.MDR(r.Instrument, r.Gross)
		observed += r.MDR
		seen++
	}

	fc.ExpectedMDR = expected
	fc.ObservedMDR = observed

	if gross == 0 || seen == 0 {
		fc.WithinBand = true
		fc.Claim = "no fee-bearing records in this reconstruction"
		return fc
	}

	fc.DeltaBps = money.BPS(observed-expected, gross)

	// The band only needs to cover what genuinely cannot be computed exactly.
	// The rounding component is the witness cardinality times the per-item
	// tolerance, expressed in basis points of the gross; on a realistic batch
	// it is a small fraction of a basis point. What remains is the slab
	// allowance, which exists because published schedules contain boundaries
	// and promotional windows a config may not fully mirror.
	roundingComponent := money.BPS(money.Paise(len(witness))*cfg.EffectiveDelta(), gross)
	fc.RoundingComponentBps = roundingComponent
	band := cfg.Policy.BandBps + roundingComponent
	fc.BandBps = band

	d := fc.DeltaBps
	if d < 0 {
		d = -d
	}
	fc.WithinBand = d <= band

	if fc.WithinBand {
		fc.Claim = fmt.Sprintf("observed effective rate is within %d bps of policy (band %d bps)", d, band)
	} else {
		fc.Claim = fmt.Sprintf(
			"observed effective rate is %d bps outside the configured band of %d bps across %s of gross; "+
				"this identifies an effective rate, not a schedule",
			d-band, band, gross)
	}
	return fc
}
