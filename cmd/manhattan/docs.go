package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"text/template"

	"github.com/Rishi0507/manhattan/internal/agent"
	"github.com/Rishi0507/manhattan/internal/bench"
	"github.com/Rishi0507/manhattan/internal/evidence"
	"github.com/Rishi0507/manhattan/internal/llm"
	"github.com/Rishi0507/manhattan/internal/money"
)

// The document generator.
//
// README.md and LIMITATIONS.md are rendered from templates in docs/ against
// the artifacts of one run. Nothing numeric is typed into either file.
//
// The rule this enforces is narrow and it was learned by breaking it: a
// benchmark was re-run, RESULTS.md regenerated, and the other two documents
// left describing the previous run. Roughly thirty figures across them went
// stale in one command, including the headline, in a project whose entire
// argument is that it does not publish claims it has not verified. A reader
// who checked any one of them would have been right to stop reading.
//
// So a figure that appears in more than one place is computed in exactly one
// place. If a number is in a document, it came from a run, and re-running
// updates every copy of it at once or none of them.

// Derived holds the figures that appear in prose rather than in a table, and
// the cross-checks that only mean something once two independently produced
// numbers are put beside each other.
type Derived struct {
	// The agent queue as a flow, because the counts do not add up when read
	// as a partition and a reader who tries will conclude something is wrong.
	ExceptionsEntered int // invoked + settled by triage
	TriagePct         float64
	AgentPct          float64
	StillHeld         int

	PostRate        float64
	B0PostRate      float64
	B0WrongOfPosted float64
	WrongPerHundred float64

	// The estimator comparison, scoped to what actually produced it.
	SweptConfigs      int
	EmpiricalLogRatio float64
	AnalyticLogRatio  float64
	EstimatorRatio    float64

	// Throughput, both ways, because a reader will divide one into the other.
	PerHourMeasured float64
	MsPerSettlement float64
	MedianMS        float64

	// Cost, fully decomposed.
	INRPer1k      float64
	B0INRPer1k    float64
	CostRatio     float64
	USDPer1k      float64
	B0USDPer1k    float64
	InputPerSettl float64

	// B0's tuning curve, reduced to the points that matter.
	B0Shipped bench.B0ThresholdPoint
	B0Best    bench.B0ThresholdPoint
	// B0MaxRight is the most correct postings the baseline achieves at ANY
	// threshold. It falls out of the sweep for free and it is the strongest
	// single number in the comparison: tuned however you like, the baseline
	// never gets more right answers than Manhattan does. It only ever adds
	// wrong ones it cannot identify.
	B0MaxRight    int
	B0MaxRightAt  float64
	ManhattanEdge int

	// Cross-checks between counters that are incremented in different places
	// and have no reason to agree unless the instrumentation is sound.
	HypothesisFlag   int
	EntropyFlag      int
	EntropyArchCount int
	PerArchetype     int
	CrossChecksHold  bool

	// TotalSweptConfigs is every configuration in the sweep. SweptConfigs
	// below is the SUBSET where exhaustive counting ran, which is what the
	// estimator can be scored against. Two different numbers describing "the
	// sweep" is exactly the drift this project claims to prevent, so they are
	// named apart and each claim uses the one it means.
	TotalSweptConfigs int

	// The composite, which is the product.
	M1Posted        int
	M1Wrong         int
	M1FromProof     int
	M1FromClaim     int
	M1ProofSharePct float64
	M1Held          int
	M1PostRate      float64
	M1FalseAlarms   int
	M1CleanChecked  int
	M1FalseAlarmPct float64
	M1Contradicted  int
	M1Uncheckable   int
	M1PostedDefect  int
	ClaimUplift     int

	// What the agent contributed, and under what conditions.
	RepairsByAction []FlagRow
	Conditions      []string
	Sensitivity     []bench.SensitivityPoint
	NarrowRepairs   int
	FeedRepairs     int
	ControlNarrow   int
	ControlDefects  int
	B1WrongAtTenth  int
	DefectsAtTenth  int

	PeakSampledMB float64
	PeakSolverMB  float64
	Host          string

	// TwinSwapInBatch says whether the flag appears in the batch, as opposed
	// to in the eleven-case fixture.
	TwinSwapInBatch bool

	// Confusions renders the diagnosis errors as one sentence rather than one
	// per pair, which read as a stutter when a map was ranged over directly.
	Confusions string

	// The period close, which is the controller doing the job the track names.
	Close          *evidence.PeriodClose
	CloseRecallPct float64
	CloseFound     int
	CloseInjected  int

	// RoleRows is where the model is used, per job, with what it contributes
	// and whether that contribution is graded. This is the table that answers
	// "the AI does almost nothing" with an accounting rather than a rebuttal.
	RoleRows []RoleRow

	// The live-versus-stub delta, present only once `manhattan live` has run.
	HasLive         bool
	LiveSettlements int
	LiveWrong       int
	StubWrong       int
	LiveM1Wrong     int
	StubM1Wrong     int
	LiveVerified    int
	StubVerified    int
	LiveRepairs     int
	StubRepairs     int
	LiveDiagAcc     float64
	StubDiagAcc     float64
	LiveINR         float64
	ModelledINR     float64
	LiveCloseRecall float64
	StubCloseRecall float64

	// The lookup, which is the answer everybody gives.
	B1Posted        int
	B1Wrong         int
	B1CorrectPct    float64
	Defects         int
	DefectsCaught   int
	DefectsMissed   int
	DefectCatchPct  float64
	DefectRatePct   float64
	B1SilentPer1000 float64

	// The queue, and whether ordering it means anything.
	ExceptionCostMin   int
	ExceptionCostMax   int
	ExceptionMinutes   int
	ExceptionHours     float64
	ExceptionValueINR  float64
	CostSpreadRatio    float64
	QueueOrderedByWhat string

	// Calibration, scoped to what the data supports.
	CardinalityBands []bench.CardinalityBand
	MonotoneAt       []int
	NotMonotoneAt    []int
	AnyBandWrong     bool

	// The archetypes that post nothing, named rather than buried.
	ZeroArchetypes []string

	// The economics of refusing.
	ExceptionCostINR    int
	WrongPostingCostINR int
	RemediationEachINR  int
	NetINR              int
	// BreakEvenINR is what unwinding one wrong posting would have to cost for
	// the baseline's coverage to be worth its error rate. Printing it lets a
	// reader who rejects the assumed figure test their own in one comparison.
	BreakEvenINR float64
	// UnnarrowedTokens is what a matcher without Manhattan's narrowing would
	// pay per settlement, which is the honest counterfactual for B0's cost.
	UnnarrowedTokens float64

	StatusVerified        int
	StatusAmbiguous       int
	StatusUnderdetermined int
	StatusNarrowing       int
	StatusUnresolved      int
	AdversarialMet        int
	AdversarialTotal      int
	AdversarialB0Wrong    int
	CasesManhattanPosted  int
	CasesManhattanWrong   int
	CasesB0Posted         int
	TwinSwapInCases       bool
	PeakEnvelopeMB        float64
	// TopBandB0Wrong is the baseline's wrong-posting rate in the highest
	// collision-index band, which is the end of the calibration curve.
	TopBandB0Wrong         float64
	QATranscript           []QAExchange
	GeneratedFromRun       string
	FlagRows               []FlagRow
	AgentRepairedCrossFlag bool
}

