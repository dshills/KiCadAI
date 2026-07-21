# Simulation-Grounded Closed-Loop Synthesis Completion Audit

Date: 2026-07-21

Status: complete inside the bounded registered-capability, reviewed-model, and
checked-in-catalog envelope described by the specification. This is not an
arbitrary-circuit, unrestricted-SPICE, RF, mains, or fabrication-signoff claim.

## Requirement Evidence

| Specification area | Authoritative implementation and tests | Result |
|---|---|---|
| Strict behavioral v3 contract and frozen neutral corpus | `internal/architecturesearch`, `internal/architecturesearch/testdata/simulation_grounded_closed_loop_corpus`, strict decode/search/corpus tests | pass |
| Trusted planning, semantic bindings, and model provenance | `internal/closedloopsynthesis/planned_simulation_resolver.go`, `resolved_simulation_contracts.go`, `data/model-provenance/registry.json`, package tests | pass |
| DC, AC, nonlinear, transient, startup, noise, stability, distortion, thermal, and bounded corners | `internal/simmodel`, `internal/compositionlowering/closed_loop_simulation.go`, solver and lowering tests | pass |
| Deterministic candidate evaluation, diagnosis, repair, bounded stops, and replay | `internal/closedloopsynthesis`, runner/planning/promotion tests, frozen double-generation comparisons | pass |
| Workflow integration and fail-closed promotion | `internal/compositionlowering/promotion_test.go`, `internal/designworkflow/explicit_simulation_test.go` | pass |
| Physical route completion and exact emitted-copper clearance | `internal/routing/validation.go`, `internal/designworkflow/explicit_pcb.go`; final transaction repair covers acute same-net junctions and clearance-conflicting free-space layer transitions | pass |
| KiCad writer fidelity and zero normalized round trip | `internal/libraryresolver`, `internal/kicadfiles/pcb`, `internal/placement`; installed-KiCad corpus promotion | pass |
| Class-A and Class-AB behavioral promotion | frozen `class_a_amplifier.json` and `class_ab_amplifier.json` requirements, optional installed-KiCad promotion subtests | pass |
| Existing protected USB-C regressions | optional installed-KiCad `usb_c_led_indicator_protected` and `usb_c_i2c_sensor_3v3_protected` subtests | pass |
| Prohibitions | production search found no corpus names, hashes, coordinates, route allowlists, or fixture-specific correction families; corpus neutrality tests remain authoritative | pass |

## Fresh Command Evidence

The completion run used KiCad CLI 10.0.3 and the installed KiCad symbol,
footprint, and template roots.

- `go test ./...`
- `go test ./internal/compositionlowering -run '^TestFrozenSimulationGroundedCorpusOptionalKiCadPromotion$' -count=1 -v`
- `go test ./internal/designworkflow -run '^TestDesignExamplesOptionalKiCadBackedTier$/(usb_c_led_indicator_protected|usb_c_i2c_sensor_3v3_protected)$' -count=1 -v`

The installed ten-design corpus run proves clean ERC, strict DRC, complete
connectivity and routing, writer correctness, deterministic replay, and zero
normalized schematic and PCB round-trip differences. The current-sense design
exercised the final emitted-copper transition repair: a free-space via and all
same-net layer endpoints attached to it moved together through deterministic
grid candidates until exact physical clearance improved without increasing any
other validation blocker. No corpus identity or circuit-specific geometry is
available to that repair.

## Remaining Boundary

Unsupported capabilities, unreviewed or out-of-domain models, ambiguous
semantic bindings, unbounded analysis requests, exhausted repair budgets, and
physical designs outside the router's measured envelope fail closed. Broader
AI-directed circuit design requires expanding those reviewed registries and
held-out evidence, not weakening promotion gates.
