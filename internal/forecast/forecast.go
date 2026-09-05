// Package forecast predicts forward cash flows from historical settlement patterns.
package forecast

import (
	"context"
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/Rishi0507/manhattan/internal/evidence"
	"github.com/Rishi0507/manhattan/internal/llm"
)

// Forecast represents predicted cash flows over a time horizon.
type Forecast struct {
	Generated   time.Time      `json:"generated"`
	Horizon     string         `json:"horizon"` // "7d" or "30d"
	Predictions []Prediction   `json:"predictions"`
	Confidence  string         `json:"confidence"`
	Assumptions []string       `json:"assumptions"`
	RiskFactors []string       `json:"risk_factors"`
	Usage       llm.Usage      `json:"-"`
	Analysis    string         `json:"analysis"`
}

// Prediction is one day's forecasted settlement activity.
type Prediction struct {
	Date                 string  `json:"date"`
	ExpectedSettlements  int     `json:"expected_settlements"`
	ExpectedAmountINR    int     `json:"expected_amount_inr"`
	LowBoundINR          int     `json:"low_bound_inr"`
	HighBoundINR         int     `json:"high_bound_inr"`
	MerchantBreakdown    map[string]int `json:"merchant_breakdown"`
}

// Forecaster builds cash flow predictions from settlement history.
type Forecaster struct {
	Provider llm.Provider
	Store    *evidence.Store
}

// New creates a forecaster over a receipt store.
func New(p llm.Provider, s *evidence.Store) *Forecaster {
	return &Forecaster{Provider: p, Store: s}
}

const forecastSystem = `You are a cash flow forecaster analyzing settlement reconciliation patterns.

You are given historical settlement data including:
- Daily settlement volumes and amounts
- Merchant type patterns (travel, marketplace, D2C, etc.)
- Settlement cycle characteristics (T+1, T+2, etc.)
- Verification rates and exception patterns
- Seasonal and day-of-week effects

Your job is to forecast expected cash inflows for the next 7 or 30 days.

RULES:
1. Base predictions on observed patterns in the historical data
2. Account for settlement cycles (T+2 means today's transactions settle in 2 days)
3. Consider merchant type mix and their typical settlement behaviors
4. Factor in verification rates (unverified settlements may delay cash)
5. Identify risk factors: high exception rates, narrowing sensitivity, feed issues
6. Provide confidence intervals, not just point estimates
7. State assumptions clearly (e.g., "assumes no change in merchant mix")
8. Be conservative: underestimate rather than overestimate

Format amounts in INR with Indian digit grouping (lakhs/crores).
Flag risks prominently - forecasting errors are costly.`

// Predict generates a cash flow forecast.
func (f *Forecaster) Predict(ctx context.Context, horizon string) (Forecast, error) {
	if horizon != "7d" && horizon != "30d" {
		horizon = "7d"
	}

	// Analyze historical patterns
	hist := f.analyzeHistory()
	
	// Build context for LLM
	prompt := f.buildPrompt(hist, horizon)

	res, err := f.Provider.Structured(ctx, llm.Request{
		Role:       llm.RoleAnswer, // Reuse existing role
		System:     forecastSystem,
		User:       prompt,
		SchemaName: "cash_forecast",
		SchemaDesc: "Generate a forward cash flow forecast with confidence intervals",
		Schema:     forecastSchema(),
		MaxTokens:  2000,
	})
	if err != nil {
		return Forecast{}, err
	}

	var fc Forecast
	if err := res.Into(&fc); err != nil {
		return Forecast{}, err
	}
	
	fc.Generated = time.Now()
	fc.Horizon = horizon
	fc.Usage = res.Usage
	
	return fc, nil
}

// Historical holds analyzed patterns from the receipt store.
type Historical struct {
	TotalSettlements    int
	TotalAmountINR      int64
	VerifiedRate        float64
	AvgDailySettlements float64
	AvgDailyAmountINR   int64
	MerchantTypes       map[string]int
	SettlementCycles    map[int]int // T+days -> count
	ExceptionRate       float64
	TopRiskFactors      []string
}

