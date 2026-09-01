package bench

import (
	"context"
	"fmt"
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

	MedianLatencyMS float64 `json:"median_latency_ms"`
	P95LatencyMS    float64 `json:"p95_latency_ms"`
	B0MedianMS      float64 `json:"b0_median_latency_ms"`
	WallClockS      float64 `json:"wall_clock_s"`
	PerHour         float64 `json:"settlements_per_hour"`
	PeakMemoryMB    float64 `json:"peak_memory_mb"`

	// Cost is reported as a decomposition rather than as a single typed
	// total, because a typed aggregate is exactly the kind of number that
	// drifts away from the receipts it is supposed to summarise.
	ParseCalls    int     `json:"parse_calls"`
	AgentCalls    int     `json:"agent_calls"`
	ModelCalls    int     `json:"model_calls"`
	InputTokens   int     `json:"input_tokens"`
	OutputTokens  int     `json:"output_tokens"`
	ExceptionRate float64 `json:"exception_rate"`
	INRPer1k      float64 `json:"inr_per_1k_settlements"`

	// B0TokensPer1k is what a fuzzy matcher would spend, which scales with
	// pool size because the pool has to enter the context window.
	B0TokensPer1k  float64 `json:"b0_input_tokens_per_1k"`
	AxiomTokPer1k  float64 `json:"input_tokens_per_1k"`
	B0INRPer1k     float64 `json:"b0_inr_per_1k_settlements"`
	Provider       string  `json:"provider"`
	ProviderModels string  `json:"provider_models"`

	Drift []guards.DriftFinding `json:"narrowing_drift,omitempty"`

	// PricedAt names the model whose published rates the cost figures use.
	// On an offline run there is no real spend, so the token counts are priced
	// at what the live path would have cost and labelled as such. Reporting a
	// zero would be true and useless; reporting a price without saying it is
	// modelled would be the failure this project argues against.
	PricedAt    string `json:"priced_at_model"`
	PriceIsReal bool   `json:"price_is_real_spend"`

	// ByArchetype is the segmentation that turns a mediocre aggregate
	// auto-post rate into a useful statement.
	//
	// A single blended figure across six merchant types describes none of
	// them. What matters commercially is that the system can say, before any
	// integration, roughly what fraction of a given merchant's settlements it
	// expects to post, because it knows exactly what makes the method fail.
	ByArchetype []ArchetypeResult `json:"by_archetype"`
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

	var latencies, b0Latencies []float64
	var usage llm.Usage
	var b0Tokens int
	aggDrops := map[narrow.Constraint]int{}
	aggUniverse := 0

	var m0 runtime.MemStats
	runtime.ReadMemStats(&m0)
	peak := m0.HeapAlloc

	start := time.Now()

	for _, arch := range spec.ArchetypeMix {
		ar := ArchetypeResult{
			Archetype:      arch,
			ExpectedRegime: generate.ByName(arch).ExpectedRegime,
		}
		var archPosted, archWrong, archB0, archB0Wrong int
		var archSigma, archTwin, archIndex float64

		gs := generate.DefaultSpec()
		gs.Archetype = arch
		gs.Seed = spec.Seed + int64(len(arch))
		gs.Settlements = perArch
		// A realistic spread of difficulty. Pool jitter is what turns a single
		// operating point into the distribution the track brief asks for.
		gs.PoolTarget = 34
		gs.PoolJitter = 14
		ds := generate.Generate(gs)

		cfg := pipeline.DefaultConfig()
		cfg.RunID = spec.RunID
		cfg.Seed = gs.Seed
		eng := pipeline.New(ds, cfg)

		parser := agent.NewParser(provider)
		resolver := agent.NewResolver(provider)

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

			// Stage 7 runs only on exceptions, which is what keeps the
			// aggregate model spend close to the parse cost.
			if rec.Status == evidence.StatusUnresolved {
				resolved, u := resolver.Resolve(ctx, eng, credit, rec)
				usage.Add(u)
				if u.Calls > 0 {
					sum.AgentCalls += u.Calls
				}
				rec = resolved
			}
			latencies = append(latencies, float64(time.Since(t0).Microseconds())/1000)

			rec.Cost = evidence.CostBlock{
				ModelCalls:   1 + rec.Agent.Iterations,
				InputTokens:  usage.InputTokens,
				OutputTokens: usage.OutputTokens,
			}
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
			nc := cfg.Narrow
			nc.CycleDays = m.SettlementCycleDays
			nc.EnforceInstrument = m.InstrumentSegregated
			pool := narrow.Apply(eng.Records, credit, m, nc).Pool

			b0start := time.Now()
			b0 := baseline.Match(pool, credit.Amount, credit.DeclaredTxnCount, baseline.DefaultConfig())
			b0Latencies = append(b0Latencies, float64(time.Since(b0start).Microseconds())/1000)
			b0Tokens += b0.TokensIn
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

	sum.ModelCalls = usage.Calls
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
	}

	// Run-level drift, which gates the batch rather than any one receipt.
	rates := map[narrow.Constraint]float64{}
	if aggUniverse > 0 {
		for c, n := range aggDrops {
			rates[c] = float64(n) / float64(aggUniverse)
		}
	}
	sum.Drift = guards.DetectDrift(rates, spec.Baseline, 0.10)

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
