# KiCad-Backed Design Examples Specification

Date: 2026-06-27

## Summary

KiCadAI now treats `examples/design/*.json` as executable regression fixtures
for the `design create` workflow. Those default examples are intentionally
small and KiCad-independent. The next step is an optional KiCad-backed design
example tier that can prove richer generated projects with real `kicad-cli`
ERC/DRC evidence when explicitly configured.

This project promotes selected full-design requests into
`examples/design/kicad-backed/`. These examples should exercise multi-block
schematic generation, component selection, PCB realization, placement, routing,
writer correctness, board validation, and real KiCad checks, while preserving a
fast default `go test ./...` path that skips the tier when KiCad is not
configured.

## Problem

The current public design examples prove that the workflow can generate
parseable projects, but they do not prove that larger generated boards are
electrically and geometrically clean in KiCad. Richer scenarios, such as an I2C
sensor breakout with connector binding or protection/power-path blocks, are
currently covered by block and intent fixtures rather than default runnable
design examples.

Without an optional KiCad-backed design tier, AI callers cannot clearly answer:

- which complete `design create` requests have real ERC/DRC evidence;
- which generated multi-block designs are blocked by placement, routing,
  writer, library, or DRC gaps;
- whether a richer example was skipped because KiCad is unavailable or failed
  because the generated board is wrong;
- how to reproduce local KiCad evidence for a complete generated design.

## Goals

- Add a first-class optional example tier under
  `examples/design/kicad-backed/`.
- Keep default examples and normal `go test ./...` KiCad-independent.
- Define metadata for each KiCad-backed example so expected acceptance,
  required checks, known limitations, and allowlists are explicit.
- Reuse the existing `designworkflow.Create` pipeline and `KiCadCheckOptions`
  rather than adding a separate runner.
- Require real KiCad ERC/DRC evidence for optional examples when
  `KICADAI_KICAD_CLI` is configured.
- Produce stable test diagnostics that identify blocked workflow stages,
  KiCad report artifacts, issue paths, and suggestions.
- Start with a conservative small set of examples that can be promoted from
  structural candidate to KiCad-backed pass as generated board quality improves.
- Document how humans and AI agents run the optional tier locally.

## Non-Goals

- Do not require KiCad for default tests or CI jobs that do not explicitly
  opt in.
- Do not make every built-in block DRC-clean in this project.
- Do not implement new placement/routing algorithms unless needed to make an
  initial fixture truthfully pass.
- Do not hide DRC failures behind broad allowlists.
- Do not mutate imported user projects.
- Do not guarantee fabrication readiness or manufacturer acceptance.
- Do not add network-dependent component sourcing.

## Current Foundations

The implementation should build on existing work:

- `examples/design/*.json` default regression coverage.
- `TestDesignExamplesOptionalKiCadBackedTier`, which skips unless
  `KICADAI_KICAD_CLI` is set and runs files under
  `examples/design/kicad-backed/*.json`.
- `designworkflow.Create` and `KiCadCheckOptions`.
- Workflow stages for block planning, component selection, schematic
  generation, PCB realization, placement, routing, project write, writer
  correctness, board validation, KiCad checks, and fabrication readiness.
- Block KiCad DRC corpus metadata and optional/required ERC/DRC behavior.
- Board-edge and entry-anchor binding evidence.
- Component identity properties and BOM/fabrication identity extraction.
- Design rationale reports and persisted `.kicadai/workflow-result.json`.

## Directory Layout

KiCad-backed examples live under:

```text
examples/design/kicad-backed/
  README.md
  <example-id>.json
  <example-id>.metadata.json
```

The request JSON is a normal `design create` request. The metadata file defines
test expectations and documentation-only context.

Generated output must not be checked in by default. Tests should write to
temporary directories. Manual runs should use caller-selected output paths such
as `./out/kicad-backed/<example-id>`.

## Metadata Contract

Each optional example must include adjacent metadata:

```json
{
  "id": "sensor_breakout_kicad_smoke",
  "request": "sensor_breakout_kicad_smoke.json",
  "tier": "smoke",
  "readiness": "candidate",
  "acceptance": "erc-drc",
  "require_erc": true,
  "require_drc": true,
  "allowlists": [],
  "expected_artifacts": [
    ".kicadai/workflow-result.json",
    ".kicadai/manifest.json",
    ".kicadai/checks/erc.json",
    ".kicadai/checks/drc.json"
  ],
  "expected_stages": [
    "block_planning",
    "component_selection",
    "schematic",
    "pcb_realization",
    "placement",
    "project_write",
    "writer_correctness",
    "validation",
    "kicad_checks"
  ],
  "known_gaps": [],
  "notes": "Small multi-block design intended to pass real KiCad ERC/DRC."
}
```

### Required Fields

- `id`: stable lowercase identifier matching the request basename.
- `request`: request filename in the same directory.
- `tier`: one of `smoke`, `block-composition`, `routing`, `fabrication`.
- `readiness`: one of `candidate`, `pass`, `expected_fail`, `blocked`.
- `acceptance`: requested workflow acceptance level.
- `require_erc`: true when schematic ERC evidence is required.
- `require_drc`: true when board DRC evidence is required.
- `expected_stages`: workflow stages that must appear.
- `known_gaps`: explicit reasons a candidate is not promoted to pass.

