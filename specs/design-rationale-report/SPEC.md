# Design Rationale And Known-Limit Report Specification

Date: 2026-06-26

## 1. Summary

Build an AI-facing rationale report that explains how KiCadAI turned user
intent into a generated design, what evidence supports each decision, and which
limits or validation findings remain.

The current system can draft natural-language intent, plan structured
requirements, generate projects, validate outputs, and report workflow stages.
However, the evidence is spread across many artifacts:

- `intent-draft.json`
- `intent-extraction.json`
- `intent-clarifications.json`
- `intent-plan.json`
- `generated-request.json`
- design workflow result JSON
- validation, repair, fabrication, and writer-correctness summaries

This project adds a consolidated `design-rationale.json` artifact and compact
CLI output so AI callers can answer:

- What did KiCadAI think the user asked for?
- Which blocks, parts, nets, constraints, and validation policies were selected?
- Why were they selected?
- What assumptions were made?
- What is known-good, partially proven, or blocked?
- What should be clarified or improved next?

## 2. Goals

- Provide a single deterministic report for AI review after `intent plan`,
  `intent explain`, and `intent create`.
- Link decisions back to source evidence from natural-language draft extraction
  when present.
- Preserve existing planner and workflow artifacts without replacing them.
- Normalize known limits, assumptions, warnings, and blocking issues into
  machine-readable categories.
- Distinguish:
  - user request interpretation;
  - planner decisions;
  - component/block evidence;
  - schematic/PCB workflow status;
  - validation proof;
  - unresolved limits.
- Make partial or blocked designs easier for an AI to explain and repair.

## 3. Non-Goals

- No new circuit synthesis.
- No live LLM integration.
- No MCP server.
- No replacement for `intent-plan.json`, validation reports, or repair bundles.
- No attempt to prove fabrication readiness beyond existing readiness gates.
- No mutation of generated projects beyond writing report artifacts.

## 4. User-Facing CLI

Add a report command under the existing `intent` family:

```text
kicadai --json --request ./intent.json --output ./out intent rationale
kicadai --json --text "make a 3.3V I2C temperature sensor breakout" intent rationale
kicadai --json --target ./out/project intent rationale
```

### 4.1 Inputs

The command should support three sources:

- `--request`: structured intent JSON.
- `--text` or `--file`: natural-language input through `intentdraft`.
- `--target`: generated project directory containing `.kicadai/` artifacts.

Only one source mode may be used at a time.

### 4.2 Output

Without `--output`, the command prints a report result to stdout.

With `--output`, the command writes:

- `design-rationale.json`

If `--target` points to a generated project, the report should be written under:

- `.kicadai/design-rationale.json`

The command must not create or modify KiCad schematic/PCB/project files.

## 5. Report Schema

Add an internal package, likely `internal/rationale`, with a top-level report:

```go
type Report struct {
    Schema          string              `json:"schema"`
    Status          Status              `json:"status"`
    Source          SourceSummary       `json:"source"`
    Intent          IntentSummary       `json:"intent"`
    Decisions       []Decision          `json:"decisions,omitempty"`
    Evidence        []EvidenceRecord    `json:"evidence,omitempty"`
    Assumptions     []RationaleNote     `json:"assumptions,omitempty"`
    Clarifications  []RationaleNote     `json:"clarifications,omitempty"`
    KnownLimits     []KnownLimit        `json:"known_limits,omitempty"`
    Validation      ValidationSummary   `json:"validation,omitempty"`
    NextActions     []NextAction        `json:"next_actions,omitempty"`
    ArtifactRefs    []reports.Artifact  `json:"artifact_refs,omitempty"`
}
```

Schema:

- `kicadai.design.rationale.v1`

Status values:

- `ready`: no blocking issues and workflow evidence reached requested
  acceptance.
- `partial`: no blocking planner issue, but validation/workflow evidence is
  incomplete or warnings remain.
- `needs_clarification`: draft or planner requires user clarification.
- `blocked`: blocking issue prevents safe generation or proof.

## 6. Source Summary

`SourceSummary` should describe the report input:

```go
type SourceSummary struct {
    Mode       string `json:"mode"` // request, text, file, target
    Path       string `json:"path,omitempty"`
    SourceHash string `json:"source_hash,omitempty"`
    Summary    string `json:"summary,omitempty"`
}
```

For natural-language input, copy `intentdraft.ExtractionReport.SourceHash` and
summary.

For target input, derive source from available `.kicadai/` artifacts:

1. `intent-extraction.json`, if present;
2. `intent-draft.json`, if present;
3. `intent-plan.json`, if present;
4. generated manifest/provenance files, where available.

If no supported artifacts exist, fail with a blocking issue explaining that the
target lacks rationale source artifacts.

## 7. Intent Summary

`IntentSummary` should summarize:

- request name;
- kind/category;
- requested acceptance;
- board dimensions/layers;
- power inputs/rails;
- interfaces;
- functions;
- manufacturing/fabrication intent;
- constraints that materially affect the design.

The report must preserve both:

- what the user requested or drafted;
- what the planner normalized.

## 8. Decisions

A `Decision` explains a selected design action:

```go
type Decision struct {
    ID             string   `json:"id"`
    Type           string   `json:"type"`
    Path           string   `json:"path,omitempty"`
    Selected       string   `json:"selected"`
    Rationale      string   `json:"rationale"`
    RequirementIDs []string `json:"requirement_ids,omitempty"`
    EvidenceIDs    []string `json:"evidence_ids,omitempty"`
    Confidence     string   `json:"confidence,omitempty"`
    Status         string   `json:"status,omitempty"`
}
```

