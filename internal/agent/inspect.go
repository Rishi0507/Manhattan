package agent

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Rishi0507/manhattan/internal/evidence"
	"github.com/Rishi0507/manhattan/internal/money"
)

// Inspector serves bounded slices of a receipt store to the controller.
//
// The close was a single call: aggregates in, report out. That is a summariser,
// not a controller. A controller reads a number, forms a suspicion, goes and
// looks at the thing the number is about, and either confirms or drops it.
//
// So the close is now a loop, and this is what it reads with. Each slice is
// bounded in size, derived from receipts rather than from anything the
// benchmark knows, and costs one turn. The controller pays for what it looks
// at, which is the same discipline the settlement agent operates under and for
// the same reason: an investigator with unlimited free lookups does not have to
// think about which one to make.
//
// Nothing here can write. Every method returns text.
type Inspector struct {
	store *evidence.Store
	arch  map[string]string
}

// NewInspector indexes a finished run for reading.
func NewInspector(store *evidence.Store, archOf map[string]string) *Inspector {
	return &Inspector{store: store, arch: archOf}
}

// SliceKind names what the controller may look at next.
type SliceKind string

const (
	// SliceMerchant is every held settlement for one merchant type, with the
	// figures that distinguish a narrowing problem from a missing-data one.
	SliceMerchant SliceKind = "INSPECT_MERCHANT"
	// SliceStatus is the held population in one status, across merchants,
	// which is how a cause that spans merchants becomes visible.
	SliceStatus SliceKind = "INSPECT_STATUS"
	// SliceResiduals is the settlements where nothing reconstructs and the
	// shortfall is exact, which is the signature of a record nobody joined.
	SliceResiduals SliceKind = "INSPECT_RESIDUALS"
	// SliceRemedies is what the system already computed and verified, which
	// is the difference between a diagnosis and a recommendation.
	SliceRemedies SliceKind = "INSPECT_REMEDIES"
	// SliceClaims is the settlements whose report failed its arithmetic check,
	// with the diagnosis attached.
	SliceClaims SliceKind = "INSPECT_CLAIM_FAILURES"
	// SliceWrite ends the loop and composes the close.
	SliceWrite SliceKind = "WRITE_CLOSE"
)

// AllSlices is the closed vocabulary offered to the controller.
var AllSlices = []SliceKind{
	SliceMerchant, SliceStatus, SliceResiduals,
	SliceRemedies, SliceClaims, SliceWrite,
}

const maxSliceRows = 12

// Serve returns the requested slice, bounded, as text.
func (in *Inspector) Serve(kind SliceKind, arg string) string {
	switch kind {
	case SliceMerchant:
		return in.merchant(arg)
	case SliceStatus:
		return in.status(arg)
	case SliceResiduals:
		return in.residuals()
	case SliceRemedies:
		return in.remedies()
	case SliceClaims:
		return in.claimFailures()
	}
	return "no such slice"
}

func (in *Inspector) held() []*evidence.Receipt {
	var out []*evidence.Receipt
	for _, r := range in.store.All() {
		if r.Status == evidence.StatusVerified {
			continue
		}
		if r.ReportClaim != nil && r.ReportClaim.Verdict == evidence.ClaimConsistent {
			continue
		}
		out = append(out, r)
	}
	return out
}

func (in *Inspector) archOf(r *evidence.Receipt) string {
	if a := in.arch[r.SettlementRef]; a != "" {
		return a
	}
	return r.Archetype
}

func (in *Inspector) merchant(name string) string {
	var b strings.Builder
	var rows []*evidence.Receipt
	for _, r := range in.held() {
		if strings.EqualFold(in.archOf(r), name) {
			rows = append(rows, r)
		}
	}
	if len(rows) == 0 {
		return fmt.Sprintf("no held settlements for %q. Merchant types in this run are named "+
			"in the period summary; use one of those exactly.", name)
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].TargetPaise.Abs() > rows[j].TargetPaise.Abs() })

	fmt.Fprintf(&b, "%s: %d held settlements. The %d largest by value:\n\n", name, len(rows),
		minInt(len(rows), maxSliceRows))
	for _, r := range rows[:minInt(len(rows), maxSliceRows)] {
		fmt.Fprintf(&b, "  %s %s  pool %d  declared %s  twin %.2f  index %.3g\n",
			r.Status, r.TargetPaise, r.Pool.N, declared(r), r.AmountEntropy.TwinMass,
			r.Feasibility.IndexAtKStar)
		fmt.Fprintf(&b, "      %s\n", clipLine(r.Claim, 110))
		if r.Solver != nil && r.Solver.NearestMiss != nil && r.Solver.NearestMiss.Valid {
			fmt.Fprintf(&b, "      nearest achievable sum is %s away at cardinality %d\n",
				r.Solver.NearestMiss.Gap, r.Solver.NearestMiss.Cardinality)
		}
	}
	return b.String()
}

