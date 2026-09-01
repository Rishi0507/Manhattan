// Package guards contains the checks that exist because of one specific
// failure, and it is the failure nobody demos.
//
// Suppose narrowing is slightly too aggressive and drops a record that
// genuinely belonged to the batch. The true witness is now unavailable. But
// the surviving pool happens to contain some other subset that sums to the
// target. The system then returns a unique witness, the accounting equation
// closes, and it posts a confidently wrong answer with a proof attached.
//
// That is strictly worse than an exception, because the audit trail lends it
// credibility. Everything in this package is aimed at that case.
package guards

import (
	"fmt"
	"math"
	"sort"

	"github.com/Rishi0507/manhattan/internal/model"
	"github.com/Rishi0507/manhattan/internal/money"
	"github.com/Rishi0507/manhattan/internal/narrow"
)

// Substitution is a rival witness expressed as an edit to the found one.
type Substitution struct {
	Removed []string    `json:"removed"`
	Added   []string    `json:"added"`
	Depth   int         `json:"depth"`
	Delta   money.Paise `json:"sum_delta_paise"`
}

// NeighbourhoodResult is the sensitivity finding for one settlement.
type NeighbourhoodResult struct {
	Method string `json:"method"`
	// Stable means no rival was constructible within the tested depth over
	// the widened pool.
	Stable bool `json:"stable"`

	MaxDepth          int                 `json:"max_substitution_depth"`
	RequestedDepth    int                 `json:"requested_substitution_depth"`
	RemovalSums       int                 `json:"removal_sums_enumerated"`
	AdditionSums      int                 `json:"addition_sums_enumerated"`
	WidenedPoolN      int                 `json:"widened_pool_n"`
	OriginalPoolN     int                 `json:"original_pool_n"`
	ConstraintsTested []narrow.Constraint `json:"constraints_tested"`

	// ExpectedSpurious is how many collisions this probe would be expected to
	// find purely by chance, given how many pairs it compared.
	//
	// The probe is a search for a coincidence, so it has a multiple
	// comparisons problem and it is a severe one. At depth two a witness of
	// size |S| generates about |S|^2/2 removal sums, and a widened pool of m
	// spare records generates about m^2/2 addition sums, so the number of
	// pairs compared grows with the square of both. A six-record witness in a
	// sixty-record pool compares about forty thousand pairs and a chance
	// collision is negligible. A one-hundred-and-thirty-eight-record witness
	// compares over a hundred million, and a chance collision is a near
	// certainty.
	//
	// A guard that fires on a coincidence it was always going to find is not
	// a guard, it is a false alarm generator, and it would hold every large
	// batch for review forever. So the probe estimates its own false positive
	// rate first and reduces its depth until that rate is small. If even
	// depth one is too noisy it says so and reports itself inconclusive,
	// which is a different and more honest statement than "stable".
	ExpectedSpurious float64 `json:"expected_spurious_collisions"`
	// Inconclusive means the probe could not distinguish a real substitution
	// from a chance one at any depth, so its silence carries no information.
	Inconclusive bool `json:"inconclusive"`

	// Rival, when present, is the alternative reconstruction the widened pool
	// admits, and Culprit is the constraint whose relaxation admitted it.
	Rival   *Substitution     `json:"rival,omitempty"`
	Culprit narrow.Constraint `json:"admitting_constraint,omitempty"`

	Note string `json:"note"`
}