// RoleRow is one model job: how many calls it made, what it contributes that
// nothing else can, and whether its output is graded against ground truth.
//
// The descriptions are fixed here rather than generated, because they are
// claims about the architecture rather than measurements. The call counts and
// the grading are measurements.
type RoleRow struct {
	Role   string
	Calls  int
	What   string
	Graded string
}

// roleFacts describes each job. Ordered by what a reader should read first,
// which is not the same as by volume.
var roleFacts = []struct {
	role, what, graded string
}{
	{"control", "reads the WHOLE period and writes the close: which merchants are degrading, whether four hundred exceptions have four hundred causes or three, which single change recovers the most held value, and what needs a human this week. The only output here that works above a single settlement, and the only one not bounded by a closed action vocabulary, because it cannot act", "**yes**, on whether it found the operational conditions this run injected"},
	{"triage", "names WHY a report's stated mapping failed its arithmetic check, from a closed vocabulary of five defect classes. The same failed check has several causes needing different remedies, and telling them apart is reading rather than counting", "**yes**, against the generator's record of what it injected"},
	{"resolve", "proposes the class of unmodelled event behind an exact residual, so the system knows what kind of record to go and look for", "indirectly: a proposal only clears an exception if the verifier re-proves it"},
	{"plan", "chooses one action from a closed set of eight for a settlement that did not post, including whether this merchant's own proved history corroborates a tighter window", "indirectly: the entire stack re-runs and rejects anything that did not improve"},
	{"remediate", "drafts the analyst-facing note: what to do, why it works in terms of what was measured, and what it will not fix. Facts supplied, figures substituted afterwards", "no, and it carries no safety risk: the settlement is held either way"},
	{"parse", "reads an unstructured bank narration into typed fields. The highest volume and the lowest difficulty, and the one job a gateway would replace with a lookup table tomorrow", "indirectly: a mis-parse produces an exception, never a posting"},
	{"answer", "answers a finance lead's question from the receipt store, or declines because the receipts do not record it", "no; declining correctly is the property that matters"},
	{"explain", "renders a derivation into readable English from facts it is given", "no"},
}

