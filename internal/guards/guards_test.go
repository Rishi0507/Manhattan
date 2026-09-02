package guards

import (
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/Rishi0507/manhattan/internal/model"
	"github.com/Rishi0507/manhattan/internal/money"
	"github.com/Rishi0507/manhattan/internal/narrow"
)

func rec(id string, contrib int64, gross, mdr, policyMDR int64) model.Record {
	return model.Record{
		ID: id, Kind: model.KindPayment, MerchantID: "m", Currency: "INR",
		EventAt:      time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC),
		Gross:        money.Paise(gross),
		MDR:          money.Paise(mdr),
		PolicyMDR:    money.Paise(policyMDR),
		Contribution: money.Paise(contrib),
	}
}

func TestCardinalityCrossCheck(t *testing.T) {
	six := 6
	cases := []struct {
		name        string
		witnessSize int
		declared    *int
		zeroDropped int
		scoped      bool
		want        CheckState
	}{
		{"no declared count", 6, nil, 0, false, CheckInactive},
		{"exact match", 6, &six, 0, false, CheckPass},
		{"witness plus zeros reconciles", 5, &six, 1, false, CheckPass},
		{"mismatch even allowing for zeros", 4, &six, 1, false, CheckFail},
		{"mismatch with none set aside", 5, &six, 0, false, CheckFail},
		// Scoping the search by the declared count weakens this check but does
		// not void it: a witness smaller than the bound still fails.
		{"scoped by declared, exact", 6, &six, 0, true, CheckPass},
		{"scoped by declared, short", 4, &six, 0, true, CheckFail},
	}
	for _, c := range cases {
		got := CardinalityCrossCheck(c.witnessSize, c.declared, c.zeroDropped, c.scoped)
		if got.State != c.want {
			t.Errorf("%s: state = %s, want %s (%s)", c.name, got.State, c.want, got.Detail)
		}
	}

	// The weakening must be visible on the receipt, not merely true.
	scoped := CardinalityCrossCheck(6, &six, 0, true)
	if scoped.Detail == CardinalityCrossCheck(6, &six, 0, false).Detail {
		t.Error("a check scoped by the counterparty's own count must say so in its detail")
	}
}

func TestGrossRatioIsInactiveWhereItCannotFail(t *testing.T) {
	witness := []model.Record{rec("a", 97_640, 100_000, 2_000, 2_000)}
	pool := append([]model.Record{}, witness...)

	got := GrossRatioCheck(witness, pool, false, 2)
	if got.State != CheckInactive {
		t.Errorf("with policy-derived contributions the check cannot fail, so it must report "+
			"itself inactive rather than passing; got %s", got.State)
	}
}

// TestGrossRatioNormalisesInstrumentMix covers both corrections this guard
// needed: a uniform gateway overcharge must not block a posting, and a witness
// whose instrument mix differs from its pool must not either.
func TestGrossRatioNormalisesInstrumentMix(t *testing.T) {
	// A merchant whose gateway charges 250 bps against a policy of 200. Every
	// record drifts identically, so the witness looks exactly like its pool.
	var witness, pool []model.Record
	for i := 0; i < 4; i++ {
		r := rec(string(rune('a'+i)), 97_050, 100_000, 2_500, 2_000)
		witness = append(witness, r)
		pool = append(pool, r)
	}
	for i := 0; i < 20; i++ {
		pool = append(pool, rec("p"+string(rune('a'+i)), 97_050, 100_000, 2_500, 2_000))
	}

	if got := GrossRatioCheck(witness, pool, true, 2); got.State != CheckPass {
		t.Errorf("a uniform fee drift must cancel out of a pool comparison, so this witness "+
			"looks exactly like its population; got %s: %s", got.State, got.Detail)
	}

	// A witness whose INSTRUMENT MIX differs from the pool must still pass.
	// This is the fault that rejected eighty correct reconstructions: UPI
	// carries no fee and cards carry two per cent, so a small witness drawn
	// from a mixed pool has a very different blended rate through ordinary
	// sampling variation, with nothing wrong.
	var mixedPool []model.Record
	for i := 0; i < 12; i++ {
		// Card records: policy 200 bps, applied 200 bps.
		mixedPool = append(mixedPool, rec("c"+string(rune('a'+i)), 97_640, 100_000, 2_000, 2_000))
	}
	for i := 0; i < 12; i++ {
		// UPI records: policy 0 bps, applied 0 bps.
		mixedPool = append(mixedPool, rec("u"+string(rune('a'+i)), 100_000, 100_000, 0, 0))
	}
	allCard := mixedPool[:5] // a witness that happens to be entirely card
	if got := GrossRatioCheck(allCard, mixedPool, true, 2); got.State != CheckFail {
		// expected: passes, because deviation from policy is zero on both sides
	}
	if got := GrossRatioCheck(allCard, mixedPool, true, 2); got.State == CheckFail {
		t.Errorf("an all-card witness drawn from a half-UPI pool is correctly priced and must "+
			"pass; instrument mix has to be normalised out. got %s: %s", got.State, got.Detail)
	}

	// A witness containing records genuinely mispriced against policy, in a
	// way the rest of the pool is not, is what remains detectable.
	odd := []model.Record{
		rec("x", 90_000, 100_000, 9_000, 2_000),
		rec("y", 90_000, 100_000, 9_000, 2_000),
	}
	if got := GrossRatioCheck(odd, mixedPool, true, 2); got.State != CheckFail {
		t.Errorf("a witness priced 700 bps above policy inside a pool priced at policy should "+
			"fail; got %s: %s", got.State, got.Detail)
	}
}

