#!/usr/bin/env bash
# Manhattan, without make.
#
# The Makefile is the reference, but make is not installed everywhere and a
# demo that will not start is worth nothing. This script does the same things.
#
#   ./run.sh demo     build, benchmark, serve the dashboard
#   ./run.sh bench    benchmark and regenerate RESULTS.md
#   ./run.sh cases    the eleven adversarial cases, head to head
#   ./run.sh test     the full test suite
#   ./run.sh serve    serve a previous run
set -euo pipefail
cd "$(dirname "$0")"

BIN=bin/manhattan
[[ "${OS:-}" == "Windows_NT" ]] && BIN=bin/manhattan.exe
N=${N:-500}
SEED=${SEED:-20260826}
ADDR=${ADDR:-:8080}

build_web() {
  if command -v npm >/dev/null 2>&1; then
    (cd web && npm install --silent && npm run build)
  else
    echo "npm not found, serving the previously built dashboard" >&2
  fi
}

build_go() { go build -trimpath -o "$BIN" ./cmd/manhattan; }

case "${1:-demo}" in
  demo)
    build_web; build_go
    "./$BIN" bench -n "$N" -seed "$SEED" -out out
    echo; echo "  Dashboard on http://localhost${ADDR}. Start at the head-to-head tab."; echo
    "./$BIN" serve -addr "$ADDR" -store out
    ;;
  bench)  build_go; "./$BIN" bench -n "$N" -seed "$SEED" -out out ;;
  cases)  build_go; "./$BIN" cases -out out ;;
  recon)  build_go; "./$BIN" recon -n 12 -archetype travel ;;
  serve)  build_go; "./$BIN" serve -addr "$ADDR" -store out ;;
  web)    build_web ;;
  docs)   build_go; "./$BIN" docs -in out ;;
  diagrams)
    # Extract every Mermaid block from the markdown and render it to SVG, so a
    # judging surface that does not render Mermaid still sees the diagrams and
    # a rendered file cannot drift from the document it came from.
    python3 tools/extract_diagrams.py
    for f in docs/diagrams/*.mmd; do
      npx --yes @mermaid-js/mermaid-cli@11 -i "$f" -o "${f%.mmd}.svg"         -c docs/diagrams/mermaid.json -b '#faf6ee' >/dev/null
    done
    echo "rendered $(ls docs/diagrams/*.svg | wc -l) diagrams"
    ;;
  test)   go test ./... -count=1 ;;
  perf)   go test ./internal/solver/ -run TestPerformanceGate -v -count=1 ;;
  *) echo "usage: ./run.sh [demo|bench|cases|recon|serve|web|docs|diagrams|test|perf]" >&2; exit 2 ;;
esac
