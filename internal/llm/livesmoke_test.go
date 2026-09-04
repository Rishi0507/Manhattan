package llm

import (
	"context"
	"os"
	"testing"
)

// TestGeminiLiveSmoke makes one real call, and is skipped without a key.
//
// It exists because a provider error inside the batch pipeline is absorbed as
// "the agent could not clear this exception", which looks like a weak model
// rather than a broken integration. One call with the error printed is the
// difference between five minutes and an afternoon.
func TestGeminiLiveSmoke(t *testing.T) {
	// Opt-in, not merely key-gated.
	//
	// `go test ./...` runs on every change, and a free-tier key is rated for
	// ten requests a minute against a few hundred a day. A smoke test that
	// spends one of those every time the suite runs will have emptied the
	// quota before the run that needed it. So it wants an explicit
	// MANHATTAN_LIVE_SMOKE=1 as well as a key.
	loadEnvForTest()
	if os.Getenv("MANHATTAN_LIVE_SMOKE") != "1" {
		t.Skip("set MANHATTAN_LIVE_SMOKE=1 to spend a real request on this")
	}
	if os.Getenv("GEMINI_API_KEY") == "" {
		t.Skip("no GEMINI_API_KEY")
	}
	p := NewGemini(DefaultGeminiConfig())
	res, err := p.Structured(context.Background(), Request{
		Role:   RoleParse,
		System: "You extract fields. Answer only with the schema.",
		User:   "The settlement is for 1250 rupees and 50 paise.",
		Schema: map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"required":             []any{"paise"},
			"properties": map[string]any{
				"paise": map[string]any{"type": "integer"},
			},
		},
		MaxTokens: 256,
	})
	if err != nil {
		t.Fatalf("live Gemini call failed: %v", err)
	}
	t.Logf("answer=%s model=%s in=%d out=%d micros=%d",
		res.JSON, res.Model, res.Usage.InputTokens, res.Usage.OutputTokens, res.Usage.INRMicros)
	if res.Usage.InputTokens == 0 {
		t.Error("usage came back zero, so cost reporting would print nothing")
	}
}

func loadEnvForTest() {
	b, err := os.ReadFile("../../.env")
	if err != nil {
		return
	}
	for _, line := range splitLines(string(b)) {
		if len(line) == 0 || line[0] == '#' {
			continue
		}
		for i := 0; i < len(line); i++ {
			if line[i] == '=' {
				k, v := line[:i], line[i+1:]
				if os.Getenv(k) == "" {
					_ = os.Setenv(k, v)
				}
				break
			}
		}
	}
}

func splitLines(s string) []string {
	var out []string
	cur := ""
	for _, r := range s {
		if r == '\n' || r == '\r' {
			out = append(out, cur)
			cur = ""
			continue
		}
		cur += string(r)
	}
	return append(out, cur)
}
