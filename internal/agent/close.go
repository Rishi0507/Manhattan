package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/Rishi0507/manhattan/internal/evidence"
	"github.com/Rishi0507/manhattan/internal/llm"
	"github.com/Rishi0507/manhattan/internal/money"
)

// Closer is the controller in "AI Finance Controller".
//
// Everything else in this repository works one settlement at a time, and a
// controller does not. A controller reads the whole period and answers the
// questions that only exist above a single settlement:
//
//	Which merchants are degrading, and since when?
//	Do these four hundred exceptions have four hundred causes, or three?
//	Which single configuration change recovers the most held value?
//	What needs a human this week, and who?
//
// None of that is arithmetic. Every input to it is arithmetic, and every one
// has already been computed: the status mix per merchant, the exception
// distribution by cause, the remediation counts, the drift findings, the
// held value. What is missing is the step that reads four hundred receipts and
// notices that eighty of them are the same problem wearing different reference
// numbers.
//
// That step is the model's, and it is the one place in this system where the
// model's contribution is not bounded by a closed action vocabulary, because
// the output is a recommendation to a human rather than an instruction to a
// ledger. The close report cannot post, cannot narrow, cannot amend an input
// and cannot alter a single receipt. It is read by a person who then decides.
//
// It is also GRADED. The benchmark injects specific operational conditions,
// two of them, and records what they are. The close report is scored on
// whether it found them from the receipts alone, which turns "the AI writes a
// nice summary" into a number. That grading is in bench.gradeClose.
type Closer struct {
	Provider llm.Provider
}

// NewCloser returns a period-close writer over one provider.
func NewCloser(p llm.Provider) *Closer { return &Closer{Provider: p} }

// RootCause is one systemic finding across the period.
type RootCause struct {
	// Scope is the merchant archetype or "all" the finding applies to.
	Scope string `json:"scope"`
	// Class is the kind of problem, from a closed vocabulary, because a free
	// text cause cannot be scored and cannot be routed.
	Class string `json:"cause_class"`
	// Evidence is what in the receipts led here. The model is required to
	// cite, and a finding with no evidence is dropped by the caller.
	Evidence string `json:"evidence"`
	// Action is what to do, and SettlementsAffected and ValueHeldINR are the
	// model's estimate of the size of the prize. Both are checked against the
	// receipts by the caller rather than trusted.
	Action              string `json:"recommended_action"`
	SettlementsAffected int    `json:"settlements_affected"`
	ValueHeldINR        int64  `json:"value_held_inr"`
}

// CauseClass is the closed vocabulary of systemic findings.
const (
	CauseUnjoinedFeed   = "UNJOINED_FEED"
	CauseWindowTooWide  = "WINDOW_TOO_WIDE"
	CauseWindowTooTight = "WINDOW_TOO_TIGHT"
	CauseNoReference    = "NO_SETTLEMENT_REFERENCE"
	CauseFlatPricing    = "AMOUNTS_DO_NOT_DISTINGUISH"
	CauseReportDefects  = "REPORT_DEFECTS"
	CauseFeePolicy      = "FEE_POLICY_DRIFT"
	CauseNone           = "NOTHING_SYSTEMIC"
)

// AllCauses is offered to the model as an enum.
var AllCauses = []string{
	CauseUnjoinedFeed, CauseWindowTooWide, CauseWindowTooTight,
	CauseNoReference, CauseFlatPricing, CauseReportDefects,
	CauseFeePolicy, CauseNone,
}

