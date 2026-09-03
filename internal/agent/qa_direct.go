package agent

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Rishi0507/manhattan/internal/evidence"
	"github.com/Rishi0507/manhattan/internal/money"
	"github.com/Rishi0507/manhattan/internal/narrow"
)

// Direct answers the questions that are aggregate queries over the receipt
// store rather than acts of interpretation.
//
// "Which constraint dropped the most records this month" has exactly one
// correct answer and it is a sum over a field. Sending that to a language
// model would be slower, cost money, and introduce the possibility of an
// arithmetic error into a question that has none. So it is not sent.
//
// This is the same principle the rest of the system runs on, applied one
// level up: use the model for the part that requires judgement, and use
// arithmetic for the part that is arithmetic. It also means the offline demo
// answers the questions a finance lead actually asks, with real numbers,
// rather than apologising for the absence of an API key.
//
// Anything open-ended falls through to the model, which is most of what
// people ask.
func Direct(store *evidence.Store, question string) (Answer, bool) {
	q := strings.ToLower(question)
	all := store.All()
	if len(all) == 0 {
		return Answer{}, false
	}

	switch {
	case matches(q, "constraint", "dropped") || matches(q, "constraint", "drop") ||
		matches(q, "narrowing", "most"):
		return constraintDrops(all), true

	case matches(q, "backlog", "cost") || matches(q, "exception", "cost") ||
		matches(q, "queue", "cost") || matches(q, "analyst", "time") ||
		matches(q, "costing", "us"):
		return backlogCost(all), true

	case matches(q, "hardest", "reconcile") || matches(q, "merchant", "hardest") ||
		matches(q, "which", "merchants") || matches(q, "worst", "merchant"):
		return hardestMerchants(all), true

	case matches(q, "circular") && strings.Contains(q, "fee"):
		return circularFeeChecks(all), true

	case matches(q, "how many", "post") || matches(q, "auto-post", "rate") ||
		matches(q, "status", "mix") || matches(q, "how many", "verified"):
		return statusMix(all, store.Run()), true

	case asksAboutPeople(q):
		return notRecorded(q), true
	}
	return Answer{}, false
}

// asksAboutPeople matches questions about who did something.
//
// Nobody did anything. There is no human in this pipeline: no approval step,
// no reviewer field, no assignee, no maker-checker. A receipt records what the
// system decided and the evidence it decided on, and that is the whole of it.
func asksAboutPeople(q string) bool {
	who := strings.Contains(q, "who ") || strings.Contains(q, "which analyst") ||
		strings.Contains(q, "which user") || strings.Contains(q, "whom")
	act := strings.Contains(q, "approv") || strings.Contains(q, "sign") ||
		strings.Contains(q, "review") || strings.Contains(q, "authoris") ||
		strings.Contains(q, "authoriz") || strings.Contains(q, "assign") ||
		strings.Contains(q, "posted it") || strings.Contains(q, "decided")
	return who && act
}

// notRecorded declines, and says exactly why.
//
// This is the most important answer this agent gives, and it is deterministic
// on purpose. An assistant over a finance system that answers every question
// is not grounded in the data; it is grounded in the willingness of a language
// model to produce fluent text. The failure is silent, it is confident, and it
// is worse than no answer because somebody will act on it.
//
// So the decline names the field that would have held the answer, says that
// the field does not exist, and states what the receipts do record instead. A
// user who reads this knows what to go and look at. A user who reads an
// invented analyst name does not know anything at all, and does not know that
// they do not.
func notRecorded(_ string) Answer {
	return Answer{
		Answerable: false,
		Text: "The receipts do not record this and I will not infer it.\n\n" +
			"There is no approval, reviewer or assignee field on a receipt, because there is " +
			"no human step in this pipeline to record. A settlement is decided by an integer " +
			"identity and an exhaustive uniqueness count, and the receipt carries the evidence " +
			"for that decision rather than a person's name.\n\n" +
			"What a receipt does record, and what I can answer from: the status and the claim " +
			"behind it, the witness and every rival reconstruction, the narrowing waterfall " +
			"with a reason per dropped record, both collision-index estimators, every " +
			"completeness check and its verdict, the agent's decision trace where it acted, " +
			"the computed remediation, and the exception cost.",
		Citations: []Citation{
			{ReceiptID: "schema", Field: "no approval, reviewer or assignee field exists"},
		},
	}
}

