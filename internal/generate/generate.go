package generate

import (
	"fmt"
	"math/rand"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/Rishi0507/manhattan/internal/accounting"
	"github.com/Rishi0507/manhattan/internal/model"
	"github.com/Rishi0507/manhattan/internal/money"
)

// Spec parameterises a generated dataset.
type Spec struct {
	Seed int64
	// Archetype selects the amount distribution.
	Archetype string
	// Settlements is how many bank credits to produce.
	Settlements int
	// BatchSize is the number of records in each true settlement batch.
	BatchSize int
	// BatchJitter randomises the batch size by plus or minus this many
	// records, so the benchmark is not measuring one cardinality.
	BatchJitter int
	// PoolJitter varies the pool size run to run, so the benchmark reports a
	// distribution across difficulty rather than one operating point.
	PoolJitter int
	// PoolTarget is roughly how many candidates should survive narrowing.
	// This, together with BatchSize, is what actually determines whether a
	// settlement is verifiable, which is why the generator exposes it
	// directly rather than leaving it to emerge.
	PoolTarget int

	// ChargebackRate, RefundRate and FullRefundShare drive the signed items.
	ChargebackRate  float64
	RefundRate      float64
	FullRefundShare float64
	AdjustmentRate  float64

	// Mode selects how much of the gateway's report the pipeline may see.
	Mode model.DataMode
	// DeclareTxnCount attaches the report's own transaction count to each
	// credit, which the solver may use to scope its search.
	DeclareTxnCount bool
	// JoinDisputes wires the disputes feed into the record universe. Leaving
	// it false is what creates a residual only an agent can explain.
	JoinDisputes bool

	// ReportDefectRate is the fraction of settlement reports whose stated
	// mapping does not match the money that actually moved.
	//
	// Six per cent is the shipped default and it is deliberately modest. Real
	// gateway reports are good, and the argument this supports does not need
	// them to be bad. It needs them to be occasionally wrong in a way nothing
	// downstream can detect, which is a different and much weaker claim, and
	// the one that is actually true. A rate chosen to make the comparison
	// dramatic would be making a dishonest point.
	ReportDefectRate float64

	// FeeDriftBps applies a systematic overcharge to the observed fee rows,
	// which the fee detector should surface without the reconciliation itself
	// being affected.
	FeeDriftBps int64

	// NegotiatedMDRBps is this merchant's ACTUAL rate, where it differs from
	// the published schedule the pipeline is configured with. Zero means the
	// merchant is on the published rate.
	//
	// MissingFeeRowRate is the fraction of payments whose per-payment fee row
	// the settlement report does not carry.
	//
	// Together these two are what make the composite's false-alarm rate a
	// measurement instead of a tautology, and the tautology is worth spelling
	// out because it was real.
	//
	// The benchmark's generator and the pipeline's accounting engine compute
	// contributions from the same policy. So a settlement report that was
	// entirely correct could never disagree with the claim check: both sides
	// were deriving the same number from the same schedule, and "zero false
	// alarms on 469 clean reports" measured nothing at all.
	//
	// A negotiated rate breaks that, and it is the most ordinary thing in
	// Indian payments. A merchant is signed at 185 bps rather than the
	// published 200. The gateway applies 185, the credit reflects 185, and
	// wherever the report carries a per-payment fee row the pipeline uses it
	// and agrees. Where the report does NOT carry a fee row, and real reports
	// have gaps, the pipeline falls back to the schedule it was configured
	// with, computes 200, and disagrees with a report that was correct.
	//
	// That is a false alarm with a real cause, and its rate is now something
	// this benchmark can be wrong about.
	NegotiatedMDRBps  int64
	MissingFeeRowRate float64

	// NoMappingRate is the fraction of settlements the report carries no
	// mapping for at all.
	//
	// This is the population the claim check cannot help with and where
	// reconstruction is the only route to a posting. A bank credit whose
	// narration carries no usable settlement reference, a merchant on a
	// gateway that ships only a net figure, a historical period being
	// backfilled: in every one of those the report has nothing to check.
	//
	// It matters because the composite's headline is mostly checked claims,
	// and a reader is entitled to ask what the solver is for. This is the
	// answer, measured rather than argued.
	NoMappingRate float64

	Policy accounting.Policy
	Start  time.Time

	// Pathology labels the adversarial case, for benchmark reporting.
	Pathology string
}

