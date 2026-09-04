package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"
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

	// limit paces requests so a batch stays inside the key's quota.
	//
	// Without it a run does not merely fail, it fails destructively: a
	// sixty-settlement batch fired several hundred requests at a ten-per-minute
	// key, collected 666 rate-limit errors, and burned the whole day's
	// allowance producing nothing. Retrying a 429 immediately is not resilience,
	// it is the cause.
	limit *rate.Limiter
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

	// Model names are pinned rather than taken from the -latest aliases.
	//
	// An alias that silently moves under a benchmark makes every published
	// figure unattributable: the run says "gemini-pro-latest" and nobody can
	// say afterwards what actually answered. Pinning costs an occasional
	// update and buys a receipt that means something.
	//
	// Both are overridable, because the tier a key sits on decides what it can
	// reach. The pro models are not available on the free tier at all, so the
	// default points every role at flash and a paid key upgrades the expensive
	// roles with one variable rather than a code change.
	flash := envOr("GEMINI_FLASH_MODEL", "gemini-3.8-flash")
	pro := envOr("GEMINI_PRO_MODEL", flash)

	return GeminiConfig{
		APIKey: key,
		Models: map[Role]string{
			RoleParse:   flash,
			RoleResolve: pro,
			RolePlan:    pro,
			RoleAnswer:  pro,
			RoleExplain: flash,
			// Diagnosis and note drafting are the cheapest jobs at the highest
			// volume, so they take the smaller model for the same reason the
			// Anthropic path points them at Sonnet.
			RoleTriage:    flash,
			RoleRemediate: flash,
			// The close is one call per period over the whole run, so it is
			// the cheapest place to spend the best model.
			RoleControl: pro,
		},
	}
}

func envOr(name, fallback string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return fallback
}

// NewGemini builds a live Gemini provider.
func NewGemini(cfg GeminiConfig) Provider {
	if cfg.Models == nil {
		cfg.Models = DefaultGeminiConfig().Models
	}
	// Ten a minute is the free tier's published ceiling and therefore the safe
	// default. A paid key raises it with GEMINI_RPM rather than a code change.
	rpm := 10.0
	if v := os.Getenv("GEMINI_RPM"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f > 0 {
			rpm = f
		}
	}
	return &geminiProvider{
		apiKey: cfg.APIKey,
		models: cfg.Models,
		http:   &http.Client{Timeout: 120 * time.Second},
		limit:  rate.NewLimiter(rate.Limit(rpm/60.0), 1),
	}
}

func (p *geminiProvider) Name() string { return "gemini" }

func (p *geminiProvider) Model(r Role) string {
	if m, ok := p.models[r]; ok {
		return m
	}
	return DefaultGeminiConfig().Models[RoleResolve]
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
	ResponseMIMEType string `json:"responseMimeType"`
	// ThinkingConfig switches off the 3.x models' internal reasoning.
	//
	// Reasoning tokens are drawn from the same output budget as the answer, so
	// a thinking model handed a small budget spends it thinking and returns a
	// truncated object. Every job here is extraction or classification against
	// a fixed schema, which is not work that reasoning improves, and turning it
	// off restores both the budget and the determinism a zero temperature was
	// asked for.
	ThinkingConfig  *geminiThinking `json:"thinkingConfig,omitempty"`
	ResponseSchema  map[string]any  `json:"responseSchema,omitempty"`
	MaxOutputTokens int             `json:"maxOutputTokens,omitempty"`
	Temperature     float64         `json:"temperature"`
}

type geminiThinking struct {
	ThinkingBudget int `json:"thinkingBudget"`
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

// Structured makes one call whose answer is guaranteed to fit the schema,
// retrying the failures that are the network's fault rather than the caller's.
//
// A batch here is fifteen hundred calls. At that volume a shared endpoint will
// return 429 or 503 several times over, and without a retry each one lands as
// "the agent could not clear this exception", which is indistinguishable in the
// published numbers from a model that was not good enough. A measurement that
// silently absorbs infrastructure noise is not a measurement.
//
// Only transient statuses are retried. A 400 means the schema is wrong and
// retrying it just spends four seconds arriving at the same answer.
func (p *geminiProvider) Structured(ctx context.Context, req Request) (*Result, error) {
	var lastErr error
	backoff := 800 * time.Millisecond
	for attempt := 0; attempt < 5; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff):
			}
			backoff *= 2
		}
		res, err := p.structuredOnce(ctx, req)
		if err == nil {
			return res, nil
		}
		lastErr = err
		if !isTransient(err) {
			return nil, warn(err)
		}
		// A daily cap does not clear by waiting thirty seconds, so retrying it
		// only spends the next day's first requests on the same refusal.
		if strings.Contains(err.Error(), "PerDay") {
			return nil, warn(err)
		}
		// The server says how long to wait. Believing it beats guessing, and a
		// quota refusal needs far longer than a transient overload.
		if d := retryAfter(err); d > 0 {
			backoff = d
		} else if strings.Contains(err.Error(), "RESOURCE_EXHAUSTED") {
			backoff = 30 * time.Second
		}
	}
	return nil, warn(fmt.Errorf("llm: %s call failed after 5 attempts: %w", req.Role, lastErr))
}

