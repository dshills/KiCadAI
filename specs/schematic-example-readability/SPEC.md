# Schematic Example Readability Specification

Date: 2026-06-29

## Purpose

Apply the new schematic readability foundation to real checked-in examples so
KiCadAI's visible schematic output looks like an engineer-readable schematic,
not just a parseable netlist drawing.

The previous schematic readability project added the core model, classifier,
placement/routing helpers, validator, workflow evidence, op-amp block spacing,
and orthogonal design API wires. This project turns that foundation into
regression coverage and improved examples, with special focus on the amplifier
fixtures that exposed the original readability problem.

## Background

Current foundations:

- `internal/schematiclayout` can classify roles/stages/lanes, place components,
  route orthogonal schematic wires, emit label fallbacks, and report geometry
  diagnostics.
- `design create` now includes schematic readability summary evidence.
- `internal/blocks/opamp.go` uses wider op-amp gain-stage coordinates.
- `internal/kicadfiles/designapi.Builder.Connect` emits orthogonal schematic
  wire segments.
- README and ROADMAP describe schematic readability support and limitations.

Remaining gap:

- The checked-in example schematics have not all been regenerated or normalized
  through readability checks.
- The Class AB headphone amplifier fixture can still visually regress unless
  strict example-level tests assert spacing, direction, rails, and overlap
  constraints.
- The current validator uses approximate geometry and needs example-specific
  adapters for actual `.kicad_sch` files.

## Goals

1. Add a schematic example readability audit that runs against checked-in
   examples.
2. Convert `.kicad_sch` files into `schematiclayout` validation inputs without
   requiring KiCad.
3. Add standard readability gates for simple examples.
4. Add strict amplifier readability gates for Class AB, Class A, and op-amp
   headphone-buffer examples.
5. Improve or regenerate examples that fail readability rules.
6. Preserve existing semantic landmark, parser, writer, and round-trip tests.
7. Keep generated example changes deterministic and reviewable.
8. Document the example readability expectations and known limitations.

## Non-Goals

- Implementing a full KiCad schematic editor.
- Guaranteeing electrical correctness beyond existing schematic/semantic tests.
- Making amplifier examples fabrication-ready.
- Automatically mutating arbitrary imported user schematics.
- Solving exact KiCad text rendering, all justifications, or all rotations.
- Replacing KiCad ERC.

## Example Scope

Initial examples:

- `examples/01_led_indicator`
- `examples/02_button_pullup`
- `examples/03_rc_filter`
- `examples/04_555_timer`
- `examples/05_sensor_node`
- `examples/06_class_ab_headphone_amp`
- `examples/09_class_a_headphone_amp`
- `examples/10_opamp_buffer_headphone_amp`

Optional later examples:

- generated PCB examples where schematic readability is meaningful;
- block-generated examples under `examples/blocks`;
- design workflow outputs under temporary test directories.

## Readability Profiles

### Standard Example Profile

Used for simple examples. It should fail on:

- diagonal wires;
- symbol body overlaps;
- wire-through-symbol body intersections;
- objects outside usable sheet bounds;
- severe title-block overlap.

It should warn, but not necessarily fail, on:

- text overlaps;
- long wires that should become labels;
- crowded local groups;
- minor rail/lane deviations.

### Strict Amplifier Profile

Used for amplifier examples. It should fail on:

- all standard-profile failures;
- input connector not left of gain/output stages;
- output connector/load not right of gain/output stages;
- feedback not above or around the active gain stage;
- positive rails below the signal lane;
- ground/return/load below expected lower lane;
- load/output labels crossing or sitting on active symbols;
- class AB output stage compressed into the op-amp body area.

## Schematic File Adapter

Add an adapter from parsed `.kicad_sch` data to `schematiclayout.Result`.

The adapter should use:

- symbol reference, value, library ID, position, rotation, and pin anchors;
- wire segments;
- labels;
- junctions;
- title block and paper settings;
- role inference from reference/value/library ID.

The adapter may use conservative default bounding boxes for symbols and text.
It must be deterministic.

## Validator Enhancements

The existing validator should be extended only as needed for examples:

- detect diagonal wires from parsed schematic files;
- map parsed symbols to role/stage/lane expectations;
- report first-class amplifier topology readability diagnostics;
- expose a compact per-example report for tests.

## Example Improvement Strategy

Prefer the safest available path in this order:

1. Regenerate examples from deterministic source requests when a generator
   exists.
2. Apply a focused coordinate/wire normalization helper to checked-in fixtures.
3. Manually patch `.kicad_sch` only when the fixture is small and the semantic
   landmarks are preserved.

All example updates must preserve:

- KiCad parseability;
- semantic landmark tests;
- existing file structure;
- deterministic UUIDs where possible;
- existing project names and filenames.

## Test Requirements

Add tests for:

- adapter conversion from parsed schematic to layout result;
- standard readability for simple examples;
- strict amplifier readability for `06`, `09`, and `10`;
- no diagonal wires in checked-in example schematics;
- no symbol body overlaps in checked-in example schematics;
- left-to-right amplifier signal flow;
- rail/ground vertical ordering in amplifier schematics;
- existing amplifier semantic landmark tests still pass.

Optional KiCad-backed tests may remain behind existing KiCad environment flags.

## Acceptance Criteria

This project is complete when:

1. Checked-in simple examples pass standard readability tests.
2. Amplifier examples pass strict amplifier readability tests.
3. At least `examples/06_class_ab_headphone_amp` is visibly improved or proven
   already compliant by tests.
4. Existing semantic landmark tests pass.
5. Existing schematic parser/writer tests pass.
6. `go test ./...` passes.
7. README/ROADMAP mention that example readability gates exist.

## Known Limitations

- Geometry remains approximate until KiCad text justification and symbol
  library extents are modeled more precisely.
- Some hand-authored examples may need focused manual adjustment because no
  source generator exists.
- Readability tests do not prove amplifier safety, stability, noise
  performance, thermal behavior, or fabrication readiness.
