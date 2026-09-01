// Package evidence defines the receipt: the structured record of one
// reconciliation decision.
//
// A receipt is not a log line. It records what the system concluded, what it
// ruled out, how hard it looked, what scope its claim is valid over, and what
// it would need in order to do better. That last part is what makes an
// exception list a work queue rather than an apology, and it is what gives
// the question-answering agent something to read.
package evidence

import (
	"github.com/Rishi0507/manhattan/internal/accounting"
	"github.com/Rishi0507/manhattan/internal/entropy"
	"github.com/Rishi0507/manhattan/internal/feasibility"
	"github.com/Rishi0507/manhattan/internal/guards"
	"github.com/Rishi0507/manhattan/internal/model"
	"github.com/Rishi0507/manhattan/internal/money"
	"github.com/Rishi0507/manhattan/internal/narrow"
	"github.com/Rishi0507/manhattan/internal/solver"
)

// Status is the decision. There are five, and four of them stop the money.
//
// The output of a reconciliation is deliberately not a confidence score.
// A score invites a threshold, a threshold trades precision against recall,
// and the whole argument of this system is that a settlement reconciler
// should not sit anywhere on that curve.
type Status string

const (
	// StatusVerified: exactly one valid explanation exists in the searched
	// region, uniqueness was counted exhaustively, and the accounting
	// equation closes. This is the only status that permits a posting.
	StatusVerified Status = "VERIFIED"

	// StatusAmbiguous: two or more distinct explanations reconstruct the
	// settlement. Both are exhibited, because an analyst may be able to
	// choose between them on grounds the arithmetic cannot see.
	StatusAmbiguous Status = "AMBIGUOUS"

	// StatusUnderdetermined: the combinatorics guarantee that a large
	// population of rival explanations exists. Exhibiting two of them would
	// be misleading, so none is shown and the remedy is named instead: more
	// data, not more thinking.
	StatusUnderdetermined Status = "UNDERDETERMINED"

	// StatusNarrowingSensitive: the answer depends on a filtering decision
	// rather than on the arithmetic. This is the most dangerous case in the
	// system and it is the one the Case 10 demo is built around.
	StatusNarrowingSensitive Status = "NARROWING_SENSITIVE"

	// StatusUnresolved: no valid explanation was found within the declared
	// tolerance. Carries the exact residual, which is what the resolution
	// agent works from.
	StatusUnresolved Status = "UNRESOLVED"
)

// Postable reports whether this status permits money to move.
func (s Status) Postable() bool { return s == StatusVerified }

// Flag is an orthogonal finding. Flags are not extra statuses: a settlement
// can be VERIFIED and simultaneously carry FEE_ANOMALY, because "the money is
// accounted for" and "the fee policy applied to it looks wrong" are different
// questions and conflating them is a modelling error.
type Flag string

const (
	FlagFeeAnomaly           Flag = "FEE_ANOMALY"
	FlagFeeCheckCircular     Flag = "FEE_CHECK_CIRCULAR"
	FlagRoundingApplied      Flag = "ROUNDING_APPLIED"
	FlagSignedItemsPresent   Flag = "SIGNED_ITEMS_PRESENT"
	FlagResolvedByHypothesis Flag = "RESOLVED_BY_HYPOTHESIS"
	FlagComplementSolved     Flag = "COMPLEMENT_SOLVED"
	FlagTwinSwap             Flag = "TWIN_SWAP"
	FlagLatticeCorrected     Flag = "LATTICE_CORRECTED"
	FlagEntropyInsufficient  Flag = "AMOUNT_ENTROPY_INSUFFICIENT"
	FlagResourceCeiling      Flag = "RESOURCE_CEILING"
)

// Remediation is a computed cure, not a suggestion. Where possible it says
// what the collision index would become if the change were made, which is
// the difference between a refusal and a refusal that is useful.
type Remediation struct {
	Action string `json:"action"`
	Effect string `json:"effect"`
	// ProjectedIndex, when set, is the collision index the named change is
	// estimated to produce.
	ProjectedIndex *float64 `json:"projected_collision_index,omitempty"`
	ProjectedPoolN *int     `json:"projected_pool_n,omitempty"`
}

