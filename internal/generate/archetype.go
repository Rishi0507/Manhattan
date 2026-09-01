// Package generate builds synthetic multi-source datasets with ground truth.
//
// The generator is not a convenience. It is the measuring instrument: every
// accuracy claim this project makes is a comparison against the ground truth
// it records, and the adversarial cases in cases.go are what stop the
// benchmark from being a demonstration that easy things are easy.
//
// Ground truth is written into the dataset and never read by the pipeline.
// Only the benchmark reads it, and only after a decision has been made.
package generate

import (
	"math"
	"math/rand"

	"github.com/Rishi0507/manhattan/internal/model"
	"github.com/Rishi0507/manhattan/internal/money"
)

// Archetype describes a merchant's transaction amount distribution.
//
// Contribution spread is the dominant variable in whether a merchant's
// settlements can be reconstructed at all, so archetypes are defined by
// their ticket shape and everything else follows. This turns what reads as a
// limitation into a segmentation: the system can tell a merchant, from one
// pass over their historical settlement amounts, roughly what fraction of
// their settlements it expects to auto-post, before any integration.
//
// These distributions are chosen to be plausible for each archetype. They
// are a statement about how the estimator behaves across distribution
// shapes, not a claim about the composition of the Indian merchant base, and
// presenting them as market data would be the same failure this project
// refuses everywhere else.
type Archetype struct {
	Name string
	// Shape selects how a ticket amount is drawn.
	Shape Shape
	// LogMean and LogSigma parameterise the lognormal shapes, in rupees.
	LogMean, LogSigma float64
	// Denominations are the fixed ticket prices for clustered shapes, in paise.
	Denominations []money.Paise
	// MinRupees and MaxRupees clamp the draw.
	MinRupees, MaxRupees float64
	// InstrumentMix is the probability weight per payment method.
	InstrumentMix map[model.Instrument]float64
	// SettlementCycleDays is the T+n this merchant settles on.
	SettlementCycleDays  int
	InstrumentSegregated bool
	// ExpectedRegime is a human label, reported alongside measured results
	// so the prediction and the observation sit side by side.
	ExpectedRegime string
}

// Shape names a draw method.
type Shape string

const (
	ShapeLognormal   Shape = "lognormal"
	ShapeDenominated Shape = "denominated"
	ShapeUniformWide Shape = "uniform_wide"
)

// Archetypes is the shipped segmentation, ordered from most reconstructable
// to least.
var Archetypes = []Archetype{
	{
		Name: "travel", Shape: ShapeLognormal,
		LogMean: math.Log(28000), LogSigma: 0.95,
		MinRupees: 8000, MaxRupees: 200000,
		InstrumentMix: map[model.Instrument]float64{
			model.InstrumentCard: 0.55, model.InstrumentNetbanking: 0.20,
			model.InstrumentUPI: 0.15, model.InstrumentEMI: 0.10,
		},
		SettlementCycleDays: 2,
		ExpectedRegime:      "verifies readily",
	},
	{
		Name: "marketplace", Shape: ShapeLognormal,
		LogMean: math.Log(3200), LogSigma: 1.35,
		MinRupees: 200, MaxRupees: 50000,
		InstrumentMix: map[model.Instrument]float64{
			model.InstrumentUPI: 0.45, model.InstrumentCard: 0.35,
			model.InstrumentNetbanking: 0.12, model.InstrumentWallet: 0.08,
		},
		SettlementCycleDays: 2,
		ExpectedRegime:      "verifies readily",
	},
	{
		Name: "d2c_ecommerce", Shape: ShapeLognormal,
		LogMean: math.Log(2400), LogSigma: 0.72,
		MinRupees: 800, MaxRupees: 8000,
		InstrumentMix: map[model.Instrument]float64{
			model.InstrumentUPI: 0.52, model.InstrumentCard: 0.33,
			model.InstrumentWallet: 0.10, model.InstrumentNetbanking: 0.05,
		},
		SettlementCycleDays: 2,
		ExpectedRegime:      "contested; narrowing decides",
	},
	{
		Name: "utility_billpay", Shape: ShapeDenominated,
		Denominations: []money.Paise{
			money.FromRupees(299), money.FromRupees(399), money.FromRupees(499),
			money.FromRupees(599), money.FromRupees(799), money.FromRupees(999),
			money.FromRupees(1199), money.FromRupees(1499),
		},
		InstrumentMix: map[model.Instrument]float64{
			model.InstrumentUPI: 0.70, model.InstrumentNetbanking: 0.20, model.InstrumentCard: 0.10,
		},
		SettlementCycleDays: 1,
		ExpectedRegime:      "contested; entropy gate often fires",
	},
	{
		Name: "subscription_saas", Shape: ShapeDenominated,
		Denominations: []money.Paise{
			money.FromRupees(499), money.FromRupees(999), money.FromRupees(1999),
		},
		InstrumentMix: map[model.Instrument]float64{
			model.InstrumentCard: 0.70, model.InstrumentUPI: 0.30,
		},
		SettlementCycleDays: 2,
		ExpectedRegime:      "entropy gate refuses first",
	},
	{
		Name: "quick_commerce", Shape: ShapeLognormal,
		LogMean: math.Log(420), LogSigma: 0.42,
		MinRupees: 150, MaxRupees: 1200,
		InstrumentMix: map[model.Instrument]float64{
			model.InstrumentUPI: 0.82, model.InstrumentCard: 0.12, model.InstrumentWallet: 0.06,
		},
		SettlementCycleDays: 1,
		ExpectedRegime:      "refused; needs a settlement reference",
	},
}

// ByName looks up an archetype.
func ByName(name string) Archetype {
	for _, a := range Archetypes {
		if a.Name == name {
			return a
		}
	}
	return Archetypes[2]
}

// Ticket draws one gross amount in paise.
func (a Archetype) Ticket(rng *rand.Rand) money.Paise {
	switch a.Shape {
	case ShapeDenominated:
		return a.Denominations[rng.Intn(len(a.Denominations))]
	case ShapeUniformWide:
		r := a.MinRupees + rng.Float64()*(a.MaxRupees-a.MinRupees)
		return money.Paise(math.Round(r * 100))
	default:
		for attempt := 0; attempt < 32; attempt++ {
			r := math.Exp(rng.NormFloat64()*a.LogSigma + a.LogMean)
			if r >= a.MinRupees && r <= a.MaxRupees {
				// Sub-rupee variation is what gives a lognormal merchant its
				// amount entropy in the first place, so paise are drawn
				// rather than rounded away.
				return money.Paise(math.Round(r*100)) + money.Paise(rng.Intn(100))
			}
		}
		return money.Paise(math.Round(a.MinRupees * 100))
	}
}

// Instrument draws a payment method from the archetype's mix.
func (a Archetype) Instrument(rng *rand.Rand) model.Instrument {
	x := rng.Float64()
	var cum float64
	for _, inst := range model.AllInstruments {
		w, ok := a.InstrumentMix[inst]
		if !ok {
			continue
		}
		cum += w
		if x <= cum {
			return inst
		}
	}
	return model.InstrumentCard
}
