# Block KiCad Layout Evidence Spec

## Summary

Raise newer built-in timing and protection block verification from
schematic/structural confidence toward PCB-backed and KiCad-backed confidence.
The target blocks are:

- `crystal_oscillator`
- `canned_oscillator`
- `reset_programming_header`
- `esd_protection`
- `reverse_polarity_protection`

These blocks already emit schematic transactions, PCB realization metadata, or
partial local-route/timing evidence. The current gap is that their verification
manifests mostly prove expected symbols, nets, placements, and structural
metadata, not that generated block projects can survive writer checks, board
validation, and optional KiCad ERC/DRC checks with stable evidence.

This project adds stronger block verification evidence without claiming
fabrication readiness for arbitrary composed boards.

## Goals

- Make block verification manifests able to request and report PCB realization,
  writer correctness, board validation, and optional KiCad ERC/DRC evidence.
- Upgrade timing/protection block manifests where the current writer and
  validation stack can prove the claim deterministically.
- Keep KiCad-backed checks optional by default but strict when explicitly
  required by a manifest or command flag.
- Produce AI-readable evidence that distinguishes:
  - schematic semantics passed;
  - PCB fragment realization passed;
  - writer correctness passed;
  - internal board validation passed;
  - KiCad ERC/DRC was run, skipped, passed, or failed.
- Avoid false readiness claims when generated block projects still have known
  limitations such as unrouted global nets, intentionally partial placement, or
  missing KiCad CLI configuration.

## Non-Goals

- Do not require `kicad-cli` for normal unit tests.
- Do not make every block fabrication-ready.
- Do not broaden block topologies in this project.
- Do not solve all KiCad DRC issues for composed multi-block boards.
- Do not mutate imported user projects.
- Do not replace the existing block verification harness; extend it.

## Current State

The block verification suite already supports:

- loading checked-in manifest files;
- validating expected components, ports, nets, and PCB placement expectations;
- optional writer stage requests;
- optional ERC/DRC stage requests;
- generated project sentinels for cached external-check artifacts;
- CLI summary goldens for built-in verification.

Current gaps:

- Manifests have no first-class way to assert `RealizeBlockPCB` timing evidence
  or local-route evidence independently from project writer output.
- `ExpectedPCB.RequiredRoutes` currently checks route operations in block
  output summaries, which does not cover `RealizeBlockPCB` local-route IDs.
- Block verification output does not clearly summarize internal board
  validation results for generated block projects.
- Some newer manifests intentionally stay at schematic evidence even when the
  block has PCB realization metadata.
- KiCad CLI checks are optional but not yet applied consistently as promoted
  evidence for timing/protection seed blocks.
- Known limitations are not consistently reflected in manifest evidence levels.

## Evidence Levels

Existing evidence levels remain:

- `definition_only`
- `schematic_verified`
- `transfer_verified`
- `pcb_verified`
- `erc_drc_verified`
- `reference_verified`

This project clarifies how to use them for block evidence:

- `schematic_verified`: schematic operations match expected roles, ports, nets,
  and component metadata.
- `pcb_verified`: schematic evidence plus PCB realization evidence, writer
  correctness, and internal board validation pass for the generated block
  project under declared allowances.
- `erc_drc_verified`: `pcb_verified` plus required KiCad ERC/DRC checks pass or
  only report explicitly allowed findings.

If KiCad CLI is unavailable, manifests that require ERC/DRC must fail unless
the command explicitly allows optional checks.

## Manifest Extensions

Add or formalize expected fields under `expected.pcb`:

```json
{
  "pcb": {
    "placements": [],
    "required_local_routes": [],
    "timing_fixtures": [],
    "require_realization": true,
    "require_board_validation": true,
    "allow_unrouted": true
  }
}
```

Field semantics:

- `require_realization`: run `RealizeBlockPCB` and fail if blocking issues
  occur.
- `required_local_routes`: assert realized local-route IDs emitted by
  `RealizeBlockPCB`.
- `timing_fixtures`: assert realized timing fixture IDs and optional satisfied
  state.
- `require_board_validation`: run internal board validation against the
  generated project when writer output exists.
- `allow_unrouted`: preserve existing behavior for partial block fragments.

Existing `required_routes` remains scoped to route operations written in block
transaction summaries. It must not be reused for PCB realization local-route
IDs.

## Verification Pipeline

For each manifest:

