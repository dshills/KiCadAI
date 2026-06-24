# Post-Repair Validation Convergence Specification

Date: 2026-06-24

## 1. Purpose

Close the remaining Priority 3 gaps in `specs/ROADMAP.md` by making
post-repair validation evidence more specific, more loop-aware, and safer when
external KiCad behavior is requested.

The existing post-repair validation adapter foundation can replay generated
projects, run writer correctness, run board validation, optionally run KiCad
ERC/DRC and round-trip checks, summarize post-apply evidence, compute issue
deltas, and block or downgrade repair status when validation remains dirty.

The next step is convergence quality:

```text
validate -> classify parser/tool findings -> plan repair -> apply -> revalidate
  -> compare category-specific evidence -> retry only while useful and bounded
  -> optionally invoke KiCad-only actions under explicit policy
```

The goal is not to make every repair possible. The goal is to ensure every
post-repair result explains exactly what validation evidence changed, what can
be repaired next, what is blocked by missing KiCad/tool evidence, and why the
loop stopped.

## 2. Current Baseline

Implemented foundations include:

- `internal/repair` persisted apply, generated-target ownership checks,
  provenance checks, repair bundles, post-validation summaries, issue deltas,
  retry budget evidence, and generated-project apply.
- `internal/writercorrectness` checks for project structure, schematic parse,
  PCB parse, connectivity, transfer, pad-net, copper-net, zone-net, and
  optional round-trip evidence.
- `internal/boardvalidation` checks for PCB structure, outline, pad nets,
  route completion, zones, and optional KiCad DRC.
- `internal/kicadfiles/checks` KiCad ERC/DRC runners and JSON parsers with
  parser issue reporting.
- `internal/routing` mapping from KiCad DRC findings into repairable routing
  diagnostics.
- `internal/designworkflow` generation, validation, optional bounded
  placement-routing retry, optional validation repair, and artifact summaries.
- CLI support for `design create`, `repair apply`, `repair export-bundle`,
  `validate-board`, and `check`.

## 3. Problem Statement

Post-repair validation is structurally present, but the remaining gaps limit
how confidently the system can drive automatic repair loops.

### 3.1 Parser-Specific Evidence Is Too Coarse

ERC/DRC parser findings, parser failures, missing report files, tool errors,
and board/writer structural findings are not normalized into one stable repair
taxonomy across all entry points. Similar conditions can appear with different
codes, paths, or messages depending on whether they came from workflow
validation, repair apply, board validation, block verification, or direct CLI
checks.

This makes it difficult to:

- compare before/after evidence accurately;
- decide whether a repair category repeated;
- map validation findings to the next repair planner input;
- explain why a finding is repairable, external-tool-blocked, or unsupported.

### 3.2 Retry Budgets Do Not Cover The Whole Loop

Persisted repair apply has budget evidence, and placement-routing retry has its
own bounded policy. The broader generate/validate/repair/revalidate loop still
needs one shared budget ledger so the workflow can answer:

- how many validation cycles ran;
- which issue categories consumed attempts;
- whether a new repair was skipped because category or total budgets were
  exhausted;
- whether loop termination was caused by success, non-improvement, repeated
  state, or unsupported findings.

### 3.3 KiCad Zone Refill Needs Explicit Policy

Zone fill is KiCad-owned derived geometry. KiCadAI should not silently mutate
zones as part of validation or repair. Zone refill may be useful before DRC or
fabrication checks, but it must only run when explicitly requested and only on
generated projects that pass ownership/provenance checks.

### 3.4 KiCad Artifact Fixtures Need Opt-In Coverage

Default tests should not require local KiCad. However, the project needs stable
fixture paths for real KiCad ERC/DRC reports when available. These fixtures
should cover parser behavior, category mapping, optional/required policy, and
repair-loop evidence without making normal unit tests environment-dependent.

## 4. Goals

This project must:

- define a stable validation finding normalization model shared by repair,
  workflow, board validation, writer correctness, and KiCad check consumers;
