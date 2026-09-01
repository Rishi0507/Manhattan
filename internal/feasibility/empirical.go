package feasibility

import (
	"math"
	"math/rand"

	"github.com/Rishi0507/manhattan/internal/money"
	"github.com/Rishi0507/manhattan/internal/solver"
)

// The analytic collision index assumes the sums of k-subsets are locally
// uniform near the target, with a density read off a moment-matched normal.
// That assumption is doing real work, and on realistic merchant data it is
// wrong in a specific and measurable way.
//
// Ticket amounts are close to lognormal: most transactions are small and a
// thin tail is very large. A sum of three such amounts is nothing like a
// normal distribution in its body, and its true density near a typical target
// is several times higher than the normal approximation says. Measured on a
// 313-record travel pool at k = 3, the analytic index predicted 0.38 rival
// reconstructions and the exhaustive count found three, on every one of sixty
// seeds. That is not sampling noise; it is the wrong model.
//
// The direction of the error is the saving grace. An index that is too low
// makes the gate accept a region containing more rivals than predicted, and
// the exhaustive count inside that region then finds them and returns an
// ambiguous result. Precision is untouched; the cost is a search that could
// have been skipped, and a triage decision made on a number that was wrong.
//
// But a gate whose estimate is off by an order of magnitude is a poor gate,
// and the fix is cheap. Rather than assuming a density, measure it: sample
// k-subsets from the actual pool, and estimate how densely their sums pack
// around the actual target. It costs a few milliseconds, it is
// distribution-free, and it is right about lognormal merchants for the same
// reason it is right about uniform ones, which is that it never assumes
// anything about the shape.
//
// Both numbers are computed and both are reported. The analytic one is what
// the published model says; the empirical one is what this pool actually
// does. Where they disagree, the receipt shows the disagreement rather than
// quietly picking a winner.

// EmpiricalSamples is how many random subsets are drawn per cardinality.
// Eight thousand puts the standard error on the density estimate at a few
// per cent, which is far tighter than the decision needs, and the whole sweep
// across every cardinality costs under ten milliseconds.
const EmpiricalSamples = 8000

// densityEstimator holds sampled subset sums, one sorted slice per
// cardinality.
type densityEstimator struct {
	sums [][]int64 // indexed by cardinality
	n    int
}

// sampleSums draws EmpiricalSamples random subsets for every cardinality up
// to kMax, in a single pass.
//
// Each trial walks a random permutation prefix, so the running total after i
// steps is a uniformly drawn i-subset sum. The samples are correlated across
// cardinalities within a trial, which does not matter: each cardinality's
// marginal is what is being estimated, and each is unbiased.
func sampleSums(contribs []money.Paise, kMax int, seed int64) *densityEstimator {
	n := len(contribs)
	if kMax > n {
		kMax = n
	}
	est := &densityEstimator{sums: make([][]int64, kMax+1), n: n}
	for k := range est.sums {
		est.sums[k] = make([]int64, 0, EmpiricalSamples)
	}

	rng := rand.New(rand.NewSource(seed))
	idx := make([]int, n)
	for i := range idx {
		idx[i] = i
	}

	for t := 0; t < EmpiricalSamples; t++ {
		var running int64
		// Partial Fisher-Yates: only the first kMax positions are shuffled,
		// which is all the prefix needs and keeps the pass O(kMax) rather
		// than O(n) per trial.
		for i := 0; i < kMax; i++ {
			j := i + rng.Intn(n-i)
			idx[i], idx[j] = idx[j], idx[i]
			running += int64(contribs[idx[i]])
			est.sums[i+1] = append(est.sums[i+1], running)
		}
	}
	for k := 1; k < len(est.sums); k++ {
		sortInt64(est.sums[k])
	}
	return est
}

// densityAt estimates the per-paise probability that a random k-subset sums
// to exactly the given target.
//
// The bandwidth adapts: it starts from a fraction of the sample's own spread
// and widens until enough samples fall inside to make the estimate stable.
// Returning zero would claim a target is unreachable, which the estimator is
// not entitled to say, so a target outside the sampled range falls back to
// the analytic density instead.
func (e *densityEstimator) densityAt(k int, target int64, fallback float64) float64 {
	if k <= 0 || k >= len(e.sums) {
		return fallback
	}
	s := e.sums[k]
	if len(s) < 32 {
		return fallback
	}

	spread := float64(s[len(s)*9/10] - s[len(s)/10])
	if spread <= 0 {
		// Every sampled subset has the same sum, so the pool's amounts do not
		// discriminate at this cardinality at all. The entropy gate catches
		// this first; reporting a very high density here keeps the two gates
		// consistent rather than letting them contradict each other.
		return 1.0
	}

	const minInWindow = 60
	h := spread / 64
	for pass := 0; pass < 12; pass++ {
		lo := lowerBoundInt64(s, target-int64(h))
		hi := lowerBoundInt64(s, target+int64(h)+1)
		if cnt := hi - lo; cnt >= minInWindow {
			return float64(cnt) / (float64(len(s)) * 2 * h)
		}
		h *= 2
	}
	// The target sits far out in the tail of what this pool can produce at
	// this cardinality. Collisions there are rarer than the analytic model
	// says, not commoner, so taking the smaller of the two is the
	// conservative reading in the direction that costs recall.
	lo := lowerBoundInt64(s, target-int64(h))
	hi := lowerBoundInt64(s, target+int64(h)+1)
	d := float64(hi-lo) / (float64(len(s)) * 2 * h)
	if d < fallback {
		return d
	}
	return fallback
}

// EmpiricalIndex is the measured expected number of subsets at free
// cardinality k whose sum lands on the target.
//
// The region at free cardinality k contains subsets of size k and subsets of
// size n-k. The latter are counted through their complements, which sum to
// the pool total minus the target, so both sides are measured with the same
// sampled distribution and added.
func (e *densityEstimator) EmpiricalIndex(n, k int, target, total int64, gcd int64, analytic float64) float64 {
	if k <= 0 {
		return 0
	}
	c := solver.Binom(n, k)
	if c >= solver.Cap {
		return math.Inf(1)
	}
	analyticDensity := 0.0
	if c > 0 {
		analyticDensity = analytic / float64(c)
	}

	direct := e.densityAt(k, target, analyticDensity)
	complement := e.densityAt(k, total-target, analyticDensity)

	// A subset of size k and a subset of size n-k are the same object counted
	// from two ends only when n = 2k, in which case counting both would
	// double it.
	if n == 2*k {
		return float64(c) * float64(gcd) * direct
	}
	return float64(c) * float64(gcd) * (direct + complement)
}

func sortInt64(a []int64) {
	// Insertion sort for tiny slices, otherwise a simple in-place quicksort.
	// This is hot enough to matter and simple enough not to need more.
	if len(a) < 24 {
		for i := 1; i < len(a); i++ {
			v := a[i]
			j := i - 1
			for j >= 0 && a[j] > v {
				a[j+1] = a[j]
				j--
			}
			a[j+1] = v
		}
		return
	}
	mid := a[len(a)/2]
	i, j := 0, len(a)-1
	for i <= j {
		for a[i] < mid {
			i++
		}
		for a[j] > mid {
			j--
		}
		if i <= j {
			a[i], a[j] = a[j], a[i]
			i++
			j--
		}
	}
	sortInt64(a[:j+1])
	sortInt64(a[i:])
}

func lowerBoundInt64(a []int64, v int64) int {
	lo, hi := 0, len(a)
	for lo < hi {
		m := int(uint(lo+hi) >> 1)
		if a[m] < v {
			lo = m + 1
		} else {
			hi = m
		}
	}
	return lo
}
