.DEFAULT_GOAL := help

.PHONY: help build install release release-smoke release-reproducibility test test-fast test-bounded test-exhaustive test-one race-short performance-report performance-scaling public-demo public-demo-refusal review-matrix promotion-bundle held-out-promotion-bundle hierarchical-promotion-bundle dynamic-electrothermal-promotion-bundle open-world-capability-promotion-bundle protocol-aware-bus-promotion-bundle component-onboarding-promotion-bundle lint coverage coverage-monolithic coverage-shard coverage-merge coverage-report coverage-threshold coverage-check coverage-merge-check run-help refresh-kicad-proto proto proto-check

BIN_DIR := $(CURDIR)/bin
BIN := $(BIN_DIR)/kicadai
INSTALL_DIR ?= $(HOME)/.local/bin
DIST_DIR ?= $(CURDIR)/dist
VERSION ?= v$(shell tr -d '[:space:]' < VERSION)
COMMIT ?= $(shell git rev-parse HEAD)
BUILD_DATE ?= $(shell git show -s --format=%cI $(COMMIT))
RELEASE_BINARY ?= $(DIST_DIR)/kicadai_$(VERSION)_$(shell go env GOOS)_$(shell go env GOARCH)
GO_CACHE_ROOT ?= $(CURDIR)/.cache/go
GOCACHE_DIR := $(GO_CACHE_ROOT)/build
GOMODCACHE_DIR := $(GO_CACHE_ROOT)/mod
GOLANGCI_LINT_CACHE := $(GO_CACHE_ROOT)/golangci-lint
PATH_WITH_TOOLS := $(CURDIR)/bin:$(PATH)
COVER_DIR := $(CURDIR)/.coverage
COVER_PROFILE := $(COVER_DIR)/kicadai.cover.out
COVER_NOGEN_PROFILE := $(COVER_DIR)/kicadai.nogen.cover.out
COVER_NOGEN_TOTAL := $(COVER_DIR)/kicadai.nogen.total
COVER_SHARD_DIR ?= $(COVER_DIR)/shards
COVER_PROFILE_SUMMARY ?= $(COVER_DIR)/profile.tsv
COVERAGE_PROOF_CACHE ?= $(CURDIR)/.cache/coverage-proofs
COVERAGE_GENERAL_SHARDS ?= 4
COVERAGE_OPEN_TOPOLOGY_SHARDS ?= 6
COVERAGE_MAX_WORKERS ?= 4
COVERAGE_SHARD_GROUP ?=
COVERAGE_SHARD_INDEX ?=
COVERAGE_SHARD_TOTAL ?=
COVERAGE_SHARD_OUTPUT ?=
COVER_TEST_SKIP ?=
COVERAGE_REFERENCE_PROFILE ?=
COVER_MONOLITHIC_PROFILE ?= $(COVER_DIR)/kicadai.monolithic.cover.out
GEN_COVER_EXCLUDE := (^|\/)internal\/kiapi\/gen\/
COVERAGE_THRESHOLD ?= 75.0
GO_TEST_TIMEOUT ?= 20m
GO_TEST_FLAGS ?=
GO_TEST_PACKAGE_PARALLELISM ?= 1
FAST_TEST_SKIP ?= ^(TestWindowedHeatingPowerPassesElectricalAndSafetyCorners|TestPowerTransferCompoundFollowerReachesBoundedAudioEnvelope|TestPowerTransferCandidatesReachTrustedSimulation|TestCurrentLimitedSwitchRelationshipIsRetainedByDefaultSearch|TestDefaultSearchCurrentLimitedFirstTrialIsPhysicallyReady|TestProtectedCurrentDriverRepairTraceReplaysAndFailsClosedPrecisely)$$
GO_TEST_PACKAGE ?= ./...
GO_TEST_NAME ?=
PERFORMANCE_BENCHTIME ?= 500ms
PERFORMANCE_COUNT ?= 3
RACE_PACKAGES := ./internal/ipc ./internal/atomicfile ./internal/aiprovider ./internal/transactions ./internal/runtimebudget ./internal/simmodel ./internal/closedloopsynthesis ./internal/libraryresolver ./internal/promotionrunner ./internal/fabrication
PROMOTION_ROOT ?= $(CURDIR)/.tmp/clean-checkout-promotion
PROMOTION_CACHE_DIR ?= $(CURDIR)/.cache/kicadai-promotion-toolchain
PROMOTION_SCENARIO_TIMEOUT ?= 20m
PROMOTION_MAX_CONCURRENT_SCENARIOS ?= 2
export PROMOTION_MAX_CONCURRENT_SCENARIOS
HELD_OUT_PROMOTION_ROOT ?= $(CURDIR)/.tmp/held-out-capability-promotion
HELD_OUT_PROMOTION_MATRIX ?= $(CURDIR)/specs/held-out-capability-expansion/PROMOTION_MATRIX.json
HIERARCHICAL_PROMOTION_ROOT ?= $(CURDIR)/.tmp/hierarchical-multi-domain-promotion
HIERARCHICAL_PROMOTION_MATRIX ?= $(CURDIR)/specs/hierarchical-multi-domain-synthesis/PROMOTION_MATRIX.json
DYNAMIC_ELECTROTHERMAL_PROMOTION_ROOT ?= $(CURDIR)/.tmp/dynamic-electrothermal-promotion
DYNAMIC_ELECTROTHERMAL_PROMOTION_MATRIX ?= $(CURDIR)/specs/dynamic-electrothermal-control-loop-synthesis/PROMOTION_MATRIX.json
OPEN_WORLD_CAPABILITY_PROMOTION_ROOT ?= $(CURDIR)/.tmp/open-world-capability-promotion
OPEN_WORLD_CAPABILITY_PROMOTION_MATRIX ?= $(CURDIR)/specs/open-world-capability-evaluation/PROMOTION_MATRIX.json
PROTOCOL_AWARE_BUS_PROMOTION_ROOT ?= $(CURDIR)/.tmp/protocol-aware-bus-promotion
PROTOCOL_AWARE_BUS_PROMOTION_MATRIX ?= $(CURDIR)/specs/protocol-aware-bus-synthesis/PROMOTION_MATRIX.json
COMPONENT_ONBOARDING_PROMOTION_ROOT ?= $(CURDIR)/.tmp/component-onboarding-promotion

