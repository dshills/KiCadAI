# Generated Target Transaction Provenance Specification

## Objective

Persist enough transaction provenance inside generated KiCadAI project targets
that later repair, validation, and export workflows can safely reconstruct the
original generated intent without depending on the in-memory `design create`
result.

The immediate goal is to let:

```sh
kicadai --json --target ./out/project --request stage-issues.json repair export-bundle
```

produce a repair bundle with transaction provenance for generated targets, so:

```sh
kicadai --json --execute --overwrite --target ./out/project --request ./out/project/.kicadai/repair-bundle.json repair apply
```

can safely mutate the generated project without requiring the caller to have
saved the original workflow result.

## Problem

KiCadAI already writes `.kicadai/manifest.json` for generated projects. That
manifest proves generated ownership with file hashes and a coarse list of
operation summaries, but it does not persist the full transaction payload.

Repair apply currently requires a repair bundle with:

- `generated: true`;
- a valid `transactions.Transaction`;
- stage issues;
- repair options.

`design create` can write a repair bundle that includes its transaction when
the workflow repair stage runs. Non-workflow target-based repair export can
verify generated ownership, but it cannot currently hydrate the full
transaction from the target itself. As a result, generated projects outside the
original `design create` response are inspectable but not reliably
mutation-ready.

## Scope

This project covers generated project provenance only.

In scope:

- versioned generated transaction provenance artifacts;
- manifest links to provenance artifacts;
- deterministic serialization and hashing;
- target hydration from generated provenance;
- repair bundle export that embeds hydrated transactions;
- repair apply safety checks against stale or mismatched provenance;
- CLI and golden coverage for target-based provenance flows;
- README and roadmap updates.

Out of scope:

- imported-project mutation;
- preservation of arbitrary unsupported KiCad nodes;
- semantic reverse engineering of a transaction from raw KiCad files;
- user-authored project ownership claims;
- natural-language intent planning;
- conflict merging between two independent generated histories.

## Current Foundation

Existing generated provenance:

- `internal/manifest.Manifest`
  - `project_name`;
  - `generator_version`;
  - `operations` summaries;
  - generated artifacts;
  - file hashes.
- `transactions.Apply`
  - writes generated project files;
  - writes `.kicadai/manifest.json`;
  - records operation summaries.
- `repair.Bundle`
  - can carry a full `transactions.Transaction`;
  - validates transactions before save/load.
- `repair.HydrateTarget`
  - blocks mutation unless bundle provenance proves generated ownership and
    includes a transaction.
- `repair.ExportBundle`
  - accepts an optional transaction in `ExportOptions`;
  - currently writes `nil` transaction unless caller supplies one.

## Desired User Workflows

### 1. Generated Project Carries Provenance

After any generated transaction apply:

```text
out/project/
  project.kicad_pro
  project.kicad_sch
  project.kicad_pcb
  .kicadai/
    manifest.json
    transaction.json
```

The generated manifest links to `.kicadai/transaction.json` and includes its
hash.

### 2. Repair Bundle Export Hydrates Transaction

Given a generated target and stage issue JSON:

```sh
kicadai --json \
  --target ./out/project \
  --request ./stage-issues.json \
  repair export-bundle
```

Dry-run output reports:

- target generated status;
- provenance status;
- transaction operation count;
- bundle path;
- `summary.has_transaction: true`.

With `--execute`, the generated repair bundle includes the hydrated transaction.

### 3. Repair Apply Uses Bundle Transaction

Given the exported bundle:

```sh
kicadai --json \
  --execute \
  --overwrite \
  --target ./out/project \
  --request ./out/project/.kicadai/repair-bundle.json \
  repair apply
```

Apply remains blocked if:

- target manifest is stale;
- transaction provenance is missing;
- transaction provenance hash does not match the manifest;
- transaction project does not match target project;
- transaction validation fails;
- unsupported preserved content is present.

### 4. Non-Generated Targets Stay Blocked

Imported or manually edited projects remain non-mutable unless future
preservation support explicitly allows them.

## Provenance Artifact

Add a versioned artifact at:

```text
.kicadai/transaction.json
```

Recommended schema:

```json
{
  "schema": "kicadai.transaction.provenance.v1",
  "project_name": "led_indicator",
  "generator_version": "0.1.0",
  "created_by": "kicadai",
  "transaction": {
    "name": "led_indicator",
    "project": "led_indicator",
    "operations": []
  },
  "operation_count": 0,
  "operation_summaries": [],
  "source": {
    "kind": "transaction_apply",
    "seed": "demo"
  }
}
```

Required fields:

- `schema`;
- `project_name`;
- `generator_version`;
- `transaction`;
- `operation_count`.

Optional fields:

- `created_by`;
- `operation_summaries`;
- `source`;
- future `request_digest`;
- future `workflow_stage_summaries`.

## Manifest Extension

Extend `internal/manifest.Manifest` with explicit provenance metadata while
preserving backward compatibility:

```go
type Manifest struct {
    ProjectName      string
    GeneratorVersion string
    Operations       []OperationSummary
    Artifacts        []reports.Artifact
    FileHashes       map[string]string
    Provenance       *ProvenanceRef `json:"provenance,omitempty"`
}

type ProvenanceRef struct {
    TransactionPath string `json:"transaction_path,omitempty"`
    Schema          string `json:"schema,omitempty"`
    OperationCount  int    `json:"operation_count,omitempty"`
    Hash            string `json:"hash,omitempty"`
}
```

Rules:

- manifests without `provenance` remain readable;
- old manifests can still prove generated ownership for read-only evaluation;
- mutation-oriented repair flows require transaction provenance;
- `FileHashes` must include `.kicadai/transaction.json`;
- `Provenance.Hash` must match the same file content hash if present.

