# Schematic Round-Trip Compatibility Specification

## Purpose

Make KiCadAI-generated schematic files match KiCad's own save-normalized output
closely enough that generated schematic fixtures can pass KiCad CLI round-trip
checks without unexpected diffs.

This is the next verification step for the circuit block library. Phase 17
showed that block-generated schematics are parseable and inspectable, but KiCad
still rewrites them on save. Until those diffs are removed or explicitly
allowlisted as intentional, blocks must remain `structural` rather than
`roundtrip_verified`.

## Background

The current writer can generate usable KiCad 10 schematics and the block
examples open in KiCad. The readiness review found remaining save churn:

- KiCad normalizes generated `-0.0` coordinates to `0`.
- KiCad adds empty `Datasheet` and `Description` symbol properties when they
  are absent.
- Existing round-trip integration tests still fail for generated schematics.

The relevant implementation areas are:

```text
internal/kicadfiles/schematic/
internal/kicadfiles/designapi/
internal/transactions/
internal/kicadfiles/roundtrip/
examples/blocks/
```

The relevant documentation and evidence are:

```text
docs/circuit-block-readiness.md
docs/circuit-block-library.md
specs/schematic-writer/SPEC.md
specs/schematic-writer/PLAN.md
```

## Goals

1. Eliminate avoidable KiCad save churn for generated schematics.
2. Make generated numeric coordinates render in KiCad-compatible canonical form.
3. Emit KiCad-compatible default symbol properties where KiCad writes them.
4. Regenerate block schematic examples using the improved writer.
5. Add unit and integration coverage that prevents regression.
6. Define when a generated schematic may be considered round-trip compatible.
7. Update readiness documentation with exact remaining diffs, if any.

## Non-Goals

- Full lossless read-modify-write preservation for arbitrary existing
  schematics.
- Implementing every KiCad schematic object type.
- Replacing ERC or semantic electrical validation.
- Promoting every circuit block to `roundtrip_verified` in this project.
- Fixing PCB writer round-trip compatibility.
- Depending on live KiCad IPC write calls.

## Compatibility Target

The target is KiCad 10 save behavior, using the installed `kicad-cli` where
available. The writer should continue to emit valid KiCad 9+ files where
possible, but round-trip compatibility decisions are based on KiCad 10.

Compatibility means:

- KiCad opens the schematic without format warnings.
- KiCad CLI can save or normalize the schematic.
- The KiCad-saved result has no unexpected normalized diff from the original.
- Any remaining allowed diff is captured in a structured allowlist with a
  specific reason and owner.

## Current Failure Evidence

The Phase 17 readiness run used:

```sh
kicadai \
  --json \
  --kicad-cli "$KICADAI_KICAD_CLI" \
  roundtrip schematic examples/blocks/led_indicator/led_indicator.kicad_sch
```

Observed result:

- `ROUNDTRIP_DIFF`
- `-0.0` changed to `0`
- missing `Datasheet` and `Description` properties were added by KiCad

Round-trip integration tests also failed:

```sh
KICADAI_RUN_KICAD_CLI=1 \
KICADAI_KICAD_CLI=/path/to/kicad-cli \
go test ./internal/kicadfiles/roundtrip
```

Observed result:

- checked-in generated schematic fixture changed after round-trip;
- generated LED schematic changed library/property rendering.

## Requirements

### 1. Numeric Coordinate Canonicalization

Generated schematic numeric values must not render negative zero.

Rules:

- Any schematic coordinate, angle, size, or scalar derived from an internal
  integer coordinate must render `0`, not `-0.0`.
- The renderer must canonicalize values close enough to zero only when the
  source representation is known to be exact integer internal units or exact
  generated constants.
- Do not hide real small non-zero coordinates from parsed user files.
- Unit tests must cover points, `at` nodes, wire endpoints, label positions,
  property positions, symbol positions, and sheet positions where applicable.

Preferred implementation:

- centralize numeric rendering in the S-expression or KiCad model layer;
- avoid ad hoc string replacement in generated files;
- add tests against rendered S-expressions, not only whole-file snapshots.

### 2. Default Symbol Properties

Generated component symbols must include the default property set KiCad writes
for saved symbols.

Minimum generated properties:

- `Reference`
- `Value`
- `Footprint`
- `Datasheet`
- `Description`

Rules:

- `Reference` and `Value` remain first and second where KiCad expects that
  order.
- `Footprint`, `Datasheet`, and `Description` must be present even when empty
  if KiCad would add them on save.
- Existing explicit property values must not be overwritten.
- Existing explicit property flags, positions, visibility, and UUIDs must be
  preserved.
- Generated default properties must receive deterministic UUIDs where KiCad
  requires UUID identity.
- Property positions and effects must match KiCad save defaults closely enough
  to avoid save churn.

Applicable generation paths:

- design API symbol creation;
- transaction `add_symbol` apply path;
- block instantiation and composition;
- legacy direct schematic constructors still used by examples.

### 3. Property Ordering and Rendering

Symbol property rendering must be deterministic and KiCad-compatible.

Rules:

- Required properties render in KiCad-preferred order.
- Extra user properties render after required properties in stable order.
- Reordering must not drop unsupported property attributes.
- Case-insensitive aliases such as `reference` should normalize to `Reference`
  only for generated symbols or validated models, not by mutating preserved raw
  user content unexpectedly.

### 4. Round-Trip Harness Behavior

The existing round-trip CLI and test harness must be able to distinguish:

