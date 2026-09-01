package feasibility

import (
	"math"
	"math/rand"
	"testing"

	"github.com/Rishi0507/manhattan/internal/money"
	"github.com/Rishi0507/manhattan/internal/solver"
)

// TestIndexMatchesPublishedTable pins the collision index to the figures
// quoted in the design document, at the reference spread of 41.8 lakh paise.
//
// These numbers appear in the README, on the receipts and in the pitch. If
// the estimator drifts away from them, the documentation becomes a set of
// claims the code no longer makes, which is the exact failure mode this
// project exists to argue against.
func TestIndexMatchesPublishedTable(t *testing.T) {
	const sigma = 4_180_000.0

	cases := []struct {
		n, k int
		want float64
	}{
		{52, 5, 0.11},
		{52, 6, 0.79},
		{52, 7, 4.83},
		{52, 8, 25.4},
		{320, 3, 0.30},
		{320, 4, 20.5},
		{500, 3, 1.14},
		{1000, 3, 9.16},
		{320, 8, 8.4e7},
	}

	for _, c := range cases {
		got := Index(c.n, c.k, sigma, 1)
		// Two significant figures is the precision the table is quoted to.
		if rel := math.Abs(got-c.want) / c.want; rel > 0.02 {
			t.Errorf("E(n=%d, k=%d) = %.4g, published table says %.4g (%.1f%% off)",
				c.n, c.k, got, c.want, rel*100)
		}
	}
}

// analyticConfig pins the gate to the published closed form. The tests that
// use it are testing the model itself, so sampling would defeat the purpose.
func analyticConfig() Config {
	cfg := DefaultConfig()
	cfg.Empirical = false
	return cfg
}

// TestKStarIsTheDispatchParameter asserts the property the whole design
// rests on: the boundary the gate accepts and the region the solver searches
// are the same boundary.
func TestKStarIsTheDispatchParameter(t *testing.T) {
	contribs := syntheticPool(52, 4_180_000, 20260826)
	rep := Assess(contribs, 0, 1, nil, analyticConfig())

	if rep.Decision != DecideEnumerate {
		t.Fatalf("a 52-item pool at realistic spread should be enumerable, got %s", rep.Decision)
	}
	if rep.KStar != 7 {
		t.Fatalf("k* = %d, want 7 for this pool", rep.KStar)
	}
	if rep.IndexAtKStar > DefaultConfig().UnderdeterminedAbove {
		t.Fatalf("E at k* is %.3g, which is past the refusal threshold; k* must be the largest k still inside it", rep.IndexAtKStar)
	}
	// One past k* must be outside, or k* was not maximal.
	if e := Index(52, rep.KStar+1, rep.SigmaPaise, 1); e <= DefaultConfig().UnderdeterminedAbove {
		t.Fatalf("E at k*+1 is %.3g, still inside the threshold; k* is not maximal", e)
	}
	// The cumulative index over the searched region must be at least the top
	// term, and it is the number a receipt should quote.
	if rep.CumulativeAtKStar < rep.IndexAtKStar {
		t.Fatalf("cumulative index %.3g is below the top term %.3g", rep.CumulativeAtKStar, rep.IndexAtKStar)
	}
}

// TestRefusalIsSpecificAboutWhy checks that a hopeless pool is refused
// without enumeration and that the refusal names the population size.
func TestRefusalIsSpecificAboutWhy(t *testing.T) {
	contribs := syntheticPool(320, 4_180_000, 7)
	declared := 312
	rep := Assess(contribs, 0, 1, &declared, analyticConfig())

	if rep.ImpliedFreeCardinality == nil || *rep.ImpliedFreeCardinality != 8 {
		t.Fatalf("312 of 320 implies a free cardinality of 8, got %v", rep.ImpliedFreeCardinality)
	}
	if rep.IndexAtImplied == nil || *rep.IndexAtImplied < 1e6 {
		t.Fatalf("the implied cardinality should be astronomically collision-prone, got %v", rep.IndexAtImplied)
	}
	// k* itself is still small and positive: the pool is not hopeless at
	// every cardinality, only at the one the report claims.
	if rep.KStar != 3 {
		t.Fatalf("k* = %d, want 3 for a 320-item pool at this spread", rep.KStar)
	}
}

