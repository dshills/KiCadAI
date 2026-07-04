# I2C Downstream Promotion Specification

Date: 2026-07-04

## Objective

Promote `examples/design/kicad-backed/i2c_sensor_breakout_candidate` past its
current `expected_fail` state by carrying the now route-tree-complete I2C design
through downstream project-write, writer-correctness, validation, and KiCad
ERC/DRC evidence.

The I2C fixture now proves all required VCC/GND/SDA/SCL route-tree contacts.
This project starts where that work ends: route completion is no longer the
blocker. The remaining question is whether the routed design can be written as a
KiCad-native project and pass the same structural and optional KiCad-backed
gates as simpler candidate fixtures.

## Background

The current fixture state is:

- route-tree execution owns VCC/GND/SDA/SCL;
- all 8 route-tree branches emit;
- all 12 required endpoint contacts are graph-proven;
- fixed-net skip notices and missing-net-class warnings are diagnostic evidence,
  not route-completion blockers;
- the metadata remains `expected_fail` because downstream project-write,
  writer-correctness, validation, and KiCad ERC/DRC evidence still need to be
  proven.

This spec turns that remaining expected failure into a concrete promotion lane.

## Non-Goals

- Do not require local KiCad for default `go test ./...`.
- Do not promote the fixture to `pass` unless required ERC/DRC and writer
  round-trip evidence are clean under a configured local KiCad CLI.
- Do not broaden routing algorithms beyond defects discovered by this fixture.
- Do not use allowlists to hide structural writer or validation failures.
- Do not change unrelated amplifier expected-fail behavior.

## Definitions

- `structural candidate`: the fixture completes internal workflow stages through
  project-write, writer-correctness, validation, and route-completion gates
  without requiring a local KiCad CLI.
- `KiCad-backed candidate`: the fixture also runs configured KiCad checks and
  records explicit skipped, warning, or pass evidence according to metadata
  policy.
- `pass`: the fixture satisfies required ERC/DRC and writer round-trip evidence
  without blocking or warning-level promotion gates.

## Required Behavior

### 1. Downstream Stage Continuation

After routing completes with all required route-tree contacts proven, the design
workflow must continue into:

- `project_write`;
- `writer_correctness`;
- `validation`;
- `kicad_checks`.

If a downstream stage is skipped, the stage must include a precise issue that
names the dependency or policy reason.

### 2. Project Write Evidence

The written project must include the expected KiCad project shape:

- `.kicad_pro`;
- `.kicad_sch`;
- `.kicad_pcb`;
- `.kicad_prl` or project-local state files when the writer normally emits
  them;
- `.kicadai/design-promotion.json`;
- any expected validation artifacts declared by metadata.

The project-write stage must record artifact paths relative to the generated
project root and must not rely on temp-only paths that break replay.

### 3. Writer Correctness Evidence

Writer correctness must verify at least:

- schematic and PCB parse/readback succeeds through KiCadAI readers;
- component identity properties survive write/readback;
- schematic-to-PCB transfer net names are preserved;
- pad net names match expected generated nets;
- copper net names and net codes read back consistently;
- route-tree-generated copper remains assigned to the intended VCC/GND/SDA/SCL
  nets;
- board outline is present;
- footprint references and library links are resolvable according to current
  resolver policy.

Writer correctness failures must be actionable. They must include stage, path,
ref/net where available, and the artifact that failed.

### 4. Board Validation Evidence

Board validation must verify:

- no unrouted required VCC/GND/SDA/SCL endpoints remain;
- every required connector/sensor/pull-up/decoupling endpoint maps to the
  intended net;
- route completion and contact graph summaries remain consistent after project
  write/readback;
- board outline and design-rule baseline are present;
- zones, if absent, are reported as absent only when not required by the fixture
  profile.

### 5. KiCad Check Policy

The fixture must support two modes:

- default local-test mode without KiCad CLI:
  - KiCad checks are skipped or not-run with explicit external-evidence status;
  - the fixture may reach structural `candidate` if all internal gates pass.
- configured KiCad mode:
  - `KICADAI_KICAD_CLI` and required policy execute ERC/DRC;
  - ERC/DRC report artifacts are recorded;
  - warnings or failures are surfaced as promotion gates, not hidden in logs.

### 6. Promotion Metadata

The fixture metadata must reflect the new truth:

- no known gap may mention VCC/GND/SDA/SCL route-tree contact proof as active;
- `expected_fail` may remain only while downstream evidence blocks promotion;
- once internal downstream gates pass in default mode, readiness should move to
  `candidate` if local KiCad is optional;
- `pass` is reserved for required ERC/DRC and writer round-trip success.

### 7. Regression Protection

Tests must fail if:

- route completion regresses below 12 proven contacts;
- project-write is skipped after routing succeeds;
- writer-correctness misses pad/copper net mismatches;
- validation ignores disconnected pads or missing outlines;
- metadata says `candidate` while required gates are blocked;
- optional KiCad evidence is silently absent instead of explicitly skipped.

## Acceptance Criteria

The project is complete when:

- the I2C fixture reaches project-write after complete route-tree proof;
- writer-correctness and validation produce deterministic evidence for the
  generated I2C project;
- the promotion report clearly separates route completion pass, downstream
  internal gates, and KiCad external evidence;
- metadata readiness matches actual achieved readiness;
- default tests remain KiCad-independent;
- configured KiCad tests can be run locally without changing fixture metadata;
- docs identify the next blocker after this work.

## Open Questions

- Does project-write currently skip because routing status is `warning`,
  because expected-fail metadata short-circuits, or because a downstream gate
  still treats fixed-net notices as blockers?
- Are missing-net-class warnings acceptable for structural `candidate`, or must
  the fixture define explicit net classes before promotion?
- Should KiCad ERC/DRC be required for `candidate`, or only for `pass`, for this
  block-composition tier?
- Does the generated I2C PCB need copper zones to satisfy DRC once KiCad checks
  run, or are explicit routes sufficient for this fixture?
