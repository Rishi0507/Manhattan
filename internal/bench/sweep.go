package bench

import (
	"math"
	"sort"
	"time"

	"github.com/Rishi0507/manhattan/internal/baseline"
	"github.com/Rishi0507/manhattan/internal/evidence"
	"github.com/Rishi0507/manhattan/internal/generate"
	"github.com/Rishi0507/manhattan/internal/model"
	"github.com/Rishi0507/manhattan/internal/money"
	"github.com/Rishi0507/manhattan/internal/narrow"
	"github.com/Rishi0507/manhattan/internal/pipeline"
	"github.com/Rishi0507/manhattan/internal/solver"
)

// SweepPoint is one configuration's predicted and observed behaviour.
//
// The real question about this system is not what its accuracy is. It is
// whether it knows in advance when it is about to be wrong. A gate that
// refuses the right settlements is worth far more than a matcher with a good
// hit rate, and it is a falsifiable claim: if the observed curves turn where
// the collision index predicts they turn, the estimator is calibrated and
// both the feasibility gate and the merchant segmentation are shippable.
type SweepPoint struct {
	Archetype string `json:"archetype"`
	PoolN     int    `json:"pool_n"`
	BatchSize int    `json:"batch_size"`
	Trials    int    `json:"trials"`

	// Predicted, before any search.
	MeanIndex         float64 `json:"mean_collision_index"`
	MeanAnalyticIndex float64 `json:"mean_analytic_collision_index"`
	MeanKStar         float64 `json:"mean_k_star"`
	MeanSigmaPaise    float64 `json:"mean_sigma_paise"`
	MeanTwinMass      float64 `json:"mean_twin_mass"`

	// Observed, after.
	VerifiedRate        float64 `json:"verified_rate"`
	AmbiguousRate       float64 `json:"ambiguous_rate"`
	UnderdeterminedRate float64 `json:"underdetermined_rate"`
	SensitiveRate       float64 `json:"narrowing_sensitive_rate"`
	UnresolvedRate      float64 `json:"unresolved_rate"`

	// The one that matters.
	WrongPostRate   float64 `json:"wrong_post_rate"`
	B0WrongPostRate float64 `json:"b0_wrong_post_rate"`
	B0PostRate      float64 `json:"b0_post_rate"`

	// MeanRivals is the exhaustively counted number of reconstructions, which
	// is what the collision index is trying to predict. Plotting the two
	// against each other is the calibration test.
	MeanRivals float64 `json:"mean_reconstructions_counted"`

	// EntropyGateRate is the fraction refused by twin mass alone, before the
	// collision index was ever consulted. It is reported separately because it
	// fires first and for a different reason.
	EntropyGateRate float64 `json:"entropy_gate_refusal_rate"`

	MeanLatencyMS float64 `json:"mean_latency_ms"`
}

// SweepSpec parameterises the calibration sweep.
type SweepSpec struct {
	Seed   int64
	Trials int
	// Pools and Batches are swept independently so the two axes can be read
	// apart: pool size drives C(n, k), batch size drives k.
	Pools      []int
	Batches    []int
	Archetypes []string
}

// DefaultSweep is the shipped calibration sweep.
func DefaultSweep() SweepSpec {
	return SweepSpec{
		Seed:   20260826,
		Trials: 8,
		// Chosen to straddle the boundary rather than to sit safely on one
		// side of it. A sweep that only measures configurations the system
		// handles well is a demonstration, not a measurement.
		Pools:      []int{16, 24, 34, 48, 70, 100, 150, 220},
		Batches:    []int{4, 6, 9},
		Archetypes: []string{"travel", "marketplace", "d2c_ecommerce", "quick_commerce"},
	}
}

// Sweep runs the calibration study.
func Sweep(spec SweepSpec) []SweepPoint {
	var out []SweepPoint

	for _, arch := range spec.Archetypes {
		for _, pool := range spec.Pools {
			for _, batch := range spec.Batches {
				if batch >= pool-1 {
					continue
				}
				out = append(out, sweepOne(spec, arch, pool, batch))
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].MeanIndex < out[j].MeanIndex })
	return out
}