- map parser/tool/validation findings into repair categories consistently;
- classify findings as repairable, unsupported, external-tool-blocked,
  preservation-blocked, or informational;
- extend issue deltas with normalized category, source, location, and evidence
  identity fields;
- add a loop budget ledger that spans generate, validate, repair, revalidate,
  and optional placement-routing retry stages;
- ensure repair loops stop deterministically on clean validation, repeated
  evidence, no improvement, exhausted budgets, stale generated state, or
  unsupported categories;
- add explicit KiCad zone-refill policy and evidence without making refill
  implicit;
- add opt-in real KiCad ERC/DRC artifact fixtures and deterministic fake-runner
  fixtures for default tests;
- expose convergence evidence in CLI JSON, workflow artifacts, and repair
  bundle outputs.

## 5. Non-Goals

This project does not:

- make imported-project mutation safe;
- add arbitrary LLM repair generation;
- guarantee KiCad DRC-clean output for every design;
- implement new placement or routing algorithms;
- make KiCad CLI mandatory for default tests;
- treat missing optional KiCad evidence as success;
- silently run zone refill;
- change KiCad files outside generated-project ownership policy.

## 6. Terminology

### Validation Finding

Any issue emitted by writer correctness, board validation, KiCad ERC/DRC,
round-trip checks, transaction validation, repair apply, or workflow stages.

### Normalized Finding

A validation finding enriched with stable source, category, location, subject,
repairability, and evidence identity fields.

### Evidence Identity

A deterministic key used to compare findings across validation cycles. It
should be stable across message wording changes when possible and should prefer
structured fields over free text.

### Loop Cycle

One complete validation attempt plus optional repair apply and revalidation.

### Zone Refill

An explicit KiCad CLI action that updates zone fill geometry before validation.
It is a write action, not a read-only validation adapter.

## 7. Normalized Finding Model

Add a model that can live in `internal/repair` or a small shared package such
as `internal/validationevidence` if reuse pressure justifies it.

Required fields:

```go
type FindingSource string

const (
    FindingSourceTransaction FindingSource = "transaction"
    FindingSourceWriter      FindingSource = "writer_correctness"
    FindingSourceBoard       FindingSource = "board_validation"
    FindingSourceKiCadERC    FindingSource = "kicad_erc"
    FindingSourceKiCadDRC    FindingSource = "kicad_drc"
    FindingSourceRoundTrip   FindingSource = "round_trip"
    FindingSourceRepair      FindingSource = "repair"
    FindingSourceWorkflow    FindingSource = "workflow"
)

type FindingCategory string

const (
    CategoryParse             FindingCategory = "parse"
    CategoryProjectStructure  FindingCategory = "project_structure"
    CategorySchematicERC      FindingCategory = "schematic_erc"
    CategoryBoardDRC          FindingCategory = "board_drc"
    CategoryConnectivity      FindingCategory = "connectivity"
    CategoryPadNet            FindingCategory = "pad_net"
    CategoryRoute             FindingCategory = "route"
    CategoryZone              FindingCategory = "zone"
    CategoryOutline           FindingCategory = "outline"
    CategoryRoundTrip         FindingCategory = "round_trip"
    CategoryExternalTool      FindingCategory = "external_tool"
    CategoryPreservation      FindingCategory = "preservation"
    CategoryUnsupported       FindingCategory = "unsupported"
)

type Repairability string

const (
    RepairabilityRepairable          Repairability = "repairable"
    RepairabilityUnsupported         Repairability = "unsupported"
    RepairabilityExternalToolBlocked Repairability = "external_tool_blocked"
    RepairabilityPreservationBlocked Repairability = "preservation_blocked"
    RepairabilityInformational       Repairability = "informational"
)

type NormalizedFinding struct {
    Key           string        `json:"key"`
    Source        FindingSource `json:"source"`
    Adapter       string        `json:"adapter,omitempty"`
    Category      FindingCategory `json:"category"`
    Repairability Repairability `json:"repairability"`
    Code          reports.Code  `json:"code"`
    Severity      reports.Severity `json:"severity"`
    Path          string        `json:"path,omitempty"`
    Message       string        `json:"message"`
    Subject       FindingSubject `json:"subject,omitempty"`
    OperationID   string        `json:"operation_id,omitempty"`
    EvidencePath  string        `json:"evidence_path,omitempty"`
    RawCode       string        `json:"raw_code,omitempty"`
}

type FindingSubject struct {
    Ref      string `json:"ref,omitempty"`
    Net      string `json:"net,omitempty"`
    Layer    string `json:"layer,omitempty"`
    Pad      string `json:"pad,omitempty"`
    File     string `json:"file,omitempty"`
    Rule     string `json:"rule,omitempty"`
    Location string `json:"location,omitempty"`
}
```

