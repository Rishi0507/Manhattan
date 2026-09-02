package agent

import (
	"fmt"
	"sort"
	"time"

	"github.com/Rishi0507/manhattan/internal/evidence"
	"github.com/Rishi0507/manhattan/internal/model"
	"github.com/Rishi0507/manhattan/internal/money"
	"github.com/Rishi0507/manhattan/internal/narrow"
	"github.com/Rishi0507/manhattan/internal/pipeline"
)

// ActionKind is the closed set of moves the agent may make on a settlement.
//
// The set is closed for the same reason the hypothesis vocabulary is: free
// text cannot be executed, cannot be verified, and cannot be counted. An
// agent that returns prose has produced something no downstream stage can act
// on, and an agent that can invent its own actions has no trust boundary.
//
// Every one of these is an edit to the INPUTS of the pipeline. None of them
// touches the decision. After any action the entire gate, solver,
// completeness and recomputation stack runs again, completely unmodified, and
// it is free to conclude that the action made things worse.
type ActionKind string

const (
	// ActionTightenWindow narrows the value-date window.
	//
	// This is the highest-value move in the whole action space and it is the
	// one a purely hypothesis-driven agent cannot make. The collision index
	// grows like C(n, k), so pool size is the dominant term in whether a
	// settlement is decidable at all, and pool size is set by narrowing.
	// A settlement refused as UNDERDETERMINED is not refused because the
	// solver was too weak; it is refused because the window was too wide.
	ActionTightenWindow ActionKind = "TIGHTEN_WINDOW"

	// ActionWidenWindow loosens it, for a settlement whose true batch was
	// partly cut out by a window that was too tight.
	ActionWidenWindow ActionKind = "WIDEN_WINDOW"

	// ActionSplitByInstrument applies the payment-method constraint, for a
	// merchant whose payouts are segregated by instrument.
	ActionSplitByInstrument ActionKind = "SPLIT_BY_INSTRUMENT"

	// ActionSearchFeed looks in a source that was never joined into the pool
	// for a record of a named class.
	ActionSearchFeed ActionKind = "SEARCH_FEED"

	// ActionProposeAdjustment asserts an unmodelled event of a named class and
	// amount. Uncited, so it can route to review and can never post.
	ActionProposeAdjustment ActionKind = "PROPOSE_ADJUSTMENT"

	// ActionRelaxReconciled admits records marked posted in a prior cycle,
	// for the case where the prior cycle was itself wrong.
	ActionRelaxReconciled ActionKind = "RELAX_RECONCILED"

	// ActionEscalate stops and hands the settlement to a human, with
	// everything tried recorded. Choosing this deliberately is a real move and
	// often the right one.
	ActionEscalate ActionKind = "ESCALATE"
)

// AllActions is the vocabulary presented to the model.
var AllActions = []ActionKind{
	ActionTightenWindow, ActionWidenWindow, ActionSplitByInstrument,
	ActionSearchFeed, ActionProposeAdjustment, ActionRelaxReconciled, ActionEscalate,
}

// Corroborated reports whether an action's result may post.
//
// This is the same rule the hypothesis loop already had, generalised, and it
// was learned the hard way: without it, an agent that could retune narrowing
// produced two wrong postings out of three hundred.
//
// The failure is worth stating exactly, because it is subtle and it is the
// most instructive thing in this package. The agent tightened a value-date
// window from fourteen hours to seven. The candidate pool fell from 44 to 40,
// and a settlement that had been AMBIGUOUS became VERIFIED. Every check
// passed: the identity closed, the uniqueness count was one, the completeness
// probe found no rival within its depth bound. And the answer was wrong,
// because the tightening had cut real records out of the batch and a
// different subset of the survivors happened to sum to the credit.
//
// The error is not in the arithmetic. It is in what tightening MEANS.
//
//	Removing candidates cannot make the survivor unique. It makes it
//	unexamined. If two reconstructions existed in the wider pool, both are
//	still candidate explanations of that credit unless a business rule
//	genuinely excludes one, and "the agent thought the window looked wide"
//	is not a business rule.
//
// So narrowing actions are assertions about the merchant's settlement
// behaviour, and assertions need corroboration exactly as hypotheses do. An
// action that cites a real record in a real feed may post. An action that
// merely changes a filter may not, however cleanly the identity closes
// afterwards.
//
// What it produces instead is better than a posting anyway: a remediation
// carrying a PROVEN outcome rather than an estimated one. "Tightening this
// window to seven hours yields exactly one reconstruction" is a far stronger
// thing to hand an analyst than "consider tightening the window", and it is
// still their decision.
func (k ActionKind) Corroborated() bool {
	switch k {
	case ActionSearchFeed:
		// Introduces a real record from a real feed, cited by id on the
		// receipt. This is evidence.
		return true
	}
	// Everything else changes a filter or asserts an unmodelled event. Both
	// are claims about the world that this system has not verified.
	return false
}

