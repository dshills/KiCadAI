# Operation Feedback Implementation Plan

## Objective

Make transaction and validation feedback traceable to stable operation IDs so AI
agents can repair specific operations instead of guessing from paths or freeform
messages.

This plan builds on the existing planner operation IDs and
`reports.Issue.operation_id` field. It keeps the current CLI behavior intact
while adding richer optional feedback views.

## Implementation Rules

- Keep report schema changes backward-compatible.
- Do not change existing transaction JSON.
- Preserve existing issue paths, codes, refs, nets, and suggestions.
- Prefer shared helpers over duplicating operation ID logic in CLI packages.
- Do not guess operation attribution in ambiguous cases.
- Run `gofmt` and `go test ./...` before every phase commit.
- Run `prism review staged` before every phase commit and address actionable
  findings.

## Phase 1: Shared Operation Identity Helpers

### Goals

Move operation ID generation and issue annotation into reusable helpers so
validation, planning, and future apply flows use the same rules.

### Tasks

1. Extract operation ID helpers from `internal/transactions/plan.go` into a
   focused file such as:

   ```text
   internal/transactions/operation_identity.go
   ```

2. Provide package-private or exported helpers as appropriate:

   ```go
   func OperationIDForPlan(planned PlannedOperation, op Operation) string
   func UniqueOperationID(base string, seen map[string]struct{}, counts map[string]int) string
   func AnnotateIssueOperationIDs(issues []reports.Issue, operations []PlannedOperation)
   ```

   Exact names may vary, but responsibilities should be explicit.

3. Preserve canonical JSON hashing with `json.Decoder.UseNumber`.
4. Preserve duplicate-safe suffix handling.
5. Keep `PlanTransactionWithOptions` behavior unchanged after the extraction.
6. Add or move tests for:
   - content-stable IDs;
   - duplicate disambiguation;
   - canonical JSON formatting;
   - operation path annotation;
   - slice-index correlation.

### Acceptance Criteria

- Planning output is byte-compatible except for harmless JSON object ordering.
- Existing operation ID tests pass.
- No operation ID generation logic remains duplicated in CLI code.

### Suggested Commit

`Extract transaction operation identity helpers`

## Phase 2: Transaction Validate Operation IDs

### Goals

Add operation IDs to transaction validation output without requiring callers to
run a full plan.

### Tasks

1. Extend `ValidationResult` to include an optional operation summary:

   ```go
   Operations []ValidatedOperation `json:"operations,omitempty"`
   ```

2. Define:

   ```go
   type ValidatedOperation struct {
       ID    string        `json:"id"`
       Index int           `json:"index"`
       Op    OperationKind `json:"op"`
       Ref   string        `json:"ref,omitempty"`
   }
   ```

   If a richer reusable model already exists after Phase 1, reuse it.

3. Generate IDs for all operations in `Validate`.
4. Annotate all validation issues whose path starts with `operations[n]`.
5. Keep `OperationCount` and `Issues` unchanged for existing consumers.
6. Add tests for:
   - invalid operation includes `operation_id`;
   - unsupported operation includes `operation_id`;
   - empty transaction issue remains unlinked;
   - operation summaries include IDs and indexes;
   - reordered unchanged operations keep the same IDs.

### Acceptance Criteria

- `transaction validate` JSON includes operation IDs for operation-scoped
  issues.
- Existing validation tests remain meaningful and pass.
- Empty transactions still report a transaction-level issue without
  `operation_id`.

### Suggested Commit

`Add operation IDs to transaction validation`

## Phase 3: Feedback Summary Model

### Goals

Add a grouped feedback model that lets agents inspect issues by operation ID
without losing the raw issue list.

### Tasks

1. Add a model in `internal/transactions` or `internal/reports`:

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

2. Add:

   ```go
   type FeedbackSummary struct {
       OperationCount int `json:"operation_count"`
       IssueCount     int `json:"issue_count"`
       BlockingCount  int `json:"blocking_count"`
       WarningCount   int `json:"warning_count"`
       UnlinkedCount  int `json:"unlinked_count"`
   }
   ```

3. Implement a builder that accepts:
   - planned or validated operations;
   - issues;
   - optional artifacts.

4. Severity for a group should be the highest severity among its issues.
5. Suggestions should be deduplicated and preserve first-seen order.
6. Unlinked issues should be counted but not forced into fake operation groups.
7. Add tests for:
   - grouping by operation ID;
   - severity escalation;
   - suggestion deduplication;
   - unlinked count;
   - empty issue sets;
   - refs/nets propagation.

### Acceptance Criteria

- Feedback grouping is deterministic.
- Raw issues remain available.
- Ambiguous issues stay unlinked.

### Suggested Commit

`Add operation feedback summaries`

