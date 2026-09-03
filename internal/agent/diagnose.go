package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Rishi0507/manhattan/internal/evidence"
	"github.com/Rishi0507/manhattan/internal/llm"
	"github.com/Rishi0507/manhattan/internal/money"
)

// Diagnostician turns a failed arithmetic check into a diagnosis and an action.
//
// This is the model doing the job a human does, on the population where the
// system finally has volume to give it. Two of these are worth separating:
//
// The CHECK is arithmetic and the model has no vote in it. Whether the
// settlement report's stated mapping sums to the credit, whether it names a
// record that does not exist, whether its count matches its own declaration:
// all computed, all before this call, all unchanged by whatever comes back.
//
// The DIAGNOSIS is reading. A report short by one record with a residual equal
// to a single chargeback, a report naming a payment that settled last cycle,
// and a report truncated by a partial write all produce the same failed check
// and three completely different remedies. Telling them apart means looking at
// the shape of the residual against the class of records involved, which is
// exactly the judgement a solver cannot make and an operations analyst makes
// in ten seconds.
//
// So the model is given the arithmetic and asked what it means. Its answer is
// forced into a closed vocabulary of defect classes, and the remedy attached
// to each class is fixed in this file rather than authored by the model, which
// is what stops a fluent diagnosis from becoming a fluent instruction.
type Diagnostician struct {
	Provider llm.Provider
}

// NewDiagnostician returns a diagnostician over one provider.
func NewDiagnostician(p llm.Provider) *Diagnostician {
	return &Diagnostician{Provider: p}
}

// DefectClass names why a stated mapping failed its check.
type DefectClass string

const (
	// DefectOmittedDispute is the documented one. A dispute is raised against
	// the original transaction and debited in whatever cycle the network
	// resolves it in, which is routinely not the cycle that carried the
	// payment, so a report whose own join is by capture date has a structural
	// reason to omit a debit that genuinely moved money.
	DefectOmittedDispute DefectClass = "OMITTED_DISPUTE"
	// DefectCrossCycle is the same timetable in the other direction: the
	// report names a record that belongs to an adjacent cycle.
	DefectCrossCycle DefectClass = "CROSS_CYCLE_MEMBER"
	// DefectTruncated is not a payments phenomenon at all. It is what a
	// partial write or a truncated file looks like downstream.
	DefectTruncated DefectClass = "TRUNCATED_MAPPING"
	// DefectFeePolicy is the one that is most often OUR fault rather than the
	// report's: the membership is right and the arithmetic disagrees, which
	// usually means the fee policy configured here does not match the one
	// actually applied.
	DefectFeePolicy DefectClass = "FEE_POLICY_MISMATCH"
	// DefectUnknown is the honest answer and it is in the vocabulary on
	// purpose. A diagnosis vocabulary with no escape hatch forces a guess.
	DefectUnknown DefectClass = "UNDIAGNOSED"
)

// AllDefects is the closed vocabulary offered to the model.
var AllDefects = []DefectClass{
	DefectOmittedDispute, DefectCrossCycle, DefectTruncated,
	DefectFeePolicy, DefectUnknown,
}

// remedies are fixed per class rather than authored by the model.
//
// The model says what happened. What to do about it is a policy decision this
// system owns, because a remedy is an instruction somebody will follow and a
// fluent instruction from a model that was only asked to diagnose is exactly
// the sort of authority creep this whole design refuses.
var remedies = map[DefectClass]struct{ action, effect string }{
	DefectOmittedDispute: {
		action: "join the disputes feed and re-run this settlement",
		effect: "the debit is already available to this run; connecting it lets both the " +
			"reconstruction and the report's own claim be checked against the whole batch",
	},
	DefectCrossCycle: {
		action: "raise the named record with the gateway as a cycle-boundary discrepancy",
		effect: "the record is real and belongs to an adjacent settlement, so posting this " +
			"mapping unchanged would double-count it across two cycles",
	},
	DefectTruncated: {
		action: "re-fetch the settlement report for this reference",
		effect: "the stated mapping is internally inconsistent with its own declared count, " +
			"which is what a partial transfer looks like rather than a reconciliation problem",
	},
	DefectFeePolicy: {
		action: "confirm the fee schedule configured for this merchant",
		effect: "the membership the report states is consistent and the arithmetic is not, " +
			"which points at the policy on this side rather than at the report",
	},
	DefectUnknown: {
		action: "route to an analyst with the residual and the failed checks attached",
		effect: "the check is exact and its cause is not established, so the honest output " +
			"is the evidence rather than a diagnosis",
	},
}

