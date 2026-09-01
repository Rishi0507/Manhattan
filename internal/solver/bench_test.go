package solver

import (
	"math/rand"
	"testing"
	"time"

	"github.com/Rishi0507/manhattan/internal/money"
)

// pool builds a pool with a contribution spread comparable to a real
// mid-market merchant: roughly 41.8 lakh paise of standard deviation.
func pool(n int, seed int64) []money.Paise {
	rng := rand.New(rand.NewSource(seed))
	out := make([]money.Paise, n)
	for i := range out {
		v := money.Paise(rng.NormFloat64()*4_180_000 + 9_600_000)
		if v < 10_000 {
			v = 10_000 + money.Paise(rng.Intn(500_000))
		}
		out[i] = v
	}
	return out
}

func plant(contribs []money.Paise, size int, seed int64) (money.Paise, []int) {
	rng := rand.New(rand.NewSource(seed))
	perm := rng.Perm(len(contribs))[:size]
	var t money.Paise
	for _, i := range perm {
		t += contribs[i]
	}
	return t, perm
}

// TestPerformanceGate is the hard gate the whole cost model rests on.
//
// Every timing published for this system assumes a flat, array-shaped
// enumeration: parallel primitive slices, a radix sort, and binary search
// over contiguous memory. A structurally identical implementation built out
// of per-entry objects and an interface-dispatched sort is correct and one
// to two orders of magnitude slower, at which point the published envelope
// becomes fiction. That is a real dependency on an implementation detail, so
// it is asserted here rather than hoped for.
func TestPerformanceGate(t *testing.T) {
	if testing.Short() {
		t.Skip("performance gate skipped under -short")
	}

	cases := []struct {
		name   string
		n      int
		kMax   int
		budget time.Duration
	}{
		{"n=52 gate-derived k*=7", 52, 7, 900 * time.Millisecond},
		{"n=52 declared count k=6", 52, 6, 400 * time.Millisecond},
		{"n=320 gate-derived k*=3", 320, 3, 900 * time.Millisecond},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			contribs := pool(tc.n, 20260826)
			target, want := plant(contribs, tc.kMax-1, 4417)

			start := time.Now()
			res := Solve(contribs, target, tc.kMax, 0, ScopeGate)
			elapsed := time.Since(start)

			if res.Matches < 1 {
				t.Fatalf("planted witness of size %d was not found", len(want))
			}
			if elapsed > tc.budget {
				t.Fatalf("solve AND uniqueness took %v, budget %v\n"+
					"  entries=%d memory=%.1f MB matches=%d\n"+
					"  this invalidates the published resource envelope; fix it here, not in the docs",
					elapsed, tc.budget, res.EntriesLeft+res.EntriesRght,
					float64(res.MemoryBytes)/(1<<20), res.Matches)
			}
			t.Logf("n=%d k_max=%d: %v, %d entries, %.1f MB, %d matches (%d rivals)",
				tc.n, tc.kMax, elapsed.Round(time.Millisecond),
				res.EntriesLeft+res.EntriesRght,
				float64(res.MemoryBytes)/(1<<20), res.Matches, res.Rivals)
		})
	}
}

func BenchmarkSolve52k7(b *testing.B) {
	contribs := pool(52, 20260826)
	target, _ := plant(contribs, 6, 4417)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Solve(contribs, target, 7, 0, ScopeGate)
	}
}

func BenchmarkSolve52k6(b *testing.B) {
	contribs := pool(52, 20260826)
	target, _ := plant(contribs, 6, 4417)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Solve(contribs, target, 6, 0, ScopeGate)
	}
}

func BenchmarkSolve320k3(b *testing.B) {
	contribs := pool(320, 20260826)
	target, _ := plant(contribs, 3, 4417)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Solve(contribs, target, 3, 0, ScopeGate)
	}
}
