# Route-Tree Branch Repair Implementation Plan

Date: 2026-07-01

## Objective

Make route-tree branch failures repairable by classifying VCC/SDA branch
blockers, feeding them into placement-routing retry, and ranking retry attempts
by route-tree completion evidence.

The target fixture is `i2c_sensor_breakout_candidate`, which currently reaches
route-tree-managed routing but remains `expected_fail` on branch pathfinding and
contact proof gaps.

## Phase 1: Branch Failure Audit And Golden Baseline

### Goals

- Freeze the current route-tree branch blocker shape in focused tests.
- Capture the exact VCC/SDA issue paths, nets, refs, and summary counts.
- Make future changes prove that blockers get narrower or route completion
  improves.

### Tasks

- Add focused tests around the I2C fixture routing stage.
- Assert `inter_block_route_trees.managed_nets` includes `GND`, `SCL`, `SDA`,
  and `VCC`.
- Assert branch-scoped issue paths are present for VCC/SDA legal-path blockers.
- Assert contact proof blockers still map to VCC/SDA.
- Add helper functions for extracting branch issue evidence by net and branch.
- Record baseline `inter_block_routing` and `inter_block_contacts` expectations
  with room for deterministic improvement.

### Acceptance

- Tests identify the current VCC/SDA route-tree branch blockers without KiCad.
- Tests fail if branch paths regress to generic `nets.SDA` / `nets.VCC`
  diagnostics.
- `go test ./internal/designworkflow -run 'RouteTree|I2CSensorBreakout' -count=1`
  passes.
- `go test ./...` passes.

### Review And Commit

- Run `prism review staged`.
- Fix high and medium findings.
- Commit message: `Audit route-tree branch repair blockers`.

## Phase 2: Branch Failure Classification Model

### Goals

- Introduce typed branch failure categories and repair hint evidence.
- Keep classification deterministic and independent of KiCad.

### Tasks

- Add branch failure category constants.
- Add branch repair hint and repair summary structs.
- Implement a classifier for:
  - other-net pad blockers;
  - keepouts;
  - board edge blockers;
  - existing copper blockers;
  - layer access;
  - via policy;
  - search exhaustion;
  - contact miss;
  - missing contact target;
  - graph split;
  - unsupported;
  - unknown.
- Preserve nearest obstacle evidence only where structured router issue fields
  expose it; otherwise keep original issue path/message evidence intact.
- Preserve original issue path and code.
- Add unit tests for each classification category that can be produced today.

### Acceptance

- VCC/SDA legal-path blockers classify into stable route-search or structured
  blocker categories without relying on brittle obstacle message parsing.
- Contact miss and missing-target blockers classify distinctly.
- Unknown messages fail closed into `unknown` without panic.
- Existing routing and contact tests remain stable.
- `go test ./internal/designworkflow -run 'BranchFailure|RouteTree|Contact' -count=1`
  passes.
- `go test ./...` passes.

### Review And Commit

- Run `prism review staged`.
- Fix high and medium findings.
- Commit message: `Classify route-tree branch failures`.

## Phase 3: Route-Tree Repair Summary

### Goals

- Expose compact route-tree repair evidence in the routing stage summary.
- Keep detailed evidence available through issues and typed helpers.

### Tasks

- Build repair hints from classified branch failures.
- Add `route_tree_repair` summary to routing stage summaries.
- Include:
  - branch failure count;
  - repairable failure count;
  - unrepairable failure count;
  - hint count;
  - affected nets;
  - affected refs.
- Ensure summary output is deterministic by sorting refs and nets.
- Add JSON stability tests for the summary struct.
- Add I2C fixture assertions for route-tree repair summary contents.

### Acceptance

- I2C summary names VCC/SDA and the affected refs.
- Connector/LED either has an empty/zero route-tree repair summary or no
  repairable failures.
- Existing `inter_block_routing`, `inter_block_route_trees`, and
  `inter_block_contacts` summaries remain backward-compatible.
- `go test ./internal/designworkflow -run 'RouteTreeRepair|I2C|ConnectorLED' -count=1`
  passes.
- `go test ./...` passes.

### Review And Commit

- Run `prism review staged`.
- Fix high and medium findings.
- Commit message: `Summarize route-tree repair hints`.

## Phase 4: Placement Retry Hint Integration

### Goals

- Feed route-tree branch repair hints into bounded placement-routing retry.
- Let retry move eligible branch refs away from blockers.

### Tasks

- Locate existing routing-diagnostic-to-placement-hint conversion.
- Add route-tree repair hints as another hint source.
- Map categories to retry dimensions:
  - other-net pad / existing copper -> spacing and fanout;
  - board edge / keepout -> edge or region adjustment;
  - layer access / via policy -> fanout/layer-access pressure;
  - search exhausted -> spacing and distance;
  - contact miss / missing target -> route/contact repair.
