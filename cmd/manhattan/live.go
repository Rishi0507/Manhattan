package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Rishi0507/manhattan/internal/bench"
	"github.com/Rishi0507/manhattan/internal/llm"
)

// The live-versus-stub delta, as one command.
//
// Every published figure in this repository comes from the deterministic
// offline path, and "the LLM was never called" is a fair one-line summary of
// that in an AI buildathon. The architectural answer, that the model must not
// be load-bearing for correctness, is right and is a different claim from
// never having run it.
//
// So this runs the SAME batch twice, once on the live Anthropic API and once
// on the offline stub, and writes the difference. What it is looking for is
// two things that should behave completely differently:
//
//	Postings should not move at all. Not by one. The verifier never asks the
//	model whether it was right, so a better proposer cannot change whether a
//	settlement is correctly posted. If this column moves, the trust boundary
//	leaks somewhere and that is a bug of the first order.
//
//	Diagnosis accuracy and repair count SHOULD move. Those are the jobs where
//	judgement is the whole task: classifying why a report failed its check, and
//	choosing which of seven actions to try. The stub confuses an omitted
//	dispute with a truncated mapping because it reads the residual's sign and
//	nothing else; a model can read the class of record involved.
//
// The first is the safety claim and the second is the value claim, and they
// are measured by the same command because they are the two halves of the same
// argument.
func runLive(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("live", flag.ExitOnError)
	n := fs.Int("n", 60, "settlements to run on each provider")
	seed := fs.Int64("seed", 20260826, "replay seed, identical for both runs")
	out := fs.String("out", "out", "output directory")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if os.Getenv("ANTHROPIC_API_KEY") == "" {
		return fmt.Errorf(
			"ANTHROPIC_API_KEY is not set, and this command exists to measure the live path\n" +
				"  against the offline one, so there is nothing honest for it to do without a key.\n\n" +
				"  export ANTHROPIC_API_KEY=sk-ant-...\n" +
				"  ./bin/manhattan live -n 60\n\n" +
				"  It writes out/live.json, and `manhattan docs` renders the delta into README.md\n" +
				"  and LIMITATIONS.md automatically once that file exists")
	}

	spec := bench.DefaultBatch()
	spec.Settlements = *n
	spec.Seed = *seed

	live := llm.NewAnthropic(llm.DefaultAnthropicConfig())

	fmt.Fprintf(os.Stderr, "running %d settlements on the live API (%s)\n",
		*n, live.Model(llm.RoleParse))
	_, liveSum, err := bench.RunBatch(ctx, spec, live)
	if err != nil {
		return fmt.Errorf("live run: %w", err)
	}

	fmt.Fprintf(os.Stderr, "running the same %d settlements on the offline stub\n", *n)
	_, stubSum, err := bench.RunBatch(ctx, spec, llm.NewOffline())
	if err != nil {
		return fmt.Errorf("stub run: %w", err)
	}

	d := bench.Delta(liveSum, stubSum)
	b, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(*out, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(*out, "live.json"), b, 0o644); err != nil {
		return err
	}

	fmt.Println(bench.RenderDelta(d))
	if d.PostingsMoved {
		fmt.Fprintln(os.Stderr,
			"\nFAILURE: postings differ between providers. The verifier is supposed to be\n"+
				"  indifferent to which model proposed anything, so this is a leak in the trust\n"+
				"  boundary rather than an interesting result. Do not publish the run.")
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "\nwrote %s/live.json. Run `manhattan docs` to fold it into the documents.\n", *out)
	return nil
}