func sweepOne(spec SweepSpec, arch string, pool, batch int) SweepPoint {
	p := SweepPoint{Archetype: arch, PoolN: pool, BatchSize: batch}

	var idx, analytic, kstar, sigma, twin, rivals, latency float64
	counts := map[evidence.Status]int{}
	wrong, b0wrong, b0posted, entropyGate, n := 0, 0, 0, 0, 0

	for t := 0; t < spec.Trials; t++ {
		gs := generate.DefaultSpec()
		gs.Archetype = arch
		gs.Seed = spec.Seed + int64(t)*7919 + int64(pool)*31 + int64(batch)
		gs.Settlements = 3
		gs.BatchSize = batch
		gs.BatchJitter = 0
		gs.PoolTarget = pool
		gs.PoolJitter = 0
		gs.Start = time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC)
		ds := generate.Generate(gs)

		cfg := pipeline.DefaultConfig()
		cfg.Seed = gs.Seed
		eng := pipeline.New(ds, cfg)

		credit := ds.Credits[len(ds.Credits)/2]
		truth := attributable(ds.GroundTruth[credit.Ref], eng)

		t0 := time.Now()
		rec := eng.Reconcile(credit)
		latency += float64(time.Since(t0).Microseconds()) / 1000

		n++
		counts[rec.Status]++
		idx += rec.Feasibility.IndexAtKStar
		analytic += rec.Feasibility.AnalyticAtKStar
		kstar += float64(rec.Feasibility.KStar)
		sigma += rec.Pool.SigmaPaise
		twin += rec.AmountEntropy.TwinMass
		if !rec.AmountEntropy.Pass {
			entropyGate++
		}
		if rec.Uniqueness != nil {
			rivals += float64(rec.Uniqueness.MatchesFound)
		}
		if rec.Status.Postable() && !sameSet(rec.Witness, truth) {
			wrong++
		}

		b0 := b0For(eng, cfg, credit)
		if b0.Posted {
			b0posted++
			if !b0.Correct(truth) {
				b0wrong++
			}
		}
	}

	if n == 0 {
		return p
	}
	f := float64(n)
	p.Trials = n
	p.MeanIndex = idx / f
	p.MeanAnalyticIndex = analytic / f
	p.MeanKStar = kstar / f
	p.MeanSigmaPaise = sigma / f
	p.MeanTwinMass = twin / f
	p.MeanRivals = rivals / f
	p.MeanLatencyMS = latency / f
	p.EntropyGateRate = float64(entropyGate) / f

	p.VerifiedRate = float64(counts[evidence.StatusVerified]) / f
	p.AmbiguousRate = float64(counts[evidence.StatusAmbiguous]) / f
	p.UnderdeterminedRate = float64(counts[evidence.StatusUnderdetermined]) / f
	p.SensitiveRate = float64(counts[evidence.StatusNarrowingSensitive]) / f
	p.UnresolvedRate = float64(counts[evidence.StatusUnresolved]) / f
	p.WrongPostRate = float64(wrong) / f
	p.B0WrongPostRate = float64(b0wrong) / f
	p.B0PostRate = float64(b0posted) / f
	return p
}

// EnvelopePoint compares the predicted resource cost against the measured one.
type EnvelopePoint struct {
	PoolN            int     `json:"pool_n"`
	KStar            int     `json:"k_star"`
	PredictedEntries int64   `json:"predicted_entries"`
	ObservedEntries  int64   `json:"observed_entries"`
	PredictedMB      float64 `json:"predicted_mb"`
	ObservedMB       float64 `json:"observed_mb"`
	SolveMS          float64 `json:"solve_and_prove_ms"`
	IncludesProof    bool    `json:"uniqueness_included"`
}

// Envelope measures the solver's resource envelope against the cost model.
//
// Publishing a modelled number under a measured heading is exactly the class
// of unverified claim this system exists to refuse, so both columns are
// produced and both are printed.
func Envelope() []EnvelopePoint {
	var out []EnvelopePoint
	for _, cfg := range []struct{ n, k int }{
		{52, 7}, {52, 6}, {100, 5}, {320, 3}, {500, 3}, {1000, 3},
	} {
		contribs := syntheticContribs(cfg.n, 20260826)
		target := contribs[0] + contribs[3] + contribs[7]

		predEntries, predBytes := solver.PredictEntries(cfg.n, cfg.k)

		t0 := time.Now()
		res := solver.Solve(contribs, target, cfg.k, 0, solver.ScopeGate)
		elapsed := float64(time.Since(t0).Microseconds()) / 1000

		out = append(out, EnvelopePoint{
			PoolN:            cfg.n,
			KStar:            cfg.k,
			PredictedEntries: predEntries,
			ObservedEntries:  res.EntriesLeft + res.EntriesRght,
			PredictedMB:      float64(predBytes) / (1 << 20),
			ObservedMB:       float64(res.MemoryBytes) / (1 << 20),
			SolveMS:          elapsed,
			IncludesProof:    true,
		})
	}
	return out
}

