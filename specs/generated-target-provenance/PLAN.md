# Generated Target Transaction Provenance Implementation Plan

## Objective

Persist generated transaction provenance in project targets and use it to make
target-based repair bundle export and apply mutation-ready for generated
projects.

## Implementation Rules

- Commit each phase independently after Prism review.
- Preserve backward compatibility with existing `.kicadai/manifest.json`.
- Keep imported-project mutation blocked.
- Keep default tests independent of KiCad CLI and network access.
- Use deterministic JSON output and stable issue paths.
- Fail closed: missing, stale, malformed, or mismatched provenance blocks
  mutation.

## Phase 1: Provenance Model And Serialization

### Goal

Add a versioned generated transaction provenance model without changing writer
behavior yet.

### Work

- Add a small provenance model, likely in `internal/manifest` or a sibling
  package if separation is cleaner.
- Define:
  - schema constant, for example `kicadai.transaction.provenance.v1`;
  - provenance file relative path `.kicadai/transaction.json`;
  - `TransactionProvenance`;
  - `ProvenanceRef`;
  - deterministic JSON marshal/write/read helpers.
- Include full `transactions.Transaction`.
- Include operation count and operation summaries.
- Validate:
  - schema;
  - project name;
  - transaction presence;
  - transaction validity;
  - operation count consistency;
  - inside-root path policy.

### Tests

- Valid provenance serializes deterministically.
- Missing schema blocks.
- Unsupported schema blocks.
- Invalid transaction blocks.
- Operation count mismatch blocks.
- Relative path helper always resolves inside project root.

### Acceptance

- Provenance files can be written, read, and validated in isolation.

### Commit

```text
Add generated transaction provenance model
```

## Phase 2: Manifest Integration

### Goal

Extend generated manifests to reference and hash transaction provenance while
remaining backward compatible.

### Work

- Extend `internal/manifest.Manifest` with optional `Provenance`.
- Add manifest helper logic to:
  - include `.kicadai/transaction.json` in file hashes;
  - verify provenance hash when present;
  - report stale/missing provenance in `manifest.Status`.
- Keep old manifests readable.
- Add status detail for provenance freshness without changing existing callers
  that only check `Present` and `Stale`.

### Tests

- Old manifest JSON without provenance still reads.
- Manifest write hashes transaction provenance when present.
- Missing transaction provenance marks manifest stale.
- Corrupt transaction provenance marks manifest stale.
- Outside-root provenance path is rejected.

### Acceptance

- `.kicadai/manifest.json` can prove transaction provenance freshness.

### Commit

```text
Track transaction provenance in generated manifests
```

## Phase 3: Transaction Apply Persistence

### Goal

Write transaction provenance whenever KiCadAI writes a generated project.

### Work

- Update `transactions.Apply` manifest path to write
  `.kicadai/transaction.json`.
- Ensure provenance write happens before final manifest write or otherwise
  cannot leave a misleading manifest.
- Add provenance artifact to generated manifest artifacts.
- Add provenance artifact to apply result artifacts if useful for CLI callers.
- Make provenance write failure block generated apply.
- Preserve deterministic operation summaries.

### Tests

- Applying a generated transaction writes `.kicadai/transaction.json`.
- Manifest hashes include `.kicadai/transaction.json`.
- Provenance transaction matches the applied transaction.
- Failed provenance write prevents misleading manifest state.
- Existing transaction apply tests still pass.

### Acceptance

- Every newly generated project carries full transaction provenance.

### Commit

```text
Persist transaction provenance during generated apply
```

## Phase 4: Target Provenance Hydration

### Goal

Teach repair target hydration to load and report generated transaction
provenance from target files.

### Work

- Extend `repair.Target` with provenance summary.
- Add helper to load target manifest and transaction provenance.
- Validate:
  - generated manifest present;
  - manifest not stale;
  - transaction provenance present;
  - transaction validates;
  - project names match where available;
  - unsupported content still blocks mutation.
