// Package taxmatch reconciles tax components across settlement fees.
package taxmatch

import (
	"context"
	"fmt"
	"math"

	"github.com/Rishi0507/manhattan/internal/evidence"
	"github.com/Rishi0507/manhattan/internal/llm"
)

// Analysis represents a tax reconciliation result.
type Analysis struct {
	TotalGSTComputed    int64       `json:"total_gst_computed"`
	TotalGSTDeclared    int64       `json:"total_gst_declared"`
	DiscrepancyPaise    int64       `json:"discrepancy_paise"`
	DiscrepancyINR      string      `json:"discrepancy_inr"`
	MatchRate           float64     `json:"match_rate"`
	Lines               []TaxLine   `json:"lines"`
	Anomalies           []Anomaly   `json:"anomalies"`
	ComplianceNotes     []string    `json:"compliance_notes"`
	Explanation         string      `json:"explanation"`
	Usage               llm.Usage   `json:"-"`
}

// TaxLine represents one line item's tax breakdown.
type TaxLine struct {
	SettlementRef  string  `json:"settlement_ref"`
	FeePaise       int64   `json:"fee_paise"`
	GSTRate        float64 `json:"gst_rate"`
	GSTComputed    int64   `json:"gst_computed"`
	GSTDeclared    int64   `json:"gst_declared"`
	DifferencePaise int64  `json:"difference_paise"`
	Status         string  `json:"status"` // "match", "mismatch", "rounding"
}

// Anomaly represents a tax discrepancy requiring attention.
type Anomaly struct {
	Type          string `json:"type"`
	SettlementRef string `json:"settlement_ref"`
	Description   string `json:"description"`
	ImpactINR     string `json:"impact_inr"`
	Severity      string `json:"severity"` // "low", "medium", "high"
}

// Matcher reconciles tax across settlements.
type Matcher struct {
	Provider llm.Provider
	Store    *evidence.Store
}

// New creates a tax matcher.
func New(p llm.Provider, s *evidence.Store) *Matcher {
	return &Matcher{Provider: p, Store: s}
}

const taxSystem = `You are a tax reconciliation specialist analyzing GST (Goods and Services Tax) 
components across payment settlement fees.

In India, payment gateway fees (MDR) are subject to 18% GST. Your job is to:
1. Verify GST is correctly applied to each fee component
2. Detect discrepancies between computed and declared GST
3. Identify rounding issues vs actual mismatches
4. Flag compliance concerns
5. Explain anomalies in plain language

RULES:
1. GST in India on payment services is 18% (as of standard rate)
2. Rounding differences of ±1 paise are normal (due to half-even vs half-up conventions)
3. Systematic over/under-collection is a compliance issue
4. UPI transactions may have zero MDR (per Indian regulation), thus zero GST
5. Chargebacks may have special GST treatment
6. Be specific: cite settlement IDs and amounts
7. Distinguish between technical rounding and actual tax errors

Format amounts in rupees. Flag compliance risks prominently.`

// Analyze performs tax reconciliation across all settlements.
func (m *Matcher) Analyze(ctx context.Context) (Analysis, error) {
	lines := m.buildTaxLines()
	
	// Compute totals and anomalies
	var totalComputed, totalDeclared int64
	var anomalies []Anomaly
	
	for _, line := range lines {
		totalComputed += line.GSTComputed
		totalDeclared += line.GSTDeclared
		
		// Flag significant mismatches
		if line.Status == "mismatch" && math.Abs(float64(line.DifferencePaise)) > 100 {
			anomalies = append(anomalies, Anomaly{
				Type:          "GST_MISMATCH",
				SettlementRef: line.SettlementRef,
				Description:   fmt.Sprintf("GST mismatch of %d paise", line.DifferencePaise),
				ImpactINR:     formatINR(line.DifferencePaise),
				Severity:      severity(line.DifferencePaise),
			})
		}
	}
	
	discrepancy := totalComputed - totalDeclared
	matchRate := 1.0
	if totalComputed > 0 {
		matchRate = float64(totalDeclared) / float64(totalComputed)
	}

	analysis := Analysis{
		TotalGSTComputed: totalComputed,
		TotalGSTDeclared: totalDeclared,
		DiscrepancyPaise: discrepancy,
		DiscrepancyINR:   formatINR(discrepancy),
		MatchRate:        matchRate,
		Lines:            lines,
		Anomalies:        anomalies,
	}

	// Get LLM explanation
	prompt := m.buildPrompt(analysis)
	
	res, err := m.Provider.Structured(ctx, llm.Request{
		Role:       llm.RoleTriage, // Reuse triage role for diagnosis
		System:     taxSystem,
		User:       prompt,
		SchemaName: "tax_analysis",
		SchemaDesc: "Explain tax discrepancies and provide compliance notes",
		Schema:     taxSchema(),
		MaxTokens:  1500,
	})
	if err != nil {
		return analysis, err
	}

	var result struct {
		Explanation     string   `json:"explanation"`
		ComplianceNotes []string `json:"compliance_notes"`
	}
	if err := res.Into(&result); err != nil {
		return analysis, err
	}

	analysis.Explanation = result.Explanation
	analysis.ComplianceNotes = result.ComplianceNotes
	analysis.Usage = res.Usage

	return analysis, nil
}

