package agent

import (
	"encoding/json"
	"testing"

	"github.com/Rishi0507/manhattan/internal/llm"
)

// TestEverySchemaConvertsForGemini runs every schema this package actually
// sends through the Gemini conversion and checks the result is something the
// API will accept.
//
// The live smoke test proves one role works. It proves nothing about the other
// seven, and the ones it does not cover are the complicated ones: the
// controller's action schema and the close schema carry nested arrays of
// objects and closed enums, which is exactly where an OpenAPI subset is fussy.
// Discovering that during a batch costs a quota and an afternoon; discovering
// it here costs nothing, because the conversion is pure and needs no network.
//
// What this cannot check is whether Google accepts the result. It checks the
// two properties that have actually broken: keywords Gemini rejects surviving
// the conversion, and constraints that carry meaning being dropped.
func TestEverySchemaConvertsForGemini(t *testing.T) {
	schemas := map[string]map[string]any{
		"close":     closeSchema(),
		"step":      stepSchema(),
		"action":    actionSchema(),
		"diagnose":  diagnoseSchema(),
		"narration": narrationSchema(),
		"answer":    answerSchema(),
		"draft":     draftSchema(),
	}

	for name, in := range schemas {
		if in == nil {
			t.Errorf("%s: schema is nil", name)
			continue
		}
		enumsBefore := countKey(in, "enum")
		reqBefore := countKey(in, "required")

		out := llm.GeminiSchemaForTest(in)

		if n := countKey(out, "additionalProperties"); n != 0 {
			t.Errorf("%s: %d additionalProperties survived; Gemini rejects the whole request", name, n)
		}
		for _, banned := range []string{"$schema", "title", "default"} {
			if n := countKey(out, banned); n != 0 {
				t.Errorf("%s: %d %q survived the conversion", name, n, banned)
			}
		}
		if got := countKey(out, "enum"); got != enumsBefore {
			t.Errorf("%s: enum count went from %d to %d. Dropping an enum turns a closed "+
				"vocabulary into free text, which is the property the corroboration tests rely on",
				name, enumsBefore, got)
		}
		if got := countKey(out, "required"); got != reqBefore {
			t.Errorf("%s: required count went from %d to %d, so fields the pipeline reads "+
				"could come back absent", name, reqBefore, got)
		}
		if n := countLowerType(out); n != 0 {
			t.Errorf("%s: %d lower-case type names survived; Gemini's subset wants them upper", name, n)
		}
		if _, err := json.Marshal(out); err != nil {
			t.Errorf("%s: converted schema does not marshal: %v", name, err)
		}
	}
}

func countKey(m map[string]any, key string) int {
	n := 0
	if _, ok := m[key]; ok {
		n++
	}
	for k, v := range m {
		if k == key {
			continue
		}
		switch t := v.(type) {
		case map[string]any:
			n += countKey(t, key)
		case []any:
			for _, e := range t {
				if em, ok := e.(map[string]any); ok {
					n += countKey(em, key)
				}
			}
		}
	}
	return n
}

func countLowerType(m map[string]any) int {
	n := 0
	if s, ok := m["type"].(string); ok {
		for _, r := range s {
			if r >= 'a' && r <= 'z' {
				n++
				break
			}
		}
	}
	for k, v := range m {
		if k == "type" {
			continue
		}
		switch t := v.(type) {
		case map[string]any:
			n += countLowerType(t)
		case []any:
			for _, e := range t {
				if em, ok := e.(map[string]any); ok {
					n += countLowerType(em)
				}
			}
		}
	}
	return n
}
