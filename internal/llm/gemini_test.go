package llm

import (
	"encoding/json"
	"testing"
)

// The schema converter is the one part of the Gemini path that can fail
// silently.
//
// Everything else in that file either works or returns an error a human reads.
// A schema that Gemini rejects, or worse accepts while ignoring a constraint,
// produces answers that parse and are wrong, and the failure surfaces hundreds
// of settlements later as a field that was never populated. So it is tested
// against the schemas this repository actually sends rather than against
// invented ones.

func TestGeminiSchemaDropsUnsupportedKeywords(t *testing.T) {
	// The shape every schema in this repository has: a closed object with
	// additionalProperties:false, which Gemini rejects outright rather than
	// ignoring.
	in := map[string]any{
		"$schema":              "http://json-schema.org/draft-07/schema#",
		"title":                "resolve",
		"type":                 "object",
		"additionalProperties": false,
		"required":             []any{"action"},
		"properties": map[string]any{
			"action": map[string]any{
				"type": "string",
				"enum": []any{"SEARCH_FEED", "NARROW_TO_HISTORY"},
			},
			"records": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type":                 "object",
					"additionalProperties": false,
					"default":              nil,
					"properties": map[string]any{
						"id":     map[string]any{"type": "string"},
						"amount": map[string]any{"type": "integer"},
					},
				},
			},
		},
	}

	out := geminiSchema(in)

	for _, banned := range []string{"$schema", "title", "additionalProperties", "default"} {
		if _, ok := out[banned]; ok {
			t.Errorf("%q survived the conversion at the top level; Gemini rejects it", banned)
		}
	}
	// The recursion is the part that actually breaks: stripping only the top
	// level leaves a nested additionalProperties that fails the whole call.
	items := out["properties"].(map[string]any)["records"].(map[string]any)["items"].(map[string]any)
	for _, banned := range []string{"additionalProperties", "default"} {
		if _, ok := items[banned]; ok {
			t.Errorf("%q survived inside an array's items; the converter is not recursing", banned)
		}
	}

	// Constraints that carry meaning must NOT be dropped. A converter that
	// strips enum turns a closed action vocabulary into free text, which is
	// exactly the property the corroboration tests rely on.
	act := out["properties"].(map[string]any)["action"].(map[string]any)
	if _, ok := act["enum"]; !ok {
		t.Error("enum was dropped; the action vocabulary would no longer be closed")
	}
	if _, ok := out["required"]; !ok {
		t.Error("required was dropped; fields the pipeline reads could come back absent")
	}
}

func TestGeminiSchemaUpperCasesTypes(t *testing.T) {
	// Gemini's responseSchema is an OpenAPI 3 subset and its type names are
	// upper case. Lower case is accepted by the JSON parser and rejected by the
	// API, so this is a whole-run failure rather than a degraded one.
	out := geminiSchema(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"n":  map[string]any{"type": "integer"},
			"xs": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
		},
	})
	if out["type"] != "OBJECT" {
		t.Errorf("top-level type is %v, want OBJECT", out["type"])
	}
	props := out["properties"].(map[string]any)
	if got := props["n"].(map[string]any)["type"]; got != "INTEGER" {
		t.Errorf("nested type is %v, want INTEGER", got)
	}
	xs := props["xs"].(map[string]any)
	if got := xs["items"].(map[string]any)["type"]; got != "STRING" {
		t.Errorf("array item type is %v, want STRING", got)
	}
}

func TestGeminiSchemaIsSerialisable(t *testing.T) {
	// A converted schema is embedded in the request body. If it does not
	// marshal, every call fails with an encoding error that names the role and
	// not the cause.
	out := geminiSchema(map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties":           map[string]any{"ok": map[string]any{"type": "boolean"}},
	})
	if _, err := json.Marshal(out); err != nil {
		t.Fatalf("the converted schema does not marshal: %v", err)
	}
	if geminiSchema(nil) != nil {
		t.Error("a nil schema must convert to nil, so responseSchema is omitted rather than sent empty")
	}
}

// TestGeminiModelsCoverEveryRole catches a role added to the pipeline and not
// to the Gemini model map.
//
// The fallback returns the expensive model, so the symptom of forgetting is a
// bill rather than an error.
func TestGeminiModelsCoverEveryRole(t *testing.T) {
	cfg := DefaultGeminiConfig()
	for _, r := range []Role{
		RoleParse, RoleResolve, RolePlan, RoleAnswer, RoleExplain,
		RoleTriage, RoleRemediate, RoleControl,
	} {
		m, ok := cfg.Models[r]
		if !ok {
			t.Errorf("role %q has no Gemini model; it would silently fall back to the expensive one", r)
			continue
		}
		if _, priced := Rates[m]; !priced {
			t.Errorf("role %q maps to %q, which has no entry in Rates, so its cost would report as zero", r, m)
		}
	}
}

// TestAnthropicModelsArePriced is the same check on the other vendor, because
// the failure it catches has already happened once: a model was pointed at a
// name that Rates did not carry and the run reported a cost of nothing.
func TestAnthropicModelsArePriced(t *testing.T) {
	for r, m := range DefaultAnthropicConfig().Models {
		if _, ok := Rates[m]; !ok {
			t.Errorf("role %q maps to %q, which has no entry in Rates", r, m)
		}
	}
}
