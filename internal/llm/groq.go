package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// The Groq provider.
//
// Groq is an LPU inference engine with extremely fast token generation
// (500+ tokens/sec vs typical 50-100). Free tier provides 30 requests/min
// and 14,400 requests/day, which is sufficient for benchmark runs without
// hitting quota issues that plague Gemini's free tier.
//
// Uses OpenAI-compatible API with JSON mode for structured outputs.
type groqProvider struct {
	apiKey string
	models map[Role]string
	http   *http.Client
	limit  *rate.Limiter

	// loose records the roles whose schema strict mode refused, so the
	// fallback is paid for once rather than on every call.
	mu    sync.Mutex
	loose map[Role]bool
}

// GroqConfig configures the Groq path.
type GroqConfig struct {
	APIKey string
	Models map[Role]string
}

// DefaultGroqConfig reads the key from the environment.
func DefaultGroqConfig() GroqConfig {
	key := os.Getenv("GROQ_API_KEY")

	// openai/gpt-oss-120b, because it is one of the models on this endpoint
	// that honours a strict json_schema, which is the whole reason a provider
	// is allowed to sit behind this interface.
	//
	// The previous default, llama-3.3-70b-versatile, is not served to this key
	// at all. It never announced that: the batch treats a provider error as an
	// exception the agent could not clear, so an unreachable model and a weak
	// one produce the same receipts.
	// Two models, split by job, and not only for quality.
	//
	// Each model carries its own daily token allowance on this endpoint, so
	// routing the high-volume extraction work and the low-volume reasoning work
	// to different models spends two budgets instead of one. The larger model
	// took the whole batch on an earlier run and hit its 200,000 token daily
	// cap partway through, which stops a benchmark dead.
	//
	// The split also happens to be the right one on merit. Parsing a narration
	// and drafting a note are extraction against a fixed schema and run on
	// every settlement; choosing an action, resolving an exception and writing
	// the period close are judgement calls that run on a fraction of them.
	light := envOr("GROQ_MODEL", "openai/gpt-oss-20b")
	heavy := envOr("GROQ_MODEL_HEAVY", "qwen/qwen3.8-27b")

	return GroqConfig{
		APIKey: key,
		Models: map[Role]string{
			RoleParse:     light,
			RoleExplain:   light,
			RoleTriage:    light,
			RoleRemediate: light,
			RoleResolve:   heavy,
			RolePlan:      heavy,
			RoleAnswer:    heavy,
			RoleControl:   heavy,
		},
	}
}

// NewGroq builds a live Groq provider.
func NewGroq(cfg GroqConfig) Provider {
	if cfg.Models == nil {
		cfg.Models = DefaultGroqConfig().Models
	}
	// Requests are not the scarce thing here.
	//
	// This endpoint serves 1000 requests a minute and 8000 TOKENS a minute, and
	// a call in this pipeline costs around a thousand tokens. The token bucket
	// therefore empties about a hundred times sooner than the request bucket,
	// so pacing on requests paces on the wrong quantity: seven or eight calls a
	// minute is what actually fits. GROQ_RPM raises it on a paid tier.
	rpm := 7.0
	if v := os.Getenv("GROQ_RPM"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f > 0 {
			rpm = f
		}
	}
	return &groqProvider{
		apiKey: cfg.APIKey,
		models: cfg.Models,
		http:   &http.Client{Timeout: 60 * time.Second},
		limit:  rate.NewLimiter(rate.Limit(rpm/60.0), 1),
	}
}

func (p *groqProvider) Name() string { return "groq" }

func (p *groqProvider) Model(r Role) string {
	if m, ok := p.models[r]; ok {
		return m
	}
	return envOr("GROQ_MODEL", "openai/gpt-oss-20b")
}

