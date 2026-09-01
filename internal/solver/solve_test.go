package solver

import (
	"math/rand"
	"testing"

	"github.com/Rishi0507/manhattan/internal/money"
)

// bruteForce enumerates every subset of the pool by brute force and returns
// the canonical index sets whose sum lands within the cardinality-scaled
// band, restricted to the same free-cardinality region the solver searches.
//
// This is the oracle. Meet-in-the-middle with colex ranks, radix-sorted
// buckets and a complement probe has a great many places to be subtly wrong,
// and "it produced a plausible answer" is not evidence of anything. Anything
// the solver claims about a pool of 20 items is checked against 2^20 subsets
// enumerated the dumb way.
func bruteForce(contribs []money.Paise, target money.Paise, kMax int, delta money.Paise) [][]int {
	n := len(contribs)
	var out [][]int
	for mask := 0; mask < 1<<n; mask++ {
		card := 0
		var sum money.Paise
		var idx []int
		for i := 0; i < n; i++ {
			if mask&(1<<i) != 0 {
				card++
				sum += contribs[i]
				idx = append(idx, i)
			}
		}
		free := card
		if n-card < free {
			free = n - card
		}
		if free > kMax {
			continue
		}
		d := sum - target
		if d < 0 {
			d = -d
		}
		if d <= money.Paise(card)*delta {
			if idx == nil {
				idx = []int{}
			}
			out = append(out, idx)
		}
	}
	return out
}

func TestSolveMatchesBruteForce(t *testing.T) {
	rng := rand.New(rand.NewSource(20260826))

	for trial := 0; trial < 400; trial++ {
		n := 6 + rng.Intn(13) // 6..18
		kMax := 1 + rng.Intn(4)

		contribs := make([]money.Paise, n)
		for i := range contribs {
			// A deliberately nasty distribution: small magnitudes so that
			// collisions are common, and roughly a fifth of items negative so
			// the signed path is exercised on most trials rather than
			// occasionally.
			v := money.Paise(rng.Intn(400) + 1)
			if rng.Intn(5) == 0 {
				v = -v
			}
			contribs[i] = v
		}

		// Target a genuine subset most of the time, and an arbitrary value
		// the rest, so both the found and not-found paths get exercised.
		var target money.Paise
		if rng.Intn(4) > 0 {
			pick := rng.Intn(kMax + 1)
			perm := rng.Perm(n)[:pick]
			for _, i := range perm {
				target += contribs[i]
			}
		} else {
			target = money.Paise(rng.Intn(2000) - 500)
		}

		delta := money.Paise(0)
		if trial%3 == 0 {
			delta = 1
		}

		want := bruteForce(contribs, target, kMax, delta)
		got := Solve(contribs, target, kMax, delta, ScopeGate)

		if got.Matches != len(want) {
			t.Fatalf("trial %d: n=%d kMax=%d delta=%d target=%d\n  solver counted %d, brute force found %d\n  contribs=%v",
				trial, n, kMax, delta, target, got.Matches, len(want), contribs)
		}

		// Every materialised witness must be a genuine member of the oracle's
		// set, not merely the right count.
		wantSet := map[string]bool{}
		for _, w := range want {
			wantSet[key(w)] = true
		}
		for _, w := range got.Witnesses {
			if !wantSet[key(w.Indices)] {
				t.Fatalf("trial %d: solver returned witness %v that brute force does not contain", trial, w.Indices)
			}
			var s money.Paise
			for _, i := range w.Indices {
				s += contribs[i]
			}
			if s != w.Sum {
				t.Fatalf("trial %d: witness sum recorded as %d but recomputes to %d (complement=%v)",
					trial, w.Sum, s, w.FromComplement)
			}
		}
	}
}

// TestComplementProbeFindsLargeWitness is the case a naive small-|S| solver
// silently cannot answer: the batch is almost the entire pool.
func TestComplementProbeFindsLargeWitness(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	n := 40
	contribs := make([]money.Paise, n)
	for i := range contribs {
		contribs[i] = money.Paise(100000 + rng.Intn(9000000))
	}
	// Batch is everything except three records.
	excluded := map[int]bool{4: true, 17: true, 33: true}
	var target money.Paise
	var wantIdx []int
	for i, v := range contribs {
		if !excluded[i] {
			target += v
			wantIdx = append(wantIdx, i)
		}
	}

	got := Solve(contribs, target, 3, 0, ScopeGate)
	if got.Matches != 1 {
		t.Fatalf("expected exactly one reconstruction, got %d", got.Matches)
	}
	w := got.Witnesses[0]
	if !w.FromComplement {
		t.Fatalf("a 37-of-40 witness should have been recovered by the complement probe")
	}
	if w.Size() != 37 {
		t.Fatalf("witness size = %d, want 37", w.Size())
	}
	if key(w.Indices) != key(wantIdx) {
		t.Fatalf("witness membership does not match the constructed batch")
	}
	if w.Sum != target {
		t.Fatalf("complement witness sum = %d, want %d", w.Sum, target)
	}
}

