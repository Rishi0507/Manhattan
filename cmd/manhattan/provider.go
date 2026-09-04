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

	// Whichever key is present. Both paths satisfy the same interface and
	// nothing downstream can tell them apart, which is the point of the
	// boundary: a second vendor is one file, not a migration.
	//
	// An explicit --provider wins, so a machine carrying both keys is not left
	// guessing.
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
				"  export GEMINI_API_KEY=...      for the Gemini path\n" +
				"  export ANTHROPIC_API_KEY=...   for the Anthropic path\n" +
				"  or drop --live to use the recorded cassette")
	}

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
// Gemini is preferred when both keys are present because it is the one the
// submission is configured for; --provider overrides that in either direction.
func liveProvider(pf providerFlags) (llm.Provider, error) {
	gem := llm.DefaultGeminiConfig()
	ant := llm.DefaultAnthropicConfig()

	switch strings.ToLower(pf.provider) {
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
			"unknown --provider %q; it is one of gemini, anthropic", pf.provider)
	}

	if gem.APIKey != "" {
		return llm.NewGemini(gem), nil
	}
	if ant.APIKey != "" {
		return llm.NewAnthropic(ant), nil
	}
	return nil, nil
}
