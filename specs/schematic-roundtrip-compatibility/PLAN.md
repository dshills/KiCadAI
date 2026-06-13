# Schematic Round-Trip Compatibility Implementation Plan

## 1. Objective

Implement the schematic round-trip compatibility work described in `SPEC.md`.
The outcome should be generated schematics that avoid known KiCad save churn,
especially `-0.0` coordinates and missing default symbol properties, and a
repeatable validation path for promoting blocks toward `roundtrip_verified`.

## 2. Implementation Rules

- Keep normal tests KiCad-free.
- Keep KiCad CLI checks opt-in through the existing round-trip environment and
  CLI flags.
- Prefer centralized writer fixes over generated-file string patching.
- Regenerate examples only after writer behavior is fixed and tested.
- Commit after each phase.
- Run Prism review on staged changes before each phase commit.
- Do not promote any block to `roundtrip_verified` unless evidence is produced
  and documented.
- Do not commit generated block `.kicad_pcb` files in this project.

## 3. Phase 1: Baseline Evidence and Failing Tests

### Goal

Capture the current schematic round-trip failures as local regression tests and
documented evidence before changing writer behavior.

### Work

1. Add focused unit tests that currently expose negative-zero rendering in
   schematic output.
2. Add focused tests showing that generated symbols can omit KiCad-added
   `Datasheet` and `Description` properties.
3. Add or update an opt-in round-trip fixture test for at least:
   - `examples/blocks/led_indicator/led_indicator.kicad_sch`;
   - a generated-in-test LED schematic, if an existing fixture is easier to
     isolate.
4. Ensure the tests either:
   - fail before the fix and pass after the fix; or
   - are introduced with `t.Skip`/documented TODO only when they require KiCad
     CLI and the normal suite must stay green.
5. Update `docs/circuit-block-readiness.md` only if new baseline evidence is
   more precise than the Phase 17 evidence.

### Tests

```sh
GOCACHE=/tmp/kicadai-gocache go test ./internal/kicadfiles/schematic ./internal/kicadfiles/roundtrip
GOCACHE=/tmp/kicadai-gocache go test ./...
```

Optional:

```sh
KICADAI_RUN_KICAD_CLI=1 \
KICADAI_KICAD_CLI=/path/to/kicad-cli \
GOCACHE=/tmp/kicadai-gocache go test ./internal/kicadfiles/roundtrip
```

### Acceptance Criteria

- The failure modes are represented by tests or documented opt-in checks.
- Normal `go test ./...` remains green.
- No writer behavior changes are included in this phase unless required for
  test harness stability.

### Commit Message

```text
Add schematic round-trip baseline tests
```

## 4. Phase 2: Canonical Numeric Rendering

### Goal

Eliminate generated `-0.0` from schematic files by fixing numeric rendering at
the shared model/rendering layer.

### Work

1. Identify the central numeric rendering path used for schematic points,
   `at` nodes, effects, sizes, angles, and line endpoints.
2. Add a canonicalization helper that renders exact zero as `0`.
3. Ensure the helper does not collapse real non-zero values from parsed user
   files.
4. Apply the helper to schematic rendering paths:
   - `renderAt`;
   - `renderPoints`;
   - property positions;
   - symbol positions;
   - label/text/sheet positions where applicable.
5. Add unit tests for:
   - `kicadfiles.Point{X: 0, Y: 0}`;
   - positive and negative non-zero coordinates;
   - zero coordinates in symbols, properties, wires, and sheets.
6. Search generated schematics for `-0.0` after regeneration in a temporary
   directory.

### Tests

```sh
GOCACHE=/tmp/kicadai-gocache go test ./internal/kicadfiles/schematic ./internal/kicadfiles/sexpr
GOCACHE=/tmp/kicadai-gocache go test ./...
```

Optional smoke:

```sh
go run ./cmd/kicadai --json \
  --request examples/blocks/requests/led_indicator.json \
  --output /tmp/kicadai-led-roundtrip-smoke \
  --name led_indicator \
  --overwrite \
  block instantiate led_indicator
rg --fixed-strings -- '-0.0' /tmp/kicadai-led-roundtrip-smoke
```

### Acceptance Criteria

- Normal tests pass.
- Generated schematic output no longer contains `-0.0`.
- No unrelated PCB numeric behavior changes are introduced unless covered by
  tests.

### Commit Message

```text
Canonicalize schematic zero values
```