func TestDriftMonitorNeedsABaseline(t *testing.T) {
	observed := map[narrow.Constraint]float64{
		narrow.ConstraintWindow:   0.77,
		narrow.ConstraintMerchant: 0.13,
	}

	// Without a stored baseline there is nothing to deviate from. This is the
	// documented hole: a constraint wrong from the first run is invisible.
	if got := DetectDrift(observed, DriftBaseline{}, 0.10); got != nil {
		t.Errorf("with no baseline the monitor cannot fire, got %v", got)
	}

	base := DriftBaseline{
		RunID: "baseline",
		Rates: map[narrow.Constraint]float64{
			narrow.ConstraintWindow:   0.30,
			narrow.ConstraintMerchant: 0.13,
		},
	}
	got := DetectDrift(observed, base, 0.10)
	if len(got) != 1 {
		t.Fatalf("expected exactly one drifted constraint, got %d: %v", len(got), got)
	}
	if got[0].Constraint != narrow.ConstraintWindow {
		t.Errorf("wrong constraint flagged: %s", got[0].Constraint)
	}
	if got[0].Gate != "hold_batch" {
		t.Errorf("drift must gate the batch, got %q", got[0].Gate)
	}

	// Within tolerance, nothing fires.
	if got := DetectDrift(map[narrow.Constraint]float64{
		narrow.ConstraintWindow: 0.35, narrow.ConstraintMerchant: 0.13,
	}, base, 0.10); len(got) != 0 {
		t.Errorf("a 5 point move inside a 10 point tolerance should not fire, got %v", got)
	}

	// Drift in the other direction is drift too: a constraint that suddenly
	// stops dropping anything is as suspicious as one that drops everything.
	if got := DetectDrift(map[narrow.Constraint]float64{
		narrow.ConstraintWindow: 0.05, narrow.ConstraintMerchant: 0.13,
	}, base, 0.10); len(got) != 1 {
		t.Errorf("a large drop in a drop rate is also drift, got %v", got)
	}
}

// TestProbeRefusesWhenItCannotDistinguish is the multiple-comparisons fix.
// Before it, a large witness produced a chance collision every time and the
// guard held every big batch forever.
func TestProbeRefusesWhenItCannotDistinguish(t *testing.T) {
	// A realistic contribution spread matters here. The probe's whole job is
	// to judge whether a collision would be surprising, so feeding it records
	// a few paise apart makes collisions genuinely certain and it correctly
	// refuses. Real settlement amounts are spread over lakhs.
	rng := rand.New(rand.NewSource(9))
	mk := func(n int) []model.Record {
		out := make([]model.Record, 0, n)
		for i := 0; i < n; i++ {
			out = append(out, rec(fmt.Sprintf("r%03d", i),
				int64(rng.Intn(9_000_000)+100_000), 0, 0, 0))
		}
		return out
	}

	// Small witness, small spare pool: a collision here would be surprising,
	// so the probe runs at full depth and its silence means something.
	small := Probe(mk(6), mk(40), 2, 0, nil, nil)
	if small.Inconclusive {
		t.Errorf("a 6-record witness against 34 spares is well inside the noise floor, "+
			"expected a usable probe; got %s", small.Note)
	}
	if small.MaxDepth < 1 {
		t.Errorf("depth collapsed to %d on a small witness", small.MaxDepth)
	}

	// Large witness: at depth 2 this compares on the order of 10^8 pairs, so a
	// chance collision is near certain and the probe must say so rather than
	// reporting a rival it was always going to find.
	large := Probe(mk(200), mk(400), 2, 0, nil, nil)
	if large.MaxDepth >= large.RequestedDepth && !large.Inconclusive {
		t.Errorf("a 200-record witness should have forced the probe to reduce depth or "+
			"declare itself inconclusive; got depth %d with %.3g expected chance collisions",
			large.MaxDepth, large.ExpectedSpurious)
	}
	if large.ExpectedSpurious <= 0 {
		t.Error("the probe must report its own expected false-positive count")
	}
}

// TestProbeFindsARealSubstitution is the case the guard exists for: narrowing
// dropped a true record and a coincidental one took its place.
func TestProbeFindsARealSubstitution(t *testing.T) {
	witness := []model.Record{
		rec("a", 100_000, 0, 0, 0),
		rec("b", 200_000, 0, 0, 0),
		rec("impostor", 300_000, 0, 0, 0),
	}
	// The widened pool contains the record narrowing cut out, carrying an
	// identical contribution to the impostor.
	widened := append([]model.Record{}, witness...)
	widened = append(widened,
		rec("victim", 300_000, 0, 0, 0),
		rec("noise1", 987_654, 0, 0, 0),
		rec("noise2", 1_234_567, 0, 0, 0),
	)
	admitted := map[string]narrow.Constraint{"victim": narrow.ConstraintWindow}

	got := Probe(witness, widened, 2, 0, []narrow.Constraint{narrow.ConstraintWindow}, admitted)
	if got.Stable {
		t.Fatalf("the probe missed a depth-1 substitution: %s", got.Note)
	}
	if got.Rival == nil {
		t.Fatal("a rival was reported without being exhibited")
	}
	if got.Culprit != narrow.ConstraintWindow {
		t.Errorf("the admitting constraint should be named, got %q", got.Culprit)
	}
}
