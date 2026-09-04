// Package model holds the domain types Manhattan reconciles over.
//
// Four sources feed the pipeline: a payment gateway settlement report, a
// bank statement, an OMS/ledger export, and a disputes feed. They arrive
// separately, they disagree, and the whole point of the system is to decide
// what the disagreement means.
package model

import (
	"time"

	"github.com/Rishi0507/manhattan/internal/money"
)

// Instrument is the payment method. Fee policy is per-instrument, and some
// settlements are instrument-segregated, which makes this a narrowing
// constraint as well as a pricing input.
type Instrument string

const (
	InstrumentCard       Instrument = "card"
	InstrumentUPI        Instrument = "upi"
	InstrumentNetbanking Instrument = "netbanking"
	InstrumentWallet     Instrument = "wallet"
	InstrumentEMI        Instrument = "emi"
)

// AllInstruments is the iteration order used anywhere output must be stable.
var AllInstruments = []Instrument{
	InstrumentCard, InstrumentUPI, InstrumentNetbanking, InstrumentWallet, InstrumentEMI,
}

// RecordKind distinguishes the shapes of thing that can contribute to a
// settlement. A settlement batch is not a list of payments; it is a list of
// events, and several of them are debits.
type RecordKind string

const (
	KindPayment    RecordKind = "payment"
	KindRefund     RecordKind = "refund"
	KindChargeback RecordKind = "chargeback"
	KindAdjustment RecordKind = "adjustment"
)

// Payment is one captured transaction as the gateway reports it.
type Payment struct {
	ID         string      `json:"id"`
	OrderID    string      `json:"order_id"`
	MerchantID string      `json:"merchant_id"`
	Instrument Instrument  `json:"instrument"`
	Currency   string      `json:"currency"`
	Gross      money.Paise `json:"gross_paise"`

	CapturedAt time.Time `json:"captured_at"`

	// FeeObserved and TaxObserved are what the settlement report *claims* was
	// deducted. They are present in data modes 1 and 2 and absent in mode 3.
	// They are deliberately separate from what policy says should have been
	// deducted; comparing the two is Leg C, and it is only meaningful while
	// these are sourced independently.
	FeeObserved *money.Paise `json:"fee_observed_paise,omitempty"`
	TaxObserved *money.Paise `json:"tax_observed_paise,omitempty"`

	// FeeApplied and TaxApplied are what ACTUALLY came out of the bank credit,
	// which is not always what the report says and not always what the policy
	// says either. The generator records them; the pipeline never reads them.
	//
	// They exist so the benchmark can build a credit that reflects the money
	// that really moved, including on payments whose fee row the report does
	// not carry. Without them a merchant on a negotiated rate would be
	// indistinguishable from one on the published schedule, and the claim
	// check could never raise a false alarm because both sides would be
	// deriving the same number from the same policy.
	FeeApplied money.Paise `json:"-"`
	TaxApplied money.Paise `json:"-"`
	// FeeRowMissing marks a payment the settlement report carries with no
	// per-payment fee row. Real reports have gaps, and a gap forces the
	// pipeline back onto the schedule it was configured with.
	FeeRowMissing bool `json:"fee_row_missing,omitempty"`

	// SettlementID is the gateway's own mapping claim. Manhattan treats it as
	// a claim to be verified, not as an answer, and the demo posture withholds
	// it entirely (data mode 2).
	SettlementID string `json:"settlement_id,omitempty"`

	// Reconciled marks a payment already posted in a prior cycle. Such a
	// payment is not available to this settlement, and forgetting that is one
	// of the more common ways a reconciler produces a plausible wrong answer.
	Reconciled bool `json:"reconciled"`
}

// Refund is a return of funds. Netted refunds reduce the settlement in the
// cycle they clear, which may not be the cycle the payment settled in.
type Refund struct {
	ID         string      `json:"id"`
	PaymentID  string      `json:"payment_id"`
	MerchantID string      `json:"merchant_id"`
	Amount     money.Paise `json:"amount_paise"`
	SettledAt  time.Time   `json:"settled_at"`
	// Full marks a refund of the entire gross. The MDR on the original
	// payment is generally not returned, which is what makes such a record
	// contribute a negative amount.
	Full bool `json:"full"`
}

// Chargeback is a disputed transaction debited back, plus the dispute fee
// the gateway charges for handling it. Always a negative contribution.
type Chargeback struct {
	ID         string      `json:"id"`
	PaymentID  string      `json:"payment_id"`
	MerchantID string      `json:"merchant_id"`
	Disputed   money.Paise `json:"disputed_paise"`
	Fee        money.Paise `json:"dispute_fee_paise"`
	DebitedAt  time.Time   `json:"debited_at"`
}