// Probe searches the neighbourhood of a witness already in hand for a rival
// inside a deliberately widened pool.
//
// The formulation matters. For a verified witness S and a widened pool P',
// a rival has the form S' = (S \ A) union B with A a subset of S and B a
// subset of P' \ S. Since the sum of S already equals the target, S' hits
// the target exactly when the sum of B equals the sum of A. So the entire
// search is a join between the removal sums and the addition sums, and it
// costs microseconds rather than a second enumeration.
//
// The obvious alternative, re-running the solver over P' at some reduced
// cardinality bound, does not work, and the reason is worth stating because
// it is easy to get wrong. Substitution depth and free cardinality are
// different quantities. A rival differing from S by a single record still has
// free cardinality min(|S'|, |P'| - |S'|), which for a six-record witness in
// a fifty-six-record pool is six, not one. A probe restricted to a small
// cardinality bound never reaches it, so the guard would be silently unable
// to fire on precisely the case it exists for. Indexing by depth directly
// makes the probe cardinality-agnostic: it finds a one-record substitution in
// a six-record witness and in a three-hundred-record witness at the same cost.
//
// Scope, stated exactly: this covers every substitution of depth at most
// maxDepth. A rival sharing fewer than |S| - maxDepth records with S is not
// covered. That is a different failure from the one this guard exists for,
// since narrowing dropping one true record and a coincidental record taking
// its place is depth one by construction, but the boundary is real.
func Probe(witness []model.Record, widened []model.Record, maxDepth int, delta money.Paise, tested []narrow.Constraint, admitted map[string]narrow.Constraint) NeighbourhoodResult {
	res := NeighbourhoodResult{
		Method:            "witness_neighbourhood_join",
		RequestedDepth:    maxDepth,
		WidenedPoolN:      len(widened),
		ConstraintsTested: tested,
		Stable:            true,
	}

	inWitness := make(map[string]bool, len(witness))
	for _, r := range witness {
		inWitness[r.ID] = true
	}
	var additions []model.Record
	for _, r := range widened {
		if !inWitness[r.ID] {
			additions = append(additions, r)
		}
	}
	res.OriginalPoolN = len(widened) - len(additions) + len(witness)

	// Pick the deepest substitution the data can actually support without the
	// answer being drowned in chance collisions.
	depth, expected := usableDepth(witness, additions, maxDepth, delta)
	res.MaxDepth = depth
	res.ExpectedSpurious = expected
	if depth < 1 {
		res.Inconclusive = true
		res.Stable = false
		res.Note = fmt.Sprintf(
			"this probe cannot distinguish a real substitution from a chance one: comparing a %d-record "+
				"witness against %d spare records would produce an estimated %.1f coincidental collisions "+
				"even if narrowing were perfect, so its result would carry no information",
			len(witness), len(additions), expected)
		return res
	}

	removals := subsetSums(witness, depth)
	adds := subsetSums(additions, depth)
	res.RemovalSums = len(removals)
	res.AdditionSums = len(adds)

	// Sorted once, then range-queried, which is the same primitive the main
	// solver uses and behaves identically under an inferred-mode tolerance.
	sort.Slice(adds, func(i, j int) bool { return adds[i].sum < adds[j].sum })
	addSums := make([]money.Paise, len(adds))
	for i, a := range adds {
		addSums[i] = a.sum
	}

	maxDepth = depth
	for _, rem := range removals {
		width := money.Paise(rem.card) * delta
		for ai := lowerBound(addSums, rem.sum-width); ai < len(adds); ai++ {
			a := adds[ai]
			w := money.Paise(rem.card+a.card) * delta
			if a.sum > rem.sum+w {
				break
			}
			if rem.card == 0 && a.card == 0 {
				continue // the witness itself
			}
			if a.sum < rem.sum-w {
				continue
			}
			res.Stable = false
			res.Rival = &Substitution{
				Removed: rem.ids,
				Added:   a.ids,
				Depth:   maxInt(rem.card, a.card),
				Delta:   a.sum - rem.sum,
			}
			for _, id := range a.ids {
				if c, ok := admitted[id]; ok {
					res.Culprit = c
					break
				}
			}
			res.Note = fmt.Sprintf(
				"the widened pool admits an alternative reconstruction at substitution depth %d; "+
					"this answer came from a filtering decision rather than from the arithmetic. "+
					"An estimated %.3g collisions would be expected here by chance, so this one is a finding",
				res.Rival.Depth, expected)
			return res
		}
	}

	res.Note = fmt.Sprintf(
		"no alternative reconstruction exists at substitution depth %d over a pool widened to %d records "+
			"(an estimated %.3g chance collisions would have been tolerable here)",
		depth, len(widened), expected)
	return res
}

