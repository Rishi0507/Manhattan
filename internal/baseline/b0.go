// Package baseline implements B0, the system Manhattan is measured against.
//
// B0 is a confidence matcher: it narrows the pool exactly as Manhattan does,
// searches for a subset that hits the target, scores its answer, and posts
// anything above a threshold. That is what most reconciliation tooling in
// this space does, and it is what a fuzzy-matching or LLM-adjudicating
// approach reduces to once the marketing is removed.
//
// It is built honestly and given every advantage. Same inputs, same
// narrowing, same integer arithmetic, no strawmanning. Where it differs from
// Manhattan is precisely and only in the things this project argues are the
// point:
//
//   - it does not count rival explanations, so it cannot tell a unique answer
//     from one of thousands
//   - it does not gate on feasibility, so it answers questions that are not
//     answerable
//   - it does not test whether its answer survives its own filtering, so it
//     cannot detect that narrowing handed it a coincidence
//   - it reports a confidence score, so its precision is a tuning parameter
//
// A comparison against a deliberately weak baseline proves nothing, so this
// one is deliberately competent. The wrong-posting count it produces is the
// number that makes Manhattan's zero mean something, and a zero reported in
// isolation would be close to tautological.
package baseline

import (
	"sort"

	"github.com/Rishi0507/manhattan/internal/model"
	"github.com/Rishi0507/manhattan/internal/money"
)

// Result is one B0 decision.
type Result struct {
	SettlementRef string      `json:"settlement_ref"`
	Proposed      []string    `json:"proposed_witness"`
	Sum           money.Paise `json:"proposed_sum_paise"`
	Target        money.Paise `json:"target_paise"`
	Gap           money.Paise `json:"gap_paise"`
	Confidence    float64     `json:"confidence"`
	Posted        bool        `json:"posted"`
	Method        string      `json:"method"`
	PoolN         int         `json:"pool_n"`
	// TokensIn estimates what an LLM matcher would have spent on this
	// settlement, because the cost comparison is part of the result and a
	// matcher that has to read the pool pays for the pool.
	TokensIn int `json:"estimated_input_tokens"`
}

// Config sets the posting threshold.
type Config struct {
	// Threshold is the confidence above which B0 posts. 0.8 is the value
	// such tools typically ship with.
	Threshold float64
	// MaxCardinality bounds the search, as any practical matcher must.
	MaxCardinality int
}

// DefaultConfig returns the shipped baseline settings.
func DefaultConfig() Config { return Config{Threshold: 0.8, MaxCardinality: 8} }

// The token model behind B0's cost figure, stated as constants so the
// arithmetic in RESULTS.md can be reproduced by hand.
//
// TokensPerRecord covers one candidate rendered as a line an LLM can reason
// over: an id, a rupee amount, a timestamp, an instrument and a kind. Forty
// tokens is measured against that rendering rather than assumed, and it is on
// the low side, which understates B0's cost rather than inflating it.
//
// The pool being counted is the NARROWED pool. B0 is handed Manhattan's
// narrowing for free, so it never pays to read the full universe. A matcher
// without that narrowing would read several hundred records per settlement
// instead of a few dozen, and the cost gap this benchmark reports would be
// roughly an order of magnitude wider. The conservative choice is deliberate:
// a cost advantage argued from a handicapped baseline is not an advantage.
const (
	TokensPerRecord = 40
	TokensOverhead  = 200
)

// Features names everything B0's confidence score is computed from.
//
// It is published because the headline comparison rests on B0's wrong-posting
// count, and a number produced by a component nobody can inspect is exactly
// the sort of claim this project refuses elsewhere. Anyone can read this list,
// read score(), and decide whether the baseline was given a fair run.
func Features() []string {
	return []string{
		"exact integer hit on the target contribution sum (confidence 0.90)",
		"near hit within 1 basis point of the target (0.72)",
		"near hit within 1 per cent of the target (0.45)",
		"no hit found within the node budget (0.15)",
		"cardinality agrees with the settlement report's declared count (+0.05)",
	}
}

