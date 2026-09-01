package pipeline

import (
	"testing"

	"github.com/Rishi0507/manhattan/internal/evidence"
	"github.com/Rishi0507/manhattan/internal/generate"
	"github.com/Rishi0507/manhattan/internal/model"
)

// TestGeneratorAgreesWithAccountingEngine is the check the entire benchmark
// rests on and it should run first.
//
// The generator computes each bank credit from its own arithmetic, and the
// accounting engine recomputes contributions independently. If the two ever
// disagreed, every ground-truth batch would fail to reconstruct, every
// accuracy figure would be wrong, and the failure would look like a solver
// bug rather than like a measurement bug.
func TestGeneratorAgreesWithAccountingEngine(t *testing.T) {
	spec := generate.DefaultSpec()
	spec.Settlements = 12
	ds := generate.Generate(spec)

	e := New(ds, DefaultConfig())

	for _, credit := range ds.Credits {
		truth := ds.GroundTruth[credit.Ref]
		var sum int64
		for _, id := range truth {
			r, ok := e.ByID[id]
			if !ok {
				t.Fatalf("%s: ground-truth record %s is not in the record universe", credit.Ref, id)
			}
			sum += int64(r.Contribution)
		}
		if sum != int64(credit.Amount) {
			t.Fatalf("%s: the %d ground-truth records sum to %d paise but the credit is %d paise (off by %d)",
				credit.Ref, len(truth), sum, credit.Amount, sum-int64(credit.Amount))
		}
	}
}

// TestReconcileEndToEnd runs the full pipeline and checks the shape of what
// comes out: every receipt is well formed, every VERIFIED one satisfies the
// status invariants, and every VERIFIED one is actually correct against
// ground truth the pipeline never saw.
func TestReconcileEndToEnd(t *testing.T) {
	spec := generate.DefaultSpec()
	spec.Settlements = 25
	ds := generate.Generate(spec)

	e := New(ds, DefaultConfig())
	store := evidence.NewStore()

	counts := map[evidence.Status]int{}
	wrong := 0

	for _, credit := range ds.Credits {
		rec := e.Reconcile(credit)
		if err := store.Put(rec); err != nil {
			t.Fatalf("%s: %v", credit.Ref, err)
		}
		counts[rec.Status]++

		if rec.Status != evidence.StatusVerified {
			continue
		}
		// A posting is only correct if the witness is exactly the ground-truth
		// batch. Matching the amount is not enough: matching the amount with
		// the wrong records is the failure this whole system exists to prevent.
		if !sameSet(rec.Witness, ds.GroundTruth[credit.Ref]) {
			wrong++
			t.Errorf("%s: VERIFIED but the witness is not the true batch\n  got  %v\n  want %v",
				credit.Ref, rec.Witness, ds.GroundTruth[credit.Ref])
		}
	}

	t.Logf("statuses over %d settlements: %v", len(ds.Credits), counts)
	if counts[evidence.StatusVerified] == 0 {
		t.Fatalf("nothing verified at all; the pipeline is not reaching a decision")
	}
	if wrong != 0 {
		t.Fatalf("%d settlements were auto-posted with the wrong batch", wrong)
	}
}

// TestVerifiedIsNeverWrongAcrossArchetypes is the headline safety property,
// swept across every merchant shape the generator models.
//
// The claim is not that Manhattan verifies everything. It is that what it
// verifies is right. A refusal costs an analyst twenty minutes; a wrong
// posting is found at audit months later and costs one to two orders of
// magnitude more, so every heuristic in the system is placed to cost recall
// rather than precision. This test is where that is measured rather than
// asserted.
func TestVerifiedIsNeverWrongAcrossArchetypes(t *testing.T) {
	for _, arch := range generate.Archetypes {
		t.Run(arch.Name, func(t *testing.T) {
			spec := generate.DefaultSpec()
			spec.Archetype = arch.Name
			spec.Settlements = 20
			spec.Seed = 4417
			ds := generate.Generate(spec)

			e := New(ds, DefaultConfig())
			verified, wrong := 0, 0
			for _, credit := range ds.Credits {
				rec := e.Reconcile(credit)
				if err := rec.Validate(); err != nil {
					t.Fatalf("%s: %v", credit.Ref, err)
				}
				if rec.Status != evidence.StatusVerified {
					continue
				}
				verified++
				if !sameSet(rec.Witness, attributable(ds.GroundTruth[credit.Ref], e)) {
					wrong++
					t.Errorf("%s: auto-posted the wrong batch", credit.Ref)
				}
			}
			t.Logf("%s (%s): %d/%d verified, %d wrong",
				arch.Name, arch.ExpectedRegime, verified, len(ds.Credits), wrong)
		})
	}
}

// TestLumpCreditModeMakesNoFeeClaim asserts the circularity handling. In
// lump-credit mode the observed fee is derived from the same policy that
// built the contributions, so agreement is guaranteed and means nothing.
func TestLumpCreditModeMakesNoFeeClaim(t *testing.T) {
	spec := generate.DefaultSpec()
	spec.Mode = model.ModeLumpCredit
	spec.Settlements = 10
	ds := generate.Generate(spec)

	e := New(ds, DefaultConfig())
	sawCheck := false
	for _, credit := range ds.Credits {
		rec := e.Reconcile(credit)
		if rec.FeeCheck == nil {
			continue
		}
		sawCheck = true
		if !rec.FeeCheck.Circular {
			t.Fatalf("%s: fee check should report itself circular in lump-credit mode", credit.Ref)
		}
		if !rec.HasFlag(evidence.FlagFeeCheckCircular) {
			t.Fatalf("%s: FEE_CHECK_CIRCULAR flag missing", credit.Ref)
		}
		if rec.HasFlag(evidence.FlagFeeAnomaly) {
			t.Fatalf("%s: an anomaly claim was made in a mode where the check cannot fail", credit.Ref)
		}
		// And the guard that depends on the same independence must report
		// itself inactive rather than passing.
		for _, c := range rec.Narrowing.Checks {
			if c.Name == "gross_ratio_sanity" && c.State == "pass" {
				t.Fatalf("%s: gross-ratio guard reported a pass where it cannot fail", credit.Ref)
			}
		}
	}
	if !sawCheck {
		t.Skip("no settlement reached a witness in this dataset")
	}
}

// attributable drops the ground-truth records whose signed contribution is
// exactly zero.
//
// Such a record moved no money, so no amount-based reconstruction can place
// it in or out of a batch. Manhattan removes it before searching and names it
// separately on the receipt, and the accounting identity closes exactly
// either way. Counting it against the reconstruction would mark the system
// wrong for declining to guess.
func attributable(truth []string, e *Engine) []string {
	out := make([]string, 0, len(truth))
	for _, id := range truth {
		if r, ok := e.ByID[id]; ok && r.Contribution == 0 {
			continue
		}
		out = append(out, id)
	}
	return out
}

func sameSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	m := map[string]int{}
	for _, x := range a {
		m[x]++
	}
	for _, x := range b {
		m[x]--
		if m[x] < 0 {
			return false
		}
	}
	return true
}
