package solver

import "github.com/Rishi0507/manhattan/internal/money"

// EntryBytes is the on-disk-equivalent size of one enumerated subset: an
// int64 sum plus a uint32 colex rank. The arrays are kept parallel and
// primitive rather than as a slice of structs, because a slice of structs
// would pad to 16 bytes and, more importantly, because the probe loop below
// walks the sums array linearly and wants nothing else in the cache line.
const EntryBytes = 12

// SortTempBytes is the transient cost of the radix sort, which needs a
// scratch buffer the size of the largest bucket. It is counted in the
// resource prediction because peak memory is what a ceiling is about, and
// quoting only the steady-state figure would understate it.
const SortTempBytes = 12

// Bucket holds every subset of one half at one exact cardinality, sorted
// ascending by sum.
//
// Cardinality is not metadata here. It is the index. That single choice is
// what makes three otherwise-awkward things fall out for free: the
// cardinality-proportional rounding band of inferred mode, the cross-check
// against a declared transaction count, and the complement's cardinality.
type Bucket struct {
	Sums  []int64
	Ranks []uint32
}

func (b Bucket) Len() int { return len(b.Sums) }

// Table is one half of the meet-in-the-middle enumeration.
type Table struct {
	// Offset is the index in the full pool of this half's first element, so
	// a local index i refers to pool element Offset+i.
	Offset  int
	M       int // number of pool items in this half
	KMax    int
	Buckets []Bucket // indexed by cardinality, 0..KMax
	Entries int64
}

// Enumerate builds the cardinality-bucketed table for one half of the pool.
//
// Combinations are generated in colexicographic order so that each entry's
// rank is its position in the stream; the sum is maintained incrementally
// across the colex successor, which touches only a short suffix of the
// index tuple on all but a vanishing fraction of steps.
func Enumerate(vals []money.Paise, offset, kMax int) *Table {
	m := len(vals)
	if kMax > m {
		kMax = m
	}
	t := &Table{Offset: offset, M: m, KMax: kMax, Buckets: make([]Bucket, kMax+1)}

	// Prefix sums of the lowest-indexed elements, used when the colex
	// successor resets a prefix back to 0,1,...,i-1.
	pre := make([]int64, m+1)
	for i, v := range vals {
		pre[i+1] = pre[i] + int64(v)
	}

	for c := 0; c <= kMax; c++ {
		n := Binom(m, c)
		if n <= 0 {
			t.Buckets[c] = Bucket{Sums: []int64{}, Ranks: []uint32{}}
			continue
		}
		sums := make([]int64, 0, n)
		ranks := make([]uint32, 0, n)

		if c == 0 {
			sums = append(sums, 0)
			ranks = append(ranks, 0)
			t.Buckets[c] = Bucket{Sums: sums, Ranks: ranks}
			t.Entries += 1
			continue
		}

		idx := make([]int, c)
		for i := range idx {
			idx[i] = i
		}
		sum := pre[c]

		for pos := uint32(0); ; pos++ {
			sums = append(sums, sum)
			ranks = append(ranks, pos)

			// Colex successor: find the lowest position that can advance.
			i := 0
			for {
				limit := m
				if i+1 < c {
					limit = idx[i+1]
				}
				if idx[i]+1 < limit {
					break
				}
				i++
				if i == c {
					break
				}
			}
			if i == c {
				break
			}
			// Advancing idx[i] and resetting idx[0..i-1] to 0..i-1.
			var dropped int64
			for j := 0; j < i; j++ {
				dropped += int64(vals[idx[j]])
			}
			sum -= dropped + int64(vals[idx[i]])
			idx[i]++
			sum += int64(vals[idx[i]]) + pre[i]
			for j := 0; j < i; j++ {
				idx[j] = j
			}
		}

		radixSortPairs(sums, ranks)
		t.Buckets[c] = Bucket{Sums: sums, Ranks: ranks}
		t.Entries += int64(len(sums))
	}
	return t
}

// Members writes the pool indices of the subset at (cardinality, rank).
func (t *Table) Members(card int, rank uint32, dst []int) []int {
	if card == 0 {
		return dst[:0]
	}
	dst = dst[:card]
	colexUnrank(rank, card, dst)
	for i := range dst {
		dst[i] += t.Offset
	}
	return dst
}

// radixSortPairs sorts sums ascending, carrying ranks along, using four
// 16-bit LSD passes with the sign bit flipped so that two's-complement
// negatives order correctly.
//
// Sign matters here and nowhere else in the solver: contributions are
// genuinely negative for chargebacks and fully-netted refunds, and a sort
// that mishandled negatives would corrupt exactly the batches the system
// most needs to get right. Once sorted, sign is invisible to everything
// downstream, which is the property that lets Manhattan skip the
// forced-inclusion machinery a truncating dynamic program would require.
func radixSortPairs(sums []int64, ranks []uint32) {
	n := len(sums)
	if n < 2 {
		return
	}
	if n < 64 {
		insertionSortPairs(sums, ranks)
		return
	}
	tmpS := make([]int64, n)
	tmpR := make([]uint32, n)
	var count [1 << 16]int32

	src, srcR, dst, dstR := sums, ranks, tmpS, tmpR
	for shift := uint(0); shift < 64; shift += 16 {
		for i := range count {
			count[i] = 0
		}
		for _, v := range src {
			count[(uint64(v)^signFlip)>>shift&0xFFFF]++
		}
		// Skip a pass entirely when every key shares the same digit, which is
		// the common case for the high words of paise-scale amounts.
		if count[(uint64(src[0])^signFlip)>>shift&0xFFFF] == int32(n) {
			continue
		}
		var total int32
		for i := range count {
			c := count[i]
			count[i] = total
			total += c
		}
		for i, v := range src {
			d := (uint64(v) ^ signFlip) >> shift & 0xFFFF
			p := count[d]
			count[d]++
			dst[p] = v
			dstR[p] = srcR[i]
		}
		src, dst = dst, src
		srcR, dstR = dstR, srcR
	}
	if &src[0] != &sums[0] {
		copy(sums, src)
		copy(ranks, srcR)
	}
}

const signFlip = uint64(1) << 63

func insertionSortPairs(sums []int64, ranks []uint32) {
	for i := 1; i < len(sums); i++ {
		s, r := sums[i], ranks[i]
		j := i - 1
		for j >= 0 && sums[j] > s {
			sums[j+1], ranks[j+1] = sums[j], ranks[j]
			j--
		}
		sums[j+1], ranks[j+1] = s, r
	}
}

// lowerBound returns the first index whose sum is >= v.
func lowerBound(a []int64, v int64) int {
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

// upperBound returns the first index whose sum is > v.
func upperBound(a []int64, v int64) int {
	lo, hi := 0, len(a)
	for lo < hi {
		mid := int(uint(lo+hi) >> 1)
		if a[mid] <= v {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	return lo
}
