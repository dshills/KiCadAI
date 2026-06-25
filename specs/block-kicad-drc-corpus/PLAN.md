# KiCad-Backed Block DRC Corpus Implementation Plan

Date: 2026-06-25

## Objective

Build an opt-in KiCad-backed corpus for generated circuit block projects so
KiCadAI can report which reusable blocks have real ERC/DRC evidence, which are
skipped locally, and which remain expected failures or blocked by known gaps.

Each phase should be implemented, reviewed with Prism, and committed before
moving to the next phase.

## Phase 1: Corpus Manifest Model

### Goal

Add corpus metadata to the block verification manifest model without changing
normal verification behavior.

### Work

- Add an `expected.kicad_corpus` manifest section or an equivalent nested Go
  model that can represent:
  - `include`;
  - `tier`;
  - `readiness`;
  - `expected_status`;
  - `requires_erc`;
  - `requires_drc`;
  - `allowed_codes`;
  - `expected_issues`;
  - `notes`.
- Define stable enums or constants for corpus tiers, readiness values, and
  result statuses.
- Validate:
  - included corpus cases have a valid tier;
  - included corpus cases have a valid readiness;
  - included corpus cases have a valid expected status;
  - pass-candidate PCB cases require either DRC evidence or an explicit
    expected-fail/blocking readiness;
  - expected-fail and blocked cases include explanatory notes.
- Preserve backward compatibility for all existing manifests.
- Add manifest load/validation tests.

### Tests

```sh
go test ./internal/blocks
```

### Acceptance

- Existing manifests load unchanged.
- New corpus metadata validates and rejects invalid tier/readiness/status
  values.
- No corpus behavior changes unless metadata is present.

### Commit

`Add block KiCad corpus manifest metadata`

## Phase 2: Corpus Selection And Summary Model

### Goal

Add deterministic corpus selection and report aggregation independent of the
real KiCad CLI.

### Work

- Add options to the verification runner for:
  - corpus mode enabled;
  - tier filters;
  - strict/required KiCad evidence policy.
- Select only included corpus manifests when corpus mode is enabled.
- Mark non-selected manifests as absent from corpus summaries rather than
  silently mixing them into corpus results.
- Add corpus result and corpus summary models with:
  - counts by status;
  - counts by tier;
  - counts by block ID;
  - selected case IDs;
  - skipped/blocked reasons;
  - KiCad context when available.
- Ensure stable sort order by case ID and stable map serialization where
  existing JSON helpers allow it.
- Add tests for selection, tier filtering, and summary counts.

### Tests

```sh
go test ./internal/blocks
```

### Acceptance

- Corpus selection is deterministic.
- Tier filters include only matching cases.
- Summary counts are stable and correct.
- Existing suite verification without corpus mode is unchanged.

### Commit

`Add block KiCad corpus selection summaries`

## Phase 3: Corpus KiCad Evidence Policy

### Goal

Wire corpus mode into existing optional/required ERC/DRC behavior and preserve
fail-closed semantics for required evidence.

### Work

- Reuse existing KiCad CLI discovery, command execution, and report parsing.
- Map corpus metadata to effective ERC/DRC requirements:
  - `requires_erc` and `requires_drc` strengthen stage requirements for the
    case;
  - CLI `--require-erc` and `--require-drc` still globally strengthen
    requirements;
  - optional unavailable KiCad produces `skip`;
  - required unavailable KiCad produces `blocked`.
- Classify findings:
  - no unexpected findings -> `pass`;
  - expected-fail case with expected findings -> `expected_fail`;
  - unexpected findings in pass candidate -> `blocked`;
  - unavailable optional evidence -> `skip`.
- Preserve allowed-code and expected-issue behavior.
- Add fake-runner tests for pass, skip, blocked, unexpected finding,
  allowlisted finding, and expected-fail result classification.

### Tests

```sh
go test ./internal/blocks
```

### Acceptance

- Corpus result status reflects KiCad evidence policy.
- Required KiCad evidence fails closed.
- Optional KiCad evidence skips cleanly.
- Expected-fail cases are reportable without failing exploratory runs.

### Commit

`Classify block KiCad corpus evidence`

## Phase 4: CLI Flags And Artifact Layout

### Goal

