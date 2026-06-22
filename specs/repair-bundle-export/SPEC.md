# Repair Bundle Export Command Specification

## 1. Purpose

Add a dedicated CLI command that exports a persisted repair bundle for generated
projects outside the `design create --repair-apply` path.

Today, generated repair bundles are written only as a side effect of the
`design create` persisted `validation_repair` stage. That works for fully
orchestrated design creation, but it leaves a gap for other workflows:

- a project was generated earlier and later validated;
- validation issues were produced by `writer check`, `validate board`,
  `check erc`, `check drc`, or external tooling;
- an agent has structured stage issues and wants a durable repair handoff;
- a user wants to inspect or archive the repair bundle before applying it;
- `repair apply --target` needs a bundle but the project did not come from a
  fresh `design create --repair-apply` run.

This project adds an explicit `repair export-bundle` command that gathers target
ownership, stage issue evidence, optional transaction evidence, and repair
policy into a parseable `kicadai.repair.bundle.v1` file.

## 2. Goals

The implementation must:

- expose a CLI command for exporting repair bundles from generated targets;
- keep imported-project mutation blocked unless preservation safety is proven
  in a future project;
- accept stage issues from an explicit JSON file;
- optionally hydrate target/project metadata from the target inspection result;
- optionally hydrate transaction evidence from a manifest or explicit
  transaction file when available;
- write a valid repair bundle through the existing `repair.SaveBundle` path;
- return a structured JSON report with bundle path, normalized summary fields,
  target metadata, and artifacts;
- make the exported bundle immediately consumable by `repair apply --target`;
- add deterministic CLI goldens for export, parse, and apply-from-export;
- keep default tests independent of real KiCad CLI, network access, and global
  KiCad library roots.

## 3. Non-Goals

This project does not:

- add new repair executors;
- change repair classification policy;
- implement imported-project mutation;
- infer repair issues by running all validators automatically;
- require real KiCad ERC/DRC in default tests;
- implement natural-language repair planning;
- add a new bundle schema version unless the current schema cannot represent the
  required evidence.

## 4. Current Baseline

Implemented foundations include:

- `repair.Bundle`, `repair.LoadBundle`, `repair.ParseBundle`, and
  `repair.SaveBundle`;
- `repair apply --target --request <bundle>` generated-project apply flow;
- generated-project ownership detection through target hydration and manifests;
- generated-project overwrite gating;
- post-apply validation adapters and issue deltas;
- `design create --repair-apply` persisted `validation_repair` stage that writes
  `.kicadai/repair-bundle.json`;
- CLI goldens covering generated bundle parseability, target apply,
  validation summaries, delta statuses, and optional/required missing KiCad DRC
  policy.

Current gaps:

- no command can export a bundle from an already generated target;
- no CLI contract exists for “stage issues + generated target -> bundle”;
- no default path convention exists for an exported bundle outside
  `design create`;
- transaction evidence cannot yet be explicitly supplied to bundle export;
- apply-from-export is not covered end to end.

## 5. CLI Contract

### 5.1 Command

The command should be:

```sh
kicadai --json \
  --target ./out/project \
  --request ./stage-issues.json \
  --output ./out/project/.kicadai/repair-bundle.json \
  repair export-bundle
```

`--json` is required for the initial implementation, matching other structured
CLI families.

### 5.2 Required Inputs

- `--target`: generated KiCad project directory or generated project file.
- `--request`: stage issues JSON file.

The stage issues file should reuse the existing repair stage issues input model
accepted by `repair plan`:

```json
[
  {
    "stage": "writer_correctness",
    "issues": [
      {
        "code": "INVALID_NET_ASSIGNMENT",
        "severity": "error",
        "path": "board.footprints.R1.pads.1",
        "message": "PCB pad references missing net code",
        "refs": ["R1"],
        "nets": ["SIG"]
      }
    ]
  }
]
```

If current `loadRepairStageIssues` also supports wrapper objects, the export
command should preserve that support. The command must reject empty or
malformed issue inputs with structured JSON issues.

### 5.3 Optional Inputs

- `--output`: bundle destination path. If omitted, default to
  `<target-root>/.kicadai/repair-bundle.json`.
- `--overwrite`: allow replacing an existing bundle.
- `--execute`: required to write the bundle. Without `--execute`, return a
  dry-run plan and the would-write path without mutating files.
- `--max-repair-attempts`: carry retry policy into `repair_options`.
- `--seed`: persist deterministic seed policy where relevant.

Future-compatible options:

- `--transaction`: explicit transaction JSON path. This may be added if the
  current CLI global flag set grows a transaction path option. If it is not
  implemented in the first pass, the spec requires the data model to leave room
  for it.
- `--from-report`: validation report JSON path that can be converted into stage
  issues. This is a future convenience, not required in the first
  implementation.

