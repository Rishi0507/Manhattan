package generate

import (
	"fmt"
	"time"

	"github.com/Rishi0507/manhattan/internal/accounting"
	"github.com/Rishi0507/manhattan/internal/model"
	"github.com/Rishi0507/manhattan/internal/money"
)

// Case is one adversarial scenario, with the outcome each system should
// produce.
//
// These exist because the track's bar is explicit that one cherry-picked
// match proves nothing. Each case is a specific, nameable way that a
// confidence-threshold matcher produces a wrong answer, and the expectations
// are written down so a regression shows up as a failing test rather than as
// a worse demo.
type Case struct {
	Number      int
	Name        string
	Scenario    string
	ExpectB0    string
	ExpectAxiom string
	// Why explains what this case is actually testing, in one line, for the
	// benchmark output.
	Why  string
	Spec Spec

	// WindowHours overrides the narrowing window half-width for this case.
	// Case 10 needs it: the pathology it exhibits is a misconfigured
	// value-date window, which is a property of the reconciler's settings
	// rather than of the data.
	WindowHours float64
	// InferredRounding runs the case with an unknown rounding convention.
	InferredRounding bool
	// GateScope forces the gate-derived search scope, for the cases whose
	// whole point is that the proof owes nothing to the counterparty's report.
	GateScope bool
}