help:
	@printf "KiCadAI targets:\n"
	@printf "  make build           Build CLI binary to ./bin/kicadai\n"
	@printf "  make install         Install CLI binary to %s\n" "$(INSTALL_DIR)"
	@printf "  make release         Build versioned macOS/Linux release artifacts\n"
	@printf "  make release-smoke   Smoke-test one host-compatible release binary\n"
	@printf "  make release-reproducibility Verify two release builds are byte-identical\n"
	@printf "  make test            Run Go tests\n"
	@printf "  make test-fast       Run the short developer-feedback tier\n"
	@printf "  make test-bounded    Run the bounded local/CI tier\n"
	@printf "  make test-exhaustive Run the complete release-verification tier\n"
	@printf "  make test-one        Run and require one named Go test (GO_TEST_NAME=...)\n"
	@printf "  make race-short      Run the local bounded concurrency race suite\n"
	@printf "  make performance-report Run representative processing benchmarks\n"
	@printf "  make performance-scaling Compare synthesis with 1/2/4/8/default workers\n"
	@printf "  make public-demo     Reproduce the featured protected-current-output proof\n"
	@printf "  make public-demo-refusal Verify fail-closed behavior outside the reviewed envelope\n"
	@printf "  make review-matrix   Run the external-review mitigation ladder twice\n"
	@printf "  make promotion-bundle Reproduce and verify the installed-KiCad promotion bundle\n"
	@printf "  make held-out-promotion-bundle Reproduce and verify the held-out capability bundle\n"
	@printf "  make hierarchical-promotion-bundle Reproduce and verify the hierarchical multi-domain bundle\n"
	@printf "  make dynamic-electrothermal-promotion-bundle Reproduce and verify the dynamic electrothermal bundle\n"
	@printf "  make open-world-capability-promotion-bundle Reproduce and verify the open-world capability bundle\n"
	@printf "  make protocol-aware-bus-promotion-bundle Reproduce and verify the protocol-aware bus bundle\n"
	@printf "  make component-onboarding-promotion-bundle Reproduce unfamiliar-part onboarding in two clean roots\n"
	@printf "  make lint            Run gofmt, go vet, and golangci-lint when installed\n"
	@printf "  make coverage        Generate coverage profiles\n"
	@printf "  make coverage-check  Enforce coverage threshold (COVERAGE_THRESHOLD=%s)\n" "$(COVERAGE_THRESHOLD)"
	@printf "  make run-help        Run kicadai --help from source\n"
	@printf "  make proto           Regenerate vendored KiCad protobuf bindings\n"
	@printf "  make proto-check     Regenerate protobuf bindings and check for diffs\n"