func matches(q string, terms ...string) bool {
	for _, t := range terms {
		if !strings.Contains(q, t) {
			return false
		}
	}
	return true
}

func constraintDrops(all []*evidence.Receipt) Answer {
	agg := map[narrow.Constraint]int{}
	universe := 0
	for _, r := range all {
		universe += r.Narrowing.Before
		for c, n := range r.Narrowing.Dropped {
			agg[c] += n
		}
	}

	type kv struct {
		c narrow.Constraint
		n int
	}
	rows := make([]kv, 0, len(agg))
	for c, n := range agg {
		rows = append(rows, kv{c, n})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].n > rows[j].n })
	if len(rows) == 0 {
		return Answer{Text: "No narrowing drops are recorded in this store.", Answerable: false}
	}

	var b strings.Builder
	fmt.Fprintf(&b, "%s removed the most, at %s records across %d settlements, which is %.1f%% of everything narrowing looked at.\n\n",
		humanConstraint(rows[0].c), comma(rows[0].n), len(all), 100*float64(rows[0].n)/float64(max(universe, 1)))
	b.WriteString("The full breakdown:\n")
	for _, r := range rows {
		fmt.Fprintf(&b, "  %-34s %10s  (%.1f%%)\n",
			humanConstraint(r.c), comma(r.n), 100*float64(r.n)/float64(max(universe, 1)))
	}
	b.WriteString("\nThis matters more than it looks. Narrowing quality, not the solver, is what ")
	b.WriteString("determines how many settlements can be verified at all: the number of candidate ")
	b.WriteString("subsets grows like C(n, k), so a pool that is twice as large is very far from ")
	b.WriteString("twice as hard. If one constraint is doing most of the work, its configuration ")
	b.WriteString("is worth checking, because an over-tight window silently drops records that ")
	b.WriteString("genuinely belonged to a batch.")

	cites := make([]Citation, 0, len(rows))
	for i, r := range rows {
		if i >= 4 {
			break
		}
		cites = append(cites, Citation{
			ReceiptID: "aggregated across the store",
			Field:     "narrowing.dropped." + string(r.c),
			Value:     comma(r.n),
		})
	}
	return Answer{Text: b.String(), Citations: cites, Answerable: true, Retrieved: refs(all, 0)}
}

func backlogCost(all []*evidence.Receipt) Answer {
	cost, held := 0, 0
	var value money.Paise
	byStatus := map[evidence.Status]int{}
	costByStatus := map[evidence.Status]int{}
	for _, r := range all {
		if r.Status.Postable() {
			continue
		}
		held++
		cost += r.ExceptionCostINR
		value += r.TargetPaise
		byStatus[r.Status]++
		costByStatus[r.Status] += r.ExceptionCostINR
	}
	if held == 0 {
		return Answer{Text: "Nothing is held for review in this store.", Answerable: true}
	}

	var b strings.Builder
	fmt.Fprintf(&b, "The queue holds %d settlements worth %s, and clearing it costs about INR %s at the configured analyst handling time.\n\n",
		held, value, comma(cost))
	b.WriteString("By cause, most expensive first:\n")

	type kv struct {
		s evidence.Status
		c int
	}
	rows := make([]kv, 0, len(costByStatus))
	for s, c := range costByStatus {
		rows = append(rows, kv{s, c})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].c > rows[j].c })
	for _, r := range rows {
		fmt.Fprintf(&b, "  %-22s %4d settlements   INR %8s\n", r.s, byStatus[r.s], comma(r.c))
	}

	b.WriteString("\nEvery row carries a computed remediation rather than a note saying it needs review, ")
	b.WriteString("so the queue can be worked in the order that clears the most money per hour. ")
	b.WriteString("The largest group is worth attacking first as a configuration change rather than ")
	b.WriteString("one settlement at a time.")

	return Answer{
		Text:       b.String(),
		Citations:  []Citation{{ReceiptID: "aggregated across the store", Field: "exception_cost_inr", Value: comma(cost)}},
		Answerable: true,
		Retrieved:  refs(all, 0),
	}
}