- Update `repair.HydrateTarget`:
  - use bundle transaction when present;
  - otherwise hydrate from target provenance;
  - block if neither source has a valid transaction.

### Tests

- Target with valid generated provenance is mutable when other gates pass.
- Target without transaction provenance reports `target.transaction`.
- Target with stale manifest blocks.
- Target with malformed provenance blocks.
- Bundle transaction still works for legacy flows.
- Imported target remains blocked.

### Acceptance

- Repair target hydration can reconstruct generated transactions from disk.

### Commit

```text
Hydrate repair targets from transaction provenance
```

## Phase 5: Repair Export Bundle Integration

### Goal

Make `repair export-bundle` embed hydrated transaction provenance when the
caller does not supply a transaction.

### Work

- Update `repair.ExportBundle` to hydrate target provenance.
- If `ExportOptions.Transaction` is nil and target provenance is valid, embed
  the hydrated transaction.
- Set `summary.has_transaction` from the bundle transaction actually used.
- Include target provenance status in export output.
- Keep dry-run behavior non-mutating.
- Ensure execute writes a repair bundle that passes `repair.ParseBundle`.

### Tests

- Export dry-run reports `has_transaction: true` for a generated target.
- Export execute writes a bundle with transaction.
- Export without stage issues still blocks before write.
- Export with missing provenance blocks or reports non-mutation-ready status
  according to target hydration rules.
- Export summary is deterministic.

### Acceptance

- Target-based repair bundle export bridges generated target state into a
  mutation-ready repair bundle.

### Commit

```text
Embed hydrated transaction provenance in repair bundles
```

## Phase 6: CLI Goldens And End-To-End Repair Flow

### Goal

Lock the new provenance workflow through CLI tests and a small generated
fixture.

### Work

- Add or update CLI golden tests for:
  - `repair plan --target <generated>`;
  - `repair export-bundle --target <generated> --request stage-issues.json`;
  - `repair export-bundle --execute --overwrite`;
  - `repair apply --execute --overwrite --target --request exported-bundle`.
- Add fixture helpers that generate targets through existing transaction apply
  instead of hand-writing KiCad files.
- Add negative fixtures:
  - missing transaction provenance;
  - malformed transaction provenance;
  - stale manifest hash.
- Normalize volatile paths in goldens as existing tests do.

### Tests

- Focused repair CLI tests pass.
- `go test ./cmd/kicadai ./internal/repair ./internal/manifest ./internal/transactions`
  passes.

### Acceptance

- The complete target provenance repair flow is covered at CLI level.

### Commit

```text
Add generated provenance repair CLI goldens
```

## Phase 7: Documentation And Roadmap

### Goal

Document generated target provenance and move the roadmap to the next item.

### Work

- Update README repair section:
  - generated projects now include `.kicadai/transaction.json`;
  - repair export can hydrate transaction provenance;
  - repair apply still blocks imported or stale targets;
  - legacy generated projects without provenance must be regenerated or supplied
    with a bundle transaction.
- Update `specs/ROADMAP.md`:
  - mark generated target transaction provenance as implemented;
  - make the next priority generated block-local placement semantics movable
    under retry.
- Add implementation notes to this plan if scope narrows.

### Tests

- `go test ./...`

### Acceptance

- Documentation matches the implemented generated provenance workflow.

### Commit

```text
Document generated target provenance
```

## Final Verification

After all phases:

```sh
go test ./...
```

Run Prism on the final staged docs/code before the last commit and resolve all
high/medium actionable findings.

## Expected Final State

- New generated projects carry `.kicadai/transaction.json`.
- `.kicadai/manifest.json` proves provenance freshness.
- `repair plan --target` reports target provenance state.
- `repair export-bundle --target` can write a mutation-ready bundle without
  caller-supplied transaction data.
- `repair apply --target --request exported-bundle.json` can safely mutate a
  generated project when validation gates pass.
- Missing, stale, malformed, imported, or unsupported targets remain blocked
  with structured issues.
