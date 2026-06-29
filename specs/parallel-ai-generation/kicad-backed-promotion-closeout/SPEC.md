# KiCad-Backed Promotion Closeout Spec

## Purpose

KiCadAI needs a small set of generated designs that are not merely parseable by
the internal writer, but are backed by real KiCad ERC/DRC evidence and promoted
from `expected_fail` to `candidate` or `pass`.

This track focuses on the optional KiCad-backed design fixtures under
`examples/design/kicad-backed/`. It gives AI workflows an evidence ladder:

1. generate a project from a structured request;
2. write project, schematic, PCB, transaction, and manifest artifacts;
3. run writer correctness, connectivity validation, routing checks, and KiCad
   ERC/DRC where configured;
4. produce `.kicadai/design-promotion.json`;
5. make the next required repair actions explicit.

## Scope

- Add stronger promotion-report action guidance for failed or incomplete gates.
- Add fixture progression checks that prevent accidental promotion without the
  required evidence.
- Add a stable readiness summary for optional KiCad-backed fixtures so multiple
  contributors can work fixture-by-fixture.
- Keep KiCad CLI execution optional in default tests.

## Non-Goals

- Do not require KiCad in default `go test ./...`.
- Do not make expected-fail fixtures pass without real ERC/DRC evidence.
- Do not hide blockers. Missing KiCad evidence must remain visible.

## Required Behavior

### Promotion Report Actions

Each promotion report should include machine-readable next actions for gates
that are failed, warning, skipped while required, or not run while required.
Actions must include:

- gate ID;
- severity;
- short summary;
- recommended action;
- related issue codes;
- related artifact paths.

These actions are intended for AI agents and contributors. They should be
deterministic and safe to show in CLI output or saved JSON. Artifact paths must
be portable paths relative to the generated project directory or repository
root, not developer-machine absolute paths.

### Fixture Progression Policy

Optional KiCad-backed metadata must satisfy:

- `expected_fail` and `blocked` fixtures must have non-empty `known_gaps` or
  an explicit runner/tooling issue in the fixture notes;
- `candidate` fixtures must require ERC, and must require DRC when the
  generated fixture includes a PCB layout artifact, unless an explicit fixture
  allowlist explains why not;
- `pass` fixtures must require ERC and, when the generated fixture includes a
  PCB layout artifact, DRC;
- `candidate` and `pass` fixtures must expect a promotion report artifact;
- `pass` fixtures must have zero ERC/DRC violations and empty allowlists.

### Readiness Summary

The codebase should expose a deterministic summary of optional KiCad-backed
fixtures that can be rendered in documentation or tests. The summary must group
fixtures by readiness and preserve fixture IDs, tiers, acceptance levels,
required KiCad evidence, and known gap counts.

## Acceptance Criteria

- Default tests validate promotion-action schema and fixture metadata policy.
- `go test ./...` passes without KiCad installed.
- Prism review means running `prism review staged` against the staged changes
  before each phase commit and resolving actionable findings.
- No optional fixture is incorrectly promoted by metadata alone.