// Match runs B0 over one narrowed pool.
//
// The search is a bounded best-first walk: sort by magnitude, take the
// largest record that does not overshoot, repeat, and backtrack a little.
// It is the shape of thing a competent engineer writes in an afternoon, and
// on clean data it works. What it never does is ask whether a second answer
// exists.
func Match(pool []model.Record, target money.Paise, declared *int, cfg Config) Result {
	res := Result{
		Target: target,
		PoolN:  len(pool),
		Method: "greedy_best_first_with_backtracking",
		// A fuzzy matcher has to put the candidate pool into the model's
		// context window to reason over it.
		TokensIn: TokensOverhead + TokensPerRecord*len(pool),
	}

	idx := make([]int, len(pool))
	for i := range idx {
		idx[i] = i
	}
	sort.Slice(idx, func(a, b int) bool {
		return pool[idx[a]].Contribution.Abs() > pool[idx[b]].Contribution.Abs()
	})

	maxCard := cfg.MaxCardinality
	if declared != nil && *declared > 0 {
		maxCard = *declared
	}

	best, bestGap, found := searchGreedy(pool, idx, target, maxCard)
	if !found {
		res.Confidence = 0.0
		res.Gap = target
		return res
	}

	var sum money.Paise
	ids := make([]string, 0, len(best))
	for _, i := range best {
		sum += pool[i].Contribution
		ids = append(ids, pool[i].ID)
	}
	sort.Strings(ids)

	res.Proposed = ids
	res.Sum = sum
	res.Gap = bestGap

	// Confidence is a similarity heuristic, which is exactly the point: it
	// measures how good the match looks, not whether it is the only one. An
	// exact hit at the declared cardinality scores very high, and it scores
	// just as high when the pool contains a thousand other exact hits.
	res.Confidence = score(bestGap, target, len(best), declared)
	res.Posted = res.Confidence >= cfg.Threshold
	return res
}

func score(gap, target money.Paise, card int, declared *int) float64 {
	conf := 0.0
	switch {
	case gap == 0:
		conf = 0.90
	case target != 0 && float64(gap.Abs())/float64(target.Abs()) < 0.0001:
		conf = 0.72
	case target != 0 && float64(gap.Abs())/float64(target.Abs()) < 0.01:
		conf = 0.45
	default:
		conf = 0.15
	}
	if declared != nil && *declared == card {
		conf += 0.05
	}
	if conf > 0.99 {
		conf = 0.99
	}
	return conf
}

// searchGreedy walks the pool largest-first, taking any record that does not
// overshoot, and backtracks over the last few choices when it stalls.
func searchGreedy(pool []model.Record, idx []int, target money.Paise, maxCard int) ([]int, money.Paise, bool) {
	type state struct {
		chosen []int
		sum    money.Paise
		next   int
	}

	bestGap := money.Paise(1) << 62
	var bestSet []int
	found := false

	// A bounded depth-first walk. The node budget is what any practical
	// matcher has to impose, and it is generous enough that this is not a
	// handicap on the pool sizes in play.
	const nodeBudget = 200000
	nodes := 0

	var walk func(s state)
	walk = func(s state) {
		nodes++
		if nodes > nodeBudget {
			return
		}
		gap := s.sum - target
		if gap.Abs() < bestGap {
			bestGap = gap.Abs()
			bestSet = append([]int{}, s.chosen...)
			found = true
		}
		if gap == 0 {
			return // an exact hit is enough; B0 does not look for a second
		}
		if len(s.chosen) >= maxCard || s.next >= len(idx) {
			return
		}
		for i := s.next; i < len(idx); i++ {
			c := pool[idx[i]].Contribution
			// Skip anything that overshoots, unless it is signed, in which
			// case it can bring an overshoot back.
			if c > 0 && s.sum+c-target > 0 && s.sum > target {
				continue
			}
			walk(state{
				chosen: append(append([]int{}, s.chosen...), idx[i]),
				sum:    s.sum + c,
				next:   i + 1,
			})
			if bestGap == 0 {
				return
			}
		}
	}
	walk(state{next: 0})

	if !found {
		return nil, 0, false
	}
	return bestSet, bestGap, true
}

// Correct reports whether a posted result matches ground truth exactly.
// Matching the amount is not enough: matching the amount with the wrong
// records is the failure being counted.
func (r Result) Correct(truth []string) bool {
	if len(r.Proposed) != len(truth) {
		return false
	}
	m := map[string]int{}
	for _, x := range r.Proposed {
		m[x]++
	}
	for _, x := range truth {
		m[x]--
		if m[x] < 0 {
			return false
		}
	}
	return true
}