// DefaultSpec is a plausible mid-market merchant.
func DefaultSpec() Spec {
	return Spec{
		Seed:             20260826,
		Archetype:        "marketplace",
		Settlements:      20,
		BatchSize:        6,
		BatchJitter:      2,
		PoolTarget:       34,
		PoolJitter:       10,
		ChargebackRate:   0.08,
		RefundRate:       0.15,
		FullRefundShare:  0.25,
		AdjustmentRate:   0.02,
		Mode:             model.ModeMappingWithheld,
		DeclareTxnCount:  true,
		JoinDisputes:     true,
		ReportDefectRate: 0.06,
		Policy:           generatorPolicy(),
		Start:            time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC),
	}
}

type builder struct {
	spec Spec
	rng  *rand.Rand
	arch Archetype
	ds   *model.Dataset
	seq  int
}

func (b *builder) id(prefix string) string {
	b.seq++
	return fmt.Sprintf("%s_%06d", prefix, b.seq)
}

// Generate builds one dataset with ground truth.
//
// The construction is deliberately physical rather than adversarial by
// default. Each settlement's true batch is a set of captures from one day;
// the decoys that survive narrowing are captures from the adjacent days that
// fall inside the value-date window, which is exactly where a real
// reconciler's false candidates come from. Making decoys arbitrary random
// amounts would produce an easier problem than the real one.
func Generate(spec Spec) *model.Dataset {
	if spec.Policy.Version == "" {
		spec.Policy = accounting.DefaultPolicy()
	}
	if spec.Start.IsZero() {
		spec.Start = time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	}
	b := &builder{
		spec: spec,
		rng:  rand.New(rand.NewSource(spec.Seed)),
		arch: ByName(spec.Archetype),
	}
	b.ds = &model.Dataset{
		Mode:              spec.Mode,
		DisputesJoined:    spec.JoinDisputes,
		GroundTruth:       map[string][]string{},
		ReportedMapping:   map[string][]string{},
		ReportDefects:     map[string]string{},
		ReportDefectClass: map[string]string{},
		Pathology:         spec.Pathology,
	}

	merchant := model.Merchant{
		ID:                   "mid_" + spec.Archetype,
		Name:                 displayName(spec.Archetype),
		Archetype:            spec.Archetype,
		SettlementCycleDays:  b.arch.SettlementCycleDays,
		InstrumentSegregated: b.arch.InstrumentSegregated,
	}
	b.ds.Merchants = append(b.ds.Merchants, merchant)

	// A second merchant supplies the records that the merchant-ID constraint
	// exists to drop. Without one, that constraint's drop count is always
	// zero and the audit trail says nothing.
	other := model.Merchant{
		ID: "mid_other", Name: "Adjacent Merchant Pvt Ltd",
		Archetype: spec.Archetype, SettlementCycleDays: b.arch.SettlementCycleDays,
	}
	b.ds.Merchants = append(b.ds.Merchants, other)

	cycle := merchant.SettlementCycleDays

	for s := 0; s < spec.Settlements; s++ {
		captureDay := spec.Start.AddDate(0, 0, s)
		valueDate := captureDay.AddDate(0, 0, cycle)

		size := spec.BatchSize
		if spec.BatchJitter > 0 {
			size += b.rng.Intn(2*spec.BatchJitter+1) - spec.BatchJitter
		}
		if size < 1 {
			size = 1
		}

		batch := b.captureDay(merchant, captureDay, size, true)

		// Decoys: same-day captures held back to a later cycle. They are the
		// records narrowing cannot remove and the solver has to distinguish
		// between, so their count is what actually sets the difficulty.
		poolTarget := spec.PoolTarget
		if spec.PoolJitter > 0 {
			poolTarget += b.rng.Intn(2*spec.PoolJitter+1) - spec.PoolJitter
		}
		decoys := poolTarget - size
		if decoys < 0 {
			decoys = 0
		}
		b.captureHoldovers(merchant, captureDay, decoys)

		// Noise the narrowing layer must remove: a different merchant, and
		// records already posted in a prior cycle.
		b.captureDay(other, captureDay, size, false)
		b.reconciledNoise(merchant, captureDay, size/2+1)

		var target money.Paise
		var truth []string
		for _, id := range batch {
			target += b.contributionOf(id)
			truth = append(truth, id)
		}
		sort.Strings(truth)

		// The merchant is part of the reference, and it has to be.
		//
		// Without it the reference is date plus sequence, which is unique
		// within one merchant and collides across them: a batch run over six
		// archetypes produced six settlements called
		// bank_credit_2026_08_04_1001, carrying different pools and different
		// verdicts. Nothing downstream was wrong, but a reader of the
		// exception queue saw one identifier with three contradictory statuses
		// and concluded the receipt store was broken, in the exact document
		// asking them to trust it. An identifier that cannot be trusted to
		// name one thing is not an identifier.
		ref := fmt.Sprintf("bank_credit_%s_%s_%04d",
			strings.TrimPrefix(merchant.ID, "mid_"), valueDate.Format("2006_01_02"), 1000+s)
		credit := model.BankCredit{
			Ref:        ref,
			Narration:  b.narration(merchant, valueDate, s),
			Amount:     target,
			ValueDate:  valueDate,
			MerchantID: merchant.ID,
			Currency:   "INR",
		}
		if spec.DeclareTxnCount {
			n := len(truth)
			credit.DeclaredTxnCount = &n
		}
		b.ds.Credits = append(b.ds.Credits, credit)
		b.ds.GroundTruth[ref] = truth
		b.reportMapping(ref, truth, merchant.ID)
	}

	return b.ds
}

