// Package narrow reduces the universe of records to the candidates that
// could financially belong to one settlement.
//
// This layer matters more than the solver, and the feasibility analysis says
// exactly how much: verification is only attainable when narrowing leaves an
// excess of roughly three to seven items over the true batch. Narrowing is
// not a performance optimisation that makes the solver practical. It is the
// thing that makes verification possible at all.
//
// A mathematically valid subset is not automatically a financially valid
// explanation. Two orders summing to the right number are irrelevant if one
// of them belongs to a different merchant, settled in a different cycle, or
// was already posted last week. So narrowing runs on deterministic business
// rules only, and every record it removes is logged with the rule that
// removed it. Narrowing is part of the audit trail, not preprocessing.
package narrow

import (
	"sort"
	"time"

	"github.com/Rishi0507/manhattan/internal/model"
)

// Constraint names one business rule. The names are stable because they
// appear on receipts, in the drift baseline and in remediation text.
type Constraint string

const (
	ConstraintMerchant     Constraint = "mid_mismatch"
	ConstraintCurrency     Constraint = "currency_mismatch"
	ConstraintWindow       Constraint = "outside_settlement_window"
	ConstraintReconciled   Constraint = "already_reconciled"
	ConstraintInstrument   Constraint = "payment_method_mismatch"
	ConstraintSettlementID Constraint = "settlement_reference_mismatch"

	// ConstraintZeroContribution removes records whose signed net effect on
	// the settlement is exactly zero.
	//
	// These are real and they are poison. A UPI payment carries zero merchant
	// discount rate under Indian regulation, so a UPI payment refunded in full
	// before settlement nets to precisely nothing: gross in, gross out, no fee
	// retained. The gateway still lists it, and it still lands in the pool.
	//
	// A single such record destroys uniqueness outright. If S reconstructs the
	// credit then so does S with the zero record added, and so does S with it
	// removed, because the sums are identical. Two zero records give four
	// reconstructions, three give eight. Every one of them is arithmetically
	// perfect and they differ only in membership, which is exactly what a
	// general ledger posting cares about.
	//
	// So they are removed and counted rather than searched over. The honest
	// statement is that these records cannot be attributed from amounts by any
	// method, that they cannot affect the credit either, and that the
	// reconstruction covers everything that moved money. Leaving them in the
	// pool would make almost every real settlement ambiguous for a reason that
	// carries no financial information.
	ConstraintZeroContribution Constraint = "zero_net_contribution"
)

// RelaxationOrder is the sequence in which constraints are considered for
// loosening when a settlement will not resolve.
//
// The order is a judgement call about which rules are most likely to be
// misconfigured rather than genuinely violated, and it is the one place the
// agent is permitted to intervene in narrowing: it may reorder this list
// given the shape of a residual. It may not add a constraint, remove one, or
// change what any of them does.
var RelaxationOrder = []Constraint{
	ConstraintWindow,     // T+n drift, weekends and holidays make this the flakiest
	ConstraintInstrument, // segregation is sometimes partial
	ConstraintReconciled, // a prior cycle may itself have been posted wrongly
	ConstraintCurrency,   // rare, and usually a genuine violation
	ConstraintMerchant,   // effectively never wrong; relaxing it is a last resort
}

// Drop records one removed candidate and why.
type Drop struct {
	RecordID   string     `json:"record_id"`
	Constraint Constraint `json:"constraint"`
}

// Config is the narrowing configuration for one settlement.
type Config struct {
	// Window is the half-width around the credit's expected capture window.
	// The gateway settles on T+n, so events belonging to a credit dated D are
	// expected around D minus n days, and Window is the tolerance on that.
	Window time.Duration
	// CycleDays is the T+n the merchant settles on.
	CycleDays int
	// EnforceInstrument applies the instrument constraint, for merchants
	// whose payouts are segregated by payment method.
	EnforceInstrument bool
	// TrustSettlementID applies the gateway's own mapping as a hard filter.
	// The demo posture leaves this false: the report is a claim to be
	// verified, and a verification that filters by the claim under test is
	// not a verification.
	TrustSettlementID bool
	// Relaxed lists constraints deliberately switched off for this pass,
	// which is how the neighbourhood probe widens a pool.
	Relaxed map[Constraint]bool
}

// DefaultConfig returns the shipped narrowing rules.
func DefaultConfig() Config {
	return Config{
		// Fourteen hours either side of the capture day's midpoint covers a
		// full trading day plus the boundary cases where a late-evening
		// capture could plausibly fall into either cycle.
		Window:            14 * time.Hour,
		CycleDays:         2,
		EnforceInstrument: false,
		TrustSettlementID: false,
	}
}

