package agent

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/Rishi0507/manhattan/internal/evidence"
	"github.com/Rishi0507/manhattan/internal/llm"
	"github.com/Rishi0507/manhattan/internal/narrow"
)

// Citation is a pointer into the receipt store: which receipt, which field.
type Citation struct {
	ReceiptID string `json:"receipt_id"`
	Field     string `json:"field"`
	Value     string `json:"value,omitempty"`
}

// Answer is a grounded response to a question about the receipt store.
type Answer struct {
	Text       string     `json:"answer"`
	Citations  []Citation `json:"citations"`
	Answerable bool       `json:"answerable"`
	// Retrieved names the receipts that were put in front of the model, which
	// is different from the ones it cited and worth being able to audit
	// separately.
	Retrieved []string  `json:"retrieved"`
	Usage     llm.Usage `json:"-"`
}

// QA answers a finance team's questions from the evidence store.
//
// The evidence object is already a complete structured derivation: what was
// dropped and why, what was searched, what was found, what was ruled out, and
// what would be needed to do better. A finance team's real questions are
// answerable from those fields, and nobody wants to answer them by reading
// JSON.
//
// The constraints are the same ones that apply everywhere else here. Every
// claim carries a receipt id and a field path. Nothing is inferred beyond
// what a receipt records; "I do not have a receipt that says that" is a valid
// and expected answer. And it is read-only: it cannot re-run a
// reconciliation, change a status, or post anything. It can link to the
// remediation block that would change a status, which is the useful thing.
//
// This is also what an auditable evidence object is FOR. The question-answering
// agent is not a bolt-on; it is the reason the verifier writes down its
// reasoning rather than only its conclusion.
type QA struct {
	Provider llm.Provider
	Store    *evidence.Store
	// MaxReceipts bounds how many receipts enter the context window.
	MaxReceipts int
}

// NewQA returns a question-answering agent over a receipt store.
func NewQA(p llm.Provider, s *evidence.Store) *QA {
	return &QA{Provider: p, Store: s, MaxReceipts: 12}
}

const qaSystem = `You answer questions from a finance team about settlement reconciliations that have
already been decided. You are given the evidence receipts and nothing else.

Rules, in order of importance:

1. Every factual claim must come from a receipt field you were shown. Attach the receipt
   id and the field path to each claim, in the citations array.
2. If the receipts do not contain what the question asks for, say so plainly and set
   answerable to false. "I do not have a receipt that records that" is a correct and
   expected answer. Do not reason your way to a number that is not in front of you.
3. You cannot re-run a reconciliation, change a status, or post anything. If a receipt
   carries a remediation block, you may describe what it says would change the outcome.
4. Amounts are integer paise. Quote them in rupees with Indian digit grouping, and never
   perform arithmetic on them beyond adding up figures that are already in the receipts.

Vocabulary the receipts use:

  VERIFIED             exactly one reconstruction exists in the region searched, the
                       uniqueness count was exhaustive, and the accounting identity closes
  AMBIGUOUS            two or more reconstructions exist and both are exhibited
  UNDERDETERMINED      the combinatorics guarantee a large population of reconstructions,
                       so no witness is shown; the remedy is more data, not more search
  NARROWING_SENSITIVE  the answer depended on a filtering decision rather than on arithmetic
  UNRESOLVED           nothing reconstructs the credit within the declared tolerance

  collision index      the estimated number of distinct subsets that hit the target in the
                       searched region; above the configured threshold the system refuses
  k*                   the largest free cardinality the gate accepts, and the parameter the
                       solver is dispatched on
  twin mass            the fraction of the pool whose amounts are indistinguishable

Write in plain English for a finance lead, not for an engineer. Be brief.`

// Ask answers one question.
func (q *QA) Ask(ctx context.Context, question string) (Answer, error) {
	receipts := q.retrieve(question)

	var ids []string
	for _, r := range receipts {
		ids = append(ids, r.SettlementRef)
	}

	res, err := q.Provider.Structured(ctx, llm.Request{
		Role:       llm.RoleAnswer,
		System:     qaSystem,
		User:       q.context(question, receipts),
		SchemaName: "answer_from_receipts",
		SchemaDesc: "Answer a question using only the evidence receipts provided.",
		Schema:     answerSchema(),
		MaxTokens:  1500,
	})
	if err != nil {
		return Answer{}, err
	}
	var a Answer
	if err := res.Into(&a); err != nil {
		return Answer{}, err
	}
	a.Usage = res.Usage
	a.Retrieved = ids
	return a, nil
}

