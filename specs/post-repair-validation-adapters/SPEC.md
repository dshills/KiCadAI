# Post-Repair Validation Adapters Specification

## 1. Purpose

Make persisted repair success depend on authoritative validation evidence.

This project implements Priority 3 from `specs/ROADMAP.md`: Post-Repair
Validation Adapters.

The repair system can already classify issues, plan deterministic repairs,
mutate generated transactions, replay generated projects, hydrate generated
targets, and run caller-supplied post-apply validators. The remaining gap is
that repair apply still needs first-class built-in validation adapters and
evidence comparison so a repaired status means:

```text
repair was applied -> project was rewritten -> required validators ran ->
blocking issues are gone -> no worsened evidence was introduced
```

## 2. Current Baseline

Implemented foundations include:

- `internal/repair` classifier, planner, executor, runner, persisted apply,
  target hydration, repair bundle model, and golden tests.
- Generated-project ownership checks and imported-project mutation blockers.
- Transaction replay through existing writer paths rather than direct KiCad
  file text surgery.
- `design create` integration with an opt-in `validation_repair` stage.
- CLI support for `repair plan` and `repair apply` with target/bundle flows.
- `internal/writercorrectness` validation for project, schematic, PCB,
  connectivity, pad-net, copper-net, zone-net, and optional round-trip checks.
- `internal/boardvalidation` for PCB outline, pad nets, unrouted nets, route
  completion, zones, and optional DRC evidence.
- `internal/kicadfiles/checks` and `internal/designworkflow` KiCad ERC/DRC
  wrappers where `kicad-cli` is configured.

The main missing piece is a standard adapter set that persisted repair can run
without tests or callers injecting fake validators.

## 3. Goals

This project must:

- add built-in post-apply validation adapters for writer correctness,
  connectivity-first board validation, KiCad ERC, KiCad DRC, and optional
  round-trip checks;
- make adapters deterministic, configurable, and safe when external tools are
  unavailable;
- preserve before/after validation evidence and issue deltas in repair results;
- prevent `repaired` status when blocking issues remain, repeat, or worsen;
- support retry budgets across repair and revalidation attempts;
- make `design create` able to persist repair bundles for reproducible
  target-based apply;
- make CLI target apply usable without caller-supplied validators;
- keep imported/user-authored target mutation blocked unless ownership and
  preservation are explicitly proven.

## 4. Non-Goals

This project does not:

- add arbitrary imported-project mutation;
- implement AI-generated free-form repair patches;
- emulate KiCad zone refill in-process;
- guarantee fabrication readiness;
- replace writer correctness, board validation, ERC, or DRC implementations;
- require KiCad CLI in default unit tests;
- implement natural-language repair planning.

## 5. Validation Adapter Contract

Persisted repair should use a small stable adapter contract.

```go
type PostApplyValidationContext struct {
    OutputDir   string
    Target      repair.Target
    Transaction transactions.Transaction
    Apply       transactions.ApplyResult
}

type PostApplyValidation struct {
    Name      string
    Issues    []reports.Issue
    Artifacts []reports.Artifact
    Skipped   bool
}
```

Adapters must:

- be read-only after project replay;
- never mutate KiCad files except through explicit future KiCad CLI refill
  policy;
- return structured issues and artifacts;
- report skipped status with a reason when an optional dependency is absent;
- use stable adapter names;
- avoid hiding parser/tool failures;
- avoid declaring success from missing evidence.

## 6. Required Built-In Adapters

### 6.1 Transaction Validation Adapter

Status: already present as a baseline validation.

Required behavior:

- run `transactions.Validate` on the final transaction;
- block repair success on transaction validation errors;
- appear first in the validation list.

### 6.2 Writer Correctness Adapter

Required behavior:

- run the existing writer correctness target/project checks against the
  persisted output directory;
- include project structure, schematic parse, PCB parse, generated
  connectivity, schematic-to-PCB transfer, pad-net, copper-net, and zone-net
  checks where available;
- optionally run KiCad round-trip checks when policy enables them;
- return artifacts from the writer correctness subsystem.

Required policy:

- enabled by default for persisted generated-project apply;
- round-trip disabled by default unless requested.

### 6.3 Board Validation Adapter

Required behavior:

- locate the generated `.kicad_pcb` target from the project output;
- run connectivity-first board validation;
- check outline, pads, net assignments, unrouted required nets, route
  completion, zones, and optional strict-zone policy;
- return board validation artifacts and issues.

Required policy:

- enabled by default when a PCB file exists;
- skipped with warning or info when the project is schematic-only.

### 6.4 KiCad ERC Adapter

Required behavior:

- locate the generated root `.kicad_sch`;
- run KiCad ERC through configured `kicad-cli`;
- parse reports through existing check parser;
- preserve report artifacts when requested;
- respect allowlisted/skipped behavior from existing KiCad check policy.

Required policy:

- disabled by default unless requested by workflow/CLI options;
- skipped with explicit issue when requested but CLI is unavailable;
- blocking when required and failing.

### 6.5 KiCad DRC Adapter

Required behavior:

- locate the generated `.kicad_pcb`;
- run KiCad DRC through configured `kicad-cli`;
- parse reports through existing check parser;
- preserve report artifacts when requested;
- respect allowlisted/skipped behavior from existing KiCad check policy.

Required policy:

- disabled by default unless requested by workflow/CLI options;
- skipped with explicit issue when requested but CLI is unavailable;
- blocking when required and failing.

### 6.6 Optional Round-Trip Adapter

Required behavior:

- invoke existing schematic/PCB round-trip correctness where configured;
- compare post-repair output against KiCad-normalized output;
- report diffs as structured blocking issues when required.

Required policy:

