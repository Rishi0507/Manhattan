// Package agent holds the four jobs a language model is trusted with, and
// the machinery that keeps it outside every decision.
//
// The boundary is usually described as a set of restrictions. That framing
// undersells it. The boundary exists so the model can be given the *harder*
// jobs, the ones involving ambiguity, natural language and world knowledge,
// without any of that ambiguity reaching the ledger. A model that is never
// allowed near a decision can be pointed at problems a cautious system would
// otherwise have to refuse outright.
//
// The model does five things here: it reads a free-text bank narration into
// typed fields; it proposes what class of event explains an unexplained
// residual; it orders the constraints worth relaxing; it answers questions
// over the receipt store; and it renders results into English. Every one of
// those is open-ended judgement, and every one of them is something a solver
// cannot do. Note what they have in common: none of them is arithmetic.
//
// The one move it cannot make is decide.
package agent

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/Rishi0507/manhattan/internal/evidence"
	"github.com/Rishi0507/manhattan/internal/llm"
	"github.com/Rishi0507/manhattan/internal/model"
	"github.com/Rishi0507/manhattan/internal/money"
	"github.com/Rishi0507/manhattan/internal/pipeline"
)

// HypothesisKind is the closed vocabulary the resolution agent may propose
// from. It is closed on purpose: free text cannot be executed, cannot be
// verified, and cannot be counted, so an agent that returns prose has
// produced something no downstream stage can act on.
type HypothesisKind string

const (
	KindChargebackDebit  HypothesisKind = "CHARGEBACK_DEBIT"
	KindLateRefund       HypothesisKind = "LATE_REFUND"
	KindBankFee          HypothesisKind = "BANK_FEE"
	KindPartialCapture   HypothesisKind = "PARTIAL_CAPTURE"
	KindMissingTxn       HypothesisKind = "MISSING_TXN"
	KindFXAdjustment     HypothesisKind = "FX_ADJUSTMENT"
	KindRoundingMismatch HypothesisKind = "ROUNDING_CONVENTION_MISMATCH"
	KindAdjustment       HypothesisKind = "ADJUSTMENT"
)

// AllKinds is the vocabulary, in the order it is presented to the model.
var AllKinds = []HypothesisKind{
	KindChargebackDebit, KindLateRefund, KindBankFee, KindPartialCapture,
	KindMissingTxn, KindFXAdjustment, KindRoundingMismatch, KindAdjustment,
}

// Effect is how a hypothesis is applied to the inputs.
type Effect string

const (
	// EffectAddItem adds a record to the candidate pool, where the residual
	// is explained by something that belongs in the batch but was not in the
	// data the pipeline was given.
	EffectAddItem Effect = "add_item"
	// EffectAdjustTarget changes the credit being reconstructed, where the
	// residual is explained by something levied against the payout rather
	// than against the batch.
	EffectAdjustTarget Effect = "adjust_target"
)

// Hypothesis is one structured, executable proposal.
type Hypothesis struct {
	Kind        HypothesisKind `json:"kind"`
	AmountPaise int64          `json:"amount_paise"`
	Effect      Effect         `json:"effect"`
	Rationale   string         `json:"rationale"`
}

type hypothesisAnswer struct {
	Hypotheses []Hypothesis `json:"hypotheses"`
}

// Resolver runs the bounded hypothesis loop over an unresolved settlement.
type Resolver struct {
	Provider      llm.Provider
	MaxIterations int
	MaxHypotheses int
}

// NewResolver returns a resolver with the shipped bounds.
func NewResolver(p llm.Provider) *Resolver {
	return &Resolver{Provider: p, MaxIterations: 4, MaxHypotheses: 3}
}

const resolveSystem = `You are the resolution stage of an audited payment settlement reconciler.

A bank credit could not be reconstructed from the candidate records. You are given
the exact arithmetic residual: the gap in paise between the nearest achievable sum
and the credit. Your job is to name what CLASS of financial event most plausibly
explains a gap of that size and sign, drawing on how Indian payment settlement
actually works.

You are not being asked whether you are right. Every hypothesis you emit is applied
to the candidate pool and the full verification stack is re-run over it unchanged:
the amount-entropy gate, the feasibility gate, an exhaustive subset enumeration with
a uniqueness count, the pool-completeness guards, and an independent re-derivation of
the accounting identity. A hypothesis that does not make the identity close exactly,
and close uniquely, is discarded. You cannot cause a wrong posting, so propose the
explanations you actually think are most likely rather than the safest-sounding ones.

Domain notes that matter for ranking:
- A negative residual on a settlement is most often a chargeback: the disputed amount
  debited back plus a flat dispute-handling fee, so the total is a round-ish amount
  plus a small fixed component.
- Refunds settle in the cycle they clear, which is not always the cycle the original
  payment settled in, so a refund that cleared late nets off a batch it is not listed in.
- Sponsor bank charges are deducted from the payout, not from the batch, so they move
  the target rather than the pool.
- MDR is not returned when a payment is refunded, so a fully refunded payment
  contributes a negative amount equal to the retained fee and its GST.
- GST on merchant discount rate is 18 percent in India.

Rank your hypotheses most likely first. Give the amount in integer paise.`

