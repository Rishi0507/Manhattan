package accounting

import (
	"math/rand"
	"testing"
	"time"

	"github.com/Rishi0507/manhattan/internal/model"
	"github.com/Rishi0507/manhattan/internal/money"
)

func p(v int64) money.Paise { return money.Paise(v) }

// TestContributionSigns pins the four shapes a record can take, because the
// sign of a contribution is an accounting fact rather than a convention and
// getting one of them backwards is invisible until a chargeback batch arrives.
func TestContributionSigns(t *testing.T) {
	cfg := DefaultConfig()
	at := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)

	ds := &model.Dataset{
		Mode:           model.ModeLumpCredit, // policy-derived, so fees are ours
		DisputesJoined: true,
		Payments: []model.Payment{
			// A plain captured card payment: positive.
			{ID: "pay_plain", MerchantID: "m", Instrument: model.InstrumentCard,
				Currency: "INR", Gross: p(100_000), CapturedAt: at},
			// Partially refunded: still positive.
			{ID: "pay_partial", MerchantID: "m", Instrument: model.InstrumentCard,
				Currency: "INR", Gross: p(100_000), CapturedAt: at},
			// Refunded in full, fee retained: NEGATIVE, and equal to the
			// retained fee plus its tax.
			{ID: "pay_full", MerchantID: "m", Instrument: model.InstrumentCard,
				Currency: "INR", Gross: p(100_000), CapturedAt: at},
			// UPI refunded in full: exactly zero, because UPI carries no MDR
			// so there is no fee to retain.
			{ID: "pay_upi_full", MerchantID: "m", Instrument: model.InstrumentUPI,
				Currency: "INR", Gross: p(100_000), CapturedAt: at},
		},
		Refunds: []model.Refund{
			{ID: "r1", PaymentID: "pay_partial", MerchantID: "m", Amount: p(40_000), SettledAt: at},
			{ID: "r2", PaymentID: "pay_full", MerchantID: "m", Amount: p(100_000), SettledAt: at, Full: true},
			{ID: "r3", PaymentID: "pay_upi_full", MerchantID: "m", Amount: p(100_000), SettledAt: at, Full: true},
		},
		Chargebacks: []model.Chargeback{
			{ID: "cbk", MerchantID: "m", Disputed: p(50_000), Fee: p(2_500), DebitedAt: at},
		},
		Adjustments: []model.Adjustment{
			{ID: "adj_pos", MerchantID: "m", Amount: p(1_000), AppliedAt: at},
			{ID: "adj_neg", MerchantID: "m", Amount: p(-1_000), AppliedAt: at},
		},
	}

	by := map[string]model.Record{}
	for _, r := range Build(ds, cfg) {
		by[r.ID] = r
	}

	// 2.00% of 100000 is 2000; 18% GST on 2000 is 360.
	mdr, gst := p(2_000), p(360)

	checks := []struct {
		id   string
		want money.Paise
		why  string
	}{
		{"pay_plain", 100_000 - mdr - gst, "gross less fee less tax"},
		{"pay_partial", 100_000 - mdr - gst - 40_000, "less the refund settled this cycle"},
		{"pay_full", -(mdr + gst), "the gateway keeps its fee, so what remains is a debit"},
		{"pay_upi_full", 0, "zero-MDR UPI refunded in full nets to exactly nothing"},
		{"cbk", -(50_000 + 2_500), "disputed amount plus the dispute fee, debited"},
		{"adj_pos", 1_000, "adjustments carry their own sign"},
		{"adj_neg", -1_000, ""},
	}
	for _, c := range checks {
		r, ok := by[c.id]
		if !ok {
			t.Fatalf("%s missing from the record universe", c.id)
		}
		if r.Contribution != c.want {
			t.Errorf("%s contributes %s, want %s (%s)", c.id, r.Contribution, c.want, c.why)
		}
	}

	if !by["pay_full"].Negative() || !by["cbk"].Negative() {
		t.Error("a fully refunded payment and a chargeback must both read as negative")
	}
}

// TestRefundReturnsMDR covers the other contractual arrangement, where the
// gateway does hand the fee back.
func TestRefundReturnsMDR(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Policy.RefundReturnsMDR = true
	at := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)

	ds := &model.Dataset{
		Mode:           model.ModeLumpCredit,
		DisputesJoined: true,
		Payments: []model.Payment{{ID: "pay", MerchantID: "m", Instrument: model.InstrumentCard,
			Currency: "INR", Gross: p(100_000), CapturedAt: at}},
		Refunds: []model.Refund{{ID: "r", PaymentID: "pay", MerchantID: "m",
			Amount: p(100_000), SettledAt: at, Full: true}},
	}
	recs := Build(ds, cfg)
	if len(recs) != 1 {
		t.Fatalf("expected one record, got %d", len(recs))
	}
	if recs[0].Contribution != 0 {
		t.Errorf("when the fee is returned a full refund nets to zero, got %s", recs[0].Contribution)
	}
}