// FlagRow is one flag with its count, sorted, so the template does not have
// to range over a map in a nondeterministic order.
type FlagRow struct {
	Flag  string
	Count int
}

// QAExchange is one recorded question and answer against the receipt store.
type QAExchange struct {
	Question   string
	Answer     string
	Answerable bool
	Citations  []string
	Path       string
}

// docData is what the templates see.
type docData struct {
	S           bench.Summary
	Cases       []bench.CaseOutcome
	Sweep       []bench.SweepPoint
	Buckets     []bench.Bucket
	Envelope    []bench.EnvelopePoint
	Sensitivity []bench.SensitivityPoint
	D           Derived
}

// The cost of unwinding one wrong posting, stated as an assumption rather
// than smuggled in as a fact.
//
// A wrong settlement posting is not corrected by an edit. Somebody has to
// notice it, usually at month end and usually from a reconciliation
// difference rather than from the posting itself; find which credit it
// belonged to; reverse the journal; re-post; and explain the movement to
// whoever signs the accounts. Two hours of a mid-level finance analyst at
// Indian metro rates is the conservative end of that, and it excludes the
// cases that reach an auditor.
//
// The number is printed everywhere it is used, so a reader who thinks it is
// wrong can substitute their own and redo the arithmetic in one step.
const remediationCostINR = 2400

