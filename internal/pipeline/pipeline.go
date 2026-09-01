// Package pipeline runs the seven stages over one bank credit and produces
// exactly one receipt.
//
// The ordering is deliberate and one part of it is easy to miss: the gate
// runs before the solver, not after it. Because the gate's output k* is the
// parameter the solver is dispatched on, triage is not a pre-check bolted
// onto the front of the search. It is what configures the search.
//
// Trust flows downward. The agent acts at stage 1, reading an unstructured
// narration into typed fields; at stage 2, choosing which constraints to
// relax and in what order; and at stage 7, as a bounded proposer whose every
// output is an executable assertion the verifier is free to reject. It has
// no write access to a posting decision at any point.
package pipeline

import (
	"fmt"
	"sort"
	"time"

	"github.com/Rishi0507/manhattan/internal/accounting"
	"github.com/Rishi0507/manhattan/internal/entropy"
	"github.com/Rishi0507/manhattan/internal/evidence"
	"github.com/Rishi0507/manhattan/internal/feasibility"
	"github.com/Rishi0507/manhattan/internal/guards"
	"github.com/Rishi0507/manhattan/internal/model"
	"github.com/Rishi0507/manhattan/internal/money"
	"github.com/Rishi0507/manhattan/internal/narrow"
	"github.com/Rishi0507/manhattan/internal/solver"
)

// ScopePolicy selects how the search region is bounded.
type ScopePolicy string

const (
	// ScopePolicyGate always uses the gate-derived k*. The uniqueness claim
	// is then unconditional within the searched region and owes nothing to
	// the counterparty's report. It is the right posture when the report is
	// itself the thing under test, and it verifies less often, because k*
	// sits at the contested boundary by construction.
	ScopePolicyGate ScopePolicy = "gate"

	// ScopePolicyDeclaredWhenAvailable uses the report's declared transaction
	// count where one exists, falling back to the gate otherwise. Cheaper,
	// and frequently the difference between a verified settlement and an
	// ambiguous one. The claim is weaker and the receipt says so explicitly,
	// recording both the scope used and the scope the gate would have used.
	ScopePolicyDeclaredWhenAvailable ScopePolicy = "declared_when_available"
)

// Config is the whole system's configuration for a run.
type Config struct {
	Accounting  accounting.Config
	Narrow      narrow.Config
	Entropy     entropy.Config
	Feasibility feasibility.Config

	ScopePolicy ScopePolicy

	// ProbeDepth is the substitution depth of the neighbourhood completeness
	// probe. Depth 2 covers any rival differing from the witness by up to two
	// records, at any witness size.
	ProbeDepth int
	// ProbeWindowWidening is how much the value-date window is loosened when
	// testing narrowing sensitivity.
	ProbeWindowWidening time.Duration

	// AnalystMinutesPerException and AnalystHourlyINR price the exception
	// queue, so a finance lead can sort a backlog by money.
	AnalystMinutesPerException int
	AnalystHourlyINR           int

	RunID string
	Seed  int64
}

// DefaultConfig returns the shipped configuration.
func DefaultConfig() Config {
	return Config{
		Accounting:  accounting.DefaultConfig(),
		Narrow:      narrow.DefaultConfig(),
		Entropy:     entropy.DefaultConfig(),
		Feasibility: feasibility.DefaultConfig(),
		ScopePolicy: ScopePolicyDeclaredWhenAvailable,

		ProbeDepth:          2,
		ProbeWindowWidening: 24 * time.Hour,

		AnalystMinutesPerException: 20,
		AnalystHourlyINR:           1000,

		Seed: 20260826,
	}
}

// Engine holds the immutable inputs for a run.
type Engine struct {
	Cfg       Config
	Records   []model.Record
	ByID      map[string]model.Record
	Merchants map[string]model.Merchant
	Mode      model.DataMode

	// Unjoined holds source records that exist but were never wired into the
	// candidate pool, such as a disputes feed nobody connected. The pipeline
	// never reads it. Only the resolution agent searches it, and only to find
	// a citation for a hypothesis the verifier has already been handed.
	Unjoined []model.Record

	// universe is the record set the current reconciliation is running over,
	// which differs from Records only when a hypothesis overlay is applied.
	universe []model.Record
}

