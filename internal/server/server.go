// Package server exposes a run over HTTP: the receipts, the exception queue,
// the head-to-head comparison, the question-answering agent, and a live
// stream of a batch as it reconciles.
//
// The API is deliberately thin. Every endpoint returns the evidence objects
// the pipeline already produced, unmodified, because a dashboard that
// reshapes its inputs is a second place for a number to be wrong.
package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"sync"
	"time"

	"github.com/Rishi0507/manhattan/internal/agent"
	"github.com/Rishi0507/manhattan/internal/bench"
	"github.com/Rishi0507/manhattan/internal/evidence"
	"github.com/Rishi0507/manhattan/internal/llm"
)

// Server holds the state a dashboard reads.
type Server struct {
	mu       sync.RWMutex
	store    *evidence.Store
	summary  *bench.Summary
	cases    []bench.CaseOutcome
	sweep    []bench.SweepPoint
	envelope []bench.EnvelopePoint

	provider llm.Provider
	static   fs.FS

	// subscribers receive live events while a batch runs.
	subs   map[chan Event]struct{}
	subsMu sync.Mutex
}

// Event is one live-run message.
type Event struct {
	Type string `json:"type"`
	// Receipt is set on a settlement event.
	Receipt *evidence.Receipt `json:"receipt,omitempty"`
	// Progress is set on every event.
	Done  int `json:"done"`
	Total int `json:"total"`
	// Summary is set on the final event.
	Summary *bench.Summary `json:"summary,omitempty"`
	Message string         `json:"message,omitempty"`
}

// New builds a server over an existing store.
func New(store *evidence.Store, provider llm.Provider, static fs.FS) *Server {
	if store == nil {
		store = evidence.NewStore()
	}
	return &Server{store: store, provider: provider, static: static, subs: map[chan Event]struct{}{}}
}

// SetResults attaches benchmark artifacts for the dashboard.
func (s *Server) SetResults(sum *bench.Summary, cases []bench.CaseOutcome, sweep []bench.SweepPoint, env []bench.EnvelopePoint) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.summary, s.cases, s.sweep, s.envelope = sum, cases, sweep, env
}

// Store returns the receipt store.
func (s *Server) Store() *evidence.Store {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.store
}

// Handler builds the HTTP routes.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"ok": true, "provider": s.provider.Name()})
	})

	mux.HandleFunc("GET /api/run", func(w http.ResponseWriter, r *http.Request) {
		s.mu.RLock()
		defer s.mu.RUnlock()
		writeJSON(w, map[string]any{
			"run":      s.store.Run(),
			"summary":  s.summary,
			"provider": s.provider.Name(),
			"models": map[string]string{
				"parse":   s.provider.Model(llm.RoleParse),
				"resolve": s.provider.Model(llm.RoleResolve),
				"answer":  s.provider.Model(llm.RoleAnswer),
			},
		})
	})

	mux.HandleFunc("GET /api/receipts", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, s.Store().All())
	})

	mux.HandleFunc("GET /api/receipts/{ref}", func(w http.ResponseWriter, r *http.Request) {
		rec, ok := s.Store().Get(r.PathValue("ref"))
		if !ok {
			http.Error(w, "no receipt with that settlement reference", http.StatusNotFound)
			return
		}
		writeJSON(w, rec)
	})

	mux.HandleFunc("GET /api/exceptions", func(w http.ResponseWriter, r *http.Request) {
		ex := s.Store().Exceptions()
		total := 0
		byCause := map[evidence.Status]int{}
		for _, e := range ex {
			total += e.ExceptionCostINR
			byCause[e.Status]++
		}
		writeJSON(w, map[string]any{
			"exceptions":     ex,
			"total_cost_inr": total,
			"by_status":      byCause,
			"note": "sorted by the cost of handling it, so the queue can be worked in the " +
				"order that clears the most money per hour",
		})
	})

	mux.HandleFunc("GET /api/cases", func(w http.ResponseWriter, r *http.Request) {
		s.mu.RLock()
		defer s.mu.RUnlock()
		writeJSON(w, s.cases)
	})

	mux.HandleFunc("GET /api/sweep", func(w http.ResponseWriter, r *http.Request) {
		s.mu.RLock()
		defer s.mu.RUnlock()
		writeJSON(w, map[string]any{
			"points":   s.sweep,
			"buckets":  bench.LogSpaced(s.sweep, 8),
			"envelope": s.envelope,
		})
	})

	mux.HandleFunc("POST /api/ask", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Question string `json:"question"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "expected a JSON body with a question field", http.StatusBadRequest)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
		defer cancel()

		ans, err := agent.NewQA(s.provider, s.Store()).Ask(ctx, body.Question)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		writeJSON(w, ans)
	})

	// A live run, streamed. This is what makes the demo read as an agent
	// working rather than as a report being displayed.
	mux.HandleFunc("GET /api/stream", s.stream)
	mux.HandleFunc("POST /api/run/start", s.startRun)

	if s.static != nil {
		mux.Handle("/", http.FileServer(http.FS(s.static)))
	}

	return withCORS(mux)
}

func (s *Server) stream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ch := make(chan Event, 64)
	s.subsMu.Lock()
	s.subs[ch] = struct{}{}
	s.subsMu.Unlock()
	defer func() {
		s.subsMu.Lock()
		delete(s.subs, ch)
		close(ch)
		s.subsMu.Unlock()
	}()

	// Replay what has already happened, so a client that connects late still
	// sees the whole run rather than joining midway.
	existing := s.Store().All()
	for i, rec := range existing {
		writeEvent(w, Event{Type: "settlement", Receipt: rec, Done: i + 1, Total: len(existing)})
	}
	flusher.Flush()

	ping := time.NewTicker(20 * time.Second)
	defer ping.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case ev := <-ch:
			writeEvent(w, ev)
			flusher.Flush()
		case <-ping.C:
			fmt.Fprint(w, ": keepalive\n\n")
			flusher.Flush()
		}
	}
}

func (s *Server) publish(ev Event) {
	s.subsMu.Lock()
	defer s.subsMu.Unlock()
	for ch := range s.subs {
		select {
		case ch <- ev:
		default: // a slow client is dropped rather than blocking the run
		}
	}
}

func (s *Server) startRun(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Settlements int   `json:"settlements"`
		Seed        int64 `json:"seed"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)

	spec := bench.DefaultBatch()
	if body.Settlements > 0 {
		spec.Settlements = body.Settlements
	}
	if body.Seed != 0 {
		spec.Seed = body.Seed
	}

	go func() {
		ctx := context.Background()
		s.publish(Event{Type: "start", Total: spec.Settlements, Message: "reconciling"})

		store, sum, err := bench.RunBatch(ctx, spec, s.provider)
		if err != nil {
			s.publish(Event{Type: "error", Message: err.Error()})
			return
		}
		s.mu.Lock()
		s.store = store
		s.summary = &sum
		s.mu.Unlock()

		all := store.All()
		for i, rec := range all {
			s.publish(Event{Type: "settlement", Receipt: rec, Done: i + 1, Total: len(all)})
		}
		s.publish(Event{Type: "done", Done: len(all), Total: len(all), Summary: &sum})
	}()

	writeJSON(w, map[string]any{"started": true, "settlements": spec.Settlements})
}

func writeEvent(w http.ResponseWriter, ev Event) {
	b, err := json.Marshal(ev)
	if err != nil {
		return
	}
	fmt.Fprintf(w, "data: %s\n\n", b)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// withCORS allows the Vite dev server to talk to this API during development.
func withCORS(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		h.ServeHTTP(w, r)
	})
}
