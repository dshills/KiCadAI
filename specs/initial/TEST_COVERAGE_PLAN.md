# Test Coverage Implementation Plan

## Overview

This plan improves test coverage for KiCadAI in phases while preserving the current rule that normal tests do not require a running KiCad instance. The plan prioritizes behavior-heavy hand-written code and treats generated protobuf bindings as an excluded coverage domain.

## Phase 1: Coverage Tooling

### Objective

Make coverage measurement repeatable and meaningful.

### Tasks

1. Add a `coverage` Make target.
2. Generate a raw profile for all packages.
3. Generate a second profile excluding `internal/kiapi/gen/**`.
4. Print both totals.
5. Keep coverage output files in the ignored `.coverage/` directory.

### Deliverables

- `make coverage`
- Raw cover profile.
- Generated-excluded cover profile.
- Console summary with the meaningful total.

### Done When

- `make coverage` runs without KiCad.
- The hand-written total matches `go tool cover` on the generated-excluded profile.

## Phase 2: CLI Coverage

### Objective

Raise `cmd/kicadai` from about 57% to at least 70%.

### Tasks

1. Add tests for text and JSON `config` output.
2. Add tests for `ping`, `version`, `documents`, and `capabilities` failure paths.
3. Cover `writeProbeFailure` for JSON and text output.
4. Cover invalid command and help paths.
5. Cover plan/draw JSON validation errors.
6. Cover credential redaction helper behavior without exposing token values.

### Suggested Test Cases

```text
config text output redacts token
ping connect failure returns JSON probe failure
version client error propagates in JSON-safe form
documents invalid document type fails clearly
capabilities version failure reports error
plan-led-demo missing document returns structured JSON error
draw-led-demo without --execute returns structured JSON error
unknown command returns usage text
```

### Done When

- `cmd/kicadai` coverage is at least 70%.
- CLI tests use fake clients and do not dial real KiCad.

## Phase 3: KiCad API Client Coverage

### Objective

Raise `internal/kiapi` from about 61% to at least 75%.

### Tasks

1. Cover `Ping` success and failure paths.
2. Cover `GetVersion` success, malformed response, and API error paths.
3. Cover `Close` behavior.
4. Cover `APIError.Error` with and without server messages.
5. Cover `ClientError.Error` and `Unwrap`.
6. Cover `newRequestHeader` missing token-field behavior only if the seam can be tested without brittle reflection mutation.
7. Add document conversion tests for board, schematic, project, and unknown/partial documents.
8. Cover `joinLibraryID` and document type mappings.

### Done When

- `internal/kiapi` coverage is at least 75%.
- Tests assert envelope behavior through fake transport payloads, not live KiCad.

## Phase 4: IPC Coverage

### Objective

Raise `internal/ipc` from about 48% to at least 65%.

### Tasks

1. Cover fake transport dial, send, receive, request, and close error setters.
2. Cover `ConnectionError.Error` and `Unwrap`.
3. Cover deadline helper behavior.
4. Cover Mangos constructor failure paths where possible without real endpoints.
5. Keep real transport behavior integration-style only if it depends on OS sockets.

### Done When

- `internal/ipc` coverage is at least 65%.
- Tests remain deterministic on macOS and Linux.

## Phase 5: Schematic and Workflow Hardening

### Objective

Nudge already-strong domain packages over their targets and cover edge cases.

### Tasks

1. Add missing schematic validation edge cases for blank library, reference, value, label text, invalid label types, and non-finite rotations.
2. Add LED workflow tests for default libraries, origin offsets, empty prefix fallback, and invalid document propagation.
3. Add workflow registry tests for multiple issue formatting and operation error fallback branches.
4. Add execution boundary tests for JSON serialization shape if needed.

### Done When

- `internal/schematic` coverage is at least 90%.
- `internal/workflows` coverage is at least 90%.

## Phase 6: Coverage Gate

### Objective

Make coverage regression visible without over-constraining generated code.

### Tasks

1. Add a script or Make target that fails when hand-written total drops below 75%.
2. Optionally add package-level threshold checks.
3. Document how to update thresholds after meaningful test improvements.
4. Keep the gate separate from `make test` until the threshold is stable.

### Recommended Targets

```text
make coverage          prints report only
make coverage-check    fails below threshold
```

### Done When

- `make coverage-check` passes locally.
- Generated protobuf packages do not affect the threshold.

## Phase 7: Optional Live KiCad Coverage Notes

### Objective

Document what live tests prove without mixing them into coverage targets.

### Tasks

1. Keep integration tests behind `-tags=integration`.
2. Document required variables:

```text
KICAD_API_SOCKET
KICAD_API_TOKEN
KICAD_TIMEOUT_MS
```

3. Add live tests only for behavior that cannot be proven with fake transport.
4. Avoid including live integration tests in coverage thresholds.

### Done When

- README explains normal coverage versus live verification.
- Integration tests remain opt-in.

## Implementation Order

1. Add tooling first so each later phase can measure progress.
2. Improve CLI and API client coverage next because they have the largest user-facing risk.
3. Improve IPC coverage after API tests reveal any missing transport seams.
4. Finish with schematic/workflow edge cases and threshold checks.

## Quality Gates

Each phase should satisfy:

- `make test`
- `make coverage`
- No live KiCad dependency in normal tests.
- No token values in logs, snapshots, or failure messages.
- No tests added directly against generated protobuf boilerplate unless they verify hand-written wrapper behavior.

## Risks

### Risk: Coverage Churn From Generated Code

Mitigation: Always use the generated-excluded profile for quality gates.

### Risk: Brittle CLI Output Tests

Mitigation: Prefer JSON assertions for structured behavior and only test text output where user-facing wording matters.

### Risk: Transport Tests Become OS-Specific

Mitigation: Keep Mangos and real IPC tests narrow; use fake transport for deterministic behavior.

### Risk: Thresholds Block Useful Refactors

Mitigation: Start with report-only coverage, then introduce `coverage-check` after the first improvement pass.