func deriveDocs(sum bench.Summary, cases []bench.CaseOutcome, sweep []bench.SweepPoint, env []bench.EnvelopePoint) Derived {
	var d Derived
	d.GeneratedFromRun = sum.RunID

	d.ExceptionsEntered = sum.AgentInvoked + sum.AgentSkipped
	if d.ExceptionsEntered > 0 {
		d.TriagePct = 100 * float64(sum.AgentSkipped) / float64(d.ExceptionsEntered)
		d.AgentPct = 100 * float64(sum.AgentInvoked) / float64(d.ExceptionsEntered)
	}
	d.StillHeld = sum.Exceptions

	if sum.Settlements > 0 {
		d.PostRate = 100 * float64(sum.AutoPosted) / float64(sum.Settlements)
		d.B0PostRate = 100 * float64(sum.B0Posted) / float64(sum.Settlements)
		d.MsPerSettlement = 3600000 / math.Max(sum.PerHour, 1)
		d.InputPerSettl = float64(sum.InputTokens) / float64(sum.Settlements)
	}
	if sum.B0Posted > 0 {
		d.B0WrongOfPosted = 100 * float64(sum.B0PostedWrong) / float64(sum.B0Posted)
	}
	d.WrongPerHundred = d.B0WrongOfPosted
	d.PerHourMeasured = sum.PerHour
	d.MedianMS = sum.MedianLatencyMS

	d.INRPer1k = sum.INRPer1k
	d.B0INRPer1k = sum.B0INRPer1k
	if sum.INRPer1k > 0 {
		d.CostRatio = sum.B0INRPer1k / sum.INRPer1k
	}
	if sum.Cost.USDToINR > 0 {
		d.USDPer1k = sum.INRPer1k / sum.Cost.USDToINR
		d.B0USDPer1k = sum.B0INRPer1k / sum.Cost.USDToINR
	}

	for _, p := range sum.B0Sweep {
		if p.Shipped {
			d.B0Shipped = p
		}
		if p.BestF1 {
			d.B0Best = p
		}
		if p.Right > d.B0MaxRight {
			d.B0MaxRight, d.B0MaxRightAt = p.Right, p.Threshold
		}
	}
	d.ManhattanEdge = sum.AutoPosted - d.B0MaxRight

	// The estimator comparison, computed over exactly the configurations that
	// carried both estimates and a counted ground truth.
	var emp, ana float64
	for _, p := range sweep {
		if p.MeanRivals <= 0 || p.MeanIndex <= 0 || p.MeanAnalyticIndex <= 0 {
			continue
		}
		emp += logAbsRatio(p.MeanIndex, p.MeanRivals)
		ana += logAbsRatio(p.MeanAnalyticIndex, p.MeanRivals)
		d.SweptConfigs++
	}
	if d.SweptConfigs > 0 {
		d.EmpiricalLogRatio = emp / float64(d.SweptConfigs)
		d.AnalyticLogRatio = ana / float64(d.SweptConfigs)
		if d.EmpiricalLogRatio > 0 {
			d.EstimatorRatio = d.AnalyticLogRatio / d.EmpiricalLogRatio
		}
	}

	d.StatusVerified = sum.StatusCounts[evidence.StatusVerified]
	d.StatusAmbiguous = sum.StatusCounts[evidence.StatusAmbiguous]
	d.StatusUnderdetermined = sum.StatusCounts[evidence.StatusUnderdetermined]
	d.StatusNarrowing = sum.StatusCounts[evidence.StatusNarrowingSensitive]
	d.StatusUnresolved = sum.StatusCounts[evidence.StatusUnresolved]

	// Two counters that are incremented in different packages, for different
	// reasons, and have no reason to agree unless the instrumentation is
	// telling the truth. Naming them is free evidence and costs one line.
	d.TwinSwapInBatch = sum.FlagCounts[evidence.FlagTwinSwap] > 0
	d.HypothesisFlag = sum.FlagCounts[evidence.FlagResolvedByHypothesis]
	d.EntropyFlag = sum.FlagCounts[evidence.FlagEntropyInsufficient]
	d.AgentRepairedCrossFlag = d.HypothesisFlag == sum.AgentRepaired
	for _, a := range sum.ByArchetype {
		if a.AutoPostRate == 0 {
			d.EntropyArchCount++
			d.PerArchetype = a.Settlements
		}
	}
	d.CrossChecksHold = d.AgentRepairedCrossFlag &&
		d.EntropyFlag == d.EntropyArchCount*d.PerArchetype

	for _, f := range roleFacts {
		if n := sum.CallsByRole[f.role]; n > 0 {
			d.RoleRows = append(d.RoleRows, RoleRow{
				Role: f.role, Calls: n, What: f.what, Graded: f.graded,
			})
		}
	}

	var conf []string
	for truth, row := range sum.DiagnosisConfusion {
		for pred, n := range row {
			if truth != pred {
				conf = append(conf, fmt.Sprintf("%d `%s` read as `%s`", n, truth, pred))
			}
		}
	}
	sort.Strings(conf)
	switch len(conf) {
	case 0:
	case 1:
		d.Confusions = conf[0]
	default:
		d.Confusions = strings.Join(conf[:len(conf)-1], ", ") + " and " + conf[len(conf)-1]
	}

	if pc := sum.Close; pc != nil {
		d.Close = pc
		d.CloseRecallPct = pc.Recall * 100
		d.CloseFound = len(pc.ConditionsFound)
		d.CloseInjected = len(pc.ConditionsInjected)
	}

	d.TotalSweptConfigs = len(sweep)
	d.PeakSampledMB, d.PeakSolverMB, d.Host = sum.PeakMemoryMB, sum.PeakSolverMB, sum.Host
	d.Conditions = sum.Conditions

	d.M1Posted, d.M1Wrong = sum.M1Posted, sum.M1PostedWrong
	d.M1FromProof, d.M1FromClaim, d.M1Held = sum.M1FromProof, sum.M1FromClaim, sum.M1Held
	if sum.Settlements > 0 {
		d.M1ProofSharePct = 100 * float64(sum.M1FromProof) / float64(sum.Settlements)
	}
	d.M1FalseAlarms, d.M1CleanChecked = sum.M1FalseAlarms, sum.M1CleanReports
	d.M1Contradicted, d.M1Uncheckable = sum.M1CaughtDefects, sum.M1HeldUncheckable
	d.M1PostedDefect = sum.M1PostedDefects
	d.ClaimUplift = sum.M1Posted - sum.AutoPosted
	if sum.Settlements > 0 {
		d.M1PostRate = 100 * float64(sum.M1Posted) / float64(sum.Settlements)
	}
	if sum.M1CleanReports > 0 {
		d.M1FalseAlarmPct = 100 * float64(sum.M1FalseAlarms) / float64(sum.M1CleanReports)
	}

	for k, n := range sum.AgentRepairedByAction {
		d.RepairsByAction = append(d.RepairsByAction, FlagRow{Flag: k, Count: n})
		switch k {
		case "NARROW_TO_HISTORY":
			d.NarrowRepairs = n
		case "SEARCH_FEED":
			d.FeedRepairs = n
		}
	}
	sort.Slice(d.RepairsByAction, func(i, j int) bool {
		return d.RepairsByAction[i].Count > d.RepairsByAction[j].Count
	})

	d.B1Posted, d.B1Wrong = sum.B1Posted, sum.B1PostedWrong
	if sum.B1Posted > 0 {
		d.B1CorrectPct = 100 * float64(sum.B1Posted-sum.B1PostedWrong) / float64(sum.B1Posted)
	}
	d.Defects = sum.B1DefectiveRpts
	d.DefectsCaught, d.DefectsMissed = sum.B1CaughtByUs, sum.B1MissedByUs
	if d.Defects > 0 {
		d.DefectCatchPct = 100 * float64(d.DefectsCaught) / float64(d.Defects)
	}
	if sum.Settlements > 0 {
		d.DefectRatePct = 100 * float64(d.Defects) / float64(sum.Settlements)
		d.B1SilentPer1000 = 1000 * float64(sum.B1PostedWrong) / float64(sum.Settlements)
	}

	d.ExceptionCostMin, d.ExceptionCostMax = sum.ExceptionCostMin, sum.ExceptionCostMax
	d.ExceptionMinutes = sum.ExceptionMinutes
	d.ExceptionHours = float64(sum.ExceptionMinutes) / 60
	d.ExceptionValueINR = float64(sum.ExceptionValuePaise) / 100
	if sum.ExceptionCostMin > 0 {
		d.CostSpreadRatio = float64(sum.ExceptionCostMax) / float64(sum.ExceptionCostMin)
	}

	for _, a := range sum.ByArchetype {
		if a.AutoPostRate == 0 {
			d.ZeroArchetypes = append(d.ZeroArchetypes, a.Archetype)
		}
	}

	d.ExceptionCostINR = sum.ExceptionCostINR
	d.RemediationEachINR = remediationCostINR
	d.WrongPostingCostINR = sum.B0PostedWrong * remediationCostINR
	d.NetINR = d.WrongPostingCostINR - d.ExceptionCostINR
	if sum.B0PostedWrong > 0 {
		d.BreakEvenINR = float64(sum.ExceptionCostINR) / float64(sum.B0PostedWrong)
	}
	d.UnnarrowedTokens = float64(sum.Cost.B0Overhead) + sum.Pools.RawMean*float64(sum.Cost.B0PerRecord)

	for _, oc := range cases {
		d.AdversarialTotal++
		if oc.Met && !oc.PostedWrong {
			d.AdversarialMet++
		}
		if oc.PostedWrong {
			d.CasesManhattanWrong++
		}
		if oc.Status == evidence.StatusVerified {
			d.CasesManhattanPosted++
		}
		if oc.B0Posted {
			d.CasesB0Posted++
			if oc.B0PostedWrong {
				d.AdversarialB0Wrong++
			}
		}
		for _, f := range oc.Flags {
			if f == evidence.FlagTwinSwap {
				d.TwinSwapInCases = true
			}
		}
	}

	d.CardinalityBands = bench.CardinalityBands(sweep, 4)
	for _, b := range d.CardinalityBands {
		if b.Monotone {
			d.MonotoneAt = append(d.MonotoneAt, b.BatchSize)
		} else {
			d.NotMonotoneAt = append(d.NotMonotoneAt, b.BatchSize)
		}
		if b.AnyWrong {
			d.AnyBandWrong = true
		}
	}

	for _, bk := range bench.LogSpaced(sweep, 8) {
		if bk.N > 0 {
			d.TopBandB0Wrong = bk.B0Wrong * 100
		}
	}

	for _, e := range env {
		if e.ObservedMB > d.PeakEnvelopeMB {
			d.PeakEnvelopeMB = e.ObservedMB
		}
	}

	type fr struct {
		f evidence.Flag
		n int
	}
	var rows []fr
	for f, n := range sum.FlagCounts {
		rows = append(rows, fr{f, n})
	}
	for i := range rows {
		for j := i + 1; j < len(rows); j++ {
			if rows[j].n > rows[i].n || (rows[j].n == rows[i].n && rows[j].f < rows[i].f) {
				rows[i], rows[j] = rows[j], rows[i]
			}
		}
	}
	for _, r := range rows {
		d.FlagRows = append(d.FlagRows, FlagRow{Flag: string(r.f), Count: r.n})
	}
	return d
}

