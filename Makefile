.PHONY: test coverage coverage-check run-help refresh-kicad-proto proto proto-check

GOCACHE_DIR := $(CURDIR)/.gocache
GOMODCACHE_DIR := $(CURDIR)/.gomodcache
PATH_WITH_TOOLS := $(CURDIR)/bin:$(PATH)
COVER_DIR := $(CURDIR)/.coverage
COVER_PROFILE := $(COVER_DIR)/kicadai.cover.out
COVER_NOGEN_PROFILE := $(COVER_DIR)/kicadai.nogen.cover.out
COVER_NOGEN_TOTAL := $(COVER_DIR)/kicadai.nogen.total
GEN_COVER_EXCLUDE := (^|\/)internal\/kiapi\/gen\/
COVERAGE_THRESHOLD ?= 75.0

test:
	GOCACHE="$(GOCACHE_DIR)" GOMODCACHE="$(GOMODCACHE_DIR)" go test ./...

coverage:
	mkdir -p "$(COVER_DIR)"
	rm -f "$(COVER_PROFILE)" "$(COVER_NOGEN_PROFILE)" "$(COVER_NOGEN_TOTAL)"
	GOCACHE="$(GOCACHE_DIR)" GOMODCACHE="$(GOMODCACHE_DIR)" go test ./... -coverprofile="$(COVER_PROFILE)"
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