// New builds an engine over a dataset, computing every record's signed
// contribution once.
func New(ds *model.Dataset, cfg Config) *Engine {
	// Whether contributions follow the observed fee rows or the policy is a
	// property of the data mode, not a free choice. Deriving it here means a
	// caller cannot accidentally configure a circular fee check into
	// independence.
	cfg.Accounting.UseObservedFees = ds.Mode.FeesObserved()
	recs := accounting.Build(ds, cfg.Accounting)
	byID := make(map[string]model.Record, len(recs))
	for _, r := range recs {
		byID[r.ID] = r
	}
	ms := make(map[string]model.Merchant, len(ds.Merchants))
	for _, m := range ds.Merchants {
		ms[m.ID] = m
	}
	eng := &Engine{Cfg: cfg, Records: recs, ByID: byID, Merchants: ms, Mode: ds.Mode, universe: recs}

	// Records from feeds that were never joined are kept to one side. They are
	// invisible to the pipeline and searchable only by the resolution agent.
	if !ds.DisputesJoined {
		joined := *ds
		joined.DisputesJoined = true
		joined.Payments, joined.Refunds, joined.Adjustments = nil, nil, nil
		for _, r := range accounting.Build(&joined, cfg.Accounting) {
			eng.Unjoined = append(eng.Unjoined, r)
		}
	}
	return eng
}

type stopwatch struct {
	t   time.Time
	out map[string]int64
}

func newStopwatch() *stopwatch { return &stopwatch{t: time.Now(), out: map[string]int64{}} }

func (s *stopwatch) mark(name string) {
	now := time.Now()
	s.out[name] = now.Sub(s.t).Milliseconds()
	s.t = now
}

// Overlay is a hypothesis applied to the inputs before the pipeline runs.
//
// This is the mechanism that keeps the resolution agent outside the trust
// boundary. An agent hypothesis is not a hint, a weight or a prior. It is a
// concrete edit to the pool or to the target, expressed as data, and the
// entire gate, solver, completeness and recomputation stack then runs over
// the edited inputs completely unchanged. The model is never consulted about
// whether its own suggestion worked.
type Overlay struct {
	// ExtraRecords are added to the record universe before narrowing, so a
	// proposed record still has to survive every business constraint.
	ExtraRecords []model.Record
	// TargetDelta adjusts the credit being reconstructed, for hypotheses
	// about charges levied against the payout rather than against the batch.
	TargetDelta money.Paise
	// Provenance names what produced this overlay, for the receipt.
	Provenance string
}

// Reconcile runs the full pipeline for one bank credit.
func (e *Engine) Reconcile(credit model.BankCredit) *evidence.Receipt {
	return e.ReconcileWith(credit, nil)
}

