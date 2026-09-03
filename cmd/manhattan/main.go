// Command manhattan is the entry point for everything this project does.
//
// One command reproduces every number in the submission:
//
//	manhattan bench --out out
//
// It is seeded and deterministic. Running it twice produces byte-identical
// receipts, which is the minimum bar for a reconciliation system: a decision
// that cannot be reproduced cannot be audited.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	cmd := os.Args[1]
	args := os.Args[2:]
	ctx := context.Background()

	var err error
	switch cmd {
	case "bench":
		err = runBench(ctx, args)
	case "cases":
		err = runCases(ctx, args)
	case "recon":
		err = runRecon(ctx, args)
	case "ask":
		err = runAsk(ctx, args)
	case "serve":
		err = runServe(ctx, args)
	case "live":
		err = runLive(ctx, args)
	case "docs":
		err = runDocs(ctx, args)
	case "help", "-h", "--help":
		usage()
		return
	default:
		fmt.Fprintf(os.Stderr, "manhattan: unknown command %q\n\n", cmd)
		usage()
		os.Exit(2)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "manhattan: %v\n", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `manhattan - an agent that proves settlements instead of guessing them

  manhattan bench    run the full benchmark and write RESULTS.md
  manhattan cases    run the eleven adversarial cases head to head against B0
  manhattan recon    reconcile one generated batch and print the receipts
  manhattan ask      ask a question of the receipt store
  manhattan serve    serve the dashboard and the API
  manhattan docs     re-render README.md and LIMITATIONS.md from a saved run
  manhattan live     run the same batch live and on the stub, and print the delta

Every command takes --help.

The language model provider is selected automatically:
  ANTHROPIC_API_KEY set        the live Anthropic API, recording to a cassette
  a cassette on disk           replay, with no network and byte-identical results
  neither                      a deterministic offline stub

The stub is not a fallback that degrades the answer. It changes how often the
agent can clear an exception; it cannot change whether a posting is correct,
because the model is never asked whether it was right.
`)
}

// providerFlags adds the provider selection flags shared by every command.
type providerFlags struct {
	cassette string
	offline  bool
	live     bool
}

func (p *providerFlags) register(fs *flag.FlagSet) {
	fs.StringVar(&p.cassette, "cassette", "testdata/cassette.json",
		"recorded model answers, for reproducible runs without a network")
	fs.BoolVar(&p.offline, "offline", false,
		"force the deterministic offline stub even if an API key is present")
	fs.BoolVar(&p.live, "live", false,
		"require the live API and fail rather than falling back")
}

func splitCSV(s string) []string {
	var out []string
	for _, x := range strings.Split(s, ",") {
		if x = strings.TrimSpace(x); x != "" {
			out = append(out, x)
		}
	}
	return out
}
