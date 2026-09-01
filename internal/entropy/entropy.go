// Package entropy inspects the distribution of contribution amounts before
// any search is attempted.
//
// Two structural properties make uniqueness either impossible or make the
// feasibility estimate misleading, and both are detectable in O(n log n),
// which is to say in less time than it takes to allocate the enumeration
// they would have wasted.
//
// The gate exists for one concrete case above all: a subscription merchant
// with two hundred identical 499-rupee charges. There the feasibility
// model's assumption of a locally smooth sum distribution is not slightly
// optimistic, it is simply the wrong model. Those settlements are genuinely
// not reconstructable from amounts alone, and saying so in eleven
// milliseconds is more useful to a finance team than saying so slowly.
package entropy

import (
	"sort"

	"github.com/Rishi0507/manhattan/internal/money"
)

// TwinClass is a set of pool positions whose contributions are
// indistinguishable at the working tolerance.
type TwinClass struct {
	Value   money.Paise `json:"value_paise"`
	Members []int       `json:"members"`
}

// Report is what the gate produces. It is attached to every receipt,
// including the ones where it passed, because "we checked and the amounts do
// distinguish these transactions" is itself a finding.
type Report struct {
	DistinctValues int         `json:"distinct_contribution_values"`
	TwinClasses    []TwinClass `json:"twin_classes,omitempty"`
	TwinClassCount int         `json:"twin_class_count"`
	// TwinMass is the fraction of the pool sitting inside some twin class.
	// It is the headline number: high twin mass means amounts do not
	// distinguish these transactions from one another, whatever else is true.
	TwinMass          float64 `json:"twin_mass"`
	TwinMassThreshold float64 `json:"twin_mass_threshold"`

	// LatticeGCD is the greatest common divisor of all contributions. Sums
	// then live on a lattice of that spacing, so achievable values near any
	// target are this many times denser than a continuous model assumes.
	// A batch of round-rupee tickets under a rupee-rounding policy has
	// gcd 100, and ignoring it understates the collision index a hundredfold.
	LatticeGCD int64 `json:"lattice_gcd_paise"`

	// ZeroContribution lists pool positions whose signed net effect is zero.
	// Narrowing normally removes these before the gate ever sees them, and
	// this check is the backstop: if any survive, uniqueness is impossible by
	// construction rather than merely unlikely, because such a record can be
	// added to or removed from any witness without changing its sum.
	ZeroContribution []int `json:"zero_contribution_members,omitempty"`

	Pass bool   `json:"pass"`
	Note string `json:"note,omitempty"`
}

// Config carries the tunable thresholds.
type Config struct {
	// TwinMassThreshold is the fraction above which the pool is refused
	// outright. Default 0.30.
	TwinMassThreshold float64
	// Delta is the per-item rounding allowance. In inferred mode, values
	// within delta of one another are clustered rather than compared for
	// exact equality, because a one-paise difference does not distinguish
	// two transactions when the tolerance is one paise.
	Delta money.Paise
}

// DefaultConfig returns the shipped thresholds.
func DefaultConfig() Config { return Config{TwinMassThreshold: 0.30, Delta: 0} }

// Analyse partitions the pool by contribution value and measures the two
// structural properties.
func Analyse(contribs []money.Paise, cfg Config) Report {
	n := len(contribs)
	rep := Report{TwinMassThreshold: cfg.TwinMassThreshold, LatticeGCD: 1, Pass: true}
	if n == 0 {
		rep.Pass = false
		rep.Note = "empty pool"
		return rep
	}

	rep.LatticeGCD = money.GCD(contribs)

	// Cluster by value. With a zero tolerance this is exact equality; with a
	// non-zero one it is a single-linkage pass over the sorted values, which
	// is the right notion of "indistinguishable" when the amounts themselves
	// are only known to within delta.
	type pv struct {
		v money.Paise
		i int
	}
	sorted := make([]pv, n)
	for i, v := range contribs {
		sorted[i] = pv{v, i}
	}
	sort.Slice(sorted, func(a, b int) bool {
		if sorted[a].v != sorted[b].v {
			return sorted[a].v < sorted[b].v
		}
		return sorted[a].i < sorted[b].i
	})

	var classes []TwinClass
	distinct := 0
	start := 0
	for i := 1; i <= n; i++ {
		split := i == n
		if !split {
			split = sorted[i].v-sorted[i-1].v > cfg.Delta
		}
		if !split {
			continue
		}
		distinct++
		if size := i - start; size >= 2 {
			members := make([]int, 0, size)
			for j := start; j < i; j++ {
				members = append(members, sorted[j].i)
			}
			sort.Ints(members)
			classes = append(classes, TwinClass{Value: sorted[start].v, Members: members})
		}
		start = i
	}

	for i, v := range contribs {
		if v.Abs() <= cfg.Delta {
			rep.ZeroContribution = append(rep.ZeroContribution, i)
		}
	}

	inTwins := 0
	for _, c := range classes {
		inTwins += len(c.Members)
	}
	rep.DistinctValues = distinct
	rep.TwinClasses = classes
	rep.TwinClassCount = len(classes)
	rep.TwinMass = float64(inTwins) / float64(n)

	if rep.TwinMass > cfg.TwinMassThreshold {
		rep.Pass = false
		rep.Note = "amounts do not distinguish transactions in this pool"
	}
	return rep
}

// ZeroRival returns a witness that differs only by a zero-contribution
// record, if one is available.
//
// Like a twin swap, this proves ambiguity by construction rather than by
// search: adding or removing a record that contributes nothing leaves the
// sum untouched, so both sets reconstruct the credit exactly and no
// arithmetic can prefer one.
func ZeroRival(witness []int, rep Report) (rival []int, changed int, ok bool) {
	if len(rep.ZeroContribution) == 0 {
		return nil, 0, false
	}
	inWitness := make(map[int]bool, len(witness))
	for _, i := range witness {
		inWitness[i] = true
	}
	for _, z := range rep.ZeroContribution {
		out := make([]int, 0, len(witness)+1)
		if inWitness[z] {
			for _, i := range witness {
				if i != z {
					out = append(out, i)
				}
			}
		} else {
			out = append(out, witness...)
			out = append(out, z)
		}
		sort.Ints(out)
		return out, z, true
	}
	return nil, 0, false
}

// SwapRival looks for an alternative witness constructible purely from twin
// structure, and returns it if one exists.
//
// If a witness S contains a non-empty proper subset of a twin class C, then
// exchanging any member of S that lies in C for any member of C that does
// not lie in S produces a distinct subset with an identical sum. Ambiguity is
// then proved by construction, in linear time, with no search whatsoever,
// and the rival is exhibited rather than merely asserted.
//
// This is why VERIFIED and a constructible twin swap are mutually
// exclusive states rather than a status plus a warning flag: the rival is
// not suspected, it is in hand.
func SwapRival(witness []int, rep Report) (rival []int, from, to int, ok bool) {
	inWitness := make(map[int]bool, len(witness))
	for _, i := range witness {
		inWitness[i] = true
	}
	for _, c := range rep.TwinClasses {
		var inS, outS []int
		for _, m := range c.Members {
			if inWitness[m] {
				inS = append(inS, m)
			} else {
				outS = append(outS, m)
			}
		}
		if len(inS) == 0 || len(outS) == 0 {
			continue
		}
		from, to = inS[0], outS[0]
		rival = make([]int, 0, len(witness))
		for _, i := range witness {
			if i == from {
				rival = append(rival, to)
				continue
			}
			rival = append(rival, i)
		}
		sort.Ints(rival)
		return rival, from, to, true
	}
	return nil, 0, 0, false
}