// Resolve works an unresolved settlement.
//
// The loop is: extract the residual, ask for hypotheses, look for a real
// record that would justify each one, apply it, and re-verify. It is bounded
// at MaxIterations and there is no path by which it can run unattended for
// longer.
func (r *Resolver) Resolve(
	ctx context.Context,
	eng *pipeline.Engine,
	credit model.BankCredit,
	rec *evidence.Receipt,
) (*evidence.Receipt, llm.Usage) {
	var usage llm.Usage

	if rec.Status != evidence.StatusUnresolved || rec.Solver == nil || rec.Solver.NearestMiss == nil {
		return rec, usage
	}

	residual := rec.Solver.NearestMiss.Sum - rec.TargetPaise
	rec.Agent.Invoked = true
	rec.Agent.Provider = r.Provider.Name()

	for iter := 1; iter <= r.MaxIterations; iter++ {
		rec.Agent.Iterations = iter

		res, err := r.Provider.Structured(ctx, llm.Request{
			Role:       llm.RoleResolve,
			System:     resolveSystem,
			User:       residualPrompt(rec, credit, residual),
			SchemaName: "propose_hypotheses",
			SchemaDesc: "Propose ranked, typed explanations for an unexplained settlement residual.",
			Schema:     hypothesisSchema(r.MaxHypotheses),
			MaxTokens:  2048,
		})
		if err != nil {
			rec.Agent.Note = "the resolution agent was unavailable: " + err.Error()
			return rec, usage
		}
		usage.Add(res.Usage)

		var answer hypothesisAnswer
		if err := res.Into(&answer); err != nil {
			rec.Agent.Note = "the resolution agent returned an answer outside its own schema: " + err.Error()
			return rec, usage
		}

		for _, h := range answer.Hypotheses {
			if len(rec.Agent.Hypotheses) >= r.MaxIterations*r.MaxHypotheses {
				break
			}
			eh := evidence.Hypothesis{
				Kind:      string(h.Kind),
				Amount:    money.Paise(h.AmountPaise),
				Effect:    string(h.Effect),
				Rationale: h.Rationale,
			}

			// The citation search is deterministic and is not the model's to
			// make. The agent contributes the judgement about what CLASS of
			// event to look for. The system contributes the evidence that such
			// an event actually occurred, and it tries every candidate of that
			// class against the verifier rather than only the one whose amount
			// the model happened to guess exactly.
			//
			// That division matters more than it looks. A residual is rarely
			// the clean size of the thing that caused it: the nearest
			// achievable sum is whatever the pool can reach, which is not
			// necessarily the true batch minus one record. Filtering candidates
			// on the model's guessed amount would make the whole loop hostage
			// to arithmetic the model was never asked to do, and it would fail
			// exactly when the pool is dense enough to have a closer near-miss.
			candidates := findCitations(eng.Unjoined, h, credit)

			var trial *evidence.Receipt
			for _, src := range candidates {
				t := eng.ReconcileWith(credit, overlayForRecord(src))
				if t.Status == evidence.StatusVerified {
					eh.SourceRef = src.ID
					eh.Amount = src.Contribution.Abs()
					eh.Evidence = fmt.Sprintf("%s in the %s feed, dated %s, contributing %s",
						src.ID, feedName(src.Kind), src.EventAt.Format("2006-01-02"), src.Contribution)
					trial = t
					break
				}
			}
			if trial == nil {
				// Nothing real closes it. The bare arithmetic claim is still
				// tested, because an uncited hypothesis that closes exactly is
				// worth putting in front of an analyst. It can never post.
				trial = eng.ReconcileWith(credit, overlayFor(h, credit, ""))
			}

			switch {
			case trial.Status != evidence.StatusVerified:
				eh.Outcome = "rejected: " + string(trial.Status) + "; the identity did not close uniquely under this hypothesis"
				rec.Agent.Hypotheses = append(rec.Agent.Hypotheses, eh)

			case !eh.Citable():
				// This is the posting rule, and it is the whole safety
				// argument for letting an agent touch an exception queue.
				// The arithmetic closed, but nothing in any feed corroborates
				// that the proposed event actually happened. An unsupported
				// story that happens to add up is not evidence, so this can
				// route to an analyst with a named, arithmetically consistent
				// suggestion and it can never post.
				eh.Outcome = "arithmetically consistent but uncited; routed to review and never posted"
				rec.Agent.Hypotheses = append(rec.Agent.Hypotheses, eh)
				rec.Remediation = append(rec.Remediation, evidence.Remediation{
					Action: fmt.Sprintf("confirm whether a %s of %s applies to this settlement",
						strings.ToLower(strings.ReplaceAll(string(h.Kind), "_", " ")), money.Paise(h.AmountPaise)),
					Effect: "the reconstruction closes exactly and uniquely under this assumption, but no record corroborates it",
				})

			default:
				eh.Outcome = "accepted: the identity closes uniquely and the hypothesis cites a real record"
				rec.Agent.Hypotheses = append(rec.Agent.Hypotheses, eh)
				accepted := eh
				rec.Agent.Accepted = &accepted

				// The trial receipt is the real one: it was produced by the
				// unmodified verifier over the modified inputs. The agent
				// block is carried across so the receipt records how the
				// answer was arrived at.
				agentBlock := rec.Agent
				remediation := rec.Remediation
				trial.Agent = agentBlock
				trial.Agent.Note = "resolved by an agent hypothesis; the verification stack was re-run unmodified over the amended pool"
				trial.Remediation = remediation
				trial.AddFlag(evidence.FlagResolvedByHypothesis)
				trial.Claim = fmt.Sprintf(
					"%s, and the missing record was found in the %s feed and cited as %s",
					trial.Claim, feedName(model.KindChargeback), eh.SourceRef)
				return trial, usage
			}
		}

		// Nothing worked. Without a new residual to reason about, asking the
		// same question again would produce the same answer, so the loop
		// stops rather than burning its remaining iterations.
		break
	}

	rec.Agent.Note = fmt.Sprintf(
		"%d hypotheses were proposed and every one was rejected by the verifier; the residual of %s remains unexplained",
		len(rec.Agent.Hypotheses), residual)
	return rec, usage
}

