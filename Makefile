.PHONY: test run-help refresh-kicad-proto proto proto-check

GOCACHE_DIR := $(CURDIR)/.gocache
PATH_WITH_TOOLS := $(CURDIR)/bin:$(PATH)

test:
	GOCACHE=$(GOCACHE_DIR) go test ./...

run-help:
	GOCACHE=$(GOCACHE_DIR) go run ./cmd/kicadai --help

refresh-kicad-proto:
	./scripts/refresh-kicad-proto.sh

proto:
	PATH="$(PATH_WITH_TOOLS)" ./scripts/generate-proto.sh

proto-check: proto
	git diff --exit-code -- internal/kiapi/gen
