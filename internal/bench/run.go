package bench

import (
	"context"
	"fmt"
	"math"
	"runtime"
	"sort"
	"time"

	"github.com/Rishi0507/manhattan/internal/agent"
	"github.com/Rishi0507/manhattan/internal/baseline"
	"github.com/Rishi0507/manhattan/internal/evidence"
	"github.com/Rishi0507/manhattan/internal/generate"
	"github.com/Rishi0507/manhattan/internal/guards"
	"github.com/Rishi0507/manhattan/internal/llm"
	"github.com/Rishi0507/manhattan/internal/model"
	"github.com/Rishi0507/manhattan/internal/money"
	"github.com/Rishi0507/manhattan/internal/narrow"
	"github.com/Rishi0507/manhattan/internal/pipeline"
)

// Summary is one batch run's measured result, for both systems.
//
// Every field here is emitted by a run and written to RESULTS.md by script.
// None of it is typed into a document, which is the only way a number in a
// submission can be trusted to still describe the code.
type Summary struct {
	RunID       string `json:"run_id"`
	Settlements int    `json:"settlements"`
	Seed        int64  `json:"seed"`

	StatusCounts map[evidence.Status]int `json:"status_counts"`
	FlagCounts   map[evidence.Flag]int   `json:"flag_counts"`

	AutoPosted      int `json:"auto_posted"`
	AutoPostedWrong int `json:"auto_posted_wrong"`
	Exceptions      int `json:"exceptions"`

	B0Posted      int `json:"b0_auto_posted"`
	B0PostedWrong int `json:"b0_auto_posted_wrong"`
	B0Unresolved  int `json:"b0_unresolved"`

	// B1 is the lookup: read the settlement report's stated mapping and post
	// it. It is the answer every payments person gives to this project, and it
	// is measured here rather than argued with.
	//
	// Where the report is right, B1 is right, instantly and for free, and
	// these numbers say so plainly. What it cannot do is notice when the
	// report is wrong, because its only check on the report is the report.
	B1Posted        int `json:"b1_auto_posted"`
	B1PostedWrong   int `json:"b1_auto_posted_wrong"`
	B1DefectiveRpts int `json:"report_defects_present"`
	// B1CaughtByUs counts defective reports where Manhattan's independent
	// reconstruction did NOT agree with the report, so a discrepancy would
	// have been raised. This is the number that answers the question.
	B1CaughtByUs int `json:"report_defects_manhattan_would_flag"`
	B1MissedByUs int `json:"report_defects_manhattan_would_miss"`

	// M1 is the composite, and it is the shape of the actual product.
	//
	// Post Manhattan's own reconstruction where it proved one. Otherwise, if
	// the gateway report names a batch, CHECK that batch against the money and
	// post it when it holds. Hold only where the report is contradicted or
	// absent.
	//
	// The reason this reaches so much further than reconstruction alone is
	// that checking a claimed batch is O(claim) while deriving one is
	// C(n, k). The claim check therefore works on the UNDERDETERMINED
	// population and on the archetypes that reconstruct nothing, which is most
	// of what reconstruction has to refuse.
	M1Posted      int `json:"m1_auto_posted"`
	M1PostedWrong int `json:"m1_auto_posted_wrong"`
	M1FromProof   int `json:"m1_posted_from_reconstruction"`
	M1FromClaim   int `json:"m1_posted_from_checked_claim"`
	M1Held        int `json:"m1_held"`
	// Defective reports split three ways, because "missed" would otherwise
	// cover two very different outcomes. Contradicted is the composite finding
	// the error and saying so. Held-uncheckable is the composite declining to
	// judge because part of the claim lives in a feed nobody joined, which is
	// safe but is not a detection. Posted is the only failure, and it is the
	// number to read.
	M1CaughtDefects   int `json:"m1_report_defects_contradicted"`
	M1HeldUncheckable int `json:"m1_report_defects_held_uncheckable"`
	M1PostedDefects   int `json:"m1_report_defects_posted_anyway"`
	// M1FalseAlarms counts CLEAN reports the claim check contradicted. This is
	// the number that decides whether the composite is shippable, because a
	// false alarm is a settlement a lookup would have posted correctly and the
	// composite held. It is measured rather than assumed.
	M1FalseAlarms  int `json:"m1_false_alarms_on_clean_reports"`
	M1CleanReports int `json:"m1_clean_reports_checked"`

	// NoClaim counts settlements the gateway report carries no mapping for.
	// There is nothing to check on these and reconstruction is the only route
	// to a posting, which is the measured answer to "what is the solver for
	// when most postings come from checked claims".
	NoClaim       int `json:"settlements_with_no_report_mapping"`
	NoClaimPosted int `json:"settlements_with_no_mapping_posted_by_reconstruction"`

	MedianLatencyMS float64 `json:"median_latency_ms"`
	P95LatencyMS    float64 `json:"p95_latency_ms"`
	B0MedianMS      float64 `json:"b0_median_latency_ms"`
	WallClockS      float64 `json:"wall_clock_s"`
	PerHour         float64 `json:"settlements_per_hour"`
	// PeakMemoryMB is a SAMPLED heap high-water mark and it varies between
	// runs of the same commit on the same seed, because where it lands depends
	// on when the garbage collector happened to run. Two runs an hour apart
	// have reported 15 MB and 119 MB for an identical batch.
	//
	// It is kept because it is the number an operator asks for, and it is
	// labelled everywhere it appears. PeakSolverMB beside it is the figure
	// that does not move: the largest enumeration any single settlement in
	// this batch actually allocated, computed from the entry counts on the
	// receipts at twelve bytes each. That is the quantity the resource
	// envelope is built on and the one a capacity estimate should use.
	PeakMemoryMB  float64 `json:"peak_memory_mb_sampled"`
	PeakSolverMB  float64 `json:"peak_solver_mb_deterministic"`
	PeakSolverRef string  `json:"peak_solver_settlement"`

	// Cost is reported as a decomposition rather than as a single typed
	// total, because a typed aggregate is exactly the kind of number that
	// drifts away from the receipts it is supposed to summarise.
	ParseCalls int `json:"parse_calls"`
	AgentCalls int `json:"agent_calls"`
	// AgentSteps is how many actions the agent actually took, and
	// AgentRepaired how many settlements it moved into a postable state.
	AgentSteps    int `json:"agent_steps"`
	AgentRepaired int `json:"agent_repaired"`
	// AgentInvoked and AgentSkipped split the exception queue by whether the
	// agent was consulted at all. The skipped count is the interesting one: it
	// is the exceptions a deterministic check settled without a model call.
	AgentInvoked int `json:"agent_invoked"`
	AgentSkipped int `json:"agent_skipped"`
	// AgentProvenCures counts settlements where the agent established, by
	// re-running the full stack, that a specific change would make the credit
	// reconstruct uniquely, without that change being corroborated enough to
	// post on.
	AgentProvenCures int     `json:"agent_proven_cures"`
	ModelCalls       int     `json:"model_calls"`
	InputTokens      int     `json:"input_tokens"`
	OutputTokens     int     `json:"output_tokens"`
	ExceptionRate    float64 `json:"exception_rate"`
	INRPer1k         float64 `json:"inr_per_1k_settlements"`

	// B0TokensPer1k is what a fuzzy matcher would spend, which scales with
	// pool size because the pool has to enter the context window.
	B0TokensPer1k  float64 `json:"b0_input_tokens_per_1k"`
	AxiomTokPer1k  float64 `json:"input_tokens_per_1k"`
	B0INRPer1k     float64 `json:"b0_inr_per_1k_settlements"`
	Provider       string  `json:"provider"`
	ProviderModels string  `json:"provider_models"`
	// Host names the machine a timing was measured on. A throughput figure
	// without one is not a measurement, it is a number.
	Host string `json:"measured_on"`

	// Conditions names the operational misconfigurations this run modelled,
	// so no reader has to discover them in the source. Both are things a
	// deployment gets wrong on its own side.
	Conditions []string `json:"modelled_operational_conditions"`
	// AgentRepairedByAction splits repairs by the action that produced them,
	// which is the only honest way to report what the agent contributed.
	AgentRepairedByAction map[string]int `json:"agent_repairs_by_action"`

	// CallsByRole is where the model is actually used, per job, so a reader
	// does not have to accept one aggregate figure for "the AI".
	CallsByRole map[string]int `json:"model_calls_by_role"`

	// DiagnosedDefects counts contradicted report claims the model classified,
	// and DefectClasses is the distribution of its diagnoses.
	DiagnosedDefects int            `json:"report_defects_diagnosed"`
	DefectClasses    map[string]int `json:"report_defect_classes"`
	// DiagnosisCorrect scores the model's diagnosis against the generator's
	// own record of what it injected. This is the one place in the repository
	// where a model's OUTPUT is graded rather than merely constrained, and it
	// is the number to quote about the model rather than about the verifier.
	DiagnosisCorrect  int     `json:"report_defects_diagnosed_correctly"`
	DiagnosisAccuracy float64 `json:"report_defect_diagnosis_accuracy"`
	// DiagnosisConfusion is graded[true][predicted], so a reader can see which
	// classes the model confuses rather than only how often it is right.
	DiagnosisConfusion map[string]map[string]int `json:"report_defect_confusion"`

	// Close is the controller's report on the whole period: the one model
	// output that reads across settlements rather than within one, and the
	// only one graded on whether it found the conditions this run injected.
	Close *evidence.PeriodClose `json:"period_close,omitempty"`

	// NotesDrafted counts analyst-facing notes the model wrote for held
	// settlements. NotesRejected counts drafts thrown away for containing a
	// figure, which the schema forbids because every number in a rendered note
	// is substituted from the receipt.
	NotesDrafted  int `json:"analyst_notes_drafted"`
	NotesRejected int `json:"analyst_notes_rejected"`

	Drift []guards.DriftFinding `json:"narrowing_drift,omitempty"`

	// PricedAt names the model whose published rates the cost figures use.
	// On an offline run there is no real spend, so the token counts are priced
	// at what the live path would have cost and labelled as such. Reporting a
	// zero would be true and useless; reporting a price without saying it is
	// modelled would be the failure this project argues against.
	PricedAt    string `json:"priced_at_model"`
	PriceIsReal bool   `json:"price_is_real_spend"`

	// B0Sweep is B0's wrong-posting rate as a function of its posting
	// threshold, with the shipped operating point and the best-F1 point marked.
	//
	// This exists because the headline result rests on a number produced by a
	// component in this repository, and "wrong on 59 per cent of its postings"
	// is not a figure anybody should accept on assertion. The sweep shows
	// there is no threshold at which the baseline is both useful and safe,
	// which is a stronger claim than the single operating point and is the
	// one actually being made.
	B0Sweep []B0ThresholdPoint `json:"b0_threshold_sweep"`

	// B0Features names what the baseline scores on, so its confidence is
	// auditable rather than a black box the comparison happens to favour.
	B0Features []string `json:"b0_features"`

	// Cost is the derivation behind the two rupee figures rather than the
	// figures alone. A cost claim with no arithmetic behind it is the exact
	// class of unverified assertion this project exists to refuse.
	Cost CostBasis `json:"cost_basis"`

	// Pools records the record counts the run actually operated over, which
	// is what the track's fifty-record floor is measured against.
	Pools PoolStats `json:"pool_stats"`

	// ExceptionCostINR totals the per-receipt exception cost across the whole
	// held queue, so the refusal has a price attached rather than only a
	// principle. ExceptionMinutes is the analyst time behind it, and
	// ExceptionValuePaise is the money sitting unposted while it is worked.
	ExceptionCostINR    int         `json:"exception_cost_inr_total"`
	ExceptionMinutes    int         `json:"exception_minutes_total"`
	ExceptionValuePaise money.Paise `json:"exception_value_at_stake_paise"`
	// ExceptionCostSpread is the cheapest and dearest row in the queue, which
	// is the number that says whether sorting it means anything.
	ExceptionCostMin int `json:"exception_cost_inr_min"`
	ExceptionCostMax int `json:"exception_cost_inr_max"`

	// TopExceptions is the actual backlog, sorted by cost, so the honest
	// exception list the track asks for is exhibited rather than argued for.
	TopExceptions []ExceptionRow `json:"top_exceptions"`

	// ByArchetype is the segmentation that turns a mediocre aggregate
	// auto-post rate into a useful statement.
	//
	// A single blended figure across six merchant types describes none of
	// them. What matters commercially is that the system can say, before any
	// integration, roughly what fraction of a given merchant's settlements it
	// expects to post, because it knows exactly what makes the method fail.
	ByArchetype []ArchetypeResult `json:"by_archetype"`
}