# Ordinary source builds intentionally retain the "dev" application version
# and Go-embedded VCS/dirty metadata. Only build-release.sh may stamp an
# official VERSION identity into a binary.
build:
	mkdir -p "$(BIN_DIR)"
	GOCACHE="$(GOCACHE_DIR)" GOMODCACHE="$(GOMODCACHE_DIR)" go build -o "$(BIN)" ./cmd/kicadai

install: build
	mkdir -p "$(INSTALL_DIR)"
	install -m 0755 "$(BIN)" "$(INSTALL_DIR)/kicadai"

release:
	VERSION="$(VERSION)" COMMIT="$(COMMIT)" BUILD_DATE="$(BUILD_DATE)" OUTPUT_DIR="$(DIST_DIR)" \
	GOCACHE="$(GOCACHE_DIR)" GOMODCACHE="$(GOMODCACHE_DIR)" ./scripts/build-release.sh

release-smoke:
	./scripts/release-smoke-test.sh "$(RELEASE_BINARY)"

release-reproducibility:
	VERSION="$(VERSION)" COMMIT="$(COMMIT)" BUILD_DATE="$(BUILD_DATE)" \
	GOCACHE="$(GOCACHE_DIR)" GOMODCACHE="$(GOMODCACHE_DIR)" ./scripts/verify-release-reproducibility.sh

test:
	GOCACHE="$(GOCACHE_DIR)" GOMODCACHE="$(GOMODCACHE_DIR)" go test -short -p="$(GO_TEST_PACKAGE_PARALLELISM)" $(GO_TEST_FLAGS) -timeout "$(GO_TEST_TIMEOUT)" ./...

test-fast:
	GOCACHE="$(GOCACHE_DIR)" GOMODCACHE="$(GOMODCACHE_DIR)" go test -short -p="$(GO_TEST_PACKAGE_PARALLELISM)" -skip="$(FAST_TEST_SKIP)" $(GO_TEST_FLAGS) -timeout "$(GO_TEST_TIMEOUT)" ./...

test-bounded:
	GOCACHE="$(GOCACHE_DIR)" GOMODCACHE="$(GOMODCACHE_DIR)" go test -short -p="$(GO_TEST_PACKAGE_PARALLELISM)" $(GO_TEST_FLAGS) -timeout "$(GO_TEST_TIMEOUT)" ./...

test-exhaustive:
	GOCACHE="$(GOCACHE_DIR)" GOMODCACHE="$(GOMODCACHE_DIR)" go test -p="$(GO_TEST_PACKAGE_PARALLELISM)" $(GO_TEST_FLAGS) -timeout "$(GO_TEST_TIMEOUT)" ./...

test-one:
	@if [ -z "$(GO_TEST_NAME)" ]; then \
		printf "GO_TEST_NAME is required\n" >&2; \
		exit 2; \
	fi

	@set +e; \
	output="$$(GOCACHE="$(GOCACHE_DIR)" GOMODCACHE="$(GOMODCACHE_DIR)" go test $(GO_TEST_FLAGS) -timeout "$(GO_TEST_TIMEOUT)" "$(GO_TEST_PACKAGE)" -run '^$(GO_TEST_NAME)$$' -count=1 -v 2>&1)"; \
	status=$$?; \
	printf "%s\n" "$$output"; \
	if [ "$$status" -ne 0 ]; then \
		exit "$$status"; \
	fi; \
	if ! printf "%s\n" "$$output" | grep -Fq -- "--- PASS: $(GO_TEST_NAME) "; then \
		printf "named test did not run and pass: %s\n" "$(GO_TEST_NAME)" >&2; \
		exit 1; \
	fi

