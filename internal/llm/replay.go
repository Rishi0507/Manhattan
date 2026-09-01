package llm

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
)

// Cassette is a recorded set of model answers, keyed by role and by a hash
// of the exact request.
//
// This exists for a reason that matters more here than in most systems. A
// reconciliation run has to be replayable: if a receipt says a settlement
// verified because the agent cited a particular dispute record, an auditor
// has to be able to re-run that and get the same receipt. A live model call
// in the middle of that chain makes the run unreproducible by construction.
// So every live call is recorded, and a recorded run replays to the paise
// with no network at all.
//
// It also means `git clone && make demo` works with no API key, which is
// what a judge with four minutes actually needs.
type Cassette struct {
	Entries map[string]CassetteEntry `json:"entries"`
}

// CassetteEntry is one recorded answer.
type CassetteEntry struct {
	Role   Role            `json:"role"`
	Model  string          `json:"model"`
	Prompt string          `json:"prompt_excerpt"`
	Answer json.RawMessage `json:"answer"`
	Usage  Usage           `json:"usage"`
}

// Key is the lookup key for a request: the role, the schema and the exact
// text of both prompt blocks. Any change to any of them is a different
// question and deserves a different recorded answer rather than a stale one.
func Key(req Request) string {
	h := sha256.New()
	fmt.Fprintf(h, "%s\x00%s\x00%s\x00%s", req.Role, req.SchemaName, req.System, req.User)
	return string(req.Role) + "_" + hex.EncodeToString(h.Sum(nil))[:16]
}

type replayProvider struct {
	mu       sync.RWMutex
	cassette *Cassette
	fallback Provider
	// misses counts requests answered by the fallback rather than by a
	// recording. It is reported, because a run that silently fell back to a
	// stub is not the run the cassette describes.
	misses int
}

// NewReplay returns a provider that answers from a cassette, falling back to
// the deterministic offline stub for anything not recorded.
func NewReplay(c *Cassette) Provider {
	if c == nil {
		c = &Cassette{Entries: map[string]CassetteEntry{}}
	}
	if c.Entries == nil {
		c.Entries = map[string]CassetteEntry{}
	}
	return &replayProvider{cassette: c, fallback: NewOffline()}
}

// LoadCassette reads a cassette from disk, returning an empty one if the
// file does not exist.
func LoadCassette(path string) (*Cassette, error) {
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return &Cassette{Entries: map[string]CassetteEntry{}}, nil
	}
	if err != nil {
		return nil, err
	}
	var c Cassette
	if err := json.Unmarshal(b, &c); err != nil {
		return nil, fmt.Errorf("llm: cassette at %s is not readable: %w", path, err)
	}
	if c.Entries == nil {
		c.Entries = map[string]CassetteEntry{}
	}
	return &c, nil
}

// Save writes a cassette, with stable key ordering so it diffs cleanly.
func (c *Cassette) Save(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	keys := make([]string, 0, len(c.Entries))
	for k := range c.Entries {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	ordered := map[string]CassetteEntry{}
	for _, k := range keys {
		ordered[k] = c.Entries[k]
	}
	b, err := json.MarshalIndent(&Cassette{Entries: ordered}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

func (p *replayProvider) Name() string { return "replay" }

func (p *replayProvider) Model(r Role) string { return "replay" }

func (p *replayProvider) Structured(ctx context.Context, req Request) (*Result, error) {
	p.mu.RLock()
	e, ok := p.cassette.Entries[Key(req)]
	p.mu.RUnlock()
	if ok {
		return &Result{JSON: e.Answer, Usage: e.Usage, Model: e.Model}, nil
	}
	p.mu.Lock()
	p.misses++
	p.mu.Unlock()
	return p.fallback.Structured(ctx, req)
}

// Misses reports how many requests fell through to the offline stub.
func (p *replayProvider) Misses() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.misses
}

// recorder wraps a live provider and writes every answer to a cassette.
type recorder struct {
	inner    Provider
	mu       sync.Mutex
	cassette *Cassette
	path     string
}

// NewRecorder wraps a provider so that every call it makes is captured. The
// cassette is flushed after each call rather than at exit, so an interrupted
// run still leaves a usable recording.
func NewRecorder(inner Provider, path string) (Provider, error) {
	c, err := LoadCassette(path)
	if err != nil {
		return nil, err
	}
	return &recorder{inner: inner, cassette: c, path: path}, nil
}

func (r *recorder) Name() string           { return r.inner.Name() + "+recording" }
func (r *recorder) Model(role Role) string { return r.inner.Model(role) }

func (r *recorder) Structured(ctx context.Context, req Request) (*Result, error) {
	res, err := r.inner.Structured(ctx, req)
	if err != nil {
		return nil, err
	}
	excerpt := req.User
	if len(excerpt) > 200 {
		excerpt = excerpt[:200] + "..."
	}
	r.mu.Lock()
	r.cassette.Entries[Key(req)] = CassetteEntry{
		Role: req.Role, Model: res.Model, Prompt: excerpt,
		Answer: res.JSON, Usage: res.Usage,
	}
	err = r.cassette.Save(r.path)
	r.mu.Unlock()
	if err != nil {
		return nil, fmt.Errorf("llm: recorded an answer but could not persist it: %w", err)
	}
	return res, nil
}
