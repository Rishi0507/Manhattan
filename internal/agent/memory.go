package agent

import (
	"fmt"
	"sort"
	"time"

	"github.com/Rishi0507/manhattan/internal/evidence"
	"github.com/Rishi0507/manhattan/internal/model"
)

// Profile is what this merchant's own settled history says about how it
// batches, built from receipts the system already proved.
//
// This is the second source the corroboration rule was always waiting for.
//
// The rule says an action may post only if it introduces evidence rather than
// removing candidates, because narrowing a pool cannot make the survivor
// unique, only unexamined. That is correct and it left a gap: a narrowing
// change is an assertion about a merchant's settlement behaviour, and the
// system had no way to establish such an assertion, so it prohibited the
// action outright. Prohibition is the right default and it is not the right
// permanent answer.
//
// A merchant's own prior VERIFIED settlements are exactly the missing source.
// If the last dozen proved batches for this merchant all fell inside a
// nine-hour window around the capture midpoint, "this merchant settles within
// nine hours" is not the model's opinion. It is a measurement over proofs,
// each of which was established by exhaustive enumeration without reference
// to any window hypothesis.
//
// So a window narrowed to a bound this history supports is corroborated in the
// same sense a cited feed record is: something outside the current settlement
// asserts it, and that something is not a language model. The model's job is
// to read the profile and propose which bound to try, which is judgement over
// evidence rather than evidence itself.
//
// Three properties keep this honest, and the first two are what make it
// postable at all:
//
//   - the profile is built ONLY from VERIFIED receipts, so it cannot be
//     bootstrapped from guesses. A merchant with no proofs has no profile and
//     the action is unavailable.
//   - MinSettlements proofs are required before any bound is offered, so one
//     unusual settlement cannot establish a pattern.
//   - the proposed bound must be no tighter than the widest offset actually
//     observed across those proofs. The agent may not invent a tighter rule
//     than the merchant's own settled history demonstrates.
//
// And the verifier still decides. A corroborated window is applied as an
// overlay, the entire stack re-runs over the narrowed pool, and if the result
// is not a unique reconstruction with the identity closing then nothing posts.
// Corroboration buys the right to be tested, not the right to be believed.
type Profile struct {
	MerchantID string `json:"merchant_id"`
	// Proofs is how many VERIFIED settlements this profile is built from.
	Proofs int `json:"verified_settlements"`

	// MaxOffsetHours is the largest gap observed between a proved batch's
	// records and its credit's value date, across every proof. It is the
	// tightest window this history can honestly support.
	MaxOffsetHours float64 `json:"max_observed_offset_hours"`
	// MedianOffsetHours is where the mass sits, reported so the model can see
	// whether the maximum is typical or an outlier.
	MedianOffsetHours float64 `json:"median_observed_offset_hours"`

	// BatchSizes are the proved batch cardinalities, smallest to largest.
	MinBatch int `json:"min_proved_batch"`
	MaxBatch int `json:"max_proved_batch"`

	// Instruments observed inside proved batches, so a SPLIT_BY_INSTRUMENT
	// proposal can be checked against whether this merchant actually
	// segregates.
	Instruments []string `json:"instruments_in_proved_batches"`
	// SignedShare is the fraction of proved batches containing a negative
	// contribution, which is what tells the model whether a residual of the
	// wrong sign is normal here.
	SignedShare float64 `json:"share_of_proved_batches_with_signed_items"`
}

// MinProofsForProfile is how many proved settlements a merchant needs before
// its history may corroborate anything.
//
// Twelve is chosen so a profile spans more than one settlement cycle for every
// archetype the generator models. Below that the "pattern" is a handful of
// settlements that happened to agree, and a rule established from a handful of
// agreeing observations is how confident wrong answers get made.
const MinProofsForProfile = 12

