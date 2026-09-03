package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

// AnthropicConfig selects the model per role.
//
// Every role defaults to Claude Opus 5. Pointing the high-volume parse role
// at a smaller model is a legitimate cost decision and the field is here so
// it can be made deliberately, but it is not made silently on the operator's
// behalf: a settlement narration that is misread produces a pool for the
// wrong merchant, and that is not a place to economise by default.
type AnthropicConfig struct {
	APIKey string
	Models map[Role]string
}

// DefaultAnthropicConfig reads the key from the environment.
func DefaultAnthropicConfig() AnthropicConfig {
	return AnthropicConfig{
		APIKey: os.Getenv("ANTHROPIC_API_KEY"),
		Models: map[Role]string{
			RoleParse:   "claude-opus-5",
			RoleResolve: "claude-opus-5",
			RolePlan:    "claude-opus-5",
			RoleAnswer:  "claude-opus-5",
			RoleExplain: "claude-opus-5",
			// The two newest roles are the cheapest jobs in the system and
			// the highest volume of them, so they are pointed at a smaller
			// model by default. Diagnosis picks from five classes given the
			// arithmetic; drafting writes three sentences from supplied facts.
			// Neither needs frontier reasoning, and paying for it on the
			// highest-volume call would be the same mistake as paying a model
			// to add up a column.
			RoleTriage:    "claude-sonnet-5",
			RoleRemediate: "claude-sonnet-5",
		},
	}
}

type anthropicProvider struct {
	client anthropic.Client
	cfg    AnthropicConfig
}

// NewAnthropic builds a live provider. It does not require ANTHROPIC_API_KEY
// to be set: the SDK also resolves an OAuth profile written by `ant auth
// login`, so an empty key here is not by itself an error.
func NewAnthropic(cfg AnthropicConfig) Provider {
	var opts []option.RequestOption
	if cfg.APIKey != "" {
		opts = append(opts, option.WithAPIKey(cfg.APIKey))
	}
	if cfg.Models == nil {
		cfg.Models = DefaultAnthropicConfig().Models
	}
	return &anthropicProvider{client: anthropic.NewClient(opts...), cfg: cfg}
}

func (p *anthropicProvider) Name() string { return "anthropic" }

func (p *anthropicProvider) Model(r Role) string {
	if m, ok := p.cfg.Models[r]; ok && m != "" {
		return m
	}
	return "claude-opus-5"
}

// Structured makes one call whose answer is guaranteed to fit the schema.
//
// The mechanism is strict tool use with a forced tool choice: the schema is
// declared as the only tool, additionalProperties is closed, every field is
// required, and the model is required to call it. The API validates the
// arguments before they are returned, so a malformed answer surfaces as a
// transport failure rather than as something this pipeline has to guess at.
//
// The system block is cached. It is byte-identical for every settlement in a
// run, and the volatile part of the request lives entirely after the cache
// breakpoint, so a five-hundred-settlement batch pays for the instructions
// once rather than five hundred times.
func (p *anthropicProvider) Structured(ctx context.Context, req Request) (*Result, error) {
	model := p.Model(req.Role)
	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 2048
	}

	schema := anthropic.ToolInputSchemaParam{
		Properties: req.Schema["properties"],
	}
	if reqd, ok := req.Schema["required"]; ok {
		schema.Required = toStrings(reqd)
	}
	// Closing the object is what makes strict mode meaningful: without it the
	// model may add fields that no consumer reads and no validator rejects.
	schema.ExtraFields = map[string]any{"additionalProperties": false}

	tool := anthropic.ToolParam{
		Name:        req.SchemaName,
		Description: anthropic.String(req.SchemaDesc),
		InputSchema: schema,
		Strict:      anthropic.Bool(true),
	}

	params := anthropic.MessageNewParams{
		Model:     anthropic.Model(model),
		MaxTokens: int64(maxTokens),
		System: []anthropic.TextBlockParam{{
			Text:         req.System,
			CacheControl: anthropic.NewCacheControlEphemeralParam(),
		}},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(req.User)),
		},
		Tools: []anthropic.ToolUnionParam{{OfTool: &tool}},
		ToolChoice: anthropic.ToolChoiceUnionParam{
			OfTool: &anthropic.ToolChoiceToolParam{Name: req.SchemaName},
		},
	}

	resp, err := p.client.Messages.New(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("llm: %s call failed: %w", req.Role, err)
	}

	u := Usage{
		InputTokens:      int(resp.Usage.InputTokens),
		OutputTokens:     int(resp.Usage.OutputTokens),
		CacheReadTokens:  int(resp.Usage.CacheReadInputTokens),
		CacheWriteTokens: int(resp.Usage.CacheCreationInputTokens),
		Calls:            1,
		ByRole:           map[Role]int{req.Role: 1},
	}
	u.INRMicros = CostMicrosINR(model, u)

	for _, block := range resp.Content {
		if tu, ok := block.AsAny().(anthropic.ToolUseBlock); ok {
			return &Result{JSON: json.RawMessage(tu.Input), Usage: u, Model: model}, nil
		}
	}
	return nil, fmt.Errorf(
		"llm: %s returned no structured answer (stop reason %q); "+
			"the tool choice was forced, so this indicates a refusal or a truncated response",
		req.Role, resp.StopReason)
}

func toStrings(v any) []string {
	switch t := v.(type) {
	case []string:
		return t
	case []any:
		out := make([]string, 0, len(t))
		for _, x := range t {
			if s, ok := x.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}
