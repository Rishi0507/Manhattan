// Package feasibility decides, before any expensive work, whether a unique
// reconstruction can exist at all, and configures the search accordingly.
//
// This is the check that keeps the system honest, and it is the one most
// reconcilers in this space do not have.
//
// Uniqueness is not a property you can hope for. It is a function of how many
// candidate subsets exist relative to how many distinct sums those subsets
// can produce, and both quantities are computable from the pool before any
// search begins. The number of subsets at free cardinality k is C(n, k).
// Their sums concentrate, by the central limit theorem, into a distribution
// whose local density near the mean is about d / (sigma * sqrt(2*pi*k)),
// where sigma is the standard deviation of the pool's contributions and d is
// the lattice spacing. The expected number of subsets colliding at any one
// target is therefore
//
//	E = C(n, k) * d / (sigma * sqrt(2*pi*k))
//
// which costs one binomial, one standard deviation and one gcd to evaluate.
//
// Read the resulting numbers honestly and they say something uncomfortable:
// verification by reconstruction is only decisive when narrowing leaves an
// excess of roughly three to seven items over the true batch. Beyond that,
// uniqueness is not merely expensive to establish, it does not exist.
// Thousands of subsets hit the target and no amount of compute changes that.
// So the gate does not pretend, and it does not merely triage: its output
// k* is the parameter the solver is dispatched on.
package feasibility

import (
	"fmt"
	"math"

	"github.com/Rishi0507/manhattan/internal/money"
	"github.com/Rishi0507/manhattan/internal/solver"
)

// Decision is what the gate concluded.
type Decision string

const (
	// DecideEnumerate: the region is small enough that an exhaustive count
	// can settle the question.
	DecideEnumerate Decision = "enumerate"
	// DecideUnderdetermined: the combinatorics guarantee a large population of
	// rival reconstructions. Exhibiting two of them would misrepresent the
	// situation, so nothing is enumerated and nothing is exhibited.
	DecideUnderdetermined Decision = "refuse_before_enumeration"
	// DecideResourceCeiling: the region is decidable in principle but the
	// enumeration would not fit in the configured memory budget.
	DecideResourceCeiling Decision = "refuse_resource_ceiling"
)

// Config carries the thresholds. Every one of them is printed on the
// receipts it influences, because a threshold that determines whether money
// moves does not get to be implicit.
type Config struct {
	// UnderdeterminedAbove is the collision index past which the gate refuses
	// without searching. Default 10.
	UnderdeterminedAbove float64
	// LikelyUniqueBelow is the index below which uniqueness is very likely.
	// Purely descriptive; it changes no behaviour. Default 0.1.
	LikelyUniqueBelow float64
	// MemoryCeilingBytes bounds the enumeration. Default 1 GiB.
	MemoryCeilingBytes int64
	// KHardCap bounds k* regardless of the index, as a backstop.
	KHardCap int
	// Empirical measures the collision index by sampling the pool rather than
	// assuming a normal sum distribution. It is on by default because the
	// analytic form is measurably wrong on heavy-tailed merchants; turning it
	// off falls back to the published closed form and is useful only for
	// reproducing that model's own numbers.
	Empirical bool
	// SampleSeed keeps the empirical estimate deterministic, so a run replays
	// to the same decisions.
	SampleSeed int64
}

// DefaultConfig returns the shipped thresholds.
func DefaultConfig() Config {
	return Config{
		UnderdeterminedAbove: 10.0,
		LikelyUniqueBelow:    0.1,
		MemoryCeilingBytes:   1 << 30,
		KHardCap:             12,
		Empirical:            true,
		SampleSeed:           20260826,
	}
}