### Readiness Semantics

- `candidate`: expected to run when KiCad is available, but may still expose
  gaps while implementation matures. Candidate failures should fail the
  optional test unless the metadata marks specific expected issues.
- `pass`: must pass all configured workflow and KiCad checks. Unexpected
  findings fail the optional test.
- `expected_fail`: must run far enough to produce the expected blocked evidence
  and must not be mistaken for a skipped test.
- `blocked`: not executed by the optional test by default; documented as a
  future fixture with a precise implementation gap.

The initial implementation should prefer `candidate` and `expected_fail` over
pretending that a complex design is fully proven.

## Initial Example Candidates

The first candidate set should be conservative:

1. `led_indicator_kicad_smoke`
   - One LED indicator block.
   - Routes or explicitly explains any skipped routing.
   - Requires real DRC, optional ERC if current schematic ERC is not clean.
   - Purpose: known-small full pipeline smoke case.

2. `connector_led_kicad_smoke`
   - Connector breakout feeding an LED indicator.
   - Exercises simple cross-block net intent and connector pad binding.
   - Requires real DRC when routing is enabled.

3. `i2c_sensor_breakout_candidate`
   - Sensor plus connector, optional regulator if stable.
   - Exercises multi-block bus nets, pull-ups, connector binding, and
     generated placement/routing.
   - May start as `expected_fail` or `candidate` with narrow known gaps if
     current generic sensor/connector PCB realization still blocks.

4. `protected_power_entry_candidate`
   - USB-C or connector power input with reverse-polarity or ESD protection.
   - Exercises entry-anchor and power-path local-route evidence.
   - May begin as candidate until DRC-clean proof is stable.

## Test Requirements

The optional test tier must:

- skip with a clear message when `KICADAI_KICAD_CLI` is unset;
- skip with a clear message when no optional fixtures exist;
- enumerate request files and metadata deterministically;
- strict-decode request JSON;
- validate metadata shape before running the workflow;
- run `designworkflow.Create` with:
  - explicit output directory;
  - `Overwrite: true`;
  - `KiCadCheckOptions.KiCadCLI` from `KICADAI_KICAD_CLI`;
  - `RequireERC`/`RequireDRC` from metadata;
  - check artifacts under `.kicadai/checks`;
- fail if required stages are missing;
- fail if required stages are blocked unexpectedly;
- read generated schematic and PCB files back with internal readers;
- assert KiCad check artifacts exist when checks pass;
- print stage, issue, artifact, and metadata diagnostics when a fixture fails.

Default `go test ./...` must remain green on systems without KiCad.

## CLI Requirements

No new CLI command is required for the first implementation. Manual runs use
the existing command:

```sh
KICADAI_KICAD_CLI=/path/to/kicad-cli \
kicadai \
  --request examples/design/kicad-backed/led_indicator_kicad_smoke.json \
  --output ./out/kicad-backed/led_indicator_kicad_smoke \
  --overwrite \
  --require-erc \
  --require-drc \
  --keep-artifacts \
  --artifact-dir ./out/kicad-backed/led_indicator_kicad_smoke/.kicadai/checks \
  design create
```

If the command surface already supports equivalent global flags, documentation
should use that existing shape. The implementation should not add duplicate
flags unless a real gap is discovered.

## Allowlist Policy

Allowlists are acceptable only for known KiCad version-specific findings or
temporary expected-fail candidates. They must be:

- metadata-declared;
- narrow by check kind and finding code/message;
- documented with a `known_gaps` entry;
- visible in generated diagnostics;
- removed before promoting a fixture to `pass` unless the finding is genuinely
  acceptable.

Broad substring allowlists are not allowed.

## Documentation Requirements

Add or update:

- `examples/design/kicad-backed/README.md` with fixture list, readiness,
  command examples, environment variables, and interpretation of pass/skip/fail.
- `examples/design/README.md` to link to the optional tier.
- `docs/intent-planning.md` or `docs/kicadai-agent-skill.md` if agent guidance
  should explain when to use optional KiCad-backed examples.
- `specs/ROADMAP.md` after implementation to record promoted examples and
  remaining gaps.

## Acceptance Criteria

- Optional fixture metadata validates deterministically.
- At least one optional fixture exists and runs through the optional tier when
  `KICADAI_KICAD_CLI` is configured.
- Default tests skip the optional tier without KiCad.
- Optional tests fail with actionable diagnostics on unexpected blocked stages
  or KiCad findings.
- Generated schematic/PCB files are read back by internal readers.
- KiCad artifact paths are stable when checks run.
- Documentation clearly distinguishes default examples from optional
  KiCad-backed examples.

## Open Questions

- Which KiCad major/minor version should be treated as the reference for
  promoted `pass` fixtures?
- Should optional design examples reuse block corpus allowlist types directly,
  or keep a smaller metadata-specific allowlist model?
- Should candidate fixtures be run in local developer smoke tests by default
  when `KICADAI_KICAD_CLI` is set, or should a second env var be required to
  avoid long local runs?
- Which multi-block design should be the first promoted `pass` candidate once
  LED-level smoke proof is stable?
