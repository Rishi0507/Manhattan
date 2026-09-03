package baseline

import (
	"sort"

	"github.com/Rishi0507/manhattan/internal/model"
	"github.com/Rishi0507/manhattan/internal/money"
)

// B1 is the answer everybody gives, implemented and measured.
//
// The first question anyone at a payments company asks about this project is
// some form of "we already ship that mapping". It is a fair question and it
// deserves a measurement rather than a paragraph.
//
// So B1 is the lookup. The settlement report states which payments make up a
// settlement; B1 reads that mapping, sums the contributions it names, and
// posts. Where the report is complete and correct this is the right answer,
// it is faster than anything else in this repository, and the solver is
// unnecessary. That is not in dispute and the numbers say so.
//
// What B1 cannot do is notice when the report is wrong. It has no independent
// account of the money, so its output is a restatement of its input. A report
// that omits a chargeback, or names a payment that settled in a different
// cycle, produces a mapping that B1 posts without complaint, because the only
// thing B1 could check the report against is the report.
//
// Manhattan reconstructs the credit from the merchant's own records and then
// compares. Where the two agree, that agreement is evidence. Where they
// disagree, that disagreement is the finding, and it is the thing no
// lookup can produce:
//
//	A reconciliation system that trusts its input is not reconciling.
//	It is transcribing.
//
// B1 is given every advantage, exactly as B0 is. It gets the report's mapping
// verbatim, it does no searching and cannot fail to find an answer, and it is
// charged nothing for the model calls a real implementation would still need
// to read the bank narration.
type B1Result struct {
	SettlementRef string      `json:"settlement_ref"`
	Posted        []string    `json:"posted_members"`
	Sum           money.Paise `json:"posted_sum_paise"`
	Target        money.Paise `json:"target_paise"`
	// Residual is what the report's own mapping leaves unexplained. B1 posts
	// regardless: a lookup that refused on a residual would be doing exactly
	// the independent check that distinguishes it from Manhattan, and it does
	// not have one.
	Residual money.Paise `json:"residual_paise"`
	Posted_  bool        `json:"posted"`
	// ReportDefective records whether the mapping B1 trusted was actually
	// wrong. It is read from ground truth by the benchmark, never by B1.
	ReportDefective bool `json:"report_was_defective"`
}

// TrustReport posts whatever the settlement report says.
func TrustReport(credit model.BankCredit, reported []string, byID map[string]model.Record) B1Result {
	res := B1Result{
		SettlementRef: credit.Ref,
		Target:        credit.Amount,
		Posted_:       true,
	}
	ids := append([]string(nil), reported...)
	sort.Strings(ids)
	res.Posted = ids
	for _, id := range ids {
		if r, ok := byID[id]; ok {
			res.Sum += r.Contribution
		}
	}
	res.Residual = res.Sum - credit.Amount

	// A mapping with nothing in it is the one case a lookup does refuse,
	// because there is nothing to post rather than because anything was
	// checked.
	if len(ids) == 0 {
		res.Posted_ = false
	}
	return res
}

// Correct reports whether B1's posting matches the truth.
func (r B1Result) Correct(truth []string) bool {
	if len(r.Posted) != len(truth) {
		return false
	}
	want := make(map[string]bool, len(truth))
	for _, t := range truth {
		want[t] = true
	}
	for _, id := range r.Posted {
		if !want[id] {
			return false
		}
	}
	return true
}