const closeSystem = `You are the finance controller closing a settlement period.

Four hundred exceptions do not have four hundred causes. They have three or four,
wearing different reference numbers, and your job is to find them and say what to
do about each one. Nobody downstream will read four hundred receipts. They will
read you.

Everything you are shown has already been computed and verified. You are not being
asked to check arithmetic and you cannot change a posting: this report is read by a
person who then decides. What you are being asked for is the thing nobody else in
the pipeline can do, which is to look across merchants and notice a pattern.

The cause vocabulary, and what each one looks like in the evidence:

UNJOINED_FEED               a merchant whose exceptions are dominated by exact,
                            unexplained residuals, or whose receipts name records
                            in a feed that was never connected. The money moved and
                            the pipeline could not see it.
WINDOW_TOO_WIDE             a merchant whose pools are large for one settlement
                            cycle and whose refusals are UNDERDETERMINED rather
                            than UNRESOLVED. Too many candidates, not too few. The
                            giveaway is a pool several times the declared batch.
WINDOW_TOO_TIGHT            the opposite: nothing reconstructs, residuals look like
                            whole transactions, and widening is what the agent
                            keeps trying.
NO_SETTLEMENT_REFERENCE     the remedy on most receipts is "supply the settlement
                            reference". The leg is a search when it should be a
                            lookup.
AMOUNTS_DO_NOT_DISTINGUISH  high twin mass. Repeated price points. No amount-based
                            method works here and none ever will; the remedy is a
                            reference or a per-payment fee row, never a better
                            matcher.
REPORT_DEFECTS              the gateway's own stated mapping is failing its check
                            on this merchant more than elsewhere.
FEE_POLICY_DRIFT            fee anomalies clustering on one merchant.
NOTHING_SYSTEMIC            use this. A period where the exceptions really are
                            idiosyncratic is a period where inventing a theme is
                            worse than saying so.

Rules:

Cite. Every finding names the figures that led you there. A finding with no
evidence is dropped before anybody reads it.

Rank by recoverable value, not by count. Eighty UNDERDETERMINED settlements worth
forty thousand rupees matter less than nine UNRESOLVED ones worth six lakh.

Separate what we got wrong from what they got wrong. An unjoined feed and a
too-wide window are OUR configuration. A defective settlement report is theirs.
Those go to different people and conflating them wastes a week.

Say what you do not know. You are reading aggregates; if two merchants look alike
and you cannot tell whether it is one cause or two, say that.`

// PeriodClose is the controller's report on a whole run.
type PeriodClose = evidence.PeriodClose

// Close reads the run and writes the period close.
func (c *Closer) Close(ctx context.Context, in CloseInput) (*PeriodClose, llm.Usage, error) {
	var usage llm.Usage

	res, err := c.Provider.Structured(ctx, llm.Request{
		Role:       llm.RoleControl,
		System:     closeSystem,
		User:       in.render(),
		SchemaName: "period_close",
		Schema:     closeSchema(),
	})
	if err != nil {
		return nil, usage, err
	}
	usage.Add(res.Usage)

	var out struct {
		Narrative   string      `json:"narrative"`
		RootCauses  []RootCause `json:"root_causes"`
		Escalations []string    `json:"escalations"`
		Unknowns    string      `json:"what_i_cannot_tell"`
	}
	if err := json.Unmarshal(res.JSON, &out); err != nil {
		return nil, usage, err
	}

	pc := &PeriodClose{
		Narrative: strings.TrimSpace(out.Narrative),
		Unknowns:  strings.TrimSpace(out.Unknowns),
		Provider:  c.Provider.Name(),
	}
	for _, e := range out.Escalations {
		if e = strings.TrimSpace(e); e != "" {
			pc.Escalations = append(pc.Escalations, e)
		}
	}
	for _, rc := range out.RootCauses {
		// A finding with no evidence is dropped rather than published. The
		// requirement to cite is only a requirement if something enforces it.
		if strings.TrimSpace(rc.Evidence) == "" {
			pc.Dropped++
			continue
		}
		if !validCause(rc.Class) {
			pc.Dropped++
			continue
		}
		pc.RootCauses = append(pc.RootCauses, evidence.RootCause{
			Scope: rc.Scope, Class: rc.Class, Evidence: strings.TrimSpace(rc.Evidence),
			Action:      strings.TrimSpace(rc.Action),
			Settlements: rc.SettlementsAffected, ValueINR: rc.ValueHeldINR,
		})
	}
	return pc, usage, nil
}

func validCause(c string) bool {
	for _, k := range AllCauses {
		if k == c {
			return true
		}
	}
	return false
}

// CloseInput is the aggregate view handed to the controller.
//
// Aggregates only. The model does not see four hundred receipts, it sees what
// four hundred receipts add up to, which is both what a controller actually
// works from and what keeps this to a single bounded call.
type CloseInput struct {
	Settlements int
	Posted      int
	PostedWrong int
	Held        int
	HeldValue   money.Paise
	HeldCost    int

	ByArchetype []ArchetypeView
	ByCause     []CauseView
	Remedies    []RemedyView
	Drift       []string
}

// ArchetypeView is one merchant type, as the controller sees it.
type ArchetypeView struct {
	Name         string
	Settlements  int
	Posted       int
	FromProof    int
	FromClaim    int
	Held         int
	MeanPoolN    float64
	MeanBatchN   float64
	MeanTwinMass float64
	MeanIndex    float64
	HeldValue    money.Paise
	HeldCost     int
	StatusMix    map[string]int
	ClaimFails   int
	FeeAnomalies int
	ExactResidua int
}

// CauseView is one claim string, aggregated.
type CauseView struct {
	Cause       string
	Settlements int
	ValueINR    int64
}

// RemedyView is one computed remedy, aggregated.
type RemedyView struct {
	Action      string
	Settlements int
	ValueINR    int64
}

