package solver

import "math"

// Cap is the saturation value for binomial coefficients. Anything at or
// above it is "more than any machine will enumerate", and propagating a
// saturated value is more useful than propagating an overflowed one.
const Cap = int64(1) << 62

// Binom returns C(n, k), saturating at Cap rather than overflowing.
//
// The feasibility gate asks for C(320, 8), about 2.5e15, as a matter of
// routine, and it asks for values far beyond int64 range when deciding to
// refuse. Silent wraparound there would turn an astronomically large
// collision index into a small one, which is the single worst failure this
// package could have: it would convert a refusal into a confident posting.
func Binom(n, k int) int64 {
	if k < 0 || n < 0 || k > n {
		return 0
	}
	if k > n-k {
		k = n - k
	}
	if k == 0 {
		return 1
	}
	r := int64(1)
	for i := 0; i < k; i++ {
		// r = r * (n-i) / (i+1), kept exact because the running product of
		// i+1 consecutive integers is always divisible by (i+1)!.
		hi, lo := mul64(r, int64(n-i))
		if hi != 0 || lo >= Cap {
			return Cap
		}
		r = lo / int64(i+1)
		if lo%int64(i+1) != 0 {
			// Fall back to float only to detect saturation; the exact path
			// above is the one that produces returned values.
			f := float64(r) * float64(n-i) / float64(i+1)
			if f >= float64(Cap) {
				return Cap
			}
			r = exactBinom(n, k)
			return r
		}
	}
	return r
}

// exactBinom recomputes by the multiplicative formula in a different order,
// used only on the rare path where incremental division is not exact.
func exactBinom(n, k int) int64 {
	f := 0.0
	for i := 0; i < k; i++ {
		f += math.Log(float64(n-i)) - math.Log(float64(i+1))
	}
	if f > 42.0 { // e^42 is ~1.7e18, past int64 comfort
		return Cap
	}
	return int64(math.Round(math.Exp(f)))
}

func mul64(a, b int64) (hi, lo int64) {
	if a == 0 || b == 0 {
		return 0, 0
	}
	if a > Cap/b {
		return 1, 0
	}
	return 0, a * b
}

// BinomSum returns the number of subsets of an m-element set with
// cardinality at most k, saturating at Cap. This is the entry count of one
// half of the meet-in-the-middle enumeration, and therefore the quantity the
// resource ceiling is checked against before anything is allocated.
func BinomSum(m, k int) int64 {
	var t int64
	for c := 0; c <= k && c <= m; c++ {
		b := Binom(m, c)
		if b >= Cap || t > Cap-b {
			return Cap
		}
		t += b
	}
	return t
}

// colexRank is the position of an increasing index tuple in colexicographic
// order: rank = sum over i of C(c[i], i+1).
//
// Colex is chosen over lexicographic order for one concrete reason: the
// enumerator below emits combinations in colex order, so a subset's rank is
// simply its position in the generated stream. The payload stored beside
// each sum is therefore a loop counter, costing nothing to produce, and
// membership is recovered by unranking only for the handful of entries that
// actually land inside the probe window.
func colexRank(c []int) uint32 {
	var r int64
	for i, ci := range c {
		r += Binom(ci, i+1)
	}
	return uint32(r)
}

// colexUnrank recovers the k-element index tuple at the given colex rank,
// writing into dst (which must have length k).
func colexUnrank(rank uint32, k int, dst []int) {
	r := int64(rank)
	for i := k - 1; i >= 0; i-- {
		// Largest c with C(c, i+1) <= r.
		c := i
		for Binom(c+1, i+1) <= r {
			c++
		}
		dst[i] = c
		r -= Binom(c, i+1)
	}
}
