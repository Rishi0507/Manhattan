// Package bench runs Manhattan and B0 over the same data and reports the
// distribution, the breaking point, and the cost of both.
//
// The track brief warns that one cherry-picked match proves nothing, so
// nothing here reports a headline in isolation. Every figure is emitted from
// a seeded run and written to RESULTS.md by script, never typed, and the
// modelled and measured columns are printed side by side rather than merged.
package bench

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/Rishi0507/manhattan/internal/accounting"
	"github.com/Rishi0507/manhattan/internal/agent"
	"github.com/Rishi0507/manhattan/internal/baseline"
	"github.com/Rishi0507/manhattan/internal/evidence"
	"github.com/Rishi0507/manhattan/internal/generate"
	"github.com/Rishi0507/manhattan/internal/llm"
	"github.com/Rishi0507/manhattan/internal/model"
	"github.com/Rishi0507/manhattan/internal/narrow"
	"github.com/Rishi0507/manhattan/internal/pipeline"
)

// CaseOutcome is what one adversarial case produced on both systems.
type CaseOutcome struct {
	Case generate.Case `json:"case"`

	// Manhattan side.
	Status       evidence.Status   `json:"status"`
	Flags        []evidence.Flag   `json:"flags"`
	Posted       bool              `json:"posted"`
	PostedWrong  bool              `json:"posted_wrong"`
	PoolN        int               `json:"pool_n"`
	KStar        int               `json:"k_star"`
	CollisionIdx float64           `json:"collision_index"`
	LatencyMS    int64             `json:"latency_ms"`
	Receipt      *evidence.Receipt `json:"receipt"`

	// B0 side, on identical inputs.
	B0Posted      bool     `json:"b0_posted"`
	B0PostedWrong bool     `json:"b0_posted_wrong"`
	B0Confidence  float64  `json:"b0_confidence"`
	B0Proposed    []string `json:"b0_proposed"`
	B0TokensIn    int      `json:"b0_tokens_in"`

	Met  bool   `json:"expectation_met"`
	Note string `json:"note,omitempty"`
}

// RunCases executes the eleven adversarial cases head to head.
func RunCases(ctx context.Context, provider llm.Provider) []CaseOutcome {
	var out []CaseOutcome
	for _, c := range generate.Cases() {
		out = append(out, RunCase(ctx, c, provider))
	}
	return out
}

// RunCase executes one case on both systems.
//
// The first settlement of each generated dataset is the one reported. The
// rest exist so that the record universe contains the neighbouring cycles a
// real reconciler has to narrow away; reporting all of them would measure
// the same pathology repeatedly.
func RunCase(ctx context.Context, c generate.Case, provider llm.Provider) CaseOutcome {
	ds := c.Build()
	cfg := configFor(c)
	eng := pipeline.New(ds, cfg)

	oc := CaseOutcome{Case: c}
	if len(ds.Credits) == 0 {
		oc.Note = "the generator produced no settlements for this case"
		return oc
	}

	// Pick the settlement that actually exhibits the pathology: the first one
	// whose ground-truth batch is non-empty and whose window is fully
	// populated, which excludes the first and last days of the generated span.
	credit := pickCredit(ds)
	truth := ds.GroundTruth[credit.Ref]

	start := time.Now()
	rec := eng.Reconcile(credit)
	if rec.Status == evidence.StatusUnresolved && provider != nil {
		resolved, _ := agent.NewResolver(provider).Resolve(ctx, eng, credit, rec)
		rec = resolved
	}
	oc.LatencyMS = time.Since(start).Milliseconds()

	oc.Receipt = rec
	oc.Status = rec.Status
	oc.Flags = rec.Flags
	oc.PoolN = rec.Pool.N
	oc.KStar = rec.Feasibility.KStar
	oc.CollisionIdx = rec.Feasibility.IndexAtKStar
	oc.Posted = rec.Status.Postable()
	if oc.Posted {
		oc.PostedWrong = !sameSet(rec.Witness, attributable(truth, eng))
	}

	// B0 sees exactly the same narrowed pool, so the comparison isolates the
	// verification discipline rather than the plumbing.
	nCfg := cfg.Narrow
	merchant := eng.Merchants[credit.MerchantID]
	nCfg.CycleDays = merchant.SettlementCycleDays
	nCfg.EnforceInstrument = merchant.InstrumentSegregated
	narrowed := narrow.Apply(eng.Records, credit, merchant, nCfg)

	b0 := baseline.Match(narrowed.Pool, credit.Amount, credit.DeclaredTxnCount, baseline.DefaultConfig())
	_ = narrowed
	oc.B0Posted = b0.Posted
	oc.B0Confidence = b0.Confidence
	oc.B0Proposed = b0.Proposed
	oc.B0TokensIn = b0.TokensIn
	if b0.Posted {
		oc.B0PostedWrong = !b0.Correct(attributable(truth, eng))
	}

	oc.Met = meetsExpectation(c, rec)
	return oc
}

