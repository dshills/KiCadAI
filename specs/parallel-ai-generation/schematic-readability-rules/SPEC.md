# Schematic Readability Rules Spec

## Purpose

Generated schematics should be understandable to humans. The writer should
prefer conventional schematic layout: signal flows left to right, power rails
are visually above signal paths, returns and lower-voltage references are lower
on the page, feedback paths are identifiable, and long shared nets use labels
instead of unreadable wires.

This track hardens deterministic readability rules so AI-generated designs can
be rejected or repaired before a human opens KiCad.

## Scope

- Define reusable readability profiles for standard and amplifier schematics.
- Add measurable layout constraints for spacing, stage order, power/return
  placement, the prohibition of diagonal wires in favor of orthogonal routing,
  and label fallback.
- Expose machine-readable diagnostics that can drive AI repair prompts.
- Add tests against checked-in example schematics and generated workflow
  summaries.

## Non-Goals

- Do not implement a full schematic autorouter.
- Do not mutate imported user schematics until preservation-safe mutation is
  implemented.
- Do not require pixel-perfect KiCad rendering in default tests.

## Required Behavior

### Rule Profiles

The readability package must expose profile metadata:

- `standard`: general block and small-board schematics;
- `amplifier`: audio or low-frequency analog schematics with stricter
  signal-flow, rail, output stage, feedback, and load/return expectations. RF
  amplifier constraints are out of scope until a separate profile exists.

Each profile must report the rules it enforces so AI agents can explain why a
schematic failed.

### Diagnostics

Diagnostics must be stable and repair-oriented. For each blocking issue, the
diagnostic should indicate which rule was violated and what kind of repair is
expected, such as spreading components, moving a return lower, moving an output
stage rightward, or replacing a long crossing wire with a label.

Diagnostic stability means repair loops must rely on constant string codes from
the readability package. Codes must not be derived from line numbers,
coordinates, generated prose, or other volatile values. Diagnostics may still
carry volatile repair context such as object references, UUIDs, positions, and
bounding boxes; that context must be separate from the stable diagnostic code.

### Workflow Evidence

`design create` readability evidence should remain compact but should include
the active profile, pass/fail state, and counts of important readability
diagnostic families.

## Acceptance Criteria

- Standard examples continue to pass standard readability gates.
- Amplifier examples continue to pass strict amplifier readability gates.
- Rule profile metadata and repair guidance have unit coverage.
- `go test ./...` passes.