// Total is the full negative impact of the dispute on the settlement.
func (c Chargeback) Total() money.Paise { return c.Disputed + c.Fee }

// Adjustment is anything else the gateway nets off or adds: recoveries,
// penalties, promotional credits, manual corrections. Signed as configured.
type Adjustment struct {
	ID         string      `json:"id"`
	MerchantID string      `json:"merchant_id"`
	Amount     money.Paise `json:"amount_paise"`
	Reason     string      `json:"reason"`
	AppliedAt  time.Time   `json:"applied_at"`
}

// Order is the merchant's own record, from their OMS or ledger. Leg B joins
// it to the gateway's payment rows on a key, and the gross amounts it
// carries are what Leg A actually reconstructs from in lump-credit mode.
type Order struct {
	ID         string      `json:"id"`
	PaymentID  string      `json:"payment_id"`
	MerchantID string      `json:"merchant_id"`
	Gross      money.Paise `json:"gross_paise"`
	GLAccount  string      `json:"gl_account"`
	PlacedAt   time.Time   `json:"placed_at"`
}

// BankCredit is one line on the bank statement: a single number that is the
// net of everything above. Recovering what produced it is the whole problem.
type BankCredit struct {
	Ref string `json:"ref"`
	// Narration is the free-text the bank supplies, e.g.
	// "NEFT-RAZORPAY SOFTWARE PVT LTD-UTR3491-CR". It is unstructured, it
	// varies by bank, and parsing it is the agent's first job.
	Narration string      `json:"narration"`
	Amount    money.Paise `json:"amount_paise"`
	ValueDate time.Time   `json:"value_date"`
	// MerchantID is resolved by the parser from the narration and the account
	// the credit landed in; it is not always directly present in the text.
	MerchantID string `json:"merchant_id"`
	Currency   string `json:"currency"`

	// DeclaredTxnCount is the transaction count the settlement report states
	// for this batch, where a report exists. Using it tightens the search
	// substantially, and it also scopes the resulting proof by a number the
	// counterparty supplied. Both facts end up on the receipt.
	DeclaredTxnCount *int `json:"declared_txn_count,omitempty"`

	// Instrument is set only where the merchant's payouts are
	// instrument-segregated, in which case it is a hard narrowing constraint.
	Instrument Instrument `json:"instrument,omitempty"`
}

// Merchant carries the per-merchant configuration that narrowing and pricing
// depend on.
type Merchant struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Archetype string `json:"archetype"`
	// SettlementCycleDays is the T+n the gateway settles on.
	SettlementCycleDays int `json:"settlement_cycle_days"`
	// InstrumentSegregated marks a merchant whose payouts arrive as one
	// credit per instrument rather than one credit per cycle. Where true,
	// instrument becomes a hard narrowing constraint.
	InstrumentSegregated bool `json:"instrument_segregated"`
}

// DataMode records how much of the gateway's own report is available and
// trusted. It determines whether the solver is needed at all and whether the
// fee check carries information; see docs/DESIGN.md sections 7 and 15.
type DataMode string

const (
	// ModeFullReport: mapping present and trusted. Leg A is a lookup.
	ModeFullReport DataMode = "full_report_mapping_trusted"
	// ModeMappingWithheld: per-payment fee rows exist, the settlement-to-credit
	// mapping does not. The solver is required and the fee check is
	// independent. This is the demo posture.
	ModeMappingWithheld DataMode = "report_present_mapping_withheld"
	// ModeLumpCredit: a bank credit and the merchant's own orders, nothing
	// else. The solver is required and the fee check is circular, so Manhattan
	// makes no fee claim at all.
	ModeLumpCredit DataMode = "lump_credit_only"
)

// FeesObserved reports whether fee figures come from a source independent of
// the policy config. This single predicate gates both the fee anomaly claim
// and the gross-ratio completeness guard, because both are tautological
// without it.
func (m DataMode) FeesObserved() bool {
	return m == ModeFullReport || m == ModeMappingWithheld
}

