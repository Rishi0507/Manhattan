package agent

import (
	"testing"
	"time"

	"github.com/Rishi0507/manhattan/internal/evidence"
	"github.com/Rishi0507/manhattan/internal/model"
	"github.com/Rishi0507/manhattan/internal/money"
)

// The corroboration rule, as a test rather than as an anecdote.
//
// The rule that only a corroborated action may post was learned from a
// measured failure: an agent allowed to retune narrowing produced two wrong
// postings in three hundred settlements, because it tightened a window, the
// pool fell from 44 records to 40, an AMBIGUOUS settlement became VERIFIED,
// and the answer was wrong since the tightening had cut real records out of
// the batch.
//
// That story was told in three documents and its evidence was a build that no
// longer exists. A load-bearing claim with a dead witness is worth very little,
// so the failure is rebuilt here and the property that prevents it is asserted
// directly. If somebody makes TIGHTEN_WINDOW postable again, this fails.
func TestOnlyCorroboratedActionsMayPost(t *testing.T) {
	// The postable set is small and deliberate. Each member must be able to
	// point at something outside the model's own judgement.
	postable := map[ActionKind]string{
		ActionSearchFeed:      "cites a real record, by id, in a real feed",
		ActionNarrowToHistory: "bounded by this merchant's own prior VERIFIED settlements",
	}

	for _, k := range AllActions {
		why, want := postable[k]
		if got := k.Corroborated(); got != want {
			if want {
				t.Errorf("%s must be postable: %s", k, why)
			} else {
				t.Errorf("%s must NOT be postable. It changes a filter or asserts an "+
					"unmodelled event, and removing candidates cannot make the survivor "+
					"unique, only unexamined. This is the rule that took two wrong "+
					"postings in three hundred settlements to learn", k)
			}
		}
	}

	// The specific action that caused the original failure.
	if ActionTightenWindow.Corroborated() {
		t.Fatal("TIGHTEN_WINDOW is postable again. This is exactly the configuration " +
			"that tightened a window from 44 records to 40, turned an AMBIGUOUS " +
			"settlement into a VERIFIED one, and posted a batch the tightening had " +
			"cut real records out of")
	}
}

// TestNarrowToHistoryRefusesToOutrunItsEvidence is the other half of the rule.
//
// NARROW_TO_HISTORY may post, which makes the bound it proposes load-bearing.
// The whole safety argument is that the bound can never be tighter than the
// merchant's own proved settlements demonstrate, because a tighter one would
// cut out records this system has already shown belong to a batch. That is the
// property under test.
func TestNarrowToHistoryRefusesToOutrunItsEvidence(t *testing.T) {
	valueDate := time.Date(2026, 8, 26, 0, 0, 0, 0, time.UTC)
	// Capture-day midpoint for a T+2 merchant, which is what narrowing centres
	// on and therefore what an offset has to be measured from.
	centre := valueDate.AddDate(0, 0, -2).Add(12 * time.Hour)

	byID := map[string]model.Record{}
	var proved []*evidence.Receipt
	for i := 0; i < MinProofsForProfile+4; i++ {
		id := string(rune('a'+i)) + "_pay"
		// Every proved record sits 9 hours from the centre, so 9h is exactly
		// what this history supports and 8h is not.
		byID[id] = model.Record{
			ID: id, MerchantID: "mid_test", Contribution: money.Paise(100 + i),
			EventAt: centre.Add(-9 * time.Hour), Instrument: model.InstrumentCard,
		}
		proved = append(proved, &evidence.Receipt{
			SettlementRef: id + "_stl",
			MerchantID:    "mid_test",
			Status:        evidence.StatusVerified,
			ValueDate:     valueDate.Format("2006-01-02"),
			Witness:       []string{id},
		})
	}

	p := BuildProfile("mid_test", proved, byID, 2)
	if p == nil {
		t.Fatal("a merchant with more than the minimum proofs should have a profile")
	}
	if got := p.MaxOffsetHours; got < 8.9 || got > 9.1 {
		t.Fatalf("widest observed offset should be 9h, got %.2f", got)
	}

	if ok, why := p.Supports(9); !ok {
		t.Errorf("9h is exactly what the history shows and must be supported: %s", why)
	}
	if ok, _ := p.Supports(12); !ok {
		t.Error("a window wider than the history shows is always safe and must be supported")
	}
	if ok, why := p.Supports(8); ok {
		t.Errorf("8h is tighter than the history supports and must be refused, because it "+
			"would cut out records already proved to belong to a batch; got supported with %q", why)
	}

	// Too few proofs is not a small profile, it is no profile.
	if p := BuildProfile("mid_test", proved[:MinProofsForProfile-1], byID, 2); p != nil {
		t.Errorf("a merchant with %d proofs must have no profile at all, since %d are required",
			MinProofsForProfile-1, MinProofsForProfile)
	}
	var none *Profile
	if ok, _ := none.Supports(1); ok {
		t.Error("a nil profile must corroborate nothing")
	}
}

// TestProfileIgnoresUnprovedSettlements stops the profile bootstrapping from
// guesses, which would make its corroboration circular.
func TestProfileIgnoresUnprovedSettlements(t *testing.T) {
	valueDate := time.Date(2026, 8, 26, 0, 0, 0, 0, time.UTC)
	byID := map[string]model.Record{
		"p1": {ID: "p1", MerchantID: "m", Contribution: 100, EventAt: valueDate},
	}
	var rs []*evidence.Receipt
	for i := 0; i < 40; i++ {
		rs = append(rs, &evidence.Receipt{
			MerchantID: "m",
			Status:     evidence.StatusAmbiguous,
			ValueDate:  valueDate.Format("2006-01-02"),
			Witness:    []string{"p1"},
		})
	}
	if p := BuildProfile("m", rs, byID, 2); p != nil {
		t.Fatal("forty AMBIGUOUS settlements must not build a profile. A history of " +
			"unproved answers corroborates nothing, and letting it would make the " +
			"corroboration rule circular")
	}
}