- Preserve mobility policy and hard constraints.
- Add tests proving route-tree hints are consumed into retry candidates.
- Add tests proving fixed refs are not moved.
- Add tests proving unsupported hints are reported but do not cause unsafe
  mutation.

### Acceptance

- Retry attempt history records eligible refs from branch hints.
- Route-tree hints can produce a changed placement when refs are movable.
- Fixed/hard-constrained components remain fixed.
- Existing placement retry goldens still pass.
- `go test ./internal/designworkflow -run 'Retry|RouteTreeRepair|Mobility' -count=1`
  passes.
- `go test ./...` passes.

### Review And Commit

- Run `prism review staged`.
- Fix high and medium findings.
- Commit message: `Feed route-tree repair hints into retry`.

## Phase 5: Route-Tree-Aware Retry Selection

### Goals

- Prefer retry attempts that improve route-tree completion, even if raw router
  status alone is unchanged.
- Avoid selecting attempts that create more disconnected or partial groups.

### Tasks

- Add route-tree completion scoring to retry attempt ranking:
  - complete groups;
  - partial groups;
  - blocked groups;
  - proven endpoints;
  - branches routed/completed;
  - graph component count;
  - contact misses;
  - issue count.
- Keep existing routing quality and board validation ranking as tie-breakers.
- Add tests for:
  - selecting an attempt with fewer blocked groups;
  - rejecting an attempt with worse contact proof;
  - deterministic tie-breaking.
- Ensure selected attempt summaries include route-tree evidence when present.

### Acceptance

- Retry selected attempt can be justified by route-tree improvement.
- Existing retry stop reasons remain stable.
- No fixture accidentally promotes without required validation evidence.
- `go test ./internal/designworkflow -run 'Retry|SelectedAttempt|RouteTree' -count=1`
  passes.
- `go test ./...` passes.

### Review And Commit

- Run `prism review staged`.
- Fix high and medium findings.
- Commit message: `Rank retry attempts by route-tree completion`.

## Phase 6: I2C Fixture Repair Run And Promotion Decision

### Goals

- Run the actual I2C fixture and update metadata based on evidence.
- Promote only if all required gates support promotion.

### Tasks

- Run:

```sh
go run ./cmd/kicadai \
  --request examples/design/kicad-backed/i2c_sensor_breakout_candidate.json \
  --output examples/.generated/i2c_sensor_breakout_candidate \
  --overwrite \
  design create
```

- Inspect:
  - routing stage status;
  - `inter_block_route_trees`;
  - `route_tree_repair`;
  - routing retry attempt history;
  - selected attempt reason;
  - promotion report;
  - project write/writer correctness/validation/KiCad stages.
- If route completion succeeds and downstream gates support it:
  - update metadata readiness toward `candidate`;
  - update expected stages/artifacts;
  - update README and roadmap.
- If it still blocks:
  - keep `expected_fail`;
  - update known gaps with the selected retry blocker evidence;
  - ensure blockers are narrower than pre-repair VCC/SDA branch failures.

### Acceptance

- Fixture metadata matches actual command output.
- Generated outputs under `examples/.generated` remain ignored and unstaged.
- I2C either promotes correctly or documents selected retry blockers.
- `go test ./...` passes.

### Review And Commit

- Run `prism review staged`.
- Fix high and medium findings.
- Commit message: `Update I2C route-tree repair evidence`.

## Phase 7: Documentation And Closeout

### Goals

- Explain how route-tree repair evidence should be interpreted by AI agents and
  humans.

### Tasks

- Update `docs/layout-routing.md` with:
  - branch failure categories;
  - `route_tree_repair` summary;
  - retry interaction;
  - current limitations.
- Update `examples/design/kicad-backed/README.md`.
- Update `specs/ROADMAP.md`.
- Add notes to this spec if any open question becomes a follow-up.
- Run:
  - focused design workflow tests;
  - routing adapter tests if pad/layer evidence changes;
  - `go test ./...`;
  - optional I2C fixture command.

### Acceptance

- Docs do not overclaim DRC-clean routing.
- Roadmap names the next blocker after route-tree repair.
- Prism review has no unresolved high or medium findings.

### Review And Commit

- Run `prism review staged`.
- Fix high and medium findings.
- Commit message: `Document route-tree branch repair`.

## Risks

- Nearest obstacle kind/source remains limited until raw router issues expose
  structured obstacle fields.
- Placement retry may improve VCC while worsening SDA unless ranking accounts
  for all route-tree groups.
- Route-tree branch order may become a meaningful variable once cross-net
  existing copper is considered.
- The current grid router may still be unable to complete the I2C fixture even
  after repair hints improve placement.

## Completion Definition

This project is complete when:

- route-tree branch blockers classify into stable repair categories;
- route-tree repair hints are visible in routing summaries;
- bounded placement-routing retry consumes those hints;
- selected retry attempts consider route-tree completion evidence;
- the I2C fixture is either promoted or documents narrower selected-retry
  blockers;
- docs and roadmap reflect the actual state.