// Uniqueness is the proof block. Its scope is printed rather than implied.
type Uniqueness struct {
	Method string `json:"method"`
	// Scope states exactly what the claim covers, in words, e.g.
	// "all subsets with k(S) <= 6".
	Scope string `json:"scope"`
	// ScopeSource says what bounded the region. A claim scoped by the
	// counterparty's own declared count is materially weaker than one scoped
	// by the gate, and the receipt never lets the two be confused.
	ScopeSource solver.ScopeSource `json:"scope_source"`
	ScopeNote   string             `json:"scope_note,omitempty"`
	// KMaxIfGateDerived is the scope the gate would have searched, recorded
	// whenever a declared count was used instead.
	KMaxIfGateDerived *int `json:"k_max_if_gate_derived,omitempty"`

	ScopeComplete     bool `json:"scope_complete"`
	MatchesFound      int  `json:"matches_found"`
	RivalsFound       int  `json:"rivals_found"`
	CountedAfterDedup bool `json:"counted_after_dedup"`
	CountSaturated    bool `json:"count_saturated"`

	AlternativeWitnesses    [][]string `json:"alternative_witnesses"`
	CumulativeIndexInRegion float64    `json:"cumulative_index_in_searched_region"`
}

// SolverBlock is what the reconstruction step did.
type SolverBlock struct {
	Method        string             `json:"method"`
	KMax          int                `json:"k_max"`
	KMaxSource    solver.ScopeSource `json:"k_max_source"`
	Split         [2]int             `json:"split"`
	EntriesLeft   int64              `json:"entries_left"`
	EntriesRight  int64              `json:"entries_right"`
	EntryEncoding string             `json:"entry_encoding"`
	MemoryBytes   int64              `json:"memory_bytes"`
	MemoryCeiling int64              `json:"memory_ceiling_bytes"`
	ProbedTargets []string           `json:"probed_targets"`
	SolveSide     string             `json:"solve_side"`
	DedupApplied  bool               `json:"dedup_applied"`
	DedupRemoved  int                `json:"dedup_removed"`
	NearestMiss   *solver.Miss       `json:"nearest_miss,omitempty"`
}

// RoundingBlock makes the tolerance assumption visible on every receipt.
type RoundingBlock struct {
	Mode           accounting.RoundingMode `json:"mode"`
	TolerancePaise money.Paise             `json:"tolerance_paise"`
	BandBasis      string                  `json:"band_basis"`
	SlackAllowed   money.Paise             `json:"slack_allowed_paise"`
	SlackConsumed  money.Paise             `json:"slack_consumed_paise"`
}

// FeeCheck is Leg C: an independent recomputation, not a join.
type FeeCheck struct {
	Mode string `json:"mode"`
	// Circular marks the data mode in which the check cannot fail, in which
	// case no anomaly claim is made at all. Reporting a check that cannot
	// fail is worse than reporting no check, because it looks like assurance.
	Circular             bool        `json:"circular"`
	ExpectedMDR          money.Paise `json:"expected_mdr_paise"`
	ObservedMDR          money.Paise `json:"observed_mdr_paise"`
	DeltaBps             int64       `json:"delta_bps"`
	BandBps              int64       `json:"band_bps"`
	RoundingComponentBps int64       `json:"rounding_component_bps"`
	WithinBand           bool        `json:"within_band"`
	Claim                string      `json:"claim"`
}

// NarrowingBlock is the audit trail for what was excluded and why.
type NarrowingBlock struct {
	Before      int                       `json:"pool_before"`
	After       int                       `json:"pool_after"`
	Dropped     map[narrow.Constraint]int `json:"dropped"`
	WindowHours float64                   `json:"window_hours"`
	Applied     []narrow.Constraint       `json:"constraints_applied"`

	Neighbourhood *guards.NeighbourhoodResult `json:"neighbourhood_probe,omitempty"`
	Checks        []guards.Check              `json:"completeness_checks"`
}

// AgentBlock records what the model contributed, and, more importantly, what
// it was not allowed to contribute.
type AgentBlock struct {
	Invoked    bool   `json:"invoked"`
	Provider   string `json:"provider,omitempty"`
	Iterations int    `json:"iterations,omitempty"`
	// Hypotheses are the structured, executable assertions the agent made.
	// Each one was applied to the pool and the unmodified verifier stack was
	// re-run over it. The model never got a vote on whether it was right.
	Hypotheses []Hypothesis `json:"hypotheses,omitempty"`
	Accepted   *Hypothesis  `json:"accepted,omitempty"`
	Note       string       `json:"note,omitempty"`
}

// Hypothesis is one proposed explanation for a residual.
type Hypothesis struct {
	Kind   string      `json:"kind"`
	Amount money.Paise `json:"amount_paise"`
	Effect string      `json:"effect"`
	// SourceRef is the record the hypothesis cites. This field is the entire
	// posting rule: a hypothesis citing a real record in a real feed can
	// yield VERIFIED with a citation attached, and a hypothesis with a null
	// source ref is speculative and can never produce a posting under any
	// circumstances. Its best outcome is a named, arithmetically consistent
	// suggestion for an analyst.
	SourceRef string `json:"source_ref,omitempty"`
	Evidence  string `json:"evidence,omitempty"`
	Rationale string `json:"rationale,omitempty"`

	Outcome string `json:"outcome,omitempty"`
}

