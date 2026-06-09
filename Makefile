.PHONY: test coverage run-help refresh-kicad-proto proto proto-check

GOCACHE_DIR := $(CURDIR)/.gocache
GOMODCACHE_DIR := $(CURDIR)/.gomodcache
PATH_WITH_TOOLS := $(CURDIR)/bin:$(PATH)
COVER_DIR := $(CURDIR)/.coverage
COVER_PROFILE := $(COVER_DIR)/kicadai.cover.out
COVER_NOGEN_PROFILE := $(COVER_DIR)/kicadai.nogen.cover.out
GEN_COVER_EXCLUDE := (^|\/)internal\/kiapi\/gen\/

test:
	GOCACHE="$(GOCACHE_DIR)" GOMODCACHE="$(GOMODCACHE_DIR)" go test ./...

coverage:
	mkdir -p "$(COVER_DIR)"
	rm -f "$(COVER_PROFILE)" "$(COVER_NOGEN_PROFILE)"
	GOCACHE="$(GOCACHE_DIR)" GOMODCACHE="$(GOMODCACHE_DIR)" go test ./... -coverprofile="$(COVER_PROFILE)"
	awk 'NR == 1 || $$0 !~ /$(GEN_COVER_EXCLUDE)/' "$(COVER_PROFILE)" > "$(COVER_NOGEN_PROFILE)"
	@printf "\nRaw coverage including generated protobuf code:\n"
	go tool cover -func="$(COVER_PROFILE)" | grep '^total:'
	@printf "\nCoverage excluding internal/kiapi/gen/**:\n"
	go tool cover -func="$(COVER_NOGEN_PROFILE)" | grep '^total:'
	@printf "\nProfiles:\n  %s\n  %s\n" "$(COVER_PROFILE)" "$(COVER_NOGEN_PROFILE)"

run-help:
	GOCACHE="$(GOCACHE_DIR)" GOMODCACHE="$(GOMODCACHE_DIR)" go run ./cmd/kicadai --help

refresh-kicad-proto:
	./scripts/refresh-kicad-proto.sh

proto:
	PATH="$(PATH_WITH_TOOLS)" ./scripts/generate-proto.sh

proto-check: proto
	git diff --exit-code -- internal/kiapi/gen