// recordQA runs three questions against the stored receipts and keeps the
// answers verbatim.
//
// The third one is the point of including this at all. An agent that answers
// every question is not grounded in anything; an agent that declines when the
// receipts do not record the answer is. That distinction is the whole thesis
// in ten lines, so it is exhibited rather than described.
func recordQA(ctx context.Context, store *evidence.Store, p llm.Provider) []QAExchange {
	// Four questions, chosen so that the transcript shows all three paths the
	// Q&A side can take rather than only the cheap one.
	//
	// An earlier version showed three questions that all resolved
	// deterministically, including the declining one, which meant the single
	// place a reader looks for the model was the place the receipt said the
	// model was not there. That was a correct fix to a smaller problem
	// (the decline should not read as a stub limitation) that created a worse
	// one.
	questions := []string{
		// Arithmetic over the store. Answering this with a model would be
		// paying it to add up a column.
		"which constraint dropped the most records?",
		"what is the backlog costing us?",
		// Open-ended. No deterministic handler can answer it, so it goes
		// through retrieval and one grounded model call.
		"why do the quick commerce settlements behave differently from the travel ones, " +
			"and what would I change first?",
		// Unanswerable from the receipts, which is the most important answer
		// this agent gives.
		"which analyst approved settlement 5502?",
	}
	var out []QAExchange
	for _, q := range questions {
		ex := QAExchange{Question: q}
		if ans, ok := agent.Direct(store, q); ok {
			ex.Path = "deterministic, no model call"
			ex.Answer = ans.Text
			ex.Answerable = ans.Answerable
			for _, c := range ans.Citations {
				ex.Citations = append(ex.Citations, citationLine(c))
			}
			out = append(out, ex)
			continue
		}
		ex.Path = "retrieval over receipts, then one grounded model call"
		ans, err := agent.NewQA(p, store).Ask(ctx, q)
		if err != nil {
			ex.Answer = "the question could not be answered from the receipt store"
			out = append(out, ex)
			continue
		}
		ex.Answer = ans.Text
		ex.Answerable = ans.Answerable
		for _, c := range ans.Citations {
			ex.Citations = append(ex.Citations, citationLine(c))
		}
		out = append(out, ex)
	}
	return out
}