func (f *Forecaster) analyzeHistory() Historical {
	all := f.Store.All()
	if len(all) == 0 {
		return Historical{}
	}

	h := Historical{
		MerchantTypes:    make(map[string]int),
		SettlementCycles: make(map[int]int),
	}

	var totalAmount int64
	var verified int
	exceptions := 0
	
	dateSet := make(map[string]bool)
	risks := make(map[string]int)

	for _, r := range all {
		h.TotalSettlements++
		totalAmount += int64(r.TargetPaise)
		
		if r.Status == evidence.StatusVerified {
			verified++
		}
		if !r.Status.Postable() {
			exceptions++
		}

		// Track merchant types
		if r.Archetype != "" {
			h.MerchantTypes[r.Archetype]++
		}

		// Track dates
		if r.ValueDate != "" {
			dateSet[r.ValueDate] = true
		}

		// Infer settlement cycle (typically T+2 in India)
		h.SettlementCycles[2]++ // Default assumption

		// Collect risk factors
		for _, flag := range r.Flags {
			risks[string(flag)]++
		}
	}

	h.TotalAmountINR = totalAmount / 100 // Convert paise to rupees
	if h.TotalSettlements > 0 {
		h.VerifiedRate = float64(verified) / float64(h.TotalSettlements)
		h.ExceptionRate = float64(exceptions) / float64(h.TotalSettlements)
	}

	days := len(dateSet)
	if days == 0 {
		days = 1
	}
	h.AvgDailySettlements = float64(h.TotalSettlements) / float64(days)
	h.AvgDailyAmountINR = h.TotalAmountINR / int64(days)

	// Top 3 risk factors
	type kv struct {
		k string
		v int
	}
	var riskList []kv
	for k, v := range risks {
		riskList = append(riskList, kv{k, v})
	}
	sort.Slice(riskList, func(i, j int) bool { return riskList[i].v > riskList[j].v })
	for i := 0; i < len(riskList) && i < 3; i++ {
		h.TopRiskFactors = append(h.TopRiskFactors, riskList[i].k)
	}

	return h
}

func (f *Forecaster) buildPrompt(h Historical, horizon string) string {
	days := 7
	if horizon == "30d" {
		days = 30
	}

	return fmt.Sprintf(`HISTORICAL SETTLEMENT PATTERNS

Total settlements analyzed: %d
Total amount: ₹%s
Average daily settlements: %.1f
Average daily amount: ₹%s
Verification rate: %.1f%%
Exception rate: %.1f%%

Merchant type distribution:
%s

Settlement cycles observed:
%s

Top risk factors in recent history:
%s

TASK: Forecast expected cash inflows for the next %d days.

For each day, predict:
- Number of expected settlements
- Expected total amount (with confidence interval)
- Merchant type breakdown
- Key risk factors that could affect the forecast

Consider:
- Settlement cycles (T+2 means delays between transaction and settlement)
- Day-of-week effects (weekends may have different patterns)
- Verification rates (unverified settlements delay cash)
- Recent exception rates and their causes

Be conservative and state assumptions clearly.`,
		h.TotalSettlements,
		formatINR(h.TotalAmountINR),
		h.AvgDailySettlements,
		formatINR(h.AvgDailyAmountINR),
		h.VerifiedRate*100,
		h.ExceptionRate*100,
		formatMerchantTypes(h.MerchantTypes),
		formatCycles(h.SettlementCycles),
		formatRisks(h.TopRiskFactors),
		days,
	)
}

func formatINR(amount int64) string {
	if amount >= 10000000 {
		return fmt.Sprintf("%.2f Cr", float64(amount)/10000000)
	} else if amount >= 100000 {
		return fmt.Sprintf("%.2f L", float64(amount)/100000)
	}
	return fmt.Sprintf("%d", amount)
}

func formatMerchantTypes(types map[string]int) string {
	if len(types) == 0 {
		return "  (no merchant type data)"
	}
	var lines []string
	for k, v := range types {
		lines = append(lines, fmt.Sprintf("  %s: %d settlements", k, v))
	}
	sort.Strings(lines)
	return join(lines, "\n")
}

