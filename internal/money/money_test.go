package money

import (
	"math/rand"
	"testing"
)

// The money package had no direct tests, which was the least defensible gap in
// the repository: it is the only numeric type in the verification path, and
// three of its five rounding modes had never been executed by a test.

func TestRoundingModes(t *testing.T) {
	// 12345 paise at 250 bps is 308.625 paise exactly, which lands on a
	// half only for the tie cases below; the fractional cases pin direction.
	cases := []struct {
		amount Paise
		bps    int64
		mode   RoundMode
		want   Paise
	}{
		// Exact ties, which is where the modes actually differ.
		{200, 250, RoundHalfUp, 5},  // 5.0 exactly
		{100, 50, RoundHalfUp, 1},   // 0.5 -> 1
		{100, 50, RoundHalfEven, 0}, // 0.5 -> 0, nearest even
		{300, 50, RoundHalfEven, 2}, // 1.5 -> 2, nearest even
		{500, 50, RoundHalfEven, 2}, // 2.5 -> 2, nearest even
		{700, 50, RoundHalfEven, 4}, // 3.5 -> 4, nearest even
		{100, 50, RoundFloor, 0},    // 0.5 -> 0
		{100, 50, RoundCeil, 1},     // 0.5 -> 1
		{100, 50, RoundTrunc, 0},    // 0.5 -> 0
		// Non-ties behave identically across half-up and half-even.
		{1000, 251, RoundHalfUp, 25}, // 25.1 -> 25
		{1000, 251, RoundHalfEven, 25},
		{1000, 259, RoundHalfUp, 26}, // 25.9 -> 26
		{1000, 259, RoundHalfEven, 26},

		// Negative amounts. A refund or a chargeback runs through the same
		// path, and rounding that is asymmetric about zero would put a
		// systematic bias into every signed contribution.
		{-100, 50, RoundHalfUp, -1},   // -0.5 away from zero
		{-100, 50, RoundTrunc, 0},     // toward zero
		{-100, 50, RoundFloor, -1},    // toward negative infinity
		{-100, 50, RoundCeil, 0},      // toward positive infinity
		{-300, 50, RoundHalfEven, -2}, // -1.5 -> -2
		{-500, 50, RoundHalfEven, -2}, // -2.5 -> -2
	}

	for _, c := range cases {
		if got := MulRateBPS(c.amount, c.bps, c.mode); got != c.want {
			t.Errorf("MulRateBPS(%d, %d bps, %s) = %d, want %d",
				c.amount, c.bps, c.mode, got, c.want)
		}
	}
}

// TestRoundingIsWithinOnePaise is the property that matters more than any
// individual case: whatever the mode, the answer is never more than a paise
// from the exact value.
func TestRoundingIsWithinOnePaise(t *testing.T) {
	rng := rand.New(rand.NewSource(11))
	modes := []RoundMode{RoundHalfUp, RoundHalfEven, RoundFloor, RoundCeil, RoundTrunc}

	for i := 0; i < 20000; i++ {
		amt := Paise(rng.Int63n(200_000_000) - 100_000_000)
		bps := rng.Int63n(2000)
		for _, m := range modes {
			got := MulRateBPS(amt, bps, m)
			// exact = amt*bps/10000, compared without leaving integers.
			num := int64(amt) * bps
			lo := num / 10000
			if num%10000 != 0 && num < 0 {
				lo--
			}
			hi := lo
			if num%10000 != 0 {
				hi = lo + 1
			}
			if int64(got) < lo || int64(got) > hi {
				t.Fatalf("MulRateBPS(%d, %d, %s) = %d, outside the exact bracket [%d, %d]",
					amt, bps, m, got, lo, hi)
			}
		}
	}
}

func TestIndianGrouping(t *testing.T) {
	cases := []struct {
		paise Paise
		want  string
	}{
		{0, "₹0.00"},
		{5, "₹0.05"},
		{99, "₹0.99"},
		{100, "₹1.00"},
		{99900, "₹999.00"},
		{100000, "₹1,000.00"},
		{999900, "₹9,999.00"},
		{1000000, "₹10,000.00"},
		{9999900, "₹99,999.00"},
		{10000000, "₹1,00,000.00"},
		{48638155, "₹4,86,381.55"},
		{100000000, "₹10,00,000.00"},
		{1000000000, "₹1,00,00,000.00"},
		{-48638155, "-₹4,86,381.55"},
		{-5, "-₹0.05"},
	}
	for _, c := range cases {
		if got := c.paise.String(); got != c.want {
			t.Errorf("Paise(%d).String() = %q, want %q", c.paise, got, c.want)
		}
	}
}

func TestParseRupeesRoundTrips(t *testing.T) {
	rng := rand.New(rand.NewSource(3))
	for i := 0; i < 5000; i++ {
		want := Paise(rng.Int63n(2_000_000_000) - 1_000_000_000)
		got, err := ParseRupees(want.String())
		if err != nil {
			t.Fatalf("ParseRupees(%q): %v", want.String(), err)
		}
		if got != want {
			t.Fatalf("round trip: %d -> %q -> %d", want, want.String(), got)
		}
	}
}

func TestParseRupeesRefusesSubPaise(t *testing.T) {
	// Silently discarding precision is exactly the class of quiet loss this
	// package exists to prevent, so it is an error rather than a truncation.
	if _, err := ParseRupees("100.005"); err == nil {
		t.Fatal("ParseRupees accepted sub-paise precision instead of refusing it")
	}
	for _, s := range []string{"", "abc", "1.2.3"} {
		if _, err := ParseRupees(s); err == nil {
			t.Errorf("ParseRupees(%q) should have failed", s)
		}
	}
}

func TestBPS(t *testing.T) {
	cases := []struct {
		num, den Paise
		want     int64
	}{
		{200, 10000, 200}, // 2.00%
		{0, 10000, 0},
		{10000, 10000, 10000}, // 100%
		{1, 10000, 1},         // rounds to 1 bps
		{-200, 10000, -200},   // a credit note, not an overflow
		{200, -10000, -200},   // sign carried by the denominator
		{-200, -10000, 200},
		{100, 0, 0}, // no panic on a zero base
	}
	for _, c := range cases {
		if got := BPS(c.num, c.den); got != c.want {
			t.Errorf("BPS(%d, %d) = %d, want %d", c.num, c.den, got, c.want)
		}
	}
}

func TestGCDDrivesTheLatticeCorrection(t *testing.T) {
	cases := []struct {
		in   []Paise
		want int64
	}{
		{[]Paise{100, 200, 300}, 100}, // round rupees
		{[]Paise{101, 200, 300}, 1},
		{[]Paise{0, 0, 0}, 1},       // no information, not a division by zero
		{[]Paise{0, 500, 250}, 250}, // zeros ignored
		{[]Paise{-100, 200}, 100},   // sign irrelevant to the lattice
		{[]Paise{}, 1},
		{[]Paise{7}, 7},
	}
	for _, c := range cases {
		if got := GCD(c.in); got != c.want {
			t.Errorf("GCD(%v) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestSumAndSigns(t *testing.T) {
	xs := []Paise{100, -50, 25, -75}
	if got := Sum(xs); got != 0 {
		t.Errorf("Sum = %d, want 0", got)
	}
	if !Paise(-1).Negative() || Paise(0).Negative() || Paise(1).Negative() {
		t.Error("Negative is wrong about the sign of a contribution")
	}
	if Paise(-5).Abs() != 5 || Paise(5).Abs() != 5 {
		t.Error("Abs is wrong")
	}
}