// B0ThresholdPoint is the baseline's behaviour at one posting threshold.
type B0ThresholdPoint struct {
	Threshold float64 `json:"threshold"`
	Posted    int     `json:"posted"`
	Right     int     `json:"right"`
	Wrong     int     `json:"wrong"`
	Precision float64 `json:"precision"`
	Recall    float64 `json:"recall"`
	F1        float64 `json:"f1"`
	// BestF1 marks the threshold a team tuning this baseline would choose,
	// and Shipped marks the one it ships with.
	BestF1  bool `json:"best_f1"`
	Shipped bool `json:"shipped_operating_point"`
}

// CostBasis is every input to the cost-per-thousand figures.
type CostBasis struct {
	Model            string  `json:"model"`
	InputUSDPerMTok  float64 `json:"input_usd_per_mtok"`
	OutputUSDPerMTok float64 `json:"output_usd_per_mtok"`
	CacheReadUSD     float64 `json:"cache_read_usd_per_mtok"`
	CacheWriteUSD    float64 `json:"cache_write_usd_per_mtok"`
	USDToINR         float64 `json:"usd_to_inr"`

	UncachedInput int `json:"uncached_input_tokens"`
	CachedInput   int `json:"cached_input_tokens"`
	CacheWrite    int `json:"cache_write_tokens"`
	Output        int `json:"output_tokens"`
	Calls         int `json:"calls"`
	// CacheHitRate is cache reads over all input tokens. On a replay run it
	// is zero, which prices every token at the uncached rate and therefore
	// overstates the live cost rather than flattering it.
	CacheHitRate float64 `json:"cache_hit_rate"`

	// The baseline's token model, stated so it can be argued with.
	B0Input          int     `json:"b0_input_tokens"`
	B0Output         int     `json:"b0_output_tokens"`
	B0PerRecord      int     `json:"b0_tokens_per_record"`
	B0Overhead       int     `json:"b0_tokens_overhead"`
	B0MeanPoolN      float64 `json:"b0_mean_narrowed_pool"`
	B0TokensPerCall  float64 `json:"b0_input_tokens_per_settlement"`
	ManhattanPerCall float64 `json:"manhattan_input_tokens_per_settlement"`
}

