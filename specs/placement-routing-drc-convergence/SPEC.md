# Placement Routing DRC Convergence Specification

## Objective

Broaden placement-routing retry from small deterministic fixtures to larger
generated board scenarios with stronger convergence evidence and optional
KiCad DRC-backed proof.

The goal is to prove generated layouts improve in ways that matter:

```text
generated schematic/PCB intent
  -> block/component placement
  -> routing
  -> route quality diagnostics
  -> placement retry
  -> rerouting
  -> board validation
  -> optional KiCad DRC evidence
  -> selected best attempt with explainable convergence evidence
```

This project should make retry evidence useful for AI agents deciding whether
to accept, repair, or regenerate a generated board. It should not claim that
the project is fully fabrication-ready or that the router is production-grade.

## Current Foundation

The codebase already has:

- deterministic placement with block-derived constraints, group intent,
  keepouts, region preferences, proximity rules, routing-readiness reports,
  congestion reports, and fanout reports;
- deterministic routing with route-quality reports, rule-aware routing,
  via/layer policy diagnostics, length policy evidence, zone policy behavior,
  and repairable diagnostics;
- routing diagnostic to placement retry hint mapping;
- opt-in bounded placement-routing retry in `design create`;
- best-attempt selection and repeated-state protection;
- generated placement mobility classes and `local_route_mobility` evidence;
- generated pad hydration, resolver-backed footprint geometry, and circuit
  block PCB realization metadata;
- connectivity-first board validation and optional KiCad DRC hooks;
- fabrication package generation/validation that can run KiCad CLI when
  explicitly configured.

Current retry evidence is strongest for small pad-backed seed boards and
focused generated boundaries. The next gap is larger generated-board
convergence with DRC-backed evidence.

## Scope

### In Scope

- Add larger generated full-board fixtures that combine multiple circuit
  blocks, more nets, and realistic constraints.
- Extend retry attempt ranking with richer convergence metrics:
  - route status;
  - routed/failed/skipped net counts;
  - route quality score;
  - placement quality score;
  - DRC issue counts when available;
  - board-validation blocking issue counts;
  - unrouted required net counts;
  - repeated-state and non-improvement evidence.
- Add a DRC evidence adapter for retry attempts:
  - default tests use fake or fixture-backed DRC evidence;
  - real `kicad-cli` DRC remains optional and explicitly configured;
  - missing optional DRC evidence is visible but not blocking;
  - required DRC evidence blocks acceptance when configured.
- Add generated larger-board retry fixtures that prove:
  - improvement across attempts;
  - safe stop when DRC or board validation gets worse;
  - best-attempt selection prefers electrically better output;
  - retry does not move fixed/unowned content;
  - local-route mobility evidence remains consistent.
- Add stable CLI/workflow evidence for AI callers:
  - per-attempt convergence summary;
  - selected best attempt reason;
  - DRC evidence status and issue deltas;
  - board validation issue deltas;
  - generated refs/groups moved or blocked.
- Keep retry opt-in.
- Keep normal `go test ./...` hermetic and independent of KiCad.

### Out Of Scope

- A new autorouter.
- Production DRC cleanliness for all generated boards.
- Manufacturer-specific DFM checks.
- Natural-language intent planning.
- Enabling placement-routing retry by default.
- Imported-project mutation.
- Full fabrication readiness.
- Differential-pair, impedance, thermal, creepage, and high-current layout
  solving except as explicit future diagnostics.

## User-Facing Behavior

`design create` requests continue to opt into retry through `routing_retry`.
When larger-board convergence evidence is available, routing stage summaries
should expose compact, stable JSON:

```json
{
  "routing_retry": {
    "enabled": true,
    "attempts": 3,
    "applied": 2,
    "stop_reason": "max_attempts",
    "selected_attempt": 2,
    "selected_reason": "best_route_and_validation_score",
    "attempt_history": [
      {
        "attempt": 1,
        "routing_status": "blocked",
        "route_score": 42,
        "routed_nets": 6,
        "failed_nets": 3,
        "board_validation_blocking": 2,
        "drc_status": "skipped"
      }
    ]
  }
}
```

When DRC evidence is configured:

- `drc_status` should be one of `pass`, `fail`, `missing`, `skipped`, or
  `warning`;
- DRC issue counts and deltas should appear in attempt summaries;
- required DRC failures prevent an attempt from being selected as clean;
- optional DRC failures are still visible and can affect ranking, but do not
  crash the workflow.

## Larger-Board Fixture Requirements

Fixtures should be generated or checked in under existing workflow testdata
patterns. They should avoid absolute paths and should not require KiCad CLI in
default tests.

Minimum fixture families:

- `generated_multiblock_converges`: multiple blocks with enough nets that retry
  improves route and validation score.
- `generated_multiblock_drc_regression`: a later attempt improves routed-net
  count but worsens DRC/validation evidence, so best-attempt selection keeps an
  earlier attempt.