// captureDay creates a day's worth of captures, with their associated
// refunds and chargebacks, and returns the record IDs that belong to that
// day's settlement batch.
func (b *builder) captureDay(m model.Merchant, day time.Time, n int, inBatch bool) []string {
	var ids []string
	for i := 0; i < n; i++ {
		at := day.Add(captureHour(b.rng))
		p := model.Payment{
			ID:         b.id("pay"),
			OrderID:    b.id("order"),
			MerchantID: m.ID,
			Instrument: b.arch.Instrument(b.rng),
			Currency:   "INR",
			Gross:      b.arch.Ticket(b.rng),
			CapturedAt: at,
		}
		b.attachFee(&p)
		b.ds.Payments = append(b.ds.Payments, p)
		b.ds.Orders = append(b.ds.Orders, model.Order{
			ID: p.OrderID, PaymentID: p.ID, MerchantID: m.ID,
			Gross: p.Gross, GLAccount: "4310", PlacedAt: at.Add(-30 * time.Minute),
		})
		if inBatch {
			ids = append(ids, p.ID)
		}

		// A refund netted in the same cycle reduces this payment's
		// contribution; a full one turns it negative, because the gateway
		// keeps its fee.
		if b.rng.Float64() < b.spec.RefundRate {
			full := b.rng.Float64() < b.spec.FullRefundShare
			amt := p.Gross
			if !full {
				amt = money.Paise(int64(p.Gross) / int64(2+b.rng.Intn(4)))
			}
			b.ds.Refunds = append(b.ds.Refunds, model.Refund{
				ID: b.id("rfnd"), PaymentID: p.ID, MerchantID: m.ID,
				Amount: amt, SettledAt: at.Add(6 * time.Hour), Full: full,
			})
		}
	}

	// Chargebacks are separate records with their own negative contribution.
	if inBatch && b.spec.ChargebackRate > 0 && b.rng.Float64() < b.spec.ChargebackRate*float64(n) {
		disputed := b.arch.Ticket(b.rng)
		cb := model.Chargeback{
			ID:         b.id("cbk"),
			MerchantID: m.ID,
			Disputed:   disputed,
			Fee:        b.spec.Policy.DisputeFee,
			DebitedAt:  day.Add(12 * time.Hour),
		}
		b.ds.Chargebacks = append(b.ds.Chargebacks, cb)
		ids = append(ids, cb.ID)
	}
	return ids
}