// PoolStats is the shape of the data the run operated over.
type PoolStats struct {
	TotalRecords    int     `json:"total_records_generated"`
	RawMin          int     `json:"universe_min"`
	RawMax          int     `json:"universe_max"`
	RawMean         float64 `json:"universe_mean"`
	NarrowedMin     int     `json:"narrowed_min"`
	NarrowedMax     int     `json:"narrowed_max"`
	NarrowedMean    float64 `json:"narrowed_mean"`
	LargestBatch    int     `json:"largest_declared_batch"`
	SettlementCount int     `json:"settlements"`
}

// ExceptionRow is one line of the held queue as an operator would see it.
type ExceptionRow struct {
	Ref       string `json:"settlement_ref"`
	Archetype string `json:"archetype"`
	Status    string `json:"status"`
	// ValuePaise is the credit that stays unposted while this sits in the
	// queue, and Minutes is what clearing it is estimated to take. The queue
	// is ordered by the first divided by the second, because that is what
	// "work the queue in the order that clears the most money per hour"
	// literally means, and ordering by handling cost alone would put a
	// forty-five minute investigation of a small credit above a twelve minute
	// data fix on a large one.
	ValuePaise  money.Paise `json:"value_at_stake_paise"`
	Minutes     int         `json:"handling_minutes"`
	INRPerHour  float64     `json:"value_cleared_per_analyst_hour_inr"`
	CostINR     int         `json:"exception_cost_inr"`
	PoolN       int         `json:"pool_n"`
	Cause       string      `json:"cause"`
	Remediation string      `json:"remediation"`
	AgentTouch  bool        `json:"agent_worked_it"`
	Index       float64     `json:"collision_index"`
}

// ArchetypeResult is one merchant shape's measured outcome.
type ArchetypeResult struct {
	Archetype       string  `json:"archetype"`
	ExpectedRegime  string  `json:"expected_regime"`
	Settlements     int     `json:"settlements"`
	AutoPostRate    float64 `json:"auto_post_rate"`
	AutoPostedWrong int     `json:"auto_posted_wrong"`
	MeanSigmaPaise  float64 `json:"mean_sigma_paise"`
	MeanTwinMass    float64 `json:"mean_twin_mass"`
	MeanIndex       float64 `json:"mean_collision_index"`
	EntropyRefused  int     `json:"entropy_gate_refusals"`
	B0PostRate      float64 `json:"b0_post_rate"`
	B0WrongRate     float64 `json:"b0_wrong_post_rate"`
	// M1PostRate is the composite's rate for this merchant type, and it is
	// the number that matters commercially. Reconstruction alone reports 0 per
	// cent on the two flat-price archetypes, which reads as "we cannot help
	// you"; the composite posts almost all of them, because checking a
	// claimed batch of two hundred identical charges is trivial while
	// deriving one is impossible.
	M1PostRate  float64 `json:"m1_post_rate"`
	M1PostedNum int     `json:"m1_posted"`
	M1Wrong     int     `json:"m1_posted_wrong"`
}

// BatchSpec parameterises a benchmark run.
type BatchSpec struct {
	Settlements int
	Seed        int64
	// ArchetypeMix runs a realistic spread of merchants rather than one, so
	// the headline is a distribution and not an operating point.
	ArchetypeMix []string
	RunID        string
	// Baseline compares against a stored per-constraint drop rate, so the
	// run-level drift monitor has something to deviate from.
	Baseline guards.DriftBaseline
	// WindowSlack overrides the modelled per-merchant window misconfiguration.
	// Nil uses the shipped map; an empty map means every merchant is
	// configured correctly, which is the control.
	WindowSlack map[string]float64
	// ReportDefectRate overrides the generator's rate of defective settlement
	// reports. Nil uses the generator default.
	ReportDefectRate *float64
	// NaiveFeeFallback prices a payment with no fee row at the configured
	// schedule instead of at the merchant's own observed rate.
	//
	// This exists to be measured against, and it is what the system did
	// before. It is the difference between a false-alarm rate that is a
	// tautology and one that is a result.
	NaiveFeeFallback bool
}

