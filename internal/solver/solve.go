// Package solver reconstructs a settlement from its underlying records and,
// in the same pass, counts how many other reconstructions exist.
//
// The problem is subset sum, which is NP-complete in the number of items, so
// everything depends on which escape hatch is taken. Manhattan takes the one
// its own feasibility analysis points at.
//
// The binding constraint in settlement reconciliation is not the value range
// , it is the cardinality. A 312-transaction batch drawn from a 315-item
// pool is trivially close to the whole pool; a 6-transaction batch from a
// 52-item pool is a small subset. Both have a small *free* cardinality
//
//	k(S) = min(|S|, n-|S|)
//
// and the feasibility gate establishes that uniqueness is only attainable at
// all when k is small, roughly 3 to 7 for realistic pools. A pseudopolynomial
// dynamic program over the value range is indifferent to k, so it spends its
// entire budget in a dimension the problem does not vary along and none in
// the dimension that decides everything.
//
// Cardinality-restricted meet-in-the-middle is dispatched on k directly. It
// buys four properties that matter more than asymptotics:
//
//  1. Uniqueness is the search, not a sweep bolted on afterwards. The probe
//     returns every match, so the count is a by-product and can never be
//     abandoned half-finished for want of budget.
//  2. Sign is irrelevant. Sums are integers and integers sort. Chargebacks
//     need no special case, no window derivation, no positivity certificate.
//  3. Cardinality is native, so the inferred-mode rounding band, the
//     declared-count cross-check and the complement's size all read straight
//     off the enumeration.
//  4. Witness and complement come from one enumeration, so both the
//     small-|S| and large-|S| sides are solved for about 1.1x the cost of
//     one, with no need to guess in advance which side is smaller.
package solver

import (
	"sort"

	"github.com/Rishi0507/manhattan/internal/money"
)

// ScopeSource records what bounded the search region, because two results
// both reading "unique" are not making the same statement if one of them
// borrowed its bound from the artifact it was meant to be validating.
type ScopeSource string

const (
	// ScopeGate: k_max came from the feasibility gate, computed from the pool
	// alone. The uniqueness claim is unconditional within the searched region.
	ScopeGate ScopeSource = "feasibility_gate"
	// ScopeDeclared: k_max came from the settlement report's own transaction
	// count. Cheaper, often the only route to a unique answer at all, and a
	// materially weaker claim. Receipts say which was used.
	ScopeDeclared ScopeSource = "declared_txn_count"
)

// Witness is one reconstruction: the pool indices whose contributions sum
// into the target band.
type Witness struct {
	Indices []int       `json:"indices"`
	Sum     money.Paise `json:"sum_paise"`
	// Slack is how much of the inferred-mode rounding allowance this witness
	// consumed. Zero in declared mode, and reported either way so the
	// assumption is never invisible.
	Slack money.Paise `json:"slack_paise"`
	// FromComplement marks a witness recovered by solving for the excluded
	// set rather than the included one.
	FromComplement bool `json:"from_complement"`
}

// Size is the witness cardinality.
func (w Witness) Size() int { return len(w.Indices) }

// Miss is the nearest achievable sum when nothing landed inside the band.
// It is tracked during the probe at no extra cost, and it is what turns an
// UNRESOLVED result from a shrug into a starting point: the resolution agent
// is handed an exact gap to explain rather than a bare failure.
type Miss struct {
	Sum         money.Paise `json:"nearest_sum_paise"`
	Gap         money.Paise `json:"gap_paise"`
	Cardinality int         `json:"cardinality"`
	Valid       bool        `json:"valid"`
}

// Result is everything one dispatch produces.
type Result struct {
	KMax        int         `json:"k_max"`
	ScopeSource ScopeSource `json:"scope_source"`
	Split       [2]int      `json:"split"`
	EntriesLeft int64       `json:"entries_left"`
	EntriesRght int64       `json:"entries_right"`
	MemoryBytes int64       `json:"memory_bytes"`

	// Matches is the exhaustive count of distinct subsets in the searched
	// region whose sum lands in the band, after canonicalisation. Rivals is
	// Matches-1 when anything was found.
	Matches int  `json:"matches_found"`
	Rivals  int  `json:"rivals_found"`
	Capped  bool `json:"count_saturated"`

	// Witnesses holds up to a small cap of the matches, canonicalised. When
	// two rivals exist, both are here, which is what lets an AMBIGUOUS
	// receipt exhibit its alternatives rather than merely assert them.
	Witnesses []Witness `json:"witnesses"`

	DedupApplied  bool `json:"dedup_applied"`
	DedupRemoved  int  `json:"dedup_removed"`
	ScopeComplete bool `json:"scope_complete"`

	Nearest Miss `json:"nearest_miss"`
}