// ReconcileWith runs the pipeline with a hypothesis applied.
func (e *Engine) ReconcileWith(credit model.BankCredit, ov *Overlay) *evidence.Receipt {
	sw := newStopwatch()
	cfg := e.Cfg
	merchant := e.Merchants[credit.MerchantID]

	records := e.Records
	if ov != nil {
		if len(ov.ExtraRecords) > 0 {
			records = append(append([]model.Record{}, e.Records...), ov.ExtraRecords...)
		}
		credit.Amount -= ov.TargetDelta
	}

	rec := &evidence.Receipt{
		SettlementRef: credit.Ref,
		RunID:         cfg.RunID,
		MerchantID:    credit.MerchantID,
		MerchantName:  merchant.Name,
		Archetype:     merchant.Archetype,
		DataMode:      e.Mode,
		Narration:     credit.Narration,
		TargetPaise:   credit.Amount,
		ValueDate:     credit.ValueDate.Format("2006-01-02"),
		Flags:         []evidence.Flag{},
		PolicyVersion: cfg.Accounting.Policy.Version,
		ReplaySeed:    cfg.Seed,
		Rounding: evidence.RoundingBlock{
			Mode:           cfg.Accounting.Mode,
			TolerancePaise: cfg.Accounting.EffectiveDelta(),
			BandBasis:      "cardinality",
		},
	}
	if cfg.Accounting.Mode == accounting.ModeInferred {
		rec.AddFlag(evidence.FlagRoundingApplied)
	}

	// ---- Stage 2: narrow -------------------------------------------------
	nCfg := cfg.Narrow
	nCfg.CycleDays = merchant.SettlementCycleDays
	if nCfg.CycleDays == 0 {
		nCfg.CycleDays = cfg.Narrow.CycleDays
	}
	nCfg.EnforceInstrument = merchant.InstrumentSegregated
	narrowed := narrow.Apply(records, credit, merchant, nCfg)
	sw.mark("narrow")

	rec.Narrowing = evidence.NarrowingBlock{
		Before:      narrowed.Before,
		After:       narrowed.After,
		Dropped:     narrowed.Dropped,
		WindowHours: narrowed.WindowHours,
		Applied:     narrowed.Applied,
	}
	for _, d := range narrowed.DropLog {
		if d.Constraint == narrow.ConstraintZeroContribution {
			rec.Narrowing.ZeroContributionRecords = append(
				rec.Narrowing.ZeroContributionRecords, d.RecordID)
		}
	}

	pool := narrowed.Pool
	contribs := model.Contributions(pool)
	signed := 0
	for _, c := range contribs {
		if c < 0 {
			signed++
		}
	}
	if signed > 0 {
		rec.AddFlag(evidence.FlagSignedItemsPresent)
	}
	rec.Pool = evidence.PoolBlock{
		N:           len(pool),
		SigmaPaise:  feasibility.Sigma(contribs),
		SignedItems: signed,
		TotalPaise:  money.Sum(contribs),
	}

	// ---- Stage 4a: amount entropy ---------------------------------------
	eCfg := cfg.Entropy
	eCfg.Delta = cfg.Accounting.EffectiveDelta()
	ent := entropy.Analyse(contribs, eCfg)
	rec.AmountEntropy = ent
	sw.mark("entropy_gate")

	if ent.LatticeGCD > 1 {
		rec.AddFlag(evidence.FlagLatticeCorrected)
	}
	if !ent.Pass {
		rec.Status = evidence.StatusUnderdetermined
		rec.AddFlag(evidence.FlagEntropyInsufficient)
		rec.Claim = fmt.Sprintf(
			"the pool holds %d distinct contribution values across %d records, with %.0f%% of it inside twin classes; "+
				"amounts do not distinguish these transactions from one another",
			ent.DistinctValues, len(pool), ent.TwinMass*100)
		rec.Remediation = entropyRemediation(merchant)
		e.finish(rec, sw, credit, pool, nil)
		return rec
	}

	// ---- Stage 4b: feasibility ------------------------------------------
	feas := feasibility.Assess(contribs, credit.Amount, ent.LatticeGCD, credit.DeclaredTxnCount, cfg.Feasibility)
	rec.Feasibility = feas
	sw.mark("feasibility")

	if feas.Underdetermined() {
		rec.Status = evidence.StatusUnderdetermined
		if feas.Decision == feasibility.DecideResourceCeiling {
			rec.AddFlag(evidence.FlagResourceCeiling)
		}
		rec.Claim = refusalClaim(feas, len(pool))
		rec.Note = "no witness is exhibited by design: displaying one arbitrary member of a population " +
			"this large would misrepresent it as an answer"
		rec.Remediation = underdeterminedRemediation(feas, contribs, ent.LatticeGCD, cfg)
		e.finish(rec, sw, credit, pool, nil)
		return rec
	}

	// ---- Stage 5: reconstruct -------------------------------------------
	kMax, scope := e.chooseScope(credit, feas)
	delta := cfg.Accounting.EffectiveDelta()
	sr := solver.Solve(contribs, credit.Amount, kMax, delta, scope)
	sw.mark("reconstruct")

	rec.Solver = &evidence.SolverBlock{
		Method:        "cardinality_restricted_meet_in_the_middle",
		KMax:          sr.KMax,
		KMaxSource:    sr.ScopeSource,
		Split:         sr.Split,
		EntriesLeft:   sr.EntriesLeft,
		EntriesRight:  sr.EntriesRght,
		EntryEncoding: "int64 sum + uint32 colex rank, 12 bytes",
		MemoryBytes:   sr.MemoryBytes,
		MemoryCeiling: cfg.Feasibility.MemoryCeilingBytes,
		ProbedTargets: []string{"T", "pool_total_minus_T"},
		DedupApplied:  sr.DedupApplied,
		DedupRemoved:  sr.DedupRemoved,
	}
	if sr.Nearest.Valid {
		m := sr.Nearest
		rec.Solver.NearestMiss = &m
	}

	e.universe = records
	e.decide(rec, sr, feas, ent, pool, narrowed, credit, merchant, kMax, scope)
	sw.mark("prove")

	e.finish(rec, sw, credit, pool, sr)
	return rec
}