func citationLine(c agent.Citation) string {
	if c.Value != "" {
		return fmt.Sprintf("%s · %s = %s", c.ReceiptID, c.Field, c.Value)
	}
	return fmt.Sprintf("%s · %s", c.ReceiptID, c.Field)
}

var docFuncs = template.FuncMap{
	"pct":  func(f float64) string { return fmt.Sprintf("%.0f%%", f) },
	"pct1": func(f float64) string { return fmt.Sprintf("%.1f%%", f) },
	"rate": func(f float64) string { return fmt.Sprintf("%.0f%%", f*100) },
	"f1":   func(f float64) string { return fmt.Sprintf("%.1f", f) },
	"f2":   func(f float64) string { return fmt.Sprintf("%.2f", f) },
	"f3g":  func(f float64) string { return fmt.Sprintf("%.3g", f) },
	"i":    func(f float64) string { return fmt.Sprintf("%.0f", f) },
	// ni is a rounded float with thousands separators, for figures a reader
	// reads as a quantity rather than as a measurement.
	"ni":    func(f float64) string { return commas(int64(f + 0.5)) },
	"n":     func(n int) string { return commas(int64(n)) },
	"n64":   func(n int64) string { return commas(n) },
	"lower": strings.ToLower,
	"mul":   func(a, b float64) float64 { return a * b },
	"div": func(a, b float64) float64 {
		if b == 0 {
			return 0
		}
		return a / b
	},
	"add":    func(a, b int) int { return a + b },
	"sub":    func(a, b int) int { return a - b },
	"trim":   strings.TrimSpace,
	"sub100": func(f float64) float64 { return 100 - f },
	"float":  func(p money.Paise) float64 { return float64(p) },
	"f0":     func(f float64) string { return fmt.Sprintf("%.0f", f) },
	"ints": func(xs []int) string {
		var parts []string
		for _, x := range xs {
			parts = append(parts, fmt.Sprintf("%d", x))
		}
		switch len(parts) {
		case 0:
			return "none"
		case 1:
			return parts[0]
		case 2:
			return parts[0] + " and " + parts[1]
		}
		return strings.Join(parts[:len(parts)-1], ", ") + " and " + parts[len(parts)-1]
	},
	"words": func(xs []string) string {
		for i, x := range xs {
			xs[i] = strings.ReplaceAll(x, "_", " ")
		}
		switch len(xs) {
		case 0:
			return "none"
		case 1:
			return xs[0]
		case 2:
			return xs[0] + " and " + xs[1]
		}
		return strings.Join(xs[:len(xs)-1], ", ") + " and " + xs[len(xs)-1]
	},
	// first takes the head of any slice, so the README can show the top of a
	// table whose full version lives in RESULTS.md.
	"first": func(n int, v any) any {
		rv := reflect.ValueOf(v)
		if !rv.IsValid() || rv.Kind() != reflect.Slice || rv.Len() <= n {
			return v
		}
		return rv.Slice(0, n).Interface()
	},
	// clip keeps a table cell to one line. Causes and remediations are written
	// as full sentences on the receipt, which is right there and wrong in a
	// column, and the receipt remains the authority either way.
	"clip": func(s string, n int) string {
		s = strings.ReplaceAll(strings.TrimSpace(s), "|", "/")
		if len(s) <= n {
			return s
		}
		cut := strings.LastIndex(s[:n], " ")
		if cut < n/2 {
			cut = n
		}
		return s[:cut] + "..."
	},
	// knees reduces the threshold sweep to the points where behaviour actually
	// changes, plus the two marked ones. Nineteen rows of which twelve are
	// identical is a table nobody reads, and the shape of the curve is the
	// claim rather than its sampling density.
	"knees": func(pts []bench.B0ThresholdPoint) []bench.B0ThresholdPoint {
		var out []bench.B0ThresholdPoint
		for i, p := range pts {
			keep := i == 0 || i == len(pts)-1 || p.Shipped || p.BestF1
			if i > 0 && (p.Posted != pts[i-1].Posted || p.Wrong != pts[i-1].Wrong) {
				keep = true
			}
			if keep {
				out = append(out, p)
			}
		}
		return out
	},
	"indent": func(pre, s string) string {
		lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
		for i, l := range lines {
			lines[i] = pre + l
		}
		return strings.Join(lines, "\n")
	},
}