- off by default because it may require external KiCad tooling;
- opt-in for release/fabrication readiness workflows.

## 7. Adapter Options

Add a typed options model, either inside `repair` or a small adjacent package.

Required fields:

```go
type PostValidationOptions struct {
    WriterCorrectness bool
    BoardValidation bool
    KiCadERC bool
    KiCadDRC bool
    RoundTrip bool
    RequireKiCadERC bool
    RequireKiCadDRC bool
    RequireRoundTrip bool
    StrictZones bool
    KeepArtifacts bool
    ArtifactDir string
    KiCadCLI string
}
```

Defaults:

- writer correctness: enabled;
- board validation: enabled when PCB exists;
- KiCad ERC/DRC: disabled unless requested;
- round-trip: disabled unless requested;
- strict zones: false;
- artifacts: follow existing CLI/workflow artifact policy.

## 8. Evidence Delta Model

Persisted repair should preserve evidence before and after repair.

Add summary fields such as:

```go
type ValidationDelta struct {
    Before ValidationSummary
    After ValidationSummary
    Cleared []reports.Issue
    Repeated []reports.Issue
    New []reports.Issue
    Worsened bool
}
```

Summary dimensions:

- total issue count;
- blocking issue count;
- error count;
- warning count;
- skipped adapter count;
- adapter statuses;
- artifact count.

Issue matching should be deterministic and conservative. A stable issue key
should include:

- code;
- severity;
- path;
- message;
- refs;
- nets;
- operation ID when present.

## 9. Status Semantics

Persisted repair status must be conservative:

- `not_needed`: no blocking source issues and validators pass.
- `repaired`: at least one repair was applied, all required validators ran or
  explicitly skipped by non-required policy, and no blocking issues remain.
- `partial`: blocking source issue cleared, no worse blocking issue appeared,
  but warnings/skipped optional evidence remain.
- `blocked`: no safe repair was available, mutation was unsafe, validation
  failed, required validation was skipped, issue count worsened, or a blocking
  issue repeated.
- `skipped`: repair disabled or dry-run plan only.

The system must not report `repaired` when:

- writer correctness has blocking issues;
- board validation has blocking issues;
- required KiCad ERC/DRC/round-trip checks are skipped or fail;
- transaction validation fails;
- a blocking issue with the same stable key repeats;
- new blocking issues appear;
- retry budget is exhausted before clean validation.

## 10. Retry Budget Requirements

Repair loops must use bounded budgets:

- total attempts;
- attempts per issue key;
- generate/validate/repair/revalidate cycle count where workflow integration
  needs it;
- optional per-adapter timeout where external KiCad tools are used.

Budget exhaustion must produce a blocked result with:

- consumed budget;
- last attempted issue/action;
- latest validation evidence;
- next suggested action.

## 11. CLI Behavior

Extend target repair behavior so callers can use built-in validators:

```sh
kicadai --json repair plan --target ./out/project
kicadai --json repair apply --target ./out/project --request repair-bundle.json --execute --overwrite
```

Add or confirm flags:

- `--validate-writer`;
- `--validate-board`;
- `--check-erc`;
- `--check-drc`;
- `--require-erc`;
- `--require-drc`;
- `--require-roundtrip`;
- `--strict-zones`;
- `--keep-artifacts`;
- `--artifact-dir`;
- `--kicad-cli`;
- `--max-repair-attempts`;
- `--max-repair-attempts-per-issue`.

Default behavior:

- `repair plan --target` remains read-only;
- `repair apply --target` requires `--execute`;
- writing to an existing project requires `--overwrite`;
- missing provenance blocks mutation;
- imported/preservation-only targets remain blocked;
- built-in writer and board validators run after apply by default.

## 12. Design Workflow Integration

`design create` should be able to emit a reproducible repair bundle.

Required behavior:

- include the normalized design request;
- include the generated transaction;
- include stage issues from writer correctness, board validation, KiCad checks,
  and validation repair stages;
- include repair options;
- include generated-project ownership metadata;
- store bundle as an artifact when requested or when repair apply is enabled.

When workflow repair apply is enabled:

- build the same bundle in memory;
- run persisted repair with built-in validators;
- attach validation delta and artifacts to the `validation_repair` stage;
- preserve current behavior when repair is disabled.

## 13. KiCad Zone Refill Policy

Zone refill must remain explicit.

Initial behavior:

- classify unfilled zones as repairable only if policy allows KiCad CLI use;
- do not mark zone refill repaired unless KiCad CLI refill/check actually ran;
- skip or block based on `RequireKiCadDRC` and strict-zone policy;
- preserve clear diagnostics when KiCad CLI is unavailable.

No in-process fake zone-fill evidence is allowed.

## 14. Acceptance Gates

This project is complete when:

- persisted repair apply has built-in writer correctness and board validation
  adapters;
- optional KiCad ERC/DRC and round-trip adapters are configurable and explicit
  about skipped evidence;
- repair result includes before/after validation summaries and issue deltas;
- `repaired` is only reported when required post-write validation is clean;
- repeated, worsened, or newly introduced blocking issues produce blocked
  status;
- `repair apply --target ...` works without test-injected validators;
- `design create` can emit a repair bundle artifact for reproducible CLI apply;
- tests cover clean repair, repeated issue, worsened issue, skipped external
  tool, missing provenance, and imported target blockers.

## 15. Risks

- Running KiCad CLI in tests can be environment-sensitive; default tests must
  keep external checks injectable or skipped.
- Writer correctness and board validation may disagree on severity; result
  aggregation must remain conservative.
- Repair bundles may grow large if they store full evidence; artifact payloads
  should be bounded and path-based where possible.
- Status names can be misleading if deltas are not clear; every status should
  include evidence summaries.