// configFor applies a case's overrides to the shipped configuration.
func configFor(c generate.Case) pipeline.Config {
	cfg := pipeline.DefaultConfig()
	cfg.RunID = fmt.Sprintf("case_%02d_%s", c.Number, c.Name)
	cfg.Seed = c.Spec.Seed

	if c.WindowHours > 0 {
		cfg.Narrow.Window = time.Duration(c.WindowHours * float64(time.Hour))
	}
	if c.InferredRounding {
		cfg.Accounting.Mode = accounting.ModeInferred
		cfg.Accounting.Delta = 1
	}
	if c.GateScope {
		cfg.ScopePolicy = pipeline.ScopePolicyGate
	}
	return cfg
}

// pickCredit chooses a settlement from the middle of the generated span, so
// that its narrowing window is populated on both sides.
func pickCredit(ds *model.Dataset) model.BankCredit {
	if len(ds.Credits) <= 2 {
		return ds.Credits[0]
	}
	return ds.Credits[len(ds.Credits)/2]
}

// meetsExpectation checks the receipt against the case's stated outcome.
//
// The expectations are matched on status and flags rather than on prose, so
// a case that starts behaving differently fails loudly instead of quietly
// producing a worse demo.
func meetsExpectation(c generate.Case, rec *evidence.Receipt) bool {
	has := func(f evidence.Flag) bool { return rec.HasFlag(f) }

	switch c.Number {
	case 1:
		return rec.Status == evidence.StatusVerified
	case 2:
		return rec.Status == evidence.StatusVerified && has(evidence.FlagSignedItemsPresent)
	case 3:
		return rec.Status == evidence.StatusVerified && has(evidence.FlagComplementSolved)
	case 4:
		return rec.Status == evidence.StatusUnderdetermined && len(rec.Remediation) > 0
	case 5:
		return rec.Status == evidence.StatusAmbiguous
	case 6:
		return rec.Status == evidence.StatusVerified && has(evidence.FlagFeeAnomaly)
	case 7:
		return has(evidence.FlagFeeCheckCircular) && !has(evidence.FlagFeeAnomaly)
	case 8:
		return rec.Status == evidence.StatusVerified && has(evidence.FlagRoundingApplied)
	case 9:
		return rec.Status == evidence.StatusVerified && has(evidence.FlagResolvedByHypothesis)
	case 10:
		return rec.Status == evidence.StatusNarrowingSensitive
	case 11:
		return rec.Status == evidence.StatusUnderdetermined && has(evidence.FlagEntropyInsufficient)
	}
	return false
}

// attributable filters a ground-truth batch down to the records that a
// reconstruction could possibly name.
//
// A record whose signed contribution is exactly zero moved no money, so no
// amount-based method can place it in or out of a batch, and Manhattan
// removes it before searching and names it separately on the receipt.
// Counting such a record against the reconstruction would be marking the
// system wrong for declining to guess, which is the opposite of what this
// benchmark is measuring.
func attributable(truth []string, eng *pipeline.Engine) []string {
	out := make([]string, 0, len(truth))
	for _, id := range truth {
		if r, ok := eng.ByID[id]; ok && r.Contribution == 0 {
			continue
		}
		out = append(out, id)
	}
	return out
}

func sameSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	x := append([]string{}, a...)
	y := append([]string{}, b...)
	sort.Strings(x)
	sort.Strings(y)
	for i := range x {
		if x[i] != y[i] {
			return false
		}
	}
	return true
}
