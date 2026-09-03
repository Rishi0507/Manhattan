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

	// ZeroContributionRecords are records that survived every business
	// constraint but net to exactly zero, so they moved no money and cannot be
	// identified from the credit by any amount-based method.
	//
	// They are named rather than merely counted, because a general ledger
	// posting has to account for them somehow and "we found 24 records and
	// there were 25 in the batch" is not an answer. The honest statement is
	// that the reconstruction is exact over everything that moved money, and
	// that these records are unattributable by amount and unattributable for a
	// reason that is a fact about them rather than a limitation of the method.
	ZeroContributionRecords []string `json:"zero_contribution_records,omitempty"`
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

	// Steps is the agent's decision trace: what it observed, what it chose to
	// do, and what the verifier concluded afterwards.
	//
	// This is not debug output. On a settlement that reached VERIFIED through
	// agent action, an auditor is entitled to know that the value-date window
	// was retuned before the identity closed, and by how much. A posting whose
	// provenance includes an agent decision has to carry that decision.
	Steps []AgentStep `json:"steps,omitempty"`
}

// AgentStep is one turn of the controller loop.
type AgentStep struct {
	N           int     `json:"step"`
	Kind        string  `json:"action"`
	Rationale   string  `json:"rationale"`
	Observed    string  `json:"observed_status"`
	Result      Status  `json:"result_status"`
	PoolBefore  int     `json:"pool_before"`
	PoolAfter   int     `json:"pool_after"`
	IndexBefore float64 `json:"collision_index_before"`
	IndexAfter  float64 `json:"collision_index_after"`
	Accepted    bool    `json:"accepted"`
	Note        string  `json:"note"`
	Citation    string  `json:"citation,omitempty"`
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

// ClaimVerdict is the outcome of checking a mapping somebody else asserted.
type ClaimVerdict string

const (
	// ClaimConsistent means the named batch does produce this credit. It is
	// deliberately weaker than VERIFIED: consistent is not unique, because
	// nothing was searched for.
	ClaimConsistent ClaimVerdict = "CLAIM_CONSISTENT"
	// ClaimContradicted means the report's own account of this settlement does
	// not survive checking against the merchant's records.
	ClaimContradicted ClaimVerdict = "CLAIM_CONTRADICTED"
	// ClaimUncheckable means the claim names a record that exists in a source
	// nobody joined, so the check could not be performed.
	//
	// This verdict was added after measuring, and the measurement is the
	// reason it exists. Without it the claim check reported 84 contradictions
	// on 469 clean reports, an 18 per cent false-alarm rate that would have
	// made the composite unshippable. Every one of the 84 was a chargeback
	// sitting in an unjoined disputes feed: the report was right, the money
	// was right, and the record was simply not in the data the checker could
	// see.
	//
	// Calling that a contradiction would blame the counterparty for a
	// connection nobody made on this side, which is the same error the feed
	// completeness guard exists to avoid. The honest verdict is that the check
	// did not run, and the remedy is to join the feed.
	ClaimUncheckable ClaimVerdict = "CLAIM_UNCHECKABLE"
)

// DraftedNote is the analyst-facing note for a held settlement.
//
// Prose only. Every figure in the rendered version is substituted from this
// receipt afterwards, and a draft containing digits is rejected wholesale
// rather than published, which is what makes the no-invented-numbers rule
// enforceable instead of advisory. Rejected carries the reason when that
// happens.
//
// A note can never change a posting: it is attached to a settlement that is
// already held either way. This is the only model output in the system whose
// failure mode is a confusing sentence rather than a wrong ledger.
type DraftedNote struct {
	Do       string `json:"what_to_do,omitempty"`
	Because  string `json:"why_it_works,omitempty"`
	NotFixed string `json:"what_it_will_not_fix,omitempty"`
	Ask      string `json:"what_to_ask_the_merchant,omitempty"`
	Provider string `json:"provider,omitempty"`
	Rejected string `json:"rejected,omitempty"`
}

// ClaimDiagnosis is the model's reading of a failed claim check.
//
// The check is arithmetic and precedes this. The diagnosis is the part that is
// reading rather than counting: the same failed check has three or four
// completely different causes and therefore three or four different remedies,
// and telling them apart is the judgement a solver cannot make.
//
// Class comes from a closed vocabulary. Action and Effect are owned by the
// system and looked up from the class, never authored by the model, because a
// remedy is an instruction somebody will follow.
type ClaimDiagnosis struct {
	Class     string `json:"defect_class"`
	Rationale string `json:"rationale"`
	Action    string `json:"remedy_action"`
	Effect    string `json:"remedy_effect"`
	Provider  string `json:"provider"`
}

// ClaimCheck records the verification of an externally supplied mapping.
//
// It exists because deriving a batch and checking a claimed one are different
// problems with different costs, and only the first is hard. See
// pipeline.CheckClaim.
type ClaimCheck struct {
	Source      string       `json:"source"`
	Verdict     ClaimVerdict `json:"verdict"`
	ClaimedSize int          `json:"claimed_size"`

	SumPaise      int64 `json:"claimed_sum_paise"`
	TargetPaise   int64 `json:"target_paise"`
	ResidualPaise int64 `json:"residual_paise"`

	ZeroContribution int      `json:"zero_contribution_records,omitempty"`
	Missing          []string `json:"named_but_absent,omitempty"`
	Unjoined         []string `json:"named_but_in_an_unjoined_feed,omitempty"`
	Findings         []string `json:"findings,omitempty"`
	Note             string   `json:"note"`

	// Diagnosis is why the check failed, where a model was asked. Absent on a
	// consistent claim, because there is nothing to diagnose.
	Diagnosis *ClaimDiagnosis `json:"diagnosis,omitempty"`
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

	// ReportClaim is the verification of the gateway's own stated mapping,
	// where one is available. It is computed by a separate entry point AFTER
	// the reconstruction above reached its own conclusion, and the search
	// never sees it.
	ReportClaim *ClaimCheck `json:"report_claim,omitempty"`

	// AnalystNote is the drafted, sendable version of this settlement's
	// remedy. Prose only; every figure is substituted from this receipt.
	AnalystNote *DraftedNote `json:"analyst_note,omitempty"`

	// ExceptionCostINR is what clearing this exception is estimated to cost,
	// and ExceptionMinutes is the handling estimate behind it.
	//
	// These vary by what clearing actually takes. A flat per-exception price
	// was simpler and it was also useless: every row costing the same makes
	// "sort the queue by cost" sort by nothing, and a queue that cannot be
	// ordered is a list rather than a work plan.
	ExceptionCostINR int `json:"exception_cost_inr,omitempty"`
	ExceptionMinutes int `json:"exception_handling_minutes,omitempty"`
	// ExceptionBasis names every term that produced the estimate, so an
	// operations lead can disagree with the model rather than with a number.
	ExceptionBasis []string `json:"exception_cost_basis,omitempty"`

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
