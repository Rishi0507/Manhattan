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
	case RoleTriage:
		out, err = o.triage(req.User)
	case RoleRemediate:
		out, err = o.remediate(req.User)
	case RoleControl:
		out, err = o.control(req.User)
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
			ByRole:       map[Role]int{req.Role: 1},
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

// plan chooses the next action on a settlement that did not auto-post.
//
// The rules are a decision table, not reasoning, and they are deliberately
// the obvious ones: if the pool is large and the answer is underdetermined,
// tighten the window; if a residual exists and an unjoined feed exists, look
// in it; otherwise escalate. A real model does better than this, especially
// on the judgement calls about when tightening will not help.
//
// It works at all because of where the boundary is. The stub is a bad
// planner, and a bad planner clears fewer exceptions. It cannot cause a wrong
// posting, because after any action it chooses the whole verification stack
// runs again and decides for itself.
func (offlineProvider) plan(user string) (map[string]any, error) {
	status := ""
	for _, s := range []string{"UNDERDETERMINED", "NARROWING_SENSITIVE", "AMBIGUOUS", "UNRESOLVED"} {
		if strings.Contains(user, "VERIFIER CONCLUDED: "+s) {
			status = s
			break
		}
	}

	pool := grabInt(user, `candidates after narrowing: (\d+)`)
	window := grabFloat(user, `window currently: plus or minus (\d+(?:\.\d+)?) hours`)
	twin := grabFloat(user, `twin mass: (\d+(?:\.\d+)?)`)
	residual := int64(grabInt(user, `RESIDUAL_PAISE=(-?\d+)`))
	act := func(kind, rationale string, extra map[string]any) (map[string]any, error) {
		out := map[string]any{"kind": kind, "rationale": rationale}
		for k, v := range extra {
			out[k] = v
		}
		return out, nil
	}

	triedTighten := strings.Contains(user, "TIGHTEN_WINDOW")
	corroborated := grabFloat(user, `CORROBORATED_WINDOW_HOURS=(\d+(?:\.\d+)?)`)
	proofs := grabInt(user, `PROVED_SETTLEMENTS=(\d+)`)

	hasFeed := strings.Contains(user, "UNJOINED FEEDS AVAILABLE: chargeback") ||
		strings.Contains(user, "(") && strings.Contains(user, "records) ")

	switch status {
	case "NARROWING_SENSITIVE":
		// Never try to make this post. A rival exists in the widened pool, so
		// the answer came from a filtering decision and a human has to confirm
		// the constraint.
		return act("ESCALATE",
			"a rival reconstruction exists once the pool is widened, so the answer came from filtering rather than arithmetic and needs a human to confirm the constraint", nil)

	case "UNDERDETERMINED", "AMBIGUOUS":
		// The one narrowing move that may post, tried before the one that
		// cannot. Only here: UNDERDETERMINED and AMBIGUOUS are pool-too-wide
		// problems, which is what narrowing addresses. UNRESOLVED is a
		// missing-record problem, and an earlier version that tried this
		// first on every status pre-empted SEARCH_FEED and cost seven
		// repairs.
		if corroborated > 0 && proofs > 0 && corroborated < window &&
			!strings.Contains(user, "chose NARROW_TO_HISTORY") {
			return act("NARROW_TO_HISTORY",
				"this merchant's own proved settlements all closed inside a narrower window than the one in use, so narrowing to the bound that history demonstrates removes candidates without removing anything a proof has shown belongs",
				map[string]any{"window_hours": corroborated})
		}
		if twin > 0.30 {
			return act("ESCALATE",
				"twin mass is high, so the amounts genuinely do not distinguish these transactions and no narrowing will help; this needs a settlement reference", nil)
		}
		if !triedTighten && pool > 24 && window > 4 {
			// Halving the window roughly halves the pool, and the collision
			// index falls far faster than linearly because it grows like
			// C(n, k).
			next := window / 2
			if next < 6 {
				next = 6
			}
			return act("TIGHTEN_WINDOW",
				"the pool is large for a single settlement cycle, which suggests the value-date window is admitting more than one capture day",
				map[string]any{"window_hours": next})
		}
		return act("ESCALATE",
			"the pool cannot be narrowed further on the constraints available, so no unique reconstruction is reachable", nil)

	case "UNRESOLVED":
		if hasFeed && residual != 0 {
			return act("SEARCH_FEED",
				"an exact residual with an unjoined disputes feed available is most often a chargeback that was never wired into the pool",
				map[string]any{"record_kind": "chargeback"})
		}
		if residual != 0 && window < 24 {
			return act("WIDEN_WINDOW",
				"nothing reconstructs the credit and the window is tight, so a record that belongs to this batch may have been cut out of it",
				map[string]any{"window_hours": window * 1.75})
		}
		abs := residual
		if abs < 0 {
			abs = -abs
		}
		return act("PROPOSE_ADJUSTMENT",
			"no feed explains the residual, so it is asserted as an unmodelled adjustment for an analyst to confirm; it cannot post uncited",
			map[string]any{"hypothesis_kind": "ADJUSTMENT", "amount_paise": abs})
	}

	return act("ESCALATE", "no action in the vocabulary applies to this state", nil)
}

