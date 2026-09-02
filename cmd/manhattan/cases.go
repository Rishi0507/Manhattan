package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Rishi0507/manhattan/internal/bench"
)

func runCases(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("cases", flag.ExitOnError)
	out := fs.String("out", "out", "directory for the case receipts")
	jsonOnly := fs.Bool("json", false, "emit JSON only, no table")
	var pf providerFlags
	pf.register(fs)
	if err := fs.Parse(args); err != nil {
		return err
	}

	provider, err := selectProvider(pf)
	if err != nil {
		return err
	}

	outcomes := bench.RunCases(ctx, provider)

	if err := os.MkdirAll(*out, 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(outcomes, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(*out, "cases.json"), b, 0o644); err != nil {
		return err
	}

	if *jsonOnly {
		return nil
	}

	printCaseTable(outcomes)
	return nil
}

func printCaseTable(outcomes []bench.CaseOutcome) {
	fmt.Println()
	fmt.Println("ELEVEN ADVERSARIAL CASES, HEAD TO HEAD")
	fmt.Println("Both systems see identical inputs and identical narrowing. The only")
	fmt.Println("difference is what each is willing to conclude from them.")
	fmt.Println()
	fmt.Printf("%-3s %-24s %-6s %-34s %-22s %s\n", "#", "case", "pool", "manhattan", "b0", "")
	fmt.Println(strings.Repeat("-", 118))

	var mPost, mWrong, b0Post, b0Wrong, met int
	for _, oc := range outcomes {
		flags := ""
		if len(oc.Flags) > 0 {
			var short []string
			for _, f := range oc.Flags {
				short = append(short, abbreviate(string(f)))
			}
			flags = " " + strings.Join(short, ",")
		}
		manhattan := string(oc.Status) + flags

		b0 := "held"
		if oc.B0Posted {
			b0 = fmt.Sprintf("POSTED @%.2f", oc.B0Confidence)
			if oc.B0PostedWrong {
				b0 += "  WRONG"
			}
		}

		mark := "ok"
		if !oc.Met {
			mark = "MISS"
		}
		if oc.PostedWrong {
			mark = "WRONG POSTING"
		}

		fmt.Printf("%-3d %-24s %-6d %-34s %-22s %s\n",
			oc.Case.Number, oc.Case.Name, oc.PoolN, truncate(manhattan, 34), b0, mark)

		if oc.Posted {
			mPost++
		}
		if oc.PostedWrong {
			mWrong++
		}
		if oc.B0Posted {
			b0Post++
		}
		if oc.B0PostedWrong {
			b0Wrong++
		}
		if oc.Met {
			met++
		}
	}

	fmt.Println(strings.Repeat("-", 118))
	fmt.Printf("expectations met      %d of %d\n", met, len(outcomes))
	fmt.Printf("manhattan             posted %d, of which wrong: %d\n", mPost, mWrong)
	fmt.Printf("b0                    posted %d, of which wrong: %d\n", b0Post, b0Wrong)
	fmt.Println()
	fmt.Println("The wrong-posting row is the only one that matters, and it only means")
	fmt.Println("something because the baseline is beside it on identical data.")
	fmt.Println()

	for _, oc := range outcomes {
		if oc.Case.Number != 10 && oc.Case.Number != 9 {
			continue
		}
		fmt.Printf("case %d, %s\n  %s\n  why it is here: %s\n",
			oc.Case.Number, oc.Case.Name, oc.Receipt.Claim, oc.Case.Why)
		if n := oc.Receipt.Narrowing.Neighbourhood; n != nil && n.Rival != nil {
			fmt.Printf("  the probe found a rival: swap %v for %v, admitted by relaxing %s\n",
				n.Rival.Removed, n.Rival.Added, n.Culprit)
		}
		if a := oc.Receipt.Agent.Accepted; a != nil {
			fmt.Printf("  the agent proposed %s of %s and cited %s; the verifier re-ran unmodified and it closed\n",
				a.Kind, a.Amount, a.SourceRef)
		}
		fmt.Println()
	}
}

func abbreviate(flag string) string {
	switch flag {
	case "SIGNED_ITEMS_PRESENT":
		return "signed"
	case "RESOLVED_BY_HYPOTHESIS":
		return "agent-cited"
	case "AMOUNT_ENTROPY_INSUFFICIENT":
		return "no-entropy"
	case "FEE_CHECK_CIRCULAR":
		return "fee-circular"
	case "FEE_ANOMALY":
		return "fee-anomaly"
	case "COMPLEMENT_SOLVED":
		return "complement"
	case "ROUNDING_APPLIED":
		return "rounding"
	case "LATTICE_CORRECTED":
		return "lattice"
	case "TWIN_SWAP":
		return "twin"
	}
	return strings.ToLower(flag)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "."
}