// Action is one structured, executable move.
type Action struct {
	Kind ActionKind `json:"kind"`
	// WindowHours is the new value-date half-width, for the window actions.
	WindowHours float64 `json:"window_hours,omitempty"`
	// RecordKind names the class of record to search for, for SEARCH_FEED.
	RecordKind string `json:"record_kind,omitempty"`
	// HypothesisKind and AmountPaise carry a PROPOSE_ADJUSTMENT.
	HypothesisKind string `json:"hypothesis_kind,omitempty"`
	AmountPaise    int64  `json:"amount_paise,omitempty"`
	// Rationale is one sentence on why this move, given what was observed.
	Rationale string `json:"rationale"`
}

// Step is one turn of the loop, recorded for the receipt.
//
// The trace is not debug output. It is the account of how the agent arrived
// at a posting, and on a VERIFIED settlement that reached its status through
// agent action it is part of the audit trail: an auditor is entitled to know
// that the window was retuned before the identity closed, and by how much.
type Step struct {
	N           int             `json:"step"`
	Action      Action          `json:"action"`
	Observed    string          `json:"observed"`
	Result      evidence.Status `json:"result"`
	PoolBefore  int             `json:"pool_before"`
	PoolAfter   int             `json:"pool_after"`
	IndexBefore float64         `json:"collision_index_before"`
	IndexAfter  float64         `json:"collision_index_after"`
	Accepted    bool            `json:"accepted"`
	Note        string          `json:"note"`
	Citation    string          `json:"citation,omitempty"`
}

// String renders a step for a terminal trace.
func (s Step) String() string {
	mark := " "
	if s.Accepted {
		mark = "*"
	}
	return fmt.Sprintf("%s %d %-22s pool %d->%d  index %.3g->%.3g  %s",
		mark, s.N, s.Action.Kind, s.PoolBefore, s.PoolAfter, s.IndexBefore, s.IndexAfter, s.Result)
}

// Candidates returns every record in the unjoined feeds that could explain
// this settlement, for a SEARCH_FEED action.
//
// Returning all of them rather than the best guess is the whole correctness
// argument for this action, and it was learned by getting it wrong. An earlier
// version picked the largest matching record and posted if the identity then
// closed. It produced ten wrong postings in five hundred settlements, because
// citing a REAL record is not the same as citing the RIGHT one: where several
// records of the same class exist, more than one can close the arithmetic, and
// closing it with the wrong one is indistinguishable from closing it with the
// right one by looking at the sum.
//
// The rest of this system already refuses to choose between reconstructions it
// cannot tell apart. The same rule has to apply to the agent's citation: the
// controller tries every candidate, and accepts only if exactly one produces a
// verified result.
func Candidates(feed []model.Record, kind model.RecordKind, credit model.BankCredit, residual money.Paise) ([]model.Record, bool) {
	if kind == "" {
		kind = model.KindChargeback
	}
	var out []model.Record
	for i := range feed {
		r := feed[i]
		if r.MerchantID != credit.MerchantID || r.Kind != kind {
			continue
		}
		gap := credit.ValueDate.Sub(r.EventAt)
		if gap < 0 {
			gap = -gap
		}
		if gap > 7*24*time.Hour {
			continue
		}
		out = append(out, r)
	}

	// Rank by how nearly a record's contribution cancels the residual, which
	// is the arithmetic definition of a plausible explanation rather than a
	// guess. Ranking by size instead, as an earlier version did, put the true
	// record outside the tested set on merchants with many disputes and let a
	// coincidental one verify in its place.
	sort.Slice(out, func(i, j int) bool {
		return (out[i].Contribution + residual).Abs() < (out[j].Contribution + residual).Abs()
	})

	complete := len(out) <= maxFeedCandidates
	if !complete {
		out = out[:maxFeedCandidates]
	}
	return out, complete
}