func grabInt(s, pattern string) int {
	m := regexp.MustCompile(pattern).FindStringSubmatch(s)
	if len(m) < 2 {
		return 0
	}
	var v int
	fmt.Sscanf(m[1], "%d", &v)
	return v
}

func grabFloat(s, pattern string) float64 {
	m := regexp.MustCompile(pattern).FindStringSubmatch(s)
	if len(m) < 2 {
		return 0
	}
	var v float64
	fmt.Sscanf(m[1], "%f", &v)
	return v
}

// answer responds to a question over the receipt store. The stub returns the
// grounded facts it was handed, with no interpretation layered on top.
// answer composes a grounded answer from the retrieved receipts.
//
// It is a stub and it reads like one, deliberately. It counts statuses across
// what retrieval handed it, names the dominant cause, quotes the remedy the
// receipts already carry, and stops. It cannot notice that two merchant types
// fail for the same underlying reason, cannot rank two remedies against each
// other, and cannot tell a finance lead which one to do on Monday.
//
// An earlier version returned "the offline stub cannot compose a narrative
// answer", which was honest and useless: it put an apology in the one place a
// reader looks for the model. Composing a flat answer from the evidence is
// both more honest about what the path does and a measurable baseline for
// `manhattan live` to beat, which is the only way the value of a real model
// here becomes a number rather than an assertion.
func (offlineProvider) answer(user string) (map[string]any, error) {
	counts := map[string]int{}
	for _, st := range []string{
		"VERIFIED", "AMBIGUOUS", "UNDERDETERMINED", "NARROWING_SENSITIVE", "UNRESOLVED",
	} {
		counts[st] = strings.Count(user, st)
	}
	dominant, most := "", 0
	for st, n := range counts {
		if n > most || (n == most && st < dominant) {
			dominant, most = st, n
		}
	}

	refs := extractRefs(user)
	var b strings.Builder
	if most == 0 || len(refs) == 0 {
		b.WriteString("The retrieved receipts do not carry enough to answer this. ")
		b.WriteString("What they do carry is listed below.")
		return map[string]any{
			"answer": b.String(), "citations": refs, "answerable": false,
		}, nil
	}

	fmt.Fprintf(&b, "Across the %d receipts retrieved for this question, the most common "+
		"outcome is %s.\n\n", len(refs), dominant)

	// Quote a remedy the receipts already computed, rather than inventing one.
	if i := strings.Index(user, "remediation:"); i >= 0 {
		line := user[i:]
		if j := strings.IndexByte(line, '\n'); j > 0 {
			line = line[:j]
		}
		fmt.Fprintf(&b, "The remedy those receipts already name: %s\n\n",
			strings.TrimSpace(strings.TrimPrefix(line, "remediation:")))
	}
	for _, st := range []string{
		"VERIFIED", "AMBIGUOUS", "UNDERDETERMINED", "NARROWING_SENSITIVE", "UNRESOLVED",
	} {
		if counts[st] > 0 {
			fmt.Fprintf(&b, "  %-22s %d\n", strings.ToLower(st), counts[st])
		}
	}
	b.WriteString("\nThis is the deterministic stub. It can count what the receipts say and " +
		"quote a remedy they already carry. It cannot weigh two remedies against each " +
		"other, notice that two merchant types fail for one underlying reason, or tell " +
		"you which change to make first. Those are the judgements a live model makes, " +
		"and `manhattan live` measures the difference.")

	return map[string]any{
		"answer": b.String(), "citations": refs, "answerable": true,
	}, nil
}