func formatCycles(cycles map[int]int) string {
	if len(cycles) == 0 {
		return "  T+2 (standard Indian gateway settlement)"
	}
	var lines []string
	for days, count := range cycles {
		lines = append(lines, fmt.Sprintf("  T+%d: %d settlements", days, count))
	}
	return join(lines, "\n")
}

func formatRisks(risks []string) string {
	if len(risks) == 0 {
		return "  (no significant risk flags)"
	}
	var lines []string
	for _, r := range risks {
		lines = append(lines, fmt.Sprintf("  - %s", r))
	}
	return join(lines, "\n")
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

// PredictOffline generates a deterministic forecast without LLM call.
// Used when ENABLE_LIVE_AI is false.
func (f *Forecaster) PredictOffline(horizon string) Forecast {
	h := f.analyzeHistory()
	days := 7
	if horizon == "30d" {
		days = 30
	}

	fc := Forecast{
		Generated:  time.Now(),
		Horizon:    horizon,
		Confidence: "Medium (based on deterministic extrapolation)",
		Assumptions: []string{
			"Settlement patterns continue at historical average",
			"No major merchant churn or acquisition",
			"T+2 settlement cycle remains standard",
			fmt.Sprintf("Verification rate stays near %.0f%%", h.VerifiedRate*100),
		},
		Analysis: fmt.Sprintf("Historical analysis of %d settlements shows average daily inflow of ₹%s. "+
			"Forecasting forward based on merchant mix and observed patterns.",
			h.TotalSettlements, formatINR(h.AvgDailyAmountINR)),
	}

	// Risk factors
	if h.ExceptionRate > 0.20 {
		fc.RiskFactors = append(fc.RiskFactors, fmt.Sprintf("High exception rate (%.0f%%) may delay settlements", h.ExceptionRate*100))
	}
	if len(h.TopRiskFactors) > 0 {
		fc.RiskFactors = append(fc.RiskFactors, "Recent flags: "+join(h.TopRiskFactors, ", "))
	}

	// Generate daily predictions
	today := time.Now()
	for i := 0; i < days; i++ {
		date := today.AddDate(0, 0, i+1)
		
		// Simple extrapolation with slight variance
		variance := 0.15
		expectedSettlements := int(h.AvgDailySettlements)
		expectedAmount := int(h.AvgDailyAmountINR)
		
		// Weekend adjustment (lower volume)
		if date.Weekday() == time.Saturday || date.Weekday() == time.Sunday {
			expectedSettlements = int(float64(expectedSettlements) * 0.7)
			expectedAmount = int(float64(expectedAmount) * 0.7)
		}

		pred := Prediction{
			Date:                date.Format("2006-01-02"),
			ExpectedSettlements: expectedSettlements,
			ExpectedAmountINR:   expectedAmount,
			LowBoundINR:         int(float64(expectedAmount) * (1 - variance)),
			HighBoundINR:        int(float64(expectedAmount) * (1 + variance)),
			MerchantBreakdown:   make(map[string]int),
		}

		// Distribute by merchant type
		for mtype, count := range h.MerchantTypes {
			share := float64(count) / float64(h.TotalSettlements)
			pred.MerchantBreakdown[mtype] = int(math.Round(float64(expectedAmount) * share))
		}

		fc.Predictions = append(fc.Predictions, pred)
	}

	return fc
}

func forecastSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"predictions": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"date":                  map[string]any{"type": "string"},
						"expected_settlements":  map[string]any{"type": "integer"},
						"expected_amount_inr":   map[string]any{"type": "integer"},
						"low_bound_inr":         map[string]any{"type": "integer"},
						"high_bound_inr":        map[string]any{"type": "integer"},
						"merchant_breakdown":    map[string]any{"type": "object"},
					},
					"required": []string{"date", "expected_settlements", "expected_amount_inr"},
				},
			},
			"confidence": map[string]any{"type": "string"},
			"assumptions": map[string]any{
				"type": "array",
				"items": map[string]any{"type": "string"},
			},
			"risk_factors": map[string]any{
				"type": "array",
				"items": map[string]any{"type": "string"},
			},
			"analysis": map[string]any{"type": "string"},
		},
		"required": []string{"predictions", "confidence", "analysis"},
	}
}
