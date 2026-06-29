# Component And Block Catalog Expansion Spec

## Purpose

AI-generated designs are only as reliable as the verified parts and reusable
blocks they can use. KiCadAI needs a clearer expansion path for component
catalog coverage and circuit block readiness, especially for the user's target
domain of Class A and Class AB headphone/power amplifiers.

This track makes catalog/block gaps explicit and testable so contributors can
add verified parts and blocks independently.

## Scope

- Add a deterministic catalog/block readiness matrix for AI-generation needs.
- Identify required component families, block families, and evidence gates.
- Add amplifier-focused gap records for output devices, bias networks,
  protection, coupling, stability, load drive, and thermal/current layout.
- Ensure expansion data is machine-readable and validated by tests.

## Non-Goals

- Do not claim unverified amplifier blocks are fabrication-ready.
- Do not add datasheet claims without source/evidence fields.
- Do not require external KiCad libraries in default tests.

## Required Behavior

### Gap Matrix

The repository must contain a machine-readable matrix describing:

- category: component, block, layout, validation, or documentation;
- domain: generic, breakout, sensor, amplifier, power, etc.;
- readiness: missing, draft, connectivity, candidate, or verified;
- blocker reason;
- minimum evidence needed for promotion;
- suggested next implementation target, represented as one of the registered
  task types: `add_component`, `add_block`, `verify_pinmap`, `verify_layout`,
  `capture_kicad_evidence`, or `write_docs`.

Record IDs must be globally unique, lower-case, dot-separated identifiers in
the form `<domain>.<category>.<slug>`, for example
`amplifier.component.output_transistor`.

### Amplifier Coverage

The matrix must explicitly cover:

- verified op-amp choices with supply/output-drive/stability evidence;
- output transistors/MOSFETs for Class A/Class AB stages;
- diode or Vbe-multiplier bias networks;
- headphone DC blocking and output protection;
- load resistors and safe test loads;
- rail decoupling and local bypassing;
- thermal/current layout constraints;
- KiCad-backed ERC/DRC evidence.

Amplifier coverage requirements should be declared in machine-readable data so
tests compare the matrix against the declared requirements rather than
hardcoding the full list in Go.

Layout readiness requires objective evidence, such as net-class width checks,
current-role routing rules, clearance/creepage constraints, thermal review
metadata, or KiCad DRC artifacts. Default tests validate checked-in evidence
statically, such as by decoding JSON reports or matrix records; they must not
invoke KiCad. A layout record is not `verified` by prose alone.

Evidence that claims to verify generated KiCad artifacts must include either a
semantic SHA-256 hash over a canonicalized representation that removes volatile
KiCad metadata, or a git object ID for checked-in source artifacts. Static
validation must compare those identifiers when the referenced files are checked
in.

### Validation

Tests must validate the gap matrix schema, stable IDs, allowed readiness values,
and required evidence fields for missing or non-verified records.

## Acceptance Criteria

- A checked-in matrix exists under `data/ai-readiness/`.
- Unit tests validate its schema and coverage against machine-readable
  requirements.
- README or docs point contributors to the matrix.
- `go test ./...` passes.