The key must be derived deterministically from source, category, code, subject,
path, operation ID, and raw KiCad code where available. Message text may be a
fallback but should not be the first choice.

## 8. Category Mapping

The mapper must normalize at least:

- KiCad ERC findings from `internal/kicadfiles/checks`;
- KiCad DRC findings from `internal/kicadfiles/checks`;
- parser issues from KiCad check reports;
- missing KiCad CLI, missing report, and tool execution failures;
- writer correctness parse and round-trip issues;
- board validation outline, pad-net, route-completion, zone, and DRC issues;
- routing DRC diagnostics already produced by `internal/routing`;
- repair ownership/provenance/preservation blockers.

The first implementation may use deterministic rule tables based on:

- existing `reports.Code`;
- adapter/check name;
- KiCad raw code;
- lowercased message fragments only where no structured code exists.

Message-fragment matching must be isolated in mapping helpers and covered by
tests so it can be replaced by richer parser fields later.

## 9. Delta And Convergence Model

Extend current validation summaries with normalized deltas:

```go
type ConvergenceSummary struct {
    Status        string `json:"status"`
    StopReason    string `json:"stop_reason"`
    Cycles        []ValidationCycleSummary `json:"cycles"`
    Final         ValidationEvidenceSummary `json:"final"`
    Delta         NormalizedDeltaSummary `json:"delta"`
    Budget        LoopBudgetSummary `json:"budget"`
}

type ValidationCycleSummary struct {
    Index             int `json:"index"`
    Stage             string `json:"stage"`
    ValidationCount   int `json:"validation_count"`
    BlockingCount     int `json:"blocking_count"`
    RepairableCount   int `json:"repairable_count"`
    UnsupportedCount  int `json:"unsupported_count"`
    NewBlockingCount  int `json:"new_blocking_count"`
    RepeatedCount     int `json:"repeated_count"`
    ClearedCount      int `json:"cleared_count"`
    AppliedRepair     bool `json:"applied_repair"`
    StopReason        string `json:"stop_reason,omitempty"`
}
```

Required stop reasons:

- `clean`;
- `partial_non_blocking_remaining`;
- `no_repairable_findings`;
- `unsupported_findings`;
- `preservation_blocked`;
- `external_tool_blocked`;
- `total_budget_exhausted`;
- `category_budget_exhausted`;
- `no_improvement`;
- `repeated_evidence`;
- `stale_generated_target`;
- `validation_error`.

## 10. Loop Budget Ledger

Add one loop budget model shared by CLI and design workflow:

```go
type LoopBudgetOptions struct {
    MaxCycles          int            `json:"max_cycles"`
    MaxRepairs         int            `json:"max_repairs"`
    MaxPerCategory     map[string]int `json:"max_per_category,omitempty"`
    StopOnNoImprovement bool          `json:"stop_on_no_improvement"`
    StopOnRepeatedEvidence bool       `json:"stop_on_repeated_evidence"`
}
```

Default policy:

- max cycles: `2` for normal CLI repair apply;
- max cycles: `3` for explicit design workflow repair loops;
- max repairs: equal to max cycles unless overridden;
- per-category default: `1` for high-risk categories such as preservation,
  external tool, parse, and round-trip; `2` for route/connectivity categories;