// Dataset is one complete multi-source input: everything the pipeline is
// allowed to look at for a run.
type Dataset struct {
	Merchants   []Merchant   `json:"merchants"`
	Payments    []Payment    `json:"payments"`
	Refunds     []Refund     `json:"refunds"`
	Chargebacks []Chargeback `json:"chargebacks"`
	Adjustments []Adjustment `json:"adjustments"`
	Orders      []Order      `json:"orders"`
	Credits     []BankCredit `json:"bank_credits"`

	// DisputesJoined records whether the disputes feed was wired into the
	// candidate pool. In case 9 it is not, which is precisely the condition
	// the resolution agent exists to notice and repair.
	DisputesJoined bool `json:"disputes_joined"`

	// GroundTruth maps a bank credit ref to the record IDs that genuinely
	// produced it. The pipeline never reads this. Only the benchmark does,
	// and only after a decision has been made.
	GroundTruth map[string][]string `json:"ground_truth,omitempty"`

	// Pathology names the adversarial case this dataset was built to exhibit,
	// for the benchmark's reporting only.
	Pathology string `json:"pathology,omitempty"`

	Mode DataMode `json:"data_mode"`

	// ReportedMapping is what the gateway's settlement report states each
	// credit is composed of, and ReportDefects names the ones where that
	// statement is wrong.
	//
	// The PIPELINE NEVER READS EITHER. They exist so the benchmark can score
	// a lookup-based reconciliation, which is the thing every payments person
	// correctly points out already exists. Manhattan reconstructs from the
	// merchant's own records and is then compared against this; reading it
	// would make the comparison circular and the whole exercise pointless.
	ReportedMapping map[string][]string `json:"-"`
	ReportDefects   map[string]string   `json:"-"`
	// ReportDefectClass is the TRUE class of each injected defect, kept so the
	// model's diagnosis can be scored against it rather than admired.
	//
	// A model job with no accuracy figure is a model job nobody can evaluate,
	// and "the AI classifies the defect" is worth exactly as much as the
	// measurement beside it.
	ReportDefectClass map[string]string `json:"-"`
}

// Record is the uniform shape the pipeline works in after Stage 3. Every
// eligible payment, refund, chargeback and adjustment becomes exactly one
// Record with a signed contribution in paise.
type Record struct {
	ID         string      `json:"id"`
	Kind       RecordKind  `json:"kind"`
	MerchantID string      `json:"merchant_id"`
	Instrument Instrument  `json:"instrument"`
	Currency   string      `json:"currency"`
	EventAt    time.Time   `json:"event_at"`
	Gross      money.Paise `json:"gross_paise"`

	// Contribution is the signed net effect on the bank credit, computed by
	// the accounting engine under the declared policy. This is the number the
	// solver sums.
	Contribution money.Paise `json:"contribution_paise"`

	// Lo and Hi bracket the contribution in inferred rounding mode, where the
	// convention is unknown. In declared mode both equal Contribution.
	Lo money.Paise `json:"lo_paise"`
	Hi money.Paise `json:"hi_paise"`

	// Components are retained so the independent verification at Stage 6 can
	// re-derive the accounting equation from the raw parts without reusing
	// anything the solver touched.
	// MDR and GST are what was actually deducted from this record. PolicyMDR
	// and PolicyGST are what the configured schedule says should have been.
	// Keeping both is what lets the fee detector be a second opinion rather
	// than a restatement of its own input.
	MDR         money.Paise  `json:"mdr_paise"`
	GST         money.Paise  `json:"gst_paise"`
	PolicyMDR   money.Paise  `json:"policy_mdr_paise"`
	PolicyGST   money.Paise  `json:"policy_gst_paise"`
	Refund      money.Paise  `json:"refund_paise"`
	Chargeback  money.Paise  `json:"chargeback_paise"`
	Adjustment  money.Paise  `json:"adjustment_paise"`
	FeeObserved *money.Paise `json:"fee_observed_paise,omitempty"`
	// FeeCalibrated marks a contribution priced at the merchant's own observed
	// effective rate because the report carried no fee row for it. It is a
	// better guess than the configured schedule and it is still a guess, which
	// is what the fee-basis guard exists to weigh.
	FeeCalibrated bool `json:"fee_calibrated,omitempty"`

	Reconciled   bool   `json:"reconciled"`
	SettlementID string `json:"settlement_id,omitempty"`
}

// Negative reports whether this record debits the settlement.
func (r Record) Negative() bool { return r.Contribution < 0 }

// Contributions extracts the signed amounts in pool order. The solver takes
// nothing else; it never sees an ID, a date or a merchant.
func Contributions(rs []Record) []money.Paise {
	out := make([]money.Paise, len(rs))
	for i, r := range rs {
		out[i] = r.Contribution
	}
	return out
}

// IDs extracts record identifiers in pool order, for receipts.
func IDs(rs []Record) []string {
	out := make([]string, len(rs))
	for i, r := range rs {
		out[i] = r.ID
	}
	return out
}