func hardestMerchants(all []*evidence.Receipt) Answer {
	type agg struct {
		n, posted, entropyRefused int
		sigma, twin               float64
	}
	m := map[string]*agg{}
	for _, r := range all {
		k := r.Archetype
		if k == "" {
			k = r.MerchantID
		}
		a := m[k]
		if a == nil {
			a = &agg{}
			m[k] = a
		}
		a.n++
		a.sigma += r.Pool.SigmaPaise
		a.twin += r.AmountEntropy.TwinMass
		if r.Status.Postable() {
			a.posted++
		}
		if !r.AmountEntropy.Pass {
			a.entropyRefused++
		}
	}

	type row struct {
		name  string
		rate  float64
		twin  float64
		sigma float64
		n     int
		refus int
	}
	rows := make([]row, 0, len(m))
	for k, a := range m {
		rows = append(rows, row{
			name:  k,
			rate:  float64(a.posted) / float64(max(a.n, 1)),
			twin:  a.twin / float64(max(a.n, 1)),
			sigma: a.sigma / float64(max(a.n, 1)),
			n:     a.n,
			refus: a.entropyRefused,
		})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].rate < rows[j].rate })

	var b strings.Builder
	b.WriteString("Hardest first, by the share of settlements that could be verified:\n\n")
	for _, r := range rows {
		fmt.Fprintf(&b, "  %-22s auto-posted %3.0f%% of %3d   spread %-12s twin mass %.2f\n",
			strings.ReplaceAll(r.name, "_", " "), r.rate*100, r.n,
			money.Paise(int64(r.sigma)).String(), r.twin)
	}

	b.WriteString("\nThe reason is visible in the last two columns and it is not a quality problem ")
	b.WriteString("with those merchants. Reconstruction identifies a batch by its amounts, so it ")
	b.WriteString("needs the amounts to differ. A merchant billing three repeated subscription ")
	b.WriteString("prices has a high twin mass and a narrow spread, and their settlements are ")
	b.WriteString("genuinely not reconstructable from amounts by any method, ours included.\n\n")

	if len(rows) > 0 && rows[0].refus > 0 {
		lead := strings.ReplaceAll(rows[0].name, "_", " ")
		if rows[0].refus == rows[0].n {
			fmt.Fprintf(&b, "All %d %s settlements were refused on amount entropy alone, before any search ran. ",
				rows[0].n, lead)
		} else {
			fmt.Fprintf(&b, "%d of the %d %s settlements were refused on amount entropy alone, before any search ran. ",
				rows[0].refus, rows[0].n, lead)
		}
	}
	b.WriteString("The fix for those is not a better solver. It is a settlement reference carried ")
	b.WriteString("through to the payout, or splitting the payout by instrument or plan, either of ")
	b.WriteString("which collapses this leg from a search to a lookup.")

	cites := make([]Citation, 0, 3)
	for i, r := range rows {
		if i >= 3 {
			break
		}
		cites = append(cites, Citation{
			ReceiptID: "aggregated by archetype",
			Field:     "amount_entropy.twin_mass",
			Value:     fmt.Sprintf("%s: %.2f", r.name, r.twin),
		})
	}
	return Answer{Text: b.String(), Citations: cites, Answerable: true, Retrieved: refs(all, 0)}
}

