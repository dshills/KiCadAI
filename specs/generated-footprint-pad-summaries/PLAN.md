# Generated Workflow Footprint Pad Summaries Implementation Plan

## Objective

Hydrate real footprint pad summaries for generated `design create` workflows so
routing, board validation, and placement-routing retry can operate on generated
boards instead of stopping at the missing-pad-summary boundary.

## Implementation Rules

- Commit each phase independently after Prism review.
- Keep default tests deterministic and independent of global KiCad installs.
- Reuse existing resolver and placement footprint hydration code before adding
  new abstractions.
- Do not satisfy connected components with arbitrary dummy pads.
- Preserve existing seed and design API behavior.
- Keep workflow/CLI evidence compact and stable.

## Phase 1: Trace Contracts And Add Failing Coverage

### Goal

Lock down the current failure and the exact routing adapter pad contract before
changing behavior.

### Work

- Trace generated component data from component selection through PCB
  realization, placement request construction, and routing adapter conversion.
- Document in code comments or tests whether `PadSummary.XMM/YMM` are
  footprint-local or board-absolute at the routing adapter boundary.
- Add focused tests that reproduce the generated workflow missing-pad blocker.
- Add tests around `routingadapters.RequestFromPlacement` for empty pads, local
  pad coordinates, pad dimensions, and net propagation.
- Add a small fixture assertion for the current `generated_led_rejected`
  boundary so later phases intentionally update it.

### Tests

- `go test ./internal/routingadapters ./internal/designworkflow`
- Existing full-board retry candidate tests still show the documented blocker.

### Acceptance

- The missing-pad workflow failure is covered by tests.
- The routing adapter coordinate and net contract is explicit.

### Commit

```text
Add generated pad summary contract tests
```

## Phase 2: Pad Hydration Result Model

### Goal

Create a reusable internal result shape for pad hydration evidence and issues.

### Work

- Add a small helper package or internal designworkflow helper for footprint pad
  hydration results.
- Represent source type, hydrated pad count, missing reason, affected ref,
  footprint ID, and issues.
- Add merge/summary helpers for workflow stage summaries.
- Ensure issues use existing `reports.Issue` taxonomy and include stable paths.
- Keep the helper independent from CLI output formatting.

### Tests

- Source-count summary ordering is deterministic.
- Missing footprint, missing pinmap, and empty resolver pad records produce
  blocking issues.
- Verified unsupported cases do not panic.

### Acceptance

- Later phases can attach pad hydration evidence without duplicating summary
  code.

### Commit

```text
Add pad hydration evidence model
```

## Phase 3: Resolver-Backed Pad Extraction

### Goal

Use existing KiCad footprint resolver records to produce placement pad summaries
with real geometry.

### Work

- Reuse `placement.BoundsFromFootprint` or
  `placement.HydrateComponentFootprint` where possible.
- Add a resolver-facing helper that accepts a footprint ID and returns bounds,
  pad summaries, source evidence, and issues.
- Add deterministic test footprint records or parser fixtures that include
  through-hole and SMD pads.
- Preserve pad names and dimensions exactly enough for routing and validation.
- Treat zero-size or unnamed connected pads as blocking issues.

### Tests

- Resolver footprint with pads hydrates positive bounds and pad summaries.
- Footprint with no pads blocks.
- Pad names are trimmed and stable.
- Pad geometry remains in the coordinate convention proven in Phase 1.

### Acceptance

- Generated components with resolver records can receive real pad geometry.

### Commit

```text
Add resolver backed pad summary hydration
```

## Phase 4: Pinmap-Based Net Assignment

### Goal

Attach generated net names to footprint pad summaries using verified selected
component pinmaps.

### Work

- Locate selected component evidence that links symbol pins to footprint pads.
- Build a deterministic map from schematic/generated endpoints to footprint pad
  names.
- Apply net names to hydrated `PadSummary` records.
- Produce blocking issues for connected pins that cannot map to footprint pads.
- Preserve unconnected pads with empty net names.
- Add duplicate/equivalent pad handling according to existing catalog evidence.

### Tests