// windowSlackHours widens the reconciliation window for merchants whose
// configuration was set too loosely.
//
// This is a modelled OPERATIONAL CONDITION, not a defect invented to give the
// agent something to do, and the distinction is worth stating because the
// obvious criticism of any agent benchmark is that the author created the
// problem the agent solves.
//
// An over-wide value-date window is the single most common misconfiguration in
// a reconciliation deployment. Somebody sets it to plus or minus 36 hours
// during onboarding to stop a merchant's edge cases falling out of the window,
// nobody revisits it, and the pool that reaches the solver is three capture
// days wide instead of one. The system is then refusing settlements for a
// reason that has nothing to do with the merchant's data.
//
// It is the exact counterpart of the unjoined disputes feed already modelled:
// both are things a deployment gets wrong on this side rather than things the
// counterparty did. Neither is exotic and both are recoverable.
//
// The honest way to publish an agent's contribution against a condition like
// this is as a FUNCTION of how much of it there is, not as a single number
// from a configuration the author chose. `manhattan bench -window-slack N`
// sweeps it, and RESULTS.md carries the curve. At zero slack the agent repairs
// nothing through narrowing, and that is the loop working rather than failing.
var windowSlackHours = map[string]float64{
	"d2c_ecommerce": 24,
	"travel":        22,
	// Marketplace carries BOTH a wide window and an unjoined disputes feed,
	// and that overlap is deliberate.
	//
	// A benchmark where every merchant has exactly one problem is a benchmark
	// a lookup table can score full marks on, and the deterministic stub that
	// writes the close is close to being one: it applies a fixed rule per
	// merchant and reports the first that matches. Giving one merchant two
	// conditions is what makes the close's condition recall a measurement of
	// reasoning rather than of rule coverage, and the stub is expected to miss
	// the second. Real deployments are not tidy either.
	"marketplace": 26,
}

// negotiatedRate names merchants signed at a rate other than the published
// schedule the pipeline is configured with, and how often their settlement
// report omits a per-payment fee row.
//
// This exists because the composite's false-alarm rate was a tautology before
// it did, and that is worth stating rather than hiding in a changelog.
//
// The generator and the accounting engine derive contributions from the same
// policy. So a settlement report that was entirely correct could never
// disagree with the claim check: both sides computed the same number from the
// same schedule, and "zero false alarms on 469 clean reports" was not a
// measurement. It was arithmetic agreeing with itself.
//
// A negotiated rate is the most ordinary thing in Indian payments. A merchant
// is signed at 185 bps rather than the published 200. The gateway applies 185,
// the credit reflects 185, and wherever the report carries a per-payment fee
// row the pipeline uses it and agrees. Where the report has a GAP, and real
// reports have gaps, the pipeline falls back to the schedule it was configured
// with, computes 200, and disagrees with a report that was correct.
//
// That is a false alarm with a real cause, and it means the false-alarm rate
// is now a number this benchmark can be wrong about.
var negotiatedRate = map[string]struct {
	bps         int64
	missingRows float64
}{
	// A large marketplace with the negotiating power to get 15 bps off, and a
	// report that drops a fee row on roughly one payment in twelve.
	"marketplace": {bps: 185, missingRows: 0.08},
	// A travel merchant on a promotional rate, with a cleaner report.
	"travel": {bps: 178, missingRows: 0.04},
}

// unjoinedFeed names the merchants whose disputes feed was never joined.
var unjoinedFeed = map[string]bool{
	"marketplace":    true,
	"quick_commerce": true,
}

// DefaultBatch is the shipped 500-settlement benchmark.
func DefaultBatch() BatchSpec {
	return BatchSpec{
		Settlements: 500,
		Seed:        20260826,
		ArchetypeMix: []string{
			"travel", "marketplace", "d2c_ecommerce",
			"utility_billpay", "subscription_saas", "quick_commerce",
		},
		RunID: "run_" + time.Now().UTC().Format("20060102_1504"),
	}
}