- `generated_multiblock_no_convergence`: eligible movement exists, but attempts
  do not improve, producing a stable non-improving stop.
- `generated_fixed_boundary`: fixed edge connectors or mechanical refs remain
  fixed while nearby generated support components may move.
- `generated_local_route_boundary`: local route mobility remains transformable,
  rebuildable, preserved, or blocked consistently after retry.

Fixture metadata should declare expected:

- retry enabled/disabled state;
- maximum attempts;
- stop reason;
- selected attempt;
- routed/failed net counts;
- DRC status/counts when fixture-backed evidence is present;
- moved refs/groups;
- blocked refs/groups;
- validation issue deltas.

## DRC Evidence Contract

The retry loop should consume DRC evidence through an adapter interface rather
than coupling retry directly to real KiCad CLI.

Required adapter behavior:

- accept a generated project or attempt output path;
- return status, issue count, blocking count, and structured issues;
- support fake fixture-backed responses for deterministic tests;
- support optional real KiCad CLI invocation through existing check plumbing;
- expose missing evidence distinctly from skipped evidence;
- never require network access.

Suggested result shape:

```go
type AttemptDRCEvidence struct {
    Status EvidenceStatus
    IssueCount int
    BlockingCount int
    Issues []reports.Issue
    Artifacts []reports.Artifact
}
```

## Ranking And Convergence Semantics

Best-attempt selection should prefer attempts in this order:

1. No required DRC failures when DRC is required.
2. Fewer board-validation blocking issues.
3. Fewer DRC blocking issues.
4. More routed required nets.
5. Fewer failed required nets.
6. Higher route quality score.
7. Higher placement quality score.
8. Earlier attempt for deterministic tie-breaking.

The system must record why an attempt was selected.

Stop reasons should remain bounded and explainable:

- `routed`;
- `max_attempts`;
- `non_improving_retry`;
- `repeated_placement_state`;
- `no_eligible_hints`;
- `no_safe_adjustment`;
- `drc_regression`;
- `board_validation_regression`;
- `context_canceled`.

## Safety Requirements

- Fixed, unowned, imported, or unsupported refs must remain immovable.
- Hard constraints, edge connectors, mounting holes, and board outlines must be
  preserved.
- Retry must stop when an attempt creates new required blockers and policy says
  to stop on new blockers.
- Best-attempt output must never hide evidence from rejected attempts.
- Optional DRC failures must not be silently ignored.
- Required DRC failures must prevent a clean/ready claim.

## Testing Strategy

Default tests:

- no real KiCad CLI;
- fixture-backed fake DRC adapter;
- deterministic larger generated fixtures;
- selected-field assertions rather than brittle full snapshots where possible;
- explicit attempt ranking tests;
- CLI JSON evidence tests;
- board validation and DRC delta tests.

Optional tests:

- real KiCad DRC smoke test gated by environment variable;
- local-only larger generated board check when `kicad-cli` is configured.

## Documentation Requirements

Update:

- README placement/routing retry section;
- README KiCad DRC caveats if real DRC evidence improves;
- `specs/ROADMAP.md` near-term sequence after implementation;
- any CLI examples that show `routing_retry` evidence.

Docs must state:

- retry remains opt-in;
- default tests do not require KiCad;
- DRC evidence is optional unless explicitly required;
- larger-board convergence evidence is a confidence gate, not fabrication
  readiness.

## Risks

### Fixture Complexity

Risk: Larger generated fixtures become hard to maintain.

Mitigation:

- keep fixture metadata explicit;
- use selected-field assertions;
- generate fixtures from structured requests where possible;
- keep local deterministic fake DRC evidence.

### Overclaiming DRC Quality

Risk: Passing fake DRC evidence is mistaken for real KiCad proof.

Mitigation:

- clearly label fake/fixture-backed evidence;
- keep real KiCad evidence optional and separately surfaced;
- do not mark fabrication readiness from retry alone.

### Retry Oscillation

Risk: Larger boards expose oscillating retry adjustments.

Mitigation:

- preserve repeated-state hashing;
- track opposing adjustment history;
- add non-improving and regression stop reasons.

### Slow Tests

Risk: Larger fixtures slow normal test runs.

Mitigation:

- keep fixtures compact enough for unit tests;
- isolate optional real KiCad checks;
- avoid full filesystem export unless needed for DRC adapter tests.

## Acceptance Criteria

- Larger generated full-board fixtures exercise placement-routing retry.
- Attempt histories include route, validation, and DRC evidence.
- Best-attempt selection can reject a later attempt that worsens validation or
  DRC evidence.
- CLI JSON exposes stable convergence and selected-attempt evidence.
- Default tests pass without KiCad.
- Optional real KiCad DRC smoke coverage is available but skipped by default.
- README and roadmap reflect the implemented larger-board convergence state.