// captureHoldovers creates captures on the batch day that do not belong to
// the batch, because they were held for risk review or capture confirmation
// and will settle in a later cycle.
//
// These are the decoys, and they are modelled this way rather than as
// arbitrary noise for a reason: they share the batch's merchant, currency,
// capture day and amount distribution, so no narrowing constraint removes
// them. They are exactly the candidates a real reconciler has to distinguish
// between on amounts alone, which is the problem this system is about.
func (b *builder) captureHoldovers(m model.Merchant, day time.Time, n int) {
	if n <= 0 {
		return
	}
	for i := 0; i < n; i++ {
		at := day.Add(captureHour(b.rng))
		p := model.Payment{
			ID:         b.id("pay"),
			OrderID:    b.id("order"),
			MerchantID: m.ID,
			Instrument: b.arch.Instrument(b.rng),
			Currency:   "INR",
			Gross:      b.arch.Ticket(b.rng),
			CapturedAt: at,
		}
		b.attachFee(&p)
		b.ds.Payments = append(b.ds.Payments, p)
		b.ds.Orders = append(b.ds.Orders, model.Order{
			ID: p.OrderID, PaymentID: p.ID, MerchantID: m.ID,
			Gross: p.Gross, GLAccount: "4310", PlacedAt: at.Add(-30 * time.Minute),
		})
	}
}

// captureHour places a capture inside the trading day, between 04:00 and
// 20:00.
//
// The bounds matter more than they look. The default narrowing window is
// fourteen hours either side of the capture day's midpoint, so a capture at
// 02:00 or 23:00 sits within a couple of hours of the window edge and the
// NEXT day's early captures fall inside this day's window. That leaked
// roughly eighteen extra records into every pool, which is invisible in the
// data and highly visible in the results: pool size drives C(n, k), so an
// unintended fifty per cent inflation moved settlements from verifiable to
// ambiguous for reasons that had nothing to do with the reconciler.
func captureHour(rng *rand.Rand) time.Duration {
	return time.Duration(4+rng.Intn(17)) * time.Hour
}

func (b *builder) reconciledNoise(m model.Merchant, day time.Time, n int) {
	for i := 0; i < n; i++ {
		at := day.Add(captureHour(b.rng))
		b.ds.Payments = append(b.ds.Payments, model.Payment{
			ID: b.id("pay"), OrderID: b.id("order"), MerchantID: m.ID,
			Instrument: b.arch.Instrument(b.rng), Currency: "INR",
			Gross: b.arch.Ticket(b.rng), CapturedAt: at, Reconciled: true,
		})
	}
}