// retrieve selects the receipts relevant to a question.
//
// Retrieval is keyword and status based rather than semantic, which is
// adequate here for a reason worth noting: the corpus is small, highly
// structured, and the questions are about categories the receipts already
// carry as fields. An embedding index would be more machinery for the same
// answers.
func (q *QA) retrieve(question string) []*evidence.Receipt {
	all := q.Store.All()
	lower := strings.ToLower(question)

	type scored struct {
		r *evidence.Receipt
		s int
	}
	var out []scored

	for _, r := range all {
		s := 0
		if strings.Contains(lower, strings.ToLower(r.SettlementRef)) {
			s += 100
		}
		// A bare settlement number, as a finance lead would say it.
		if tail := lastSegment(r.SettlementRef); tail != "" && strings.Contains(lower, tail) {
			s += 80
		}
		if strings.Contains(lower, strings.ToLower(string(r.Status))) {
			s += 20
		}
		if r.MerchantName != "" && strings.Contains(lower, strings.ToLower(r.MerchantName)) {
			s += 30
		}
		if r.Archetype != "" && strings.Contains(lower, strings.ReplaceAll(r.Archetype, "_", " ")) {
			s += 20
		}
		for _, f := range r.Flags {
			if strings.Contains(lower, strings.ToLower(strings.ReplaceAll(string(f), "_", " "))) {
				s += 25
			}
		}
		if strings.Contains(lower, "circular") && r.FeeCheck != nil && r.FeeCheck.Circular {
			s += 40
		}
		if (strings.Contains(lower, "fail") || strings.Contains(lower, "exception") ||
			strings.Contains(lower, "not post") || strings.Contains(lower, "didn't post") ||
			strings.Contains(lower, "backlog") || strings.Contains(lower, "cost")) && !r.Status.Postable() {
			s += 15
		}
		if strings.Contains(lower, "hardest") || strings.Contains(lower, "merchant") {
			s += 5
		}
		out = append(out, scored{r, s})
	}

	sort.SliceStable(out, func(i, j int) bool {
		if out[i].s != out[j].s {
			return out[i].s > out[j].s
		}
		return out[i].r.ExceptionCostINR > out[j].r.ExceptionCostINR
	})

	n := q.MaxReceipts
	if n <= 0 || n > len(out) {
		n = len(out)
	}
	res := make([]*evidence.Receipt, 0, n)
	for _, s := range out[:n] {
		res = append(res, s.r)
	}
	return res
}

func lastSegment(ref string) string {
	i := strings.LastIndex(ref, "_")
	if i < 0 || i+1 >= len(ref) {
		return ""
	}
	return ref[i+1:]
}

// context renders the retrieved receipts plus a run-level summary.
//
// Receipts are rendered as compact labelled lines rather than as raw JSON.
// The full evidence object carries fields no question needs, and paying for
// them on every call would make the question-answering agent the most
// expensive thing in the system.
func (q *QA) context(question string, receipts []*evidence.Receipt) string {
	var b strings.Builder

	if run := q.Store.Run(); run != nil {
		fmt.Fprintf(&b, "RUN %s: %d settlements, %d auto-posted, %d exceptions, %d auto-posted wrong.\n",
			run.RunID, run.Settlements, run.AutoPosted, run.Exceptions, run.AutoPostedWrong)
		if len(run.Drift) > 0 {
			fmt.Fprintf(&b, "RUN FLAG: narrowing drift on %s (observed %.2f against a baseline of %.2f); the batch is held.\n",
				run.Drift[0].Constraint, run.Drift[0].Observed, run.Drift[0].Baseline)
		}
		b.WriteString("\n")
	}

	// Aggregate drop counts, which is what "which constraint dropped the most
	// records" is actually asking about.
	agg := map[narrow.Constraint]int{}
	for _, r := range q.Store.All() {
		for c, n := range r.Narrowing.Dropped {
			agg[c] += n
		}
	}
	if len(agg) > 0 {
		type kv struct {
			c narrow.Constraint
			n int
		}
		var rows []kv
		for c, n := range agg {
			rows = append(rows, kv{c, n})
		}
		sort.Slice(rows, func(i, j int) bool { return rows[i].n > rows[j].n })
		b.WriteString("AGGREGATE NARROWING DROPS ACROSS THE WHOLE STORE (field: narrowing.dropped)\n")
		for _, r := range rows {
			fmt.Fprintf(&b, "  %-32s %d\n", r.c, r.n)
		}
		b.WriteString("\n")
	}

	b.WriteString("RECEIPTS\n")
	for _, r := range receipts {
		b.WriteString(renderReceipt(r))
		b.WriteString("\n")
	}

	fmt.Fprintf(&b, "\nQUESTION: %s\n", question)
	return b.String()
}