// maxCollected caps how many full witnesses are materialised. Counting
// continues past it; only reconstruction of membership stops. Two is enough
// to prove ambiguity and eight is enough to show an analyst a pattern.
const maxCollected = 8

// countCap saturates the exhaustive count. The feasibility gate should
// prevent this from ever being reached in practice, and reaching it is
// reported rather than hidden.
const countCap = 1 << 20

// maxCardinality bounds the membership scratch buffers.
const maxCardinality = 48

// Solve reconstructs target T from the pool contributions and counts rivals.
//
// delta is the per-item rounding allowance in paise: zero in declared mode,
// typically one in inferred mode. The accepted band for a candidate pairing
// is (c_left + c_right) * delta, which is exact per cardinality. The naive
// alternative, a band of n*delta across the whole pool, is wrong, because
// only items inside the witness accumulate rounding drift, and on a
// 400-candidate pool it widens the target until essentially every value is
// reachable.
func Solve(contribs []money.Paise, target money.Paise, kMax int, delta money.Paise, scope ScopeSource) *Result {
	n := len(contribs)
	if kMax > n {
		kMax = n
	}
	if kMax < 0 {
		kMax = 0
	}
	// Membership scratch is fixed-size; a k this large is many orders of
	// magnitude past anything the feasibility gate would ever accept, so
	// clamping here is a safety rail rather than a policy.
	if kMax > maxCardinality {
		kMax = maxCardinality
	}

	half := n / 2
	left, right := contribs[:half], contribs[half:]

	L := Enumerate(left, 0, kMax)
	R := Enumerate(right, half, kMax)

	res := &Result{
		KMax:          kMax,
		ScopeSource:   scope,
		Split:         [2]int{len(left), len(right)},
		EntriesLeft:   L.Entries,
		EntriesRght:   R.Entries,
		ScopeComplete: true,
	}
	res.MemoryBytes = (L.Entries + R.Entries) * EntryBytes
	if mx := maxBucket(L, R); mx > 0 {
		res.MemoryBytes += mx * SortTempBytes
	}

	total := money.Sum(contribs)

	c := &collector{
		L: L, R: R, n: n, kMax: kMax, delta: int64(delta),
		seen: map[string]struct{}{},
		// Deduplication is only needed where the two probes can find the same
		// subset, which happens exactly when a subset can satisfy both
		// |S| <= kMax and n-|S| <= kMax. Outside that range the two probes
		// search disjoint cardinalities and a set lookup would be pure cost.
		dedupNeeded: n <= 2*kMax,
	}

	c.contribs = contribs
	c.probe(int64(target), false)
	if !c.capped {
		c.probe(int64(total-target), true)
	}

	res.Matches = c.count
	res.Capped = c.capped
	if res.Matches > 0 {
		res.Rivals = res.Matches - 1
	}
	res.DedupApplied = c.dedupNeeded
	res.DedupRemoved = c.dedupRemoved
	res.Witnesses = c.witnesses
	res.Nearest = c.nearest

	// Present witnesses in a stable order so receipts diff cleanly across
	// runs and a replay produces byte-identical output.
	sort.Slice(res.Witnesses, func(i, j int) bool {
		a, b := res.Witnesses[i].Indices, res.Witnesses[j].Indices
		for k := 0; k < len(a) && k < len(b); k++ {
			if a[k] != b[k] {
				return a[k] < b[k]
			}
		}
		return len(a) < len(b)
	})
	return res
}

func maxBucket(ts ...*Table) int64 {
	var mx int64
	for _, t := range ts {
		for _, b := range t.Buckets {
			if int64(b.Len()) > mx {
				mx = int64(b.Len())
			}
		}
	}
	return mx
}

type collector struct {
	L, R     *Table
	contribs []money.Paise
	n        int
	kMax     int
	delta    int64

	count        int
	capped       bool
	witnesses    []Witness
	seen         map[string]struct{}
	dedupNeeded  bool
	dedupRemoved int
	nearest      Miss

	scratchL [maxCardinality]int
	scratchR [maxCardinality]int
}

