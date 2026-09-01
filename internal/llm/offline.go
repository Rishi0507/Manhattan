package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// NewOffline returns a deterministic provider that needs no network.
//
// It is a stub, it says so on every receipt it touches, and it is not
// pretending to be a language model. It exists because of a property of this
// architecture that is worth demonstrating rather than merely asserting:
//
//	the quality of the proposer changes the recall of the system.
//	It cannot change the correctness of the system.
//
// Every hypothesis this stub emits is applied to the pool and re-verified by
// the unmodified gate, solver, completeness and recomputation stack. A stub
// that guesses badly produces exceptions. A stub that guesses well produces
// citations. Neither can produce a wrong posting, because neither is asked
// whether it was right. Running the whole demo on a deliberately unintelligent
// proposer is the cleanest available evidence that the verifier, not the
// model, is what makes the money safe.
func NewOffline() Provider { return offlineProvider{} }

type offlineProvider struct{}

func (offlineProvider) Name() string      { return "offline-stub" }
func (offlineProvider) Model(Role) string { return "replay" }

func (o offlineProvider) Structured(ctx context.Context, req Request) (*Result, error) {
	var (
		out any
		err error
	)
	switch req.Role {
	case RoleParse:
		out, err = o.parse(req.User)
	case RoleResolve:
		out, err = o.resolve(req.User)
	case RolePlan:
		out, err = o.plan(req.User)
	case RoleAnswer:
		out, err = o.answer(req.User)
	case RoleExplain:
		out, err = o.explain(req.User)
	default:
		err = fmt.Errorf("llm: offline provider has no behaviour for role %q", req.Role)
	}
	if err != nil {
		return nil, err
	}
	b, err := json.Marshal(out)
	if err != nil {
		return nil, err
	}
	// Token counts are estimated at roughly four characters per token, and
	// labelled as estimates wherever they are aggregated. Reporting a
	// fabricated exact count would be worse than reporting an approximate one.
	return &Result{
		JSON:  b,
		Model: "replay",
		Usage: Usage{
			InputTokens:  (len(req.System) + len(req.User)) / 4,
			OutputTokens: len(b) / 4,
			Calls:        1,
		},
	}, nil
}

var (
	reUTR     = regexp.MustCompile(`(?i)\b(?:UTR|REF|RRN)[^A-Za-z0-9]?([A-Z0-9]{6,22})\b`)
	reDigits  = regexp.MustCompile(`\b(\d{6,22})\b`)
	reChannel = regexp.MustCompile(`(?i)\b(NEFT|RTGS|IMPS|UPI|ACH|NACH)\b`)
)

// parse pulls typed fields out of a bank narration.
//
// A regex is exactly the wrong tool for this in production, which is the
// point: narration formats differ by bank, by channel and by decade, and a
// pattern set rots. That is why the live path hands this job to a model. The
// stub covers the handful of shapes the generator produces so the offline
// demo has something to work with.
func (offlineProvider) parse(user string) (map[string]any, error) {
	upper := strings.ToUpper(user)

	ref := ""
	if m := reUTR.FindStringSubmatch(user); len(m) > 1 {
		ref = m[1]
	} else if m := reDigits.FindStringSubmatch(user); len(m) > 1 {
		ref = m[1]
	}

	channel := "UNKNOWN"
	if m := reChannel.FindStringSubmatch(user); len(m) > 1 {
		channel = strings.ToUpper(m[1])
	}

	counterparty := ""
	if strings.Contains(upper, "RAZORPAY") {
		counterparty = "RAZORPAY SOFTWARE PVT LTD"
	}

	confident := ref != "" && counterparty != ""
	return map[string]any{
		"bank_reference":  ref,
		"channel":         channel,
		"counterparty":    counterparty,
		"is_settlement":   strings.Contains(upper, "SETTL") || counterparty != "",
		"direction":       "credit",
		"confident":       confident,
		"provenance_span": firstSpan(user, ref),
	}, nil
}

func firstSpan(s, needle string) string {
	if needle == "" {
		return ""
	}
	i := strings.Index(s, needle)
	if i < 0 {
		return ""
	}
	return fmt.Sprintf("%d:%d", i, i+len(needle))
}

var reResidual = regexp.MustCompile(`RESIDUAL_PAISE=(-?\d+)`)

// resolve proposes typed hypotheses for an unexplained residual.
//
// The stub's whole strategy is to enumerate the vocabulary in a fixed order
// and let the verifier reject what does not close. That is a bad proposer.
// It is also a safe one, and the difference between those two words is the
// architecture.
func (offlineProvider) resolve(user string) (map[string]any, error) {
	residual := int64(0)
	if m := reResidual.FindStringSubmatch(user); len(m) > 1 {
		fmt.Sscanf(m[1], "%d", &residual)
	}
	abs := residual
	if abs < 0 {
		abs = -abs
	}

	// Ranking reflects base rates rather than insight: an unexplained debit
	// on a settlement is most often a dispute, then a late refund, then a
	// bank charge.
	kinds := []struct{ kind, effect, why string }{
		{"CHARGEBACK_DEBIT", "add_item",
			"an unexplained debit of this shape is most often a disputed transaction plus its handling fee"},
		{"LATE_REFUND", "add_item",
			"a refund that cleared after the batch was cut would net off this cycle without appearing in it"},
		{"BANK_FEE", "adjust_target",
			"a sponsor bank charge is deducted from the credit rather than from the batch"},
	}

	var hyps []map[string]any
	for _, k := range kinds {
		hyps = append(hyps, map[string]any{
			"kind":         k.kind,
			"amount_paise": abs,
			"effect":       k.effect,
			"rationale":    k.why,
		})
	}
	return map[string]any{"hypotheses": hyps}, nil
}

// plan orders the narrowing constraints to relax. The stub returns the
// configured default order unchanged, which is the honest no-op: it has no
// basis on which to reorder.
func (offlineProvider) plan(user string) (map[string]any, error) {
	return map[string]any{
		"order": []string{
			"outside_settlement_window",
			"payment_method_mismatch",
			"already_reconciled",
			"currency_mismatch",
		},
		"rationale": "default order; the offline stub has no basis on which to reprioritise",
	}, nil
}

// answer responds to a question over the receipt store. The stub returns the
// grounded facts it was handed, with no interpretation layered on top.
func (offlineProvider) answer(user string) (map[string]any, error) {
	return map[string]any{
		"answer": "The offline stub cannot compose a narrative answer. The receipt fields " +
			"relevant to this question are listed in the citations below, and they are the " +
			"complete basis on which any answer would have been given.",
		"citations":  extractRefs(user),
		"answerable": true,
	}, nil
}

func (offlineProvider) explain(user string) (map[string]any, error) {
	return map[string]any{
		"explanation": "See the receipt fields; the offline stub does not paraphrase them.",
	}, nil
}

var reRef = regexp.MustCompile(`bank_credit_[0-9_]+`)

func extractRefs(s string) []map[string]string {
	seen := map[string]bool{}
	var out []map[string]string
	for _, m := range reRef.FindAllString(s, -1) {
		if seen[m] {
			continue
		}
		seen[m] = true
		out = append(out, map[string]string{"receipt_id": m, "field": "status"})
	}
	if out == nil {
		out = []map[string]string{}
	}
	return out
}
