package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/Rishi0507/manhattan/internal/agent"
	"github.com/Rishi0507/manhattan/internal/evidence"
)

func runAsk(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("ask", flag.ExitOnError)
	dir := fs.String("store", "out", "directory holding receipts.ndjson from a previous run")
	var pf providerFlags
	pf.register(fs)
	if err := fs.Parse(args); err != nil {
		return err
	}

	question := strings.TrimSpace(strings.Join(fs.Args(), " "))
	if question == "" {
		return fmt.Errorf("ask what? for example:\n" +
			"  manhattan ask \"why didn't 5502 post, and what would make it?\"\n" +
			"  manhattan ask \"which constraint dropped the most records?\"\n" +
			"  manhattan ask \"which merchants are hardest to reconcile?\"")
	}

	store, err := evidence.Load(*dir)
	if err != nil {
		return fmt.Errorf("no receipt store at %s: %w\n  run `manhattan bench` first", *dir, err)
	}
	provider, err := selectProvider(pf)
	if err != nil {
		return err
	}

	ans, err := agent.NewQA(provider, store).Ask(ctx, question)
	if err != nil {
		return err
	}

	fmt.Printf("\n%s\n\n", ans.Text)
	if !ans.Answerable {
		fmt.Fprintln(os.Stderr,
			"The receipts do not contain what this question asks for. That is a valid answer:\n"+
				"the agent reads stored evidence and declines rather than inferring.")
	}
	if len(ans.Citations) > 0 {
		fmt.Println("grounded in:")
		for _, c := range ans.Citations {
			if c.Value != "" {
				fmt.Printf("  %s  %s = %s\n", c.ReceiptID, c.Field, c.Value)
			} else {
				fmt.Printf("  %s  %s\n", c.ReceiptID, c.Field)
			}
		}
		fmt.Println()
	}
	return nil
}
