# Generated Design PCB Net Assignment Implementation Plan

Date: 2026-06-28

## Objective

Close the generated design PCB net-assignment gap so generated boards carry
consistent net tables, pad net codes, and copper net codes before writer
correctness and optional KiCad validation run.

The immediate success target is to move the optional KiCad-backed LED design
past the current missing pad/copper net assignment blocker, then apply the same
path to connector/LED and I2C fixtures as far as current component and routing
evidence allows.

## Phase 1: Audit Current Net Assignment Failures

### Tasks

- Add focused test helpers that run the optional design example generation path
  without requiring KiCad.
- Capture the current writer correctness failure categories for:
  - LED indicator KiCad-backed fixture;
  - connector/LED KiCad-backed fixture;
  - I2C sensor breakout KiCad-backed fixture.
- Normalize failure assertions around categories, not brittle full text.
- Document which failures are pad-net, copper-net, route-endpoint, or zone-net
  related.

### Acceptance

- Tests reproduce the current blocker without invoking KiCad.
- The failing categories are visible in deterministic test output or golden
  evidence.
- No behavior changes are made in this phase.
- `go test ./...` passes.

### Review And Commit

- Review staged changes with `prism review staged`.
- Commit message: `Add generated design net assignment audit`

## Phase 2: Canonical Generated Net Table Builder

### Tasks

- Add a generated-design net table builder in the workflow layer or a shared
  internal package used by the workflow layer.
- Collect net names from:
  - schematic connections;
  - block ports;
  - required local routes;
  - generated power and ground nets;
  - explicit route endpoints;
  - zone intent where present.
- Normalize net names using KiCad-compatible display names.
- Reserve code `0` for no-net.
- Assign deterministic positive net codes.
- Return diagnostics for aliases, empty names, duplicates, and conflicts.

### Acceptance

- Unit tests cover deterministic ordering, no-net handling, repeated names,
  power nets, block-local nets, and route-derived nets.
- The same request produces identical net codes across repeated runs.
- Net table diagnostics are deterministic.
- `go test ./...` passes.

### Review And Commit

- Review staged changes with `prism review staged`.
- Commit message: `Add generated design net table builder`

## Phase 3: Footprint Pad Net Assignment

### Tasks

- Add pad assignment from selected component evidence into generated PCB
  footprints.
- Resolve pad assignments using:
  - verified component pinmaps;
  - block component pin definitions;
  - selected component ID and variant ID;
  - hydrated footprint pad summaries;
  - schematic-to-PCB transfer bindings;
  - explicit block port bindings.
- Attach both net name and net code to generated pads.
- Emit diagnostics for missing pinmap, ambiguous pad, missing footprint pad,
  intentionally unused pin, and conflicting pin evidence.
- Keep unknown active-component pins blocked unless the block explicitly marks
  them unused.

### Acceptance

- LED/resistor/connector fixture pads receive valid net names and net codes
  when evidence is available.
- Missing or ambiguous pin evidence produces actionable diagnostics.
- Writer correctness no longer reports missing pad net code for the simple LED
  fixture once its mappings are known.
- `go test ./...` passes.

### Review And Commit

- Review staged changes with `prism review staged`.
- Commit message: `Assign generated footprint pad nets`

## Phase 4: Copper, Route, Via, And Zone Net Assignment

### Tasks

- Assign route and local-route copper from route endpoint net identity.
- Ensure tracks and vias reference net codes from the canonical board net
  table.
- Assign zones from explicit zone intent or block power-plane intent.
- Validate copper endpoints against assigned pad nets when endpoints are known.
- Add route mismatch diagnostics when a copper object bridges pads from
  different nets.

### Acceptance

- Generated tracks/local routes for LED and connector/LED fixtures carry valid
  net codes.
- Copper objects referencing unknown nets fail before KiCad.
- Endpoint pad/copper mismatches fail with operation-correlated diagnostics.
- Zone assignment is either valid or explicitly diagnosed as unsupported.
- `go test ./...` passes.

### Review And Commit

- Review staged changes with `prism review staged`.
- Commit message: `Assign generated copper nets`

## Phase 5: Workflow Evidence And CLI Reporting

### Tasks

- Add net assignment evidence to design workflow summaries.
- Include counts for:
  - net table entries;
  - assigned pads;
  - unresolved pads;
  - assigned copper objects;
  - unresolved copper objects;
  - mismatched assignments.
- Persist evidence with generated project artifacts where the workflow already
  writes validation or repair bundles.
- Surface concise CLI JSON fields under the existing design workflow output
  structure.

### Acceptance

- `kicadai design create` output includes generated net assignment evidence.
- Evidence is stable enough for golden tests.
- Default human-facing documentation stays concise and points to evidence
  fields rather than dumping full internal traces.
- `go test ./...` passes.

### Review And Commit

- Review staged changes with `prism review staged`.
- Commit message: `Expose generated net assignment evidence`

## Phase 6: Optional KiCad-Backed Fixture Promotion

### Tasks

- Run optional KiCad-backed design workflow tests when
  `KICADAI_KICAD_CLI` is configured.
- Promote the LED fixture from `expected_fail` to `candidate` or `pass` only
  after internal validation and configured KiCad checks justify it.
- Promote connector/LED if it passes the same evidence threshold.
- Keep I2C as `expected_fail` if remaining blockers are deeper than net
  assignment, and update its expected failure category.
- Update fixture metadata and generated evidence expectations.

### Acceptance

- At least one optional KiCad-backed fixture progresses beyond the current
  pad/copper net assignment blocker.
- Any fixture left as `expected_fail` has a narrower documented reason.
- Optional tests still skip cleanly without KiCad.
- `go test ./...` passes.

### Review And Commit

- Review staged changes with `prism review staged`.
- Commit message: `Promote KiCad-backed design examples with net evidence`

## Phase 7: Regression And Golden Coverage

### Tasks

- Add golden snapshots for generated net tables.
- Add golden snapshots for pad assignment evidence.
- Add golden snapshots for copper assignment evidence.
- Add regression tests that fail if generated pads or copper lose net codes.
- Add a small negative fixture for ambiguous or missing pinmap evidence.
- Update README and roadmap with the new status and remaining blockers.

### Acceptance

- Regressions prevent missing pad/copper net assignment from returning.
- Golden evidence is deterministic and reviewable.
- README describes the net assignment guarantee at a high level.
- ROADMAP reflects fixture promotions and remaining generated-board gaps.
- `go test ./...` passes.

### Review And Commit

- Review staged changes with `prism review staged`.
- Commit message: `Add generated design net assignment regressions`

## Implementation Notes

- Prefer using existing component, block, footprint resolver, and writer
  correctness models before adding new abstractions.
- Keep KiCad-backed validation optional and environment-gated.
- Do not hide unresolved mappings by assigning arbitrary net codes.
- Do not weaken writer correctness to make fixtures pass.
- Preserve imported-project round-trip behavior.

## Done Definition

- The generated design workflow has one canonical PCB net assignment path.
- Generated footprints and copper objects use net codes that match the board
  net table.
- Unknown mappings fail with actionable diagnostics.
- Optional KiCad-backed examples make measurable progress.
- The default test suite remains KiCad-independent.
- Prism review has been run for each implementation phase.
- Each implementation phase is committed independently.
