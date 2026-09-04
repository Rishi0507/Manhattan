package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Rishi0507/manhattan/internal/bench"
	"github.com/Rishi0507/manhattan/internal/guards"
	"github.com/Rishi0507/manhattan/internal/narrow"
)

func runBench(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("bench", flag.ExitOnError)
	settlements := fs.Int("n", 500, "settlements to reconcile")
	seed := fs.Int64("seed", 20260826, "replay seed; the same seed produces byte-identical receipts")
	out := fs.String("out", "out", "output directory for receipts, run object and figures")
	results := fs.String("results", "RESULTS.md", "generated results document")
	skipSweep := fs.Bool("skip-sweep", false, "skip the collision-index calibration sweep")
	mix := fs.String("archetypes", "", "comma-separated merchant archetypes, default is all six")
	drift := fs.Bool("demo-drift", false,
		"compare against a deliberately stale narrowing baseline, to show the run-level drift monitor firing")
	var pf providerFlags
	pf.register(fs)
	if err := fs.Parse(args); err != nil {
		return err
	}

	provider, err := selectProvider(pf)
	if err != nil {
		return err
	}

	spec := bench.DefaultBatch()
	spec.Settlements = *settlements
	spec.Seed = *seed
	if *mix != "" {
		spec.ArchetypeMix = splitCSV(*mix)
	}
	// A stored baseline so the run-level narrowing drift monitor has something
	// to deviate from.
	//
	// These rates were measured from a prior clean run rather than guessed,
	// which matters: a baseline of invented numbers makes the monitor fire on
	// every run, and a gate that always fires is indistinguishable from no gate
	// at all. Drift is detected against what this configuration actually does.
	//
	// The honest boundary, stated here because it is easy to miss: this catches
	// a constraint that has CHANGED. It cannot catch one that was wrong from the
	// first run, because there is nothing to deviate from. That is the sharpest
	// remaining hole in the completeness argument and it is in LIMITATIONS.md.
	spec.Baseline = guards.DriftBaseline{
		RunID: "baseline_run_20260826",
		Rates: map[narrow.Constraint]float64{
			narrow.ConstraintMerchant:         0.134,
			narrow.ConstraintWindow:           0.772,
			narrow.ConstraintReconciled:       0.085,
			narrow.ConstraintZeroContribution: 0.000,
		},
	}
	if *drift {
		// Deliberately stale baseline, to demonstrate the monitor firing. The
		// point of the demonstration is that this is a run-level finding: it
		// gates the batch and never appears on an individual receipt, because
		// a diagnostic about a population does not belong on a receipt about
		// one member of it.
		spec.Baseline.Rates[narrow.ConstraintWindow] = 0.30
		spec.Baseline.RunID = "baseline_run_20260719_stale"
	}

	fmt.Fprintf(os.Stderr, "reconciling %d settlements across %d merchant archetypes, seed %d\n",
		spec.Settlements, len(spec.ArchetypeMix), spec.Seed)

	store, summary, err := bench.RunBatch(ctx, spec, provider)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(*out, 0o755); err != nil {
		return err
	}
	if err := store.Save(*out); err != nil {
		return err
	}

	fmt.Fprintln(os.Stderr, "running the eleven adversarial cases")
	cases := bench.RunCases(ctx, provider)

	var sweep []bench.SweepPoint
	if !*skipSweep {
		fmt.Fprintln(os.Stderr, "running the collision-index calibration sweep")
		sweep = bench.Sweep(bench.DefaultSweep())
	}

	fmt.Fprintln(os.Stderr, "sweeping the agent's contribution against operational misconfiguration")
	sensitivity := bench.Sensitivity(ctx, spec, provider)

	fmt.Fprintln(os.Stderr, "measuring the solver resource envelope")
	envelope := bench.Envelope()

	arts := map[string]any{
		"summary":            summary,
		"cases":              cases,
		"sweep":              sweep,
		"envelope":           envelope,
		"operating_envelope": bench.OperatingEnvelope(1024),
		"sensitivity":        sensitivity,
		"buckets":            bench.LogSpaced(sweep, 8),
		// The same curve segmented by batch cardinality, because that is the
		// variable the index has to be read against.
		"cardinality_bands": bench.CardinalityBands(sweep, 4),
	}
	for name, v := range arts {
		b, err := json.MarshalIndent(v, "", "  ")
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(*out, name+".json"), b, 0o644); err != nil {
			return err
		}
	}

	doc := renderResults(summary, cases, sweep, envelope, sensitivity)
	if err := os.WriteFile(*results, []byte(doc), 0o644); err != nil {
		return err
	}

	// README.md and LIMITATIONS.md are regenerated in the same command that
	// produced the numbers, so the three documents cannot disagree. Leaving
	// this as a separate step is what let them drift apart once already.
	if err := renderNarrativeDocs(ctx, summary, cases, sweep, envelope, sensitivity,
		bench.OperatingEnvelope(1024), store, provider); err != nil {
		return err
	}

	printCaseTable(cases)
	fmt.Print(doc)
	fmt.Fprintf(os.Stderr, "\nwrote %s and %s/{receipts.ndjson,run.json,summary.json,cases.json,sweep.json,envelope.json}\n",
		*results, *out)
	return nil
}