## Phase 4: CLI Validate And Plan Feedback Output

### Goals

Expose operation feedback summaries through CLI JSON output for transaction
validation and planning.

### Tasks

1. Inspect current transaction CLI command routing.
2. Add a `--feedback` flag to:

   ```text
   kicadai transaction validate
   kicadai transaction plan
   ```

3. When `--feedback` is set, include a feedback summary in the command payload.
4. Keep default output compatible when `--feedback` is not set.
5. Ensure JSON report envelope remains stable.
6. Add CLI tests for:
   - validate with feedback;
   - validate without feedback remains compatible;
   - plan with feedback;
   - invalid transaction returns operation-scoped feedback;
   - unlinked issue counts appear in summary.

### Acceptance Criteria

- Agents can request compact grouped feedback from validation or planning.
- Existing CLI tests pass without updating callers that do not use
  `--feedback`.

### Suggested Commit

`Expose transaction operation feedback in CLI`

## Phase 5: Apply Failure Correlation

### Goals

Preserve operation IDs through transaction apply failures and pre-write checks.

### Tasks

1. Build or reuse an operation trace list at the start of `transactions.Apply`.
2. Annotate plan issues already returned by apply.
3. Annotate operation execution failures with the failing operation ID.
4. Annotate write/pre-write validation issues when they map to an operation
   index or unique ref/net source.
5. Avoid attribution when multiple operations touch the same ref/net.
6. Add tests for:
   - apply stops on a failing operation and includes `operation_id`;
   - plan issues returned from apply include `operation_id`;
   - ambiguous ref issues remain unlinked;
   - write-project missing/invalid cases preserve useful operation context.

### Acceptance Criteria

- Apply failures can be traced back to a source operation when known.
- No broad guessing is introduced for ambiguous generated-file issues.

### Suggested Commit

`Link apply issues to source operations`

## Phase 6: Trace Map For Generated Object Checks

### Goals

Add an in-memory trace map that can link post-generation findings to source
operations when the relationship is unambiguous.

### Tasks

1. Define:

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

2. Add lookup helpers by:
   - operation index;
   - operation ID;
   - single ref;
   - single net;
   - artifact path.

3. Integrate trace map into apply and check flows where transaction context is
   already available.
4. Leave KiCad CLI findings unlinked unless a unique trace match exists.
5. Add tests for:
   - unique ref attribution;
   - ambiguous ref remains unlinked;
   - unique net attribution;
   - ambiguous net remains unlinked;
   - artifact path attribution.

### Acceptance Criteria

- Post-generation issues can be attributed when there is a unique trace source.
- Ambiguous findings remain unlinked and counted.

### Suggested Commit

`Add operation trace map`

## Phase 7: Documentation And Examples

### Goals

Document the feedback loop and provide examples that an AI agent can follow.

### Tasks

1. Update `README.md` transaction sections with:
   - operation IDs;
   - `operation_id` on issues;
   - `--feedback` examples;
   - repair-loop guidance.

2. Add or update an example transaction that intentionally fails validation.
3. Add expected command snippets showing:

   ```text
   kicadai transaction validate bad.json --json --feedback
   kicadai transaction plan out bad.json --json --feedback
   ```

4. Document limitations:
   - unlinked issues;
   - ambiguous ref/net attribution;
   - KiCad CLI findings without trace data.

### Acceptance Criteria

- README explains how an AI should use operation feedback.
- Examples are runnable.
- Documentation does not promise automatic repair.

### Suggested Commit

`Document operation feedback workflow`

## Final Verification

After all phases:

1. Run:

   ```text
   go test ./...
   ```

2. Run representative CLI commands for:
   - transaction validate;
   - transaction validate with feedback;
   - transaction plan;
   - transaction plan with feedback;
   - apply failure case if fixture exists.

3. Run `prism review staged` for each commit.
4. Ensure `git status --short` is clean after final commit.

## Risks And Mitigations

### Risk: Incorrect Attribution

Incorrect operation IDs are worse than missing operation IDs.

Mitigation:

- only annotate direct operation paths automatically;
- require unique ref/net/artifact matches for trace attribution;
- leave ambiguous issues unlinked.

### Risk: Report Schema Churn

Downstream users may parse report JSON by expected field names and order.

Mitigation:

- make all additions optional;
- append fields where possible;
- preserve existing issue paths and codes.

### Risk: Duplicate Operation Content

Identical operations may produce the same base ID.

Mitigation:

- keep duplicate-safe suffix handling;
- test natural suffix collisions.

### Risk: Hash Cost

Canonical operation hashing costs CPU for very large transactions.

Mitigation:

- keep implementation simple first;
- optimize later only if profiling shows a real bottleneck;
- avoid repeated hashing when a trace list can be reused.