// WithRelaxed returns a copy with one more constraint switched off.
func (c Config) WithRelaxed(cs ...Constraint) Config {
	out := c
	out.Relaxed = map[Constraint]bool{}
	for k, v := range c.Relaxed {
		out.Relaxed[k] = v
	}
	for _, x := range cs {
		out.Relaxed[x] = true
	}
	return out
}

// WithWindow returns a copy with a different window half-width.
func (c Config) WithWindow(w time.Duration) Config {
	out := c
	out.Window = w
	return out
}

// Result is the narrowed pool plus the complete account of what was removed.
type Result struct {
	Pool        []model.Record     `json:"-"`
	Before      int                `json:"pool_before"`
	After       int                `json:"pool_after"`
	Dropped     map[Constraint]int `json:"dropped"`
	DropLog     []Drop             `json:"-"`
	WindowHours float64            `json:"window_hours"`
	Applied     []Constraint       `json:"constraints_applied"`
	Relaxed     []Constraint       `json:"constraints_relaxed,omitempty"`
}

// Apply narrows the record universe to candidates for one bank credit.
func Apply(records []model.Record, credit model.BankCredit, merchant model.Merchant, cfg Config) Result {
	res := Result{
		Before:      len(records),
		Dropped:     map[Constraint]int{},
		WindowHours: cfg.Window.Hours(),
	}

	enforce := func(c Constraint) bool { return !cfg.Relaxed[c] }
	for _, c := range []Constraint{
		ConstraintMerchant, ConstraintCurrency, ConstraintWindow, ConstraintReconciled,
		ConstraintZeroContribution,
	} {
		if enforce(c) {
			res.Applied = append(res.Applied, c)
		}
	}
	if cfg.EnforceInstrument && enforce(ConstraintInstrument) {
		res.Applied = append(res.Applied, ConstraintInstrument)
	}
	if cfg.TrustSettlementID && enforce(ConstraintSettlementID) {
		res.Applied = append(res.Applied, ConstraintSettlementID)
	}
	for c := range cfg.Relaxed {
		res.Relaxed = append(res.Relaxed, c)
	}
	sort.Slice(res.Relaxed, func(i, j int) bool { return res.Relaxed[i] < res.Relaxed[j] })

	// The batch settling on value date D covers events captured on the day
	// D minus the settlement cycle, so the window is centred on the middle of
	// that capture day rather than on its midnight boundary.
	//
	// This is not cosmetic. Centring on midnight and widening by a day and a
	// half, which is the obvious first implementation, sweeps in most of the
	// two adjacent capture days and triples the candidate pool. Since the
	// collision index grows like C(n, k), tripling n is the difference
	// between a settlement that verifies and one that is provably
	// underdetermined. Narrowing geometry is not a detail.
	centre := credit.ValueDate.AddDate(0, 0, -cfg.CycleDays).Add(12 * time.Hour)
	lo, hi := centre.Add(-cfg.Window), centre.Add(cfg.Window)

	drop := func(id string, c Constraint) {
		res.Dropped[c]++
		res.DropLog = append(res.DropLog, Drop{RecordID: id, Constraint: c})
	}

	for _, r := range records {
		switch {
		case enforce(ConstraintMerchant) && r.MerchantID != credit.MerchantID:
			drop(r.ID, ConstraintMerchant)
		case enforce(ConstraintCurrency) && r.Currency != "" && credit.Currency != "" && r.Currency != credit.Currency:
			drop(r.ID, ConstraintCurrency)
		case enforce(ConstraintReconciled) && r.Reconciled:
			drop(r.ID, ConstraintReconciled)
		case enforce(ConstraintWindow) && (r.EventAt.Before(lo) || r.EventAt.After(hi)):
			drop(r.ID, ConstraintWindow)
		case enforce(ConstraintZeroContribution) && r.Contribution == 0:
			drop(r.ID, ConstraintZeroContribution)
		case cfg.EnforceInstrument && enforce(ConstraintInstrument) &&
			credit.Instrument != "" && r.Instrument != "" && r.Instrument != credit.Instrument:
			drop(r.ID, ConstraintInstrument)
		case cfg.TrustSettlementID && enforce(ConstraintSettlementID) &&
			credit.Ref != "" && r.SettlementID != "" && r.SettlementID != credit.Ref:
			drop(r.ID, ConstraintSettlementID)
		default:
			res.Pool = append(res.Pool, r)
		}
	}

	res.After = len(res.Pool)
	return res
}

// DropRates returns each constraint's drop rate as a fraction of the input
// universe. The run-level drift monitor compares these against a stored
// baseline, which is the only way to catch a constraint that is wrong
// systematically rather than for one settlement.
func (r Result) DropRates() map[Constraint]float64 {
	out := map[Constraint]float64{}
	if r.Before == 0 {
		return out
	}
	for c, n := range r.Dropped {
		out[c] = float64(n) / float64(r.Before)
	}
	return out
}