// RunBatch reconciles a batch through both systems and measures everything.
func RunBatch(ctx context.Context, spec BatchSpec, provider llm.Provider) (*evidence.Store, Summary, error) {
	store := evidence.NewStore()
	sum := Summary{
		RunID:        spec.RunID,
		Seed:         spec.Seed,
		StatusCounts: map[evidence.Status]int{},
		FlagCounts:   map[evidence.Flag]int{},
		Provider:     provider.Name(),
		ProviderModels: fmt.Sprintf("parse=%s resolve=%s answer=%s",
			provider.Model(llm.RoleParse), provider.Model(llm.RoleResolve), provider.Model(llm.RoleAnswer)),
	}

	perArch := spec.Settlements / len(spec.ArchetypeMix)
	if perArch < 1 {
		perArch = 1
	}

	repairedBy := map[string]int{}
	var latencies, b0Latencies []float64
	var usage llm.Usage
	var b0Tokens int
	aggDrops := map[narrow.Constraint]int{}
	aggUniverse := 0

	// Every B0 decision as a (confidence, was-it-right) pair, so the threshold
	// sweep is computed from the same decisions the headline uses rather than
	// from a second run that might differ.
	type b0point struct {
		conf    float64
		correct bool
	}
	var b0points []b0point
	var b0PoolTotal int
	ps := PoolStats{RawMin: 1 << 30, NarrowedMin: 1 << 30}
	archOf := map[string]string{}

	var m0 runtime.MemStats
	runtime.ReadMemStats(&m0)
	peak := m0.HeapAlloc

	start := time.Now()

	for _, arch := range spec.ArchetypeMix {
		ar := ArchetypeResult{
			Archetype:      arch,
			ExpectedRegime: generate.ByName(arch).ExpectedRegime,
		}
		var archPosted, archWrong, archB0, archB0Wrong, archM1, archM1Wrong int
		var archSigma, archTwin, archIndex float64

		gs := generate.DefaultSpec()
		gs.Archetype = arch
		gs.Seed = spec.Seed + int64(len(arch))
		gs.Settlements = perArch
		// A realistic spread of difficulty. Pool jitter is what turns a single
		// operating point into the distribution the track brief asks for.
		gs.PoolTarget = 34
		// One settlement in six arrives with no mapping to check, which is the
		// population reconstruction exists for.
		gs.NoMappingRate = 0.17
		if spec.ReportDefectRate != nil {
			gs.ReportDefectRate = *spec.ReportDefectRate
		}
		if nr, ok := negotiatedRate[arch]; ok {
			gs.NegotiatedMDRBps = nr.bps
			gs.MissingFeeRowRate = nr.missingRows
		}
		gs.PoolJitter = 14

		// Two merchants run with their disputes feed never wired into the
		// candidate pool.
		//
		// This is not a contrivance to give the agent something to do. A feed
		// that exists but was never connected is one of the most common real
		// conditions in reconciliation, it affects a whole merchant rather than
		// one settlement, and it is invisible to the verifier: the arithmetic
		// simply does not close and nothing in the pool explains why. Naming
		// the class of event behind that gap is the job no solver can do.
		gs.JoinDisputes = !unjoinedFeed[arch]

		ds := generate.Generate(gs)

		cfg := pipeline.DefaultConfig()
		if spec.NaiveFeeFallback {
			cfg.Accounting.CalibrateMissingFees = false
		}
		slack := windowSlackHours
		if spec.WindowSlack != nil {
			slack = spec.WindowSlack
		}
		if h, ok := slack[arch]; ok && h > 0 {
			cfg.Narrow.Window = time.Duration(h * float64(time.Hour))
		}
		cfg.RunID = spec.RunID
		cfg.Seed = gs.Seed
		eng := pipeline.New(ds, cfg)
		ps.TotalRecords += len(eng.Records) + len(eng.Unjoined)

		parser := agent.NewParser(provider)
		controller := agent.NewController(provider)

		// Cross-settlement memory, scoped to one merchant's own proved
		// settlements. It accumulates as the run proceeds, so early
		// settlements have no history to draw on and late ones do, which is
		// the same position a real deployment is in on day one.
		profiles := agent.NewProfileStore()
		diagnostician := agent.NewDiagnostician(provider)
		drafter := agent.NewDrafter(provider)

		for _, credit := range ds.Credits {
			truth := attributable(ds.GroundTruth[credit.Ref], eng)

			t0 := time.Now()

			// Stage 1. Every settlement pays for exactly one small model call,
			// whatever the size of its candidate pool.
			if n, u, err := parser.Parse(ctx, credit); err == nil {
				usage.Add(u)
				sum.ParseCalls++
				if ok, why := n.Reconcilable(); !ok {
					_ = why
				}
			}

			rec := eng.Reconcile(credit)

			// Stage 7 runs on everything that did not post, not only on the
			// UNRESOLVED minority.
			//
			// That widening is the point of having a controller rather than a
			// hypothesis loop. A missing record explains an UNRESOLVED
			// settlement, which is a small population. Most refusals are
			// UNDERDETERMINED, and those are caused by a candidate pool that
			// is too wide, which is a narrowing decision the agent can act on
			// and a hypothesis cannot.
			if !rec.Status.Postable() {
				worked, st, u := controller.Work(ctx, eng, credit, rec, profiles.For(credit.MerchantID, eng.ByID, eng.Merchants[credit.MerchantID].SettlementCycleDays))
				usage.Add(u)
				sum.AgentCalls += u.Calls
				sum.AgentSteps += len(st)
				if worked.Agent.Invoked {
					sum.AgentInvoked++
				} else {
					sum.AgentSkipped++
				}
				cured := false
				for _, s := range st {
					if s.Accepted {
						sum.AgentRepaired++
						repairedBy[string(s.Action.Kind)]++
					}
					if s.Result == evidence.StatusVerified && !s.Accepted {
						cured = true
					}
				}
				if cured {
					sum.AgentProvenCures++
				}
				rec = worked
			}
			latencies = append(latencies, float64(time.Since(t0).Microseconds())/1000)

			rec.Cost = evidence.CostBlock{
				ModelCalls:   1 + rec.Agent.Iterations,
				InputTokens:  usage.InputTokens,
				OutputTokens: usage.OutputTokens,
			}
			// The analyst-facing note, for held settlements that have
			// something specific to act on. This is the highest-volume place
			// the model does work nothing else can, and the only model output
			// here whose failure mode is a confusing sentence rather than a
			// wrong ledger, because it is attached to a settlement that is
			// held either way.
			if !rec.Status.Postable() {
				note, u, err := drafter.Draft(ctx, rec)
				usage.Add(u)
				if err == nil && note != nil {
					rec.AnalystNote = note
					if note.Rejected != "" {
						sum.NotesRejected++
					} else {
						sum.NotesDrafted++
					}
				}
			}

			// Memory is written only from proved settlements, and only after
			// the decision is final, so nothing the profile later corroborates
			// can have been influenced by a guess.
			profiles.Observe(rec)

			if err := store.Put(rec); err != nil {
				return nil, sum, fmt.Errorf("%s: %w", credit.Ref, err)
			}

			sum.StatusCounts[rec.Status]++
			for _, f := range rec.Flags {
				sum.FlagCounts[f]++
			}
			if rec.Status.Postable() {
				sum.AutoPosted++
				archPosted++
				if !sameSet(rec.Witness, truth) {
					sum.AutoPostedWrong++
					archWrong++
				}
			} else {
				sum.Exceptions++
			}

			ar.Settlements++
			archSigma += rec.Pool.SigmaPaise
			archTwin += rec.AmountEntropy.TwinMass
			archIndex += rec.Feasibility.IndexAtKStar
			if !rec.AmountEntropy.Pass {
				ar.EntropyRefused++
			}

			for c, n := range rec.Narrowing.Dropped {
				aggDrops[c] += n
			}
			aggUniverse += rec.Narrowing.Before

			// B0 on identical inputs.
			m := eng.Merchants[credit.MerchantID]
			byID := make(map[string]model.Record, len(eng.Records))
			for _, r := range eng.Records {
				byID[r.ID] = r
			}
			for _, r := range eng.Unjoined {
				byID[r.ID] = r
			}
			nc := cfg.Narrow
			nc.CycleDays = m.SettlementCycleDays
			nc.EnforceInstrument = m.InstrumentSegregated
			pool := narrow.Apply(eng.Records, credit, m, nc).Pool

			if _, hasMapping := ds.ReportedMapping[credit.Ref]; !hasMapping {
				sum.NoClaim++
				if rec.Status == evidence.StatusVerified {
					sum.NoClaimPosted++
					sum.M1Posted++
					sum.M1FromProof++
					archM1++
					if !sameSet(rec.Witness, truth) {
						sum.M1PostedWrong++
						archM1Wrong++
					}
				} else {
					sum.M1Held++
				}
			}

			// B1, the lookup. It reads the report's mapping directly; the
			// pipeline above never saw it.
			if stated, ok := ds.ReportedMapping[credit.Ref]; ok {
				// The composite. CheckClaim is a separate entry point called
				// only now, after Reconcile has already reached its own
				// conclusion from the merchant's records alone.
				cc := eng.CheckClaim(credit, stated)
				rec.ReportClaim = cc

				// The check is arithmetic and has already run. What the model
				// contributes is the diagnosis: the same failed check has
				// several causes with completely different remedies, and
				// telling them apart is reading rather than counting.
				if cc != nil && cc.Verdict == evidence.ClaimContradicted {
					dg, u, err := diagnostician.Diagnose(ctx, rec)
					usage.Add(u)
					if err == nil && dg != nil {
						cc.Diagnosis = dg
						rec.Remediation = append(rec.Remediation, evidence.Remediation{
							Action: dg.Action,
							Effect: dg.Effect,
						})
						sum.DiagnosedDefects++
						if sum.DefectClasses == nil {
							sum.DefectClasses = map[string]int{}
						}
						sum.DefectClasses[dg.Class]++

						// Graded against the truth the generator recorded and
						// the pipeline never saw.
						if want := ds.ReportDefectClass[credit.Ref]; want != "" {
							if sum.DiagnosisConfusion == nil {
								sum.DiagnosisConfusion = map[string]map[string]int{}
							}
							if sum.DiagnosisConfusion[want] == nil {
								sum.DiagnosisConfusion[want] = map[string]int{}
							}
							sum.DiagnosisConfusion[want][dg.Class]++
							if dg.Class == want {
								sum.DiagnosisCorrect++
							}
						}
					}
				}
				defectiveRpt := ds.ReportDefects[credit.Ref] != ""

				switch {
				case rec.Status == evidence.StatusVerified:
					sum.M1Posted++
					sum.M1FromProof++
					archM1++
					if !sameSet(rec.Witness, truth) {
						sum.M1PostedWrong++
						archM1Wrong++
					}
				case cc != nil && cc.Verdict == evidence.ClaimConsistent:
					sum.M1Posted++
					sum.M1FromClaim++
					archM1++
					if !sameSet(stated, ds.GroundTruth[credit.Ref]) {
						sum.M1PostedWrong++
						archM1Wrong++
					}
				default:
					sum.M1Held++
				}
				if defectiveRpt {
					switch {
					case cc != nil && cc.Verdict == evidence.ClaimContradicted:
						sum.M1CaughtDefects++
					case cc != nil && cc.Verdict == evidence.ClaimUncheckable:
						sum.M1HeldUncheckable++
					default:
						sum.M1PostedDefects++
					}
				} else {
					sum.M1CleanReports++
					if cc != nil && cc.Verdict == evidence.ClaimContradicted {
						sum.M1FalseAlarms++
					}
				}
			}
			if stated, ok := ds.ReportedMapping[credit.Ref]; ok {
				b1 := baseline.TrustReport(credit, stated, byID)
				defective := ds.ReportDefects[credit.Ref] != ""
				if defective {
					sum.B1DefectiveRpts++
				}
				if b1.Posted_ {
					sum.B1Posted++
					// Scored against the FULL ground truth rather than the
					// attributable subset. Manhattan is scored on the
					// attributable subset because a zero-contribution record is
					// invisible to any amount-based method, but B1 is not doing
					// arithmetic: it is copying a list of record ids, and that
					// list legitimately contains records that moved no money.
					// Scoring it on the reduced set would mark it wrong for
					// being right, which is the mirror image of the mistake
					// this benchmark exists to avoid.
					if !b1.Correct(ds.GroundTruth[credit.Ref]) {
						sum.B1PostedWrong++
					}
				}
				// Would Manhattan have raised a discrepancy? It posts only on
				// a proof, so a defective report is caught whenever Manhattan
				// either refused or reconstructed something different. It is
				// missed only if Manhattan verified the same wrong batch, and
				// that cannot happen while wrong postings are zero.
				if defective {
					agrees := rec.Status == evidence.StatusVerified && sameSet(rec.Witness, stated)
					if agrees {
						sum.B1MissedByUs++
					} else {
						sum.B1CaughtByUs++
					}
				}
			}

			b0start := time.Now()
			b0 := baseline.Match(pool, credit.Amount, credit.DeclaredTxnCount, baseline.DefaultConfig())
			b0Latencies = append(b0Latencies, float64(time.Since(b0start).Microseconds())/1000)
			b0Tokens += b0.TokensIn
			b0PoolTotal += len(pool)
			b0points = append(b0points, b0point{conf: b0.Confidence, correct: b0.Correct(truth)})
			archOf[rec.SettlementRef] = arch

			ps.SettlementCount++
			ps.RawMin = minInt(ps.RawMin, rec.Narrowing.Before)
			ps.RawMax = maxInt(ps.RawMax, rec.Narrowing.Before)
			ps.RawMean += float64(rec.Narrowing.Before)
			ps.NarrowedMin = minInt(ps.NarrowedMin, len(pool))
			ps.NarrowedMax = maxInt(ps.NarrowedMax, len(pool))
			ps.NarrowedMean += float64(len(pool))
			if credit.DeclaredTxnCount != nil && *credit.DeclaredTxnCount > ps.LargestBatch {
				ps.LargestBatch = *credit.DeclaredTxnCount
			}

			if b0.Posted {
				sum.B0Posted++
				archB0++
				if !b0.Correct(truth) {
					sum.B0PostedWrong++
					archB0Wrong++
				}
			} else {
				sum.B0Unresolved++
			}

			var ms runtime.MemStats
			runtime.ReadMemStats(&ms)
			if ms.HeapAlloc > peak {
				peak = ms.HeapAlloc
			}
		}

		if ar.Settlements > 0 {
			f := float64(ar.Settlements)
			ar.AutoPostRate = float64(archPosted) / f
			ar.AutoPostedWrong = archWrong
			ar.MeanSigmaPaise = archSigma / f
			ar.MeanTwinMass = archTwin / f
			ar.MeanIndex = archIndex / f
			ar.B0PostRate = float64(archB0) / f
			ar.B0WrongRate = float64(archB0Wrong) / f
			ar.M1PostRate = float64(archM1) / f
			ar.M1PostedNum = archM1
			ar.M1Wrong = archM1Wrong
		}
		sum.ByArchetype = append(sum.ByArchetype, ar)
	}

	sum.Settlements = len(store.All())
	sum.WallClockS = time.Since(start).Seconds()
	if sum.WallClockS > 0 {
		sum.PerHour = float64(sum.Settlements) / sum.WallClockS * 3600
	}
	sum.MedianLatencyMS = percentile(latencies, 0.50)
	sum.P95LatencyMS = percentile(latencies, 0.95)
	sum.B0MedianMS = percentile(b0Latencies, 0.50)
	sum.PeakMemoryMB = float64(peak) / (1 << 20)
	for _, r := range store.All() {
		if r.Solver == nil {
			continue
		}
		mb := float64(r.Solver.EntriesLeft+r.Solver.EntriesRight) * 12 / (1 << 20)
		if mb > sum.PeakSolverMB {
			sum.PeakSolverMB, sum.PeakSolverRef = mb, r.SettlementRef
		}
	}

	sum.AgentRepairedByAction = repairedBy
	for arch, h := range func() map[string]float64 {
		if spec.WindowSlack != nil {
			return spec.WindowSlack
		}
		return windowSlackHours
	}() {
		if h > 0 {
			sum.Conditions = append(sum.Conditions, fmt.Sprintf(
				"%s: reconciliation window misconfigured to plus or minus %.0f hours", arch, h))
		}
	}
	for arch := range unjoinedFeed {
		sum.Conditions = append(sum.Conditions, arch+": disputes feed never joined into the pool")
	}
	sort.Strings(sum.Conditions)

	sum.Host = fmt.Sprintf("%s/%s, %d logical cores, go%s",
		runtime.GOOS, runtime.GOARCH, runtime.NumCPU(), runtime.Version()[2:])

	if sum.DiagnosedDefects > 0 {
		sum.DiagnosisAccuracy = float64(sum.DiagnosisCorrect) / float64(sum.DiagnosedDefects)
	}

	sum.ModelCalls = usage.Calls
	if len(usage.ByRole) > 0 {
		sum.CallsByRole = map[string]int{}
		for r, n := range usage.ByRole {
			sum.CallsByRole[string(r)] = n
		}
	}
	sum.InputTokens = usage.InputTokens
	sum.OutputTokens = usage.OutputTokens
	if sum.Settlements > 0 {
		sum.ExceptionRate = float64(sum.Exceptions) / float64(sum.Settlements)
		per1k := 1000.0 / float64(sum.Settlements)
		sum.AxiomTokPer1k = float64(usage.InputTokens) * per1k
		sum.B0TokensPer1k = float64(b0Tokens) * per1k

		// Both systems are priced at the same published rates, so the
		// comparison isolates the architecture rather than the vendor. On an
		// offline run there is no real spend, so the token counts are priced
		// at what the live path would have cost and the receipt says so.
		priceModel := provider.Model(llm.RoleParse)
		sum.PriceIsReal = true
		if _, ok := llm.Rates[priceModel]; !ok || priceModel == "replay" {
			priceModel = "claude-opus-5"
			sum.PriceIsReal = false
		}
		sum.PricedAt = priceModel

		axiomUsage := llm.Usage{InputTokens: usage.InputTokens, OutputTokens: usage.OutputTokens}
		sum.INRPer1k = float64(llm.CostMicrosINR(priceModel, axiomUsage)) / 1e6 * per1k

		// B0's output is a proposed subset plus a score, which is short; its
		// input is the whole candidate pool, which is not.
		b0Usage := llm.Usage{InputTokens: b0Tokens, OutputTokens: 120 * sum.Settlements}
		sum.B0INRPer1k = float64(llm.CostMicrosINR(priceModel, b0Usage)) / 1e6 * per1k

		rate := llm.Rates[priceModel]
		sum.Cost = CostBasis{
			Model:            priceModel,
			InputUSDPerMTok:  rate.InputPerMTok,
			OutputUSDPerMTok: rate.OutputPerMTok,
			CacheReadUSD:     rate.CacheReadPerMTok,
			CacheWriteUSD:    rate.CacheWritePerMTok,
			USDToINR:         llm.USDToINR,
			UncachedInput:    usage.InputTokens,
			CachedInput:      usage.CacheReadTokens,
			CacheWrite:       usage.CacheWriteTokens,
			Output:           usage.OutputTokens,
			Calls:            usage.Calls,
			B0Input:          b0Tokens,
			B0Output:         120 * sum.Settlements,
			B0PerRecord:      baseline.TokensPerRecord,
			B0Overhead:       baseline.TokensOverhead,
			B0MeanPoolN:      float64(b0PoolTotal) / float64(sum.Settlements),
			B0TokensPerCall:  float64(b0Tokens) / float64(sum.Settlements),
			ManhattanPerCall: float64(usage.InputTokens) / float64(sum.Settlements),
		}
		if tot := usage.InputTokens + usage.CacheReadTokens; tot > 0 {
			sum.Cost.CacheHitRate = float64(usage.CacheReadTokens) / float64(tot)
		}

		ps.RawMean /= float64(sum.Settlements)
		ps.NarrowedMean /= float64(sum.Settlements)
		sum.Pools = ps

		// B0 at every threshold its scoring function can produce, plus the
		// round values in between, so the curve has no gaps a reader could
		// suspect were chosen.
		var thresholds []float64
		for t := 0.10; t <= 1.001; t += 0.05 {
			thresholds = append(thresholds, math.Round(t*100)/100)
		}
		bestF1, bestAt := -1.0, -1
		for _, t := range thresholds {
			pt := B0ThresholdPoint{Threshold: t}
			for _, p := range b0points {
				if p.conf >= t {
					pt.Posted++
					if p.correct {
						pt.Right++
					} else {
						pt.Wrong++
					}
				}
			}
			if pt.Posted > 0 {
				pt.Precision = float64(pt.Right) / float64(pt.Posted)
			}
			if sum.Settlements > 0 {
				pt.Recall = float64(pt.Right) / float64(sum.Settlements)
			}
			if pt.Precision+pt.Recall > 0 {
				pt.F1 = 2 * pt.Precision * pt.Recall / (pt.Precision + pt.Recall)
			}
			if math.Abs(t-baseline.DefaultConfig().Threshold) < 1e-9 {
				pt.Shipped = true
			}
			if pt.F1 > bestF1 {
				bestF1, bestAt = pt.F1, len(sum.B0Sweep)
			}
			sum.B0Sweep = append(sum.B0Sweep, pt)
		}
		if bestAt >= 0 {
			sum.B0Sweep[bestAt].BestF1 = true
		}
		sum.B0Features = baseline.Features()

		// The held queue, priced and sorted, which is the deliverable the
		// track actually asks for.
		sum.ExceptionCostMin = 1 << 30
		for _, r := range store.All() {
			if r.Status == evidence.StatusVerified {
				continue
			}
			sum.ExceptionCostINR += r.ExceptionCostINR
			sum.ExceptionMinutes += r.ExceptionMinutes
			sum.ExceptionValuePaise += r.TargetPaise.Abs()
			sum.ExceptionCostMin = minInt(sum.ExceptionCostMin, r.ExceptionCostINR)
			sum.ExceptionCostMax = maxInt(sum.ExceptionCostMax, r.ExceptionCostINR)
		}
		if sum.ExceptionCostMin == 1<<30 {
			sum.ExceptionCostMin = 0
		}
		sum.TopExceptions = topExceptions(store, archOf, 15)
	}

	// Run-level drift, which gates the batch rather than any one receipt.
	rates := map[narrow.Constraint]float64{}
	if aggUniverse > 0 {
		for c, n := range aggDrops {
			rates[c] = float64(n) / float64(aggUniverse)
		}
	}
	sum.Drift = guards.DetectDrift(rates, spec.Baseline, 0.10)

	// The period close. One call, after everything else has been decided,
	// reading aggregates rather than receipts. It is the only model output
	// that works above a single settlement, and it is graded against the
	// operational conditions this run injected, which the model never sees.
	if pc, u := runClose(ctx, agent.NewCloser(provider), store, sum, archOf); pc != nil {
		usage.Add(u)
		sum.Close = pc
		sum.ModelCalls = usage.Calls
		if len(usage.ByRole) > 0 {
			sum.CallsByRole = map[string]int{}
			for r, n := range usage.ByRole {
				sum.CallsByRole[string(r)] = n
			}
		}
		sum.InputTokens = usage.InputTokens
		sum.OutputTokens = usage.OutputTokens
	}

	run := &evidence.Run{
		RunID:           spec.RunID,
		Settlements:     sum.Settlements,
		StatusCounts:    sum.StatusCounts,
		FlagCounts:      sum.FlagCounts,
		AutoPosted:      sum.AutoPosted,
		AutoPostedWrong: sum.AutoPostedWrong,
		Exceptions:      sum.Exceptions,
		Drift:           sum.Drift,
		DropRates:       rates,
		Seed:            spec.Seed,
		Throughput: evidence.Throughput{
			WallClockSeconds:   sum.WallClockS,
			SettlementsPerHour: sum.PerHour,
			MedianLatencyMS:    sum.MedianLatencyMS,
			P95LatencyMS:       sum.P95LatencyMS,
			PeakMemoryMB:       sum.PeakMemoryMB,
		},
		Cost: evidence.RunCost{
			ModelCalls: sum.ModelCalls, ParseCalls: sum.ParseCalls, AgentCalls: sum.AgentCalls,
			ExceptionRate: sum.ExceptionRate, InputTokens: sum.InputTokens,
			OutputTokens: sum.OutputTokens, INRPer1k: sum.INRPer1k,
		},
		PolicyVersion: pipeline.DefaultConfig().Accounting.Policy.Version,
	}
	if len(sum.Drift) > 0 {
		run.Flags = append(run.Flags, evidence.FlagNarrowingDrift)
	}
	store.SetRun(run)

	return store, sum, nil
}