// TestObservedFeesDisplacePolicy is the split the whole fee argument rests on.
// Where the report supplies fee rows they are what actually came out of the
// credit, and the policy figure becomes an independent second opinion.
func TestObservedFeesDisplacePolicy(t *testing.T) {
	at := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	observed, tax := p(2_500), p(450) // a gateway charging 2.50%, not 2.00%

	ds := &model.Dataset{
		Mode:           model.ModeMappingWithheld,
		DisputesJoined: true,
		Payments: []model.Payment{{
			ID: "pay", MerchantID: "m", Instrument: model.InstrumentCard, Currency: "INR",
			Gross: p(100_000), CapturedAt: at, FeeObserved: &observed, TaxObserved: &tax,
		}},
	}

	cfg := DefaultConfig()
	cfg.UseObservedFees = true
	r := Build(ds, cfg)[0]

	if r.MDR != observed {
		t.Errorf("applied fee = %s, want the observed %s", r.MDR, observed)
	}
	if r.PolicyMDR != p(2_000) {
		t.Errorf("policy fee = %s, want 2000 paise; the second opinion must survive", r.PolicyMDR)
	}
	if want := p(100_000) - observed - tax; r.Contribution != want {
		t.Errorf("contribution = %s, want %s: the credit reflects what was actually deducted",
			r.Contribution, want)
	}

	// With the same data in lump-credit mode the fee has to be policy-derived,
	// which is precisely why the fee check is circular there.
	cfg.UseObservedFees = false
	r2 := Build(ds, cfg)[0]
	if r2.MDR != r2.PolicyMDR {
		t.Errorf("policy-derived mode must use the policy fee, got %s against %s", r2.MDR, r2.PolicyMDR)
	}
}

// TestRecomputeClosesOnTheGeneratedIdentity is the check the whole verifier
// leans on: the equation re-derived from raw components has to equal the sum
// of the contributions the solver worked with, or the two halves of the system
// disagree about what a settlement is.
func TestRecomputeClosesOnEveryShape(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	cfg := DefaultConfig()
	at := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)

	for trial := 0; trial < 300; trial++ {
		ds := &model.Dataset{Mode: model.ModeLumpCredit, DisputesJoined: true}

		n := 1 + rng.Intn(8)
		for i := 0; i < n; i++ {
			id := string(rune('a' + i))
			gross := p(int64(rng.Intn(500_000) + 1_000))
			inst := []model.Instrument{
				model.InstrumentCard, model.InstrumentUPI, model.InstrumentNetbanking,
			}[rng.Intn(3)]
			ds.Payments = append(ds.Payments, model.Payment{
				ID: "pay_" + id, MerchantID: "m", Instrument: inst,
				Currency: "INR", Gross: gross, CapturedAt: at,
			})
			if rng.Intn(3) == 0 {
				full := rng.Intn(2) == 0
				amt := gross
				if !full {
					amt = p(int64(gross) / 2)
				}
				ds.Refunds = append(ds.Refunds, model.Refund{
					ID: "r_" + id, PaymentID: "pay_" + id, MerchantID: "m",
					Amount: amt, SettledAt: at, Full: full,
				})
			}
		}
		if rng.Intn(2) == 0 {
			ds.Chargebacks = append(ds.Chargebacks, model.Chargeback{
				ID: "cbk", MerchantID: "m",
				Disputed: p(int64(rng.Intn(200_000) + 1_000)), Fee: p(2_500), DebitedAt: at,
			})
		}
		if rng.Intn(3) == 0 {
			ds.Adjustments = append(ds.Adjustments, model.Adjustment{
				ID: "adj", MerchantID: "m",
				Amount: p(int64(rng.Intn(20_000)) - 10_000), AppliedAt: at,
			})
		}

		recs := Build(ds, cfg)
		target := money.Sum(model.Contributions(recs))

		eq := Recompute(recs, target, cfg)
		if !eq.Closes {
			t.Fatalf("trial %d: the re-derived identity does not close.\n"+
				"  reconstructed %s against a target of %s, residual %s\n"+
				"  gross %s mdr %s gst %s refunds %s chargebacks %s adjustments %s",
				trial, eq.Reconstructed, eq.Target, eq.Residual,
				eq.Gross, eq.MDR, eq.GST, eq.Refunds, eq.Chargebacks, eq.Adjustments)
		}
		if eq.Residual != 0 {
			t.Fatalf("trial %d: residual %s in declared mode, where the tolerance is zero",
				trial, eq.Residual)
		}
	}
}

