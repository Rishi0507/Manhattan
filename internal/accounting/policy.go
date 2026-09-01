// Package accounting turns raw source records into the signed integer
// contributions the solver reconstructs from, under a declared fee policy.
//
// Two things in here are load bearing.
//
// The first is sign. Most subset-sum reconcilers assume every item adds to
// the total. Settlement batches do not work that way: a chargeback debit is
// negative, and so is a fully refunded payment whose MDR was retained. That
// is an accounting fact, and modelling it explicitly here is what lets the
// solver stay indifferent to it.
//
// The second is rounding. Where the convention is declared, every
// contribution is an exact integer and the reconstruction target is a single
// exact value. Where it is not, each contribution becomes an interval and
// the tolerance has to be bounded by the witness cardinality rather than by
// the pool size. Getting that wrong in either direction is fatal: demanding
// exact equality matches nothing on realistic data, and allowing a pool-wide
// band matches everything, which is worse.
package accounting

import (
	"fmt"

	"github.com/Rishi0507/manhattan/internal/model"
	"github.com/Rishi0507/manhattan/internal/money"
)

// RoundingMode selects how contributions are computed.
type RoundingMode string

const (
	// ModeDeclared: the rounding convention is known and configured. Every
	// contribution is exact and the tolerance is zero.
	ModeDeclared RoundingMode = "declared"
	// ModeInferred: the convention is unknown. Each contribution becomes an
	// interval of width Delta, and the accepted band is the witness
	// cardinality times Delta.
	ModeInferred RoundingMode = "inferred"
)

// Policy is the fee agreement, as configuration rather than as an assumption
// buried in code. Rates are in basis points so that no float appears
// anywhere in the derivation of an amount.
type Policy struct {
	Version string `json:"version"`

	// MDRByInstrument is the merchant discount rate in basis points, per
	// payment method. 200 bps is 2.00 per cent.
	MDRByInstrument map[model.Instrument]int64 `json:"mdr_bps_by_instrument"`

	// MinFee and MaxFee bound the per-transaction MDR where the published
	// schedule does so. Zero means unbounded.
	MinFee map[model.Instrument]money.Paise `json:"min_fee_paise,omitempty"`
	MaxFee map[model.Instrument]money.Paise `json:"max_fee_paise,omitempty"`

	// GSTBps is the tax on the MDR. 1800 bps is the Indian rate of 18 per
	// cent. It is charged on the fee, not on the transaction.
	GSTBps int64 `json:"gst_bps"`

	// FeeRounding and TaxRounding are applied at each step independently,
	// because that is how gateways actually compute them: the fee is rounded
	// to paise, and then the tax on the rounded fee is rounded again.
	FeeRounding money.RoundMode `json:"fee_rounding"`
	TaxRounding money.RoundMode `json:"tax_rounding"`

	// RefundReturnsMDR records whether the gateway returns the original MDR
	// when a payment is refunded. It usually does not, which is what makes a
	// fully refunded payment contribute a negative amount to the settlement.
	RefundReturnsMDR bool `json:"refund_returns_mdr"`

	// DisputeFee is the flat charge levied per chargeback, on top of the
	// disputed amount being debited back.
	DisputeFee money.Paise `json:"dispute_fee_paise"`

	// BandBps is the fee-anomaly allowance, over and above the rounding
	// component computed per settlement. It exists because published
	// schedules contain slab boundaries, minimum and maximum fees and
	// promotional windows that a config may not fully mirror.
	BandBps int64 `json:"fee_band_bps"`
}

// DefaultPolicy is a plausible Indian payment aggregator schedule, used by
// the generator and as the shipped default.
func DefaultPolicy() Policy {
	return Policy{
		Version: "fees_2026_08",
		MDRByInstrument: map[model.Instrument]int64{
			model.InstrumentCard:       200, // 2.00%
			model.InstrumentUPI:        0,   // zero-MDR under regulation
			model.InstrumentNetbanking: 190,
			model.InstrumentWallet:     210,
			model.InstrumentEMI:        265,
		},
		GSTBps:           1800,
		FeeRounding:      money.RoundHalfUp,
		TaxRounding:      money.RoundHalfUp,
		RefundReturnsMDR: false,
		DisputeFee:       money.FromRupees(25),
		BandBps:          2,
	}
}

// MDR computes the merchant discount rate on one transaction, applying any
// configured floor and cap.
func (p Policy) MDR(inst model.Instrument, gross money.Paise) money.Paise {
	bps, ok := p.MDRByInstrument[inst]
	if !ok {
		bps = p.MDRByInstrument[model.InstrumentCard]
	}
	fee := money.MulRateBPS(gross, bps, p.FeeRounding)
	if mn, ok := p.MinFee[inst]; ok && mn > 0 && fee < mn {
		fee = mn
	}
	if mx, ok := p.MaxFee[inst]; ok && mx > 0 && fee > mx {
		fee = mx
	}
	return fee
}

// GST computes the tax on an already-rounded fee.
func (p Policy) GST(fee money.Paise) money.Paise {
	return money.MulRateBPS(fee, p.GSTBps, p.TaxRounding)
}

// Validate catches a policy that cannot produce sensible contributions,
// before it produces confident nonsense.
func (p Policy) Validate() error {
	if p.GSTBps < 0 || p.GSTBps > 10000 {
		return fmt.Errorf("accounting: gst of %d bps is outside any plausible range", p.GSTBps)
	}
	for inst, bps := range p.MDRByInstrument {
		if bps < 0 || bps > 2000 {
			return fmt.Errorf("accounting: mdr of %d bps for %s is outside any plausible range", bps, inst)
		}
	}
	if p.Version == "" {
		return fmt.Errorf("accounting: policy has no version, so no receipt could name the rules it was decided under")
	}
	return nil
}
