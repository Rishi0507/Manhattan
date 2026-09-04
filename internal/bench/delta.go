package bench

import (
	"fmt"
	"sort"
	"strings"
)

// LiveDelta is what a real model buys over the deterministic stub, and what it
// must not change.
//
// The two halves are load-bearing in opposite directions. Postings, wrong
// postings and the composite must be IDENTICAL, because the verifier never
// asks the model whether it was right; a difference there is a leak in the
// trust boundary and the run should not be published. Diagnosis accuracy,
// repairs and proven cures MAY differ, because those are the jobs where
// judgement is the whole task.
type LiveDelta struct {
	Settlements int    `json:"settlements"`
	LiveModel   string `json:"live_model"`
	StubModel   string `json:"stub_model"`

	// Must not move.
	LiveWrong     int  `json:"live_auto_posted_wrong"`
	StubWrong     int  `json:"stub_auto_posted_wrong"`
	LiveM1Wrong   int  `json:"live_m1_posted_wrong"`
	StubM1Wrong   int  `json:"stub_m1_posted_wrong"`
	PostingsMoved bool `json:"postings_moved"`

	// May move, and the reason the command exists.
	LiveVerified      int     `json:"live_verified"`
	StubVerified      int     `json:"stub_verified"`
	LiveM1            int     `json:"live_m1_posted"`
	StubM1            int     `json:"stub_m1_posted"`
	LiveDiagnosisAcc  float64 `json:"live_diagnosis_accuracy"`
	StubDiagnosisAcc  float64 `json:"stub_diagnosis_accuracy"`
	LiveRepairs       int     `json:"live_agent_repairs"`
	StubRepairs       int     `json:"stub_agent_repairs"`
	LiveCures         int     `json:"live_proven_cures"`
	StubCures         int     `json:"stub_proven_cures"`
	LiveNotes         int     `json:"live_notes_drafted"`
	StubNotes         int     `json:"stub_notes_drafted"`
	LiveNotesRejected int     `json:"live_notes_rejected"`
	LiveCloseRecall   float64 `json:"live_close_condition_recall"`
	StubCloseRecall   float64 `json:"stub_close_condition_recall"`
	LiveCloseFindings int     `json:"live_close_findings"`
	StubCloseFindings int     `json:"stub_close_findings"`

	// Actually billed, which the offline path can only model.
	LiveINRPer1k  float64 `json:"live_inr_per_1k_settlements"`
	ModelledPer1k float64 `json:"modelled_inr_per_1k_settlements"`
	LiveCalls     int     `json:"live_model_calls"`
	LiveCacheHit  float64 `json:"live_cache_hit_rate"`
	PriceIsReal   bool    `json:"live_price_is_real_spend"`

	// What the provider did not do.
	//
	// A quality column that moves can mean the model answered worse or that it
	// did not answer, and those call for opposite responses: one is a model
	// choice, the other is an integration to fix. The batch records a failed
	// call as an exception it could not clear, so without these the two are
	// indistinguishable in the delta table.
	LiveFailures       int            `json:"live_model_call_failures"`
	LiveReliability    float64        `json:"live_model_call_reliability"`
	LiveFailuresByRole map[string]int `json:"live_model_call_failures_by_role,omitempty"`
}

// Delta compares a live run against an identical stub run.
func Delta(live, stub Summary) LiveDelta {
	d := LiveDelta{
		LiveFailures:       live.ModelFailures,
		LiveReliability:    live.ModelReliability,
		LiveFailuresByRole: live.FailuresByRole,
		Settlements:        live.Settlements,
		LiveModel:          live.ProviderModels,
		StubModel:          stub.ProviderModels,
		LiveVerified:       live.AutoPosted,
		StubVerified:       stub.AutoPosted,
		LiveWrong:          live.AutoPostedWrong,
		StubWrong:          stub.AutoPostedWrong,
		LiveM1:             live.M1Posted,
		StubM1:             stub.M1Posted,
		LiveM1Wrong:        live.M1PostedWrong,
		StubM1Wrong:        stub.M1PostedWrong,
		LiveDiagnosisAcc:   live.DiagnosisAccuracy,
		StubDiagnosisAcc:   stub.DiagnosisAccuracy,
		LiveRepairs:        live.AgentRepaired,
		StubRepairs:        stub.AgentRepaired,
		LiveCures:          live.AgentProvenCures,
		StubCures:          stub.AgentProvenCures,
		LiveNotes:          live.NotesDrafted,
		StubNotes:          stub.NotesDrafted,

		LiveNotesRejected: live.NotesRejected,
		LiveINRPer1k:      live.INRPer1k,
		ModelledPer1k:     stub.INRPer1k,
		LiveCalls:         live.ModelCalls,
		LiveCacheHit:      live.Cost.CacheHitRate,
		PriceIsReal:       live.PriceIsReal,
	}
	if live.Close != nil {
		d.LiveCloseRecall = live.Close.Recall
		d.LiveCloseFindings = len(live.Close.RootCauses)
	}
	if stub.Close != nil {
		d.StubCloseRecall = stub.Close.Recall
		d.StubCloseFindings = len(stub.Close.RootCauses)
	}

	// Wrong postings are the property that must hold. Verified counts may
	// legitimately differ, because a better proposer clears more exceptions;
	// what may never differ is whether anything posted was WRONG, or whether
	// the composite posted something wrong.
	d.PostingsMoved = d.LiveWrong != d.StubWrong || d.LiveM1Wrong != d.StubM1Wrong
	return d
}