### 5.4 Output JSON

The result should use the standard `reports.Result` envelope:

```json
{
  "ok": true,
  "command": "repair",
  "data": {
    "target": {
      "path": "./out/project",
      "root": "./out/project",
      "kind": "project_dir",
      "generated": true,
      "mutable": true
    },
    "bundle_path": "./out/project/.kicadai/repair-bundle.json",
    "dry_run": false,
    "summary": {
      "stage_count": 1,
      "issue_count": 3,
      "blocking_count": 2,
      "has_transaction": false,
      "generated": true
    }
  },
  "issues": [],
  "artifacts": [
    {
      "kind": "validation_report",
      "path": "./out/project/.kicadai/repair-bundle.json",
      "description": "repair bundle"
    }
  ]
}
```

Selected field names are part of the CLI contract:

- `data.target`;
- `data.bundle_path`;
- `data.dry_run`;
- `data.summary.stage_count`;
- `data.summary.issue_count`;
- `data.summary.blocking_count`;
- `data.summary.has_transaction`;
- `data.summary.generated`;
- `artifacts[].path`.

## 6. Bundle Contents

The exported bundle must use the existing schema:

```json
{
  "schema": "kicadai.repair.bundle.v1",
  "project_root": "./out/project",
  "project_name": "project",
  "generated": true,
  "transaction": null,
  "stage_issues": [],
  "repair_options": {}
}
```

Required fields:

- `schema`;
- `project_root`;
- `project_name`;
- `generated`;
- `stage_issues`;
- `repair_options`.

`transaction` should be included when known and valid. If transaction evidence is
missing, the bundle may still be exported, but `repair apply --target` may be
blocked for repairs that require transaction replay. The CLI result must make
`has_transaction` visible so agents can decide whether an apply attempt is
expected to work.

## 7. Safety Rules

The command must:

- block if `--target` does not exist;
- block if the target is imported or not generated;
- block if generated ownership cannot be established;
- block if `--request` contains no stage issues;
- block if the output path escapes the generated project root unless an
  explicit future flag permits external bundle export;
- require `--overwrite` before replacing an existing bundle;
- require `--execute` before writing files;
- never mutate schematic, PCB, project, or library files;
- write only the bundle path and needed parent `.kicadai` directory.

Imported targets should return a structured blocking issue with a message that
explains imported-project repair remains unsupported.

## 8. Path Policy

Default output path:

```text
<target-root>/.kicadai/repair-bundle.json
```

If `--output` is provided:

- relative paths should resolve against the current working directory;
- output must be inside the generated target root for the first
  implementation;
- parent directories may be created only under the target root;
- returned paths should use slash separators;
- tests should normalize temp roots before assertions.

## 9. Stage Issue Policy

The export command should preserve issue fields exactly enough for later repair
classification:

- `code`;
- `severity`;
- `path`;
- `message`;
- `uuid`;
- `refs`;
- `nets`;
- `suggestion`;
- `operation_id`.

The command should not deduplicate, rewrite, or downgrade issue severity during
export. It may compute summary counts.

## 10. Repair Options Policy

Exported `repair_options` should match `repairOptionsFromCLI(opts, true)` or a
bundle-export-specific equivalent with:

- `enabled: true`;
- `apply: true`;
- `max_attempts` from `--max-repair-attempts`;
- `max_attempts_per_issue` normalized consistently with target apply;
- enabled deterministic repair categories matching target apply policy.

This makes exported bundles behave like bundles created by
`design create --repair-apply`.

## 11. Testing Requirements

Default tests must cover:

- export dry-run returns the intended path and does not write a bundle;
- export execute writes a parseable bundle;
- default path is `<target>/.kicadai/repair-bundle.json`;
- explicit output path inside target root works;
- existing bundle blocks without `--overwrite`;
- missing target blocks;
- imported or non-generated target blocks;
- malformed stage issue JSON blocks;
- empty stage issue JSON blocks;
- exported bundle can be passed to `repair apply --target`;
- output paths are normalized in assertions.

Tests must not require real KiCad CLI.

## 12. Documentation Requirements

README must document:

- `repair export-bundle`;
- required flags;
- dry-run vs execute behavior;
- default bundle path;
- generated-target-only safety policy;
- apply-from-export command sequence.

`specs/ROADMAP.md` must move the dedicated bundle export command from remaining
work to implemented foundation when complete.

## 13. Acceptance Gates

This project is complete when:

- `repair export-bundle` writes valid `kicadai.repair.bundle.v1` files for
  generated targets;
- exported bundles parse through `repair.LoadBundle`;
- exported bundles are consumable by `repair apply --target`;
- imported targets and unsafe output paths are blocked;
- CLI JSON fields are stable and covered by tests;
- README and roadmap reflect the new command.