// chooseScope decides what bounds the search region and records the choice.
func (e *Engine) chooseScope(credit model.BankCredit, feas feasibility.Report) (int, solver.ScopeSource) {
	if e.Cfg.ScopePolicy == ScopePolicyDeclaredWhenAvailable &&
		credit.DeclaredTxnCount != nil && feas.ImpliedFreeCardinality != nil {
		k := *feas.ImpliedFreeCardinality
		if k >= 0 && k <= feas.KStar {
			return k, solver.ScopeDeclared
		}
	}
	return feas.KStar, solver.ScopeGate
}

// decide turns a solver result into a status, applying every guard in order.
func (e *Engine) decide(
	rec *evidence.Receipt,
	sr *solver.Result,
	feas feasibility.Report,
	ent entropy.Report,
	pool []model.Record,
	narrowed narrow.Result,
	credit model.BankCredit,
	merchant model.Merchant,
	kMax int,
	scope solver.ScopeSource,
) {
	cfg := e.Cfg

	uq := &evidence.Uniqueness{
		Method:                  "exhaustive_enumeration_count",
		Scope:                   fmt.Sprintf("all subsets with k(S) <= %d", sr.KMax),
		ScopeSource:             sr.ScopeSource,
		ScopeComplete:           sr.ScopeComplete && !sr.Capped,
		MatchesFound:            sr.Matches,
		RivalsFound:             sr.Rivals,
		CountedAfterDedup:       sr.DedupApplied,
		CountSaturated:          sr.Capped,
		CumulativeIndexInRegion: cumulativeIndex(feas, sr.KMax),
	}
	if scope == solver.ScopeDeclared {
		k := feas.KStar
		uq.KMaxIfGateDerived = &k
		uq.ScopeNote = fmt.Sprintf(
			"the region was bounded by the report's declared transaction count, not by the gate; "+
				"the gate would have searched k(S) <= %d, where an estimated %.2f subsets hit this target",
			feas.KStar, cumulativeIndex(feas, feas.KStar))
	}
	rec.Uniqueness = uq

	if sr.Matches == 0 {
		rec.Status = evidence.StatusUnresolved
		gap := money.Paise(0)
		if sr.Nearest.Valid {
			gap = sr.Nearest.Gap
		}
		rec.Claim = fmt.Sprintf(
			"no combination of the %d candidate records reconstructs this credit within the declared tolerance; "+
				"the nearest achievable sum is %s short or over, a residual of %s",
			len(pool), gap, gap)
		rec.Remediation = []evidence.Remediation{{
			Action: "search for an unjoined source that could explain the residual",
			Effect: "the resolution agent proposes typed hypotheses and the verifier re-runs unmodified over each",
		}}
		return
	}

	if sr.Matches > 1 {
		rec.Status = evidence.StatusAmbiguous
		rec.Claim = fmt.Sprintf(
			"%d distinct reconstructions of this credit exist within the searched region; "+
				"the arithmetic cannot choose between them",
			sr.Matches)
		for _, w := range sr.Witnesses {
			uq.AlternativeWitnesses = append(uq.AlternativeWitnesses, idsOf(pool, w.Indices))
		}
		if len(sr.Witnesses) > 0 {
			e.attachWitness(rec, pool, sr.Witnesses[0], credit)
			// The twin check runs here as well as on the unique path. The flag
			// records that identical contributions make a rival CONSTRUCTIBLE,
			// which is true whether or not the enumeration happened to find
			// that rival on its own. It also tells an analyst something the
			// bare count does not: the ambiguity is structural, so no amount
			// of further narrowing on amounts will resolve it.
			if _, from, to, ok := entropy.SwapRival(sr.Witnesses[0].Indices, ent); ok {
				rec.AddFlag(evidence.FlagTwinSwap)
				rec.Claim = fmt.Sprintf(
					"%s. %s and %s carry an identical contribution, so an alternative "+
						"reconstruction is constructible by exchanging one for the other; "+
						"this ambiguity is structural rather than incidental",
					rec.Claim, pool[from].ID, pool[to].ID)
			}
		}
		return
	}

	// Exactly one match in the searched region.
	w := sr.Witnesses[0]
	witness := recordsAt(pool, w.Indices)
	e.attachWitness(rec, pool, w, credit)
	if w.FromComplement {
		rec.AddFlag(evidence.FlagComplementSolved)
		rec.Solver.SolveSide = "complement"
	} else {
		rec.Solver.SolveSide = "witness"
	}

	// Guard 0: a twin swap constructs a rival outright, with no search.
	if rival, from, to, ok := entropy.SwapRival(w.Indices, ent); ok {
		rec.Status = evidence.StatusAmbiguous
		rec.AddFlag(evidence.FlagTwinSwap)
		uq.RivalsFound = 1
		uq.MatchesFound = 2
		uq.AlternativeWitnesses = [][]string{
			idsOf(pool, w.Indices), idsOf(pool, rival),
		}
		rec.Claim = fmt.Sprintf(
			"an alternative reconstruction is constructible by exchanging %s for %s, which carry an identical amount; "+
				"ambiguity here is proved rather than searched for",
			pool[from].ID, pool[to].ID)
		return
	}

	// Guard 1: independent re-derivation of the accounting identity.
	eq := accounting.Recompute(witness, credit.Amount, cfg.Accounting)
	rec.Accounting = &eq
	rec.Rounding.SlackAllowed = eq.SlackAllowed
	rec.Rounding.SlackConsumed = eq.SlackConsumed

	if !eq.Closes {
		rec.Status = evidence.StatusUnresolved
		rec.Claim = fmt.Sprintf(
			"the solver found a reconstruction but the accounting identity re-derived from the raw records "+
				"does not close: a residual of %s remains against an allowance of %s. "+
				"When these two disagree the solver is wrong and nothing posts",
			eq.Residual, eq.SlackAllowed)
		return
	}

	// Guard 2: pool completeness. This is the dangerous case, so it runs on
	// every candidate posting rather than on a sample.
	probe := e.neighbourhoodProbe(witness, credit, merchant, narrowed)
	rec.Narrowing.Neighbourhood = &probe

	// Guard 3 and 4: the declared-count cross-check and the gross-ratio check,
	// the latter of which reports itself inactive rather than passing where it
	// cannot carry information.
	rec.Narrowing.Checks = []guards.Check{
		guards.CardinalityCrossCheck(len(witness), credit.DeclaredTxnCount,
			narrowed.Dropped[narrow.ConstraintZeroContribution], scope == solver.ScopeDeclared),
		guards.GrossRatioCheck(witness, pool, e.Mode.FeesObserved(), cfg.Accounting.Policy.BandBps),
	}

	if !probe.Stable {
		rec.Status = evidence.StatusNarrowingSensitive
		culprit := probe.Culprit
		if culprit == "" {
			culprit = narrow.ConstraintWindow
		}
		rec.Claim = fmt.Sprintf(
			"widening the pool by relaxing %s admits an alternative reconstruction of the same credit; "+
				"this answer came from a filtering decision rather than from the arithmetic",
			culprit)
		rec.Remediation = []evidence.Remediation{{
			Action: fmt.Sprintf("confirm the %s constraint is correctly configured for this merchant", culprit),
			Effect: "either the rival is genuinely ineligible, in which case the answer stands, or the batch is misidentified",
		}}
		return
	}
	for _, c := range rec.Narrowing.Checks {
		if c.State == guards.CheckFail {
			rec.Status = evidence.StatusUnresolved
			rec.Claim = "a completeness guard failed: " + c.Detail
			return
		}
	}

	rec.Status = evidence.StatusVerified
	rec.Claim = fmt.Sprintf(
		"this credit is reconstructed exactly by %d records, and that reconstruction is unique among %s",
		len(witness), uq.Scope)
	if z := len(rec.Narrowing.ZeroContributionRecords); z > 0 {
		rec.Claim += fmt.Sprintf(
			". A further %d record(s) in this window net to exactly zero and are named separately: "+
				"they moved no money and cannot be attributed from the credit by any method",
			z)
	}
}