func (in CloseInput) render() string {
	var b strings.Builder
	fmt.Fprintf(&b, "PERIOD: %d settlements. %d posted, %d of those wrong. %d held.\n",
		in.Settlements, in.Posted, in.PostedWrong, in.Held)
	fmt.Fprintf(&b, "Held value %s, estimated %d INR of analyst time to clear.\n\n",
		in.HeldValue, in.HeldCost)

	fmt.Fprintf(&b, "BY MERCHANT TYPE\n")
	for _, a := range in.ByArchetype {
		fmt.Fprintf(&b, "\n  %s: %d settlements, %d posted (%d proved, %d claim-checked), %d held\n",
			a.Name, a.Settlements, a.Posted, a.FromProof, a.FromClaim, a.Held)
		fmt.Fprintf(&b, "    mean pool after narrowing %.0f candidates for a mean batch of %.0f\n",
			a.MeanPoolN, a.MeanBatchN)
		fmt.Fprintf(&b, "    twin mass %.2f, collision index %.3g\n", a.MeanTwinMass, a.MeanIndex)
		fmt.Fprintf(&b, "    held value %s, clearing cost %d INR\n", a.HeldValue, a.HeldCost)
		var mix []string
		for _, k := range []string{
			"VERIFIED", "AMBIGUOUS", "UNDERDETERMINED", "NARROWING_SENSITIVE", "UNRESOLVED",
		} {
			if n := a.StatusMix[k]; n > 0 {
				mix = append(mix, fmt.Sprintf("%s %d", k, n))
			}
		}
		fmt.Fprintf(&b, "    statuses: %s\n", strings.Join(mix, ", "))
		fmt.Fprintf(&b, "    settlements whose report claim failed its check: %d\n", a.ClaimFails)
		fmt.Fprintf(&b, "    settlements with a fee anomaly: %d\n", a.FeeAnomalies)
		fmt.Fprintf(&b, "    settlements where NOTHING reconstructs and the residual is exact: %d\n",
			a.ExactResidua)
	}

	fmt.Fprintf(&b, "\nEXCEPTIONS BY CAUSE\n")
	for _, c := range in.ByCause {
		fmt.Fprintf(&b, "  %5d settlements, %10d INR held : %s\n", c.Settlements, c.ValueINR, c.Cause)
	}

	if len(in.Remedies) > 0 {
		fmt.Fprintf(&b, "\nREMEDIES ALREADY COMPUTED, each re-verified by re-running the full stack\n")
		for _, r := range in.Remedies {
			fmt.Fprintf(&b, "  %5d settlements, %10d INR held : %s\n",
				r.Settlements, r.ValueINR, r.Action)
		}
	}
	for _, d := range in.Drift {
		fmt.Fprintf(&b, "\nRUN-LEVEL DRIFT: %s\n", d)
	}

	fmt.Fprintf(&b, "\nWrite the close. Rank by recoverable value. Cite figures. Separate what "+
		"this side configured wrongly from what the counterparty sent wrongly.\n")
	return b.String()
}

func closeSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"narrative": map[string]any{
				"type": "string",
				"description": "The close, in at most six sentences. What happened this period, " +
					"what it cost, and what to do first.",
			},
			"root_causes": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"scope": map[string]any{
							"type":        "string",
							"description": "The merchant archetype this applies to, or 'all'.",
						},
						"cause_class": map[string]any{
							"type": "string", "enum": AllCauses,
						},
						"evidence": map[string]any{
							"type": "string",
							"description": "The specific figures that led here. Required; a " +
								"finding without it is dropped before anybody reads it.",
						},
						"recommended_action":   map[string]any{"type": "string"},
						"settlements_affected": map[string]any{"type": "integer"},
						"value_held_inr":       map[string]any{"type": "integer"},
					},
					"required": []string{
						"scope", "cause_class", "evidence", "recommended_action",
						"settlements_affected", "value_held_inr",
					},
					"additionalProperties": false,
				},
			},
			"escalations": map[string]any{
				"type":        "array",
				"items":       map[string]any{"type": "string"},
				"description": "What needs a human decision this week, and from whom.",
			},
			"what_i_cannot_tell": map[string]any{
				"type": "string",
				"description": "What the aggregates do not settle. Saying two merchants may " +
					"share one cause or may have two is more useful than guessing.",
			},
		},
		"required":             []string{"narrative", "root_causes", "escalations", "what_i_cannot_tell"},
		"additionalProperties": false,
	}
}

// SortCauses orders findings by the value they would recover.
func SortCauses(cs []evidence.RootCause) {
	sort.SliceStable(cs, func(i, j int) bool { return cs[i].ValueINR > cs[j].ValueINR })
}
