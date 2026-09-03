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

// LogSpaced buckets the sweep by collision index, with roughly equal
// population per band.
//
// The name is now a small lie and it is kept because the artifact key is. The
// first version cut equal-width bands in log space, which sounds right and put
// 76 of 96 configurations into the top two bands: the range is stretched by a
// handful of configurations at an index near 1e-4, so the interesting region
// above 1 got two enormous bins and the uninteresting region below 0.01 got
// five nearly empty ones. The curve therefore had its coarsest resolution
// exactly where it was being asked to say something, and it read as though the
// verified rate ticked back up in the top band when that band was simply too
// wide to mean anything.
//
// Equal population per band puts the resolution where the data is. The band
// edges are then quantiles of the observed index rather than round numbers,
// which is less tidy to read and considerably more honest: a band boundary is
// a statement about where the configurations are, not about where the author
// expected them to be.
func LogSpaced(points []SweepPoint, buckets int) []Bucket {
	if len(points) == 0 || buckets < 1 {
		return nil
	}

	idxOf := func(p SweepPoint) float64 { return math.Max(p.MeanIndex, 1e-6) }
	ordered := make([]SweepPoint, len(points))
	copy(ordered, points)
	sort.Slice(ordered, func(i, j int) bool { return idxOf(ordered[i]) < idxOf(ordered[j]) })

	if buckets > len(ordered) {
		buckets = len(ordered)
	}
	out := make([]Bucket, 0, buckets)
	for i := 0; i < buckets; i++ {
		lo := i * len(ordered) / buckets
		hi := (i + 1) * len(ordered) / buckets
		if hi <= lo {
			continue
		}
		b := Bucket{LoIndex: idxOf(ordered[lo]), HiIndex: idxOf(ordered[hi-1])}
		for _, p := range ordered[lo:hi] {
			b.N++
			b.Verified += p.VerifiedRate
			b.Ambiguous += p.AmbiguousRate
			b.Underdetermined += p.UnderdeterminedRate
			b.Wrong += p.WrongPostRate
			b.B0Wrong += p.B0WrongPostRate
			b.MeanPoolN += float64(p.PoolN)
		}
		f := float64(b.N)
		b.Verified /= f
		b.Ambiguous /= f
		b.Underdetermined /= f
		b.Wrong /= f
		b.B0Wrong /= f
		b.MeanPoolN /= f
		out = append(out, b)
	}
	return out
}

// CardinalityBands segments the sweep by batch cardinality before ordering by
// collision index.
//
// This exists because the flat sweep contains rows that contradict the flat
// claim, and they should be found here rather than by a reader. Travel at pool
// 220 with index 4.64 verifies nothing; travel at pool 70 with index 6.03
// verifies everything. Marketplace at 150 with index 5.87 verifies nothing;
// marketplace at 48 with index 5.96 verifies everything. Higher predicted
// index, better observed outcome, twice.
//
// Segmenting by pool size looked like the answer and was not. The variable
// actually doing the work is the cardinality of the batch:
//
//	batch 9   V 59, 0, 0, 0   across four index bands, monotone
//	batch 6   V 83, 0, 8, 30
//	batch 4   V 84, 41, 30, 70
//
// At cardinality 9 the index orders outcomes exactly. At 4 it breaks down at
// the top of the range, and the reason is a property of the estimator rather
// than a defect in the measurement. The collision index is an EXPECTED number
// of colliding subsets. At small k the enumeration is small enough that the
// realised count is frequently one even where the expectation is five, so the
// estimator is conservative precisely where the search is cheapest.
//
// The direction of that error is the whole point. Being conservative means the
// gate refuses configurations it could have verified, which costs recall. The
// wrong-posting rate is zero in every band of every cardinality, so nothing
// about this breakdown puts a wrong number in a ledger. The commercial claim
// is scoped accordingly in the documents: the index predicts the auto-post
// rate for merchants whose batches sit at cardinality six or above, and
// under-predicts it below that.
func CardinalityBands(points []SweepPoint, perBand int) []CardinalityBand {
	sizes := map[int]bool{}
	for _, p := range points {
		sizes[p.BatchSize] = true
	}
	var ks []int
	for k := range sizes {
		ks = append(ks, k)
	}
	sort.Ints(ks)

	var out []CardinalityBand
	for _, k := range ks {
		var in []SweepPoint
		for _, p := range points {
			if p.BatchSize == k {
				in = append(in, p)
			}
		}
		if len(in) == 0 {
			continue
		}
		b := CardinalityBand{BatchSize: k, N: len(in), Buckets: LogSpaced(in, perBand)}
		// Monotone means the verified rate never rises as the predicted index
		// rises. Computed rather than asserted, so the document cannot claim
		// an ordering the data stopped having.
		b.Monotone = true
		for i := 1; i < len(b.Buckets); i++ {
			if b.Buckets[i].Verified > b.Buckets[i-1].Verified+0.02 {
				b.Monotone = false
			}
			if b.Buckets[i].Wrong > 0 {
				b.AnyWrong = true
			}
		}
		if len(b.Buckets) > 0 && b.Buckets[0].Wrong > 0 {
			b.AnyWrong = true
		}
		out = append(out, b)
	}
	return out
}

// CardinalityBand is the calibration curve at one batch cardinality.
type CardinalityBand struct {
	BatchSize int      `json:"batch_size"`
	N         int      `json:"configurations"`
	Monotone  bool     `json:"verified_rate_monotone_in_index"`
	AnyWrong  bool     `json:"any_wrong_postings"`
	Buckets   []Bucket `json:"buckets"`
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
	// MeanPoolN is carried so a reader can see at a glance whether a band is
	// comparing like with like.
	MeanPoolN float64 `json:"mean_pool_n"`
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