func percentile(xs []float64, p float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	s := append([]float64{}, xs...)
	sort.Float64s(s)
	i := int(float64(len(s)-1) * p)
	return s[i]
}

// AttributableTruth is the exported form of the ground-truth filter, so the
// command line can report correctness the same way the benchmark does.
func AttributableTruth(truth []string, eng *pipeline.Engine) []string {
	return attributable(truth, eng)
}

var _ = model.KindPayment

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// topExceptions renders the held queue the way an operations lead would work
// it: most expensive first, each row carrying the cause and the computed
// remediation rather than a status code.
//
// The track asks for an honest exception list, and a count of exceptions is
// not one. This is the list.
func topExceptions(store *evidence.Store, archOf map[string]string, n int) []ExceptionRow {
	var rows []ExceptionRow
	for _, r := range store.All() {
		if r.Status == evidence.StatusVerified {
			continue
		}
		row := ExceptionRow{
			Ref:        r.SettlementRef,
			Archetype:  archOf[r.SettlementRef],
			Status:     string(r.Status),
			ValuePaise: r.TargetPaise.Abs(),
			Minutes:    r.ExceptionMinutes,
			CostINR:    r.ExceptionCostINR,
			PoolN:      r.Pool.N,
			Cause:      r.Claim,
			AgentTouch: r.Agent.Invoked,
			Index:      r.Feasibility.IndexAtKStar,
		}
		if row.Minutes > 0 {
			row.INRPerHour = float64(row.ValuePaise) / 100 * 60 / float64(row.Minutes)
		}
		if len(r.Remediation) > 0 {
			row.Remediation = r.Remediation[0].Action
			if r.Remediation[0].ProjectedIndex != nil {
				row.Remediation += fmt.Sprintf(" (projected index %.3g)", *r.Remediation[0].ProjectedIndex)
			}
		}
		rows = append(rows, row)
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].INRPerHour != rows[j].INRPerHour {
			return rows[i].INRPerHour > rows[j].INRPerHour
		}
		return rows[i].Ref < rows[j].Ref
	})
	if len(rows) > n {
		rows = rows[:n]
	}
	return rows
}
