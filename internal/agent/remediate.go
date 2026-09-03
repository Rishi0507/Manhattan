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

// Drafter turns a computed remedy into something an operations lead can send.
//
// This is the highest-volume place in the system where the model does work
// nobody else can, and it exists because the review that found it was right: a
// proven cure was the loop's most valuable output and there was no workflow
// around it at all. It sat on a receipt as two structured strings.
//
// The division of labour is the same one everywhere else here, and the reason
// it is safe is worth stating precisely rather than assumed:
//
//	The FACTS are computed and supplied. Which change, what the pool becomes,
//	what the collision index becomes, whether the identity closes, what the
//	residual is, what class of defect was diagnosed. Every one of those is
//	arithmetic that has already run, and the model receives them rather than
//	sourcing them.
//
//	The WRITING is the model's. Which of those facts an analyst needs first,
//	what the change will not fix, what to ask the merchant for, and how to say
//	it in three sentences somebody will actually read.
//
// A drafted note can never change a posting. It is attached to a settlement
// that is already held, and holding is the outcome either way. So the worst a
// bad draft costs is a confusing sentence in a work queue, which is a real
// cost and a bounded one, and it is the only category of model output in this
// repository whose failure mode is embarrassment rather than a wrong ledger.
//
// The schema forbids the model from inventing figures: it returns prose fields
// only, and every number in the rendered note is substituted from the receipt
// afterwards.
type Drafter struct {
	Provider llm.Provider
}

// NewDrafter returns a drafter over one provider.
func NewDrafter(p llm.Provider) *Drafter { return &Drafter{Provider: p} }

const draftSystem = `You write the note an operations analyst reads next to a held settlement.

Every fact you are given has already been computed and verified. You are not being
asked whether the remedy works; that was established by re-running the entire
verification stack over the amended inputs. You are being asked to turn it into
something a person can act on.

Write for a finance operations lead who has forty of these in a queue this morning.
They need, in this order:

1. What to do. One imperative sentence.
2. Why it will work, in terms of what was actually measured.
3. What it will NOT fix. This is the part analysts most need and most rarely get,
   and leaving it out is how a remedy becomes a false promise.

Rules that matter:

Do not invent numbers. Not one. Every figure in the final note is substituted from
the receipt after you write, so a number you type is a number that will be wrong.
Refer to quantities in words: "the pool falls by about two thirds", "the residual is
a single chargeback-sized debit".

Do not promise a posting. Most of these remedies are changes to a filter or a data
source, and a filter change is an assertion about the merchant that a human has to
confirm. Say what was verified, not what will happen.

Do not pad. Three sentences. An analyst skimming a queue reads the first eight words
of each entry and nothing else if those eight words do not earn it.`

// Draft writes the analyst-facing note for one held settlement.
//
// Returns nil when there is nothing worth drafting, which keeps the caller
// from paying for a note that says "no remedy is available" more elaborately
// than the receipt already does.
func (d *Drafter) Draft(
	ctx context.Context, r *evidence.Receipt,
) (*evidence.DraftedNote, llm.Usage, error) {
	var usage llm.Usage
	if r.Status.Postable() || !worthDrafting(r) {
		return nil, usage, nil
	}

	res, err := d.Provider.Structured(ctx, llm.Request{
		Role:       llm.RoleRemediate,
		System:     draftSystem,
		User:       renderForDraft(r),
		SchemaName: "draft_remediation_note",
		Schema:     draftSchema(),
	})
	if err != nil {
		return nil, usage, err
	}
	usage.Add(res.Usage)

	var out struct {
		Do       string `json:"what_to_do"`
		Because  string `json:"why_it_works"`
		NotFixed string `json:"what_it_will_not_fix"`
		Ask      string `json:"what_to_ask_the_merchant,omitempty"`
	}
	if err := json.Unmarshal(res.JSON, &out); err != nil {
		return nil, usage, err
	}

	note := &evidence.DraftedNote{
		Do:       strings.TrimSpace(out.Do),
		Because:  strings.TrimSpace(out.Because),
		NotFixed: strings.TrimSpace(out.NotFixed),
		Ask:      strings.TrimSpace(out.Ask),
		Provider: d.Provider.Name(),
	}
	// The one guard that makes the no-invented-numbers rule enforceable rather
	// than advisory. A drafted note that smuggled in a figure is rejected
	// wholesale rather than published with a wrong number in it.
	if n := digitsIn(note.Do + note.Because + note.NotFixed + note.Ask); n > 0 {
		note.Rejected = fmt.Sprintf(
			"the draft contained %d numeric character(s) and the schema forbids figures, "+
				"because every number in this note is substituted from the receipt. "+
				"Dropped rather than published", n)
		note.Do, note.Because, note.NotFixed, note.Ask = "", "", "", ""
	}
	return note, usage, nil
}

