// Package llm is the boundary between Manhattan and a language model.
//
// Everything the model is allowed to do passes through this one interface,
// and the interface only speaks in structured objects. There is no method
// here that returns free text into a decision path, because there is no
// point in the pipeline where free text could be acted on.
//
// The package ships two live providers, Gemini and Anthropic. Neither is
// privileged: they implement one interface, they are both forced into
// structured output by the vendor's own mechanism, and nothing downstream can
// tell which one answered. That is the point of putting the boundary here.
// The replay provider answers from a recorded fixture, which is what makes
// `make demo` work with no API key, makes the benchmark reproducible to the
// paise, and makes it possible to assert in tests that a specific model
// answer produces a specific verified outcome. A reconciliation system whose
// results cannot be reproduced without a network call is not auditable, so
// the offline path is a design requirement rather than a convenience.
package llm

import (
	"context"
	"encoding/json"
	"fmt"
)

// Role names what the model is being asked to do. Each role can be pointed
// at a different model, because the jobs differ by an order of magnitude in
// difficulty and in call volume: a narration parse runs on every settlement
// and reads a few hundred tokens, while the resolution agent runs only on
// exceptions and has to reason about what class of financial event explains
// a residual.
type Role string

const (
	// RoleParse reads an unstructured bank narration into typed fields.
	// High volume, low difficulty, and the only call most settlements make.
	RoleParse Role = "parse"
	// RoleResolve proposes typed hypotheses for an unexplained residual.
	// Low volume, high difficulty, strictly bounded iterations.
	RoleResolve Role = "resolve"
	// RolePlan orders the narrowing constraints to relax. It can only
	// reorder a list the system would otherwise walk exhaustively.
	RolePlan Role = "plan"
	// RoleAnswer answers questions over the receipt store, grounded in
	// stored evidence and nothing else.
	RoleAnswer Role = "answer"
	// RoleExplain renders a verified derivation into readable English. It
	// writes from facts it is given; it never sources them.
	RoleExplain Role = "explain"

	// RoleTriage classifies WHY a settlement report's stated mapping failed
	// its arithmetic check, from a closed vocabulary of defect classes.
	//
	// The check itself is arithmetic and the model has no vote in it: the
	// residual, the missing ids and the count mismatch are all computed before
	// this call is made. What the model contributes is the diagnosis, which is
	// the part a human otherwise does. "The report is short by one record with
	// a residual equal to a single chargeback" and "the report names a payment
	// that settled last cycle" produce the same failed check and completely
	// different remedies, and telling them apart is reading rather than
	// counting.
	RoleTriage Role = "triage"

	// RoleRemediate drafts the merchant-facing action for a held settlement
	// whose cure has already been computed and verified.
	//
	// This is the highest-volume place the model does something nobody else
	// can. A proven cure is a fact: tightening this window to seven hours
	// yields exactly one reconstruction with the identity closing to zero.
	// What an operations lead needs is that fact turned into a sentence they
	// can send, naming the change, its effect and what it will not fix. The
	// facts are supplied and schema-checked; the model never sources them.
	RoleRemediate Role = "remediate"

	// RoleControl reads a whole period and writes the close.
	//
	// This is the role the track is named after and it is the only one that
	// works above a single settlement. Four hundred exceptions do not have
	// four hundred causes; they have three or four wearing different reference
	// numbers, and finding them is reading across receipts rather than
	// counting within one.
	//
	// It cannot act. The close posts nothing, narrows nothing and amends no
	// input. A person reads it and then decides, which is why it is the one
	// model output here not bounded by a closed action vocabulary.
	RoleControl Role = "control"
)

// Request is one structured call. The model is always given a JSON schema
// and forced to answer inside it, so a malformed answer is a transport error
// rather than something the pipeline has to interpret.
type Request struct {
	Role Role
	// System is the instruction block. It is stable per role so that prompt
	// caching actually hits: the volatile part of the call lives entirely in
	// User, after the cache breakpoint.
	System string
	User   string

	// SchemaName, SchemaDesc and Schema define the shape of a valid answer.
	SchemaName string
	SchemaDesc string
	Schema     map[string]any

	MaxTokens int
}

// Usage is what a call cost.
type Usage struct {
	InputTokens      int `json:"input_tokens"`
	OutputTokens     int `json:"output_tokens"`
	CacheReadTokens  int `json:"cache_read_input_tokens"`
	CacheWriteTokens int `json:"cache_creation_input_tokens"`
	Calls            int `json:"calls"`
	// ByRole counts calls per role, so a submission can state where the model
	// is actually being used rather than quoting one aggregate.
	ByRole    map[Role]int `json:"calls_by_role,omitempty"`
	INRMicros int64        `json:"inr_micros"`
}