## 5. Phase 3: Default Symbol Property Completion

### Goal

Emit KiCad-compatible default symbol properties so KiCad does not add empty
`Datasheet` and `Description` properties during save.

### Work

1. Add a schematic helper that ensures generated symbols have:
   - `Reference`;
   - `Value`;
   - `Footprint`;
   - `Datasheet`;
   - `Description`.
2. Preserve explicit values and flags when any of these properties already
   exist.
3. Preserve existing property UUIDs when present.
4. Generate deterministic UUIDs for inserted default properties if the writer
   requires them.
5. Match KiCad-compatible default property attributes:
   - position;
   - hidden/show-name behavior;
   - `do_not_autoplace`;
   - effects.
6. Wire the helper into generated-symbol paths:
   - design API `AddSymbol`;
   - transaction apply `add_symbol`;
   - legacy direct schematic constructors used by examples.
7. Decide and document whether power symbols get the same defaults. Start
   conservative: only generated component-like symbols where KiCad currently
   adds the properties.

### Tests

```sh
GOCACHE=/tmp/kicadai-gocache go test ./internal/kicadfiles/schematic ./internal/kicadfiles/designapi ./internal/transactions ./internal/blocks
GOCACHE=/tmp/kicadai-gocache go test ./...
```

Unit coverage must include:

- missing `Datasheet`/`Description` are inserted;
- explicit values are preserved;
- required property order is stable;
- generated transaction symbols and design API symbols behave the same way.

### Acceptance Criteria

- KiCad-added empty property churn is eliminated for the target generated
  block examples.
- Existing explicit properties are not overwritten.
- Normal tests pass.

### Commit Message

```text
Emit KiCad default schematic properties
```

## 6. Phase 4: Property Ordering and Snapshot Hardening

### Goal

Make symbol property order deterministic and KiCad-save-compatible across all
generated schematic paths.

### Work

1. Review `symbolProperties` and property rendering in
   `internal/kicadfiles/schematic/schematic.go`.
2. Define required property order:
   - `Reference`;
   - `Value`;
   - `Footprint`;
   - `Datasheet`;
   - `Description`;
   - remaining user properties.
3. Keep user-defined extra properties stable without dropping attributes.
4. Add snapshot or targeted render tests for:
   - simple LED schematic;
   - block-generated symbol list;
   - a symbol with extra properties.
5. Ensure read-then-write for parsed schematic properties does not unexpectedly
   reorder unsupported raw content.

### Tests

```sh
GOCACHE=/tmp/kicadai-gocache go test ./internal/kicadfiles/schematic ./internal/kicadfiles/designapi ./internal/transactions
GOCACHE=/tmp/kicadai-gocache go test ./...
```

### Acceptance Criteria

- Property order matches the expected KiCad-compatible order.
- Existing tests and snapshots are updated intentionally.
- Normal tests pass.

### Commit Message

```text
Stabilize schematic property ordering
```

## 7. Phase 5: Round-Trip Harness and Allowlist Tightening

### Goal

Make schematic round-trip checks useful for block examples and agent-facing
reports.

### Work

1. Review existing `internal/kicadfiles/roundtrip` schematic behavior.
2. Add opt-in tests for checked-in block schematic examples:
   - LED indicator;
   - connector breakout.
3. Ensure round-trip failures report:
   - file path;
   - KiCad CLI path/version when available;
   - normalized diff category;
   - artifact path when artifacts are retained.
4. If a harmless KiCad diff remains, add a narrow structured allowlist entry
   with a reason. Do not broad-match arbitrary property or coordinate changes.
5. Update `internal/kicadfiles/roundtrip/README.md` if flags or interpretation
   change.

### Tests

```sh
GOCACHE=/tmp/kicadai-gocache go test ./internal/kicadfiles/roundtrip
GOCACHE=/tmp/kicadai-gocache go test ./...
```

Optional:

```sh
KICADAI_RUN_KICAD_CLI=1 \
KICADAI_KICAD_CLI=/path/to/kicad-cli \
GOCACHE=/tmp/kicadai-gocache go test ./internal/kicadfiles/roundtrip
```

### Acceptance Criteria

- Normal tests pass without KiCad.
- Opt-in schematic round-trip tests pass for LED and connector examples, or
  produce only documented narrow allowlist diffs.
- Failure output remains structured for CLI/agent consumers.

### Commit Message

```text
Tighten schematic round-trip checks
```

