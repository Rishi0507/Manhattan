// Package money defines the only numeric type this system uses for value.
//
// Every amount in Manhattan is an integer count of paise (1/100 of a rupee),
// held in an int64. There is no float64 anywhere in the verification path.
// This is not fastidiousness: exact arithmetic is what makes exact
// verification possible, and Razorpay reports settlement amounts in the
// smallest currency unit already, so nothing is lost by staying there.
package money

import (
	"fmt"
	"strconv"
	"strings"
)

// Paise is a signed amount in the smallest currency unit.
//
// It is signed deliberately. A chargeback debit or a fully-refunded payment
// whose MDR was retained contributes a negative amount to a settlement, and
// modelling that as an unsigned magnitude plus a direction flag is how
// reconcilers end up with sign bugs in production.
type Paise int64

// Zero is the additive identity.
const Zero Paise = 0

// FromRupees converts whole rupees to paise.
func FromRupees(r int64) Paise { return Paise(r * 100) }

// Add, Sub and Neg exist so call sites read as accounting rather than as
// integer manipulation. They are free at runtime.
func (p Paise) Add(q Paise) Paise { return p + q }
func (p Paise) Sub(q Paise) Paise { return p - q }
func (p Paise) Neg() Paise        { return -p }

// Abs returns the magnitude.
func (p Paise) Abs() Paise {
	if p < 0 {
		return -p
	}
	return p
}

// Negative reports whether this is a debit-shaped contribution.
func (p Paise) Negative() bool { return p < 0 }

// Sum totals a slice exactly. Overflow is not a practical concern: int64
// holds ~9.2e18 paise, which is ~9.2e16 rupees, or roughly a hundred
// thousand times India's annual GDP.
func Sum(xs []Paise) Paise {
	var t Paise
	for _, x := range xs {
		t += x
	}
	return t
}

// String renders in Indian digit grouping with an explicit sign, e.g.
// "₹4,86,381.55" or "-₹1,240.00". This is the format finance teams read,
// and getting it wrong on a receipt undermines everything else on it.
func (p Paise) String() string { return p.Format(true) }

// Format renders the amount, optionally with the rupee symbol.
func (p Paise) Format(symbol bool) string {
	neg := p < 0
	v := int64(p)
	if neg {
		v = -v
	}
	whole := v / 100
	frac := v % 100

	var b strings.Builder
	if neg {
		b.WriteByte('-')
	}
	if symbol {
		b.WriteString("\u20b9")
	}
	b.WriteString(groupIndian(whole))
	b.WriteByte('.')
	if frac < 10 {
		b.WriteByte('0')
	}
	b.WriteString(strconv.FormatInt(frac, 10))
	return b.String()
}

// groupIndian applies the 3-2-2 grouping used on the subcontinent:
// the last three digits, then pairs. 48638155 paise -> "4,86,381".
func groupIndian(n int64) string {
	s := strconv.FormatInt(n, 10)
	if len(s) <= 3 {
		return s
	}
	head, tail := s[:len(s)-3], s[len(s)-3:]
	var parts []string
	for len(head) > 2 {
		parts = append([]string{head[len(head)-2:]}, parts...)
		head = head[:len(head)-2]
	}
	if head != "" {
		parts = append([]string{head}, parts...)
	}
	return strings.Join(parts, ",") + "," + tail
}

// BPS returns numerator/denominator expressed in basis points, rounded
// half-away-from-zero, computed entirely in integers. Used by the fee
// detector, where a float would introduce exactly the kind of drift the
// detector exists to measure.
func BPS(num, den Paise) int64 {
	if den == 0 {
		return 0
	}
	n, d := int64(num)*10000, int64(den)
	if (n < 0) != (d < 0) {
		return -((-n + d/2) / d)
	}
	return (n + d/2) / d
}

// MulRateBPS multiplies an amount by a rate in basis points using the
// declared rounding convention. Every fee in the system flows through here,
// so the convention is applied in exactly one place.
func MulRateBPS(amount Paise, bps int64, mode RoundMode) Paise {
	num := int64(amount) * bps
	return Paise(divRound(num, 10000, mode))
}

// RoundMode names a rounding convention. Which one a payment gateway
// actually uses is a contractual fact, not a mathematical one, which is why
// it is configuration rather than a constant.
type RoundMode string

const (
	RoundHalfUp   RoundMode = "half_up"   // ties away from zero
	RoundHalfEven RoundMode = "half_even" // banker's rounding
	RoundFloor    RoundMode = "floor"     // toward negative infinity
	RoundCeil     RoundMode = "ceil"      // toward positive infinity
	RoundTrunc    RoundMode = "trunc"     // toward zero
)

func divRound(num, den int64, mode RoundMode) int64 {
	if den == 0 {
		panic("money: division by zero")
	}
	q := num / den
	r := num - q*den
	if r == 0 {
		return q
	}
	sameSign := (num < 0) == (den < 0)
	twice := r * 2
	if twice < 0 {
		twice = -twice
	}
	absDen := den
	if absDen < 0 {
		absDen = -absDen
	}
	switch mode {
	case RoundFloor:
		if !sameSign {
			return q - 1
		}
		return q
	case RoundCeil:
		if sameSign {
			return q + 1
		}
		return q
	case RoundTrunc:
		return q
	case RoundHalfEven:
		if twice > absDen || (twice == absDen && q%2 != 0) {
			if sameSign {
				return q + 1
			}
			return q - 1
		}
		return q
	default: // RoundHalfUp
		if twice >= absDen {
			if sameSign {
				return q + 1
			}
			return q - 1
		}
		return q
	}
}

// GCD of a set of amounts, ignoring zeros, taken over magnitudes. This is
// the lattice spacing d of the feasibility model: if every contribution is
// divisible by d, achievable sums live on a lattice of that spacing and are
// d times denser near any target than a continuous model assumes.
func GCD(xs []Paise) int64 {
	var g int64
	for _, x := range xs {
		a := int64(x.Abs())
		if a == 0 {
			continue
		}
		g = gcd2(g, a)
		if g == 1 {
			return 1
		}
	}
	if g == 0 {
		return 1
	}
	return g
}

func gcd2(a, b int64) int64 {
	for b != 0 {
		a, b = b, a%b
	}
	if a < 0 {
		return -a
	}
	return a
}

// ParseRupees reads "4,86,381.55" or "486381.55" or "-1240" into paise.
// Used by the fixture loader, never on the hot path.
func ParseRupees(s string) (Paise, error) {
	s = strings.TrimSpace(strings.NewReplacer(",", "", "\u20b9", "", " ", "").Replace(s))
	if s == "" {
		return 0, fmt.Errorf("money: empty amount")
	}
	neg := false
	if s[0] == '-' {
		neg, s = true, s[1:]
	} else if s[0] == '+' {
		s = s[1:]
	}
	whole, frac := s, "0"
	if i := strings.IndexByte(s, '.'); i >= 0 {
		whole, frac = s[:i], s[i+1:]
	}
	if whole == "" {
		whole = "0"
	}
	for len(frac) < 2 {
		frac += "0"
	}
	if len(frac) > 2 {
		return 0, fmt.Errorf("money: %q has sub-paise precision, which this system refuses to silently discard", s)
	}
	w, err := strconv.ParseInt(whole, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("money: bad rupee part in %q: %w", s, err)
	}
	f, err := strconv.ParseInt(frac, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("money: bad paise part in %q: %w", s, err)
	}
	v := Paise(w*100 + f)
	if neg {
		v = -v
	}
	return v, nil
}
