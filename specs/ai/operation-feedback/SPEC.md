# Operation Feedback Specification

## Purpose

Make KiCadAI feedback directly actionable for AI agents and humans by linking
validation, planning, apply, ERC, DRC, placement, routing, and writer findings
back to stable transaction operation identifiers.

The current transaction planner can produce stable `plan.operations[].id`
values, and planner issues can include `operation_id` when their paths reference
`operations[n]`. This specification extends that foundation into a consistent
feedback contract across CLI commands.

The intended loop is:

```text
transaction or generated intent
  -> validate / plan / apply / check
  -> issues with operation_id, refs, nets, and paths
  -> AI edits the specific operation or adds corrective operations
  -> repeat until checks pass
```

## Current Context

The project already has:

- transaction parsing, validation, planning, and apply flows;
- stable content-derived transaction plan operation IDs;
- optional `reports.Issue.operation_id`;
- planner-side annotation of issues whose path begins with `operations[n]`;
- structured issue codes, severities, paths, refs, nets, suggestions, and
  artifacts;
- placement, routing, ERC/DRC, round-trip, library resolver, block generation,
  and writer validation packages;
- CLI JSON report envelopes for agent consumption.

The missing layer is consistency. Some commands expose issue paths and operation
indexes, some expose refs and nets, and some operate after transaction
application where the original operation context can be lost. AI agents need a
stable way to answer:

- which operation caused this issue?
- what should be changed?
- what generated object did the operation produce?
- which later check failed because of that operation?

## Goals

1. Add operation IDs to transaction validation output, not only planner output.
2. Preserve operation IDs through plan, apply, and generated artifact checks
   wherever the source operation is known.
3. Provide a compact operation feedback summary suitable for AI repair loops.
4. Keep existing report JSON backward-compatible by only adding optional fields.
5. Avoid embedding hidden reasoning or natural-language-only state in reports.
6. Keep operation correlation deterministic for the same transaction content.
7. Provide tests that prove issues can be traced from CLI output back to
   transaction operations.

## Non-Goals

- No MCP server.
- No natural-language command parser.
- No automatic AI repair engine in this phase.
- No database or persistent telemetry store.
- No attempt to perfectly attribute KiCad CLI ERC/DRC findings when there is no
  modeled source object or operation context.
- No breaking changes to existing transaction JSON.
- No mandatory operation IDs in reports that are not transaction-derived.

## Terminology

### Operation ID

An operation ID is a stable, content-derived identifier emitted in
`plan.operations[].id`. It is intended for correlation, not for authorization or
global object identity.

Examples:

```text
op-add-symbol-ref-r1-8f1a2b3c4d
op-route-net-gnd-a92b1018cc
op-write-project-0ad401bc92
```

### Operation Index

An operation index is the transaction slice position, used in issue paths such
as `operations[3].ref`. Indexes are useful for local diagnostics but change
when operations are inserted, removed, or reordered.

### Operation Feedback

Operation feedback is a grouped summary of issues, affected refs/nets, severity,
suggestions, and artifacts for a specific operation ID.

### Source Operation

A source operation is the transaction operation that most directly produced the
file object or validation issue being reported.

## Report Schema Additions

### Issue

`reports.Issue` already includes:

```go
OperationID string `json:"operation_id,omitempty"`
```

This field should be populated when:

- the issue path references a transaction operation;
- the caller has an operation context while validating/applying an operation;
- a generated object can be traced back to a transaction operation;
- a higher-level command is summarizing transaction-derived issues.

This field must stay optional. Non-transaction commands should omit it unless
there is a real operation source.

### Operation Feedback Summary

Add a reusable model, likely in `internal/transactions` or `internal/reports`:

```go
type OperationFeedback struct {
    OperationID string             `json:"operation_id"`
    Index       int                `json:"index"`
    Op          OperationKind      `json:"op"`
    Refs        []string           `json:"refs,omitempty"`
    Nets        []string           `json:"nets,omitempty"`
    Severity    reports.Severity   `json:"severity"`
    Issues      []reports.Issue    `json:"issues"`
    Artifacts   []reports.Artifact `json:"artifacts,omitempty"`
    Suggestions []string           `json:"suggestions,omitempty"`
}
```

The summary should be derived from existing plan and issue data. It should not
replace `issues`; it should provide an additional grouped view.

### Feedback Report

Add a wrapper for transaction-centered commands:

```go
type FeedbackReport struct {
    Target     string              `json:"target,omitempty"`
    Operations []OperationFeedback `json:"operations"`
    Issues     []reports.Issue     `json:"issues"`
    Artifacts  []reports.Artifact  `json:"artifacts,omitempty"`
    Summary    FeedbackSummary     `json:"summary"`
}
```

```go
type FeedbackSummary struct {
    OperationCount int `json:"operation_count"`
    IssueCount     int `json:"issue_count"`
    BlockingCount  int `json:"blocking_count"`
    WarningCount   int `json:"warning_count"`
    UnlinkedCount  int `json:"unlinked_count"`
}
```