race-short:
	GOCACHE="$(GOCACHE_DIR)" GOMODCACHE="$(GOMODCACHE_DIR)" go test -race -short -count=1 -timeout "$(GO_TEST_TIMEOUT)" $(RACE_PACKAGES)

performance-report:
	GOCACHE="$(GOCACHE_DIR)" GOMODCACHE="$(GOMODCACHE_DIR)" go test -run '^$$' -bench '^(BenchmarkRouteRequestGoldenDetour|BenchmarkPlaceModerateBoard|BenchmarkWriteProjectDirectory|BenchmarkCompareProjects)$$' -benchmem -benchtime "$(PERFORMANCE_BENCHTIME)" -count "$(PERFORMANCE_COUNT)" ./internal/routing ./internal/placement ./internal/kicadfiles/design ./internal/promotionrunner
	GOCACHE="$(GOCACHE_DIR)" GOMODCACHE="$(GOMODCACHE_DIR)" go test -run '^$$' -bench '^(BenchmarkEvaluateTransientSwitch|BenchmarkResistorPathWithin|BenchmarkSynthesizePoweredLowpass)$$' -benchmem -benchtime 1x -count 1 ./internal/simmodel ./internal/opentopologysynthesis

performance-scaling:
	@set -e; \
	for workers in 1 2 4 8; do \
		printf '\nKICADAI_MAX_WORKERS=%s\n' "$$workers"; \
		KICADAI_MAX_WORKERS="$$workers" GOCACHE="$(GOCACHE_DIR)" GOMODCACHE="$(GOMODCACHE_DIR)" go test -run '^$$' -bench '^BenchmarkSynthesizePoweredLowpass$$' -benchmem -benchtime 1x -count 1 ./internal/opentopologysynthesis; \
	done; \
	printf '\nKICADAI_MAX_WORKERS=default\n'; \
	env -u KICADAI_MAX_WORKERS GOCACHE="$(GOCACHE_DIR)" GOMODCACHE="$(GOMODCACHE_DIR)" go test -run '^$$' -bench '^BenchmarkSynthesizePoweredLowpass$$' -benchmem -benchtime 1x -count 1 ./internal/opentopologysynthesis

public-demo:
	./examples/public-demo/protected-programmable-current-output/run.sh positive

public-demo-refusal:
	./examples/public-demo/protected-programmable-current-output/run.sh refusal

review-matrix:
	KICADAI_RUN_EXTERNAL_REVIEW_MATRIX=1 GOCACHE="$(GOCACHE_DIR)" GOMODCACHE="$(GOMODCACHE_DIR)" go test -timeout "$(GO_TEST_TIMEOUT)" -count=2 ./cmd/kicadai ./internal/placement ./internal/circuitgraph ./internal/designworkflow ./internal/creationevidence -run '^TestExternalReviewMatrix'

promotion-bundle:
	PROMOTION_ROOT="$(PROMOTION_ROOT)" \
	PROMOTION_CACHE_DIR="$(PROMOTION_CACHE_DIR)" \
	PROMOTION_SCENARIO_TIMEOUT="$(PROMOTION_SCENARIO_TIMEOUT)" \
	GOCACHE="$(GOCACHE_DIR)" \
	GOMODCACHE="$(GOMODCACHE_DIR)" \
	./scripts/clean-checkout-promotion.sh

held-out-promotion-bundle:
	PROMOTION_ROOT="$(HELD_OUT_PROMOTION_ROOT)" \
	PROMOTION_MATRIX="$(HELD_OUT_PROMOTION_MATRIX)" \
	PROMOTION_CACHE_DIR="$(PROMOTION_CACHE_DIR)" \
	PROMOTION_SCENARIO_TIMEOUT="$(PROMOTION_SCENARIO_TIMEOUT)" \
	GOCACHE="$(GOCACHE_DIR)" \
	GOMODCACHE="$(GOMODCACHE_DIR)" \
	./scripts/clean-checkout-promotion.sh