func renderReceipt(r *evidence.Receipt) string {
	var b strings.Builder
	fmt.Fprintf(&b, "--- receipt %s (merchant %s, archetype %s, value date %s)\n",
		r.SettlementRef, r.MerchantName, r.Archetype, r.ValueDate)
	fmt.Fprintf(&b, "  status: %s   flags: %v\n", r.Status, r.Flags)
	fmt.Fprintf(&b, "  target_paise: %d (%s)\n", int64(r.TargetPaise), r.TargetPaise)
	fmt.Fprintf(&b, "  pool.n: %d   pool.contribution_sigma_paise: %.0f   pool.signed_items: %d\n",
		r.Pool.N, r.Pool.SigmaPaise, r.Pool.SignedItems)
	fmt.Fprintf(&b, "  amount_entropy.twin_mass: %.3f   .distinct_contribution_values: %d   .gate: %v\n",
		r.AmountEntropy.TwinMass, r.AmountEntropy.DistinctValues, passWord(r.AmountEntropy.Pass))
	fmt.Fprintf(&b, "  feasibility.k_star: %d   .collision_index_at_k_star: %.4g   .decision: %s\n",
		r.Feasibility.KStar, r.Feasibility.IndexAtKStar, r.Feasibility.Decision)
	if r.Uniqueness != nil {
		fmt.Fprintf(&b, "  uniqueness.scope: %s (source: %s)   .matches_found: %d   .rivals_found: %d\n",
			r.Uniqueness.Scope, r.Uniqueness.ScopeSource, r.Uniqueness.MatchesFound, r.Uniqueness.RivalsFound)
	}
	if r.WitnessSize > 0 {
		fmt.Fprintf(&b, "  witness_size: %d\n", r.WitnessSize)
	}
	if r.Accounting != nil {
		fmt.Fprintf(&b, "  accounting.residual_paise: %d   .closes: %v\n",
			int64(r.Accounting.Residual), r.Accounting.Closes)
	}
	if r.FeeCheck != nil {
		fmt.Fprintf(&b, "  fee_check.mode: %s   .circular: %v   .delta_bps: %d   .band_bps: %d   .within_band: %v\n",
			r.FeeCheck.Mode, r.FeeCheck.Circular, r.FeeCheck.DeltaBps, r.FeeCheck.BandBps, r.FeeCheck.WithinBand)
	}
	if r.Agent.Invoked {
		fmt.Fprintf(&b, "  agent.invoked: true   .iterations: %d   .hypotheses: %d\n",
			r.Agent.Iterations, len(r.Agent.Hypotheses))
		if r.Agent.Accepted != nil {
			fmt.Fprintf(&b, "  agent.accepted: %s %s cites %s\n",
				r.Agent.Accepted.Kind, r.Agent.Accepted.Amount, r.Agent.Accepted.SourceRef)
		}
	}
	fmt.Fprintf(&b, "  claim: %s\n", r.Claim)
	if r.ExceptionCostINR > 0 {
		fmt.Fprintf(&b, "  exception_cost_inr: %d\n", r.ExceptionCostINR)
	}
	for i, rem := range r.Remediation {
		fmt.Fprintf(&b, "  remediation[%d].action: %s\n", i, rem.Action)
		fmt.Fprintf(&b, "  remediation[%d].effect: %s\n", i, rem.Effect)
	}
	return b.String()
}

func passWord(b bool) string {
	if b {
		return "pass"
	}
	return "fail"
}

func answerSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"answer": map[string]any{
				"type":        "string",
				"description": "The answer in plain English for a finance lead. Brief.",
			},
			"answerable": map[string]any{
				"type":        "boolean",
				"description": "False if the receipts do not contain what the question asks for.",
			},
			"citations": map[string]any{
				"type":        "array",
				"description": "One entry per factual claim, pointing at the receipt field it came from.",
				"items": map[string]any{
					"type":                 "object",
					"additionalProperties": false,
					"required":             []string{"receipt_id", "field"},
					"properties": map[string]any{
						"receipt_id": map[string]any{"type": "string"},
						"field":      map[string]any{"type": "string", "description": "Dotted field path, e.g. feasibility.k_star."},
						"value":      map[string]any{"type": "string", "description": "The value read from that field."},
					},
				},
			},
		},
		"required": []string{"answer", "answerable", "citations"},
	}
}
