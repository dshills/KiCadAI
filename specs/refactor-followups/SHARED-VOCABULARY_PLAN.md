# Shared Domain Vocabulary Plan

## Objective

Remove duplicated Go type definitions for acceptance levels, component roles,
and PCB net roles while preserving every existing JSON spelling and package
constant name. The shared vocabulary is a type-identity refactor, not a
schema migration.

## Phase 1: Add the vocabulary kernel

- Add `internal/domain/vocabulary.go` with dependency-free string types and the
  superset of values already used by the circuit graph, schematic IR, placer,
  router, components, and workflow packages.
- Add tests that assert all existing spellings are represented.
- Acceptance: the package builds without importing any application package.

## Phase 2: Alias acceptance and component roles

- Replace the `AcceptanceLevel` and `ComponentRole` declarations in
  `components`, `designworkflow`, and `circuitgraph` with aliases to
  `internal/domain`.
- Preserve package-local constant names, including the historical underscore
  and hyphen acceptance spellings.
- Add compile-time assignment tests across package boundaries.
- Acceptance: focused component, circuit graph, and workflow tests pass with no
  serialized golden changes.

## Phase 3: Alias net roles

- Replace the `NetRole` declarations in `circuitgraph`, `placement`, `routing`,
  and `schematicir` with aliases to the shared type.
- Preserve all package-local constants, including placement/router-specific
  roles and schematic no-connect/bus roles.
- Add cross-package conversion tests and rerun all routing/layout tests.

## Phase 4: Full verification and documentation

- Run `go test ./...`, lint, and deterministic golden tests.
- Verify JSON schemas and recorded promotion fixtures are byte-for-byte stable.
- Update the architecture review with the completed refactor and remaining
  bounded-context differences.

## Risks and rollback

The main risk is compile-time incompatibility caused by aliases exposing
additional constants to package-local validation. No wire format should change.
Each phase is independently revertible; the previous package-local declarations
can be restored without changing data files.