// warn prints the first few provider failures to stderr.
//
// The batch treats a provider error as "the agent could not clear this
// exception", which is the right behaviour for a run and the wrong behaviour
// for a diagnosis: a completely broken integration produces the same receipts
// as a model that was not good enough, and the only visible symptom is a cost
// of zero. This has now cost two debugging sessions, so the errors are printed
// once each rather than inferred from a suspiciously round number.
//
// It is capped so a systematically failing run does not print fifteen hundred
// identical lines over the summary a reader is trying to read.
func warn(err error) error {
	warnMu.Lock()
	defer warnMu.Unlock()
	if warnCount < 5 {
		warnCount++
		fmt.Fprintf(os.Stderr, "llm warning: %v\n", err)
		if warnCount == 5 {
			fmt.Fprintln(os.Stderr, "llm warning: further provider errors suppressed")
		}
	}
	return err
}

var (
	warnMu    sync.Mutex
	warnCount int
)

// isTransient reports whether an error is worth trying again.
func isTransient(err error) bool {
	s := err.Error()
	for _, marker := range []string{
		"UNAVAILABLE", "RESOURCE_EXHAUSTED", "INTERNAL", "DEADLINE_EXCEEDED",
		"high demand", "overloaded", "HTTP 429", "HTTP 500", "HTTP 502",
		"HTTP 503", "HTTP 504", "connection reset", "EOF", "timeout",
	} {
		if strings.Contains(s, marker) {
			return true
		}
	}
	return false
}

func (p *geminiProvider) structuredOnce(ctx context.Context, req Request) (*Result, error) {
	if p.apiKey == "" {
		return nil, fmt.Errorf("llm: GEMINI_API_KEY is not set")
	}
	model := p.Model(req.Role)
	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 2048
	}
	// A floor, because a caller's tight budget that was fine for a
	// non-reasoning model truncates the object here and the failure reads as
	// malformed JSON rather than as a budget.
	if maxTokens < 1024 {
		maxTokens = 1024
	}

	body := geminiRequest{
		SystemInstruction: &geminiContent{Parts: []geminiPart{{Text: req.System}}},
		Contents: []geminiContent{{
			Role: "user", Parts: []geminiPart{{Text: req.User}},
		}},
		GenerationConfig: geminiGenConfig{
			ResponseMIMEType: "application/json",
			ResponseSchema:   geminiSchema(req.Schema),
			ThinkingConfig:   &geminiThinking{ThinkingBudget: 0},
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

	// Pace, rather than discover the pace by being refused.
	if p.limit != nil {
		if err := p.limit.Wait(ctx); err != nil {
			return nil, err
		}
	}

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

	text := extractJSON(out.Candidates[0].Content.Parts)
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

// extractJSON pulls the answer out of a candidate's parts.
//
// The response schema guarantees the model emits JSON, and the 3.x models
// still sometimes put a sentence in one part and the object in the next, or
// wrap the object in a markdown fence. Reading parts[0] and trusting it turned
// a working call into "does not parse as JSON", so this joins every part and
// then takes the first balanced object or array.
//
// This is not leniency about what the model may return. The result is still
// required to parse and still validated against the schema by the caller. It
// is only about where in the envelope the object was placed.
func extractJSON(parts []geminiPart) string {
	var b strings.Builder
	for _, p := range parts {
		b.WriteString(p.Text)
	}
	s := strings.TrimSpace(b.String())

	if i := strings.Index(s, "```"); i >= 0 {
		rest := s[i+3:]
		rest = strings.TrimPrefix(rest, "json")
		if j := strings.Index(rest, "```"); j >= 0 {
			s = strings.TrimSpace(rest[:j])
		}
	}
	if json.Valid([]byte(s)) {
		return s
	}

	// The first balanced object or array, ignoring braces inside strings.
	for i, r := range s {
		if r != '{' && r != '[' {
			continue
		}
		open, close := byte('{'), byte('}')
		if r == '[' {
			open, close = '[', ']'
		}
		depth, inStr, esc := 0, false, false
		for j := i; j < len(s); j++ {
			c := s[j]
			switch {
			case esc:
				esc = false
			case c == '\\' && inStr:
				esc = true
			case c == '"':
				inStr = !inStr
			case inStr:
			case c == open:
				depth++
			case c == close:
				depth--
				if depth == 0 {
					if cand := s[i : j+1]; json.Valid([]byte(cand)) {
						return cand
					}
					break
				}
			}
		}
	}
	return s
}

// retryAfter reads the retryDelay the API returns with a quota refusal.
func retryAfter(err error) time.Duration {
	m := retryDelayRe.FindStringSubmatch(err.Error())
	if len(m) != 2 {
		return 0
	}
	d, e := time.ParseDuration(m[1])
	if e != nil {
		return 0
	}
	return d
}

var retryDelayRe = regexp.MustCompile(`retryDelay"?:\s*"?([0-9.]+m?s)`)

// GeminiSchemaForTest exposes the conversion so other packages can check that
// the schemas they actually send survive it.
//
// The alternative is duplicating every schema into this package's tests, where
// the copy drifts from the original and the test then guards a schema nobody
// sends.
func GeminiSchemaForTest(in map[string]any) map[string]any { return geminiSchema(in) }
