# Post-Repair CLI Golden Fixtures Implementation Plan

## Objective

Add deterministic CLI golden fixtures that lock down post-repair validation
summaries, issue deltas, generated repair bundle artifacts, and target apply
evidence for AI-facing workflows.

## Implementation Rules

- Commit each phase independently after Prism review.
- Keep default tests independent of real KiCad CLI, network access, and global
  KiCad library roots.
- Prefer selected-field assertions over full JSON snapshots.
- Normalize paths before comparing CLI output.
- Use small generated fixtures that run quickly in normal `go test`.
- Do not change repair semantics unless a test exposes missing evidence needed
  for the CLI contract.

## Phase 1: CLI Golden Harness

### Goal

Create reusable helpers for running CLI repair/design fixtures and asserting
stable evidence fields.

### Work

- Add CLI test helpers for:
  - running `design create` into `t.TempDir()`;
  - running `repair apply --target` against generated fixtures;
  - decoding JSON results into compact test structs;
  - finding stages by name;
  - reading nested summary fields safely;
  - normalizing output and artifact paths.
- Add a minimal fixture directory for post-repair CLI goldens.
- Add helper to assert that paths stay inside the temp output root.

### Tests

- Missing stage helper fails with useful stage names.
- Path normalization strips temp roots.
- Fixture loader reports missing request/bundle files clearly.

### Acceptance

- Later phases can add repair CLI fixtures without duplicating JSON plumbing.

### Commit

```text
Add post repair CLI golden harness
```

## Phase 2: Generated Repair Bundle Fixture

### Goal

Prove `design create` can emit a generated repair bundle artifact and that the
artifact is parseable.

### Work

- Add a small generated `design create` request with validation repair enabled.
- Run it through the CLI with overwrite into a temp output directory.
- Assert `validation_repair` stage presence when repair is enabled.
- Locate the repair bundle artifact from stage artifacts or output `.kicadai`
  metadata.
- Parse the bundle through existing repair bundle parser/model.
- Assert generated ownership, project root/name, transaction presence, stage
  issues, and repair options.

### Tests

- Bundle artifact exists.
- Bundle parses.
- Bundle is marked generated.
- Bundle paths normalize under output root.
- Disabled repair mode does not emit the bundle artifact.

### Acceptance

- AI callers can rely on a generated bundle handoff from `design create`.

### Commit

```text
Add generated repair bundle CLI golden
```

## Phase 3: Post-Repair Validation Summary CLI Fields

### Goal

Lock down the CLI JSON shape for built-in post-validation adapter summaries.

### Work

- Add selected-field assertions for validation adapter list and counts.
- Cover writer correctness validation summary.
- Cover board validation summary when a PCB exists.
- Cover schematic-only skip behavior if a compact fixture exists.
- Assert adapter names remain stable.
- Assert skipped validators include visible reason/evidence.

### Tests

- CLI JSON includes validation count.
- Writer correctness adapter appears in validation evidence.
- Board validation adapter appears for PCB projects.
- Schematic-only or missing-board case reports explicit skip.
- Blocking adapter issues affect final status.

### Acceptance

- Post-repair validation evidence is stable at the CLI boundary.

### Commit

```text
Add post repair validation summary goldens
```

## Phase 4: Validation Delta CLI Goldens

### Goal

Prove CLI output exposes before/after issue deltas in a machine-readable,
deterministic way.

### Work

- Add fixtures or harness inputs for:
  - cleared issue;
  - repeated blocking issue;
  - new blocking issue;
  - non-blocking residual issue.
- Assert delta fields for cleared, repeated, new, and worsened counts.
- Assert final status rules:
  - clean required validation -> `repaired`;
  - residual non-blocking evidence -> `partial`;
  - repeated/new blockers -> `blocked`.
- Normalize issue keys to avoid temp path drift.

### Tests

- Delta counts match expected fixture metadata.
- Status matches delta severity.
- Top-level CLI `ok` agrees with final repair status policy.

### Acceptance

- AI callers can decide whether a repair improved, stalled, or regressed.

### Commit

```text
Add repair validation delta CLI goldens
```

## Phase 5: Optional And Required KiCad Check Policy

### Goal

Cover external validator policy at the CLI boundary without requiring real
KiCad in default tests.

### Work

- Use existing fake CLI/check helpers where available, or add a small fake
  command fixture.
- Add optional missing KiCad CLI case.
- Add required missing KiCad CLI case.
- Add fake ERC/DRC report case if current harness supports it.
- Assert skipped vs blocking behavior and artifact preservation.

### Tests

- Optional missing KiCad CLI is visible but non-blocking.
- Required missing KiCad CLI blocks.
- Fake report artifacts appear when keep-artifacts is enabled.
- No default test invokes real KiCad.

### Acceptance

- External validation policy is test-covered for AI-facing CLI flows.

### Commit

```text
Add KiCad validation policy CLI goldens
```

## Phase 6: Target Apply Bundle Flow

### Goal

Prove `repair apply --target` can consume generated repair evidence and expose
built-in post-validation results.

### Work

- Use the generated bundle fixture or a compact checked-in bundle fixture.
- Run `repair apply --target` with built-in validators.
- Assert overwrite policy and generated ownership gates.
- Assert validation summaries and deltas appear in CLI JSON.
- Add imported/preservation-only negative case if an existing fixture is
  available.

### Tests

- Target apply with generated bundle runs validators.
- Missing overwrite blocks when output exists.
- Imported target mutation remains blocked.
- Validation delta is present and deterministic.

### Acceptance

- CLI target repair is covered end to end for generated projects.

### Commit

```text
Add repair target apply CLI golden
```

## Phase 7: Documentation And Roadmap Update

### Goal

Document the CLI evidence contract and move the roadmap to the next item.

### Work

- Update README repair section with:
  - validation summary fields;
  - validation delta interpretation;
  - repair bundle location;
  - rerun commands for generated bundle/target flows.
- Update `specs/ROADMAP.md`:
  - mark post-repair CLI goldens as implemented;
  - make the next item the dedicated repair bundle export command for
    non-`design create` flows unless implementation changed the ordering.
- Add a short note to this spec or plan if any evidence gaps remain.

### Tests

- `go test ./cmd/kicadai ./internal/designworkflow ./internal/repair`
- `go test ./...` before final commit.

### Acceptance

- Docs match implemented CLI behavior and roadmap points to the next priority.

### Commit

```text
Document post repair CLI goldens
```

## Final Verification

Run:

```sh
go test ./...
prism review staged
```

The project is ready to move on when default tests prove post-repair validation
summaries, issue deltas, and generated repair bundles are stable in CLI output.

