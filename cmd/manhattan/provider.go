package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/Rishi0507/manhattan/internal/llm"
)

// selectProvider resolves which language model backs this run, and says so on
// stderr rather than deciding silently.
//
// The order is deliberate. A live key means the real thing, wrapped in a
// recorder so the run can be replayed later without a network. A cassette on
// disk with no key means replay, which reproduces a previous run exactly. And
// with neither, the offline stub keeps `make demo` working for someone who
// cloned the repository four minutes ago.
//
// Announcing the choice matters. Every receipt records which provider
// produced it, and a run whose provenance is ambiguous is not evidence of
// anything.
func selectProvider(pf providerFlags) (llm.Provider, error) {
	if pf.offline {
		fmt.Fprintln(os.Stderr, "provider: offline stub (forced with --offline)")
		return llm.NewOffline(), nil
	}

	// The live path is opt-in, and that is a correction rather than a
	// preference.
	//
	// It used to go live whenever a key was visible, which meant that leaving a
	// key in .env silently turned `manhattan bench` into fifteen hundred billed
	// calls against a key rated for ten a minute. The run produced nothing and
	// spent the day's whole allowance doing it. A default that can empty a
	// quota is not a default.
	//
	// So the reproducible path is what you get unless you ask for the other
	// one, which also happens to be the honest default for a repository whose
	// argument is that its numbers reproduce offline.
	if !pf.live && pf.provider == "" {
		return offlineOrReplay(pf)
	}

	// Both live paths satisfy the same interface and nothing downstream can
	// tell them apart, which is the point of the boundary: a second vendor is
	// one file, not a migration. An explicit --provider wins, so a machine
	// carrying both keys is not left guessing.
	if live, err := liveProvider(pf); err != nil {
		return nil, err
	} else if live != nil {
		rec, err := llm.NewRecorder(live, pf.cassette)
		if err != nil {
			return nil, fmt.Errorf("could not open the cassette at %s: %w", pf.cassette, err)
		}
		fmt.Fprintf(os.Stderr, "provider: %s (%s), recording to %s\n",
			live.Name(), live.Model(llm.RoleResolve), pf.cassette)
		return rec, nil
	}

	if pf.live {
		return nil, fmt.Errorf(
			"--live was requested but no API key is set.\n" +
				"  export GROQ_API_KEY=...        for Groq (best free tier)\n" +
				"  export GEMINI_API_KEY=...      for the Gemini path\n" +
				"  export ANTHROPIC_API_KEY=...   for the Anthropic path\n" +
				"  or drop --live to use the recorded cassette")
	}

	return offlineOrReplay(pf)
}

// offlineOrReplay is the reproducible path: a recorded cassette if there is
// one, the deterministic stub otherwise.
func offlineOrReplay(pf providerFlags) (llm.Provider, error) {
	cas, err := llm.LoadCassette(pf.cassette)
	if err != nil {
		return nil, err
	}
	if n := len(cas.Entries); n > 0 {
		fmt.Fprintf(os.Stderr, "provider: replay from %s (%d recorded answers, no network)\n", pf.cassette, n)
		return llm.NewReplay(cas), nil
	}

	fmt.Fprintln(os.Stderr,
		"provider: offline stub (no GEMINI_API_KEY or ANTHROPIC_API_KEY, and no cassette).\n"+
			"          The stub is deliberately unintelligent. It changes how often the agent\n"+
			"          can clear an exception; it cannot change whether a posting is correct,\n"+
			"          because the verifier never asks the model whether it was right.")
	return llm.NewOffline(), nil
}

// liveProvider returns the configured live provider, or nil when no key is set.
//
// Groq is preferred when its key is present because it has the best free tier
// without quota issues. Gemini and Anthropic are also supported.
func liveProvider(pf providerFlags) (llm.Provider, error) {
	groq := llm.DefaultGroqConfig()
	gem := llm.DefaultGeminiConfig()
	ant := llm.DefaultAnthropicConfig()

	switch strings.ToLower(pf.provider) {
	case "groq":
		if groq.APIKey == "" {
			return nil, fmt.Errorf(
				"--provider groq was requested but GROQ_API_KEY is not set")
		}
		return llm.NewGroq(groq), nil
	case "gemini", "google":
		if gem.APIKey == "" {
			return nil, fmt.Errorf(
				"--provider gemini was requested but neither GEMINI_API_KEY nor " +
					"GOOGLE_API_KEY is set")
		}
		return llm.NewGemini(gem), nil
	case "anthropic", "claude":
		if ant.APIKey == "" {
			return nil, fmt.Errorf(
				"--provider anthropic was requested but ANTHROPIC_API_KEY is not set")
		}
		return llm.NewAnthropic(ant), nil
	case "":
	default:
		return nil, fmt.Errorf(
			"unknown --provider %q; it is one of groq, gemini, anthropic", pf.provider)
	}

	// Auto-select: prefer Groq (best free tier), then Gemini, then Anthropic
	if groq.APIKey != "" {
		return llm.NewGroq(groq), nil
	}
	if gem.APIKey != "" {
		return llm.NewGemini(gem), nil
	}
	if ant.APIKey != "" {
		return llm.NewAnthropic(ant), nil
	}
	return nil, nil
}