- LED and resistor nets map to the expected pad names.
- Connector breakout nets map by pin number.
- Missing pinmap blocks with affected ref and pin path.
- Duplicate connected pin mappings block unless explicitly allowed.
- Unconnected pads remain present without false net assignment.

### Acceptance

- Routing receives pad summaries whose `Net` values match generated net
  endpoints.

### Commit

```text
Assign generated nets to hydrated footprint pads
```

## Phase 5: Integrate Hydration Into Generated Placement

### Goal

Populate `placement.Component.Pads` during generated workflow placement request
construction.

### Work

- Integrate pad hydration into the PCB realization or placement request adapter
  path used by `design create`.
- Preserve existing resolver-backed footprint bounds, graphics, and model
  hydration.
- Attach pad hydration summaries to the relevant workflow stage summary.
- Ensure components with blocking pad hydration issues still allow partial
  project artifacts to be written when safe.
- Keep routing blocked only for affected components with unresolved pad evidence.

### Tests

- Generated LED design placement request contains pad summaries for LED and
  resistor components.
- Generated connector or sensor request contains connector pad summaries.
- Routing stage no longer reports missing pad summaries for hydrated generated
  components.
- Existing design API `PlaceFootprint` tests still pass.

### Acceptance

- The generated workflow reaches routing diagnostics for seed hydrated designs.

### Commit

```text
Hydrate generated placement pads
```

## Phase 6: Verified Built-In Fallbacks

### Goal

Allow checked-in verified seed components to route when external resolver data
is unavailable, without inventing unsafe generic pads.

### Work

- Identify seed catalog footprints that need local fallback coverage for normal
  tests.
- Add explicit footprint pad templates only for verified seed records.
- Tie each fallback to component identity, footprint ID, and pinmap evidence.
- Mark evidence source as `verified_template`.
- Block all other unresolved footprints.

### Tests

- Seed LED/resistor/connector/regulator fallback templates hydrate when resolver
  data is absent.
- Mismatched footprint ID does not use a fallback template.
- Fallback pads carry expected names, dimensions, and nets.
- Unknown active components remain blocked.

### Acceptance

- Default tests can prove generated pad hydration without relying on a local
  KiCad library install.

### Commit

```text
Add verified pad summary fallbacks
```

## Phase 7: Full-Board Retry Candidate Upgrade

### Goal

Convert the generated full-board retry candidate from a documented pad-summary
rejection into real generated-board routing evidence.

### Work

- Rename or update `generated_led_rejected` metadata to reflect the new expected
  behavior.
- Update workflow boundary tests to assert the absence of the pad-summary
  blocker.
- Assert routing either succeeds, produces real route diagnostics, or enters the
  bounded retry loop according to the candidate fixture.
- Add selected assertions for retry summary fields when retry is enabled.
- Keep pad-backed seed fixtures as separate regression coverage.

### Tests

- Full-board generated candidate no longer contains the
  `component has no footprint pad summaries for routing` issue.
- Retry summary, route metrics, and stage status are deterministic.
- Existing spacing, distance, and safe-stop fixtures still pass.

### Acceptance

- Placement-routing retry evidence includes at least one true generated
  workflow candidate.

### Commit

```text
Upgrade generated full board retry candidate
```

## Phase 8: CLI Evidence And Documentation

### Goal

Expose the new pad hydration behavior to CLI users and document the resolved
roadmap gap.

### Work

- Add selected-field CLI tests for pad hydration summary output.
- Update README with current generated workflow routing behavior.
- Update `specs/ROADMAP.md` to move this blocker from remaining work into the
  implemented foundation or near-term completed notes.
- Add a short troubleshooting note for unresolved footprint pad evidence.

### Tests

- CLI JSON output includes stable pad hydration evidence for a seed generated
  design.
- Documentation examples parse if they include request JSON.
- `go test ./...`

### Acceptance

- Users and AI agents can see whether generated routing had trusted pad
  evidence.

### Commit

```text
Document generated pad hydration evidence
```

## Final Verification

For the final phase, run:

```sh
go test ./...
prism review staged
```

The project is ready to move on when the generated workflow no longer blocks on
missing pad summaries and the full-board retry corpus includes generated-board
routing evidence.

