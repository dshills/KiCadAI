# Protection Entry Anchor And Power-Path Realization Plan

## Overview

This plan implements route-through and power-path evidence for the ESD and
reverse-polarity protection blocks. Each phase should be implemented, tested,
reviewed with Prism, and committed before moving to the next phase.

## Phase 1: Entry Anchor Data Model

### Goal

Add PCB realization metadata for block-boundary entry anchors without changing
existing component-to-component route behavior.

### Work

- Add `PCBEntryAnchor` to `internal/blocks/realization.go`.
- Add `EntryAnchors []PCBEntryAnchor` to `PCBRealization`.
- Add `RealizedPCBEntryAnchor` and `EntryAnchors` to
  `BlockPCBRealizationResult`.
- Extend `RouteEndpoint` to support anchor or port endpoints while preserving
  current `component_role` plus `pin` behavior.
- Add validation helpers for endpoint mode:
  - component endpoint: `component_role` and `pin`;
  - anchor endpoint: `anchor_id`;
  - port endpoint: `port`;
  - exactly one endpoint mode must be present.
- Validate anchors:
  - ID required and unique;
  - port exists on the block;
  - net template, when provided, is non-empty and stable;
  - placement is finite and layer is valid;
  - conditional `When` still uses existing behavior.

### Tests

- Add tests in `internal/blocks/realization_test.go` for:
  - valid anchor metadata;
  - duplicate anchor IDs;
  - unknown anchor port;
  - endpoint with no mode;
  - endpoint with conflicting component and anchor modes.

### Acceptance

- Existing block realization tests pass unchanged.
- Existing local routes using `component_role` and `pin` remain valid.
- Invalid anchor metadata produces stable `block.<id>.pcb_realization...`
  issue paths.

### Commands

```sh
go test ./internal/blocks
```

### Commit

`Add PCB entry anchor realization model`

## Phase 2: Anchor-Aware Realization

### Goal

Teach `RealizeBlockPCB` to realize anchors and resolve anchor-based local
routes.

### Work

- Emit `RealizedPCBEntryAnchor` values for anchors whose `When` condition
  matches.
- Resolve anchor net names from `NetTemplate`.
- Build an anchor lookup map by ID.
- Update local-route realization to resolve:
  - component-to-component;
  - anchor-to-component;
  - component-to-anchor;
  - anchor-to-anchor.
- Compute route length using anchor coordinates and current waypoint logic.
- Add stable route endpoint output for anchors:
  - prefer explicit transaction endpoint fields if they already exist;
  - otherwise use a synthetic stable ref such as `@anchor:<id>` with the anchor
    ID as pin metadata.
- Ensure anchor route operations are deterministic.
- Return blocking issues for local routes that reference missing or inactive
  anchors.

### Tests

- Add focused realization tests for:
  - anchor emission with origin offsets;
  - anchor-to-component route length;
  - component-to-anchor route length with waypoints;
  - missing anchor route endpoint issue;
  - conditional anchors skipped consistently with conditional routes.

### Acceptance

- Realized output includes anchors and route evidence.
- Existing route length tests continue to pass.
- Anchor routes appear in `realization.local_routes`.

### Commands

```sh
go test ./internal/blocks
```

### Commit

`Realize anchor-based block local routes`

## Phase 3: ESD Protection Anchors And Routes

### Goal

Upgrade `esd_protection` to model connector-adjacent signal entry and short TVS
ground return evidence.

### Work

- Add ESD entry anchors:
  - `signal_entry` for `SIGNAL` / `signal`;
  - `ground_return` for `GND` / `gnd`.
- Add ESD local routes:
  - `esd_signal_entry_to_tvs`;
  - `esd_tvs_to_ground`.
- Set route widths and waypoints conservatively.
- Update ESD constraints and unsupported behavior notes:
  - remove entry-anchor unsupported note;
  - keep surge/capacitance and KiCad-backed proof limitations.
- Update ESD block tests to assert anchors and local routes.

### Tests

- `go test ./internal/blocks`

### Acceptance

- `RealizeBlockPCB(esd_protection)` emits two anchors and two local routes.
- No blocking realization issues for default ESD params.
- Unsupported behavior no longer says entry anchors are missing.

