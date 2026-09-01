package feasibility

import (
	"math"
	"math/rand"
	"testing"

	"github.com/Rishi0507/manhattan/internal/money"
)

// TestEmpiricalIndexTracksTheTruth is the test that earns the empirical
// estimator its place in the decision path.
//
// The claim being made is that sampling the pool estimates the number of
// colliding subsets better than a moment-matched normal does. That is a
// falsifiable claim about numbers, so it is checked against an exact count:
// enumerate every 3-subset of a small pool by brute force, count how many hit
// each of a spread of targets, and compare both estimators against that.
//
// The tolerance is deliberately loose. Neither estimator is trying to be
// exact; the gate only needs to know which side of a threshold it is on. What
// the test enforces is that the empirical estimate is not systematically off
// by an order of magnitude on a heavy-tailed pool, which is precisely the
// failure the analytic form has.
func TestEmpiricalIndexTracksTheTruth(t *testing.T) {
	rng := rand.New(rand.NewSource(20260826))

	// A lognormal pool, which is what real ticket amounts look like and what
	// the normal approximation handles worst.
	const n = 34
	contribs := make([]money.Paise, n)
	for i := range contribs {
		r := math.Exp(rng.NormFloat64()*1.1 + math.Log(30000))
		contribs[i] = money.Paise(math.Round(r*100)) + money.Paise(rng.Intn(100))
	}
	total := money.Sum(contribs)
	sigma := Sigma(contribs)

	// Exact distribution of 3-subset sums.
	exact := map[int64]int{}
	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			for k := j + 1; k < n; k++ {
				exact[int64(contribs[i]+contribs[j]+contribs[k])]++
			}
		}
	}

	est := sampleSums(contribs, 4, 7)
	analytic := Index(n, 3, sigma, 1)

	// Count exact hits in a window around each probe point, and compare with
	// what each estimator predicts for that window.
	var empRatio, anaRatio []float64
	for trial := 0; trial < 40; trial++ {
		i, j, k := rng.Intn(n), rng.Intn(n), rng.Intn(n)
		if i == j || j == k || i == k {
			continue
		}
		target := int64(contribs[i] + contribs[j] + contribs[k])

		// A window wide enough to hold a meaningful count, since exact hits on
		// a single paise value are rare by construction.
		const window = 400_000
		hits := 0
		for v, c := range exact {
			if v >= target-window && v <= target+window {
				hits += c
			}
		}
		if hits < 5 {
			continue
		}
		truthPerPaise := float64(hits) / (2 * window)

		empDensity := est.densityAt(3, target, analytic/float64(len(exact)))
		empPredicted := empDensity * float64(binom3(n))
		anaPredicted := analytic

		empRatio = append(empRatio, empPredicted/(truthPerPaise*1))
		anaRatio = append(anaRatio, anaPredicted/(truthPerPaise*1))
	}

	if len(empRatio) < 10 {
		t.Skip("not enough usable probe points")
	}

	empErr := geomMeanLogAbs(empRatio)
	anaErr := geomMeanLogAbs(anaRatio)
	t.Logf("median absolute log-ratio against the exact count: empirical %.2f, analytic %.2f "+
		"(0 is perfect; 1.0 means off by a factor of e)", empErr, anaErr)

	if empErr > anaErr {
		t.Errorf("the empirical estimator (%.2f) is no better than the analytic one (%.2f) on a "+
			"lognormal pool, which is the case it was introduced to fix", empErr, anaErr)
	}
	if empErr > 1.2 {
		t.Errorf("empirical estimate is off by more than a factor of e^1.2 on average (%.2f)", empErr)
	}
	_ = total
}

func binom3(n int) int64 { return int64(n) * int64(n-1) * int64(n-2) / 6 }

func geomMeanLogAbs(xs []float64) float64 {
	var s float64
	for _, x := range xs {
		if x <= 0 {
			x = 1e-9
		}
		s += math.Abs(math.Log(x))
	}
	return s / float64(len(xs))
}

// TestEmpiricalEstimateIsDeterministic asserts the sampled gate replays.
//
// A reconciliation decision that changes between two runs of the same data is
// not auditable, whatever else is true of it. The sample seed is fixed and
// carried on the receipt for exactly this reason.
func TestEmpiricalEstimateIsDeterministic(t *testing.T) {
	rng := rand.New(rand.NewSource(3))
	contribs := make([]money.Paise, 60)
	for i := range contribs {
		contribs[i] = money.Paise(rng.Intn(9_000_000) + 100_000)
	}
	target := contribs[0] + contribs[7] + contribs[19]

	a := Assess(contribs, target, 1, nil, DefaultConfig())
	b := Assess(contribs, target, 1, nil, DefaultConfig())

	if a.KStar != b.KStar || a.IndexAtKStar != b.IndexAtKStar || a.Decision != b.Decision {
		t.Fatalf("the sampled gate is not deterministic:\n  run A: k*=%d E=%.6g %s\n  run B: k*=%d E=%.6g %s",
			a.KStar, a.IndexAtKStar, a.Decision, b.KStar, b.IndexAtKStar, b.Decision)
	}
	if a.Estimator != "empirical_subset_sampling" {
		t.Fatalf("estimator = %q, want the sampled one by default", a.Estimator)
	}
}