// Citable reports whether this hypothesis may lead to a posting.
func (h Hypothesis) Citable() bool { return h.SourceRef != "" }

// CostBlock is the model spend for this settlement.
type CostBlock struct {
	ModelCalls   int   `json:"model_calls"`
	InputTokens  int   `json:"input_tokens"`
	OutputTokens int   `json:"output_tokens"`
	INRMicros    int64 `json:"inr_micros"`
}

// Receipt is the complete evidence object for one settlement.
type Receipt struct {
	SettlementRef string         `json:"settlement_ref"`
	RunID         string         `json:"run_id"`
	MerchantID    string         `json:"merchant_id"`
	MerchantName  string         `json:"merchant_name,omitempty"`
	Archetype     string         `json:"merchant_archetype,omitempty"`
	Status        Status         `json:"status"`
	Flags         []Flag         `json:"flags"`
	DataMode      model.DataMode `json:"data_mode"`

	Narration   string      `json:"narration,omitempty"`
	TargetPaise money.Paise `json:"target_paise"`
	ValueDate   string      `json:"value_date"`

	Pool          PoolBlock          `json:"pool"`
	AmountEntropy entropy.Report     `json:"amount_entropy"`
	Feasibility   feasibility.Report `json:"feasibility"`
	Solver        *SolverBlock       `json:"solver,omitempty"`
	Uniqueness    *Uniqueness        `json:"uniqueness,omitempty"`

	Witness         []string `json:"witness,omitempty"`
	WitnessSize     int      `json:"witness_size"`
	NegativeMembers []string `json:"negative_members,omitempty"`

	Accounting *accounting.Equation `json:"accounting,omitempty"`
	Rounding   RoundingBlock        `json:"rounding"`
	Narrowing  NarrowingBlock       `json:"narrowing"`
	FeeCheck   *FeeCheck            `json:"fee_check,omitempty"`
	Agent      AgentBlock           `json:"agent"`

	Claim       string        `json:"claim"`
	Note        string        `json:"note,omitempty"`
	Remediation []Remediation `json:"remediation,omitempty"`

	ExceptionCostINR int `json:"exception_cost_inr,omitempty"`

	Cost     CostBlock        `json:"cost"`
	TimingMS map[string]int64 `json:"timing_ms"`

	PolicyVersion string `json:"policy_version"`
	ReplaySeed    int64  `json:"replay_seed"`
}

// PoolBlock describes the candidate pool the decision was made over.
type PoolBlock struct {
	N           int         `json:"n"`
	SigmaPaise  float64     `json:"contribution_sigma_paise"`
	SignedItems int         `json:"signed_items"`
	TotalPaise  money.Paise `json:"total_contribution_paise"`
}

// HasFlag reports whether a flag is present.
func (r *Receipt) HasFlag(f Flag) bool {
	for _, x := range r.Flags {
		if x == f {
			return true
		}
	}
	return false
}

// AddFlag appends a flag once.
func (r *Receipt) AddFlag(f Flag) {
	if !r.HasFlag(f) {
		r.Flags = append(r.Flags, f)
	}
}

// Validate enforces the two hard invariants that tie status to evidence.
//
// Orthogonality between status and flags has exactly two exceptions, and
// both are enforced here rather than assumed. VERIFIED asserts that
// uniqueness was established over a completely searched region, so it
// requires zero rivals and a complete scope. And TWIN_SWAP means an
// alternative witness has been constructed, not merely suspected, so the
// combination of VERIFIED and TWIN_SWAP is unrepresentable.
//
// Note what is absent: there is no flag for uniqueness having been left
// unverified. The solver has no budget that can run out mid-proof.
// Uniqueness is not a sweep bolted on after the search, it is a count
// produced by the search itself. Either the region was enumerated or it was
// not, and if it was not, nothing was found at all.
func (r *Receipt) Validate() error {
	if r.Status != StatusVerified {
		return nil
	}
	if r.Uniqueness == nil {
		return errInvariant("VERIFIED with no uniqueness block")
	}
	if r.Uniqueness.RivalsFound != 0 {
		return errInvariant("VERIFIED with %d rivals found", r.Uniqueness.RivalsFound)
	}
	if !r.Uniqueness.ScopeComplete {
		return errInvariant("VERIFIED without a completely searched region")
	}
	if r.HasFlag(FlagTwinSwap) {
		return errInvariant("VERIFIED carrying TWIN_SWAP: an alternative witness was constructed, so uniqueness is false")
	}
	if r.Accounting == nil || !r.Accounting.Closes {
		return errInvariant("VERIFIED without the accounting equation closing")
	}
	return nil
}