func (offlineProvider) explain(user string) (map[string]any, error) {
	return map[string]any{
		"explanation": "See the receipt fields; the offline stub does not paraphrase them.",
	}, nil
}

var reRef = regexp.MustCompile(`bank_credit_[a-z0-9_]+`)

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

// triage classifies a failed claim check, deterministically.
//
// The stub reads the same figures a model would and applies the obvious rules.
// It is deliberately unintelligent: it looks at whether a chargeback is among
// the records named but absent, whether the count contradicts the report's own
// declaration, and whether the residual is a whole record or a fraction of a
// fee. A real model does better on the ambiguous cases and cannot do better on
// these, which is the point of the stub existing at all.
func (o offlineProvider) triage(user string) (any, error) {
	residual := int64(grabInt(user, `RESIDUAL_PAISE=(-?\d+)`))
	abs := residual
	if abs < 0 {
		abs = -abs
	}
	claimed := grabInt(user, `the report names (\d+) records`)
	declared := grabInt(user, `declares (\d+) transactions`)

	class, why := "UNDIAGNOSED",
		"the check is exact and none of the modelled defect classes matches its shape"

	// The discriminating signal is the SIGN of the residual, not the count.
	//
	// An omission and a truncation both break the declared count, so counting
	// cannot tell them apart. What separates them is what was left out. The
	// claimed sum minus the target is positive when a DEBIT is missing from
	// the mapping, because the remaining members over-explain the credit, and
	// negative when a CREDIT is missing. A dispute is a debit.
	//
	// An earlier version checked the count first and classified all 25
	// defects as truncation, which was a stub that agreed with itself rather
	// than one that read the evidence.
	switch {
	case strings.Contains(user, "named and present only in an unjoined feed"):
		class = "OMITTED_DISPUTE"
		why = "the report names a record that is not in the joined data, which is the shape " +
			"of a dispute debited in this cycle but raised against an earlier one"
	case claimed > declared && declared > 0:
		class = "CROSS_CYCLE_MEMBER"
		why = "the report names more records than it declares, so one of them belongs to an " +
			"adjacent settlement and posting this mapping would double-count it"
	case strings.Contains(user, "already posted in a prior cycle"),
		strings.Contains(user, "belongs to a different merchant"):
		class = "CROSS_CYCLE_MEMBER"
		why = "the report names a record that belongs to an adjacent settlement, so posting " +
			"this mapping unchanged would double-count it across two cycles"
	case abs > 0 && abs < 500_00 && claimed == declared:
		class = "FEE_POLICY_MISMATCH"
		why = "the membership matches the declaration and only the arithmetic disagrees, by " +
			"far less than any whole record, which points at the fee schedule on this side"
	case residual > 0:
		class = "OMITTED_DISPUTE"
		why = "the named records over-explain the credit, so a debit that moved money in " +
			"this cycle is missing from the mapping, and a dispute is the usual cause"
	case residual < 0:
		class = "TRUNCATED_MAPPING"
		why = "the named records under-explain the credit and the count contradicts the " +
			"report's own declaration, which is what a partial file looks like"
	}
	return map[string]any{"defect_class": class, "rationale": why}, nil
}