- clean round-trip;
- known allowlisted diff;
- unexpected diff;
- skipped KiCad CLI;
- command failure.

Requirements:

- Preserve failed artifacts when requested with existing artifact options.
- Keep normal unit tests KiCad-free.
- Keep KiCad CLI tests opt-in through existing environment variables.
- Report schematic round-trip differences in structured JSON for agent use.
- Add at least one generated block schematic fixture to opt-in round-trip
  coverage.

### 5. Example Regeneration

After writer fixes, regenerate:

- `examples/blocks/led_indicator`
- `examples/blocks/connector_breakout`
- `examples/blocks/voltage_regulator`
- `examples/blocks/i2c_sensor`
- `examples/blocks/opamp_gain_stage`
- `examples/blocks/usb_c_power`
- `examples/blocks/mcu_minimal`
- `examples/blocks/composed_sensor_breakout`

Do not commit generated `.kicad_pcb` files for block examples until PCB block
output is in scope.

Each regenerated example must:

- parse with `inspect project`;
- avoid `-0.0` text in committed schematic files;
- include KiCad-compatible default properties for generated component symbols;
- keep manifests consistent with the generation command.

### 6. Readiness and Verification Levels

This project may promote a block from `structural` to `roundtrip_verified` only
when:

- its generated schematic example passes KiCad CLI schematic round-trip with no
  unexpected diff;
- any required allowlist entries are narrow, justified, and documented;
- normal tests pass;
- the readiness document names the evidence.

No block may claim fabrication readiness from round-trip alone. Fabrication
readiness still requires stronger electrical, pinmap, ERC/DRC, and PCB checks.

## Data Model Changes

The implementation may add helper APIs such as:

```go
func CanonicalSchematicNumber(value kicadfiles.IU) sexpr.Node
func EnsureDefaultSymbolProperties(symbol *schematic.SchematicSymbol, defaults PropertyDefaults)
func (level VerificationLevel) AllowsRoundTripPromotion() bool
```

The exact API names are not prescribed. The important contract is centralized
canonicalization and default-property completion.

## Tests

### Unit Tests

Add or update tests for:

- no negative zero in rendered points and `at` nodes;
- default `Datasheet` and `Description` properties on generated symbols;
- explicit `Datasheet` and `Description` values are preserved;
- property order matches KiCad-save expectations;
- transaction-generated symbols use the same property defaults as design API
  symbols;
- block-generated operations produce symbols with default properties after
  apply.

### Golden/Snapshot Tests

Add fixtures or assertions for:

- LED indicator schematic render;
- at least one block-generated schematic;
- composed block schematic with multiple generated symbols.

Snapshots should be stable and readable. Avoid overfitting to UUID values beyond
deterministic identity requirements.

### Opt-In KiCad CLI Tests

When `KICADAI_RUN_KICAD_CLI=1` and `KICADAI_KICAD_CLI` or `kicad-cli` is
available:

- round-trip `examples/blocks/led_indicator/led_indicator.kicad_sch`;
- round-trip `examples/blocks/connector_breakout/connector_breakout.kicad_sch`;
- optionally round-trip the composed sensor breakout after the simplest
  examples are clean.

The opt-in tests should fail on unexpected diffs and report artifact paths.

## CLI Expectations

Existing commands should continue to work:

```sh
kicadai --json inspect project examples/blocks/led_indicator
kicadai --json roundtrip schematic examples/blocks/led_indicator/led_indicator.kicad_sch
```

No new CLI command is required for this project. If a helper is added, it should
be documented separately and should not replace the existing `roundtrip`
command family.

## Documentation Updates

Update:

- `docs/circuit-block-readiness.md`
- `docs/circuit-block-library.md`
- `internal/kicadfiles/roundtrip/README.md` if command behavior changes

The readiness document must record:

- KiCad CLI version used for evidence;
- which examples passed;
- any allowlisted differences;
- remaining schematic writer gaps.

## Acceptance Criteria

1. `go test ./...` passes.
2. Block examples parse with `inspect project`.
3. Generated block schematics contain no `-0.0`.
4. Generated symbols include KiCad-compatible `Datasheet` and `Description`
   defaults.
5. At least LED indicator and connector breakout schematic examples pass
   opt-in KiCad CLI round-trip, or any remaining diffs are explicitly captured
   in `docs/circuit-block-readiness.md`.
6. No current block is promoted to `roundtrip_verified` unless its evidence is
   documented.
7. Prism review has no unresolved correctness findings.

## Risks

- KiCad save behavior may differ between KiCad 9 and KiCad 10. Mitigation:
  record the KiCad version in readiness evidence and target KiCad 10 for
  promotion.
- Default properties may vary by symbol source or library metadata. Mitigation:
  preserve explicit values and use KiCad-source/saved-output evidence for empty
  defaults.
- Round-trip equality can become over-strict. Mitigation: use narrow structured
  allowlists only for known harmless differences.
- Ad hoc number formatting could damage parsed user files. Mitigation:
  centralize canonicalization and distinguish generated data from preserved raw
  content.

## Open Questions

1. Should empty `Datasheet` and `Description` always be emitted for every symbol,
   including power symbols, or only for component-like symbols KiCad rewrites?
2. Should round-trip promotion be tracked per block, per example, or per
   generator path?
3. Should allowlists live beside examples, under `internal/kicadfiles/roundtrip`,
   or in a top-level readiness directory?
4. Should the writer normalize parsed user schematics on write, or only
   generated schematics?
