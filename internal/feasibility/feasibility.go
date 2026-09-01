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
}

// DefaultConfig returns the shipped thresholds.
func DefaultConfig() Config {
	return Config{
		UnderdeterminedAbove: 10.0,
		LikelyUniqueBelow:    0.1,
		MemoryCeilingBytes:   1 << 30,
		KHardCap:             12,
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
	// IndexAtKStar is E evaluated at k*.
	IndexAtKStar float64 `json:"collision_index_at_k_star"`
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
	K       int     `json:"k"`
	Subsets float64 `json:"subsets"`
	Index   float64 `json:"collision_index"`
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
		subsets = math.Inf(1)
	}
	density := float64(gcd) / (sigma * math.Sqrt(2*math.Pi*float64(k)))
	return subsets * density
}

// Assess runs the gate over a pool and returns k* plus the full curve.
//
// declared, when non-nil, is the transaction count the settlement report
// states for this batch. It is used only to make a refusal specific and to
// offer the tighter dispatch scope; it never widens what the gate accepts.
func Assess(contribs []money.Paise, gcd int64, declared *int, cfg Config) Report {
	n := len(contribs)
	sigma := Sigma(contribs)

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

	kStar := 0
	var cum float64
	var cumAtStar float64
	for k := 1; k <= hard; k++ {
		e := Index(n, k, sigma, gcd)
		rep.Curve = append(rep.Curve, Point{K: k, Subsets: float64(solver.Binom(n, k)), Index: e})
		if e <= cfg.UnderdeterminedAbove {
			kStar = k
			cum += e
			cumAtStar = cum
			rep.IndexAtKStar = e
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
		e := Index(n, implied, sigma, gcd)
		rep.ImpliedFreeCardinality = &implied
		rep.IndexAtImplied = &e
	}

	if kStar == 0 {
		rep.Decision = DecideUnderdetermined
		e1 := Index(n, 1, sigma, gcd)
		rep.Note = fmt.Sprintf(
			"even a single-record reconstruction collides an estimated %.3g times in this pool; "+
				"amounts carry too little information to identify anything", e1)
		return rep
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