// remediate drafts the analyst note, deterministically and without figures.
//
// Every sentence here is assembled from phrases rather than written, which is
// exactly the limitation being demonstrated: the stub cannot decide which fact
// an analyst needs first, so it uses a fixed order. That the eleven-case suite
// and every published number survive this is a statement about the verifier.
// The quality of these notes is the clearest thing a live model would improve,
// and it cannot change a single posting.
func (o offlineProvider) remediate(user string) (any, error) {
	var do, why, not, ask string

	switch {
	case strings.Contains(user, "join the disputes feed"):
		do = "Connect this merchant's disputes feed to the reconciliation inputs."
		why = "The debit is already available to this run, so joining it lets both the " +
			"independent reconstruction and the report's own claim be checked against " +
			"the whole batch rather than part of it."
		not = "It will not help settlements held because the amounts themselves do not " +
			"distinguish the transactions, which is a different and larger population."
	case strings.Contains(user, "confirm that window narrowed"),
		strings.Contains(user, "value-date window"):
		do = "Confirm the value-date window this merchant actually settles within."
		why = "Narrowing to the bound their own proved settlements demonstrate leaves a " +
			"single reconstruction with the accounting identity closing exactly, and that " +
			"was re-verified rather than estimated."
		not = "It will not post on its own, because a window is an assertion about the " +
			"merchant's behaviour and one of you has to own it."
		ask = "Ask the merchant to confirm their capture cutoff time."
	case strings.Contains(user, "supply the settlement reference"),
		strings.Contains(user, "settlement_id to payment_id"):
		do = "Request the settlement reference, or the mapping from settlement to payment."
		why = "The amounts in this pool repeat, so no arithmetic method can distinguish " +
			"one candidate batch from another; a reference collapses the whole leg from a " +
			"search to a lookup."
		not = "It will not make the existing amount-based reconstruction work, and nothing " +
			"will, because the information is not present in the amounts."
		ask = "Ask for the settlement reference on the credit narration."
	case strings.Contains(user, "re-fetch the settlement report"):
		do = "Re-fetch the settlement report for this reference."
		why = "The stated mapping contradicts its own declared count, which is what a " +
			"partial transfer looks like downstream rather than a reconciliation fault."
		not = "It will not resolve anything if the refetched file is identical, in which " +
			"case the discrepancy is real and belongs with the gateway."
	case strings.Contains(user, "confirm the fee schedule"):
		do = "Confirm the fee schedule configured for this merchant."
		why = "The membership the report states is consistent and only the arithmetic " +
			"disagrees, by a fraction of a per-transaction fee, which points at the policy " +
			"on this side."
		not = "It will not change the report, and if the schedule here is correct then the " +
			"gap is a pricing question for the gateway."
	default:
		do = "Route this to an analyst with the residual and the failed checks attached."
		why = "The check is exact and its cause is not established, so the evidence is " +
			"more useful than a diagnosis would be."
		not = "It will not clear itself, and nothing in the action vocabulary changes that."
	}
	return map[string]any{
		"what_to_do":               do,
		"why_it_works":             why,
		"what_it_will_not_fix":     not,
		"what_to_ask_the_merchant": ask,
	}, nil
}