1. Validate manifest shape and block request.
2. Instantiate block and assert schematic semantics.
3. If PCB expectations request realization, run `RealizeBlockPCB`.
4. Assert realized placements, local routes, timing fixtures, and timing
   findings.
5. If writer checks are requested, generate a project and run writer
   correctness.
6. If board validation is requested, validate the generated PCB/project with
   internal board validation.
7. If ERC/DRC is requested, run KiCad checks according to manifest and CLI
   policy.
8. Emit stage summaries and artifacts with stable paths for AI consumers.

## Target Block Upgrades

### Crystal Oscillator

Expected upgrade:

- Require PCB realization.
- Assert `crystal_loop` timing fixture is satisfied.
- Assert local routes:
  - `xtal1_load`
  - `xtal2_load`
  - `load_caps_ground`
- Keep writer/DRC optional until generated crystal fixtures are known clean
  across KiCad versions.

### Canned Oscillator

Expected upgrade:

- Require PCB realization.
- Assert `canned_oscillator_core` timing fixture is satisfied.
- Assert local routes:
  - `osc_vcc_decoupling`
  - `osc_gnd_decoupling`
  - `osc_enable_pullup`
  - `osc_enable_vcc`
- Assert decoupling/control evidence appears in timing output.

### Reset Programming Header

Expected upgrade:

- Require PCB realization for the default ISP plus reset-switch mode.
- Assert `reset_programming_path` timing fixture is satisfied.
- Assert local routes:
  - `reset_pullup_to_header`
  - `reset_switch_to_header_ground`
- Add a separate UART/no-switch verification case if needed to prove
  conditional realization does not reference absent roles.

### ESD Protection

Expected upgrade:

- Require PCB realization where metadata is present.
- Assert placements and local routes that represent connector-adjacent shunt
  behavior.
- If entry-anchor modeling is not yet sufficient, keep KiCad DRC optional and
  document the blocker in manifest notes.

### Reverse Polarity Protection

Expected upgrade:

- Require PCB realization where metadata is present.
- Assert source/load local route or power-path evidence.
- Keep KiCad DRC optional until power-path clearance/width rules are enforced
  end to end.

## CLI Behavior

`kicadai --json --builtins block verify` should include the new stages in
result summaries when manifests request them.

Expected stage names:

- `manifest`
- `instantiate`
- `semantic_assertions`
- `pcb_realization`
- `pcb_assertions`
- `writer`
- `board_validation`
- `erc_drc`

Skipped optional stages should report skipped status with a reason, not silent
absence, when the manifest requested the stage as optional evidence.

## Reporting Requirements

Stage summaries should expose:

- number of realized components;
- number of realized local routes;
- number of timing fixtures;
- number of timing findings;
- writer correctness status;
- board validation issue counts;
- KiCad ERC/DRC check status and artifact paths when run.

Issues should include:

- manifest case ID;
- block ID;
- stage path;
- refs and nets where available;
- suggested next action.

## Testing Requirements

- Unit tests for manifest validation of new PCB fields.
- Unit tests for `RunCase` realization assertions.
- Negative tests for missing local route and unsatisfied timing fixture.
- Built-in verification suite updated with upgraded manifests.
- CLI golden updates for built-in verification output.
- Full `go test ./...` passes without requiring KiCad CLI.
- Optional integration tests can require KiCad CLI only behind existing
  explicit flags or environment gates.

## Acceptance Criteria

- Built-in timing/protection manifests express stronger PCB evidence where the
  current writer can support it.
- `kicadai --json --builtins block verify` reports the additional evidence
  stages deterministically.
- Missing or unsatisfied PCB realization evidence fails with stable issue paths.
- Normal unit tests remain KiCad-CLI independent.
- README and roadmap accurately describe which evidence is implemented and
  which KiCad-backed checks remain optional or pending.

## Risks

- KiCad CLI output and DRC behavior can vary by installed KiCad version.
- Some single-block PCB projects may be intentionally incomplete and require
  `allow_unrouted`.
- Promoting evidence too aggressively could imply fabrication readiness before
  composed-board proof exists.
- Conditional block realization can create false failures if manifest requests
  are not aligned with block parameters.

## Open Questions

- Should `pcb_verified` require writer correctness for every block, or is
  `RealizeBlockPCB` plus board validation enough for partial fragments?
- Should KiCad-backed block verification artifacts be checked in as goldens or
  generated on demand only?
- Should ESD and reverse-polarity blocks wait for entry-anchor/power-path
  enforcement before being upgraded beyond realization evidence?