// renderDoc executes one template against the run's figures.
func renderDoc(tmplPath, outPath string, data docData) error {
	raw, err := os.ReadFile(tmplPath)
	if err != nil {
		return fmt.Errorf("read %s: %w", tmplPath, err)
	}
	t, err := template.New(filepath.Base(tmplPath)).Funcs(docFuncs).Parse(string(raw))
	if err != nil {
		return fmt.Errorf("parse %s: %w", tmplPath, err)
	}
	var b strings.Builder
	if err := t.Execute(&b, data); err != nil {
		return fmt.Errorf("execute %s: %w", tmplPath, err)
	}
	out := b.String()
	if strings.Contains(out, "—") {
		return fmt.Errorf("%s rendered an em dash, which the house style forbids", outPath)
	}
	return os.WriteFile(outPath, []byte(out), 0o644)
}

// renderNarrativeDocs writes README.md and LIMITATIONS.md from the run.
func renderNarrativeDocs(ctx context.Context, sum bench.Summary, cases []bench.CaseOutcome,
	sweep []bench.SweepPoint, env []bench.EnvelopePoint, sens []bench.SensitivityPoint,
	store *evidence.Store, p llm.Provider) error {

	d := deriveDocs(sum, cases, sweep, env)
	d.Sensitivity = sens

	// The live-versus-stub delta, folded in only if `manhattan live` has been
	// run. Absent by default, and the documents say so plainly rather than
	// leaving a reader to notice.
	if b, err := os.ReadFile(filepath.Join("out", "live.json")); err == nil {
		var ld bench.LiveDelta
		if json.Unmarshal(b, &ld) == nil && ld.Settlements > 0 {
			d.HasLive = true
			d.LiveSettlements = ld.Settlements
			d.LiveWrong, d.StubWrong = ld.LiveWrong, ld.StubWrong
			d.LiveM1Wrong, d.StubM1Wrong = ld.LiveM1Wrong, ld.StubM1Wrong
			d.LiveVerified, d.StubVerified = ld.LiveVerified, ld.StubVerified
			d.LiveRepairs, d.StubRepairs = ld.LiveRepairs, ld.StubRepairs
			d.LiveDiagAcc, d.StubDiagAcc = ld.LiveDiagnosisAcc, ld.StubDiagnosisAcc
			d.LiveINR, d.ModelledINR = ld.LiveINRPer1k, ld.ModelledPer1k
			d.LiveCloseRecall, d.StubCloseRecall = ld.LiveCloseRecall, ld.StubCloseRecall
		}
	}
	for _, sp := range sens {
		if sp.SlackScale == 0 && sp.DefectRate == 0 {
			d.ControlNarrow = sp.RepairedBy["NARROW_TO_HISTORY"]
		}
		if sp.SlackScale > 0 && sp.DefectRate > 0 && sp.DefectRate < 0.01 {
			d.B1WrongAtTenth, d.DefectsAtTenth = sp.B1Wrong, sp.Defects
		}
	}
	if store != nil {
		d.QATranscript = recordQA(ctx, store, p)
	}
	data := docData{
		S: sum, Cases: cases, Sweep: sweep,
		Buckets: bench.LogSpaced(sweep, 8), Envelope: env, Sensitivity: sens, D: d,
	}
	for _, pair := range [][2]string{
		{filepath.Join("docs", "README.tmpl.md"), "README.md"},
		{filepath.Join("docs", "LIMITATIONS.tmpl.md"), "LIMITATIONS.md"},
	} {
		if _, err := os.Stat(pair[0]); os.IsNotExist(err) {
			continue
		}
		if err := renderDoc(pair[0], pair[1], data); err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "rendered %s from %s\n", pair[1], pair[0])
	}
	return nil
}

