package main

import (
	"context"
	"embed"
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/Rishi0507/manhattan/internal/bench"
	"github.com/Rishi0507/manhattan/internal/evidence"
	"github.com/Rishi0507/manhattan/internal/server"
)

// The dashboard is compiled into the binary, so `manhattan serve` needs
// nothing on disk and a judge can run the demo from a single file.
//
//go:embed all:dist
var dist embed.FS

func runServe(ctx context.Context, args []string) error {
	fs_ := flag.NewFlagSet("serve", flag.ExitOnError)
	addr := fs_.String("addr", ":8080", "listen address")
	dir := fs_.String("store", "out", "directory holding a previous run, if any")
	webDir := fs_.String("web", "", "serve the dashboard from this directory instead of the embedded build")
	var pf providerFlags
	pf.register(fs_)
	if err := fs_.Parse(args); err != nil {
		return err
	}

	provider, err := selectProvider(pf)
	if err != nil {
		return err
	}

	store, err := evidence.Load(*dir)
	if err != nil {
		fmt.Fprintf(os.Stderr,
			"no previous run at %s, starting empty. Run `manhattan bench` or POST /api/run/start.\n", *dir)
		store = evidence.NewStore()
	} else {
		fmt.Fprintf(os.Stderr, "loaded %d receipts from %s\n", len(store.All()), *dir)
	}

	var static fs.FS
	if *webDir != "" {
		static = os.DirFS(*webDir)
	} else if sub, err := fs.Sub(dist, "dist"); err == nil {
		static = sub
	}

	srv := server.New(store, provider, static)
	loadArtifacts(srv, *dir)

	h := &http.Server{
		Addr:              *addr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	fmt.Fprintf(os.Stderr, "\n  Manhattan is serving on http://localhost%s\n\n", *addr)
	return h.ListenAndServe()
}

// loadArtifacts attaches benchmark output produced by a previous run, so the
// dashboard has the cases and the calibration sweep without re-running them.
func loadArtifacts(srv *server.Server, dir string) {
	read := func(name string, v any) bool {
		b, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return false
		}
		return json.Unmarshal(b, v) == nil
	}
	var (
		sum      bench.Summary
		cases    []bench.CaseOutcome
		sweep    []bench.SweepPoint
		envelope []bench.EnvelopePoint
	)
	haveSum := read("summary.json", &sum)
	read("cases.json", &cases)
	read("sweep.json", &sweep)
	read("envelope.json", &envelope)

	var p *bench.Summary
	if haveSum {
		p = &sum
	}
	srv.SetResults(p, cases, sweep, envelope)
}
