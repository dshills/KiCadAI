# Validation Repair Loop

KiCadAI includes a deterministic validation-repair foundation for AI agents.
It classifies validation issues, builds a bounded repair plan, can apply safe
transaction-level repairs in memory, and requires revalidation before reporting
a design as repaired.

## Safety Model

Repair is disabled unless explicitly requested. Planning is read-only. Apply
mode is guarded by the CLI `--execute` flag and the lower-level runner refuses
to report `repaired` without a validator.

Current safe repair actions are transaction scoped:

- assign a verified footprint when evidence is available;
- regenerate generated pad net hints from known pad-net evidence;
- generate or replace a board outline from board dimensions;
- replace generated placement operations for targeted refs;
- replace generated route operations for targeted nets.

Repairs that need KiCad to refill zones, preserve unknown user-authored nodes,
or resolve unsupported imported objects are reported as skipped or blocked.

## CLI

Plan repairs from captured stage issues:

```sh
go run ./cmd/kicadai --json \
  --request ./examples/repair/missing_footprint_stage_issues.json \
  repair plan
```

Apply mode is intentionally gated:

```sh
go run ./cmd/kicadai --json --execute \
  --request ./examples/repair/missing_footprint_stage_issues.json \
  repair apply
```

The current CLI emits the repair plan/report contract. File mutation is still
performed through the lower-level repair executor/runner integration points.

## Workflow Integration

`design create` can opt into a `validation_repair` stage through
`designworkflow.CreateOptions.Repair`. The stage summarizes planned, skipped,
and blocked repair attempts after writer correctness, validation, and KiCad
checks. Planned repairs are surfaced as pending repair evidence; callers must
still retain the original validation issue severities when deciding whether a
design is acceptable.

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

- The CLI repair command consumes stage-issue JSON and reports plans; it does
  not yet reopen and mutate a KiCad project directory by itself.
- KiCad zone refill is classified but not executed without explicit KiCad CLI
  integration.
- Preservation-aware repairs for unsupported imported KiCad nodes remain
  blocked.
- Full closed-loop workflow mutation needs more project-state hydration so the
  executor can be safely wired into `design create` apply mode.