// runDocs re-renders the narrative documents from saved artifacts, without
// re-running the benchmark.
func runDocs(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("docs", flag.ExitOnError)
	dir := fs.String("in", "out", "directory holding a previous run's artifacts")
	var pf providerFlags
	pf.register(fs)
	if err := fs.Parse(args); err != nil {
		return err
	}

	var sum bench.Summary
	var cases []bench.CaseOutcome
	var sweep []bench.SweepPoint
	var env []bench.EnvelopePoint
	var sens []bench.SensitivityPoint
	for name, dst := range map[string]any{
		"summary": &sum, "cases": &cases, "sweep": &sweep,
		"envelope": &env, "sensitivity": &sens,
	} {
		b, err := os.ReadFile(filepath.Join(*dir, name+".json"))
		if err != nil {
			return fmt.Errorf("no %s.json in %s: %w\n  run `manhattan bench` first", name, *dir, err)
		}
		if err := json.Unmarshal(b, dst); err != nil {
			return fmt.Errorf("parse %s.json: %w", name, err)
		}
	}
	if len(sweep) == 0 {
		fmt.Fprintln(os.Stderr,
			"warning: this run carries no calibration sweep, so the documents cannot make\n"+
				"  their calibration claim. Re-run `manhattan bench` without -skip-sweep.")
	}

	store, err := evidence.Load(*dir)
	if err != nil {
		store = nil
	}
	provider, err := selectProvider(pf)
	if err != nil {
		return err
	}
	return renderNarrativeDocs(ctx, sum, cases, sweep, env, sens, store, provider)
}