// Cases returns the eleven scenarios in demo order.
func Cases() []Case {
	base := func(seed int64) Spec {
		s := DefaultSpec()
		s.Seed = seed
		s.Settlements = 6
		s.BatchJitter = 0
		s.PoolJitter = 0
		s.Start = time.Date(2026, 8, 20, 0, 0, 0, 0, time.UTC)
		return s
	}

	cs := []Case{
		{
			Number: 1, Name: "clean_positive_only",
			Scenario:    "50 candidates, no signed items, a five-record batch",
			ExpectB0:    "posts, correct",
			ExpectAxiom: "VERIFIED",
			Why:         "the base case; it should look boring, and a system that cannot do this is not worth discussing",
			Spec: func() Spec {
				s := base(1001)
				s.Archetype = "travel"
				s.BatchSize = 5
				s.PoolTarget = 40
				s.ChargebackRate = 0
				s.RefundRate = 0
				s.Pathology = "clean"
				return s
			}(),
		},
		{
			Number: 2, Name: "signed_items",
			Scenario:    "the batch contains a chargeback debit and a fully refunded payment",
			ExpectB0:    "posts, often wrong",
			ExpectAxiom: "VERIFIED with SIGNED_ITEMS_PRESENT",
			Why:         "sign is accountingly essential and computationally a non-event; nothing in the solver special-cases it",
			Spec: func() Spec {
				s := base(1002)
				s.Archetype = "travel"
				s.BatchSize = 6
				s.PoolTarget = 24
				s.ChargebackRate = 1.0
				s.RefundRate = 0.4
				s.FullRefundShare = 0.8
				s.Pathology = "signed_items"
				return s
			}(),
		},
		{
			Number: 3, Name: "large_batch_complement",
			Scenario:    "a 257-record batch drawn from a 260-record pool, with the settlement mapping withheld",
			ExpectB0:    "no match, low confidence",
			ExpectAxiom: "VERIFIED with COMPLEMENT_SOLVED, at the gate-derived scope",
			Why:         "the batch is almost the whole pool, so the answer is recovered by solving for the excluded set in the same enumeration",
			GateScope:   true,
			Spec: func() Spec {
				s := base(1003)
				s.Archetype = "travel"
				s.BatchSize = 257
				s.PoolTarget = 260
				s.Settlements = 3
				s.ChargebackRate = 0
				s.RefundRate = 0.1
				s.DeclareTxnCount = false // the proof must owe nothing to the report
				s.Pathology = "complement"
				return s
			}(),
		},
		{
			Number: 4, Name: "underdetermined",
			Scenario:    "the same 257-record batch, but narrowing leaves 320 candidates instead of 260",
			ExpectB0:    "posts, arbitrary",
			ExpectAxiom: "UNDERDETERMINED, refused before enumeration, with a computed remediation",
			Why:         "the pair with case 3: identical settlement, opposite verdict, and the difference is narrowing quality rather than the solver",
			Spec: func() Spec {
				s := base(1003)
				s.Archetype = "travel"
				s.BatchSize = 257
				s.PoolTarget = 320
				s.Settlements = 3
				s.ChargebackRate = 0
				s.RefundRate = 0.1
				s.DeclareTxnCount = true
				s.Pathology = "underdetermined"
				return s
			}(),
		},
		{
			Number: 5, Name: "ambiguous_twin",
			Scenario:    "two records carry an identical contribution, so a rival witness is constructible",
			ExpectB0:    "picks one, high confidence",
			ExpectAxiom: "AMBIGUOUS with TWIN_SWAP, both witnesses exhibited",
			Why:         "ambiguity proved by construction in linear time, with no search at all",
			Spec: func() Spec {
				s := base(1005)
				s.Archetype = "travel"
				s.BatchSize = 5
				s.PoolTarget = 34
				s.ChargebackRate = 0
				s.RefundRate = 0
				s.Pathology = "twin"
				return s
			}(),
		},
		{
			Number: 6, Name: "fee_anomaly_observed",
			Scenario:    "the reconstruction closes exactly, but the effective rate is 8 bps off policy",
			ExpectB0:    "silent",
			ExpectAxiom: "VERIFIED with FEE_ANOMALY, sized in basis points",
			Why:         "the money being accounted for and the fee being right are different questions",
			Spec: func() Spec {
				s := base(1006)
				s.Archetype = "travel"
				s.BatchSize = 5
				s.PoolTarget = 34
				s.ChargebackRate = 0
				s.RefundRate = 0
				s.FeeDriftBps = 8
				s.Mode = model.ModeMappingWithheld
				s.Pathology = "fee_anomaly"
				return s
			}(),
		},
		{
			Number: 7, Name: "fee_check_circular",
			Scenario:    "the same batch in lump-credit mode, where fee rows do not exist",
			ExpectB0:    "silent",
			ExpectAxiom: "VERIFIED with FEE_CHECK_CIRCULAR and no anomaly claim at all",
			Why:         "reporting a check that cannot fail is worse than reporting no check, because it looks like assurance",
			Spec: func() Spec {
				s := base(1006)
				s.Archetype = "travel"
				s.BatchSize = 5
				s.PoolTarget = 34
				s.ChargebackRate = 0
				s.RefundRate = 0
				s.FeeDriftBps = 8
				s.Mode = model.ModeLumpCredit
				s.Pathology = "fee_circular"
				return s
			}(),
		},
		{
			Number: 8, Name: "rounding_inferred",
			Scenario:         "the rounding convention is unknown, on a large pool",
			ExpectB0:         "posts, wrong subset",
			ExpectAxiom:      "VERIFIED with ROUNDING_APPLIED, band scaled by witness cardinality",
			Why:              "a pool-width tolerance band matches everything, which is worse than matching nothing",
			InferredRounding: true,
			Spec: func() Spec {
				s := base(1008)
				s.Archetype = "travel"
				s.BatchSize = 5
				s.PoolTarget = 26
				s.ChargebackRate = 0
				s.RefundRate = 0.1
				s.Pathology = "inferred_rounding"
				return s
			}(),
		},
		{
			Number: 9, Name: "unjoined_disputes_feed",
			Scenario:    "a chargeback exists in a disputes feed nobody wired into the pool",
			ExpectB0:    "no match",
			ExpectAxiom: "UNRESOLVED, then VERIFIED with RESOLVED_BY_HYPOTHESIS and a citation",
			Why:         "the finance-ops loop, closed end to end: model proposes, system finds the record, verifier decides",
			Spec: func() Spec {
				s := base(1009)
				s.Archetype = "travel"
				s.BatchSize = 6
				s.PoolTarget = 24
				s.ChargebackRate = 1.0
				s.RefundRate = 0.1
				s.JoinDisputes = false
				s.Pathology = "unjoined_disputes"
				return s
			}(),
		},
		{
			Number: 10, Name: "narrowing_sensitive",
			Scenario:    "narrowing drops a record that genuinely belonged, and a coincidental subset closes",
			ExpectB0:    "posts, confidently wrong",
			ExpectAxiom: "NARROWING_SENSITIVE, held for review, constraint named",
			Why:         "the dangerous failure nobody demos: a wrong posting with a proof attached",
			// Ten hours either side of the capture day's midpoint is a
			// plausible-looking but slightly-too-tight setting, and it is
			// enough to cut a late-evening capture out of its own batch.
			WindowHours: 10,
			Spec: func() Spec {
				s := base(1010)
				s.Archetype = "travel"
				s.BatchSize = 6
				s.PoolTarget = 30
				s.ChargebackRate = 0
				s.RefundRate = 0
				s.Pathology = "narrowing_sensitive"
				return s
			}(),
		},
		{
			Number: 11, Name: "subscription_entropy",
			Scenario:    "210 candidates drawn from three subscription price points",
			ExpectB0:    "posts, arbitrary",
			ExpectAxiom: "UNDERDETERMINED with AMOUNT_ENTROPY_INSUFFICIENT, refused in milliseconds",
			Why:         "these settlements are genuinely not reconstructable from amounts, and saying so fast is more useful than saying so slowly",
			Spec: func() Spec {
				s := base(1011)
				s.Archetype = "subscription_saas"
				s.BatchSize = 8
				s.PoolTarget = 210
				s.ChargebackRate = 0
				s.RefundRate = 0
				s.Pathology = "amount_entropy"
				return s
			}(),
		},
	}
	return cs
}