func (m *Matcher) buildTaxLines() []TaxLine {
	all := m.Store.All()
	var lines []TaxLine

	for _, r := range all {
		// Extract fee and GST info from receipt
		// In Manhattan, fees are embedded in contribution calculation
		feePaise := m.estimateFee(r)
		gstRate := 0.18 // Standard 18% GST
		
		// Compute expected GST
		gstComputed := int64(math.Round(float64(feePaise) * gstRate))
		
		// In real system, declared GST would come from gateway report
		// For demo, add slight variance
		gstDeclared := gstComputed
		if r.FeeCheck != nil && r.FeeCheck.DeltaBps != 0 {
			// Fee mismatch implies possible GST mismatch
			adjustment := float64(feePaise) * float64(r.FeeCheck.DeltaBps) / 10000.0
			gstDeclared = int64(math.Round((float64(feePaise) + adjustment) * gstRate))
		}

		diff := gstComputed - gstDeclared
		status := "match"
		if math.Abs(float64(diff)) > 1 {
			status = "mismatch"
		} else if diff != 0 {
			status = "rounding"
		}

		lines = append(lines, TaxLine{
			SettlementRef:  r.SettlementRef,
			FeePaise:       feePaise,
			GSTRate:        gstRate,
			GSTComputed:    gstComputed,
			GSTDeclared:    gstDeclared,
			DifferencePaise: diff,
			Status:         status,
		})
	}

	return lines
}

func (m *Matcher) estimateFee(r *evidence.Receipt) int64 {
	// Estimate fee from target amount and typical MDR
	// Real implementation would read from fee rows
	grossApprox := int64(r.TargetPaise) * 100 / 98 // Assume ~2% MDR
	feePaise := grossApprox - int64(r.TargetPaise)
	if feePaise < 0 {
		feePaise = 0
	}
	return feePaise
}

func (m *Matcher) buildPrompt(a Analysis) string {
	mismatchCount := 0
	for _, line := range a.Lines {
		if line.Status == "mismatch" {
			mismatchCount++
		}
	}

	return fmt.Sprintf(`TAX RECONCILIATION SUMMARY

Total GST computed: ₹%s (%d paise)
Total GST declared: ₹%s (%d paise)
Discrepancy: ₹%s
Match rate: %.2f%%

Settlements analyzed: %d
Mismatches found: %d

ANOMALIES:
%s

TASK: Explain the tax discrepancies and provide compliance notes.

Consider:
- Are differences due to rounding conventions?
- Is there systematic over/under-collection?
- Are there merchant-specific patterns?
- What compliance actions are needed?

Be specific and cite settlement IDs where relevant.`,
		formatINR(a.TotalGSTComputed),
		a.TotalGSTComputed,
		formatINR(a.TotalGSTDeclared),
		a.TotalGSTDeclared,
		a.DiscrepancyINR,
		a.MatchRate*100,
		len(a.Lines),
		mismatchCount,
		formatAnomalies(a.Anomalies),
	)
}

