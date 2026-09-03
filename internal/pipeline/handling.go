package pipeline

import (
	"fmt"

	"github.com/Rishi0507/manhattan/internal/evidence"
)

// HandlingModel prices a held settlement by what clearing it takes.
//
// The first version charged a flat twenty minutes for every exception. That is
// defensible as an average and useless as a queue: every row costs the same,
// so sorting by cost sorts by nothing, and the claim that the backlog can be
// worked in the order that clears the most money per hour is not demonstrated
// by a table where every row is identical.
//
// The inputs were already on the receipt. What separates a cheap exception
// from an expensive one is not its status alone but how much of the work has
// already been done:
//
//   - a remediation that NAMES the change is a decision, not an investigation
//   - a remediation whose effect was RE-VERIFIED is barely even a decision
//   - two exhibited rivals is a choice between two things somebody can see
//   - nothing reconstructing the credit, with no remedy, is open-ended, and
//     open-ended work is where analyst time actually goes
//
// Every term is configured, and every term that fired is named on the receipt,
// so an operations lead who thinks thirty-five minutes is wrong for a
// narrowing-sensitive settlement can argue with the model rather than with the
// output.
type HandlingModel struct {
	// BaseMinutes is the starting estimate per status.
	BaseMinutes map[evidence.Status]int
	// RemedyDiscountPct applies when a computed remediation names the change.
	RemedyDiscountPct int
	// CureDiscountPct applies when that remediation carries a projected
	// collision index, which means its effect was computed rather than
	// suggested.
	CureDiscountPct int
	// NoRemedyPenaltyPct applies when the receipt offers nothing to act on.
	NoRemedyPenaltyPct int
	// PerRivalMinutes charges for each exhibited alternative past the first,
	// because adjudicating between five candidate batches is not the same
	// task as adjudicating between two.
	PerRivalMinutes int
	MaxRivalMinutes int
	// PerPoolBlockMinutes charges for each PoolBlock candidates in the pool,
	// which is the cost of reading the thing at all.
	PerPoolBlockMinutes int
	PoolBlock           int
	// Floor and Ceiling bound the estimate, because a model this simple should
	// not be trusted at its extremes.
	Floor   int
	Ceiling int
}

// DefaultHandling is the shipped model.
//
// The base figures are ordered by how open-ended the work is rather than by
// how severe the status sounds. UNDERDETERMINED is the cheapest because there
// is usually nothing to adjudicate: the amounts do not distinguish the
// transactions, the receipt says which change would fix that, and the action
// is a data or configuration decision somebody makes once for the merchant.
// UNRESOLVED is the most expensive because nothing reconstructs the credit and
// somebody has to go and find out why.
func DefaultHandling() HandlingModel {
	return HandlingModel{
		BaseMinutes: map[evidence.Status]int{
			evidence.StatusUnderdetermined:    12,
			evidence.StatusAmbiguous:          25,
			evidence.StatusNarrowingSensitive: 35,
			evidence.StatusUnresolved:         45,
		},
		RemedyDiscountPct:   30,
		CureDiscountPct:     55,
		NoRemedyPenaltyPct:  25,
		PerRivalMinutes:     4,
		MaxRivalMinutes:     20,
		PerPoolBlockMinutes: 1,
		PoolBlock:           25,
		Floor:               5,
		Ceiling:             120,
	}
}

// Minutes estimates the handling time for one held settlement, and returns the
// terms that produced it.
func (h HandlingModel) Minutes(r *evidence.Receipt) (int, []string) {
	base, ok := h.BaseMinutes[r.Status]
	if !ok {
		base = 20
	}
	mins := float64(base)
	basis := []string{fmt.Sprintf("%s baseline %d min", r.Status, base)}

	// What the receipt already worked out.
	var cured, named bool
	for _, rem := range r.Remediation {
		named = true
		if rem.ProjectedIndex != nil || rem.ProjectedPoolN != nil {
			cured = true
		}
	}
	switch {
	case cured:
		mins *= 1 - float64(h.CureDiscountPct)/100
		basis = append(basis, fmt.Sprintf("remedy re-verified, less %d%%", h.CureDiscountPct))
	case named:
		mins *= 1 - float64(h.RemedyDiscountPct)/100
		basis = append(basis, fmt.Sprintf("remedy computed, less %d%%", h.RemedyDiscountPct))
	default:
		mins *= 1 + float64(h.NoRemedyPenaltyPct)/100
		basis = append(basis, fmt.Sprintf("nothing to act on, plus %d%%", h.NoRemedyPenaltyPct))
	}

	// What is left to adjudicate. A refusal that never reached the search has
	// no uniqueness block at all, which is most of the UNDERDETERMINED
	// population and part of why they are the cheapest rows in the queue.
	if r.Uniqueness == nil {
		basis = append(basis, "refused before the search, so nothing to adjudicate")
	} else if n := len(r.Uniqueness.AlternativeWitnesses); n > 1 {
		add := (n - 1) * h.PerRivalMinutes
		if add > h.MaxRivalMinutes {
			add = h.MaxRivalMinutes
		}
		mins += float64(add)
		basis = append(basis, fmt.Sprintf("%d rivals to choose between, plus %d min", n, add))
	}

	// What has to be read.
	if h.PoolBlock > 0 && r.Pool.N > h.PoolBlock {
		add := (r.Pool.N - h.PoolBlock) / h.PoolBlock * h.PerPoolBlockMinutes
		if add > 0 {
			mins += float64(add)
			basis = append(basis, fmt.Sprintf("%d candidates to read, plus %d min", r.Pool.N, add))
		}
	}

	out := int(mins + 0.5)
	if out < h.Floor {
		out = h.Floor
	}
	if out > h.Ceiling {
		out = h.Ceiling
	}
	return out, basis
}