### Commit

`Add ESD entry anchor local routes`

## Phase 4: Reverse-Polarity Power-Path Routes

### Goal

Upgrade `reverse_polarity_protection` to model raw input entry, protected output
exit, and width-aware power-path local routes.

### Work

- Add reverse-polarity entry anchors:
  - `vin_raw_entry` for `VIN_RAW` / `vin_raw`;
  - `vin_protected_exit` for `VIN_PROTECTED` / `vin_protected`.
- Add local routes:
  - `raw_input_to_diode`;
  - `diode_to_protected_output`.
- Set route width to the existing current-path seed policy, initially 0.6 mm.
- Keep current min-width constraint and align route width with it.
- Update unsupported behavior notes:
  - remove raw connector anchor missing note;
  - keep thermal and MOSFET/ideal-diode limitations.
- Update reverse-polarity block tests to assert anchors, routes, and widths.

### Tests

- `go test ./internal/blocks`

### Acceptance

- `RealizeBlockPCB(reverse_polarity_protection)` emits two anchors and two local
  routes.
- Route widths reflect the current power-path width policy.
- No blocking realization issues for default params.

### Commit

`Add reverse-polarity power-path routes`

## Phase 5: Verification Manifest And Golden Updates

### Goal

Make protection block verification assert the new local-route evidence.

### Work

- Update `esd_protection_5v` manifest:
  - add `required_local_routes` for the two ESD local routes.
- Update `reverse_polarity_schottky` manifest:
  - add `required_local_routes` for the two power-path routes.
- Keep `require_realization: true`.
- Do not add `require_board_validation` yet unless generated project validation
  is stable with anchor evidence.
- Refresh block verification goldens.

### Tests

```sh
go test ./internal/blocks/verification ./cmd/kicadai
go test ./cmd/kicadai -run TestRunBlockVerificationGoldens -update-block-verification-goldens
go test ./internal/blocks/verification ./cmd/kicadai
```

### Acceptance

- Built-in block verification passes.
- CLI summary remains deterministic.
- ESD and reverse-polarity manifests fail if required route IDs disappear.

### Commit

`Verify protection block local route evidence`

## Phase 6: Workflow Evidence Integration

### Goal

Ensure AI-facing workflow outputs expose the new anchor and power-path evidence.

### Work

- Review `design create` block-planning and realization summaries.
- Ensure entry anchors do not break JSON serialization or summary generation.
- Add or update tests/goldens where block-planning output enumerates:
  - required local routes;
  - unsupported behaviors;
  - realization readiness.
- If current workflow output already carries this evidence through generic
  realization fields, add regression tests only.

### Tests

```sh
go test ./internal/designworkflow ./cmd/kicadai
```

### Acceptance

- Generated workflow evidence remains stable.
- Protection blocks expose their new local-route IDs to AI callers.
- No fabrication-readiness claim is introduced by this phase.

### Commit

`Expose protection route evidence in workflows`

## Phase 7: Documentation And Roadmap Update

### Goal

Document the new protection anchor and power-path evidence accurately.

### Work

- Update README circuit-block section.
- Update `docs/circuit-block-verification.md` if manifest examples or anchor
  discovery need clarification.
- Update `specs/ROADMAP.md`:
  - move entry-anchor/power-path realization from remaining work to current
    foundation;
  - keep KiCad-backed DRC proof and thermal/surge limitations explicit.
- Mention that entry anchors are evidence until board-level composition binds
  them to actual connector pads or board-edge features.

### Tests

```sh
go test ./...
```

### Acceptance

- Full test suite passes.
- Docs state what is proven and what is not.
- Roadmap next gaps remain clear.

### Commit

`Document protection anchor route evidence`

## Implementation Notes

- Keep field names ASCII and JSON-compatible.
- Prefer additive schema changes over renaming existing fields.
- Avoid requiring KiCad CLI in normal tests.
- Do not add `require_board_validation` to protection manifests unless the
  writer and board validator can handle anchor evidence without false physical
  pad assumptions.
- If transaction endpoints cannot represent anchors cleanly, keep anchor route
  operations evidence-only and document the limitation in the realization
  output and README.
