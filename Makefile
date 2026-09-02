# Manhattan
#
# One command reproduces every number in the submission:
#
#     make bench
#
# It is seeded and deterministic. Run it twice and the receipts are
# byte-identical, which is the minimum bar for a reconciliation system: a
# decision that cannot be reproduced cannot be audited.

BIN      := bin/manhattan
OUT      := out
SEED     ?= 20260826
N        ?= 500
ADDR     ?= :8080

GO       ?= go
NPM      ?= npm

ifeq ($(OS),Windows_NT)
	BIN := bin/manhattan.exe
endif

.DEFAULT_GOAL := help

.PHONY: help
help:
	@echo ""
	@echo "  Manhattan - an agent that proves settlements instead of guessing them"
	@echo ""
	@echo "  make demo       build everything, run the benchmark, serve the dashboard"
	@echo "  make bench      run the benchmark and regenerate RESULTS.md"
	@echo "  make cases      run the eleven adversarial cases head to head against B0"
	@echo "  make serve      serve the dashboard and API on $(ADDR)"
	@echo "  make dev        API on :8080, interface on Vite :5173 with hot reload"
	@echo "  make test       the full test suite, including the brute-force oracle"
	@echo "  make perf       the solver performance gate and benchmarks"
	@echo "  make build      compile the binary with the dashboard embedded"
	@echo "  make web        build the dashboard only"
	@echo "  make clean      remove build output and generated results"
	@echo ""
	@echo "  No API key is needed. Without one the agent runs on a deterministic"
	@echo "  offline stub, which changes how often an exception can be cleared and"
	@echo "  cannot change whether a posting is correct."
	@echo ""

# ---------------------------------------------------------------- build ----

.PHONY: web
web:
	cd web && $(NPM) install --silent && $(NPM) run build

.PHONY: build
build: web
	$(GO) build -trimpath -o $(BIN) ./cmd/manhattan

# Compiling without rebuilding the dashboard, for the inner loop.
.PHONY: build-go
build-go:
	$(GO) build -trimpath -o $(BIN) ./cmd/manhattan

# ----------------------------------------------------------------- run -----

.PHONY: demo
demo: build bench
	@echo ""
	@echo "  Benchmark complete and RESULTS.md written."
	@echo "  Opening the dashboard. Start at the head-to-head tab."
	@echo ""
	./$(BIN) serve -addr $(ADDR) -store $(OUT)

.PHONY: bench
bench: build-go
	./$(BIN) bench -n $(N) -seed $(SEED) -out $(OUT)

# The same benchmark with a deliberately stale narrowing baseline, so the
# run-level drift monitor fires and gates the batch.
.PHONY: bench-drift
bench-drift: build-go
	./$(BIN) bench -n $(N) -seed $(SEED) -out $(OUT) -demo-drift

.PHONY: cases
cases: build-go
	./$(BIN) cases -out $(OUT)

.PHONY: recon
recon: build-go
	./$(BIN) recon -n 12 -archetype travel

.PHONY: serve
serve: build-go
	./$(BIN) serve -addr $(ADDR) -store $(OUT)

# API on 8080, interface on Vite with hot reload proxied to it.
.PHONY: dev
dev: build-go
	@echo "API on http://localhost:8080, interface on http://localhost:5173"
	./$(BIN) serve -addr :8080 -store $(OUT) & \
	cd web && $(NPM) run dev

.PHONY: ask
ask: build-go
	./$(BIN) ask -store $(OUT) "$(Q)"

# ---------------------------------------------------------------- test -----

.PHONY: test
test:
	$(GO) test ./... -count=1

.PHONY: test-short
test-short:
	$(GO) test ./... -count=1 -short

# The day-three gate. Every published timing assumes the array-shaped,
# flat-slice enumeration; a structurally identical implementation built out of
# per-entry objects is correct and one to two orders of magnitude slower, at
# which point the whole resource envelope becomes fiction.
.PHONY: perf
perf:
	$(GO) test ./internal/solver/ -run TestPerformanceGate -v -count=1
	$(GO) test ./internal/solver/ -bench . -benchtime 3x -run XXX

.PHONY: vet
vet:
	$(GO) vet ./...
	gofmt -l . | tee /dev/stderr | (! read)

# --------------------------------------------------------------- tidy ------

.PHONY: clean
clean:
	rm -rf bin $(OUT) RESULTS.md cmd/manhattan/dist/assets web/node_modules