// AnalyzeOffline generates deterministic analysis without LLM.
func (m *Matcher) AnalyzeOffline() Analysis {
	lines := m.buildTaxLines()
	
	var totalComputed, totalDeclared int64
	var anomalies []Anomaly
	mismatchCount := 0
	
	for _, line := range lines {
		totalComputed += line.GSTComputed
		totalDeclared += line.GSTDeclared
		
		if line.Status == "mismatch" {
			mismatchCount++
			if math.Abs(float64(line.DifferencePaise)) > 100 {
				anomalies = append(anomalies, Anomaly{
					Type:          "GST_MISMATCH",
					SettlementRef: line.SettlementRef,
					Description:   fmt.Sprintf("GST mismatch of %d paise", line.DifferencePaise),
					ImpactINR:     formatINR(line.DifferencePaise),
					Severity:      severity(line.DifferencePaise),
				})
			}
		}
	}
	
	discrepancy := totalComputed - totalDeclared
	matchRate := 1.0
	if totalComputed > 0 {
		matchRate = float64(totalDeclared) / float64(totalComputed)
	}

	// Deterministic explanation
	explanation := fmt.Sprintf("Analyzed %d settlements for GST compliance. ", len(lines))
	if mismatchCount == 0 {
		explanation += "All GST calculations match within rounding tolerance (±1 paise). "
	} else {
		explanation += fmt.Sprintf("Found %d settlements with GST discrepancies. ", mismatchCount)
	}
	if math.Abs(float64(discrepancy)) > 1000 {
		explanation += fmt.Sprintf("Total discrepancy of ₹%s requires review. ", formatINR(discrepancy))
	}
	explanation += "GST rate assumed at 18% per standard Indian payment service regulations."

	complianceNotes := []string{
		"GST on MDR is 18% as per standard Indian tax regulations",
		"Rounding differences of ±1 paise per transaction are acceptable",
	}
	if mismatchCount > len(lines)/10 {
		complianceNotes = append(complianceNotes, "High mismatch rate (>10%) warrants systematic review")
	}
	if len(anomalies) > 0 {
		complianceNotes = append(complianceNotes, fmt.Sprintf("%d settlements flagged for detailed audit", len(anomalies)))
	}

	return Analysis{
		TotalGSTComputed: totalComputed,
		TotalGSTDeclared: totalDeclared,
		DiscrepancyPaise: discrepancy,
		DiscrepancyINR:   formatINR(discrepancy),
		MatchRate:        matchRate,
		Lines:            lines[:min(50, len(lines))], // Limit response size
		Anomalies:        anomalies,
		ComplianceNotes:  complianceNotes,
		Explanation:      explanation,
	}
}

func formatINR(paise int64) string {
	rupees := float64(paise) / 100.0
	if math.Abs(rupees) >= 100000 {
		return fmt.Sprintf("%.2f L", rupees/100000)
	}
	return fmt.Sprintf("%.2f", rupees)
}

func formatAnomalies(anomalies []Anomaly) string {
	if len(anomalies) == 0 {
		return "  (no significant anomalies detected)"
	}
	var lines []string
	for i, a := range anomalies {
		if i >= 5 {
			lines = append(lines, fmt.Sprintf("  ... and %d more", len(anomalies)-5))
			break
		}
		lines = append(lines, fmt.Sprintf("  - %s: %s (%s impact, %s severity)",
			a.SettlementRef, a.Description, a.ImpactINR, a.Severity))
	}
	return join(lines, "\n")
}

func severity(paise int64) string {
	abs := math.Abs(float64(paise))
	if abs < 100 {
		return "low"
	} else if abs < 1000 {
		return "medium"
	}
	return "high"
}

func join(strs []string, sep string) string {
	if len(strs) == 0 {
		return ""
	}
	result := strs[0]
	for i := 1; i < len(strs); i++ {
		result += sep + strs[i]
	}
	return result
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func taxSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"explanation": map[string]any{
				"type": "string",
				"description": "Plain language explanation of tax discrepancies and their causes",
			},
			"compliance_notes": map[string]any{
				"type": "array",
				"items": map[string]any{"type": "string"},
				"description": "Actionable compliance recommendations",
			},
		},
		"required": []string{"explanation", "compliance_notes"},
	}
}
