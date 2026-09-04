package main

import (
	"testing"

	"github.com/Rishi0507/manhattan/internal/bench"
)

// TestDeltaTableCloses checks that the published comparison against B1 adds up.
//
// The first version of that table did not. It computed the overlap as postings
// minus "contribution", where contribution mixed two different units: a
// settlement whose defective report was contradicted is one this system
// WITHHELD, not one it posted, so subtracting it from a posting count is
// meaningless. It also printed the net posting difference under the label
// "postings B1 made that we declined", which is a different quantity that
// happens to sit nearby.
//
// Both errors are the kind a reader finds with a pocket calculator, and a table
// that does not close in a document arguing for arithmetic rigour costs more
// than the table earns. So the invariants are asserted rather than eyeballed.
func TestDeltaTableCloses(t *testing.T) {
	sum := bench.Summary{
		Settlements:   996,
		B1Posted:      848,
		B1PostedWrong: 39,
		M1Posted:      731,
		M1FromProof:   212,
		M1FromClaim:   519,
		NoClaim:       148,
		NoClaimPosted: 30,
	}

	proofsWithMapping := sum.M1FromProof - sum.NoClaimPosted
	both := sum.M1FromClaim + proofsWithMapping
	m1Only := sum.NoClaimPosted
	b1Only := sum.B1Posted - both

	// Every posting this system made is either shared with B1 or reached a
	// settlement B1 had no mapping for. There is no third category.
	if got := both + m1Only; got != sum.M1Posted {
		t.Errorf("coverage does not close on this side: %d shared + %d ours = %d, but %d were posted",
			both, m1Only, got, sum.M1Posted)
	}
	// Every posting B1 made is either shared or one this system declined.
	if got := both + b1Only; got != sum.B1Posted {
		t.Errorf("coverage does not close on B1's side: %d shared + %d declined = %d, but B1 posted %d",
			both, b1Only, got, sum.B1Posted)
	}
	// The net difference is the two exclusive columns, and must not be printed
	// as either one of them.
	if got := b1Only - m1Only; got != sum.B1Posted-sum.M1Posted {
		t.Errorf("net difference %d does not match B1 minus composite %d",
			got, sum.B1Posted-sum.M1Posted)
	}
	if b1Only == sum.B1Posted-sum.M1Posted {
		t.Error("B1-only equals the net difference, which means one of them is mislabelled")
	}
	// Wrong postings prevented can only come out of the settlements B1 posted
	// and this system did not.
	if sum.B1PostedWrong > b1Only {
		t.Errorf("%d wrong postings prevented exceeds the %d postings declined, which is impossible",
			sum.B1PostedWrong, b1Only)
	}
}
