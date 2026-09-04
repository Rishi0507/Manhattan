package bench

import (
	"context"

	"github.com/Rishi0507/manhattan/internal/llm"
)

// SensitivityPoint is one run of the batch under a different amount of
// operational misconfiguration.
type SensitivityPoint struct {
	// Scenario names the condition, and SlackScale is how much of the modelled
	// window misconfiguration was applied. 0 is a correctly configured
	// deployment.
	Scenario   string  `json:"scenario"`
	SlackScale float64 `json:"window_slack_scale"`
	DefectRate float64 `json:"report_defect_rate"`
	NaiveFees  bool    `json:"missing_fee_rows_priced_naively"`

	Verified   int            `json:"verified"`
	Wrong      int            `json:"auto_posted_wrong"`
	Repaired   int            `json:"agent_repaired"`
	RepairedBy map[string]int `json:"agent_repairs_by_action"`
	Cures      int            `json:"agent_proven_cures"`
	Invoked    int            `json:"agent_invoked"`
	M1Posted   int            `json:"m1_auto_posted"`
	M1Wrong    int            `json:"m1_auto_posted_wrong"`
	M1FalseAl  int            `json:"m1_false_alarms"`
	B1Wrong    int            `json:"b1_auto_posted_wrong"`
	Defects    int            `json:"report_defects_present"`
}

// Sensitivity answers the only fair question about an agent benchmark: how
// much of what the agent achieves is an artifact of conditions the author
// chose?
//
// The answer is published as a curve rather than defended in prose. The batch
// is re-run with the modelled misconfiguration scaled from zero upwards, and
// the agent's contribution is reported at each point.
//
// At scale 0 every merchant is configured correctly, every feed is joined, and
// the agent should repair NOTHING through narrowing. That is not the loop
// failing, it is the loop being unnecessary, and a reader is entitled to see
// it rather than take it on assurance.
//
// The same sweep varies the report defect rate, because the composite's value
// depends on how often a gateway report is wrong and that rate is a modelling
// assumption too. If a real gateway's rate is a tenth of the one modelled here
// the volume argument shrinks by a factor of ten, and the structural argument
// does not move at all: a reconciliation whose only check on the report is the
// report cannot detect a defective one at ANY rate, including zero.
func Sensitivity(ctx context.Context, base BatchSpec, provider llm.Provider) []SensitivityPoint {
	type scen struct {
		name       string
		slackScale float64
		defect     float64
		naiveFees  bool
	}
	scens := []scen{
		{"correctly configured, reports clean", 0, 0, false},
		{"correctly configured, reports as modelled", 0, 0.06, false},
		{"window misconfiguration as modelled", 1, 0.06, false},
		{"window misconfiguration, reports ten times cleaner", 1, 0.006, false},
		{"window misconfiguration twice as bad", 1.5, 0.06, false},
		// The control that makes the false-alarm rate a measurement. Missing
		// fee rows priced at the configured schedule rather than at the rate
		// the merchant's own report demonstrates.
		{"missing fee rows priced naively", 1, 0.06, true},
	}

	var out []SensitivityPoint
	for _, sc := range scens {
		spec := base
		// Large enough that a merchant can accumulate the twelve proved
		// settlements a profile requires. At 180 no profile ever built, so the
		// narrowing action could not fire and the sweep silently measured only
		// the feed path.
		spec.Settlements = 360
		spec.ReportDefectRate = &sc.defect
		spec.NaiveFeeFallback = sc.naiveFees
		spec.WindowSlack = map[string]float64{}
		for m, h := range windowSlackHours {
			if sc.slackScale > 0 {
				spec.WindowSlack[m] = 14 + (h-14)*sc.slackScale
			}
		}
		_, sum, err := RunBatch(ctx, spec, provider)
		if err != nil {
			continue
		}
		out = append(out, SensitivityPoint{
			Scenario:   sc.name,
			SlackScale: sc.slackScale,
			DefectRate: sc.defect,
			NaiveFees:  sc.naiveFees,
			Verified:   sum.AutoPosted,
			Wrong:      sum.AutoPostedWrong,
			Repaired:   sum.AgentRepaired,
			RepairedBy: sum.AgentRepairedByAction,
			Cures:      sum.AgentProvenCures,
			Invoked:    sum.AgentInvoked,
			M1Posted:   sum.M1Posted,
			M1Wrong:    sum.M1PostedWrong,
			M1FalseAl:  sum.M1FalseAlarms,
			B1Wrong:    sum.B1PostedWrong,
			Defects:    sum.B1DefectiveRpts,
		})
	}
	return out
}