// overlayFor turns a hypothesis into a concrete edit to the inputs.
func overlayFor(h Hypothesis, credit model.BankCredit, sourceRef string) *pipeline.Overlay {
	amt := money.Paise(h.AmountPaise)
	id := sourceRef
	if id == "" {
		id = fmt.Sprintf("hyp_%s_%d", strings.ToLower(string(h.Kind)), amt)
	}

	switch h.Effect {
	case EffectAdjustTarget:
		return &pipeline.Overlay{TargetDelta: -amt, Provenance: string(h.Kind)}
	default:
		// Sign follows the kind. A chargeback or a late refund reduces the
		// settlement; a missing transaction increases it.
		contribution := -amt
		if h.Kind == KindMissingTxn || h.Kind == KindPartialCapture {
			contribution = amt
		}
		return &pipeline.Overlay{
			Provenance: string(h.Kind),
			ExtraRecords: []model.Record{{
				ID:           id,
				Kind:         kindToRecordKind(h.Kind),
				MerchantID:   credit.MerchantID,
				Currency:     credit.Currency,
				EventAt:      credit.ValueDate.Add(-24 * time.Hour),
				Contribution: contribution,
				Lo:           contribution,
				Hi:           contribution,
				Chargeback:   negIfChargeback(h.Kind, amt),
				Refund:       refundPart(h.Kind, amt),
			}},
		}
	}
}

func kindToRecordKind(k HypothesisKind) model.RecordKind {
	switch k {
	case KindChargebackDebit:
		return model.KindChargeback
	case KindLateRefund:
		return model.KindRefund
	case KindMissingTxn, KindPartialCapture:
		return model.KindPayment
	}
	return model.KindAdjustment
}

func negIfChargeback(k HypothesisKind, amt money.Paise) money.Paise {
	if k == KindChargebackDebit {
		return amt
	}
	return 0
}

func refundPart(k HypothesisKind, amt money.Paise) money.Paise {
	if k == KindLateRefund {
		return amt
	}
	return 0
}

// maxCitationCandidates bounds how many real records are tried per
// hypothesis, so the loop stays bounded end to end: iterations times
// hypotheses times candidates, every factor configured.
const maxCitationCandidates = 5

