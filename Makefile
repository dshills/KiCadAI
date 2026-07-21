.DEFAULT_GOAL := help

.PHONY: help build install test lint coverage coverage-check run-help refresh-kicad-proto proto proto-check

BIN_DIR := $(CURDIR)/bin
BIN := $(BIN_DIR)/kicadai
GOCACHE_DIR := $(CURDIR)/.gocache
GOMODCACHE_DIR := $(CURDIR)/.gomodcache
GOLANGCI_LINT_CACHE := $(CURDIR)/.cache/golangci-lint
PATH_WITH_TOOLS := $(CURDIR)/bin:$(PATH)
COVER_DIR := $(CURDIR)/.coverage
COVER_PROFILE := $(COVER_DIR)/kicadai.cover.out
COVER_NOGEN_PROFILE := $(COVER_DIR)/kicadai.nogen.cover.out
COVER_NOGEN_TOTAL := $(COVER_DIR)/kicadai.nogen.total
GEN_COVER_EXCLUDE := (^|\/)internal\/kiapi\/gen\/
COVERAGE_THRESHOLD ?= 75.0
GO_TEST_TIMEOUT ?= 20m
COVER_TEST_FLAGS ?=

help:
	@printf "KiCadAI targets:\n"
	@printf "  make build           Build CLI binary to ./bin/kicadai\n"
	@printf "  make install         Install CLI binary to ./bin using go install\n"
	@printf "  make test            Run Go tests\n"
	@printf "  make lint            Run gofmt, go vet, and golangci-lint when installed\n"
	@printf "  make coverage        Generate coverage profiles\n"
	@printf "  make coverage-check  Enforce coverage threshold (COVERAGE_THRESHOLD=%s)\n" "$(COVERAGE_THRESHOLD)"
	@printf "  make run-help        Run kicadai --help from source\n"
	@printf "  make proto           Regenerate vendored KiCad protobuf bindings\n"
	@printf "  make proto-check     Regenerate protobuf bindings and check for diffs\n"

build:
	mkdir -p "$(BIN_DIR)"
	GOCACHE="$(GOCACHE_DIR)" GOMODCACHE="$(GOMODCACHE_DIR)" go build -o "$(BIN)" ./cmd/kicadai

install:
	GOCACHE="$(GOCACHE_DIR)" GOMODCACHE="$(GOMODCACHE_DIR)" go install ./cmd/kicadai

test:
	GOCACHE="$(GOCACHE_DIR)" GOMODCACHE="$(GOMODCACHE_DIR)" go test -timeout "$(GO_TEST_TIMEOUT)" ./...

lint:
	@unformatted="$$(gofmt -l $$(git ls-files '*.go'))"; \
	if [ -n "$$unformatted" ]; then \
		printf "gofmt required:\n%s\n" "$$unformatted" >&2; \
		exit 1; \
	fi
	GOCACHE="$(GOCACHE_DIR)" GOMODCACHE="$(GOMODCACHE_DIR)" go vet ./...
	@if command -v golangci-lint >/dev/null 2>&1; then \
		GOCACHE="$(GOCACHE_DIR)" GOMODCACHE="$(GOMODCACHE_DIR)" GOLANGCI_LINT_CACHE="$(GOLANGCI_LINT_CACHE)" golangci-lint run ./cmd/... ./internal/...; \
	else \
		printf "golangci-lint not installed; skipped optional lint pass\n"; \
	fi

coverage:
	mkdir -p "$(COVER_DIR)"
	rm -f "$(COVER_PROFILE)" "$(COVER_NOGEN_PROFILE)" "$(COVER_NOGEN_TOTAL)"
	GOCACHE="$(GOCACHE_DIR)" GOMODCACHE="$(GOMODCACHE_DIR)" go test $(COVER_TEST_FLAGS) -timeout "$(GO_TEST_TIMEOUT)" -coverprofile="$(COVER_PROFILE)" ./...
	awk 'NR == 1 || $$0 !~ /$(GEN_COVER_EXCLUDE)/' "$(COVER_PROFILE)" > "$(COVER_NOGEN_PROFILE)"
	@printf "\nRaw coverage including generated protobuf code:\n"
	@printf "Raw total: "
	@GOCACHE="$(GOCACHE_DIR)" GOMODCACHE="$(GOMODCACHE_DIR)" go tool cover -func="$(COVER_PROFILE)" | LC_ALL=C awk '/^total:/ { print $$NF }'
	@printf "\nCoverage excluding internal/kiapi/gen/**:\n"
	@set -e; \
	filtered="$$(GOCACHE="$(GOCACHE_DIR)" GOMODCACHE="$(GOMODCACHE_DIR)" go tool cover -func="$(COVER_NOGEN_PROFILE)" | LC_ALL=C awk '/^total:/ { print $$NF }')"; \
	if [ -z "$$filtered" ]; then \
		printf "failed to calculate generated-excluded coverage\n" >&2; \
		exit 1; \
	fi; \
	printf "Generated-excluded total: %s\n" "$$filtered"; \
	printf "%s\n" "$$filtered" > "$(COVER_NOGEN_TOTAL)"
	@printf "\nProfiles:\n  %s\n  %s\n" "$(COVER_PROFILE)" "$(COVER_NOGEN_PROFILE)"

coverage-check: coverage
	@actual="$$(LC_ALL=C awk '{ sub(/%/, "", $$1); print $$1 }' "$(COVER_NOGEN_TOTAL)")"; \
	if [ -z "$$actual" ]; then \
		printf "failed to read generated-excluded coverage total\n" >&2; \
		exit 1; \
	fi; \
	LC_ALL=C awk -v actual="$$actual" -v threshold="$(COVERAGE_THRESHOLD)" 'BEGIN { \
		if (actual + 0 < threshold + 0) { \
			printf("coverage %.2f%% below threshold %.2f%%\n", actual, threshold); \
			exit 1; \
		} \
		printf("coverage %.2f%% meets threshold %.2f%%\n", actual, threshold); \
	}'

run-help:
	GOCACHE="$(GOCACHE_DIR)" GOMODCACHE="$(GOMODCACHE_DIR)" go run ./cmd/kicadai --help

refresh-kicad-proto:
	./scripts/refresh-kicad-proto.sh

proto:
	PATH="$(PATH_WITH_TOOLS)" ./scripts/generate-proto.sh

proto-check: proto
	git diff --exit-code -- internal/kiapi/gen
