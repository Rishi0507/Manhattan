package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// The Gemini provider.
//
// Manhattan's trust boundary is a single interface that speaks only in schemas,
// and the point of that design is that a second vendor costs one file. Nothing
// downstream knows which model answered: the verifier re-derives every posting
// from integer arithmetic regardless, and `manhattan live` asserts across
// providers that the wrong-posting count does not move.
//
// Structured output here uses Gemini's native JSON mode: responseMimeType set
// to application/json with a responseSchema, which is the same guarantee
// Anthropic's strict forced tool use gives. The model cannot return prose where
// an object is expected, so a malformed answer is a transport failure rather
// than something this pipeline has to interpret.
//
// The REST API is called directly rather than through a generated client. The
// surface used is one endpoint and four fields, a dependency would be larger
// than the code it replaced, and a reader can check this against Google's
// published request shape without leaving the file.
type geminiProvider struct {
	apiKey string
	models map[Role]string
	http   *http.Client
}

// GeminiConfig configures the Gemini path.
type GeminiConfig struct {
	APIKey string
	Models map[Role]string
}

// DefaultGeminiConfig reads the key from the environment.
//
// GEMINI_API_KEY is the name Google's own tooling uses; GOOGLE_API_KEY is
// accepted because half the ecosystem sets that instead and a submission that
// fails on the wrong variable name wastes somebody's evening.
func DefaultGeminiConfig() GeminiConfig {
	key := os.Getenv("GEMINI_API_KEY")
	if key == "" {
		key = os.Getenv("GOOGLE_API_KEY")
	}
	return GeminiConfig{
		APIKey: key,
		Models: map[Role]string{
			RoleParse:   "gemini-2.5-flash",
			RoleResolve: "gemini-2.5-pro",
			RolePlan:    "gemini-2.5-pro",
			RoleAnswer:  "gemini-2.5-pro",
			RoleExplain: "gemini-2.5-flash",
			// Diagnosis and note drafting are the cheapest jobs at the highest
			// volume, so they take the smaller model for the same reason the
			// Anthropic path points them at Sonnet.
			RoleTriage:    "gemini-2.5-flash",
			RoleRemediate: "gemini-2.5-flash",
			// The close is one call per period over the whole run, so it is
			// the cheapest place to spend the best model.
			RoleControl: "gemini-2.5-pro",
		},
	}
}

// NewGemini builds a live Gemini provider.
func NewGemini(cfg GeminiConfig) Provider {
	if cfg.Models == nil {
		cfg.Models = DefaultGeminiConfig().Models
	}
	return &geminiProvider{
		apiKey: cfg.APIKey,
		models: cfg.Models,
		http:   &http.Client{Timeout: 120 * time.Second},
	}
}

func (p *geminiProvider) Name() string { return "gemini" }

func (p *geminiProvider) Model(r Role) string {
	if m, ok := p.models[r]; ok {
		return m
	}
	return "gemini-2.5-pro"
}

// geminiRequest is the subset of the generateContent body this uses.
type geminiRequest struct {
	SystemInstruction *geminiContent  `json:"systemInstruction,omitempty"`
	Contents          []geminiContent `json:"contents"`
	GenerationConfig  geminiGenConfig `json:"generationConfig"`
	SafetySettings    []geminiSafety  `json:"safetySettings,omitempty"`
}

type geminiContent struct {
	Role  string       `json:"role,omitempty"`
	Parts []geminiPart `json:"parts"`
}

type geminiPart struct {
	Text string `json:"text"`
}

type geminiGenConfig struct {
	// ResponseMIMEType and ResponseSchema together are Gemini's structured
	// output mode, and they are what make this provider interchangeable with
	// the Anthropic one: the answer is guaranteed to parse into the shape the
	// caller declared.
	ResponseMIMEType string         `json:"responseMimeType"`
	ResponseSchema   map[string]any `json:"responseSchema,omitempty"`
	MaxOutputTokens  int            `json:"maxOutputTokens,omitempty"`
	Temperature      float64        `json:"temperature"`
}

type geminiSafety struct {
	Category  string `json:"category"`
	Threshold string `json:"threshold"`
}

type geminiResponse struct {
	Candidates []struct {
		Content      geminiContent `json:"content"`
		FinishReason string        `json:"finishReason"`
	} `json:"candidates"`
	UsageMetadata struct {
		PromptTokenCount        int `json:"promptTokenCount"`
		CandidatesTokenCount    int `json:"candidatesTokenCount"`
		CachedContentTokenCount int `json:"cachedContentTokenCount"`
	} `json:"usageMetadata"`
	Error *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Status  string `json:"status"`
	} `json:"error"`
}