// Add accumulates usage across calls.
func (u *Usage) Add(o Usage) {
	u.InputTokens += o.InputTokens
	u.OutputTokens += o.OutputTokens
	u.CacheReadTokens += o.CacheReadTokens
	u.CacheWriteTokens += o.CacheWriteTokens
	u.Calls += o.Calls
	if len(o.ByRole) > 0 {
		if u.ByRole == nil {
			u.ByRole = map[Role]int{}
		}
		for r, n := range o.ByRole {
			u.ByRole[r] += n
		}
	}
	u.INRMicros += o.INRMicros
}

// Result is a structured answer plus what it cost.
type Result struct {
	// JSON is the model's answer, already validated against the schema by
	// the API's strict tool-use enforcement.
	JSON  json.RawMessage
	Usage Usage
	Model string
}

// Into unmarshals the answer into a typed value.
func (r *Result) Into(v any) error {
	if len(r.JSON) == 0 {
		return fmt.Errorf("llm: empty structured result")
	}
	if err := json.Unmarshal(r.JSON, v); err != nil {
		return fmt.Errorf("llm: structured result did not fit its own schema: %w", err)
	}
	return nil
}

// Provider is the whole surface a language model is given.
//
// Note what is absent: there is no Complete, no Chat, no method returning a
// string. Adding one would create a path by which model output could reach a
// posting decision without passing through a schema, and the trust boundary
// this package exists to draw would stop being enforceable by the type
// system.
type Provider interface {
	// Name identifies the provider on receipts, so a run can be attributed.
	Name() string
	// Model reports which model backs a given role.
	Model(Role) string
	// Structured makes one call and returns a schema-conforming answer.
	Structured(ctx context.Context, req Request) (*Result, error)
}

// Pricing is the per-million-token rate for one model, in US dollars.
type Pricing struct {
	InputPerMTok  float64
	OutputPerMTok float64
	// CacheReadPerMTok is charged at a tenth of the input rate, and cache
	// writes at 1.25x, which is why prompt caching is worth the trouble on
	// the parse role: the system block is identical on every settlement.
	CacheReadPerMTok  float64
	CacheWritePerMTok float64
}

// Rates are the vendors' published prices, in USD per million tokens.
var Rates = map[string]Pricing{
	"claude-opus-5":    {InputPerMTok: 5.00, OutputPerMTok: 25.00, CacheReadPerMTok: 0.50, CacheWritePerMTok: 6.25},
	"claude-sonnet-5":  {InputPerMTok: 2.00, OutputPerMTok: 10.00, CacheReadPerMTok: 0.20, CacheWritePerMTok: 2.50},
	"claude-haiku-4-5": {InputPerMTok: 1.00, OutputPerMTok: 5.00, CacheReadPerMTok: 0.10, CacheWritePerMTok: 1.25},
	"claude-fable-5":   {InputPerMTok: 10.00, OutputPerMTok: 50.00, CacheReadPerMTok: 1.00, CacheWritePerMTok: 12.50},
	// Google's published rates, USD per million tokens. Gemini bills cached
	// input at a quarter of the input rate rather than a tenth, which is why
	// the cache figure on a Gemini run is less dramatic than on an Anthropic
	// one and is reported separately rather than blended.
	"gemini-2.5-pro":   {InputPerMTok: 1.25, OutputPerMTok: 10.00, CacheReadPerMTok: 0.31, CacheWritePerMTok: 1.25},
	"gemini-2.5-flash": {InputPerMTok: 0.30, OutputPerMTok: 2.50, CacheReadPerMTok: 0.075, CacheWritePerMTok: 0.30},
	"replay":           {},
}

// USDToINR is the conversion used when pricing a run in rupees. It is a
// configured assumption like any other, and it is printed alongside the
// figures it produces rather than buried.
const USDToINR = 88.0

// CostMicrosINR prices one call's usage in millionths of a rupee, which
// keeps the aggregate exact in integer arithmetic even when a single call
// costs a fraction of a paisa.
func CostMicrosINR(model string, u Usage) int64 {
	p, ok := Rates[model]
	if !ok {
		return 0
	}
	usd := float64(u.InputTokens)*p.InputPerMTok/1e6 +
		float64(u.OutputTokens)*p.OutputPerMTok/1e6 +
		float64(u.CacheReadTokens)*p.CacheReadPerMTok/1e6 +
		float64(u.CacheWriteTokens)*p.CacheWritePerMTok/1e6
	return int64(usd * USDToINR * 1e6)
}
