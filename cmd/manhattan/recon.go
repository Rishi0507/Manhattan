package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/Rishi0507/manhattan/internal/agent"
	"github.com/Rishi0507/manhattan/internal/evidence"
	"github.com/Rishi0507/manhattan/internal/generate"
	"github.com/Rishi0507/manhattan/internal/pipeline"
)

func runRecon(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("recon", flag.ExitOnError)
	n := fs.Int("n", 12, "settlements to generate and reconcile")
	arch := fs.String("archetype", "marketplace", "merchant archetype")
	seed := fs.Int64("seed", 20260826, "replay seed")
	pool := fs.Int("pool", 34, "target candidate pool size after narrowing")
	full := fs.Bool("full", false, "print the complete evidence object for each settlement")
	out := fs.String("out", "", "optional directory to write receipts into")
	var pf providerFlags
	pf.register(fs)
	if err := fs.Parse(args); err != nil {
		return err
	}

	provider, err := selectProvider(pf)
	if err != nil {
		return err
	}

	gs := generate.DefaultSpec()
	gs.Archetype = *arch
	gs.Seed = *seed
	gs.Settlements = *n
	gs.PoolTarget = *pool
	ds := generate.Generate(gs)

	cfg := pipeline.DefaultConfig()
	cfg.Seed = *seed
	cfg.RunID = fmt.Sprintf("recon_%s_%d", *arch, *seed)
	eng := pipeline.New(ds, cfg)
	store := evidence.NewStore()
	resolver := agent.NewResolver(provider)

	fmt.Printf("\n%-30s %5s %5s %6s %-22s %s\n", "settlement", "n", "|S|", "E@k*", "status", "claim")
	for _, credit := range ds.Credits {
		rec := eng.Reconcile(credit)
		if rec.Status == evidence.StatusUnresolved {
			rec, _ = resolver.Resolve(ctx, eng, credit, rec)
		}
		if err := store.Put(rec); err != nil {
			return err
		}
		fmt.Printf("%-30s %5d %5d %6.3g %-22s %s\n",
			rec.SettlementRef, rec.Pool.N, rec.WitnessSize,
			rec.Feasibility.IndexAtKStar, rec.Status, truncate(rec.Claim, 90))

		if *full {
			b, _ := json.MarshalIndent(rec, "", "  ")
			fmt.Println(string(b))
		}
	}

	printExceptions(store)

	if *out != "" {
		if err := os.MkdirAll(*out, 0o755); err != nil {
			return err
		}
		return store.Save(*out)
	}
	return nil
}

// printExceptions renders the exception queue as a work list.
//
// The track brief asks for an honest exception list. Most systems treat that
// as an apology: a list of things that failed, in arrival order, for someone
// else to sort out. Sorted by the cost of handling it, grouped by named
// cause, with a computed cure attached to each row, it is a different object.
func printExceptions(store *evidence.Store) {
	ex := store.Exceptions()
	if len(ex) == 0 {
		return
	}
	total := 0
	for _, r := range ex {
		total += r.ExceptionCostINR
	}

	fmt.Printf("\nEXCEPTION QUEUE: %d settlements, INR %d at the configured analyst rate\n", len(ex), total)
	fmt.Println("Sorted by cost, so it can be worked in the order that clears the most money per hour.")
	fmt.Println()
	for i, r := range ex {
		if i >= 8 {
			fmt.Printf("  ... and %d more\n", len(ex)-8)
			break
		}
		fmt.Printf("  %-28s %-20s INR %-6d  %s\n",
			r.SettlementRef, r.Status, r.ExceptionCostINR, truncate(r.Claim, 78))
		for _, rem := range r.Remediation {
			fmt.Printf("      cure: %s\n            %s\n", rem.Action, rem.Effect)
			break
		}
	}
	fmt.Println()
}
