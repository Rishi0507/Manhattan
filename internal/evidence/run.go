package evidence

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"github.com/Rishi0507/manhattan/internal/guards"
	"github.com/Rishi0507/manhattan/internal/narrow"
)

type invariantError struct{ msg string }

func (e invariantError) Error() string { return "evidence invariant violated: " + e.msg }

func errInvariant(format string, args ...any) error {
	return invariantError{fmt.Sprintf(format, args...)}
}

// RunFlag is a finding about a batch rather than about a settlement.
type RunFlag string

const (
	// FlagNarrowingDrift is deliberately a run-level flag and not a
	// per-settlement one. A misconfigured value-date window is stable under a
	// one-day relaxation on every settlement in the run, so no receipt can
	// see it. Putting it on receipts would also invite an analyst to clear it
	// settlement by settlement, which is exactly the wrong response: it gates
	// the batch.
	FlagNarrowingDrift RunFlag = "NARROWING_DRIFT"
)

// Throughput is the run's measured performance.
type Throughput struct {
	WallClockSeconds   float64 `json:"wall_clock_s"`
	SettlementsPerHour float64 `json:"settlements_per_hour"`
	MedianLatencyMS    float64 `json:"median_latency_ms"`
	P95LatencyMS       float64 `json:"p95_latency_ms"`
	PeakMemoryMB       float64 `json:"peak_memory_mb"`
}

// RunCost aggregates the model spend across a run. It is reported as a
// decomposition rather than as a single typed total, because a typed
// aggregate is exactly the kind of number that drifts away from the receipts
// it is supposed to summarise.
type RunCost struct {
	ModelCalls    int     `json:"model_calls"`
	ParseCalls    int     `json:"parse_calls"`
	AgentCalls    int     `json:"agent_calls"`
	ExceptionRate float64 `json:"exception_rate"`
	InputTokens   int     `json:"input_tokens"`
	OutputTokens  int     `json:"output_tokens"`
	INR           float64 `json:"inr"`
	INRPer1k      float64 `json:"inr_per_1k_settlements"`
}

// Run is the batch-level object. It gates the run.
type Run struct {
	RunID       string                `json:"run_id"`
	Settlements int                   `json:"settlements"`
	Flags       []RunFlag             `json:"flags"`
	Drift       []guards.DriftFinding `json:"narrowing_drift,omitempty"`

	StatusCounts map[Status]int `json:"status_counts"`
	FlagCounts   map[Flag]int   `json:"flag_counts"`

	AutoPosted      int `json:"auto_posted"`
	AutoPostedWrong int `json:"auto_posted_wrong"`
	Exceptions      int `json:"exceptions"`

	Throughput Throughput `json:"throughput"`
	Cost       RunCost    `json:"cost"`

	DropRates map[narrow.Constraint]float64 `json:"aggregate_drop_rates,omitempty"`

	Seed          int64  `json:"replay_seed"`
	PolicyVersion string `json:"policy_version"`
	Note          string `json:"note,omitempty"`
}

// Gated reports whether the run should be held rather than posted.
func (r *Run) Gated() bool { return len(r.Drift) > 0 }

// Store is the receipt store. Every decision lands here, and it is what the
// question-answering agent retrieves over. Keeping it as an explicit,
// queryable collection rather than as log output is what turns an audit
// trail into something a finance lead can actually ask questions of.
type Store struct {
	mu       sync.RWMutex
	receipts []*Receipt
	byRef    map[string]*Receipt
	run      *Run
}

// NewStore returns an empty store.
func NewStore() *Store {
	return &Store{byRef: map[string]*Receipt{}}
}

// Put files a receipt, enforcing the status invariants on the way in. A
// receipt that violates them is a bug in the pipeline, and it is refused at
// the boundary rather than persisted and discovered later.
func (s *Store) Put(r *Receipt) error {
	if err := r.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.receipts = append(s.receipts, r)
	s.byRef[r.SettlementRef] = r
	return nil
}

// Get returns one receipt by settlement reference.
func (s *Store) Get(ref string) (*Receipt, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	r, ok := s.byRef[ref]
	return r, ok
}

// All returns every receipt in insertion order.
func (s *Store) All() []*Receipt {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*Receipt, len(s.receipts))
	copy(out, s.receipts)
	return out
}

// SetRun attaches the run object.
func (s *Store) SetRun(r *Run) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.run = r
}

// Run returns the run object.
func (s *Store) Run() *Run {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.run
}

// Exceptions returns every receipt that did not auto-post, ordered by the
// cost of handling it.
//
// This ordering is the point. An exception list sorted by money is a work
// queue that can be worked in the order that clears the most value per hour.
// An exception list in arrival order is a failure log.
func (s *Store) Exceptions() []*Receipt {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []*Receipt
	for _, r := range s.receipts {
		if !r.Status.Postable() {
			out = append(out, r)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].ExceptionCostINR != out[j].ExceptionCostINR {
			return out[i].ExceptionCostINR > out[j].ExceptionCostINR
		}
		return out[i].TargetPaise > out[j].TargetPaise
	})
	return out
}

// Save writes the store to a directory as newline-delimited receipts plus a
// run object, so a run is replayable and diffable rather than ephemeral.
func (s *Store) Save(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	f, err := os.Create(filepath.Join(dir, "receipts.ndjson"))
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	for _, r := range s.receipts {
		if err := enc.Encode(r); err != nil {
			return err
		}
	}
	if s.run != nil {
		b, err := json.MarshalIndent(s.run, "", "  ")
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(dir, "run.json"), b, 0o644); err != nil {
			return err
		}
	}
	return nil
}

// Load reads a store back from disk.
func Load(dir string) (*Store, error) {
	s := NewStore()
	b, err := os.ReadFile(filepath.Join(dir, "receipts.ndjson"))
	if err != nil {
		return nil, err
	}
	dec := json.NewDecoder(newLineReader(b))
	for dec.More() {
		var r Receipt
		if err := dec.Decode(&r); err != nil {
			return nil, err
		}
		rr := r
		s.receipts = append(s.receipts, &rr)
		s.byRef[rr.SettlementRef] = &rr
	}
	if rb, err := os.ReadFile(filepath.Join(dir, "run.json")); err == nil {
		var run Run
		if err := json.Unmarshal(rb, &run); err == nil {
			s.run = &run
		}
	}
	return s, nil
}

// newLineReader wraps a byte slice for streaming JSON decode.
func newLineReader(b []byte) *bytesReader { return &bytesReader{b: b} }

type bytesReader struct {
	b []byte
	i int
}

func (r *bytesReader) Read(p []byte) (int, error) {
	if r.i >= len(r.b) {
		return 0, io.EOF
	}
	n := copy(p, r.b[r.i:])
	r.i += n
	return n, nil
}