`UnlinkedCount` counts issues with no `operation_id`.

## CLI Behavior

### Transaction Validate

Current validation reports should keep their existing fields and add operation
IDs where possible.

Target behavior:

```text
kicadai transaction validate tx.json --json
```

Returns:

- operation count;
- issues;
- each operation-scoped issue includes `operation_id`;
- no full plan is required from the caller.

The implementation may internally build operation IDs using the same planner ID
helper or a shared operation identity helper.

### Transaction Plan

Current planning behavior should remain:

```text
kicadai transaction plan <target> tx.json --json
```

Add or preserve:

- `operations[].id`;
- `issues[].operation_id`;
- grouped feedback if requested by a flag.

Suggested flag:

```text
--feedback
```

When enabled, include an additional `feedback` object in the command payload or
return a feedback-focused report if that matches existing CLI conventions.

### Transaction Apply

Apply should preserve operation IDs across:

- validation failures before writing;
- plan issues;
- operation execution failures;
- generated artifact validation failures when the source operation is known.

If apply stops at operation `n`, the issue should include the operation ID for
operation `n`.

### Check Commands

Check commands that inspect written files should include `operation_id` only
when the file object can be traced to a generated operation. This requires
source metadata or a trace map.

Initial scope:

- do not force operation IDs onto generic KiCad CLI findings;
- include refs/nets/paths as today;
- add operation IDs only for transaction-derived checks where a trace map is
  available.

## Operation Trace Map

Some feedback happens after file generation, when the original transaction
operation is no longer directly in scope. Add an in-memory trace map for apply
and validation flows.

```go
type OperationTrace struct {
    OperationID string
    Index       int
    Op          OperationKind
    Refs        []string
    Nets        []string
    Paths       []string
    Artifacts   []reports.Artifact
}
```

The trace map should support lookup by:

- operation index;
- operation ID;
- reference designator;
- net name;
- generated artifact path;
- optionally generated object UUID when available.

Initial implementation can support index/ref/net/artifact lookup. UUID lookup
can wait until writers consistently expose source metadata.

## Correlation Rules

### Direct Operation Issues

If an issue path starts with `operations[n]`, attach the ID for operation `n`.

### Ref-Based Issues

If an issue has no operation ID but has exactly one ref, and the trace map has
exactly one operation that owns that ref, attach that operation ID.

If multiple operations touch the same ref, do not guess unless the issue path or
code narrows the source operation.

### Net-Based Issues

If an issue has no operation ID but has exactly one net, and exactly one
operation owns that net, attach that operation ID.

Route/connect/zone operations may share a net. Ambiguous cases should remain
unlinked rather than incorrectly attributed.

### Artifact-Based Issues

If an issue path references an artifact generated by a single operation, attach
that operation ID.

### Ambiguous Issues

When the source cannot be determined safely:

- leave `operation_id` empty;
- preserve refs, nets, paths, and suggestions;
- include the issue in `unlinked_count`.

## AI Repair Loop Requirements

A downstream AI should be able to:

1. call validate or plan;
2. inspect `issues[].operation_id`;
3. find the matching operation in `operations[]`;
4. modify or replace that operation;
5. rerun validation;
6. see whether the same operation ID still has issues.

For inserted/reordered operations, content-derived IDs should remain stable for
unchanged operations. If the operation content changes, a new ID is acceptable
and expected.

## Compatibility

- Existing JSON consumers must continue to work.
- `operation_id` is optional and appended to `Issue` to minimize field-order
  churn for encoders that preserve struct order.
- Existing issue `path` values must remain unchanged.
- Existing issue `refs` and `nets` values must remain unchanged.
- Existing CLI commands should not require new flags for current behavior.

## Testing Requirements

Add tests for:

- validate output annotates operation-scoped issues;
- plan output still annotates operation-scoped issues;
- feedback grouping includes operation IDs, refs, nets, severity, and issue
  counts;
- ambiguous ref/net issues remain unlinked;
- apply failures include operation IDs where the failing operation is known;
- JSON report shape omits `operation_id` when empty and includes it when set;
- operation IDs remain stable when unchanged operations are reordered;
- duplicate operation IDs are disambiguated without collision.

## Acceptance Criteria

- A transaction with invalid operations can be validated and each operation
  issue includes `operation_id`.
- A transaction plan includes both `operations[].id` and
  `issues[].operation_id` for operation-scoped issues.
- A feedback summary groups issues by operation ID without removing the raw
  issue list.
- Ambiguous post-generation findings are not incorrectly attributed.
- `go test ./...` passes.
- `prism review staged` has no unresolved high or actionable medium findings.

## Future Work

- Persist operation trace metadata into generated project sidecar files.
- Map generated KiCad object UUIDs back to source operations.
- Use operation feedback to drive automatic transaction patch suggestions.
- Add a CLI command that emits a minimal repair prompt for AI agents.
- Add MCP support after the CLI feedback loop is stable.