// TestDedupAtSmallPool covers the case where a single subset satisfies both
// |S| <= kMax and n-|S| <= kMax, so both probes find it. Without
// canonicalisation the count would read 2 and a correct unique answer would
// be reported as ambiguous.
func TestDedupAtSmallPool(t *testing.T) {
	contribs := []money.Paise{
		1_000_00, 2_500_00, 7_314_00, 990_00, 4_412_00, 8_881_00,
	}
	// n = 6, kMax = 4, so n <= 2*kMax and the regions overlap.
	target := contribs[0] + contribs[2] + contribs[4]
	got := Solve(contribs, target, 4, 0, ScopeGate)

	if !got.DedupApplied {
		t.Fatalf("dedup should be active when n <= 2*kMax (n=6, kMax=4)")
	}
	if got.Matches != 1 {
		t.Fatalf("matches = %d, want 1 (dedup removed %d)", got.Matches, got.DedupRemoved)
	}
	if got.DedupRemoved == 0 {
		t.Fatalf("expected the complement probe to rediscover the witness and be deduplicated")
	}
	if got.Rivals != 0 {
		t.Fatalf("rivals = %d, want 0", got.Rivals)
	}
}

// TestSignedItemsAreANonEvent asserts that a chargeback in the batch needs
// no special handling at all. The design claim is that sign is accountingly
// essential but computationally irrelevant; this is where that is checked
// rather than asserted.
func TestSignedItemsAreANonEvent(t *testing.T) {
	contribs := []money.Paise{
		1_24_000, 89_400, 2_10_500, 41_200, 18_900, -85_000, 3_11_000, -12_400,
	}
	want := []int{0, 1, 3, 5} // includes the chargeback
	var target money.Paise
	for _, i := range want {
		target += contribs[i]
	}

	got := Solve(contribs, target, 4, 0, ScopeGate)
	if got.Matches != 1 {
		t.Fatalf("matches = %d, want 1", got.Matches)
	}
	if key(got.Witnesses[0].Indices) != key(want) {
		t.Fatalf("witness = %v, want %v", got.Witnesses[0].Indices, want)
	}
}

// TestNearestMissIsUsable checks that an unsatisfiable target still yields
// the exact gap the resolution agent needs to work with.
func TestNearestMissIsUsable(t *testing.T) {
	contribs := []money.Paise{
		1_00_000, 2_00_000, 3_00_000, 4_00_000, 5_00_000, 6_00_000,
	}
	// Reachable at k<=2: sums of at most two items. 1_24_000 is not one.
	got := Solve(contribs, 1_24_000, 2, 0, ScopeGate)
	if got.Matches != 0 {
		t.Fatalf("expected no reconstruction, got %d", got.Matches)
	}
	if !got.Nearest.Valid {
		t.Fatalf("nearest miss should always be populated when nothing matched")
	}
	if got.Nearest.Gap != 24_000 {
		t.Fatalf("nearest gap = %s, want the distance to 1,00,000", got.Nearest.Gap)
	}
}

func TestColexRoundTrip(t *testing.T) {
	for _, m := range []int{5, 12, 26, 60} {
		for k := 1; k <= 4 && k <= m; k++ {
			total := Binom(m, k)
			if total > 5000 {
				total = 5000
			}
			dst := make([]int, k)
			for r := int64(0); r < total; r++ {
				colexUnrank(uint32(r), k, dst)
				for i := 1; i < k; i++ {
					if dst[i] <= dst[i-1] {
						t.Fatalf("m=%d k=%d rank=%d unranked to a non-increasing tuple %v", m, k, r, dst)
					}
				}
				if dst[k-1] >= m {
					t.Fatalf("m=%d k=%d rank=%d unranked out of range: %v", m, k, r, dst)
				}
				if got := colexRank(dst); int64(got) != r {
					t.Fatalf("m=%d k=%d: rank %d round-tripped to %d via %v", m, k, r, got, dst)
				}
			}
		}
	}
}

func TestRadixSortPairsHandlesNegatives(t *testing.T) {
	rng := rand.New(rand.NewSource(99))
	for trial := 0; trial < 50; trial++ {
		n := 1 + rng.Intn(3000)
		sums := make([]int64, n)
		ranks := make([]uint32, n)
		orig := map[int64]int{}
		for i := range sums {
			sums[i] = int64(rng.Intn(2_000_000)) - 1_000_000
			ranks[i] = uint32(i)
			orig[sums[i]]++
		}
		radixSortPairs(sums, ranks)
		for i := 1; i < n; i++ {
			if sums[i-1] > sums[i] {
				t.Fatalf("not sorted at %d: %d > %d", i, sums[i-1], sums[i])
			}
		}
		seen := map[int64]int{}
		for _, s := range sums {
			seen[s]++
		}
		for k, v := range orig {
			if seen[k] != v {
				t.Fatalf("multiset changed for %d: had %d, now %d", k, v, seen[k])
			}
		}
	}
}