## Provenance Validation

Validation should return structured issues, not panics or prose-only errors.

Required checks:

- provenance artifact path is inside project root;
- file exists;
- JSON decodes;
- schema is supported;
- transaction validates with `transactions.Validate`;
- transaction project/name matches manifest or target project where available;
- operation count matches transaction length;
- manifest file hash matches transaction artifact;
- stale manifest remains blocking for mutation.

Suggested issue paths:

- `provenance.transaction`;
- `provenance.schema`;
- `provenance.hash`;
- `provenance.project`;
- `target.transaction`.

## Target Hydration

Add a target provenance hydration layer in `internal/repair` or a small shared
package. It should:

1. inspect the target;
2. read `.kicadai/manifest.json`;
3. verify generated ownership and non-stale hashes;
4. load `.kicadai/transaction.json`;
5. validate the transaction;
6. expose transaction status in `repair.Target`.

Recommended additions:

```go
type Target struct {
    ...
    Provenance *TargetProvenance `json:"provenance,omitempty"`
}

type TargetProvenance struct {
    Present        bool   `json:"present"`
    Path           string `json:"path,omitempty"`
    Schema         string `json:"schema,omitempty"`
    OperationCount int    `json:"operation_count,omitempty"`
    Valid          bool   `json:"valid"`
}
```

`repair.HydrateTarget` should remain strict:

- if bundle supplies a transaction, use the bundle transaction;
- if bundle lacks a transaction but target provenance is valid, hydrate from
  target provenance;
- if neither exists, block mutation.

## Repair Export Integration

`repair.ExportBundle` should use target provenance when
`ExportOptions.Transaction` is nil:

1. hydrate target;
2. if target has valid transaction provenance, embed that transaction in the
   exported bundle;
3. set `summary.has_transaction` from the actual bundle transaction;
4. include provenance issues when hydration fails;
5. keep dry-run non-mutating.

This allows `repair export-bundle` to become a bridge from generated target
state to future safe repair apply.

## Transaction Apply Integration

`transactions.Apply` should write transaction provenance whenever it writes a
generated project manifest.

Behavior:

- write `.kicadai/transaction.json` atomically before or with manifest update;
- include provenance artifact in manifest artifacts/hashes;
- fail generated apply if provenance cannot be written;
- avoid leaving a manifest that claims complete generated ownership when
  transaction provenance failed;
- keep deterministic JSON formatting.

## CLI Behavior

Existing commands stay the same:

```sh
kicadai --json --target <project> repair plan
kicadai --json --target <project> --request <stage-issues.json> repair export-bundle
kicadai --json --execute --overwrite --target <project> --request <bundle.json> repair apply
```

Expected changes:

- `repair plan --target <generated-project>` reports provenance status;
- `repair export-bundle --target <generated-project>` dry-run reports
  `has_transaction: true` when provenance is valid;
- `repair export-bundle --execute` writes a bundle with full transaction;
- missing provenance remains blocking for mutation.

## Testing Strategy

Unit tests:

- provenance model serialization is deterministic;
- invalid schema blocks;
- operation count mismatch blocks;
- invalid transaction blocks;
- missing provenance blocks mutation;
- stale manifest blocks mutation;
- target provenance hydrates a valid transaction;
- repair export embeds hydrated transaction;
- apply can use exported bundle transaction.

Integration tests:

- generate a small project;
- verify `.kicadai/transaction.json` exists;
- verify `.kicadai/manifest.json` hashes it;
- export repair bundle from target and stage issues;
- apply exported bundle in dry-run and execute modes;
- corrupt transaction provenance and verify structured blocking issue.

CLI golden tests:

- target plan with valid provenance;
- export-bundle dry-run with hydrated transaction;
- export-bundle execute writes bundle containing transaction;
- missing provenance output;
- stale provenance output;
- malformed provenance output.

## Backward Compatibility

Existing generated projects without `.kicadai/transaction.json` should:

- remain inspectable;
- remain evaluable;
- remain fabrication-previewable;
- stay blocked for repair apply until regenerated or manually supplied with a
  valid repair bundle transaction.

Existing manifests should parse without migration.

## Security And Safety

- Never trust transaction provenance just because the file exists.
- Validate hashes through the manifest.
- Keep provenance paths inside target root.
- Do not hydrate transactions from imported targets.
- Do not allow absolute or parent-traversing provenance paths.
- Do not mutate if unsupported KiCad content is detected.
- Keep stage issue input separate from transaction provenance; stage issues do
  not prove ownership.

## Acceptance Criteria

- Generated projects include `.kicadai/transaction.json`.
- Manifest hashes cover the transaction provenance artifact.
- `repair plan --target` reports provenance state.
- `repair export-bundle --target` can embed transaction provenance without
  caller-supplied transaction data.
- `repair apply --target --request exported-bundle.json` can safely mutate a
  generated target when all validation gates pass.
- Missing, stale, malformed, or mismatched provenance blocks mutation with
  structured issues.
- Existing generated projects without provenance remain readable but not
  mutation-ready.
- `go test ./...` passes.

## Open Questions

- Should the provenance artifact include the original normalized
  `designworkflow.Request` when available, or should that wait for intent-level
  planning artifacts?
- Should transaction provenance eventually support an append-only history, or is
  latest generated transaction enough for current repair flows?
- Should repair apply update transaction provenance after successful mutation to
  reflect the repaired transaction as the new generated baseline?

For this phase, persist the latest full transaction and update it after
successful generated repair apply only if the implementation can do so without
weakening safety. Otherwise, document repair-apply provenance refresh as a
follow-up.