// TestInferredModeBandIsCardinalityScaled guards the distinction that makes
// inferred mode usable at all: the allowance grows with the witness, not with
// the pool.
func TestInferredModeBandIsCardinalityScaled(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Mode = ModeInferred
	cfg.Delta = 1

	if cfg.EffectiveDelta() != 1 {
		t.Fatalf("inferred mode tolerance = %s, want 1 paise", cfg.EffectiveDelta())
	}
	declared := DefaultConfig()
	if declared.EffectiveDelta() != 0 {
		t.Fatalf("declared mode must have zero tolerance whatever Delta says, got %s",
			declared.EffectiveDelta())
	}

	at := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	ds := &model.Dataset{Mode: model.ModeLumpCredit, DisputesJoined: true}
	for i := 0; i < 5; i++ {
		ds.Payments = append(ds.Payments, model.Payment{
			ID: "pay_" + string(rune('a'+i)), MerchantID: "m",
			Instrument: model.InstrumentCard, Currency: "INR",
			Gross: p(100_000), CapturedAt: at,
		})
	}
	recs := Build(ds, cfg)
	for _, r := range recs {
		if r.Hi-r.Lo != 2*cfg.Delta {
			t.Fatalf("contribution interval width = %s, want twice the tolerance", r.Hi-r.Lo)
		}
	}

	target := money.Sum(model.Contributions(recs))
	eq := Recompute(recs, target, cfg)
	if eq.SlackAllowed != money.Paise(len(recs))*cfg.Delta {
		t.Errorf("slack allowed = %s, want witness cardinality times delta", eq.SlackAllowed)
	}

	// Two paise of drift on a five-record witness is inside the allowance.
	if eq := Recompute(recs, target+2, cfg); !eq.Closes {
		t.Error("2 paise of drift on a 5-record witness should be inside a 5-paise allowance")
	}
	// Six is not.
	if eq := Recompute(recs, target+6, cfg); eq.Closes {
		t.Error("6 paise of drift on a 5-record witness must exceed a 5-paise allowance")
	}
}

func TestPolicyValidation(t *testing.T) {
	good := DefaultPolicy()
	if err := good.Validate(); err != nil {
		t.Fatalf("the shipped policy does not validate: %v", err)
	}
	for name, mut := range map[string]func(*Policy){
		"no version":   func(p *Policy) { p.Version = "" },
		"absurd gst":   func(p *Policy) { p.GSTBps = 50_000 },
		"absurd mdr":   func(p *Policy) { p.MDRByInstrument[model.InstrumentCard] = 9_000 },
		"negative gst": func(p *Policy) { p.GSTBps = -1 },
	} {
		pol := DefaultPolicy()
		pol.MDRByInstrument = map[model.Instrument]int64{}
		for k, v := range DefaultPolicy().MDRByInstrument {
			pol.MDRByInstrument[k] = v
		}
		mut(&pol)
		if err := pol.Validate(); err == nil {
			t.Errorf("%s should not validate", name)
		}
	}
}

func TestPolicyFloorsAndCaps(t *testing.T) {
	pol := DefaultPolicy()
	pol.MinFee = map[model.Instrument]money.Paise{model.InstrumentCard: p(500)}
	pol.MaxFee = map[model.Instrument]money.Paise{model.InstrumentCard: p(5_000)}

	if got := pol.MDR(model.InstrumentCard, p(1_000)); got != p(500) {
		t.Errorf("a 2%% fee on 1000 paise is 20, so the 500 floor applies; got %s", got)
	}
	if got := pol.MDR(model.InstrumentCard, p(1_000_000)); got != p(5_000) {
		t.Errorf("a 2%% fee on 1000000 paise is 20000, so the 5000 cap applies; got %s", got)
	}
	if got := pol.MDR(model.InstrumentUPI, p(1_000_000)); got != 0 {
		t.Errorf("UPI carries no MDR under Indian regulation; got %s", got)
	}
	// An instrument with no configured rate falls back rather than charging
	// nothing, because silently zero-rating an unknown method would make every
	// contribution for it wrong in the same direction.
	if got := pol.MDR(model.Instrument("crypto"), p(100_000)); got != p(2_000) {
		t.Errorf("unknown instrument should fall back to the card rate, got %s", got)
	}
}