// BuildProfile summarises a merchant's proved settlements.
//
// Returns nil when the merchant has too few proofs, which is the common case
// early in a run and is the correct answer: no history, no corroboration, no
// postable narrowing.
func BuildProfile(merchantID string, proved []*evidence.Receipt, byID map[string]model.Record, cycleDays int) *Profile {
	var offsets []float64
	p := &Profile{MerchantID: merchantID, MinBatch: 1 << 30}
	instruments := map[string]bool{}
	var signed int

	for _, r := range proved {
		if r.MerchantID != merchantID || r.Status != evidence.StatusVerified || len(r.Witness) == 0 {
			continue
		}
		// Offsets have to be measured from the SAME reference narrowing uses,
		// or the number is not comparable to the window it is meant to
		// corroborate. Narrowing centres on the capture day's midpoint, which
		// is the value date less the merchant's settlement cycle, so this
		// does the same.
		//
		// Measuring from the value date instead was the first version and it
		// produced offsets of sixty hours against a fourteen-hour window on a
		// T+2 merchant, so the profile never corroborated anything and the
		// action never fired. The bug was silent: no error, no wrong answer,
		// just a feature that quietly did nothing.
		vd, err := time.Parse("2006-01-02", r.ValueDate)
		if err != nil {
			continue
		}
		centre := vd.AddDate(0, 0, -cycleDays).Add(12 * time.Hour)
		p.Proofs++
		if n := len(r.Witness); n < p.MinBatch {
			p.MinBatch = n
		}
		if n := len(r.Witness); n > p.MaxBatch {
			p.MaxBatch = n
		}

		var worst float64
		var hasSigned bool
		for _, id := range r.Witness {
			rec, ok := byID[id]
			if !ok {
				continue
			}
			if rec.Contribution < 0 {
				hasSigned = true
			}
			if rec.Instrument != "" {
				instruments[string(rec.Instrument)] = true
			}
			gap := centre.Sub(rec.EventAt)
			if gap < 0 {
				gap = -gap
			}
			if h := gap.Hours(); h > worst {
				worst = h
			}
		}
		if hasSigned {
			signed++
		}
		offsets = append(offsets, worst)
	}

	if p.Proofs < MinProofsForProfile {
		return nil
	}
	sort.Float64s(offsets)
	p.MaxOffsetHours = offsets[len(offsets)-1]
	p.MedianOffsetHours = offsets[len(offsets)/2]
	p.SignedShare = float64(signed) / float64(p.Proofs)
	for i := range instruments {
		p.Instruments = append(p.Instruments, i)
	}
	sort.Strings(p.Instruments)
	return p
}

// Supports reports whether this merchant's proved history corroborates a
// window of the given half-width, and says why or why not.
//
// The bound is the widest offset the history actually shows. Proposing
// anything tighter is proposing a rule the merchant's own settlements
// contradict, and it is refused rather than argued with.
func (p *Profile) Supports(hours float64) (bool, string) {
	if p == nil {
		return false, "this merchant has no proved settlement history, so nothing corroborates a window change"
	}
	if p.Proofs < MinProofsForProfile {
		return false, fmt.Sprintf(
			"this merchant has %d proved settlements and %d are required before its history may "+
				"corroborate a narrowing change", p.Proofs, MinProofsForProfile)
	}
	if hours < p.MaxOffsetHours {
		return false, fmt.Sprintf(
			"%.0fh is tighter than this merchant's own history supports: across %d proved "+
				"settlements the widest gap between a batch record and its credit was %.1fh, so a "+
				"%.0fh window would have cut real records out of a batch this system already proved",
			hours, p.Proofs, p.MaxOffsetHours, hours)
	}
	return true, fmt.Sprintf(
		"corroborated by %d proved settlements for this merchant, whose widest observed gap is "+
			"%.1fh and whose median is %.1fh. The bound is not tighter than the history shows",
		p.Proofs, p.MaxOffsetHours, p.MedianOffsetHours)
}

// Render describes the profile to the model, in the observation block.
func (p *Profile) Render() string {
	if p == nil {
		return "merchant history   none yet; too few proved settlements to corroborate anything\n"
	}
	return fmt.Sprintf(
		"merchant history   %d proved settlements. batches of %d to %d records. widest gap between\n"+
			"                   a proved record and its credit %.1fh, median %.1fh. instruments %v.\n"+
			"                   %.0f%% of proved batches contained a signed item.\n"+
			"                   a window of at least %.0fh is corroborated by this history.\n",
		p.Proofs, p.MinBatch, p.MaxBatch, p.MaxOffsetHours, p.MedianOffsetHours,
		p.Instruments, p.SignedShare*100, p.MaxOffsetHours)
}

// ProfileStore accumulates proved settlements per merchant across a run.
//
// Ordinary map, deliberately: cross-settlement memory here means "what this
// merchant's already-proved settlements demonstrate", not a learned model.
// Anything a proof did not establish does not go in.
type ProfileStore struct {
	proved map[string][]*evidence.Receipt
}

// NewProfileStore returns an empty store.
func NewProfileStore() *ProfileStore {
	return &ProfileStore{proved: map[string][]*evidence.Receipt{}}
}

// Observe records a settlement, keeping only the proved ones.
func (s *ProfileStore) Observe(r *evidence.Receipt) {
	if s == nil || r == nil || r.Status != evidence.StatusVerified || len(r.Witness) == 0 {
		return
	}
	s.proved[r.MerchantID] = append(s.proved[r.MerchantID], r)
}

// For builds the profile for one merchant, or nil if there is not enough
// history.
func (s *ProfileStore) For(merchantID string, byID map[string]model.Record, cycleDays int) *Profile {
	if s == nil {
		return nil
	}
	return BuildProfile(merchantID, s.proved[merchantID], byID, cycleDays)
}