// Build materialises a case's dataset, applying the structural surgery that
// some pathologies need after generation.
func (c Case) Build() *model.Dataset {
	ds := Generate(c.Spec)
	switch c.Spec.Pathology {
	case "twin":
		plantTwin(ds)
	case "narrowing_sensitive":
		plantNarrowingTrap(ds)
	}
	RecomputeCredits(ds, c.Spec.Policy)
	return ds
}

// plantNarrowingTrap builds the single most dangerous failure this system
// guards against.
//
// A record that genuinely belongs to the batch is moved to a late-evening
// capture, so a slightly-too-tight value-date window cuts it out. A record
// that does NOT belong is given an identical contribution and left inside
// the window. The surviving pool therefore contains a subset that sums to
// the credit exactly, uniquely, and wrongly.
//
// Every arithmetic check in the system passes on that subset. The identity
// closes, the uniqueness count is one, the fee ratio is right. Only the
// neighbourhood probe catches it, by widening the window and discovering
// that a one-record substitution reproduces the same total. That is why the
// probe exists, and it is why this case is the one the demo opens with.
func plantNarrowingTrap(ds *model.Dataset) {
	pol := accounting.DefaultPolicy()

	for ci := range ds.Credits {
		credit := &ds.Credits[ci]
		truth := ds.GroundTruth[credit.Ref]
		if len(truth) == 0 {
			continue
		}
		inSet := map[string]bool{}
		for _, id := range truth {
			inSet[id] = true
		}

		var victim, impostor *model.Payment
		for i := range ds.Payments {
			p := &ds.Payments[i]
			if p.MerchantID != credit.MerchantID || p.Reconciled {
				continue
			}
			day := credit.ValueDate.AddDate(0, 0, -2)
			if p.CapturedAt.Before(day) || p.CapturedAt.After(day.Add(24*time.Hour)) {
				continue
			}
			if inSet[p.ID] && victim == nil {
				victim = p
			} else if !inSet[p.ID] && impostor == nil {
				impostor = p
			}
			if victim != nil && impostor != nil {
				break
			}
		}
		if victim == nil || impostor == nil {
			continue
		}

		day := credit.ValueDate.AddDate(0, 0, -2)
		// Late enough that a ten-hour half-width window excludes it, while a
		// fourteen-hour one keeps it. That is a two-hour configuration error,
		// which is entirely realistic.
		victim.CapturedAt = day.Add(23 * time.Hour)
		impostor.CapturedAt = day.Add(11 * time.Hour)

		impostor.Gross = victim.Gross
		impostor.Instrument = victim.Instrument
		if victim.FeeObserved != nil {
			fee := *victim.FeeObserved
			tax := money.MulRateBPS(fee, pol.GSTBps, pol.TaxRounding)
			if victim.TaxObserved != nil {
				tax = *victim.TaxObserved
			}
			impostor.FeeObserved = &fee
			impostor.TaxObserved = &tax
		}
		dropRefunds(ds, victim.ID)
		dropRefunds(ds, impostor.ID)
	}
}