const diagnoseSystem = `You diagnose why a payment gateway's settlement report failed an arithmetic check.

The check has already run. You are given its exact output and you do not get to
disagree with it: the residual, the records named but absent, the count mismatch and
the class of every record involved are all computed facts.

Your job is the part that is reading rather than counting. Name which defect class
explains what you were shown.

OMITTED_DISPUTE      the report leaves out a debit that moved money in this cycle.
                     Disputes are the usual cause: they are raised against the
                     original transaction and debited when the network resolves
                     them, which is often a different cycle. Look for a residual
                     whose sign and magnitude match a single chargeback, or for a
                     chargeback among the records named but absent.
CROSS_CYCLE_MEMBER   the report names a record that belongs to an adjacent cycle.
                     Look for a named record that exists and is already reconciled,
                     or a residual that matches one whole payment with the wrong sign.
TRUNCATED_MAPPING    the report contradicts its OWN declared transaction count. That
                     is not a payments phenomenon, it is a partial file.
FEE_POLICY_MISMATCH  the membership looks right and the arithmetic is off by a small
                     proportion of gross rather than by a whole record. A residual
                     that is a fraction of a per-transaction fee points here.
UNDIAGNOSED          nothing above fits. Say so. An unexplained exact residual with
                     the evidence attached is more useful than a confident guess, and
                     this vocabulary has an escape hatch so you never need to force one.

You do not choose the remedy. Each class has one already, owned by the system.`

// Diagnose classifies a contradicted claim.
//
// Returns nil when there is nothing to diagnose, which keeps the caller from
// spending a call to be told that a consistent claim is consistent.
func (d *Diagnostician) Diagnose(
	ctx context.Context, r *evidence.Receipt,
) (*evidence.ClaimDiagnosis, llm.Usage, error) {
	var usage llm.Usage
	if r.ReportClaim == nil || r.ReportClaim.Verdict != evidence.ClaimContradicted {
		return nil, usage, nil
	}

	res, err := d.Provider.Structured(ctx, llm.Request{
		Role:       llm.RoleTriage,
		System:     diagnoseSystem,
		User:       renderClaimFailure(r),
		SchemaName: "diagnose_report_defect",
		Schema:     diagnoseSchema(),
	})
	if err != nil {
		return nil, usage, err
	}
	usage.Add(res.Usage)

	var out struct {
		Class     string `json:"defect_class"`
		Rationale string `json:"rationale"`
	}
	if err := json.Unmarshal(res.JSON, &out); err != nil {
		return nil, usage, err
	}

	class := DefectClass(out.Class)
	rem, ok := remedies[class]
	if !ok {
		// An answer outside the vocabulary is a transport failure rather than
		// something to interpret. Strict forced tool use makes it very
		// unlikely and it is still handled, because a schema is a contract
		// and a contract nobody checks is a comment.
		class, rem = DefectUnknown, remedies[DefectUnknown]
	}
	return &evidence.ClaimDiagnosis{
		Class:     string(class),
		Rationale: strings.TrimSpace(out.Rationale),
		Action:    rem.action,
		Effect:    rem.effect,
		Provider:  d.Provider.Name(),
	}, usage, nil
}

// renderClaimFailure gives the model the arithmetic and nothing else.
func renderClaimFailure(r *evidence.Receipt) string {
	c := r.ReportClaim
	var b strings.Builder
	fmt.Fprintf(&b, "SETTLEMENT %s (%s, %s)\n", r.SettlementRef, r.Archetype, r.MerchantID)
	fmt.Fprintf(&b, "credit: %s\n", r.TargetPaise)
	fmt.Fprintf(&b, "the report names %d records summing to %s\n",
		c.ClaimedSize, money.Paise(c.SumPaise))
	fmt.Fprintf(&b, "RESIDUAL_PAISE=%d (%s)\n", c.ResidualPaise, money.Paise(c.ResidualPaise))
	if n := r.Feasibility.DeclaredTxnCount; n != nil {
		fmt.Fprintf(&b, "the report separately declares %d transactions\n", *n)
	}
	if len(c.Missing) > 0 {
		fmt.Fprintf(&b, "named but absent from this merchant's data entirely: %v\n", c.Missing)
	}
	if len(c.Unjoined) > 0 {
		fmt.Fprintf(&b, "named and present only in an unjoined feed: %v\n", c.Unjoined)
	}
	for _, f := range c.Findings {
		fmt.Fprintf(&b, "finding: %s\n", f)
	}
	fmt.Fprintf(&b, "\nfor scale: the pool holds %d candidates with a contribution spread of %s\n",
		r.Pool.N, money.Paise(int64(r.Pool.SigmaPaise)))
	if r.Solver != nil && r.Solver.NearestMiss != nil && r.Solver.NearestMiss.Valid {
		fmt.Fprintf(&b, "the independent reconstruction's nearest achievable sum is %s away\n",
			money.Paise(r.Solver.NearestMiss.Gap))
	}
	fmt.Fprintf(&b, "\nWhich defect class explains this?\n")
	return b.String()
}

func diagnoseSchema() map[string]any {
	classes := make([]string, len(AllDefects))
	for i, c := range AllDefects {
		classes[i] = string(c)
	}
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"defect_class": map[string]any{
				"type": "string", "enum": classes,
				"description": "The class that explains the failed check. UNDIAGNOSED when none fits.",
			},
			"rationale": map[string]any{
				"type": "string",
				"description": "One sentence citing the specific figure that led you there: " +
					"the residual, a named-but-absent id, or the count mismatch.",
			},
		},
		"required":             []string{"defect_class", "rationale"},
		"additionalProperties": false,
	}
}