## 8. Phase 6: Regenerate Block Examples

### Goal

Refresh checked-in block examples with the round-trip-compatible schematic
writer output.

### Work

1. Regenerate all block examples from `examples/blocks/requests`.
2. Remove generated `.kicad_pcb` files before staging.
3. Inspect every generated block project.
4. Search all block schematics for `-0.0`.
5. Verify generated symbols contain `Datasheet` and `Description` defaults
   where expected.
6. Check that manifests remain accurate and deterministic.

### Commands

Use the existing request files, for example:

```sh
go run ./cmd/kicadai --json \
  --request examples/blocks/requests/led_indicator.json \
  --output examples/blocks/led_indicator \
  --name led_indicator \
  --overwrite \
  block instantiate led_indicator
```

Run analogous commands for:

- `connector_breakout`;
- `voltage_regulator`;
- `i2c_sensor`;
- `opamp_gain_stage`;
- `usb_c_power`;
- `mcu_minimal`;
- `composed_sensor_breakout` using `block compose`.

Validation:

```sh
find examples/blocks -mindepth 1 -maxdepth 1 -type d \
  ! -name requests \
  ! -name reports \
  -print0 |
while IFS= read -r -d '' d; do
  go run ./cmd/kicadai --json inspect project "$d" >/dev/null
done

rg --fixed-strings -- '-0.0' examples/blocks
```

### Tests

```sh
GOCACHE=/tmp/kicadai-gocache go test ./...
```

Optional:

```sh
KICADAI_RUN_KICAD_CLI=1 \
KICADAI_KICAD_CLI=/path/to/kicad-cli \
GOCACHE=/tmp/kicadai-gocache go test ./internal/kicadfiles/roundtrip
```

### Acceptance Criteria

- Block examples are regenerated and inspect cleanly.
- No block schematic contains `-0.0`.
- `.kicad_pcb` block artifacts are not committed.
- Normal tests pass.

### Commit Message

```text
Regenerate round-trip-ready block schematics
```

## 9. Phase 7: Readiness Documentation and Verification Status

### Goal

Update documentation with final evidence and decide whether any block can be
promoted to `roundtrip_verified`.

### Work

1. Update `docs/circuit-block-readiness.md` with:
   - KiCad CLI version used;
   - examples checked;
   - pass/fail result;
   - remaining diffs, if any;
   - any allowlist entries.
2. Update `docs/circuit-block-library.md` if usage or limitations changed.
3. If LED and connector examples pass round-trip without unexpected diffs,
   decide whether to promote their block verification levels. Promotion is not
   required by this project and must not imply fabrication readiness.
4. If any block is promoted, update:
   - block definition;
   - tests asserting current verification levels;
   - readiness documentation.
5. If no block is promoted, document exactly why.

### Tests

```sh
GOCACHE=/tmp/kicadai-gocache go test ./...
```

Optional:

```sh
KICADAI_RUN_KICAD_CLI=1 \
KICADAI_KICAD_CLI=/path/to/kicad-cli \
GOCACHE=/tmp/kicadai-gocache go test ./internal/kicadfiles/roundtrip
```

### Acceptance Criteria

- Readiness documentation matches actual test evidence.
- Verification levels are conservative and tested.
- Normal tests pass.
- Prism has no unresolved correctness findings.

### Commit Message

```text
Document schematic round-trip readiness
```

## 10. Final Validation Checklist

Run before declaring the project complete:

```sh
GOCACHE=/tmp/kicadai-gocache go test ./...
```

Run if KiCad CLI is available:

```sh
KICADAI_RUN_KICAD_CLI=1 \
KICADAI_KICAD_CLI=/path/to/kicad-cli \
GOCACHE=/tmp/kicadai-gocache go test ./internal/kicadfiles/roundtrip
```

Run against block examples:

```sh
find examples/blocks -mindepth 1 -maxdepth 1 -type d \
  ! -name requests \
  ! -name reports \
  -print0 |
while IFS= read -r -d '' d; do
  go run ./cmd/kicadai --json inspect project "$d" >/dev/null
done

rg --fixed-strings -- '-0.0' examples/blocks
```

Review before final commit:

```sh
prism review staged
```

## 11. Expected Follow-Up Work

These are outside this plan:

- full schematic object preservation;
- ERC integration;
- block readiness aggregate CLI command;
- PCB block placement/routing;
- resolver semantic expansion for complex symbols;
- fabrication package generation.