// maxFeedCandidates bounds how many feed records are tried for one action.
//
// The bound is a resource limit, and the second return value reports whether
// it bit. That distinction is load bearing: a unique verification among a
// TRUNCATED candidate list is not a unique citation, because an untested
// record might have verified too, and this system does not post on a
// uniqueness it did not establish.
const maxFeedCandidates = 40

// apply turns an action into an overlay, which is the only way the agent can
// affect anything.
//
// Returning an overlay rather than mutating state is what makes the boundary
// checkable: every effect the agent can have on a decision is a value of this
// type, and a reader can enumerate the ways it could possibly matter.
func (a Action) apply(base narrow.Config, merchant model.Merchant, credit model.BankCredit, feed []model.Record) (*pipeline.Overlay, string, bool) {
	n := base
	n.CycleDays = merchant.SettlementCycleDays
	n.EnforceInstrument = merchant.InstrumentSegregated

	switch a.Kind {
	case ActionTightenWindow:
		h := a.WindowHours
		if h <= 0 || h >= base.Window.Hours() {
			// A tighten that does not tighten is a wasted step, so it is
			// refused here rather than burning an iteration on a re-solve
			// that cannot differ.
			return nil, "the proposed window is not tighter than the current one", false
		}
		if h < 2 {
			h = 2
		}
		n.Window = time.Duration(h * float64(time.Hour))
		return &pipeline.Overlay{Narrowing: &n, Provenance: string(a.Kind)},
			fmt.Sprintf("value-date window tightened to plus or minus %.0f hours", h), true

	case ActionWidenWindow:
		h := a.WindowHours
		if h <= base.Window.Hours() {
			h = base.Window.Hours() * 1.75
		}
		if h > 72 {
			h = 72
		}
		n.Window = time.Duration(h * float64(time.Hour))
		return &pipeline.Overlay{Narrowing: &n, Provenance: string(a.Kind)},
			fmt.Sprintf("value-date window widened to plus or minus %.0f hours", h), true

	case ActionSplitByInstrument:
		if credit.Instrument == "" {
			return nil, "this payout is not instrument-segregated, so there is nothing to split on", false
		}
		n.EnforceInstrument = true
		return &pipeline.Overlay{Narrowing: &n, Provenance: string(a.Kind)},
			"pool restricted to the payout's own payment method", true

	case ActionRelaxReconciled:
		n.Relaxed = map[narrow.Constraint]bool{narrow.ConstraintReconciled: true}
		return &pipeline.Overlay{Narrowing: &n, Provenance: string(a.Kind)},
			"records posted in a prior cycle admitted back into the pool", true

	case ActionSearchFeed:
		// Handled by Candidates, because one feed record is not an answer.
		return nil, "", false

	case ActionProposeAdjustment:
		amt := money.Paise(a.AmountPaise)
		if amt == 0 {
			return nil, "an adjustment of zero cannot change anything", false
		}
		h := Hypothesis{
			Kind:        HypothesisKind(a.HypothesisKind),
			AmountPaise: a.AmountPaise,
			Effect:      EffectAddItem,
		}
		if h.Kind == "" {
			h.Kind = KindAdjustment
		}
		return overlayFor(h, credit, ""),
			fmt.Sprintf("asserted an unmodelled %s of %s", h.Kind, amt), true
	}

	return nil, "", false
}