// contributionOf computes a record's signed contribution under the spec's
// policy, so the generator can compute the exact bank credit its batch would
// produce. This mirrors the accounting engine deliberately: if the two ever
// disagree, the benchmark's ground truth would be wrong and every accuracy
// number with it, so the agreement is asserted in the generator's tests.
func (b *builder) contributionOf(id string) money.Paise {
	for _, c := range b.ds.Chargebacks {
		if c.ID == id {
			return -(c.Disputed + c.Fee)
		}
	}
	for _, a := range b.ds.Adjustments {
		if a.ID == id {
			return a.Amount
		}
	}
	for _, p := range b.ds.Payments {
		if p.ID != id {
			continue
		}
		// The credit reflects what was actually deducted, which is the drifted
		// fee where drift was configured. That is what makes the fee detector
		// a real finding rather than a restatement: the settlement still
		// reconstructs exactly, and the rate applied to it is still wrong.
		// What ACTUALLY came out of the credit, which is FeeApplied wherever
		// the generator recorded it: the drifted rate where drift was
		// configured, the negotiated rate where the merchant has one, and the
		// published schedule otherwise.
		//
		// This is deliberately independent of whether the REPORT carries a fee
		// row. A payment whose fee row is missing still had a fee deducted,
		// and the credit reflects it. The pipeline cannot see that and falls
		// back to the schedule, which is exactly the divergence that makes a
		// false alarm possible.
		mdr := b.spec.Policy.MDR(p.Instrument, p.Gross)
		gst := b.spec.Policy.GST(mdr)
		if p.FeeApplied > 0 || p.FeeRowMissing {
			mdr, gst = p.FeeApplied, p.TaxApplied
		} else if p.FeeObserved != nil {
			mdr = *p.FeeObserved
			if p.TaxObserved != nil {
				gst = *p.TaxObserved
			}
		}
		var refunded money.Paise
		for _, r := range b.ds.Refunds {
			if r.PaymentID == p.ID {
				refunded += r.Amount
			}
		}
		return p.Gross - mdr - gst - refunded
	}
	return 0
}

// attachFee populates the per-payment fee row the settlement report would
// carry, applying any configured drift.
//
// These rows exist in data modes 1 and 2 and are what make the fee check
// independent rather than circular. In lump-credit mode they are withheld
// entirely, and the pipeline then refuses to make a fee claim at all.
func (b *builder) attachFee(p *model.Payment) {
	if !b.spec.Mode.FeesObserved() {
		return
	}
	bps := b.spec.Policy.MDRByInstrument[p.Instrument] + b.spec.FeeDriftBps
	// A negotiated rate overrides the published schedule for this merchant.
	// UPI stays at zero: it is zero by regulation, not by negotiation.
	if b.spec.NegotiatedMDRBps > 0 && p.Instrument != model.InstrumentUPI {
		bps = b.spec.NegotiatedMDRBps + b.spec.FeeDriftBps
	}
	if bps < 0 {
		bps = 0
	}
	fee := money.MulRateBPS(p.Gross, bps, b.spec.Policy.FeeRounding)
	tax := money.MulRateBPS(fee, b.spec.Policy.GSTBps, b.spec.Policy.TaxRounding)

	// The money moved either way. What varies is whether the REPORT tells us
	// how much, and a report with a gap is a report the pipeline has to guess
	// at from the schedule it was configured with.
	p.FeeApplied, p.TaxApplied = fee, tax
	if b.spec.MissingFeeRowRate > 0 && b.rng.Float64() < b.spec.MissingFeeRowRate {
		p.FeeRowMissing = true
		return
	}
	p.FeeObserved = &fee
	p.TaxObserved = &tax
}

func (b *builder) narration(m model.Merchant, valueDate time.Time, seq int) string {
	utr := fmt.Sprintf("%06d", 100000+b.rng.Intn(899999))
	forms := []string{
		"NEFT-RAZORPAY SOFTWARE PVT LTD-UTR%s-CR",
		"IMPS/P2A/%s/RAZORPAY SOFTWARE/SETTLEMENT",
		"RTGS CR RAZORPAYSOFTWAREPVTLTD REF %s NET STLMNT",
		"UPI/CREDIT/%s/RAZORPAY/SETTLE/NA",
	}
	return fmt.Sprintf(forms[seq%len(forms)], utr)
}

func displayName(archetype string) string {
	switch archetype {
	case "travel":
		return "Meridian Travel Pvt Ltd"
	case "marketplace":
		return "Bazaar Commerce Pvt Ltd"
	case "d2c_ecommerce":
		return "Kettle & Co D2C Pvt Ltd"
	case "utility_billpay":
		return "Sahyadri Utilities Ltd"
	case "subscription_saas":
		return "Northwind SaaS Pvt Ltd"
	case "quick_commerce":
		return "TenMinute Retail Pvt Ltd"
	}
	return "Merchant Pvt Ltd"
}