// usableDepth returns the deepest substitution depth at which a collision
// would still be surprising, together with the number of chance collisions
// expected at that depth.
//
// The estimate is the same collision-index reasoning the feasibility gate
// uses, applied to the probe's own search: the number of pairs compared,
// times the density of the difference distribution near zero. Differences of
// up to d records either side have a standard deviation of about sigma times
// the square root of 2d, so the density at zero is about
// 1/(sigma * sqrt(2*pi*2*d)).
func usableDepth(witness, additions []model.Record, maxDepth int, delta money.Paise) (int, float64) {
	sigma := spread(append(append([]model.Record{}, witness...), additions...))
	if sigma <= 0 {
		// Every record carries the same contribution, so a substitution is
		// always available and the probe can say nothing useful.
		return 0, maxReportable
	}
	const tolerable = 0.05

	for d := maxDepth; d >= 1; d-- {
		rem := float64(countSubsets(len(witness), d))
		add := float64(countSubsets(len(additions), d))
		density := 1.0 / (sigma * math.Sqrt(2*math.Pi*2*float64(d)))
		// An inferred-mode tolerance widens the accepted window and therefore
		// the chance of a spurious hit, proportionally.
		window := 1.0 + 2*float64(delta)*float64(d)
		expected := rem * add * density * window
		if expected <= tolerable {
			return d, expected
		}
		if d == 1 {
			return 0, expected
		}
	}
	return 0, maxReportable
}

// maxReportable keeps the estimate finite so a receipt can always be written.
const maxReportable = 1e18

// countSubsets is the number of subsets of size at most d.
func countSubsets(n, d int) int64 {
	total := int64(1)
	term := int64(1)
	for i := 1; i <= d && i <= n; i++ {
		term = term * int64(n-i+1) / int64(i)
		total += term
	}
	return total
}

// spread is the population standard deviation of the contributions.
func spread(rs []model.Record) float64 {
	if len(rs) < 2 {
		return 0
	}
	var mean float64
	for _, r := range rs {
		mean += float64(r.Contribution)
	}
	mean /= float64(len(rs))
	var ss float64
	for _, r := range rs {
		d := float64(r.Contribution) - mean
		ss += d * d
	}
	return math.Sqrt(ss / float64(len(rs)))
}

type sumEntry struct {
	sum  money.Paise
	card int
	ids  []string
}

// subsetSums enumerates every subset of rs up to the given size. At depth 2
// this is 1 + n + n(n-1)/2 entries, which for the pools this system operates
// on is a few thousand at most.
func subsetSums(rs []model.Record, maxDepth int) []sumEntry {
	out := []sumEntry{{sum: 0, card: 0, ids: nil}}
	if maxDepth >= 1 {
		for _, r := range rs {
			out = append(out, sumEntry{sum: r.Contribution, card: 1, ids: []string{r.ID}})
		}
	}
	if maxDepth >= 2 {
		for i := 0; i < len(rs); i++ {
			for j := i + 1; j < len(rs); j++ {
				out = append(out, sumEntry{
					sum:  rs[i].Contribution + rs[j].Contribution,
					card: 2,
					ids:  []string{rs[i].ID, rs[j].ID},
				})
			}
		}
	}
	if maxDepth >= 3 {
		for i := 0; i < len(rs); i++ {
			for j := i + 1; j < len(rs); j++ {
				for k := j + 1; k < len(rs); k++ {
					out = append(out, sumEntry{
						sum:  rs[i].Contribution + rs[j].Contribution + rs[k].Contribution,
						card: 3,
						ids:  []string{rs[i].ID, rs[j].ID, rs[k].ID},
					})
				}
			}
		}
	}
	return out
}

func lowerBound(a []money.Paise, v money.Paise) int {
	lo, hi := 0, len(a)
	for lo < hi {
		mid := int(uint(lo+hi) >> 1)
		if a[mid] < v {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	return lo
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
