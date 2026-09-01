package bench

import (
	"context"
	"testing"

	"github.com/Rishi0507/manhattan/internal/llm"
)

// TestElevenCases is the adversarial suite, and it is the test that decides
// whether the demo is real.
//
// Each case is a specific, nameable way a confidence matcher produces a
// wrong answer. Asserting the expected status and flags here means a
// regression shows up as a failing test rather than as a demo that quietly
// stopped proving anything.
//
// It runs on the offline stub, with no API key and no network, which is
// itself part of the claim: the verifier decides, so a deliberately
// unintelligent proposer changes recall and cannot change correctness.
func TestElevenCases(t *testing.T) {
	ctx := context.Background()
	provider := llm.NewOffline()

	outcomes := RunCases(ctx, provider)
	if len(outcomes) != 11 {
		t.Fatalf("expected 11 cases, got %d", len(outcomes))
	}

	var manhattanWrong, b0Wrong, b0Posted, manhattanPosted int

	for _, oc := range outcomes {
		t.Run(oc.Case.Name, func(t *testing.T) {
			t.Logf("pool n=%d  k*=%d  E=%.3g  %dms", oc.PoolN, oc.KStar, oc.CollisionIdx, oc.LatencyMS)
			t.Logf("manhattan: %s %v", oc.Status, oc.Flags)
			t.Logf("b0:        posted=%v confidence=%.2f wrong=%v", oc.B0Posted, oc.B0Confidence, oc.B0PostedWrong)

			if !oc.Met {
				t.Errorf("case %d expected %q, got %s with flags %v",
					oc.Case.Number, oc.Case.ExpectAxiom, oc.Status, oc.Flags)
			}
			if oc.PostedWrong {
				t.Errorf("case %d: Manhattan auto-posted the WRONG batch. "+
					"This is the failure the entire system exists to prevent.", oc.Case.Number)
			}
		})

		if oc.Posted {
			manhattanPosted++
		}
		if oc.PostedWrong {
			manhattanWrong++
		}
		if oc.B0Posted {
			b0Posted++
		}
		if oc.B0PostedWrong {
			b0Wrong++
		}
	}

	t.Logf("")
	t.Logf("across 11 adversarial cases:")
	t.Logf("  manhattan  posted %2d, of which wrong: %d", manhattanPosted, manhattanWrong)
	t.Logf("  b0         posted %2d, of which wrong: %d", b0Posted, b0Wrong)

	if manhattanWrong != 0 {
		t.Fatalf("manhattan auto-posted %d wrong batches", manhattanWrong)
	}
	// The comparison is the result. A zero reported on its own would be close
	// to tautological, since Manhattan posts only when an identity closes and
	// nothing rivals it. A zero next to B0's count, on identical inputs, is a
	// finding.
	if b0Wrong == 0 {
		t.Errorf("B0 posted nothing wrong across the adversarial suite, which means the suite " +
			"is not adversarial enough to demonstrate anything")
	}
}
