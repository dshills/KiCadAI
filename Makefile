.PHONY: test run-help refresh-kicad-proto

GOCACHE_DIR := $(CURDIR)/.gocache

test:
	GOCACHE=$(GOCACHE_DIR) go test ./...

run-help:
	GOCACHE=$(GOCACHE_DIR) go run ./cmd/kicadai --help

refresh-kicad-proto:
	./scripts/refresh-kicad-proto.sh