// Structured makes one call whose answer is guaranteed to fit the schema.
func (p *geminiProvider) Structured(ctx context.Context, req Request) (*Result, error) {
	if p.apiKey == "" {
		return nil, fmt.Errorf("llm: GEMINI_API_KEY is not set")
	}
	model := p.Model(req.Role)
	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 2048
	}

	body := geminiRequest{
		SystemInstruction: &geminiContent{Parts: []geminiPart{{Text: req.System}}},
		Contents: []geminiContent{{
			Role: "user", Parts: []geminiPart{{Text: req.User}},
		}},
		GenerationConfig: geminiGenConfig{
			ResponseMIMEType: "application/json",
			ResponseSchema:   geminiSchema(req.Schema),
			MaxOutputTokens:  maxTokens,
			// Zero temperature, because a reconciliation that answers
			// differently on a re-run of the same settlement is not one an
			// auditor can check.
			Temperature: 0,
		},
	}

	raw, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("llm: encoding the %s request: %w", req.Role, err)
	}

	url := fmt.Sprintf(
		"https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent", model)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	// The key travels in a header rather than a query string, so it does not
	// end up in a proxy log.
	httpReq.Header.Set("x-goog-api-key", p.apiKey)

	resp, err := p.http.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("llm: %s call failed: %w", req.Role, err)
	}
	defer resp.Body.Close()

	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var out geminiResponse
	if err := json.Unmarshal(payload, &out); err != nil {
		return nil, fmt.Errorf("llm: %s returned something that is not JSON: %w", req.Role, err)
	}
	if out.Error != nil {
		return nil, fmt.Errorf("llm: %s call failed: %s (%s)", req.Role, out.Error.Message, out.Error.Status)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("llm: %s call failed with HTTP %d: %s",
			req.Role, resp.StatusCode, truncate(string(payload), 300))
	}
	if len(out.Candidates) == 0 || len(out.Candidates[0].Content.Parts) == 0 {
		return nil, fmt.Errorf("llm: %s returned no candidate", req.Role)
	}

	text := strings.TrimSpace(out.Candidates[0].Content.Parts[0].Text)
	if !json.Valid([]byte(text)) {
		return nil, fmt.Errorf(
			"llm: %s returned an answer that does not parse as JSON despite the response "+
				"schema, which is a transport failure rather than something to interpret: %s",
			req.Role, truncate(text, 200))
	}

	u := Usage{
		InputTokens:     out.UsageMetadata.PromptTokenCount,
		OutputTokens:    out.UsageMetadata.CandidatesTokenCount,
		CacheReadTokens: out.UsageMetadata.CachedContentTokenCount,
		Calls:           1,
		ByRole:          map[Role]int{req.Role: 1},
	}
	u.INRMicros = CostMicrosINR(model, u)

	return &Result{JSON: json.RawMessage(text), Model: model, Usage: u}, nil
}

// geminiSchema converts the caller's JSON Schema into the subset Gemini
// accepts.
//
// Gemini's responseSchema is an OpenAPI 3 subset: it has no additionalProperties
// and rejects unknown keywords outright rather than ignoring them. The strictness
// Anthropic gets from additionalProperties:false is instead achieved by the
// schema being closed by construction, since only declared properties are ever
// emitted.
func geminiSchema(in map[string]any) map[string]any {
	if in == nil {
		return nil
	}
	out := map[string]any{}
	for k, v := range in {
		switch k {
		case "additionalProperties", "$schema", "title", "default":
			// Not part of the accepted subset.
			continue
		case "properties":
			props, ok := v.(map[string]any)
			if !ok {
				continue
			}
			cleaned := map[string]any{}
			for name, spec := range props {
				if m, ok := spec.(map[string]any); ok {
					cleaned[name] = geminiSchema(m)
				} else {
					cleaned[name] = spec
				}
			}
			out[k] = cleaned
		case "items":
			if m, ok := v.(map[string]any); ok {
				out[k] = geminiSchema(m)
			} else {
				out[k] = v
			}
		case "type":
			// Gemini expects the type in upper case.
			if s, ok := v.(string); ok {
				out[k] = strings.ToUpper(s)
			} else {
				out[k] = v
			}
		default:
			out[k] = v
		}
	}
	return out
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