- stop on no improvement: enabled;
- stop on repeated evidence: enabled.

The ledger must record:

- consumed total cycles;
- consumed repairs;
- consumed category attempts;
- remaining budget;
- the category that caused exhaustion;
- the evidence key that repeated if repeated-evidence stopping triggers.

## 11. KiCad Zone Refill Policy

Zone refill is a write action and must be gated separately from validation.

Add policy:

```go
type ZoneRefillPolicy string

const (
    ZoneRefillNever    ZoneRefillPolicy = "never"
    ZoneRefillValidate ZoneRefillPolicy = "before_validation"
    ZoneRefillRepair   ZoneRefillPolicy = "after_repair_before_validation"
)
```

Required behavior:

- default is `never`;
- any non-default policy requires a configured KiCad CLI;
- any non-default policy requires generated-project ownership/provenance;
- refill must write only inside the generated target;
- refill must produce an artifact/evidence entry;
- refill failure is blocking when policy requested it;
- refill must run before KiCad DRC only when explicitly configured;
- refill must not run for imported projects.

The first implementation may use a fake runner in unit tests and can defer
real CLI command wiring if the exact KiCad command is unstable. The API and CLI
policy must still be explicit.

## 12. Opt-In KiCad Artifact Fixtures

Add deterministic fixture support for real KiCad reports without requiring
local KiCad in default tests.

Required fixture modes:

- fake-runner JSON fixtures committed to the repo and used by normal tests;
- optional integration fixtures generated from local KiCad CLI under an
  opt-in flag or environment variable;
- golden summaries that redact machine-specific paths and timestamps;
- parser fixtures for report files with violations, warnings, parser errors,
  missing reports, and clean results.

Optional integration tests must skip cleanly when KiCad CLI is unavailable.

## 13. CLI And Artifact Surface

Extend existing JSON output where applicable:

- `repair apply --target ...`;
- `repair export-bundle`;
- `design create --repair` or equivalent workflow repair options;
- `validate-board` if normalized DRC evidence is emitted there;
- future report commands that consume repair bundles.

New output must include:

- normalized findings;
- normalized delta summary;
- convergence summary;
- budget ledger summary;
- zone refill status when configured;
- opt-in artifact paths.

Existing fields must remain backward-compatible. New fields should be additive.

## 14. Safety Requirements

- Never mutate imported projects.
- Never run zone refill unless explicitly requested.
- Never treat missing required KiCad evidence as repaired.
- Never hide parser errors behind clean DRC/ERC status.
- Never loop indefinitely.
- Never rely on free-text messages alone when structured evidence exists.
- Always preserve raw issue data in artifacts or nested fields when possible.

## 15. Acceptance Criteria

This project is complete when:

- ERC/DRC/parser/tool findings normalize into stable categories across repair
  apply, design workflow, board validation, and direct check consumption.
- Validation deltas report category-level cleared, repeated, new, worsened,
  unsupported, and external-tool-blocked findings.
- Generate/validate/repair/revalidate loops share a deterministic budget ledger.
- Repeated evidence and no-improvement loops stop with explicit reasons.
- Zone refill is represented as an explicit policy and never runs by default.
- Optional KiCad report fixtures can be used in integration mode while default
  tests remain fake-runner/deterministic.
- CLI JSON and workflow artifacts expose enough convergence evidence for an AI
  agent to decide whether to retry, ask for help, or stop.

## 16. Open Questions

- Should normalized validation evidence live in `internal/repair` first or a
  dedicated shared package? Start in `internal/repair` unless imports become
  awkward.
- Which exact KiCad CLI zone-refill command should be used for the local KiCad
  version? Keep this behind a runner interface until confirmed.
- Should category budgets be user-configurable from the first CLI version or
  initially fixed with JSON evidence only? Prefer fixed defaults first, with
  CLI flags added only where workflow behavior needs them.