// Report is the gate's output, attached verbatim to every receipt.
type Report struct {
	N          int     `json:"n"`
	SigmaPaise float64 `json:"contribution_sigma_paise"`
	LatticeGCD int64   `json:"lattice_gcd_paise"`

	// KStar is the largest free cardinality whose collision index is still
	// within the contested band. It is both the triage boundary and the
	// solver's dispatch parameter; the two are deliberately the same number.
	KStar int `json:"k_star"`
	// IndexAtKStar is E evaluated at k*, by whichever estimator was used.
	IndexAtKStar float64 `json:"collision_index_at_k_star"`
	// Estimator names how the index was arrived at, and AnalyticAtKStar is
	// what the published closed form would have said. Both are on the receipt
	// because they disagree on realistic data, and a receipt that showed only
	// the one that happened to be used would hide a known limitation of the
	// model this system publishes.
	Estimator       string  `json:"collision_index_estimator"`
	AnalyticAtKStar float64 `json:"collision_index_analytic_at_k_star"`
	// CumulativeAtKStar sums E over every cardinality the dispatch will
	// actually search. Because meet-in-the-middle at k* covers the whole
	// region k(S) <= k*, this cumulative figure, not the top term alone, is
	// the number of rivals to expect. The two are close because E is
	// dominated by its top term, but quoting the smaller one would flatter
	// the result.
	CumulativeAtKStar float64 `json:"cumulative_index_at_k_star"`

	ThresholdUnderdetermined float64 `json:"threshold_underdetermined"`
	LatticeCorrectionApplied bool    `json:"lattice_correction_applied"`

	Decision Decision `json:"decision"`

	// ImpliedFreeCardinality is k(S) implied by a declared transaction count,
	// where the report supplied one. It is what makes a refusal specific:
	// "this batch is claimed to be 312 of 320, which is k = 8" is a far more
	// useful statement than "too many possibilities".
	DeclaredTxnCount       *int     `json:"declared_txn_count,omitempty"`
	ImpliedFreeCardinality *int     `json:"implied_free_cardinality,omitempty"`
	IndexAtImplied         *float64 `json:"collision_index_at_implied,omitempty"`

	PredictedEntries int64 `json:"predicted_entries"`
	PredictedBytes   int64 `json:"predicted_bytes"`
	MemoryCeiling    int64 `json:"memory_ceiling_bytes"`

	// Curve is E at each cardinality up to the hard cap, so a receipt can
	// show where the boundary sits rather than only that it was crossed.
	Curve []Point `json:"curve"`

	Note string `json:"note"`
}

// Point is one evaluation of the collision index.
type Point struct {
	K        int     `json:"k"`
	Subsets  float64 `json:"subsets"`
	Index    float64 `json:"collision_index"`
	Analytic float64 `json:"collision_index_analytic"`
}

// Sigma is the population standard deviation of the contributions, in paise,
// as a float. This is the one place a float appears in the decision path,
// and it appears in an estimator that is explicitly labelled as an estimator.
// It never touches an amount, a witness or a posting.
func Sigma(contribs []money.Paise) float64 {
	n := len(contribs)
	if n < 2 {
		return 0
	}
	var mean float64
	for _, v := range contribs {
		mean += float64(v)
	}
	mean /= float64(n)
	var ss float64
	for _, v := range contribs {
		d := float64(v) - mean
		ss += d * d
	}
	return math.Sqrt(ss / float64(n))
}

// Index evaluates the collision index at one cardinality.
func Index(n, k int, sigma float64, gcd int64) float64 {
	if k <= 0 {
		return 0
	}
	c := solver.Binom(n, k)
	if sigma <= 0 {
		// Every contribution is identical. Every subset of a given size has
		// the same sum, so the whole binomial collides. The entropy gate
		// catches this first; returning the honest value here means the two
		// gates cannot disagree.
		return float64(c)
	}
	subsets := float64(c)
	if c >= solver.Cap {
		subsets = IndexSaturated
	}
	density := float64(gcd) / (sigma * math.Sqrt(2*math.Pi*float64(k)))
	return clampIndex(subsets * density)
}

// IndexSaturated is the value reported when the collision index exceeds any
// meaningful scale.
//
// It is finite on purpose. An infinity serialises to nothing a JSON receipt
// can carry, and a receipt that cannot be written is worse than one carrying
// a saturated number: the distinction between "1e18 reconstructions" and
// "more than 1e18 reconstructions" changes no decision anyone will ever make,
// while a receipt that fails to serialise loses the whole audit trail.
const IndexSaturated = 1e18

func clampIndex(v float64) float64 {
	if math.IsNaN(v) {
		return IndexSaturated
	}
	if v > IndexSaturated || math.IsInf(v, 1) {
		return IndexSaturated
	}
	if v < 0 {
		return 0
	}
	return v
}