// worthDrafting is the deterministic screen, the same principle as agent
// triage: do not pay a model to restate a receipt.
//
// A note is worth writing when there is something specific to act on, which
// means a computed remediation, a diagnosed report defect, or a proven cure.
// A bare UNDERDETERMINED with no remedy has nothing to say that the status
// does not already say.
func worthDrafting(r *evidence.Receipt) bool {
	if len(r.Remediation) > 0 {
		return true
	}
	return r.ReportClaim != nil && r.ReportClaim.Diagnosis != nil
}

func renderForDraft(r *evidence.Receipt) string {
	var b strings.Builder
	fmt.Fprintf(&b, "SETTLEMENT %s, held as %s\n", r.SettlementRef, r.Status)
	fmt.Fprintf(&b, "merchant type: %s\n", r.Archetype)
	fmt.Fprintf(&b, "why it is held: %s\n\n", r.Claim)

	fmt.Fprintf(&b, "POOL AND GATE\n")
	fmt.Fprintf(&b, "  %d candidates after narrowing, spread %s\n",
		r.Pool.N, money.Paise(int64(r.Pool.SigmaPaise)))
	fmt.Fprintf(&b, "  twin mass %.2f against a threshold of %.2f\n",
		r.AmountEntropy.TwinMass, r.AmountEntropy.TwinMassThreshold)
	fmt.Fprintf(&b, "  collision index %.3g at k*=%d\n\n",
		r.Feasibility.IndexAtKStar, r.Feasibility.KStar)

	if r.Uniqueness != nil {
		fmt.Fprintf(&b, "SEARCH\n  %d reconstructions found, %d rivals, scope: %s\n\n",
			r.Uniqueness.MatchesFound, r.Uniqueness.RivalsFound, r.Uniqueness.Scope)
	}
	if r.Solver != nil && r.Solver.NearestMiss != nil && r.Solver.NearestMiss.Valid {
		fmt.Fprintf(&b, "  nearest achievable sum is %s from the credit, at cardinality %d\n\n",
			money.Paise(r.Solver.NearestMiss.Gap), r.Solver.NearestMiss.Cardinality)
	}

	if c := r.ReportClaim; c != nil {
		fmt.Fprintf(&b, "THE GATEWAY REPORT'S OWN CLAIM: %s\n  %s\n", c.Verdict, c.Note)
		if c.Diagnosis != nil {
			fmt.Fprintf(&b, "  diagnosed as %s: %s\n", c.Diagnosis.Class, c.Diagnosis.Rationale)
			fmt.Fprintf(&b, "  the system's remedy for that class: %s\n", c.Diagnosis.Action)
		}
		b.WriteString("\n")
	}

	if len(r.Remediation) > 0 {
		fmt.Fprintf(&b, "COMPUTED REMEDIES, each already re-verified\n")
		for _, rem := range r.Remediation {
			fmt.Fprintf(&b, "  action: %s\n  verified effect: %s\n", rem.Action, rem.Effect)
			if rem.ProjectedIndex != nil {
				fmt.Fprintf(&b, "  the collision index would become %.3g\n", *rem.ProjectedIndex)
			}
			if rem.ProjectedPoolN != nil {
				fmt.Fprintf(&b, "  the pool would become %d candidates\n", *rem.ProjectedPoolN)
			}
		}
		b.WriteString("\n")
	}
	if len(r.Agent.Steps) > 0 {
		fmt.Fprintf(&b, "WHAT WAS ALREADY TRIED: ")
		var tried []string
		for _, s := range r.Agent.Steps {
			tried = append(tried, s.Kind)
		}
		fmt.Fprintf(&b, "%s\n\n", strings.Join(tried, ", "))
	}

	fmt.Fprintf(&b, "Write the note. No figures.\n")
	return b.String()
}

func digitsIn(s string) int {
	n := 0
	for _, r := range s {
		if r >= '0' && r <= '9' {
			n++
		}
	}
	return n
}

func draftSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"what_to_do": map[string]any{
				"type":        "string",
				"description": "One imperative sentence. No figures.",
			},
			"why_it_works": map[string]any{
				"type": "string",
				"description": "One sentence, in terms of what was measured. " +
					"Quantities in words, never digits.",
			},
			"what_it_will_not_fix": map[string]any{
				"type":        "string",
				"description": "One sentence. The part analysts need and rarely get. No figures.",
			},
			"what_to_ask_the_merchant": map[string]any{
				"type":        "string",
				"description": "Optional. One short request, where the remedy needs the merchant.",
			},
		},
		"required":             []string{"what_to_do", "why_it_works", "what_it_will_not_fix"},
		"additionalProperties": false,
	}
}
