# Validation Repair Loop

KiCadAI includes a deterministic validation-repair foundation for AI agents.
It classifies validation issues, builds a bounded repair plan, can apply safe
transaction-level repairs in memory, and requires revalidation before reporting
a design as repaired.

## Safety Model

Repair is disabled unless explicitly requested. Planning is read-only. Persisted
apply mode is guarded by `--execute`, generated-project provenance, and
`--overwrite` when replacing an existing project. The runner refuses to report
`repaired` without validation evidence.

Generated-project provenance means the repair bundle carries the generated
transaction history and the target project is recognized as KiCadAI-generated
rather than imported or preservation-only user content.

Current safe repair actions are transaction scoped:

- assign a verified footprint when evidence is available;
- regenerate generated pad net hints from known pad-net evidence;
- generate or replace a board outline from board dimensions;
- replace generated placement operations for targeted refs;
- replace generated route operations for targeted nets.

Repairs that need KiCad to refill zones, preserve unknown user-authored nodes,
or resolve unsupported imported objects are reported as skipped or blocked.

## CLI

Target-based persisted apply is the project mutation path for integrations that
already produce a repair bundle with generated transaction provenance. CLI-only
users should use stage-issue planning until a bundle export command is added.
The repair request is provided to the existing `--request` flag as a bundle
file; `repair plan` does not create a bundle from stage issues by itself:

```sh
kicadai --json \
  --target ./out/project \
  --request ./path/to/generated-repair-bundle.json \
  repair plan

kicadai --json --execute --overwrite \
  --target ./out/project \
  --request ./path/to/generated-repair-bundle.json \
  repair apply
```

`--overwrite` authorizes replacement of KiCadAI-managed generated files through
transaction replay. Stale cleanup is manifest-backed and should not delete
unmanaged user files, but imported or preservation-only projects remain blocked.

Plan repairs from captured stage issues without a project target. This emits a
plan report; it does not create the repair bundle required by target-based
persisted apply:

```sh
kicadai --json \
  --request ./examples/repair/missing_footprint_stage_issues.json \
  repair plan
```

Legacy stage-issue apply mode is intentionally gated and reports the
transaction-level repair result without selecting a project target:

```sh
kicadai --json --execute \
  --request ./examples/repair/missing_footprint_stage_issues.json \
  repair apply
```

Imported or preservation-only projects remain blocked until preservation-aware
mutation is explicit.

## Workflow Integration

`design create` can opt into a `validation_repair` stage through
`designworkflow.CreateOptions.Repair`. Plan mode summarizes planned, skipped,
and blocked repair attempts after writer correctness, validation, and KiCad
checks. Apply mode builds an in-memory repair bundle from the generated
transaction, replays repaired output, and reports final validation evidence.

## Status Values

- `not_needed`: no issues were provided.
- `planned`: deterministic repair actions are available.
- `repaired`: repairs were applied to the active repair target and
  revalidation passed. For the current CLI this is plan/report state only; it
  does not mean a KiCad project directory was persisted unless a caller wires
  the lower-level executor to project writing.
- `partial`: at least one repair or improvement happened, but issues remain.
- `skipped`: repair was disabled or excluded by policy.
- `blocked`: no safe deterministic repair path exists.

## Current Limits

- KiCad zone refill is classified but not executed without explicit KiCad CLI
  integration.
- Preservation-aware repairs for unsupported imported KiCad nodes remain
  blocked.
- Post-write validators currently include transaction validation plus optional
  adapters; broader writer, board, and KiCad-backed validation adapters should
  keep expanding with new repair cases.