hierarchical-promotion-bundle:
	PROMOTION_ROOT="$(HIERARCHICAL_PROMOTION_ROOT)" \
	PROMOTION_MATRIX="$(HIERARCHICAL_PROMOTION_MATRIX)" \
	PROMOTION_CACHE_DIR="$(PROMOTION_CACHE_DIR)" \
	PROMOTION_SCENARIO_TIMEOUT="$(PROMOTION_SCENARIO_TIMEOUT)" \
	GOCACHE="$(GOCACHE_DIR)" \
	GOMODCACHE="$(GOMODCACHE_DIR)" \
	./scripts/clean-checkout-promotion.sh

dynamic-electrothermal-promotion-bundle:
	PROMOTION_ROOT="$(DYNAMIC_ELECTROTHERMAL_PROMOTION_ROOT)" \
	PROMOTION_MATRIX="$(DYNAMIC_ELECTROTHERMAL_PROMOTION_MATRIX)" \
	PROMOTION_CACHE_DIR="$(PROMOTION_CACHE_DIR)" \
	PROMOTION_SCENARIO_TIMEOUT="$(PROMOTION_SCENARIO_TIMEOUT)" \
	GOCACHE="$(GOCACHE_DIR)" \
	GOMODCACHE="$(GOMODCACHE_DIR)" \
	./scripts/clean-checkout-promotion.sh

open-world-capability-promotion-bundle:
	PROMOTION_ROOT="$(OPEN_WORLD_CAPABILITY_PROMOTION_ROOT)" \
	PROMOTION_MATRIX="$(OPEN_WORLD_CAPABILITY_PROMOTION_MATRIX)" \
	PROMOTION_CACHE_DIR="$(PROMOTION_CACHE_DIR)" \
	PROMOTION_SCENARIO_TIMEOUT="$(PROMOTION_SCENARIO_TIMEOUT)" \
	GOCACHE="$(GOCACHE_DIR)" \
	GOMODCACHE="$(GOMODCACHE_DIR)" \
	./scripts/clean-checkout-promotion.sh

protocol-aware-bus-promotion-bundle:
	PROMOTION_ROOT="$(PROTOCOL_AWARE_BUS_PROMOTION_ROOT)" \
	PROMOTION_MATRIX="$(PROTOCOL_AWARE_BUS_PROMOTION_MATRIX)" \
	PROMOTION_CACHE_DIR="$(PROMOTION_CACHE_DIR)" \
	PROMOTION_SCENARIO_TIMEOUT="$(PROMOTION_SCENARIO_TIMEOUT)" \
	GOCACHE="$(GOCACHE_DIR)" \
	GOMODCACHE="$(GOMODCACHE_DIR)" \
	./scripts/clean-checkout-promotion.sh

component-onboarding-promotion-bundle:
	COMPONENT_ONBOARDING_PROMOTION_ROOT="$(COMPONENT_ONBOARDING_PROMOTION_ROOT)" \
	GOCACHE="$(GOCACHE_DIR)" \
	GOMODCACHE="$(GOMODCACHE_DIR)" \
	./scripts/component-onboarding-promotion.sh

lint:
	./scripts/check-go-format.sh
	GOCACHE="$(GOCACHE_DIR)" GOMODCACHE="$(GOMODCACHE_DIR)" go vet ./...
	@if command -v golangci-lint >/dev/null 2>&1; then \
		GOCACHE="$(GOCACHE_DIR)" GOMODCACHE="$(GOMODCACHE_DIR)" GOLANGCI_LINT_CACHE="$(GOLANGCI_LINT_CACHE)" golangci-lint run ./cmd/... ./internal/...; \
	else \
		printf "golangci-lint not installed; skipped optional lint pass\n"; \
	fi