func (e *Engine) attachWitness(rec *evidence.Receipt, pool []model.Record, w solver.Witness, credit model.BankCredit) {
	rec.Witness = idsOf(pool, w.Indices)
	rec.WitnessSize = len(w.Indices)
	for _, i := range w.Indices {
		if pool[i].Contribution < 0 {
			rec.NegativeMembers = append(rec.NegativeMembers, pool[i].ID)
		}
	}
}

// neighbourhoodProbe widens the pool one constraint at a time and searches
// the neighbourhood of the witness already in hand.
func (e *Engine) neighbourhoodProbe(witness []model.Record, credit model.BankCredit, merchant model.Merchant, base narrow.Result) guards.NeighbourhoodResult {
	cfg := e.Cfg
	nCfg := cfg.Narrow
	nCfg.CycleDays = merchant.SettlementCycleDays
	if nCfg.CycleDays == 0 {
		nCfg.CycleDays = cfg.Narrow.CycleDays
	}
	nCfg.EnforceInstrument = merchant.InstrumentSegregated

	inBase := map[string]bool{}
	for _, r := range base.Pool {
		inBase[r.ID] = true
	}

	// Widen the value-date window first, then drop one constraint at a time,
	// tracking which relaxation admitted each newly available record so that
	// a rival can be attributed to a specific rule.
	admitted := map[string]narrow.Constraint{}
	widened := append([]model.Record{}, base.Pool...)
	tested := []narrow.Constraint{narrow.ConstraintWindow}

	wide := narrow.Apply(e.universe, credit, merchant, nCfg.WithWindow(nCfg.Window+cfg.ProbeWindowWidening))
	for _, r := range wide.Pool {
		if !inBase[r.ID] {
			widened = append(widened, r)
			inBase[r.ID] = true
			admitted[r.ID] = narrow.ConstraintWindow
		}
	}

	for _, c := range narrow.RelaxationOrder {
		if c == narrow.ConstraintWindow || c == narrow.ConstraintMerchant {
			continue // the window is already covered; merchant is never relaxed
		}
		tested = append(tested, c)
		rel := narrow.Apply(e.universe, credit, merchant, nCfg.WithRelaxed(c))
		for _, r := range rel.Pool {
			if !inBase[r.ID] {
				widened = append(widened, r)
				inBase[r.ID] = true
				admitted[r.ID] = c
			}
		}
	}

	return guards.Probe(witness, widened, cfg.ProbeDepth,
		cfg.Accounting.EffectiveDelta(), tested, admitted)
}