// findCitations returns the records in the unjoined feeds that could
// corroborate this class of hypothesis, ranked by how close their amount is
// to the one proposed.
//
// The filter is on merchant, record class and date proximity, all of which
// are facts about the data. The proposed amount is used only to rank and
// never to exclude, so a model that names the right kind of event and the
// wrong number still gets the citation it deserves, while a model that names
// the wrong kind of event gets nothing however good its arithmetic was.
func findCitations(feed []model.Record, h Hypothesis, credit model.BankCredit) []model.Record {
	want := money.Paise(h.AmountPaise)
	kind := kindToRecordKind(h.Kind)

	var out []model.Record
	for _, r := range feed {
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
	sort.Slice(out, func(i, j int) bool {
		return (out[i].Contribution.Abs() - want).Abs() < (out[j].Contribution.Abs() - want).Abs()
	})
	if len(out) > maxCitationCandidates {
		out = out[:maxCitationCandidates]
	}
	return out
}

// overlayForRecord applies a real, cited record to the pool. Its amount, date
// and sign come from the record itself rather than from anything the model
// said, so the model cannot influence the arithmetic even indirectly.
func overlayForRecord(src model.Record) *pipeline.Overlay {
	return &pipeline.Overlay{
		Provenance:   "cited:" + src.ID,
		ExtraRecords: []model.Record{src},
	}
}

func feedName(k model.RecordKind) string {
	switch k {
	case model.KindChargeback:
		return "disputes"
	case model.KindRefund:
		return "refunds"
	}
	return "adjustments"
}

// residualPrompt is the volatile half of the request. It carries only what
// the model needs to reason about the gap, and no candidate pool: putting
// hundreds of records into the context window is what makes a fuzzy matcher's
// per-settlement cost scale with merchant size.
func residualPrompt(rec *evidence.Receipt, credit model.BankCredit, residual money.Paise) string {
	var b strings.Builder
	fmt.Fprintf(&b, "SETTLEMENT %s\n", rec.SettlementRef)
	fmt.Fprintf(&b, "bank narration: %s\n", credit.Narration)
	fmt.Fprintf(&b, "value date: %s\n", rec.ValueDate)
	fmt.Fprintf(&b, "merchant archetype: %s\n\n", rec.Archetype)

	fmt.Fprintf(&b, "credit to reconstruct: %s\n", rec.TargetPaise)
	fmt.Fprintf(&b, "nearest achievable sum: %s at cardinality %d\n",
		rec.Solver.NearestMiss.Sum, rec.Solver.NearestMiss.Cardinality)
	fmt.Fprintf(&b, "RESIDUAL_PAISE=%d\n", int64(residual))
	fmt.Fprintf(&b, "residual in rupees: %s (%s)\n\n", residual,
		map[bool]string{true: "the achievable sum overshoots, so something was deducted that is not in the pool",
			false: "the achievable sum falls short, so something is missing from the pool"}[residual > 0])

	fmt.Fprintf(&b, "candidate pool: %d records, %d of them negative\n", rec.Pool.N, rec.Pool.SignedItems)
	fmt.Fprintf(&b, "search was exhaustive over %s\n", rec.Uniqueness.Scope)
	fmt.Fprintf(&b, "rounding mode: %s, tolerance %s per record\n", rec.Rounding.Mode, rec.Rounding.TolerancePaise)
	fmt.Fprintf(&b, "data mode: %s\n", rec.DataMode)
	return b.String()
}

// hypothesisSchema is the closed shape a valid answer must take.
func hypothesisSchema(max int) map[string]any {
	kinds := make([]string, len(AllKinds))
	for i, k := range AllKinds {
		kinds[i] = string(k)
	}
	sort.Strings(kinds)

	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"hypotheses": map[string]any{
				"type":        "array",
				"maxItems":    max,
				"description": "Ranked explanations, most likely first.",
				"items": map[string]any{
					"type":                 "object",
					"additionalProperties": false,
					"required":             []string{"kind", "amount_paise", "effect", "rationale"},
					"properties": map[string]any{
						"kind": map[string]any{
							"type": "string", "enum": kinds,
							"description": "The class of financial event proposed.",
						},
						"amount_paise": map[string]any{
							"type":        "integer",
							"description": "Magnitude in integer paise. Never a decimal.",
						},
						"effect": map[string]any{
							"type": "string", "enum": []string{string(EffectAddItem), string(EffectAdjustTarget)},
							"description": "add_item puts a record into the candidate pool; adjust_target changes the credit being reconstructed.",
						},
						"rationale": map[string]any{
							"type":        "string",
							"description": "One sentence on why this event would produce a residual of this size and sign.",
						},
					},
				},
			},
		},
		"required": []string{"hypotheses"},
	}
}