// control writes the period close, deterministically.
//
// This is the stub's hardest job and the one where it most obviously falls
// short, which is the point of it existing rather than returning an apology.
// It applies four fixed rules per merchant: a large pool against a small batch
// means the window is wide, exact unexplained residuals mean a feed is missing,
// high twin mass means the amounts do not distinguish, and claim failures
// clustering means the report is at fault.
//
// What it cannot do is the actual controller's job. It cannot notice that two
// merchants share one root cause, cannot weigh a small expensive population
// against a large cheap one except by sorting, cannot tell that a wide window
// and a missing feed on the same merchant are one story rather than two, and
// writes a narrative by concatenation. Those are the judgements `manhattan
// live` measures, and the condition-recall figure beside this is the number
// that moves when a real model runs.
func (o offlineProvider) control(user string) (any, error) {
	type block struct {
		name                       string
		pool, batch, held, settle  float64
		twin                       float64
		exact, claimFails, feeAnom int
		heldINR                    int64
		underdet, unresolved       int
	}

	var blocks []block
	for _, chunk := range strings.Split(user, "\n\n  ")[1:] {
		lines := strings.Split(chunk, "\n")
		if len(lines) == 0 || !strings.Contains(lines[0], "settlements,") {
			continue
		}
		bl := block{name: strings.TrimSpace(strings.SplitN(lines[0], ":", 2)[0])}
		bl.settle = grabFloat(chunk, `(\d+) settlements,`)
		bl.pool = grabFloat(chunk, `mean pool after narrowing (\d+(?:\.\d+)?) candidates`)
		bl.batch = grabFloat(chunk, `mean batch of (\d+(?:\.\d+)?)`)
		bl.twin = grabFloat(chunk, `twin mass (\d+(?:\.\d+)?)`)
		bl.exact = grabInt(chunk, `residual is exact: (\d+)`)
		bl.claimFails = grabInt(chunk, `report claim failed its check: (\d+)`)
		bl.feeAnom = grabInt(chunk, `fee anomaly: (\d+)`)
		bl.heldINR = int64(grabFloat(chunk, `clearing cost (\d+) INR`))
		bl.underdet = grabInt(chunk, `UNDERDETERMINED (\d+)`)
		bl.unresolved = grabInt(chunk, `UNRESOLVED (\d+)`)
		bl.held = grabFloat(chunk, `(\d+) held`)
		if bl.name != "" {
			blocks = append(blocks, bl)
		}
	}

	var causes []map[string]any
	add := func(scope, class, ev, action string, n int, inr int64) {
		causes = append(causes, map[string]any{
			"scope": scope, "cause_class": class, "evidence": ev,
			"recommended_action": action, "settlements_affected": n, "value_held_inr": inr,
		})
	}

	for _, b := range blocks {
		switch {
		case b.twin >= 0.30:
			add(b.name, "AMOUNTS_DO_NOT_DISTINGUISH",
				fmt.Sprintf("twin mass %.2f, above the 0.30 refusal threshold, across %.0f settlements",
					b.twin, b.settle),
				"supply a settlement reference or a per-payment fee row; no amount-based method can work here",
				int(b.held), b.heldINR)
		case b.exact > 2:
			add(b.name, "UNJOINED_FEED",
				fmt.Sprintf("%d settlements where nothing reconstructs the credit and the residual is exact",
					b.exact),
				"join the disputes feed for this merchant and re-run", b.exact, b.heldINR)
		case b.batch > 0 && b.pool/b.batch > 3.5 && b.underdet > b.unresolved:
			add(b.name, "WINDOW_TOO_WIDE",
				fmt.Sprintf("mean pool of %.0f candidates for a mean batch of %.0f, and refusals are UNDERDETERMINED rather than UNRESOLVED",
					b.pool, b.batch),
				"narrow the value-date window to what this merchant's proved settlements support",
				b.underdet, b.heldINR)
		case b.claimFails > 3:
			add(b.name, "REPORT_DEFECTS",
				fmt.Sprintf("%d settlements where the gateway's own stated mapping failed its arithmetic check",
					b.claimFails),
				"raise the failing references with the gateway", b.claimFails, b.heldINR)
		case b.feeAnom > 3:
			add(b.name, "FEE_POLICY_DRIFT",
				fmt.Sprintf("%d settlements carrying a fee anomaly", b.feeAnom),
				"confirm the fee schedule configured for this merchant", b.feeAnom, b.heldINR)
		}
		// A merchant can have two problems, and the stub only ever reports the
		// first that matches. That is the single clearest thing a real model
		// improves here and it is left in rather than papered over.
	}
	if len(causes) == 0 {
		add("all", "NOTHING_SYSTEMIC",
			"no merchant crossed a threshold on any modelled cause",
			"work the queue by value per analyst hour", 0, 0)
	}

	var names []string
	for _, c := range causes {
		names = append(names, fmt.Sprintf("%s on %s", c["cause_class"], c["scope"]))
	}
	narrative := fmt.Sprintf(
		"This period closed with %d systemic findings across %d merchant types: %s. "+
			"They are ranked by held value below, and each names the figures it was read from. "+
			"This close was written by the deterministic stub, which applies one fixed rule per "+
			"merchant and reports only the first that matches, so a merchant with two problems "+
			"shows one.",
		len(causes), len(blocks), strings.Join(names, "; "))

	return map[string]any{
		"narrative":   narrative,
		"root_causes": causes,
		"escalations": []string{
			"confirm with operations which merchants are expected to have a disputes feed connected",
			"confirm the value-date window configured for each merchant against their capture cutoff",
		},
		"what_i_cannot_tell": "Whether two merchants showing the same class share one underlying " +
			"configuration or two separate ones, and whether a merchant flagged for one cause also " +
			"has a second. The stub reports the first rule that matches per merchant and stops.",
	}, nil
}