func syntheticContribs(n int, seed int64) []moneyPaise {
	gs := generate.DefaultSpec()
	gs.Archetype = "travel"
	gs.Seed = seed
	gs.Settlements = 1
	gs.BatchSize = 2
	gs.PoolTarget = n
	gs.PoolJitter = 0
	ds := generate.Generate(gs)
	eng := pipeline.New(ds, pipeline.DefaultConfig())

	out := make([]moneyPaise, 0, n)
	for _, r := range eng.Records {
		if r.Contribution == 0 {
			continue
		}
		out = append(out, r.Contribution)
		if len(out) == n {
			break
		}
	}
	// Pad if the generator produced fewer usable records than asked for, so
	// the envelope always measures the size it claims to.
	for len(out) < n {
		out = append(out, moneyPaise(1_000_00+len(out)*7919))
	}
	return out
}

// LogSpaced buckets the sweep by collision index for plotting.
func LogSpaced(points []SweepPoint, buckets int) []Bucket {
	if len(points) == 0 {
		return nil
	}
	lo, hi := math.Inf(1), math.Inf(-1)
	for _, p := range points {
		v := math.Log10(math.Max(p.MeanIndex, 1e-6))
		lo = math.Min(lo, v)
		hi = math.Max(hi, v)
	}
	if hi <= lo {
		hi = lo + 1
	}
	out := make([]Bucket, buckets)
	for i := range out {
		out[i].LoIndex = math.Pow(10, lo+(hi-lo)*float64(i)/float64(buckets))
		out[i].HiIndex = math.Pow(10, lo+(hi-lo)*float64(i+1)/float64(buckets))
	}
	for _, p := range points {
		v := math.Log10(math.Max(p.MeanIndex, 1e-6))
		b := int((v - lo) / (hi - lo) * float64(buckets))
		if b >= buckets {
			b = buckets - 1
		}
		if b < 0 {
			b = 0
		}
		out[b].N++
		out[b].Verified += p.VerifiedRate
		out[b].Ambiguous += p.AmbiguousRate
		out[b].Underdetermined += p.UnderdeterminedRate
		out[b].Wrong += p.WrongPostRate
		out[b].B0Wrong += p.B0WrongPostRate
	}
	for i := range out {
		if out[i].N == 0 {
			continue
		}
		f := float64(out[i].N)
		out[i].Verified /= f
		out[i].Ambiguous /= f
		out[i].Underdetermined /= f
		out[i].Wrong /= f
		out[i].B0Wrong /= f
	}
	return out
}

// Bucket is one band of the collision index, with the observed rates in it.
type Bucket struct {
	LoIndex         float64 `json:"lo_collision_index"`
	HiIndex         float64 `json:"hi_collision_index"`
	N               int     `json:"configurations"`
	Verified        float64 `json:"verified_rate"`
	Ambiguous       float64 `json:"ambiguous_rate"`
	Underdetermined float64 `json:"underdetermined_rate"`
	Wrong           float64 `json:"wrong_post_rate"`
	B0Wrong         float64 `json:"b0_wrong_post_rate"`
}

// moneyPaise aliases the money type so this file reads as arithmetic rather
// than as plumbing.
type moneyPaise = money.Paise

// b0For runs the baseline over the same narrowed pool the pipeline used.
func b0For(eng *pipeline.Engine, cfg pipeline.Config, credit model.BankCredit) baseline.Result {
	m := eng.Merchants[credit.MerchantID]
	nc := cfg.Narrow
	nc.CycleDays = m.SettlementCycleDays
	nc.EnforceInstrument = m.InstrumentSegregated
	pool := narrow.Apply(eng.Records, credit, m, nc).Pool
	return baseline.Match(pool, credit.Amount, credit.DeclaredTxnCount, baseline.DefaultConfig())
}