// reportMapping records what the gateway's settlement report SAYS this
// settlement is made of, which is usually but not always the truth.
//
// The three defects modelled are the ones that actually happen, and each one
// is invisible to anything that reads the report as authority:
//
//   - a chargeback debited in this cycle but raised against an earlier one,
//     which the settlement report omits because its own join is by capture
//     date. The money moved; the mapping does not mention it.
//   - a payment named in the mapping that actually settled in the previous
//     cycle, double-counted across a cycle boundary.
//   - a mapping that is simply short by one, which is what a partial write or
//     a truncated file looks like downstream.
//
// None of these is exotic and none is a strawman. The point is not that
// gateway reports are unreliable, it is that a reconciliation with no
// independent account of the money cannot tell a correct report from a
// defective one, because the only thing it can check the report against is
// the report.
func (b *builder) reportMapping(ref string, truth []string, merchantID string) {
	stated := append([]string(nil), truth...)
	defect, class := "", ""

	if b.spec.ReportDefectRate > 0 && b.rng.Float64() < b.spec.ReportDefectRate && len(stated) > 1 {
		switch b.rng.Intn(3) {
		case 0:
			// Omit a signed item. The report's own join misses it.
			drop := -1
			for i, id := range stated {
				if c := b.contributionOf(id); c < 0 {
					drop = i
					break
				}
			}
			if drop < 0 {
				// No signed item in this batch, so the defect that gets
				// injected is a plain omission rather than a dispute. The
				// label has to follow the data, not the intent.
				drop = b.rng.Intn(len(stated))
				class = "TRUNCATED_MAPPING"
			}
			defect = "omits " + stated[drop] + ", a record that moved money in this cycle"
			// A dispute debited in this cycle but raised against an earlier
			// one, which the report's capture-date join misses.
			class = "OMITTED_DISPUTE"
			stated = append(stated[:drop:drop], stated[drop+1:]...)

		case 1:
			// Name a record that belongs to a different cycle.
			var outside []string
			for _, p := range b.ds.Payments {
				if p.MerchantID == merchantID && !contains(truth, p.ID) {
					outside = append(outside, p.ID)
				}
			}
			if len(outside) == 0 {
				return
			}
			sort.Strings(outside)
			add := outside[b.rng.Intn(len(outside))]
			defect = "names " + add + ", which settled in a different cycle"
			class = "CROSS_CYCLE_MEMBER"
			stated = append(stated, add)

		case 2:
			drop := b.rng.Intn(len(stated))
			defect = "is short by one record, " + stated[drop]
			class = "TRUNCATED_MAPPING"
			stated = append(stated[:drop:drop], stated[drop+1:]...)
		}
	}

	sort.Strings(stated)
	if b.spec.NoMappingRate > 0 && b.rng.Float64() < b.spec.NoMappingRate {
		// The report carries no mapping for this settlement at all. There is
		// nothing to check and reconstruction is the only route.
		return
	}
	b.ds.ReportedMapping[ref] = stated
	if defect != "" {
		b.ds.ReportDefects[ref] = defect
		b.ds.ReportDefectClass[ref] = class
	}
}

func contains(xs []string, v string) bool {
	for _, x := range xs {
		if x == v {
			return true
		}
	}
	return false
}

// generatorPolicy selects the schedule the synthetic data is built with.
//
// The divergent schedule is the default because a false-alarm figure measured
// against the verifier's own constants is not a measurement. The shared
// schedule is kept reachable so the two arms can be compared directly, which
// is the only way to show the divergence actually changes the data rather than
// being asserted to:
//
//	MANHATTAN_FEE_MODEL=shared ./bin/manhattan bench -n 120
func generatorPolicy() accounting.Policy {
	if os.Getenv("MANHATTAN_FEE_MODEL") == "shared" {
		return accounting.DefaultPolicy()
	}
	return accounting.GeneratorPolicy()
}