Initial decision types:

- `block_selection`
- `component_selection`
- `connection`
- `validation_policy`
- `routing_policy`
- `fabrication_policy`
- `clarification`
- `known_gap`

Decisions should be generated from:

- `intentplanner.SelectedBlockRecord`
- `intentplanner.ConnectionRecord`
- component-selection workflow summaries
- validation/routing/fabrication workflow stage summaries
- draft extraction fields when present

## 9. Evidence Records

`EvidenceRecord` links decisions to proof or source data:

```go
type EvidenceRecord struct {
    ID          string   `json:"id"`
    Kind        string   `json:"kind"`
    Path        string   `json:"path,omitempty"`
    Summary     string   `json:"summary"`
    SourceText  string   `json:"source_text,omitempty"`
    Confidence  float64  `json:"confidence,omitempty"`
    ArtifactRef string   `json:"artifact_ref,omitempty"`
    Notes       []string `json:"notes,omitempty"`
}
```

Evidence kinds:

- `source_text`
- `draft_field`
- `planner_requirement`
- `block_verification`
- `component_evidence`
- `workflow_stage`
- `validation_issue`
- `artifact`

## 10. Known Limits

Known limits normalize what the system cannot yet prove:

```go
type KnownLimit struct {
    ID         string   `json:"id"`
    Category   string   `json:"category"`
    Severity   string   `json:"severity"`
    Path       string   `json:"path,omitempty"`
    Message    string   `json:"message"`
    Suggestion string   `json:"suggestion,omitempty"`
    EvidenceIDs []string `json:"evidence_ids,omitempty"`
}
```

Categories:

- `unsupported_intent`
- `missing_component_evidence`
- `missing_block_evidence`
- `placement_blocked`
- `routing_blocked`
- `validation_blocked`
- `fabrication_not_proven`
- `external_tool_missing`
- `preservation_unsupported`

Known limits should be generated from:

- draft clarifications;
- planner known gaps;
- workflow stage warnings/errors/skips;
- validation issues;
- fabrication readiness issues;
- missing artifact/source evidence.

## 11. Validation Summary

The report should summarize proof state:

```go
type ValidationSummary struct {
    RequestedAcceptance string `json:"requested_acceptance,omitempty"`
    AchievedAcceptance  string `json:"achieved_acceptance,omitempty"`
    BlockingCount       int    `json:"blocking_count"`
    WarningCount        int    `json:"warning_count"`
    StageCount          int    `json:"stage_count,omitempty"`
    CompletedStages     int    `json:"completed_stages,omitempty"`
    SkippedStages       int    `json:"skipped_stages,omitempty"`
}
```

It should not duplicate every workflow issue; those belong in `KnownLimits` and
evidence records.

## 12. Next Actions

`NextAction` should tell an AI what to do next:

```go
type NextAction struct {
    ID         string `json:"id"`
    Priority   int    `json:"priority"`
    Action     string `json:"action"`
    Reason     string `json:"reason"`
    Command    string `json:"command,omitempty"`
    TargetPath string `json:"target_path,omitempty"`
}
```

Examples:

- Ask user to choose battery voltage.
- Add missing component evidence.
- Increase board size or relax fixed placement constraints.
- Run `kicadai --target <project> repair export-bundle`.
- Run KiCad ERC/DRC with configured `--kicad-cli`.

## 13. Target-Mode Artifact Loading

For `--target`, the builder should load known artifacts from `.kicadai/`:

- `intent-draft.json`
- `intent-extraction.json`
- `intent-clarifications.json`
- `intent-plan.json`
- `generated-request.json`
- design workflow summaries where persisted
- validation or repair bundle artifacts where present

Missing optional artifacts should become known limits, not hard failures.

Missing all rationale sources should block.

## 14. Integration Points

### 14.1 `intent plan`

When `--output` is provided to `intent plan`, optionally write
`design-rationale.json` in the output directory once the report builder exists.

### 14.2 `intent explain`

Include a compact rationale summary in the explain JSON, or add a stable
artifact reference when `--output` is provided.

### 14.3 `intent create`

Persist `.kicadai/design-rationale.json` after planning and workflow execution.
If workflow execution is blocked, the rationale report should still be written
when the output directory exists and contains enough managed metadata.

## 15. Testing

Required tests:

- unit tests for report construction from draft + plan;
- unit tests for report construction from blocked clarification;
- unit tests for known-limit category mapping;
- unit tests for validation summary counts;
- CLI tests for:
  - `intent rationale --request`;
  - `--text ... intent rationale`;
  - `--target ... intent rationale`;
  - source-mode conflict handling;
  - output artifact writing;
- regression tests proving existing `intent plan/explain/create` output remains
  compatible.

## 16. Acceptance Criteria

- `kicadai --json --text "..." intent rationale` returns a deterministic
  rationale report with source, intent, decisions, evidence, limits, validation
  summary, and next actions.
- Blocking clarifications produce `needs_clarification`, not a guessed design.
- Generated project targets can produce a rationale report from `.kicadai/`
  artifacts.
- `intent create` persists `.kicadai/design-rationale.json` where enough
  metadata exists.
- Full `go test ./...` passes without KiCad or network access.