// RecomputeCredits recalculates every bank credit from its ground-truth batch.
//
// Any surgery on generated data changes contributions, and a credit that no
// longer equals the sum of its own batch would make every downstream figure
// meaningless while looking like a solver failure. So the credits are always
// re-derived after the fact rather than assumed to have survived.
func RecomputeCredits(ds *model.Dataset, pol accounting.Policy) {
	if pol.Version == "" {
		pol = accounting.DefaultPolicy()
	}
	refunds := map[string]money.Paise{}
	for _, r := range ds.Refunds {
		refunds[r.PaymentID] += r.Amount
	}
	pays := map[string]*model.Payment{}
	for i := range ds.Payments {
		pays[ds.Payments[i].ID] = &ds.Payments[i]
	}
	cbs := map[string]model.Chargeback{}
	for _, c := range ds.Chargebacks {
		cbs[c.ID] = c
	}

	for ci := range ds.Credits {
		credit := &ds.Credits[ci]
		var total money.Paise
		for _, id := range ds.GroundTruth[credit.Ref] {
			if c, ok := cbs[id]; ok {
				total -= c.Disputed + c.Fee
				continue
			}
			p, ok := pays[id]
			if !ok {
				continue
			}
			mdr := pol.MDR(p.Instrument, p.Gross)
			if p.FeeObserved != nil {
				mdr = *p.FeeObserved
			}
			gst := pol.GST(mdr)
			if p.TaxObserved != nil {
				gst = *p.TaxObserved
			}
			total += p.Gross - mdr - gst - refunds[p.ID]
		}
		credit.Amount = total
		if credit.DeclaredTxnCount != nil {
			n := len(ds.GroundTruth[credit.Ref])
			credit.DeclaredTxnCount = &n
		}
	}
}

// plantTwin gives two pool records an identical contribution, one of which
// is inside the true batch.
//
// This is surgery on the generated data rather than a distributional
// accident, because the case has to fire deterministically. It is exactly
// the structure that makes a subscription merchant unreconstructable, in
// miniature: two records that the amounts cannot tell apart.
func plantTwin(ds *model.Dataset) {
	if len(ds.Credits) == 0 {
		return
	}
	pol := accounting.DefaultPolicy()

	for ci := range ds.Credits {
		credit := &ds.Credits[ci]
		truth := ds.GroundTruth[credit.Ref]
		if len(truth) == 0 {
			continue
		}

		// Find a payment inside the batch, and one outside it that shares the
		// merchant and lands in the same window.
		var inBatch, outside *model.Payment
		inSet := map[string]bool{}
		for _, id := range truth {
			inSet[id] = true
		}
		for i := range ds.Payments {
			p := &ds.Payments[i]
			if p.MerchantID != credit.MerchantID || p.Reconciled {
				continue
			}
			day := credit.ValueDate.AddDate(0, 0, -2)
			if p.CapturedAt.Before(day) || p.CapturedAt.After(day.Add(24*time.Hour)) {
				continue
			}
			if inSet[p.ID] && inBatch == nil {
				inBatch = p
			} else if !inSet[p.ID] && outside == nil {
				outside = p
			}
			if inBatch != nil && outside != nil {
				break
			}
		}
		if inBatch == nil || outside == nil {
			continue
		}

		// Both must sit squarely inside the narrowing window. A twin that
		// narrowing removes is not a twin at all: the case would then exhibit
		// narrowing sensitivity, which is a different finding entirely.
		day := credit.ValueDate.AddDate(0, 0, -2)
		inBatch.CapturedAt = day.Add(9 * time.Hour)
		outside.CapturedAt = day.Add(13 * time.Hour)

		// Make the outsider identical in every respect that determines its
		// contribution. Anything less and the twin is not a twin.
		outside.Gross = inBatch.Gross
		outside.Instrument = inBatch.Instrument
		if inBatch.FeeObserved != nil {
			fee := *inBatch.FeeObserved
			tax := money.MulRateBPS(fee, pol.GSTBps, pol.TaxRounding)
			if inBatch.TaxObserved != nil {
				tax = *inBatch.TaxObserved
			}
			outside.FeeObserved = &fee
			outside.TaxObserved = &tax
		}
		// Neither may carry a refund, or the contributions diverge again.
		dropRefunds(ds, inBatch.ID)
		dropRefunds(ds, outside.ID)
	}
}

func dropRefunds(ds *model.Dataset, paymentID string) {
	out := ds.Refunds[:0]
	for _, r := range ds.Refunds {
		if r.PaymentID != paymentID {
			out = append(out, r)
		}
	}
	ds.Refunds = out
}

// String renders a case header for benchmark output.
func (c Case) String() string {
	return fmt.Sprintf("case %2d  %-24s  %s", c.Number, c.Name, c.Scenario)
}