// RenderDelta prints the comparison for a terminal.
func RenderDelta(d LiveDelta) string {
	var b strings.Builder
	fmt.Fprintf(&b, "\n%d settlements, identical seed, two providers\n\n", d.Settlements)
	fmt.Fprintf(&b, "  %-34s %10s %10s\n", "", "live", "stub")
	fmt.Fprintf(&b, "  %-34s %10s %10s\n", strings.Repeat("-", 34),
		strings.Repeat("-", 10), strings.Repeat("-", 10))

	row := func(label string, a, c any) {
		fmt.Fprintf(&b, "  %-34s %10v %10v\n", label, a, c)
	}
	fmt.Fprintf(&b, "  MUST NOT MOVE\n")
	row("auto-posted wrong", d.LiveWrong, d.StubWrong)
	row("composite posted wrong", d.LiveM1Wrong, d.StubM1Wrong)
	fmt.Fprintf(&b, "\n  MAY MOVE, and is why this runs\n")
	row("verified", d.LiveVerified, d.StubVerified)
	row("composite posted", d.LiveM1, d.StubM1)
	row("agent repairs", d.LiveRepairs, d.StubRepairs)
	row("proven cures", d.LiveCures, d.StubCures)
	row("defect diagnosis accuracy",
		fmt.Sprintf("%.0f%%", d.LiveDiagnosisAcc*100),
		fmt.Sprintf("%.0f%%", d.StubDiagnosisAcc*100))
	row("analyst notes drafted", d.LiveNotes, d.StubNotes)
	row("close condition recall",
		fmt.Sprintf("%.0f%%", d.LiveCloseRecall*100),
		fmt.Sprintf("%.0f%%", d.StubCloseRecall*100))
	row("close root causes found", d.LiveCloseFindings, d.StubCloseFindings)
	fmt.Fprintf(&b, "\n  COST\n")
	row("INR per 1k settlements",
		fmt.Sprintf("%.0f", d.LiveINRPer1k), fmt.Sprintf("%.0f", d.ModelledPer1k))
	row("cache hit rate", fmt.Sprintf("%.0f%%", d.LiveCacheHit*100), "n/a")
	row("actually billed", d.PriceIsReal, false)

	fmt.Fprintf(&b, "\n  PROVIDER RELIABILITY\n")
	row("calls completed",
		fmt.Sprintf("%.0f%%", d.LiveReliability*100), "100%")
	row("calls that failed", d.LiveFailures, 0)
	if len(d.LiveFailuresByRole) > 0 {
		roles := make([]string, 0, len(d.LiveFailuresByRole))
		for r := range d.LiveFailuresByRole {
			roles = append(roles, r)
		}
		sort.Strings(roles)
		for _, r := range roles {
			row("  failed: "+r, d.LiveFailuresByRole[r], 0)
		}
		fmt.Fprintf(&b, "\n  A failed call is recorded as an exception the agent could not clear,\n")
		fmt.Fprintf(&b, "  so a role failing here depresses the quality column above without\n")
		fmt.Fprintf(&b, "  the model having answered anything badly. Read the two together.\n")
	}

	if d.PostingsMoved {
		fmt.Fprintf(&b, "\n  wrong-posting counts differ between providers, which they must not.\n")
	} else {
		fmt.Fprintf(&b, "\n  Wrong postings identical across providers, which is the property the\n")
		fmt.Fprintf(&b, "  whole design exists to have: the model changes how much gets cleared,\n")
		fmt.Fprintf(&b, "  never whether what cleared was right.\n")
	}
	return b.String()
}