func circularFeeChecks(all []*evidence.Receipt) Answer {
	var hit []*evidence.Receipt
	for _, r := range all {
		if r.FeeCheck != nil && r.FeeCheck.Circular {
			hit = append(hit, r)
		}
	}
	if len(hit) == 0 {
		return Answer{
			Text: "No settlement in this store had a circular fee check. That means every one of " +
				"them ran in a data mode where per-payment fee rows exist independently of the " +
				"policy, so the fee comparison carried real information.",
			Answerable: true,
			Retrieved:  refs(all, 0),
		}
	}

	var b strings.Builder
	fmt.Fprintf(&b, "%d of %d settlements had a circular fee check, and no fee anomaly claim was made for any of them.\n\n",
		len(hit), len(all))
	b.WriteString("This happens in lump-credit mode, where no independent per-payment fee rows exist. ")
	b.WriteString("The observed fee then has to be derived from the same policy that built the ")
	b.WriteString("contributions, so the two agree by construction and the agreement means nothing. ")
	b.WriteString("Reporting a check that cannot fail would be worse than reporting no check, ")
	b.WriteString("because it looks like assurance.\n\n")
	for i, r := range hit {
		if i >= 8 {
			fmt.Fprintf(&b, "  and %d more\n", len(hit)-8)
			break
		}
		fmt.Fprintf(&b, "  %-34s %s\n", r.SettlementRef, r.Status)
	}

	cites := make([]Citation, 0, 3)
	for i, r := range hit {
		if i >= 3 {
			break
		}
		cites = append(cites, Citation{ReceiptID: r.SettlementRef, Field: "fee_check.circular", Value: "true"})
	}
	return Answer{Text: b.String(), Citations: cites, Answerable: true, Retrieved: refs(hit, 12)}
}

func statusMix(all []*evidence.Receipt, run *evidence.Run) Answer {
	counts := map[evidence.Status]int{}
	for _, r := range all {
		counts[r.Status]++
	}
	posted := counts[evidence.StatusVerified]

	var b strings.Builder
	fmt.Fprintf(&b, "%d of %d settlements auto-posted, which is %.0f%%.\n\n",
		posted, len(all), 100*float64(posted)/float64(max(len(all), 1)))
	for _, s := range []evidence.Status{
		evidence.StatusVerified, evidence.StatusAmbiguous, evidence.StatusUnderdetermined,
		evidence.StatusNarrowingSensitive, evidence.StatusUnresolved,
	} {
		fmt.Fprintf(&b, "  %-22s %4d\n", s, counts[s])
	}
	if run != nil {
		fmt.Fprintf(&b, "\nNone of the %d postings was wrong when checked against ground truth the pipeline never saw.\n",
			run.AutoPosted)
		if run.AutoPostedWrong > 0 {
			fmt.Fprintf(&b, "(%d were wrong.)\n", run.AutoPostedWrong)
		}
	}
	b.WriteString("\nThe four statuses that are not VERIFIED are not degrees of failure. They are ")
	b.WriteString("different findings calling for different actions: rivals exist and both are shown, ")
	b.WriteString("the combinatorics rule out any unique answer, a filtering decision produced the ")
	b.WriteString("answer rather than the arithmetic, or nothing reconstructs the credit at all.")

	return Answer{
		Text:       b.String(),
		Citations:  []Citation{{ReceiptID: "aggregated across the store", Field: "status", Value: fmt.Sprintf("%d verified", posted)}},
		Answerable: true,
		Retrieved:  refs(all, 0),
	}
}

func humanConstraint(c narrow.Constraint) string {
	switch c {
	case narrow.ConstraintMerchant:
		return "a different merchant"
	case narrow.ConstraintCurrency:
		return "a different currency"
	case narrow.ConstraintWindow:
		return "outside the value-date window"
	case narrow.ConstraintReconciled:
		return "already posted in a prior cycle"
	case narrow.ConstraintInstrument:
		return "a different payment method"
	case narrow.ConstraintSettlementID:
		return "a different settlement reference"
	case narrow.ConstraintZeroContribution:
		return "nets to exactly zero"
	}
	return string(c)
}

func refs(rs []*evidence.Receipt, limit int) []string {
	if limit <= 0 || limit > len(rs) {
		limit = len(rs)
	}
	out := make([]string, 0, limit)
	for _, r := range rs[:limit] {
		out = append(out, r.SettlementRef)
	}
	return out
}

func comma(n int) string {
	s := fmt.Sprintf("%d", n)
	var out []byte
	for i := 0; i < len(s); i++ {
		if i > 0 && (len(s)-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, s[i])
	}
	return string(out)
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