// TestLatticeCorrectionRaisesTheIndex covers the round-rupee case. Without
// the gcd correction, a pool of amounts all divisible by 100 looks a hundred
// times more distinguishable than it is, and the gate would accept a region
// full of rivals.
func TestLatticeCorrectionRaisesTheIndex(t *testing.T) {
	base := syntheticPool(60, 4_180_000, 11)
	rounded := make([]money.Paise, len(base))
	for i, v := range base {
		rounded[i] = money.Paise((int64(v) / 100) * 100)
	}
	gcd := money.GCD(rounded)
	if gcd != 100 {
		t.Fatalf("constructed pool should have lattice spacing 100, got %d", gcd)
	}

	uncorrected := Assess(rounded, 0, 1, nil, analyticConfig())
	corrected := Assess(rounded, 0, gcd, nil, analyticConfig())

	if !corrected.LatticeCorrectionApplied {
		t.Fatalf("correction flag not set")
	}
	if corrected.KStar >= uncorrected.KStar {
		t.Fatalf("lattice correction should shrink the accepted region: k* went from %d to %d",
			uncorrected.KStar, corrected.KStar)
	}
	if ratio := corrected.Curve[2].Index / uncorrected.Curve[2].Index; math.Abs(ratio-100) > 1e-6 {
		t.Fatalf("correction should scale the index by the lattice spacing, got a factor of %.4g", ratio)
	}
}

// TestResourceCeilingRefusesRatherThanAllocating asserts the prediction is
// consulted before the allocation, which is the difference between declining
// a job and being killed by the operating system halfway through one.
func TestResourceCeilingRefusesRatherThanAllocating(t *testing.T) {
	contribs := syntheticPool(1000, 4_180_000, 3)
	cfg := analyticConfig()
	cfg.MemoryCeilingBytes = 64 << 20 // 64 MB, well below what k=3 at n=1000 needs

	rep := Assess(contribs, 0, 1, nil, cfg)
	if rep.Decision != DecideResourceCeiling {
		t.Fatalf("decision = %s, want a resource-ceiling refusal", rep.Decision)
	}
	if rep.PredictedBytes <= cfg.MemoryCeilingBytes {
		t.Fatalf("predicted %d bytes, which is under the ceiling; the refusal is inconsistent", rep.PredictedBytes)
	}
	// And the prediction must agree with what the solver would actually build.
	entries, _ := solver.PredictEntries(1000, rep.KStar)
	if entries != rep.PredictedEntries {
		t.Fatalf("gate predicted %d entries, solver predicts %d", rep.PredictedEntries, entries)
	}
}

// TestErrorsCostRecallNotPrecision is the safety argument, asserted.
//
// The index is a heuristic, and admitting a heuristic into an otherwise
// deterministic pipeline is only defensible if every possible error runs in
// the same direction. If E is wrongly high, a verifiable settlement is
// refused: a lost auto-post, the cheap failure. If E is wrongly low, the gate
// accepts a slightly larger region and the exhaustive count inside that
// region catches the collision anyway. What must never happen is for the
// gate to shrink the searched region below what it declared enumerable.
func TestErrorsCostRecallNotPrecision(t *testing.T) {
	rng := rand.New(rand.NewSource(5))
	for trial := 0; trial < 200; trial++ {
		n := 20 + rng.Intn(200)
		sigma := math.Pow(10, 4+rng.Float64()*3)
		contribs := syntheticPool(n, sigma, int64(trial))
		gcd := money.GCD(contribs)
		rep := Assess(contribs, 0, gcd, nil, analyticConfig())

		if rep.Decision != DecideEnumerate {
			continue
		}
		// Whatever the gate accepted, the solver must be able to search all of
		// it inside the declared budget.
		_, bytes := solver.PredictEntries(n, rep.KStar)
		if bytes > DefaultConfig().MemoryCeilingBytes {
			t.Fatalf("trial %d: gate accepted k*=%d at n=%d but that needs %.0f MB, past the ceiling",
				trial, rep.KStar, n, float64(bytes)/(1<<20))
		}
		if rep.IndexAtKStar > analyticConfig().UnderdeterminedAbove {
			t.Fatalf("trial %d: accepted a region whose own index (%.3g) exceeds the refusal threshold",
				trial, rep.IndexAtKStar)
		}
	}
}

func syntheticPool(n int, sigma float64, seed int64) []money.Paise {
	rng := rand.New(rand.NewSource(seed))
	out := make([]money.Paise, n)
	for i := range out {
		v := money.Paise(rng.NormFloat64()*sigma + sigma*2.3)
		if v < 1000 {
			v = money.Paise(1000 + rng.Intn(int(sigma)+1000))
		}
		out[i] = v
	}
	return out
}