Expose the corpus through `kicadai block verify` and write stable corpus
artifacts when an output directory is provided.

### Work

- Add CLI flags:
  - `--kicad-corpus`;
  - `--kicad-corpus-tier <tier>` repeatable or comma-separated;
  - optional `--kicad-corpus-strict` if needed beyond existing
    `--require-erc` and `--require-drc`.
- Keep existing `block verify` behavior unchanged without `--kicad-corpus`.
- Write:
  - `<output>/corpus-summary.json`;
  - `<output>/<case-id>/reports/corpus-result.json`;
  - links or artifact descriptors for ERC, DRC, writer, and board validation
    reports where they already exist.
- Ensure output overwrite behavior follows current block verification policy.
- Add CLI golden tests for:
  - corpus selection output;
  - tier filtering output;
  - optional unavailable KiCad skip output;
  - required unavailable KiCad blocked output.

### Tests

```sh
go test ./cmd/kicadai ./internal/blocks
```

### Acceptance

- Users can run corpus mode from the compiled `kicadai` binary.
- JSON output includes corpus summary and per-case statuses.
- Artifact paths are deterministic.
- Existing CLI goldens remain valid.

### Commit

`Expose block KiCad corpus CLI`

## Phase 5: Initial Corpus Manifests

### Goal

Populate the first small corpus honestly, with smoke candidates and expected
failures where appropriate.

### Work

- Add corpus metadata to at least:
  - `led_indicator_default`;
  - `connector_breakout_4pin`.
- Evaluate whether additional existing manifests should enter as:
  - `candidate`;
  - `expected_fail`;
  - `blocked`.
- Prefer conservative readiness:
  - do not mark a case as a pass candidate unless internal board validation is
    already meaningful;
  - use expected-fail/blocked notes for known routing, footprint, or local DRC
    gaps.
- Add tests that built-in corpus selection finds the expected cases.
- Add a corpus inventory test that included cases have notes where required and
  stable IDs.

### Tests

```sh
go test ./internal/blocks
```

### Acceptance

- At least two smoke corpus cases are checked in.
- Expected-fail or blocked cases explain the known gap.
- Built-in corpus inventory is deterministic.

### Commit

`Seed block KiCad DRC corpus manifests`

## Phase 6: Opt-In Real KiCad Integration Test

### Goal

Add a local integration test path that can run the corpus against real KiCad
without affecting default CI or developer tests.

### Work

- Add or extend an opt-in test gated by `KICADAI_RUN_KICAD_CLI=1`.
- Use `KICADAI_KICAD_CLI` when set, then existing KiCad CLI discovery.
- Run only the smoke corpus by default.
- Keep artifacts under a temp or configured output directory that does not
  trigger the known KiCad working-directory issue.
- Skip with a clear message when KiCad is not configured.
- Fail when smoke corpus pass candidates produce unexpected findings.
- Include a short helper command in docs for local runs.

### Tests

Default:

```sh
go test ./internal/blocks
```

Opt-in:

```sh
KICADAI_RUN_KICAD_CLI=1 \
KICADAI_KICAD_CLI=/path/to/kicad-cli \
go test ./internal/blocks
```

### Acceptance

- Default tests skip real KiCad.
- Opt-in smoke corpus can run on machines with KiCad configured.
- Missing KiCad produces a skip in optional mode and a block in required mode.

### Commit

`Add opt-in block KiCad corpus test`

## Phase 7: Documentation And Roadmap Update

### Goal

Document the corpus workflow and update project status.

### Work

- Update `docs/circuit-block-verification.md` with:
  - corpus metadata fields;
  - status meanings;
  - CLI examples using `kicadai`;
  - artifact layout;
  - optional versus required KiCad policy.
- Update `README.md` with the current corpus capability.
- Update `specs/ROADMAP.md`:
  - move this gap into implemented foundation once complete;
  - list remaining DRC-clean corpus expansion work.
- Add any focused docs tests or CLI golden updates needed by changed examples.

### Tests

```sh
go test ./...
```

### Acceptance

- Documentation matches the implemented CLI and manifest model.
- Roadmap accurately reflects what the corpus proves and does not prove.
- Full test suite passes.

### Commit

`Document block KiCad DRC corpus`
