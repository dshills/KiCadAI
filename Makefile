.PHONY: test run-help

GOCACHE_DIR := $(CURDIR)/.gocache

test:
	GOCACHE=$(GOCACHE_DIR) go test ./...

run-help:
	GOCACHE=$(GOCACHE_DIR) go run ./cmd/kicadai --help