// Assess runs the gate over a pool and returns k* plus the full curve.
//
// declared, when non-nil, is the transaction count the settlement report
// states for this batch. It is used only to make a refusal specific and to
// offer the tighter dispatch scope; it never widens what the gate accepts.
func Assess(contribs []money.Paise, target money.Paise, gcd int64, declared *int, cfg Config) Report {
	n := len(contribs)
	sigma := Sigma(contribs)
	total := money.Sum(contribs)

	rep := Report{
		N:                        n,
		SigmaPaise:               sigma,
		LatticeGCD:               gcd,
		ThresholdUnderdetermined: cfg.UnderdeterminedAbove,
		LatticeCorrectionApplied: gcd > 1,
		MemoryCeiling:            cfg.MemoryCeilingBytes,
		DeclaredTxnCount:         declared,
	}

	hard := cfg.KHardCap
	if hard > n/2 {
		hard = n / 2
	}
	if hard < 1 {
		hard = 1
	}

	rep.Estimator = "analytic_normal"
	var est *densityEstimator
	if cfg.Empirical && n >= 8 {
		rep.Estimator = "empirical_subset_sampling"
		est = sampleSums(contribs, hard, cfg.SampleSeed)
	}

	indexAt := func(k int) (used, analytic float64) {
		analytic = Index(n, k, sigma, gcd)
		if est == nil {
			return analytic, analytic
		}
		return est.EmpiricalIndex(n, k, int64(target), int64(total), gcd, analytic), analytic
	}

	kStar := 0
	var cum float64
	var cumAtStar float64
	for k := 1; k <= hard; k++ {
		e, a := indexAt(k)
		rep.Curve = append(rep.Curve, Point{
			K: k, Subsets: float64(solver.Binom(n, k)), Index: e, Analytic: a,
		})
		if e <= cfg.UnderdeterminedAbove {
			kStar = k
			cum += e
			cumAtStar = cum
			rep.IndexAtKStar = e
			rep.AnalyticAtKStar = a
		}
	}
	rep.KStar = kStar
	rep.CumulativeAtKStar = cumAtStar

	if declared != nil {
		implied := *declared
		if n-implied < implied {
			implied = n - implied
		}
		if implied < 0 {
			implied = 0
		}
		e, _ := indexAt(implied)
		rep.ImpliedFreeCardinality = &implied
		rep.IndexAtImplied = &e
	}

	if kStar == 0 {
		rep.Decision = DecideUnderdetermined
		e1, _ := indexAt(1)
		rep.Note = fmt.Sprintf(
			"even a single-record reconstruction collides an estimated %.3g times in this pool; "+
				"amounts carry too little information to identify anything", e1)
		return rep
	}

	// A declared transaction count can put the batch outside the region any
	// search could settle. That is a refusal the gate can make before
	// touching memory, and it is a far more specific one than "too many
	// possibilities": the report says this batch is 312 of 320, which is a
	// free cardinality of 8, and at 8 this pool admits an estimated 8.4e7
	// reconstructions. Enumerating would be both futile and dishonest.
	if rep.ImpliedFreeCardinality != nil && rep.IndexAtImplied != nil {
		implied, idx := *rep.ImpliedFreeCardinality, *rep.IndexAtImplied
		if idx > cfg.UnderdeterminedAbove || implied > kStar {
			rep.Decision = DecideUnderdetermined
			rep.Note = fmt.Sprintf(
				"the declared batch of %d records in a pool of %d implies a free cardinality of %d, "+
					"at which an estimated %.3g subsets hit this target; the decidable region here stops at k = %d",
				*rep.DeclaredTxnCount, n, implied, idx, kStar)
			return rep
		}
	}

	entries, bytes := solver.PredictEntries(n, kStar)
	rep.PredictedEntries, rep.PredictedBytes = entries, bytes

	if bytes > cfg.MemoryCeilingBytes {
		rep.Decision = DecideResourceCeiling
		rep.Note = fmt.Sprintf(
			"the region k(S) <= %d is decidable in principle but enumerating it needs %.0f MB against a %.0f MB ceiling; "+
				"narrowing the pool is the remedy",
			kStar, float64(bytes)/(1<<20), float64(cfg.MemoryCeilingBytes)/(1<<20))
		return rep
	}

	rep.Decision = DecideEnumerate
	switch {
	case rep.IndexAtKStar < cfg.LikelyUniqueBelow:
		rep.Note = "uniqueness very likely at this scope"
	default:
		rep.Note = fmt.Sprintf(
			"contested band: an estimated %.2f subsets hit this target across the searched region, "+
				"so an ambiguous outcome is a normal and correct result here", cumAtStar)
	}
	return rep
}

// Underdetermined reports whether the gate refused.
func (r Report) Underdetermined() bool {
	return r.Decision == DecideUnderdetermined || r.Decision == DecideResourceCeiling
}

// EstimatedReconstructions is the population size a refusal is claiming
// exists, used verbatim in the refusal's own wording.
func (r Report) EstimatedReconstructions() float64 {
	if r.IndexAtImplied != nil {
		return *r.IndexAtImplied
	}
	var worst float64
	for _, p := range r.Curve {
		if p.Index > worst {
			worst = p.Index
		}
	}
	return worst
}