// groqRequest follows OpenAI-compatible format
type groqRequest struct {
	Model          string        `json:"model"`
	Messages       []groqMessage `json:"messages"`
	Temperature    float64       `json:"temperature"`
	MaxTokens      int           `json:"max_tokens,omitempty"`
	ResponseFormat *groqFormat   `json:"response_format,omitempty"`
	// ReasoningEffort is what the gpt-oss models call their thinking budget.
	//
	// Left at its default they spend most of an answer's token allowance
	// reasoning and the object arrives truncated, which the API reports as a
	// schema validation failure. Every job here is extraction or
	// classification against a fixed schema, which is not work that
	// deliberation improves, and the saved tokens matter directly because
	// this endpoint rations tokens a hundred times harder than requests.
	ReasoningEffort string `json:"reasoning_effort,omitempty"`
}

type groqMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type groqFormat struct {
	Type       string          `json:"type"`
	JSONSchema *groqJSONSchema `json:"json_schema,omitempty"`
}

type groqJSONSchema struct {
	Name   string         `json:"name"`
	Schema map[string]any `json:"schema"`
	Strict bool           `json:"strict"`
}

type groqResponse struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	Model   string `json:"model"`
	Choices []struct {
		Index        int         `json:"index"`
		Message      groqMessage `json:"message"`
		FinishReason string      `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    string `json:"code"`
	} `json:"error"`
}

func (p *groqProvider) Structured(ctx context.Context, req Request) (*Result, error) {
	var lastErr error
	backoff := 500 * time.Millisecond
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
			RecordRetry(attempt)
			return res, nil
		}
		lastErr = err
		// A schema strict mode will not accept is not a transport failure and
		// not the caller's to retry blindly. The role drops to json_object and
		// the same request is tried again immediately.
		if isSchemaRefusal(err) && !p.looseRole(req.Role) {
			p.markLoose(req.Role, err.Error())
			backoff = 0
			continue
		}
		if !isTransient(err) {
			RecordFailure(req.Role, attempt+1, isSchemaRefusal(err))
			return nil, warn(err)
		}
		if strings.Contains(err.Error(), "rate_limit") {
			backoff = 2 * time.Second
		}
	}
	RecordFailure(req.Role, 5, isSchemaRefusal(lastErr))
	return nil, warn(fmt.Errorf("llm: %s call failed after 5 attempts: %w", req.Role, lastErr))
}

func (p *groqProvider) structuredOnce(ctx context.Context, req Request) (*Result, error) {
	if p.apiKey == "" {
		return nil, fmt.Errorf("llm: GROQ_API_KEY is not set")
	}

	model := p.Model(req.Role)
	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 2048
	}
	// Clamp the output budget to what the tier will accept.
	//
	// This endpoint rates output tokens per minute, and it does not merely
	// throttle a request that asks for more than the whole minute's allowance,
	// it refuses it: "Request too large ... Limit 1000, Requested 2048". So a
	// caller's perfectly reasonable 2048 was rejected before the model ran, on
	// every single call to a model whose limit is 1000, and the batch recorded
	// each one as an exception the agent could not clear. Nothing in the output
	// suggested the requests were never attempted.
	//
	// The ceiling is per tier rather than per model, so it is configuration
	// rather than a table that would go stale.
	if maxTokens > groqMaxOutput {
		maxTokens = groqMaxOutput
	}

	messages := []groqMessage{
		{Role: "system", Content: req.System + "\n\nRespond with valid JSON only."},
		{Role: "user", Content: req.User},
	}

	body := groqRequest{
		Model:       model,
		Messages:    messages,
		Temperature: 0,
		MaxTokens:   maxTokens,
	}
	if strings.Contains(model, "gpt-oss") {
		body.ReasoningEffort = envOr("GROQ_REASONING", "low")
	}

	// Structured output, forced by the schema rather than asked for in prose.
	//
	// json_object mode only promises the answer is SOME valid JSON. This
	// pipeline's claim is stronger: the model cannot return a shape the caller
	// did not declare, which is what makes a malformed answer a transport
	// failure rather than something to interpret. That needs json_schema with
	// strict set, and the models this defaults to support it.
	//
	// Strict mode additionally requires every declared property to be
	// required, so a schema carrying optional fields is rejected outright. When
	// that happens the role falls back to json_object once and remembers, which
	// is weaker and is reported rather than hidden.
	if req.Schema != nil {
		if p.looseRole(req.Role) {
			body.ResponseFormat = &groqFormat{Type: "json_object"}
		} else {
			body.ResponseFormat = &groqFormat{
				Type: "json_schema",
				JSONSchema: &groqJSONSchema{
					Name:   schemaName(req),
					Strict: true,
					Schema: strictSchema(req.Schema),
				},
			}
		}
	}

	raw, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("llm: encoding the %s request: %w", req.Role, err)
	}

	url := "https://api.groq.com/openai/v1/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)

	// Rate limiting
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

	var out groqResponse
	if err := json.Unmarshal(payload, &out); err != nil {
		return nil, fmt.Errorf("llm: %s returned something that is not JSON: %w", req.Role, err)
	}

	if out.Error != nil {
		return nil, fmt.Errorf("llm: %s call failed: %s (%s)", req.Role, out.Error.Message, out.Error.Type)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("llm: %s call failed with HTTP %d: %s",
			req.Role, resp.StatusCode, truncate(string(payload), 300))
	}

	if len(out.Choices) == 0 {
		return nil, fmt.Errorf("llm: %s returned no choices", req.Role)
	}

	text := strings.TrimSpace(out.Choices[0].Message.Content)
	if !json.Valid([]byte(text)) {
		return nil, fmt.Errorf("llm: %s returned invalid JSON: %s", req.Role, truncate(text, 200))
	}

	u := Usage{
		InputTokens:  out.Usage.PromptTokens,
		OutputTokens: out.Usage.CompletionTokens,
		Calls:        1,
		ByRole:       map[Role]int{req.Role: 1},
	}
	u.INRMicros = CostMicrosINR(model, u)

	return &Result{JSON: json.RawMessage(text), Model: model, Usage: u}, nil
}

// looseRole reports whether this role has already been refused by strict mode.
func (p *groqProvider) looseRole(r Role) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.loose[r]
}

// markLoose downgrades one role to json_object, once, and says so.
func (p *groqProvider) markLoose(r Role, because string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.loose == nil {
		p.loose = map[Role]bool{}
	}
	if !p.loose[r] {
		p.loose[r] = true
		fmt.Fprintf(os.Stderr,
			"llm: %s falls back to json_object because strict schema was refused: %s\n",
			r, truncate(because, 160))
	}
}

// schemaName gives the schema the name the API wants.
func schemaName(req Request) string {
	if req.SchemaName != "" {
		return req.SchemaName
	}
	return string(req.Role)
}

// isSchemaRefusal reports whether the API rejected the SCHEMA, as opposed to
// the model having failed to satisfy it.
//
// The distinction matters and an earlier version got it wrong by matching the
// word "schema" anywhere in the error. Two very different failures both
// mention it:
//
//	"invalid JSON schema for response_format ... additionalProperties:false
//	 must be set on every object"     the schema is unusable, downgrade
//	"Generated JSON does not match the expected schema"
//	                                  the schema is fine, the model missed
//
// Treating the second as a refusal dropped the role out of strict mode for the
// rest of the run, which quietly removed the very guarantee the strict mode is
// there to provide, and did it in response to a single bad generation. Only
// the first kind downgrades; the second is retried under the same schema.
func isSchemaRefusal(err error) bool {
	s := strings.ToLower(err.Error())
	for _, modelMissed := range []string{
		"generated json does not match",
		"failed to generate json",
		"failed to validate json",
	} {
		if strings.Contains(s, modelMissed) {
			return false
		}
	}
	for _, unusable := range []string{
		"invalid json schema",
		"must be set on every object",
		"is required to be supplied",
		"response_format",
		"not supported",
	} {
		if strings.Contains(s, unusable) {
			return true
		}
	}
	return false
}

// strictSchema rewrites a schema into the closed form strict mode demands.
//
// The endpoint refuses a strict json_schema unless EVERY object in it, not
// only the root, sets additionalProperties:false and lists every one of its
// properties as required. The schemas here set both at the top level and rely
// on the reader for nested objects, which is fine for a human and is rejected
// here.
//
// Making every property required has a consequence that has to be handled or
// it breaks the roles it was meant to strengthen. A field the schema treated
// as optional must now be emitted even when it does not apply, and a model
// asked for a number it has no value for correctly emits null, which the
// declared type then rejects:
//
//	'/window_hours' does not validate: expected number, but got null
//
// That failed every planning call that did not happen to involve a window,
// and the batch recorded each one as an exception the agent could not clear.
// So a property that was NOT originally required has null added to its type.
// It is required to be present and permitted to be empty, which is what
// "optional" meant in the first place. Fields that were already required keep
// their exact type and cannot be null.
func strictSchema(in map[string]any) map[string]any {
	if in == nil {
		return nil
	}

	// The schemas in this repository write "required" as a []string and this
	// once only handled []any, so the list read as empty and every field,
	// including the genuinely required ones, was marked optional. Both shapes
	// are accepted now, because a Go literal may reasonably be either.
	wasRequired := map[string]bool{}
	switch req := in["required"].(type) {
	case []any:
		for _, r := range req {
			if name, ok := r.(string); ok {
				wasRequired[name] = true
			}
		}
	case []string:
		for _, name := range req {
			wasRequired[name] = true
		}
	}

	out := make(map[string]any, len(in)+2)
	for k, v := range in {
		switch k {
		case "properties":
			props, ok := v.(map[string]any)
			if !ok {
				out[k] = v
				continue
			}
			cleaned := make(map[string]any, len(props))
			for name, spec := range props {
				m, ok := spec.(map[string]any)
				if !ok {
					cleaned[name] = spec
					continue
				}
				sub := strictSchema(m)
				if !wasRequired[name] {
					sub = nullable(sub)
				}
				cleaned[name] = sub
			}
			out[k] = cleaned
		case "items":
			if m, ok := v.(map[string]any); ok {
				out[k] = strictSchema(m)
			} else {
				out[k] = v
			}
		default:
			out[k] = v
		}
	}

	if t, _ := out["type"].(string); t == "object" {
		out["additionalProperties"] = false
		if props, ok := out["properties"].(map[string]any); ok {
			names := make([]string, 0, len(props))
			for name := range props {
				names = append(names, name)
			}
			// Sorted, so the request body for a given schema is stable between
			// runs and a cassette keyed on it still matches.
			sort.Strings(names)
			req := make([]any, len(names))
			for i, n := range names {
				req[i] = n
			}
			out["required"] = req
		}
	}
	return out
}

// nullable widens a property's type so it may be present and empty.
//
// Strict mode requires every property to be listed as required, so a field the
// schema treated as optional must now be emitted even where it does not apply.
// A model asked for a value it does not have correctly answers null, and the
// declared type then rejects the whole response.
//
// An enum needs the same treatment and an earlier version skipped it, on the
// reasoning that adding null to a closed vocabulary would let the model answer
// "none of these". That was the wrong call. The vocabulary stays closed for
// every answer that IS one: null is not a member of it, it is the absence of a
// member, which is precisely what the field being optional already meant. And
// skipping it meant an optional enum could not be filled in at all, so every
// planning call that did not involve that field failed outright, which is a
// far worse outcome than the one the exclusion was guarding against.
//
// JSON Schema validates enum membership as well as type, so null has to be
// added to both or the widened type is not enough on its own.
func nullable(spec map[string]any) map[string]any {
	switch t := spec["type"].(type) {
	case string:
		if t == "null" {
			return spec
		}
		spec["type"] = []any{t, "null"}
	case []any:
		for _, e := range t {
			if s, _ := e.(string); s == "null" {
				return spec
			}
		}
		spec["type"] = append(t, "null")
	}

	switch vals := spec["enum"].(type) {
	case []any:
		for _, v := range vals {
			if v == nil {
				return spec
			}
		}
		spec["enum"] = append(vals, nil)
	case []string:
		widened := make([]any, len(vals), len(vals)+1)
		for i, v := range vals {
			widened[i] = v
		}
		spec["enum"] = append(widened, nil)
	}
	return spec
}

// groqMaxOutput is the largest output budget a request may ask for.
//
// 900 leaves headroom under the free tier's 1000 output tokens per minute. A
// paid tier raises the limit and GROQ_MAX_OUTPUT raises this with it.
var groqMaxOutput = func() int {
	if v := os.Getenv("GROQ_MAX_OUTPUT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return 900
}()