coverage:
	mkdir -p "$(COVER_DIR)"
	rm -f "$(COVER_PROFILE)" "$(COVER_NOGEN_PROFILE)" "$(COVER_NOGEN_TOTAL)"
	GOCACHE="$(GOCACHE_DIR)" GOMODCACHE="$(GOMODCACHE_DIR)" \
		GO_TEST_TIMEOUT="$(GO_TEST_TIMEOUT)" COVER_TEST_SKIP="$(COVER_TEST_SKIP)" \
		COVER_SHARD_DIR="$(COVER_SHARD_DIR)" COVER_PROFILE="$(COVER_PROFILE)" \
		COVER_PROFILE_SUMMARY="$(COVER_PROFILE_SUMMARY)" COVERAGE_PROOF_CACHE="$(COVERAGE_PROOF_CACHE)" \
		COVERAGE_GENERAL_SHARDS="$(COVERAGE_GENERAL_SHARDS)" \
		COVERAGE_OPEN_TOPOLOGY_SHARDS="$(COVERAGE_OPEN_TOPOLOGY_SHARDS)" \
		COVERAGE_MAX_WORKERS="$(COVERAGE_MAX_WORKERS)" ./scripts/run-coverage-suite.sh
	@if [ -n "$(COVERAGE_REFERENCE_PROFILE)" ]; then \
		./scripts/compare-coverprofiles.sh "$(COVERAGE_REFERENCE_PROFILE)" "$(COVER_PROFILE)"; \
	fi

coverage-monolithic:
	mkdir -p "$(COVER_DIR)"
	rm -f "$(COVER_MONOLITHIC_PROFILE)"
	GOCACHE="$(GOCACHE_DIR)" GOMODCACHE="$(GOMODCACHE_DIR)" go test -short -count=1 -p=1 \
		$(if $(COVER_TEST_SKIP),-skip="$(COVER_TEST_SKIP)",) -timeout "$(GO_TEST_TIMEOUT)" \
		-covermode=set -coverprofile="$(COVER_MONOLITHIC_PROFILE)" ./...

coverage-shard:
	@if [ -z "$(COVERAGE_SHARD_GROUP)" ] || [ -z "$(COVERAGE_SHARD_INDEX)" ] || [ -z "$(COVERAGE_SHARD_TOTAL)" ] || [ -z "$(COVERAGE_SHARD_OUTPUT)" ]; then \
		printf 'COVERAGE_SHARD_GROUP, COVERAGE_SHARD_INDEX, COVERAGE_SHARD_TOTAL, and COVERAGE_SHARD_OUTPUT are required\n' >&2; \
		exit 2; \
	fi
	GOCACHE="$(GOCACHE_DIR)" GOMODCACHE="$(GOMODCACHE_DIR)" \
		GO_TEST_TIMEOUT="$(GO_TEST_TIMEOUT)" COVER_TEST_SKIP="$(COVER_TEST_SKIP)" \
		COVERAGE_PROOF_CACHE="$(COVERAGE_PROOF_CACHE)" ./scripts/run-coverage-shard.sh \
		"$(COVERAGE_SHARD_GROUP)" "$(COVERAGE_SHARD_INDEX)" "$(COVERAGE_SHARD_TOTAL)" "$(COVERAGE_SHARD_OUTPUT)"

coverage-merge:
	COVER_TEST_SKIP="$(COVER_TEST_SKIP)" \
		COVERAGE_GENERAL_SHARDS="$(COVERAGE_GENERAL_SHARDS)" \
		COVERAGE_OPEN_TOPOLOGY_SHARDS="$(COVERAGE_OPEN_TOPOLOGY_SHARDS)" \
		./scripts/merge-coverage-shards.sh "$(COVER_SHARD_DIR)" "$(COVER_PROFILE)"
	./scripts/summarize-coverage-profile.sh "$(COVER_SHARD_DIR)" "$(COVER_PROFILE_SUMMARY)"

coverage-report:
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
	@printf "\nProfiles:\n  %s\n  %s\n  %s\n" "$(COVER_PROFILE)" "$(COVER_NOGEN_PROFILE)" "$(COVER_PROFILE_SUMMARY)"

coverage-threshold:
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

coverage-check: coverage coverage-report coverage-threshold

coverage-merge-check: coverage-merge coverage-report coverage-threshold

run-help:
	GOCACHE="$(GOCACHE_DIR)" GOMODCACHE="$(GOMODCACHE_DIR)" go run ./cmd/kicadai --help

refresh-kicad-proto:
	./scripts/refresh-kicad-proto.sh

proto:
	PATH="$(PATH_WITH_TOOLS)" ./scripts/generate-proto.sh

proto-check: proto
	git diff --exit-code -- internal/kiapi/gen
