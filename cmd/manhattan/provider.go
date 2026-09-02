package main

import (
	"fmt"
	"os"

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
	key := os.Getenv("ANTHROPIC_API_KEY")

	if pf.offline {
		fmt.Fprintln(os.Stderr, "provider: offline stub (forced with --offline)")
		return llm.NewOffline(), nil
	}

	if key != "" {
		live := llm.NewAnthropic(llm.DefaultAnthropicConfig())
		rec, err := llm.NewRecorder(live, pf.cassette)
		if err != nil {
			return nil, fmt.Errorf("could not open the cassette at %s: %w", pf.cassette, err)
		}
		fmt.Fprintf(os.Stderr, "provider: Anthropic API (%s), recording to %s\n",
			live.Model(llm.RoleResolve), pf.cassette)
		return rec, nil
	}

	if pf.live {
		return nil, fmt.Errorf(
			"--live was requested but ANTHROPIC_API_KEY is not set.\n" +
				"  Set it, or run `ant auth login`, or drop --live to use the recorded cassette")
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
		"provider: offline stub (no ANTHROPIC_API_KEY and no cassette).\n"+
			"          The stub is deliberately unintelligent. It changes how often the agent\n"+
			"          can clear an exception; it cannot change whether a posting is correct,\n"+
			"          because the verifier never asks the model whether it was right.")
	return llm.NewOffline(), nil
}