func (in *Inspector) status(st string) string {
	var b strings.Builder
	byArch := map[string]int{}
	var pool, twin, index float64
	var n float64
	var sample []*evidence.Receipt
	for _, r := range in.held() {
		if !strings.EqualFold(string(r.Status), st) {
			continue
		}
		byArch[in.archOf(r)]++
		pool += float64(r.Pool.N)
		twin += r.AmountEntropy.TwinMass
		index += r.Feasibility.IndexAtKStar
		n++
		if len(sample) < 6 {
			sample = append(sample, r)
		}
	}
	if n == 0 {
		return fmt.Sprintf("no held settlements with status %q", st)
	}
	fmt.Fprintf(&b, "%s: %.0f held settlements.\n  mean pool %.0f, mean twin mass %.2f, mean index %.3g\n",
		strings.ToUpper(st), n, pool/n, twin/n, index/n)
	fmt.Fprintf(&b, "  by merchant type:\n")
	var keys []string
	for k := range byArch {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return byArch[keys[i]] > byArch[keys[j]] })
	for _, k := range keys {
		fmt.Fprintf(&b, "    %-20s %d\n", k, byArch[k])
	}
	fmt.Fprintf(&b, "  examples:\n")
	for _, r := range sample {
		fmt.Fprintf(&b, "    %s %s pool %d: %s\n", in.archOf(r), r.TargetPaise, r.Pool.N,
			clipLine(r.Claim, 90))
	}
	return b.String()
}

func (in *Inspector) residuals() string {
	var b strings.Builder
	type row struct {
		r   *evidence.Receipt
		gap money.Paise
	}
	var rows []row
	for _, r := range in.held() {
		if r.Status != evidence.StatusUnresolved || r.Solver == nil ||
			r.Solver.NearestMiss == nil || !r.Solver.NearestMiss.Valid {
			continue
		}
		rows = append(rows, row{r, r.Solver.NearestMiss.Gap})
	}
	if len(rows) == 0 {
		return "no settlement in this run has an exact unexplained residual"
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].gap.Abs() > rows[j].gap.Abs() })

	byArch := map[string]int{}
	for _, x := range rows {
		byArch[in.archOf(x.r)]++
	}
	fmt.Fprintf(&b, "%d settlements where nothing reconstructs the credit and the shortfall is exact.\n",
		len(rows))
	fmt.Fprintf(&b, "  by merchant type: %v\n\n", byArch)
	fmt.Fprintf(&b, "  An exact residual means the arithmetic is sound and a record is absent.\n")
	fmt.Fprintf(&b, "  The %d largest:\n", minInt(len(rows), maxSliceRows))
	for _, x := range rows[:minInt(len(rows), maxSliceRows)] {
		fmt.Fprintf(&b, "    %-18s residual %12s  pool %3d  %s\n",
			in.archOf(x.r), x.gap, x.r.Pool.N, x.r.SettlementRef)
	}
	return b.String()
}

func (in *Inspector) remedies() string {
	type agg struct {
		n     int
		value money.Paise
	}
	byAction := map[string]*agg{}
	for _, r := range in.held() {
		for _, rem := range r.Remediation {
			a := byAction[rem.Action]
			if a == nil {
				a = &agg{}
				byAction[rem.Action] = a
			}
			a.n++
			a.value += r.TargetPaise.Abs()
		}
	}
	if len(byAction) == 0 {
		return "no remedies were computed for the held population"
	}
	var keys []string
	for k := range byAction {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return byAction[keys[i]].value > byAction[keys[j]].value })

	var b strings.Builder
	fmt.Fprintf(&b, "Remedies already computed and re-verified, by held value:\n\n")
	for _, k := range keys[:minInt(len(keys), maxSliceRows)] {
		fmt.Fprintf(&b, "  %4d settlements, %14s held : %s\n",
			byAction[k].n, byAction[k].value, clipLine(k, 100))
	}
	return b.String()
}

func (in *Inspector) claimFailures() string {
	byClass := map[string]int{}
	var sample []*evidence.Receipt
	total := 0
	for _, r := range in.store.All() {
		c := r.ReportClaim
		if c == nil || c.Verdict != evidence.ClaimContradicted {
			continue
		}
		total++
		if c.Diagnosis != nil {
			byClass[c.Diagnosis.Class]++
		}
		if len(sample) < 6 {
			sample = append(sample, r)
		}
	}
	if total == 0 {
		return "no settlement report failed its arithmetic check in this run"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%d settlements where the gateway's stated mapping failed its check.\n", total)
	fmt.Fprintf(&b, "  diagnosed classes: %v\n\n", byClass)
	for _, r := range sample {
		fmt.Fprintf(&b, "  %-18s %s residual %s\n", in.archOf(r), r.SettlementRef,
			money.Paise(r.ReportClaim.ResidualPaise))
		fmt.Fprintf(&b, "      %s\n", clipLine(r.ReportClaim.Note, 110))
	}
	return b.String()
}

func declared(r *evidence.Receipt) string {
	if n := r.Feasibility.DeclaredTxnCount; n != nil {
		return fmt.Sprintf("%d", *n)
	}
	return "unstated"
}

func clipLine(s string, n int) string {
	s = strings.TrimSpace(strings.ReplaceAll(s, "\n", " "))
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