// finish attaches the fee check, the exception price and the timings.
func (e *Engine) finish(rec *evidence.Receipt, sw *stopwatch, credit model.BankCredit, pool []model.Record, sr *solver.Result) {
	if rec.Witness != nil {
		witness := make([]model.Record, 0, len(rec.Witness))
		for _, id := range rec.Witness {
			if r, ok := e.ByID[id]; ok {
				witness = append(witness, r)
			}
		}
		fc := FeeAnomaly(witness, e.Cfg.Accounting, e.Mode)
		rec.FeeCheck = &fc
		if fc.Circular {
			rec.AddFlag(evidence.FlagFeeCheckCircular)
		} else if !fc.WithinBand {
			rec.AddFlag(evidence.FlagFeeAnomaly)
		}
	}
	sw.mark("fee_check")

	if !rec.Status.Postable() {
		rec.ExceptionCostINR = e.Cfg.AnalystMinutesPerException * e.Cfg.AnalystHourlyINR / 60
	}
	rec.TimingMS = sw.out
	sort.Slice(rec.Flags, func(i, j int) bool { return rec.Flags[i] < rec.Flags[j] })
}

func idsOf(pool []model.Record, idx []int) []string {
	out := make([]string, len(idx))
	for i, j := range idx {
		out[i] = pool[j].ID
	}
	return out
}

func recordsAt(pool []model.Record, idx []int) []model.Record {
	out := make([]model.Record, len(idx))
	for i, j := range idx {
		out[i] = pool[j]
	}
	return out
}

func cumulativeIndex(feas feasibility.Report, k int) float64 {
	var t float64
	for _, p := range feas.Curve {
		if p.K <= k {
			t += p.Index
		}
	}
	return t
}
