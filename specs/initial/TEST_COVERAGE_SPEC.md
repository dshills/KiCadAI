# Test Coverage Specification

## Purpose

Define the expected test coverage posture for KiCadAI after the initial KiCad IPC, schematic planning, and AI-safe workflow boundary work. The goal is not to chase a single global percentage, because generated protobuf bindings dominate raw totals, but to make coverage meaningful for the hand-written behavior that carries project risk.

## Current Baseline

Measured with:

```sh
go test ./... -cover
mkdir -p .coverage
go test ./... -coverprofile=.coverage/kicadai.cover.out
awk 'NR==1 || $0 !~ /(^|\/)internal\/kiapi\/gen\//' .coverage/kicadai.cover.out > .coverage/kicadai.nogen.cover.out
go tool cover -func=.coverage/kicadai.nogen.cover.out
```

Current package-local coverage from `go test ./... -cover`:

```text
cmd/kicadai             57.5%
internal/config        100.0%
internal/ipc            47.7%
internal/kiapi          60.6%
internal/schematic      89.1%
internal/workflows      89.8%
```

Current `make coverage` totals:

```text
including generated protobuf code:     5.2%
excluding internal/kiapi/gen/**:      63.9%
```

## Coverage Accounting Rules

Generated protobuf code under `internal/kiapi/gen/**` is excluded from actionable coverage targets. It should remain covered indirectly by tests for hand-written command wrappers, marshaling, unmarshaling, and API envelope behavior.

Coverage reports should publish both:

- Raw Go coverage including all packages.
- Meaningful hand-written coverage excluding `internal/kiapi/gen/**`.

The meaningful coverage number is the primary quality signal.

## Targets

Initial target for hand-written code:

```text
total excluding generated protobuf code: >= 75%
```

Package-level targets:

```text
cmd/kicadai             >= 70%
internal/config        >= 95%
internal/ipc            >= 65%
internal/kiapi          >= 75%
internal/schematic      >= 90%
internal/workflows      >= 90%
```

Stretch target after transport and API test seams mature:

```text
total excluding generated protobuf code: >= 85%
```

## Required Test Categories

### Unit Tests

Unit tests must avoid live KiCad dependencies. They should cover:

- CLI parsing, command dispatch, JSON output, and error output.
- Configuration precedence, normalization, and redaction.
- Fake IPC transport behavior and transport error wrappers.
- KiCad API envelope construction, token capture, API status handling, and typed wrapper commands.
- Document conversion and document type parsing.
- Capability detection.
- Schematic request validation and deterministic operation planning.
- Workflow registry validation and operation errors.

### Optional Integration Tests

Live KiCad tests must remain opt-in with the `integration` build tag and environment variables. They should verify:

- Dialing a real KiCad IPC endpoint.
- `ping`, `version`, and `documents` against a running KiCad instance.
- Capability reporting for the running version.

Integration tests must be skipped when `KICAD_API_SOCKET` is not set. They are not included in the normal `make coverage` or `make coverage-check` thresholds; those gates measure deterministic unit coverage for hand-written code.

Live verification uses:

```text
KICAD_API_SOCKET   required socket endpoint
KICAD_API_TOKEN    optional token when the KiCad instance requires one
KICAD_TIMEOUT_MS   optional request timeout override
```

### Regression Tests

Any bug found by Prism review, manual testing, or live KiCad testing should get a focused regression test unless it is exclusively in generated code.

## Exclusions

Do not add direct tests for generated protobuf getter/setter boilerplate. Do not force live KiCad into the normal `make test` path. Do not use coverage thresholds to justify broad, brittle tests that assert incidental formatting unrelated to behavior.

## Tooling Requirements

Add a repeatable way to generate meaningful coverage reports. The workflow should support:

- Normal test execution.
- Cover profile generation.
- Generated-code exclusion.
- A concise coverage summary.

Preferred commands:

```sh
make test
make coverage
```

`make coverage` should write profiles to a temporary or ignored path and print the hand-written total.

## Acceptance Criteria

The coverage improvement work is complete when:

- `make test` passes.
- `make coverage` reports generated-inclusive and generated-excluded totals.
- Hand-written total coverage is at least 75%.
- Package targets are either met or documented with a specific follow-up reason.
- No normal test requires KiCad to be running.
