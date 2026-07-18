# Implementation Plan

## Phase 1: Baseline And Contract

- Audit the existing headphone-only selector, Class AB block, simulation,
  layout policy, fixture harness, and fabrication exporter.
- Check in this specification and a requirement-to-evidence checklist.
- Add deterministic golden-report scaffolding before promotion claims.

Exit: every specification requirement has an identified code path and test
target; known unsupported speaker behavior remains blocked.

## Phase 2: Concrete Component Intelligence

- Add reviewed audio op-amp, driver-pair, power-resistor, and relay records.
- Extend component validation for driver and relay application evidence.
- Extend complementary output selection to the bounded power application.
- Test pin maps, lifecycle, ratings, SOA, thermal data, companion requirements,
  substitutions, and fail-closed incomplete records.

Exit: the 10 W envelope selects a complete concrete component set while
inadequate or unreviewed alternatives block.

## Phase 3: Speaker Power Topology And Protection

- Add a BCE-aware complementary power-stage block with drivers, bias spreader,
  emitter resistors, current limiting, and Zobel network.
- Add a speaker-isolation and DC-protection block with delayed engagement,
  supply-fault release, relay clamp, and explicit load return.
- Add semantic landmarks and deterministic operations for both blocks.
- Add block verification manifests and negative parameter tests.

Exit: reusable blocks generate complete, deterministic, connected transaction
operations with no fixture-specific behavior.

## Phase 4: Electrical, Distortion, Load, Thermal, And SOA Gates

- Add a speaker-amplifier validation request with resistive and reactive load
  cases, THD budget, tolerance corners, heatsink model, transient thermal
  cases, and protection timing.
- Calculate power, clipping, driver demand, phase margin, THD, copper current,
  thermal paths, and SOA/fault margins.
- Reject invalid numbers and unsafe corners with stable issue codes and paths.
- Extend simulation artifacts and runner measurements for speaker-specific
  output power, THD, load phase, clipping, current limit, and protection timing.

Exit: all declared positive cases pass all gates and the required unsafe cases
block for the intended reason.

## Phase 5: Layout And Mechanical Evidence

- Add a speaker-power layout profile and required constraint categories.
- Quantify star return, Kelvin sense, feedback, decoupling, high-current width,
  thermal coupling, device symmetry, heatsink keepout, mounting access, and
  orientation constraints.
- Add geometry-derived verification for copper width and physical placement.

Exit: incomplete or unquantified layout evidence blocks; the generated power
stage provides every required category.

## Phase 6: Real KiCad Fixture And Fabrication Package

- Add `class_ab_speaker_10w_protected` request and pass metadata.
- Iterate only through generic placement/routing/writer corrections until the
  board has clean ERC, strict DRC, connectivity, and route-completion evidence.
- Require writer-correctness and zero-diff replay.
- Export and validate BOM, CPL, Gerbers, drills, manifest, readiness report,
  and physical rules against a declared manufacturer profile.

Exit: the real KiCad fixture and fabrication package pass without allowlists or
fixture-specific code.

## Phase 7: Preservation, Review, And Publication

- Run Class A, headphone Class AB, USB-C, speaker, fabrication, and full-suite
  regressions.
- Generate and verify the capability report and SHA-256 sidecar.
- Update documentation and the AI-readiness matrix with exact supported and
  unsupported boundaries.
- Stage changes, run Prism, fix every actionable finding, and repeat until
  clean.
- Commit, push, and verify GitHub Actions.

Exit: all specification evidence is present, Prism is clean, GitHub Actions is
green, and the repository has no uncommitted changes.