// probe walks every (left cardinality, right cardinality) pair whose sum is
// within kMax and range-queries the right bucket for each left entry.
//
// When complement is true the target is the pool total minus the settlement
// target, so a hit describes the set of records *excluded* from the batch;
// its complement is the witness. That is how a 312-of-315 batch is recovered
// in the same enumeration that recovers a 6-of-52 one.
func (c *collector) probe(target int64, complement bool) {
	// Each half caps its own enumeration at its own size, so a k_max larger
	// than a half is legal and simply means that half contributes fewer
	// cardinalities to the pairing.
	maxL := min(c.kMax, c.L.KMax)
	maxR := min(c.kMax, c.R.KMax)

	for cl := 0; cl <= maxL; cl++ {
		lb := c.L.Buckets[cl]
		if lb.Len() == 0 {
			continue
		}
		for cr := 0; cr <= maxR && cr+cl <= c.kMax; cr++ {
			rb := c.R.Buckets[cr]
			if rb.Len() == 0 {
				continue
			}
			// The rounding band is proportional to the cardinality of the
			// *settlement* subset, because only items actually inside the
			// batch accumulate per-transaction rounding drift.
			//
			// On the complement probe the enumerated cardinality is that of
			// the excluded set, so the band is n minus it. Applying the
			// excluded set's own cardinality here would silently under-widen
			// the band for exactly the large-batch settlements the complement
			// probe exists to solve, and the failure would look like a clean
			// UNRESOLVED rather than like a bug.
			band := cl + cr
			if complement {
				band = c.n - band
			}
			width := int64(band) * c.delta
			for li := 0; li < lb.Len(); li++ {
				need := target - lb.Sums[li]
				lo := lowerBound(rb.Sums, need-width)
				hi := upperBound(rb.Sums, need+width)

				// The nearest miss costs two array reads at the window edges
				// and is what makes an UNRESOLVED receipt actionable.
				c.trackNear(rb.Sums, lo, hi, need, lb.Sums[li], cl+cr, target)

				for ri := lo; ri < hi; ri++ {
					c.record(cl, lb.Ranks[li], cr, rb.Ranks[ri],
						lb.Sums[li]+rb.Sums[ri], target, complement)
					if c.capped {
						return
					}
				}
			}
		}
	}
}

func (c *collector) trackNear(sums []int64, lo, hi int, need, lsum int64, card int, target int64) {
	if lo < hi {
		return // something landed inside the band; no miss to record
	}
	consider := func(ri int) {
		if ri < 0 || ri >= len(sums) {
			return
		}
		s := lsum + sums[ri]
		gap := s - target
		if gap < 0 {
			gap = -gap
		}
		if !c.nearest.Valid || gap < int64(c.nearest.Gap) {
			c.nearest = Miss{
				Sum:         money.Paise(s),
				Gap:         money.Paise(gap),
				Cardinality: card,
				Valid:       true,
			}
		}
	}
	consider(lo - 1)
	consider(lo)
}

func (c *collector) record(cl int, rl uint32, cr int, rr uint32, sum, target int64, complement bool) {
	members := c.L.Members(cl, rl, c.scratchL[:])
	idx := make([]int, 0, cl+cr+8)
	idx = append(idx, members...)
	members = c.R.Members(cr, rr, c.scratchR[:])
	idx = append(idx, members...)

	if complement {
		idx = invert(idx, c.n)
	}
	sort.Ints(idx)

	if c.dedupNeeded {
		k := key(idx)
		if _, dup := c.seen[k]; dup {
			c.dedupRemoved++
			return
		}
		c.seen[k] = struct{}{}
	}

	c.count++
	if c.count >= countCap {
		c.capped = true
	}
	if len(c.witnesses) < maxCollected {
		slack := sum - target
		if slack < 0 {
			slack = -slack
		}
		// The probed sum belongs to the excluded set on a complement hit, so
		// the settlement-side sum is recomputed from the witness itself
		// rather than inferred. Receipts quote this number, and quoting the
		// wrong side of a complement would be a quiet, plausible-looking lie.
		var wsum money.Paise
		for _, i := range idx {
			wsum += c.contribs[i]
		}
		c.witnesses = append(c.witnesses, Witness{
			Indices:        idx,
			Sum:            wsum,
			Slack:          money.Paise(slack),
			FromComplement: complement,
		})
	}
}

// invert returns the complement of idx within [0, n).
func invert(idx []int, n int) []int {
	in := make([]bool, n)
	for _, i := range idx {
		in[i] = true
	}
	out := make([]int, 0, n-len(idx))
	for i := 0; i < n; i++ {
		if !in[i] {
			out = append(out, i)
		}
	}
	return out
}

// key canonicalises a sorted index set for deduplication.
func key(idx []int) string {
	b := make([]byte, 0, len(idx)*3)
	for _, i := range idx {
		b = append(b, byte(i), byte(i>>8), byte(i>>16))
	}
	return string(b)
}

// PredictEntries returns the enumeration size a dispatch would allocate,
// without allocating it. The resource ceiling is checked against this, which
// is the difference between refusing a job and being killed by the OS while
// doing it.
func PredictEntries(n, kMax int) (entries int64, bytes int64) {
	half := n / 2
	l := BinomSum(half, kMax)
	r := BinomSum(n-half, kMax)
	if l >= Cap || r >= Cap {
		return Cap, Cap
	}
	entries = l + r
	// Steady state plus the largest single bucket as sort scratch; the
	// largest bucket is bounded above by the larger half's total.
	return entries, entries*EntryBytes + r*SortTempBytes
}
